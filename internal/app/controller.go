package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/sawy3r/voxsteer/internal/transcriber"
)

type Recorder interface {
	Record(ctx context.Context, onLevel func(float32)) ([]float32, error)
}

type Injector interface {
	Type(text string) error
}

type Status string

const (
	Idle         Status = "idle"
	Recording    Status = "recording"
	Transcribing Status = "transcribing"
)

type Controller struct {
	recorder    Recorder
	transcriber transcriber.Transcriber
	injector    Injector
	onStatus    func(Status)
	onLevel     func(float32)
	onError     func(error)
	timeout     time.Duration

	mu     sync.Mutex
	status Status
	cancel context.CancelFunc
}

func New(
	recorder Recorder,
	engine transcriber.Transcriber,
	injector Injector,
	onStatus func(Status),
	onLevel func(float32),
	onError func(error),
) *Controller {
	if onStatus == nil {
		onStatus = func(Status) {}
	}
	if onError == nil {
		onError = func(error) {}
	}
	if onLevel == nil {
		onLevel = func(float32) {}
	}
	return &Controller{
		recorder: recorder, transcriber: engine, injector: injector,
		onStatus: onStatus, onLevel: onLevel, onError: onError,
		timeout: 2 * time.Minute, status: Idle,
	}
}

func (c *Controller) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Toggle starts recording when idle and stops it when recording.
func (c *Controller) Toggle() error {
	c.mu.Lock()
	switch c.status {
	case Idle:
		ctx, cancel := context.WithCancel(context.Background())
		c.cancel = cancel
		c.status = Recording
		c.mu.Unlock()
		c.onStatus(Recording)
		go c.capture(ctx)
		return nil
	case Recording:
		c.cancel()
		c.mu.Unlock()
		return nil
	default:
		status := c.status
		c.mu.Unlock()
		return fmt.Errorf("dictation is already %s", status)
	}
}

func (c *Controller) capture(ctx context.Context) {
	samples, err := c.recorder.Record(ctx, c.onLevel)
	if err != nil && !errors.Is(err, context.Canceled) {
		c.finish(Idle)
		c.onError(fmt.Errorf("record audio: %w", err))
		return
	}
	if len(samples) == 0 {
		c.finish(Idle)
		return
	}
	log.Printf(
		"captured audio: samples=%d duration=%.2fs",
		len(samples),
		float64(len(samples))/16000,
	)

	c.finish(Transcribing)
	transcribeCtx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()
	text, err := c.transcriber.Transcribe(transcribeCtx, samples)
	if err == nil {
		log.Printf("transcription complete: characters=%d", len([]rune(strings.TrimSpace(text))))
	}
	if err == nil && strings.TrimSpace(text) != "" {
		err = c.injector.Type(strings.TrimSpace(text) + " ")
	}
	if err != nil {
		c.finish(Idle)
		c.onError(err)
		return
	}
	c.finish(Idle)
}

func (c *Controller) finish(status Status) {
	c.mu.Lock()
	c.cancel = nil
	c.status = status
	c.mu.Unlock()
	if status != Recording {
		c.onLevel(0)
	}
	c.onStatus(status)
}
