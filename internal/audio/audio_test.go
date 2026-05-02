package audio

import (
	"math"
	"testing"
)

func TestIsSilent_silence(t *testing.T) {
	if !IsSilent(make([]float32, 16000), 0.005, 0.9) {
		t.Error("all-zero samples should be silent")
	}
}

func TestIsSilent_loudSineWave(t *testing.T) {
	samples := make([]float32, 16000)
	for i, n := 0, len(samples); i < n; i++ {
		samples[i] = float32(math.Sin(2 * math.Pi * float64(i) / 100.0))
	}
	if IsSilent(samples, 0.005, 0.9) {
		t.Error("loud sine wave should not be silent")
	}
}

func TestIsSilent_belowRMSThreshold(t *testing.T) {
	samples := make([]float32, 16000)
	for i := range samples {
		samples[i] = 0.001 // well below the 0.005 threshold
	}
	if !IsSilent(samples, 0.005, 0.9) {
		t.Error("signal below RMS threshold should be silent")
	}
}

func TestIsSilent_mostlySilentSamples(t *testing.T) {
	// 95% of samples are near-zero; only 5% are loud.
	samples := make([]float32, 16000)
	for i := 0; i < 800; i++ {
		samples[i] = 0.8
	}
	if !IsSilent(samples, 0.005, 0.9) {
		t.Error("mostly-silent samples should be detected as silent")
	}
}

func TestIsSilent_empty(t *testing.T) {
	if !IsSilent(nil, 0.005, 0.9) {
		t.Error("empty/nil samples should be silent")
	}
}

func TestPCMToFloat32_zeroInput(t *testing.T) {
	raw := make([]byte, 8) // 4 int16 samples, all zero
	out := PCMToFloat32(raw)
	if len(out) != 4 {
		t.Fatalf("expected 4 samples, got %d", len(out))
	}
	for i, v := range out {
		if v != 0 {
			t.Errorf("sample[%d] = %v, want 0", i, v)
		}
	}
}

func TestPCMToFloat32_maxPositive(t *testing.T) {
	// int16 max = 0x7FFF = 32767 → float32 ≈ +1.0
	raw := []byte{0xFF, 0x7F} // little-endian 32767
	out := PCMToFloat32(raw)
	if len(out) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(out))
	}
	if out[0] < 0.999 || out[0] > 1.0 {
		t.Errorf("max int16 should map to ~1.0, got %v", out[0])
	}
}

func TestPCMToFloat32_maxNegative(t *testing.T) {
	// int16 min = -32768 (0x8000) → float32 = -1.0
	raw := []byte{0x00, 0x80} // little-endian -32768
	out := PCMToFloat32(raw)
	if len(out) != 1 {
		t.Fatalf("expected 1 sample, got %d", len(out))
	}
	if out[0] < -1.001 || out[0] > -0.999 {
		t.Errorf("min int16 should map to ~-1.0, got %v", out[0])
	}
}

func TestPCMToFloat32_oddLength(t *testing.T) {
	// Odd byte count: last byte is ignored (truncated to complete int16 pairs).
	raw := make([]byte, 5) // 2 complete int16 + 1 trailing byte
	out := PCMToFloat32(raw)
	if len(out) != 2 {
		t.Errorf("expected 2 samples from 5 bytes, got %d", len(out))
	}
}
