package transcriber

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// fallbackWhisperDirs are searched after PATH. A macOS application started
// from Finder or at login gets only the system PATH, which leaves out where
// Homebrew installs whisper.cpp.
var fallbackWhisperDirs = func() []string {
	if runtime.GOOS == "darwin" {
		return []string{"/opt/homebrew/bin", "/usr/local/bin"}
	}
	return nil
}()

// FindWhisperCLI returns the whisper.cpp CLI the local engine runs: binary
// itself when it is a path, otherwise a command of that name (whisper-cli when
// blank) found on PATH or in the usual install locations.
func FindWhisperCLI(binary string) (string, error) {
	if binary == "" {
		binary = "whisper-cli"
	}
	if strings.ContainsRune(binary, os.PathSeparator) {
		if _, err := os.Stat(binary); err != nil {
			return "", err
		}
		return binary, nil
	}
	path, err := exec.LookPath(binary)
	if err == nil {
		return path, nil
	}
	for _, dir := range fallbackWhisperDirs {
		if candidate, lookErr := exec.LookPath(filepath.Join(dir, binary)); lookErr == nil {
			return candidate, nil
		}
	}
	return "", err
}
