//go:build !windows

package transcriber

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalWhisperReadsTranscriptFromStandardOutput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-whisper")
	script := "#!/bin/sh\nprintf 'verbose whisper diagnostics\\n' >&2\nprintf '  hello from CUDA  \\n'\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	engine := &LocalWhisper{
		Binary:   binary,
		Model:    filepath.Join(dir, "model.bin"),
		Language: "en-AU",
	}
	text, err := engine.Transcribe(context.Background(), []float32{0.1, -0.1})
	if err != nil {
		t.Fatal(err)
	}
	if text != "hello from CUDA" {
		t.Fatalf("text = %q", text)
	}
}

func TestLocalWhisperTreatsZeroExitStderrErrorAsFailure(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-whisper")
	script := "#!/bin/sh\nprintf \"error: unknown language 'en-au'\\n\" >&2\nexit 0\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	engine := &LocalWhisper{
		Binary: binary,
		Model:  filepath.Join(dir, "model.bin"),
	}
	_, err := engine.Transcribe(context.Background(), []float32{0.1})
	if err == nil || !strings.Contains(err.Error(), "unknown language") {
		t.Fatalf("error = %v", err)
	}
}

func TestWhisperLanguageNormalizesRegionalTags(t *testing.T) {
	tests := map[string]string{
		"en-AU": "en",
		"pt_BR": "pt",
		" FR ":  "fr",
		"auto":  "auto",
		"":      "",
	}
	for input, want := range tests {
		if got := whisperLanguage(input); got != want {
			t.Errorf("whisperLanguage(%q) = %q, want %q", input, got, want)
		}
	}
}
