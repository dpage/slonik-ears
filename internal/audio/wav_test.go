package audio

import (
	"math"
	"testing"
)

func TestWAVRoundTrip(t *testing.T) {
	in := make([]float32, 1600)
	for i := range in {
		in[i] = float32(0.5 * math.Sin(float64(i)/10))
	}
	out, rate, err := DecodeWAV(EncodeWAV(in, SampleRate))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if rate != SampleRate {
		t.Fatalf("sample rate: got %d want %d", rate, SampleRate)
	}
	if len(out) != len(in) {
		t.Fatalf("length: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if diff := math.Abs(float64(in[i] - out[i])); diff > 1.0/32767*2 {
			t.Fatalf("sample %d differs by %v after a 16-bit round trip", i, diff)
		}
	}
}

func TestDecodeRejectsRubbish(t *testing.T) {
	if _, _, err := DecodeWAV([]byte("this is not a wav file, sorry")); err == nil {
		t.Fatal("expected an error for non-WAV input")
	}
}

func TestResampleLength(t *testing.T) {
	in := make([]float32, 48000)
	out := Resample(in, 48000, 16000)
	if len(out) != 16000 {
		t.Fatalf("got %d samples, want 16000", len(out))
	}
	if same := Resample(in, 16000, 16000); len(same) != len(in) {
		t.Fatal("resampling to the same rate should be a no-op")
	}
}
