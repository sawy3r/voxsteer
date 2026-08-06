package audio

import (
	"encoding/binary"
	"fmt"
)

// DecodeWAV decodes the canonical mono 16-bit PCM WAV format emitted by
// EncodeWAV. maxSamples bounds allocations and recording duration.
func DecodeWAV(data []byte, maxSamples int) ([]float32, error) {
	if len(data) < 44 {
		return nil, fmt.Errorf("WAV file is shorter than its 44-byte header")
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("audio is not a RIFF/WAVE file")
	}
	if string(data[12:16]) != "fmt " || binary.LittleEndian.Uint32(data[16:20]) != 16 {
		return nil, fmt.Errorf("WAV must contain a canonical PCM format chunk")
	}
	if binary.LittleEndian.Uint16(data[20:22]) != 1 {
		return nil, fmt.Errorf("WAV encoding must be PCM")
	}
	if binary.LittleEndian.Uint16(data[22:24]) != 1 {
		return nil, fmt.Errorf("WAV audio must be mono")
	}
	if binary.LittleEndian.Uint32(data[24:28]) != SampleRate {
		return nil, fmt.Errorf("WAV sample rate must be %d Hz", SampleRate)
	}
	if binary.LittleEndian.Uint32(data[28:32]) != SampleRate*2 ||
		binary.LittleEndian.Uint16(data[32:34]) != 2 ||
		binary.LittleEndian.Uint16(data[34:36]) != 16 {
		return nil, fmt.Errorf("WAV audio must use mono 16-bit PCM")
	}
	if string(data[36:40]) != "data" {
		return nil, fmt.Errorf("WAV must contain a canonical data chunk")
	}

	dataSize := int(binary.LittleEndian.Uint32(data[40:44]))
	if dataSize%2 != 0 || dataSize != len(data)-44 {
		return nil, fmt.Errorf("WAV data size does not match the file")
	}
	if int(binary.LittleEndian.Uint32(data[4:8])) != len(data)-8 {
		return nil, fmt.Errorf("WAV RIFF size does not match the file")
	}
	sampleCount := dataSize / 2
	if maxSamples < 0 || sampleCount > maxSamples {
		return nil, fmt.Errorf("WAV contains %d samples; maximum is %d", sampleCount, maxSamples)
	}

	samples := make([]float32, sampleCount)
	for i := range samples {
		raw := int16(binary.LittleEndian.Uint16(data[44+i*2 : 46+i*2]))
		if raw == -32768 {
			samples[i] = -1
		} else {
			samples[i] = float32(raw) / 32767
		}
	}
	return samples, nil
}
