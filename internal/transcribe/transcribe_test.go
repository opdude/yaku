package transcribe

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-audio/wav"
)

// modelPath returns the path to the test model, or skips if not available.
func testModelPath(t *testing.T) string {
	t.Helper()
	// Use the bundled test model from the whisper.cpp submodule
	candidates := []string{
		filepath.Join("..", "..", "third_party", "whisper.cpp", "models", "for-tests-ggml-base.bin"),
		os.Getenv("WHISPER_MODEL"),
	}
	for _, p := range candidates {
		if p != "" {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	t.Skip("no whisper model available; set WHISPER_MODEL or provide third_party/whisper.cpp/models/for-tests-ggml-base.bin")
	return ""
}

// sampleAudioPath returns a German sample WAV, or skips.
func sampleAudioPath(t *testing.T) string {
	t.Helper()
	// Use the bundled sample from go-whisper module
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	candidates := []string{
		filepath.Join(gopath, "pkg", "mod", "github.com", "mutablelogic", "go-whisper@v0.0.39", "samples", "de-podcast.wav"),
		filepath.Join("..", "..", "..", "go-whisper", "samples", "de-podcast.wav"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	t.Skip("no sample audio available")
	return ""
}

func loadWAV(t *testing.T, path string) []float32 {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open wav: %v", err)
	}
	defer f.Close()

	dec := wav.NewDecoder(f)
	buf, err := dec.FullPCMBuffer()
	if err != nil {
		t.Fatalf("decode wav: %v", err)
	}

	samples := make([]float32, len(buf.Data))
	for i, s := range buf.Data {
		samples[i] = float32(s) / float32(int32(1)<<(dec.BitDepth-1))
	}
	return samples
}

func TestNew_invalidPath(t *testing.T) {
	_, err := New("/nonexistent/path/model.bin")
	if err == nil {
		t.Error("expected error for nonexistent model path")
	}
}

func TestTranscriber_LoadAndClose(t *testing.T) {
	modelPath := testModelPath(t)
	tr, err := New(modelPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	tr.Close()
	tr.Close() // double close should not panic
}

func TestTranscribe_GermanAudio(t *testing.T) {
	// This test requires a real model with weights to produce output.
	// The bundled for-tests model has no tensors so it will return empty string.
	// Set WHISPER_MODEL_REAL to a real ggml model path for full integration testing.
	realModel := os.Getenv("WHISPER_MODEL_REAL")
	if realModel == "" {
		t.Skip("set WHISPER_MODEL_REAL=<path-to-real-ggml-model.bin> to run transcription integration test")
	}

	samplePath := sampleAudioPath(t)

	tr, err := New(realModel)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tr.Close()

	samples := loadWAV(t, samplePath)

	// Whisper expects 16kHz mono; take first 5 seconds worth
	chunkSize := 16000 * 5
	if len(samples) > chunkSize {
		samples = samples[:chunkSize]
	}

	text, dur, err := tr.Transcribe(samples, "de", nil)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	t.Logf("Transcribed (%v): %q", dur, text)
	if text == "" {
		t.Error("expected non-empty transcription for German podcast sample")
	}
}

func TestTranscribe_StubModel(t *testing.T) {
	// Exercises the full Transcribe code path using the stub test model.
	// The stub has no weights so transcription returns empty — that is expected here.
	modelPath := testModelPath(t)
	samplePath := sampleAudioPath(t)

	tr, err := New(modelPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tr.Close()

	samples := loadWAV(t, samplePath)
	chunkSize := 16000 * 5
	if len(samples) > chunkSize {
		samples = samples[:chunkSize]
	}

	_, dur, err := tr.Transcribe(samples, "de", nil)
	if err != nil {
		t.Fatalf("Transcribe with stub model: %v", err)
	}
	t.Logf("Stub model inference took %v", dur)
}

func TestTranscribe_Silence(t *testing.T) {
	modelPath := testModelPath(t)
	tr, err := New(modelPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer tr.Close()

	// Silence should produce empty or near-empty output
	samples := make([]float32, 16000*3)
	text, _, err := tr.Transcribe(samples, "de", nil)
	if err != nil {
		t.Fatalf("Transcribe silence: %v", err)
	}
	t.Logf("Transcribed silence: %q", text)
}
