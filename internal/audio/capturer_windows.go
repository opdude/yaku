//go:build windows

package audio

import (
	"context"
	"fmt"
)

// windowsCapturer will implement WASAPI loopback capture.
// TODO Stage 2: replace stubs with go-wca WASAPI loopback implementation.
type windowsCapturer struct{}

// NewCapturer returns the Windows Capturer.
func NewCapturer() Capturer { return windowsCapturer{} }

func (windowsCapturer) DefaultDevice() (string, error) {
	// TODO Stage 2: use IMMDeviceEnumerator to find the default render endpoint.
	return "default", nil
}

func (windowsCapturer) ListDevices() ([]string, error) {
	// TODO Stage 2: enumerate WASAPI render + loopback endpoints.
	return []string{"default"}, nil
}

func (windowsCapturer) Stream(_ context.Context, _ string) (<-chan []float32, error) {
	return nil, fmt.Errorf("audio capture not yet implemented on Windows")
}
