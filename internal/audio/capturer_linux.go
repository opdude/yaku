//go:build linux

package audio

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// linuxCapturer implements Capturer using PulseAudio/PipeWire via parec/pactl.
type linuxCapturer struct{}

// NewCapturer returns the Linux (PipeWire/PulseAudio) Capturer.
func NewCapturer() Capturer { return linuxCapturer{} }

// DefaultDevice returns the monitor source for the default audio sink, which
// captures system audio (loopback) rather than a microphone input.
func (linuxCapturer) DefaultDevice() (string, error) {
	out, err := exec.Command("pactl", "get-default-sink").Output()
	if err != nil {
		return "", fmt.Errorf("pactl get-default-sink failed (is pipewire-pulse running?): %w", err)
	}
	sink := strings.TrimSpace(string(out))
	if sink == "" {
		return "", fmt.Errorf("pactl returned an empty default sink name")
	}
	return sink + ".monitor", nil
}

// ListDevices returns the names of all available PulseAudio/PipeWire sources.
func (linuxCapturer) ListDevices() ([]string, error) {
	out, err := exec.Command("pactl", "list", "sources", "short").Output()
	if err != nil {
		return nil, fmt.Errorf("pactl list sources: %w", err)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 {
			names = append(names, fields[1])
		}
	}
	return names, nil
}

// Stream starts a persistent parec subprocess that captures from device at
// 16 kHz mono s16le.  It sends IncrSamples-sized float32 chunks on the
// returned channel until ctx is cancelled or parec exits.
func (linuxCapturer) Stream(ctx context.Context, device string) (<-chan []float32, error) {
	cmd := exec.CommandContext(ctx, "parec",
		"--device", device,
		"--format=s16le",
		fmt.Sprintf("--rate=%d", Rate),
		fmt.Sprintf("--channels=%d", Channels),
		"--latency-msec=10",
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("parec pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("parec start: %w", err)
	}

	// Buffer 64 increments (~32 s) so a slow processor never blocks parec.
	ch := make(chan []float32, 64)
	go func() {
		defer func() {
			close(ch)
			cmd.Wait() //nolint:errcheck
		}()
		raw := make([]byte, IncrSamples*2) // 2 bytes per int16 sample
		for {
			if _, err := io.ReadFull(stdout, raw); err != nil {
				return
			}
			select {
			case ch <- PCMToFloat32(raw):
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}
