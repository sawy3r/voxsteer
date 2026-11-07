//go:build !windows

package transcriber

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindWhisperCLISearchesInstallLocationsMissingFromPath(t *testing.T) {
	installed := t.TempDir()
	binary := filepath.Join(installed, "whisper-cli")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	previous := fallbackWhisperDirs
	fallbackWhisperDirs = []string{filepath.Join(installed, "missing"), installed}
	t.Cleanup(func() { fallbackWhisperDirs = previous })

	got, err := FindWhisperCLI("")
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("FindWhisperCLI() = %q, want %q", got, binary)
	}
}

func TestFindWhisperCLIPrefersPath(t *testing.T) {
	onPath := t.TempDir()
	installed := t.TempDir()
	for _, dir := range []string{onPath, installed} {
		if err := os.WriteFile(filepath.Join(dir, "whisper-cli"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", onPath)
	previous := fallbackWhisperDirs
	fallbackWhisperDirs = []string{installed}
	t.Cleanup(func() { fallbackWhisperDirs = previous })

	got, err := FindWhisperCLI("whisper-cli")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(onPath, "whisper-cli"); got != want {
		t.Fatalf("FindWhisperCLI() = %q, want %q", got, want)
	}
}

func TestFindWhisperCLIReportsAMissingCommand(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	previous := fallbackWhisperDirs
	fallbackWhisperDirs = []string{t.TempDir()}
	t.Cleanup(func() { fallbackWhisperDirs = previous })

	if _, err := FindWhisperCLI(""); err == nil {
		t.Fatal("FindWhisperCLI() error = nil, want not found")
	}
}

func TestFindWhisperCLIKeepsAnExplicitPath(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "custom-whisper")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	got, err := FindWhisperCLI(binary)
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("FindWhisperCLI() = %q, want %q", got, binary)
	}
	if _, err := FindWhisperCLI(binary + "-missing"); err == nil {
		t.Fatal("FindWhisperCLI() error = nil for a missing explicit path")
	}
}
