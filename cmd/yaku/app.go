package main

import (
	"context"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	appconfig "github.com/opdude/yaku/internal/config"
	"github.com/opdude/yaku/internal/audio"
	"github.com/opdude/yaku/internal/modelstore"
	"github.com/opdude/yaku/internal/transcribe"
	"github.com/opdude/yaku/internal/translate"
)

// bracketedRe matches whisper hallucinations emitted during silence: [MUSIC], (applause), ♪ etc.
var bracketedRe = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|♪+`)

func isHallucination(text string) bool {
	stripped := strings.TrimSpace(bracketedRe.ReplaceAllString(text, ""))
	stripped = strings.Trim(stripped, " .,!?-—")
	return stripped == ""
}

// App is the Wails application struct. Its exported methods become callable
// from the frontend JavaScript via the auto-generated bindings.
type App struct {
	ctx context.Context

	mu            sync.Mutex
	cfg           appconfig.Config
	transcriber   *transcribe.Transcriber
	ollamaContext string // rolling ~300-char tail of accumulated translation

	// Pipeline lifecycle
	cancelPipeline context.CancelFunc
	pipelineWg     sync.WaitGroup
	running        bool

	// Model download lifecycle
	cancelDownload context.CancelFunc
}

// startup is called by the Wails runtime once the window is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	runtime.WindowSetAlwaysOnTop(ctx, true)
	cfg, _ := appconfig.Load()
	a.cfg = cfg
}

// shutdown is called by the Wails runtime when the window closes.
func (a *App) shutdown(_ context.Context) {
	a.StopPipeline()
	a.CancelModelDownload()
	a.mu.Lock()
	if a.transcriber != nil {
		a.transcriber.Close()
		a.transcriber = nil
	}
	a.mu.Unlock()
}

// ── Frontend-callable methods ─────────────────────────────────────────────────

// GetConfig returns the current configuration.
func (a *App) GetConfig() appconfig.Config {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cfg
}

// SaveConfig persists a new configuration.
func (a *App) SaveConfig(cfg appconfig.Config) error {
	if err := appconfig.Save(cfg); err != nil {
		return err
	}
	a.mu.Lock()
	// Invalidate the cached transcriber if the model path changed.
	if cfg.ModelPath != a.cfg.ModelPath && a.transcriber != nil {
		a.transcriber.Close()
		a.transcriber = nil
	}
	a.cfg = cfg
	a.mu.Unlock()
	return nil
}

// GetAudioDevices returns available audio source names for the current platform.
func (a *App) GetAudioDevices() []string {
	names, _ := audio.NewCapturer().ListDevices()
	return names
}

// NeedsModelSetup returns true when no valid model file is configured, indicating
// the model setup panel should be shown before the user can start the pipeline.
func (a *App) NeedsModelSetup() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.ModelPath == "" {
		return true
	}
	_, err := os.Stat(a.cfg.ModelPath)
	return err != nil
}

// GetAvailableModels returns the list of known whisper models, each marked with
// whether it has already been downloaded to the local model cache.
func (a *App) GetAvailableModels() []modelstore.Model {
	return modelstore.Available()
}

// UseDownloadedModel switches the app to a locally cached model by catalog name
// (for example "ggml-large-v3-turbo.bin").
func (a *App) UseDownloadedModel(name string) error {
	path := modelstore.Path(name)
	if _, err := os.Stat(path); err != nil {
		return err
	}

	cfg := a.GetConfig()
	cfg.ModelPath = path
	return a.SaveConfig(cfg)
}

// DownloadModel starts downloading the named model in the background.
// Progress is reported via "model:progress" events; completion via "model:done"
// or "model:error".  Only one download runs at a time.
func (a *App) DownloadModel(name string) {
	a.mu.Lock()
	if a.cancelDownload != nil {
		a.mu.Unlock()
		return // already downloading
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelDownload = cancel
	a.mu.Unlock()

	go func() {
		defer func() {
			a.mu.Lock()
			a.cancelDownload = nil
			a.mu.Unlock()
		}()

		path, err := modelstore.Download(ctx, name, func(received, total int64) {
			runtime.EventsEmit(a.ctx, "model:progress", map[string]any{
				"name":     name,
				"received": received,
				"total":    total,
			})
		})

		if err != nil {
			if ctx.Err() != nil {
				return // cancelled by user — no event needed
			}
			runtime.EventsEmit(a.ctx, "model:error", map[string]string{"message": err.Error()})
			return
		}

		// Persist the new model path so the user doesn't have to configure it manually.
		a.mu.Lock()
		a.cfg.ModelPath = path
		cfg := a.cfg
		a.mu.Unlock()
		_ = appconfig.Save(cfg)

		runtime.EventsEmit(a.ctx, "model:done", map[string]string{
			"name": name,
			"path": path,
		})
	}()
}

// CancelModelDownload cancels any in-progress model download.
func (a *App) CancelModelDownload() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelDownload != nil {
		a.cancelDownload()
		a.cancelDownload = nil
	}
}

// StartPipeline loads the whisper model and starts the capture→transcribe→translate loop.
// Non-blocking; events are emitted as results arrive.
func (a *App) StartPipeline() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	cfg := a.cfg

	if a.transcriber == nil {
		t, err := transcribe.New(cfg.ModelPath)
		if err != nil {
			return err
		}
		a.transcriber = t
	}

	device := cfg.AudioDevice
	if device == "" {
		var err error
		device, err = audio.NewCapturer().DefaultDevice()
		if err != nil {
			return err
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelPipeline = cancel
	a.running = true
	a.ollamaContext = ""

	a.pipelineWg.Add(1)
	go a.runPipeline(ctx, device, cfg)

	return nil
}

// StopPipeline cancels the running pipeline and waits for it to finish.
func (a *App) StopPipeline() {
	a.mu.Lock()
	if !a.running || a.cancelPipeline == nil {
		a.mu.Unlock()
		return
	}
	cancel := a.cancelPipeline
	a.mu.Unlock()

	cancel()
	a.pipelineWg.Wait()

	a.mu.Lock()
	a.running = false
	a.cancelPipeline = nil
	a.mu.Unlock()

	a.emitStatus("stopped")
}

// IsRunning reports whether the pipeline is active.
func (a *App) IsRunning() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.running
}

// ── Pipeline goroutine ────────────────────────────────────────────────────────

func (a *App) runPipeline(ctx context.Context, device string, cfg appconfig.Config) {
	defer func() {
		a.pipelineWg.Done()
		a.mu.Lock()
		a.running = false
		a.mu.Unlock()
	}()

	streamCh, err := audio.NewCapturer().Stream(ctx, device)
	if err != nil {
		a.emitError("audio stream: " + err.Error())
		return
	}

	a.emitStatus("listening")

	var audioBuf []float32
	const maxBuf = audio.Rate * 10 // 10-second rolling window

	for {
		select {
		case <-ctx.Done():
			return
		case chunk, ok := <-streamCh:
			if !ok {
				return
			}
			audioBuf = append(audioBuf, chunk...)
		}

		if len(audioBuf) > maxBuf {
			audioBuf = audioBuf[len(audioBuf)-maxBuf:]
		}

		minSamples := cfg.ChunkSeconds * audio.Rate
		if len(audioBuf) < minSamples || audio.IsSilent(audioBuf, 0.005, 0.9) {
			continue
		}

		toProcess := make([]float32, len(audioBuf))
		copy(toProcess, audioBuf)
		// Keep 0.5 s overlap so consecutive batches share a sentence boundary.
		if len(audioBuf) > audio.IncrSamples {
			audioBuf = audioBuf[len(audioBuf)-audio.IncrSamples:]
		} else {
			audioBuf = audioBuf[:0]
		}

		a.mu.Lock()
		t := a.transcriber
		a.mu.Unlock()
		if t == nil {
			return
		}

		a.emitStatus("transcribing")
		text, _, err := t.Transcribe(toProcess, cfg.Language, func(seg string) {
			runtime.EventsEmit(a.ctx, "translation:segment", map[string]string{"text": seg})
		})
		if err != nil {
			a.emitError("transcription: " + err.Error())
			a.emitStatus("listening")
			continue
		}
		if text == "" || isHallucination(text) {
			a.emitStatus("listening")
			continue
		}
		runtime.EventsEmit(a.ctx, "translation:source", map[string]string{"text": text})

		a.emitStatus("translating")

		a.mu.Lock()
		ollamaCtx := a.ollamaContext
		a.mu.Unlock()

		var sb strings.Builder
		err = translate.Stream(
			cfg.OllamaURL, cfg.OllamaModel,
			text, cfg.Language, cfg.TargetLanguage, ollamaCtx,
			func(token string) {
				sb.WriteString(token)
				runtime.EventsEmit(a.ctx, "translation:token", map[string]string{"token": token})
			},
		)

		translation := strings.TrimSpace(sb.String())

		if err != nil {
			a.emitError("translation: " + err.Error())
		} else {
			a.mu.Lock()
			combined := a.ollamaContext
			if combined != "" {
				combined += " "
			}
			a.ollamaContext = tailStr(combined+translation, 300)
			a.mu.Unlock()
		}

		runtime.EventsEmit(a.ctx, "translation:done", map[string]string{
			"source":      text,
			"translation": translation,
		})
		a.emitStatus("listening")
	}
}

func (a *App) emitStatus(status string) {
	runtime.EventsEmit(a.ctx, "pipeline:status", map[string]string{"status": status})
}

func (a *App) emitError(msg string) {
	runtime.EventsEmit(a.ctx, "translation:error", map[string]string{"message": msg})
}

func tailStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && s[cut] != ' ' {
		cut++
	}
	return strings.TrimSpace(s[cut:])
}
