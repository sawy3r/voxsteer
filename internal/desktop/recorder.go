//go:build desktop && !windows

package desktop

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"github.com/gen2brain/malgo"
)

type Recorder struct{}

func (Recorder) Record(ctx context.Context) ([]float32, error) {
	malgoCtx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("initialize audio: %w", err)
	}
	defer func() {
		_ = malgoCtx.Uninit()
		malgoCtx.Free()
	}()

	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = 1
	config.SampleRate = 16000
	config.Alsa.NoMMap = 1

	var mu sync.Mutex
	samples := make([]float32, 0, 16000*30)
	callbacks := malgo.DeviceCallbacks{
		Data: func(_, input []byte, frameCount uint32) {
			mu.Lock()
			defer mu.Unlock()
			for i := 0; i+1 < len(input); i += 2 {
				raw := int16(binary.LittleEndian.Uint16(input[i : i+2]))
				samples = append(samples, float32(raw)/math.MaxInt16)
			}
		},
	}

	device, err := malgo.InitDevice(malgoCtx.Context, config, callbacks)
	if err != nil {
		return nil, fmt.Errorf("open default microphone: %w", err)
	}
	defer device.Uninit()
	if err := device.Start(); err != nil {
		return nil, fmt.Errorf("start microphone: %w", err)
	}
	<-ctx.Done()
	_ = device.Stop()

	mu.Lock()
	defer mu.Unlock()
	return append([]float32(nil), samples...), nil
}
