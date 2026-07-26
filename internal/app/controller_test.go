package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recorderFunc func(context.Context, func(float32)) ([]float32, error)

func (f recorderFunc) Record(ctx context.Context, onLevel func(float32)) ([]float32, error) {
	return f(ctx, onLevel)
}

func TestControllerReportsTranscriptionErrorAfterReturningIdle(t *testing.T) {
	var mu sync.Mutex
	var statusAtError Status
	reported := make(chan error, 1)
	controller := New(
		recorderFunc(func(ctx context.Context, _ func(float32)) ([]float32, error) {
			<-ctx.Done()
			return []float32{0.1}, nil
		}),
		transcriberFunc(func(context.Context, []float32) (string, error) {
			return "", errors.New("CUDA failed")
		}),
		injectorFunc(func(string) error {
			t.Fatal("injector should not run after transcription failure")
			return nil
		}),
		func(status Status) {
			mu.Lock()
			defer mu.Unlock()
			statusAtError = status
		},
		nil,
		func(err error) {
			mu.Lock()
			defer mu.Unlock()
			reported <- err
		},
	)

	if err := controller.Toggle(); err != nil {
		t.Fatal(err)
	}
	if err := controller.Toggle(); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-reported:
		if err.Error() != "CUDA failed" {
			t.Fatalf("error = %q", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for transcription error")
	}
	mu.Lock()
	defer mu.Unlock()
	if statusAtError != Idle {
		t.Fatalf("status at error = %q, want idle", statusAtError)
	}
}

type transcriberFunc func(context.Context, []float32) (string, error)

func (f transcriberFunc) Transcribe(ctx context.Context, in []float32) (string, error) {
	return f(ctx, in)
}

type injectorFunc func(string) error

func (f injectorFunc) Type(text string) error { return f(text) }

func TestControllerTogglePipeline(t *testing.T) {
	var mu sync.Mutex
	var typed string
	var levels []float32
	controller := New(
		recorderFunc(func(ctx context.Context, onLevel func(float32)) ([]float32, error) {
			onLevel(0.5)
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
		func(level float32) {
			mu.Lock()
			levels = append(levels, level)
			mu.Unlock()
		},
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
	if len(levels) != 3 || levels[0] != 0.5 || levels[1] != 0 || levels[2] != 0 {
		t.Fatalf("levels = %v, want [0.5 0 0]", levels)
	}
}
