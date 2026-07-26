package transcriber

import "context"

// Transcriber converts 16 kHz mono float32 PCM audio into text.
type Transcriber interface {
	Transcribe(ctx context.Context, samples []float32) (string, error)
}
