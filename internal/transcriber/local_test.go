//go:build !windows

package transcriber

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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

func TestLocalWhisperBoundsProcessOutput(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-whisper")
	script := "#!/bin/sh\ndd if=/dev/zero bs=1048577 count=1 2>/dev/null\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	engine := &LocalWhisper{Binary: binary, Model: filepath.Join(dir, "model.bin")}
	text, err := engine.Transcribe(context.Background(), []float32{0.1})
	if err == nil || !strings.Contains(err.Error(), "output exceeded") {
		t.Fatalf("error = %v, output bytes = %d", err, len(text))
	}
}

func TestLocalWhisperPassesThreadCount(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "fake-whisper")
	script := "#!/bin/sh\nprintf '%s ' \"$@\"\n"
	if err := os.WriteFile(binary, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	explicit := &LocalWhisper{Binary: binary, Model: filepath.Join(dir, "model.bin"), Threads: 6}
	args, err := explicit.Transcribe(context.Background(), []float32{0.1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(args, "-t 6") {
		t.Fatalf("args = %q, want -t 6", args)
	}
	if !strings.Contains(args, "-mc 0") {
		t.Fatalf("args = %q, want -mc 0 so long dictations cannot loop on earlier text", args)
	}

	automatic := &LocalWhisper{Binary: binary, Model: filepath.Join(dir, "model.bin")}
	args, err = automatic.Transcribe(context.Background(), []float32{0.1})
	if err != nil {
		t.Fatal(err)
	}
	want := automatic.threadCount()
	if want < 1 || want > 8 {
		t.Fatalf("threadCount() = %d, want between 1 and 8", want)
	}
	if !strings.Contains(args, "-t "+strconv.Itoa(want)) {
		t.Fatalf("args = %q, want -t %d", args, want)
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
