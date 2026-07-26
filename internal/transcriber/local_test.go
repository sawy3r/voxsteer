//go:build !windows

package transcriber

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalWhisperReadsTranscriptFromStandardOutput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-whisper")
	script := "#!/bin/sh\nprintf '  hello from CUDA  \\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	engine := &LocalWhisper{
		Binary: binary,
		Model:  filepath.Join(dir, "model.bin"),
	}
	text, err := engine.Transcribe(context.Background(), []float32{0.1, -0.1})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello from CUDA" {
		t.Fatalf("text = %q", text)
	}
}
