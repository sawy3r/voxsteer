package audio

import (
	"encoding/binary"
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
