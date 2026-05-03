//go:build windows

package audio

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"time"
	"unsafe"

	ole "github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// Windows audio format tag constants.
const (
	waveFormatPCM        uint16 = 1
	waveFormatIEEEFloat  uint16 = 3
	waveFormatExtensible uint16 = 0xFFFE
)

// windowsCapturer captures system audio via WASAPI loopback on the default
// render endpoint (i.e. whatever is currently playing through the speakers).
type windowsCapturer struct{}

// NewCapturer returns the Windows (WASAPI loopback) Capturer.
func NewCapturer() Capturer { return windowsCapturer{} }

func (windowsCapturer) DefaultDevice() (string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInit(); err != nil {
		return "", err
	}
	defer ole.CoUninitialize()

	de, err := newEnumerator()
	if err != nil {
		return "", err
	}
	defer de.Release()

	var dev *wca.IMMDevice
	if err := de.GetDefaultAudioEndpoint(uint32(wca.ERender), uint32(wca.EConsole), &dev); err != nil {
		return "", fmt.Errorf("get default render endpoint: %w", err)
	}
	dev.Release()
	return "default", nil
}

func (windowsCapturer) ListDevices() ([]string, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := coInit(); err != nil {
		return nil, err
	}
	defer ole.CoUninitialize()

	de, err := newEnumerator()
	if err != nil {
		return nil, err
	}
	defer de.Release()

	// DEVICE_STATE_ACTIVE = 0x1
	var dc *wca.IMMDeviceCollection
	if err := de.EnumAudioEndpoints(uint32(wca.ERender), 0x1, &dc); err != nil {
		return nil, fmt.Errorf("enumerate render endpoints: %w", err)
	}
	defer dc.Release()

	var count uint32
	if err := dc.GetCount(&count); err != nil {
		return nil, fmt.Errorf("get device count: %w", err)
	}

	names := make([]string, 0, int(count)+1)
	names = append(names, "default")
	for i := uint32(0); i < count; i++ {
		var dev *wca.IMMDevice
		if err := dc.Item(i, &dev); err != nil {
			continue
		}
		var id string
		if err := dev.GetId(&id); err == nil && id != "" {
			names = append(names, id)
		}
		dev.Release()
	}
	return names, nil
}

// Stream starts WASAPI loopback capture on the default render endpoint and
// streams IncrSamples-sized 16 kHz mono float32 chunks until ctx is cancelled.
// The device parameter is accepted for interface compatibility but ignored;
// capture always uses the default render endpoint.
func (windowsCapturer) Stream(ctx context.Context, _ string) (<-chan []float32, error) {
	ch := make(chan []float32, 64)
	setupErr := make(chan error, 1)

	go func() {
		// Lock to one OS thread for the lifetime of this goroutine so COM
		// apartment state remains consistent.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		if err := coInit(); err != nil {
			setupErr <- err
			return
		}
		defer ole.CoUninitialize()

		de, err := newEnumerator()
		if err != nil {
			setupErr <- err
			return
		}
		defer de.Release()

		var mmDev *wca.IMMDevice
		if err := de.GetDefaultAudioEndpoint(uint32(wca.ERender), uint32(wca.EConsole), &mmDev); err != nil {
			setupErr <- fmt.Errorf("get default render endpoint: %w", err)
			return
		}
		defer mmDev.Release()

		var ac *wca.IAudioClient
		// CLSCTX_ALL = 0x17
		if err := mmDev.Activate(wca.IID_IAudioClient, 0x17, nil, &ac); err != nil {
			setupErr <- fmt.Errorf("activate IAudioClient: %w", err)
			return
		}
		defer ac.Release()

		var wfx *wca.WAVEFORMATEX
		if err := ac.GetMixFormat(&wfx); err != nil {
			setupErr <- fmt.Errorf("get mix format: %w", err)
			return
		}

		// 200 ms buffer in 100-nanosecond units.
		const bufDuration wca.REFERENCE_TIME = 200 * 10_000
		if err := ac.Initialize(
			wca.AUDCLNT_SHAREMODE_SHARED,
			wca.AUDCLNT_STREAMFLAGS_LOOPBACK,
			bufDuration, 0, wfx, nil,
		); err != nil {
			setupErr <- fmt.Errorf("initialize audio client: %w", err)
			return
		}

		var cc *wca.IAudioCaptureClient
		if err := ac.GetService(wca.IID_IAudioCaptureClient, &cc); err != nil {
			setupErr <- fmt.Errorf("get capture client service: %w", err)
			return
		}
		defer cc.Release()

		if err := ac.Start(); err != nil {
			setupErr <- fmt.Errorf("start audio client: %w", err)
			return
		}
		defer ac.Stop() //nolint:errcheck

		nativeRate := int(wfx.NSamplesPerSec)
		nativeChs := int(wfx.NChannels)
		nativeBits := int(wfx.WBitsPerSample)
		nativeTag := resolveFormatTag(wfx)

		// Signal successful setup before entering the capture loop.
		setupErr <- nil

		defer close(ch)

		// accumulator holds native-rate mono float32 samples not yet emitted.
		var acc []float32
		// How many native-rate samples correspond to one IncrSamples output chunk.
		neededNative := IncrSamples * nativeRate / Rate

		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}

			for {
				var packetLen uint32
				if err := cc.GetNextPacketSize(&packetLen); err != nil || packetLen == 0 {
					break
				}

				var data *byte
				var frames, flags uint32
				if err := cc.GetBuffer(&data, &frames, &flags, nil, nil); err != nil {
					return
				}

				if flags&wca.AUDCLNT_BUFFERFLAGS_SILENT == 0 && data != nil && frames > 0 {
					bytesPerFrame := nativeBits / 8 * nativeChs
					raw := unsafe.Slice(data, int(frames)*bytesPerFrame)
					acc = append(acc, toMonoFloat32(raw, int(frames), nativeChs, nativeBits, nativeTag)...)
				}
				cc.ReleaseBuffer(frames) //nolint:errcheck

				for len(acc) >= neededNative {
					chunk := resampleLinear(acc[:neededNative], nativeRate, Rate)
					acc = acc[neededNative:]
					select {
					case ch <- chunk:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	if err := <-setupErr; err != nil {
		return nil, err
	}
	return ch, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// coInit calls CoInitializeEx with COINIT_APARTMENTTHREADED (0x2).
// S_FALSE (0x1) means COM is already initialised on this thread — that is fine.
func coInit() error {
	if err := ole.CoInitializeEx(0, 0x2); err != nil {
		if oleErr, ok := err.(*ole.OleError); ok && oleErr.Code() == 0x1 {
			return nil // S_FALSE — already initialised
		}
		return fmt.Errorf("CoInitializeEx: %w", err)
	}
	return nil
}

func newEnumerator() (*wca.IMMDeviceEnumerator, error) {
	var de *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator, 0, 0x17, // CLSCTX_ALL
		wca.IID_IMMDeviceEnumerator, &de,
	); err != nil {
		return nil, fmt.Errorf("create IMMDeviceEnumerator: %w", err)
	}
	return de, nil
}

// resolveFormatTag returns the effective WAVE_FORMAT tag for wfx.
// For WAVE_FORMAT_EXTENSIBLE it inspects the SubFormat GUID embedded after
// the base WAVEFORMATEX struct.
func resolveFormatTag(wfx *wca.WAVEFORMATEX) uint16 {
	if wfx.WFormatTag != waveFormatExtensible || wfx.CbSize < 22 {
		return wfx.WFormatTag
	}
	// WAVEFORMATEXTENSIBLE layout (bytes from start of WAVEFORMATEX):
	//   0–17  WAVEFORMATEX (18 bytes)
	//   18–19 wSamples union (2 bytes)
	//   20–23 dwChannelMask (4 bytes)
	//   24–39 SubFormat GUID (16 bytes); Data1 is little-endian uint32 at [24..27]
	raw := (*[40]byte)(unsafe.Pointer(wfx))
	data1 := uint32(raw[24]) | uint32(raw[25])<<8 | uint32(raw[26])<<16 | uint32(raw[27])<<24
	switch data1 {
	case 1:
		return waveFormatPCM
	case 3:
		return waveFormatIEEEFloat
	}
	return wfx.WFormatTag
}

// toMonoFloat32 converts a raw WASAPI capture buffer to mono float32 samples.
// It handles PCM (16/24/32-bit) and IEEE float (32/64-bit) formats and
// downmixes multi-channel audio to mono by averaging across channels.
func toMonoFloat32(raw []byte, frames, channels, bitsPerSample int, tag uint16) []float32 {
	bytesPerSample := bitsPerSample / 8
	out := make([]float32, frames)
	for i := range frames {
		var sum float32
		for ch := 0; ch < channels; ch++ {
			offset := (i*channels + ch) * bytesPerSample
			sum += decodeSample(raw[offset:], bitsPerSample, tag)
		}
		out[i] = sum / float32(channels)
	}
	return out
}

func decodeSample(b []byte, bits int, tag uint16) float32 {
	switch tag {
	case waveFormatIEEEFloat:
		switch bits {
		case 32:
			u := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
			return math.Float32frombits(u)
		case 64:
			u := uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
				uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
			return float32(math.Float64frombits(u))
		}
	case waveFormatPCM:
		switch bits {
		case 16:
			s := int16(b[0]) | int16(b[1])<<8
			return float32(s) / 32768.0
		case 24:
			s := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16
			if s&0x800000 != 0 {
				s |= ^int32(0xFFFFFF)
			}
			return float32(s) / 8388608.0
		case 32:
			s := int32(b[0]) | int32(b[1])<<8 | int32(b[2])<<16 | int32(b[3])<<24
			return float32(s) / 2147483648.0
		}
	}
	return 0
}

// resampleLinear resamples samples from fromRate to toRate using linear
// interpolation.  Returns a new slice; the input slice is not modified.
func resampleLinear(samples []float32, fromRate, toRate int) []float32 {
	if fromRate == toRate {
		out := make([]float32, len(samples))
		copy(out, samples)
		return out
	}
	ratio := float64(fromRate) / float64(toRate)
	outLen := int(math.Round(float64(len(samples)) / ratio))
	if outLen == 0 {
		return nil
	}
	out := make([]float32, outLen)
	for i := range out {
		pos := float64(i) * ratio
		idx := int(pos)
		frac := float32(pos - float64(idx))
		if idx+1 < len(samples) {
			out[i] = samples[idx]*(1-frac) + samples[idx+1]*frac
		} else if idx < len(samples) {
			out[i] = samples[idx]
		}
	}
	return out
}
