package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config holds all user-configurable settings, persisted to YAML.
type Config struct {
	ModelPath      string `yaml:"model_path"`
	Language       string `yaml:"language"`        // source language code, e.g. "de"
	TargetLanguage string `yaml:"target_language"` // translation target, e.g. "en"
	OllamaURL      string `yaml:"ollama_url"`
	OllamaModel    string `yaml:"ollama_model"`
	AudioDevice    string `yaml:"audio_device"` // empty = auto-detect default device
	ChunkSeconds   int    `yaml:"chunk_seconds"` // minimum audio length before transcribing
}

// IsComplete returns true when all required fields are filled in.
func (c Config) IsComplete() bool {
	return c.ModelPath != "" && c.Language != "" &&
		c.OllamaURL != "" && c.OllamaModel != "" &&
		c.ChunkSeconds > 0
}

// Default returns sensible defaults. ModelPath is intentionally blank so the
// user is prompted to configure it on first run.
func Default() Config {
	return Config{
		Language:       "de",
		TargetLanguage: "en",
		OllamaURL:      "http://localhost:11434/api/generate",
		OllamaModel:    "translategemma:4b",
		ChunkSeconds:   3,
	}
}

// Path returns the platform-appropriate config file path:
//   - Linux:   $XDG_CONFIG_HOME/yaku/config.yaml  (default ~/.config/…)
//   - macOS:   ~/Library/Application Support/yaku/config.yaml
//   - Windows: %AppData%\yaku\config.yaml
func Path() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		// Fallback: use home directory directly if the config dir cannot be determined.
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "yaku", "config.yaml")
}

// Load reads the config from disk, merging missing fields with defaults.
// The second return value indicates whether a config file was found.
func Load() (Config, bool) {
	cfg := Default()
	data, err := os.ReadFile(Path())
	if err != nil {
		return cfg, false
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, false
	}
	return cfg, true
}

// Save writes cfg to disk, creating parent directories as needed.
func Save(cfg Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
