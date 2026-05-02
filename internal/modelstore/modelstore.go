// Package modelstore handles discovery and downloading of whisper.cpp models.
package modelstore

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const baseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

// Model describes a whisper model available for download.
type Model struct {
	Name        string `json:"name"`         // file name, e.g. "ggml-large-v3-turbo.bin"
	DisplayName string `json:"display_name"` // human-readable label shown in the UI
	Description string `json:"description"`  // brief quality/speed note
	Size        string `json:"size"`         // human-readable download size
	SizeBytes   int64  `json:"size_bytes"`
	Recommended bool   `json:"recommended"`
	Downloaded  bool   `json:"downloaded"` // true if the file already exists on disk
}

var catalog = []Model{
	{Name: "ggml-tiny.bin", DisplayName: "Tiny", Description: "Fastest, lowest accuracy", Size: "75 MB", SizeBytes: 75_000_000},
	{Name: "ggml-base.bin", DisplayName: "Base", Description: "Fast", Size: "142 MB", SizeBytes: 142_000_000},
	{Name: "ggml-small.bin", DisplayName: "Small", Description: "Balanced speed and accuracy", Size: "466 MB", SizeBytes: 466_000_000},
	{Name: "ggml-medium.bin", DisplayName: "Medium", Description: "Good accuracy", Size: "1.5 GB", SizeBytes: 1_500_000_000},
	{Name: "ggml-large-v3-turbo.bin", DisplayName: "Large v3 Turbo", Description: "Fast and accurate", Size: "874 MB", SizeBytes: 874_000_000, Recommended: true},
	{Name: "ggml-large-v3.bin", DisplayName: "Large v3", Description: "Best accuracy", Size: "3.1 GB", SizeBytes: 3_100_000_000},
}

// Dir returns the platform-appropriate directory where whisper models are cached:
//   - Linux:   $XDG_CACHE_HOME/yaku/models  (default ~/.cache/…)
//   - macOS:   ~/Library/Caches/yaku/models
//   - Windows: %LocalAppData%\yaku\models
func Dir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache")
	}
	return filepath.Join(dir, "yaku", "models")
}

// Path returns the full path for the named model file.
func Path(name string) string {
	return filepath.Join(Dir(), name)
}

// Available returns the model catalog with Downloaded set based on what exists on disk.
func Available() []Model {
	dir := Dir()
	out := make([]Model, len(catalog))
	copy(out, catalog)
	for i, m := range out {
		_, err := os.Stat(filepath.Join(dir, m.Name))
		out[i].Downloaded = err == nil
	}
	return out
}

// Download fetches name into Dir(), reporting byte progress via onProgress.
// It writes to a .tmp file first and renames on success so the destination is
// never left in a partial state.  Returns the final path.
func Download(ctx context.Context, name string, onProgress func(received, total int64)) (string, error) {
	if !isKnown(name) {
		return "", fmt.Errorf("unknown model %q", name)
	}

	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		return "", fmt.Errorf("creating model directory: %w", err)
	}

	dest := Path(name)
	tmp := dest + ".tmp"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+name, nil)
	if err != nil {
		return "", err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned HTTP %d", resp.StatusCode)
	}

	f, err := os.Create(tmp)
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	defer func() {
		f.Close()
		os.Remove(tmp) // no-op after a successful rename
	}()

	var received int64
	total := resp.ContentLength
	buf := make([]byte, 32*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return "", fmt.Errorf("writing download: %w", writeErr)
			}
			received += int64(n)
			if onProgress != nil {
				onProgress(received, total)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("reading download: %w", readErr)
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}

	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dest); err != nil {
		return "", fmt.Errorf("finalising download: %w", err)
	}
	return dest, nil
}

func isKnown(name string) bool {
	for _, m := range catalog {
		if m.Name == name {
			return true
		}
	}
	return false
}
