//go:build darwin

package audio

import (
	"context"
	"fmt"
)

// darwinCapturer will implement CoreAudio system audio capture.
// TODO Stage 2: replace stubs with CoreAudio/AVFoundation or BlackHole integration.
type darwinCapturer struct{}

// NewCapturer returns the macOS Capturer.
func NewCapturer() Capturer { return darwinCapturer{} }

func (darwinCapturer) DefaultDevice() (string, error) {
	// TODO Stage 2: find the system audio loopback device (BlackHole or aggregate).
	return "default", nil
}

func (darwinCapturer) ListDevices() ([]string, error) {
	// TODO Stage 2: enumerate CoreAudio devices via AudioObjectGetPropertyData.
	return []string{"default"}, nil
}

func (darwinCapturer) Stream(_ context.Context, _ string) (<-chan []float32, error) {
	return nil, fmt.Errorf("audio capture not yet implemented on macOS")
}
