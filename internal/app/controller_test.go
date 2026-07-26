package app

import (
	"context"
	"sync"
	"testing"
	"time"
)

type recorderFunc func(context.Context) ([]float32, error)

func (f recorderFunc) Record(ctx context.Context) ([]float32, error) { return f(ctx) }

type transcriberFunc func(context.Context, []float32) (string, error)

func (f transcriberFunc) Transcribe(ctx context.Context, in []float32) (string, error) {
	return f(ctx, in)
}

type injectorFunc func(string) error

func (f injectorFunc) Type(text string) error { return f(text) }

func TestControllerTogglePipeline(t *testing.T) {
	var mu sync.Mutex
	var typed string
	controller := New(
		recorderFunc(func(ctx context.Context) ([]float32, error) {
			<-ctx.Done()
			return []float32{0.1}, nil
		}),
		transcriberFunc(func(context.Context, []float32) (string, error) {
			return "hello", nil
		}),
		injectorFunc(func(text string) error {
			mu.Lock()
			typed = text
			mu.Unlock()
			return nil
		}),
		nil,
		nil,
	)

	if err := controller.Toggle(); err != nil {
		t.Fatal(err)
	}
	if err := controller.Toggle(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for controller.Status() != Idle && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if typed != "hello " {
		t.Fatalf("typed = %q", typed)
	}
}
