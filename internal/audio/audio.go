// Package audio provides platform-independent audio utilities and the Capturer
// interface for continuous, platform-native audio capture.
package audio

import (
	"context"
	"math"
)

// Sample-rate and channel constants expected by whisper.cpp.
const (
	Rate      = 16000
	Channels  = 1
	IncrSamples = Rate / 2 // 500 ms per streaming increment
)

// Capturer is the platform-independent interface for audio capture.
// Call NewCapturer() to get the implementation for the current OS.
type Capturer interface {
	// DefaultDevice returns the best loopback/monitor source for the current
	// platform (e.g. the PipeWire monitor sink on Linux, WASAPI loopback on
	// Windows, the system audio virtual device on macOS).
	DefaultDevice() (string, error)

	// ListDevices returns the names of all available capture sources.
	ListDevices() ([]string, error)

	// Stream starts a persistent audio capture goroutine and returns a channel
	// that receives IncrSamples-sized float32 chunks.  The goroutine exits and
	// the channel is closed when ctx is cancelled or a fatal error occurs.
	Stream(ctx context.Context, device string) (<-chan []float32, error)
}

// IsSilent returns true when the chunk contains no meaningful speech.
// Two criteria:  RMS below threshold, or >silentRatio fraction near-silent.
func IsSilent(samples []float32, threshold, silentRatio float32) bool {
	if len(samples) == 0 {
		return true
	}
	var sumSq float64
	nearSilent := 0
	halfThreshold := threshold * 0.5
	for _, s := range samples {
		sumSq += float64(s) * float64(s)
		if s > -halfThreshold && s < halfThreshold {
			nearSilent++
		}
	}
	rms := float32(math.Sqrt(sumSq / float64(len(samples))))
	if rms < threshold {
		return true
	}
	return float32(nearSilent)/float32(len(samples)) > silentRatio
}

// PCMToFloat32 converts raw little-endian signed-16-bit PCM bytes to [-1, 1] float32.
func PCMToFloat32(raw []byte) []float32 {
	n := len(raw) / 2
	out := make([]float32, n)
	for i := range out {
		s := int16(raw[i*2]) | int16(raw[i*2+1])<<8
		out[i] = float32(s) / 32768.0
	}
	return out
}
