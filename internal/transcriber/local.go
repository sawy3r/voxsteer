package transcriber

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	appaudio "github.com/sawy3r/voxsteer/internal/audio"
)

// LocalWhisper runs a locally built whisper.cpp CLI. The CLI may be compiled
// with CUDA, Vulkan, Metal, or CPU support without changing this Go binary.
type LocalWhisper struct {
	Binary   string
	Model    string
	Language string
}

func (l *LocalWhisper) Transcribe(ctx context.Context, samples []float32) (string, error) {
	if l.Model == "" {
		return "", fmt.Errorf("local whisper model path is required")
	}
	wav, err := appaudio.EncodeWAV(samples)
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp("", "voxsteer-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	inputPath := filepath.Join(dir, "audio.wav")
	if err := os.WriteFile(inputPath, wav, 0o600); err != nil {
		return "", err
	}

	binary := l.Binary
	if binary == "" {
		binary = "whisper-cli"
	}
	// --no-prints keeps diagnostics off stdout while preserving the transcript
	// result. Reading stdout directly avoids version-specific output-file
	// behavior and an unnecessary temporary transcript file.
	args := []string{"-m", l.Model, "-f", inputPath, "-np", "-nt"}
	if l.Language != "" {
		args = append(args, "-l", l.Language)
	}
	command := exec.CommandContext(ctx, binary, args...)
	configureCommand(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper.cpp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
