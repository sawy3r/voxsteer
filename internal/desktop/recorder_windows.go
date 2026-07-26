//go:build desktop && windows

package desktop

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"runtime"
	"time"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

// Recorder captures 16 kHz mono PCM from the default Windows communications
// microphone through WASAPI. It is pure Go and builds for Windows amd64/arm64.
type Recorder struct{}

func (Recorder) Record(ctx context.Context, onLevel func(float32)) ([]float32, error) {
	if onLevel == nil {
		onLevel = func(float32) {}
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	if err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED); err != nil {
		return nil, fmt.Errorf("initialize COM: %w", err)
	}
	defer ole.CoUninitialize()

	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(
		wca.CLSID_MMDeviceEnumerator,
		0,
		wca.CLSCTX_ALL,
		wca.IID_IMMDeviceEnumerator,
		&enumerator,
	); err != nil {
		return nil, fmt.Errorf("create audio device enumerator: %w", err)
	}
	defer enumerator.Release()

	var device *wca.IMMDevice
	if err := enumerator.GetDefaultAudioEndpoint(wca.ECapture, wca.ECommunications, &device); err != nil {
		// Some systems do not assign a communications device.
		if err := enumerator.GetDefaultAudioEndpoint(wca.ECapture, wca.EConsole, &device); err != nil {
			return nil, fmt.Errorf("get default microphone: %w", err)
		}
	}
	defer device.Release()

	var client *wca.IAudioClient
	if err := device.Activate(wca.IID_IAudioClient, wca.CLSCTX_ALL, nil, &client); err != nil {
		return nil, fmt.Errorf("activate microphone: %w", err)
	}
	defer client.Release()

	format := &wca.WAVEFORMATEX{
		WFormatTag:      1, // WAVE_FORMAT_PCM
		NChannels:       1,
		NSamplesPerSec:  16000,
		NAvgBytesPerSec: 32000,
		NBlockAlign:     2,
		WBitsPerSample:  16,
	}
	flags := uint32(wca.AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM | wca.AUDCLNT_STREAMFLAGS_SRC_DEFAULT_QUALITY)
	if err := client.Initialize(
		wca.AUDCLNT_SHAREMODE_SHARED,
		flags,
		wca.REFERENCE_TIME(10_000_000), // one second in 100 ns units
		0,
		format,
		nil,
	); err != nil {
		return nil, fmt.Errorf("initialize WASAPI capture: %w", err)
	}

	var capture *wca.IAudioCaptureClient
	if err := client.GetService(wca.IID_IAudioCaptureClient, &capture); err != nil {
		return nil, fmt.Errorf("get WASAPI capture service: %w", err)
	}
	defer capture.Release()

	if err := client.Start(); err != nil {
		return nil, fmt.Errorf("start WASAPI capture: %w", err)
	}
	defer client.Stop()

	samples := make([]float32, 0, 16000*30)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	lastLevel := time.Now()
	var levelSumSquares float64
	var levelSamples int

	for {
		select {
		case <-ctx.Done():
			return samples, nil
		case <-ticker.C:
			for {
				var packetFrames uint32
				if err := capture.GetNextPacketSize(&packetFrames); err != nil {
					return nil, fmt.Errorf("query microphone packet: %w", err)
				}
				if packetFrames == 0 {
					break
				}

				var data *byte
				var frames, bufferFlags uint32
				var devicePosition, qpcPosition uint64
				if err := capture.GetBuffer(
					&data,
					&frames,
					&bufferFlags,
					&devicePosition,
					&qpcPosition,
				); err != nil {
					return nil, fmt.Errorf("read microphone packet: %w", err)
				}

				if bufferFlags&wca.AUDCLNT_BUFFERFLAGS_SILENT != 0 {
					samples = append(samples, make([]float32, frames)...)
					levelSamples += int(frames)
				} else if frames > 0 {
					raw := unsafe.Slice(data, int(frames)*2)
					for offset := 0; offset < len(raw); offset += 2 {
						value := int16(binary.LittleEndian.Uint16(raw[offset : offset+2]))
						sample := float32(value) / math.MaxInt16
						samples = append(samples, sample)
						levelSumSquares += float64(sample * sample)
						levelSamples++
					}
				}
				if err := capture.ReleaseBuffer(frames); err != nil {
					return nil, fmt.Errorf("release microphone packet: %w", err)
				}
			}
			if levelSamples > 0 && time.Since(lastLevel) >= 40*time.Millisecond {
				onLevel(visualLevel(levelSumSquares / float64(levelSamples)))
				levelSumSquares = 0
				levelSamples = 0
				lastLevel = time.Now()
			}
		}
	}
}

func visualLevel(meanSquare float64) float32 {
	rms := math.Sqrt(meanSquare)
	return float32(math.Min(1, math.Sqrt(rms*3)))
}
