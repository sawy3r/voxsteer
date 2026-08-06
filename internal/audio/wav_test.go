package audio

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestEncodeWAV(t *testing.T) {
	got, err := EncodeWAV([]float32{-1, 0, 1})
	if err != nil {
		t.Fatal(err)
	}
	if string(got[:4]) != "RIFF" || string(got[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header: %q", got[:12])
	}
	if size := binary.LittleEndian.Uint32(got[40:44]); size != 6 {
		t.Fatalf("data size = %d, want 6", size)
	}
	if sample := int16(binary.LittleEndian.Uint16(got[48:50])); sample != 32767 {
		t.Fatalf("last sample = %d, want 32767", sample)
	}
}

func TestDecodeWAVRoundTrip(t *testing.T) {
	wav, err := EncodeWAV([]float32{-1, -0.25, 0, 0.25, 1})
	if err != nil {
		t.Fatal(err)
	}
	samples, err := DecodeWAV(wav, 5)
	if err != nil {
		t.Fatal(err)
	}
	want := []float32{-1, -0.25, 0, 0.25, 1}
	for i := range want {
		if math.Abs(float64(samples[i]-want[i])) > 1.0/32767 {
			t.Fatalf("samples[%d] = %f, want %f", i, samples[i], want[i])
		}
	}
}

func TestDecodeWAVRejectsMalformedAndOversizedInput(t *testing.T) {
	wav, err := EncodeWAV([]float32{0, 0})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		data []byte
		max  int
	}{
		{name: "short", data: wav[:20], max: 2},
		{name: "trailing", data: append(append([]byte(nil), wav...), 0), max: 2},
		{name: "too many samples", data: wav, max: 1},
		{name: "negative limit", data: wav, max: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := DecodeWAV(test.data, test.max); err == nil {
				t.Fatal("DecodeWAV() error = nil")
			}
		})
	}
}

func FuzzDecodeWAV(f *testing.F) {
	wav, err := EncodeWAV([]float32{-1, 0, 1})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(wav)
	f.Add([]byte("not a WAV"))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = DecodeWAV(data, SampleRate*120)
	})
}
