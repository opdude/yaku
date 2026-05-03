package transcribe

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	syswhisper "github.com/mutablelogic/go-whisper/sys/whisper"
)

// Transcriber wraps a loaded whisper model context.
type Transcriber struct {
	ctx       *syswhisper.Context
	backend   string // "CPU", "GPU (Vulkan)", "GPU (CUDA)", …
	modelPath string
}

// New loads a whisper model from modelPath with GPU acceleration if available.
// It detects the active backend by capturing whisper.cpp initialisation log lines.
func New(modelPath string) (*Transcriber, error) {
	params := syswhisper.DefaultContextParams()
	params.SetUseGpu(true)

	backend, ctx := loadWithBackendDetection(modelPath, params)
	if ctx == nil {
		return nil, fmt.Errorf("failed to load whisper model from %s", modelPath)
	}
	return &Transcriber{ctx: ctx, backend: backend, modelPath: modelPath}, nil
}

// Close releases model resources.
func (t *Transcriber) Close() {
	if t.ctx != nil {
		syswhisper.Whisper_free(t.ctx)
		t.ctx = nil
	}
}

// Backend returns a human-readable string for the compute backend in use,
// e.g. "CPU", "GPU (Vulkan)", "GPU (CUDA)".
func (t *Transcriber) Backend() string { return t.backend }

// ModelName returns just the filename of the loaded model.
func (t *Transcriber) ModelName() string { return filepath.Base(t.modelPath) }

// ModelPath returns the full path to the loaded model file.
func (t *Transcriber) ModelPath() string { return t.modelPath }

// Transcribe converts 16 kHz mono float32 samples to text in the given language.
// onSegment (may be nil) is called from the whisper decoder thread each time a
// new segment is ready — use it for partial/live display before the full result.
func (t *Transcriber) Transcribe(
	samples []float32,
	language string,
	onSegment func(text string),
) (string, time.Duration, error) {
	params := syswhisper.DefaultFullParams(syswhisper.SAMPLING_GREEDY)
	defer params.Close()

	params.SetLanguage(language)
	// On GPU runs the encoder is offloaded so fewer CPU threads are needed.
	// On CPU use all cores for maximum throughput.
	threads := runtime.NumCPU()
	if strings.HasPrefix(t.backend, "GPU") {
		threads = max(1, runtime.NumCPU()/4)
	}
	params.SetNumThreads(threads)
	params.SetNoContext(true)
	params.SetPrintProgress(false)
	params.SetPrintRealtime(false)
	params.SetPrintTimestamps(false)
	params.SetSingleSegment(false)
	params.SetNoTimestamps(true)

	if onSegment != nil {
		params.SetSegmentCallback(t.ctx, func(n int) {
			total := t.ctx.NumSegments()
			for i := total - n; i < total; i++ {
				if seg := strings.TrimSpace(t.ctx.SegmentText(i)); seg != "" {
					onSegment(seg)
				}
			}
		})
		defer params.SetSegmentCallback(t.ctx, nil)
	}

	start := time.Now()
	if err := syswhisper.Whisper_full(t.ctx, params, samples); err != nil {
		return "", 0, fmt.Errorf("whisper transcription: %w", err)
	}
	elapsed := time.Since(start)

	var sb strings.Builder
	for i := 0; i < t.ctx.NumSegments(); i++ {
		if text := strings.TrimSpace(t.ctx.SegmentText(i)); text != "" {
			if sb.Len() > 0 {
				sb.WriteByte(' ')
			}
			sb.WriteString(text)
		}
	}
	return sb.String(), elapsed, nil
}

// ── backend detection via log callback ────────────────────────────────────────

// logMu guards the global syswhisper log callback.
var logMu sync.Mutex

func loadWithBackendDetection(modelPath string, params syswhisper.ContextParams) (string, *syswhisper.Context) {
	logMu.Lock()
	defer logMu.Unlock()

	var logLines []string
	syswhisper.Whisper_log_set(func(_ syswhisper.LogLevel, text string) {
		logLines = append(logLines, text)
	})
	ctx := syswhisper.Whisper_init_from_file_with_params(modelPath, params)
	syswhisper.Whisper_log_set(nil) // restore default logging

	if ctx == nil {
		return "CPU", nil
	}

	backend := detectBackend(logLines)
	return backend, ctx
}

// detectBackend parses whisper.cpp init log lines to find which GPU backend was used.
// Actual log patterns (from whisper.cpp/ggml source):
//   GPU found:    "ggml_vulkan: Found 1 device(s):"
//   No GPU:       "ggml_vulkan: No devices found."
//                 "whisper_backend_init_gpu: no GPU found"
func detectBackend(lines []string) string {
	for _, line := range lines {
		switch {
		// Vulkan device present — line looks like "ggml_vulkan: Found N device(s):"
		case strings.Contains(line, "ggml_vulkan:") &&
			strings.Contains(line, "Found") &&
			!strings.Contains(line, "No devices"):
			return "GPU (Vulkan)"
		// CUDA device present
		case strings.Contains(line, "ggml_cuda:") &&
			(strings.Contains(line, "found") || strings.Contains(line, "Found")):
			return "GPU (CUDA)"
		// Metal (macOS)
		case strings.Contains(line, "ggml_metal:") &&
			!strings.Contains(strings.ToLower(line), "error"):
			return "GPU (Metal)"
		// HIP/ROCm
		case strings.Contains(line, "ggml_hip:") &&
			(strings.Contains(line, "found") || strings.Contains(line, "Found")):
			return "GPU (ROCm)"
		}
	}
	return "CPU"
}
