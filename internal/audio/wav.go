package audio

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
)

const SampleRate = 16000

// EncodeWAV converts mono float32 PCM samples to a standard 16-bit PCM WAV.
func EncodeWAV(samples []float32) ([]byte, error) {
	if uint64(len(samples)) > (math.MaxUint32-36)/2 {
		return nil, fmt.Errorf("audio is too large for a WAV file")
	}

	var out bytes.Buffer
	out.Grow(44 + len(samples)*2)

	dataSize := uint32(len(samples) * 2)
	out.WriteString("RIFF")
	_ = binary.Write(&out, binary.LittleEndian, uint32(36)+dataSize)
	out.WriteString("WAVE")
	out.WriteString("fmt ")
	_ = binary.Write(&out, binary.LittleEndian, uint32(16))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint32(SampleRate))
	_ = binary.Write(&out, binary.LittleEndian, uint32(SampleRate*2))
	_ = binary.Write(&out, binary.LittleEndian, uint16(2))
	_ = binary.Write(&out, binary.LittleEndian, uint16(16))
	out.WriteString("data")
	_ = binary.Write(&out, binary.LittleEndian, dataSize)

	for _, sample := range samples {
		v := math.Max(-1, math.Min(1, float64(sample)))
		_ = binary.Write(&out, binary.LittleEndian, int16(math.Round(v*32767)))
	}
	return out.Bytes(), nil
}
