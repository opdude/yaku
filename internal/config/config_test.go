package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Language != "de" {
		t.Errorf("Language = %q, want %q", cfg.Language, "de")
	}
	if cfg.ChunkSeconds != 3 {
		t.Errorf("ChunkSeconds = %d, want 3", cfg.ChunkSeconds)
	}
	if cfg.ModelPath != "" {
		t.Errorf("default ModelPath should be empty (triggers first-run setup), got %q", cfg.ModelPath)
	}
}

func TestIsComplete_missingModelPath(t *testing.T) {
	cfg := Default()
	if cfg.IsComplete() {
		t.Error("config without ModelPath should not be complete")
	}
}

func TestIsComplete_allFields(t *testing.T) {
	cfg := Default()
	cfg.ModelPath = "/models/ggml-large-v3.bin"
	if !cfg.IsComplete() {
		t.Error("fully populated config should be complete")
	}
}

func TestIsComplete_missingOllamaURL(t *testing.T) {
	cfg := Default()
	cfg.ModelPath = "/models/model.bin"
	cfg.OllamaURL = ""
	if cfg.IsComplete() {
		t.Error("config without OllamaURL should not be complete")
	}
}

func TestSaveAndLoad_roundtrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	orig := Config{
		ModelPath:    "/models/ggml-large-v3.bin",
		Language:     "fr",
		OllamaURL:    "http://localhost:11434/api/generate",
		OllamaModel:  "mistral",
		AudioDevice:  "hw:0",
		ChunkSeconds: 7,
	}
	if err := Save(orig); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, exists := Load()
	if !exists {
		t.Fatal("Load should return exists=true after saving")
	}
	if loaded.ModelPath != orig.ModelPath {
		t.Errorf("ModelPath: got %q, want %q", loaded.ModelPath, orig.ModelPath)
	}
	if loaded.Language != orig.Language {
		t.Errorf("Language: got %q, want %q", loaded.Language, orig.Language)
	}
	if loaded.ChunkSeconds != orig.ChunkSeconds {
		t.Errorf("ChunkSeconds: got %d, want %d", loaded.ChunkSeconds, orig.ChunkSeconds)
	}
}

func TestLoad_noFile(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	cfg, exists := Load()
	if exists {
		t.Error("Load should return exists=false when no file is present")
	}
	// Defaults should be populated even when file is absent.
	if cfg.Language != "de" {
		t.Errorf("default Language = %q, want %q", cfg.Language, "de")
	}
}

func TestPath_underConfigDir(t *testing.T) {
	// Path() must always place the config file inside the platform config dir.
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		t.Skip("UserConfigDir not available:", err)
	}
	p := Path()
	if !strings.HasPrefix(p, cfgDir) {
		t.Errorf("Path() = %q, should be under UserConfigDir %q", p, cfgDir)
	}
	if !strings.HasSuffix(p, filepath.Join("yaku", "config.yaml")) {
		t.Errorf("Path() = %q, should end with yaku/config.yaml", p)
	}
}
