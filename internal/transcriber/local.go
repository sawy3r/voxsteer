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
	outputBase := filepath.Join(dir, "transcript")
	if err := os.WriteFile(inputPath, wav, 0o600); err != nil {
		return "", err
	}

	binary := l.Binary
	if binary == "" {
		binary = "whisper-cli"
	}
	args := []string{"-m", l.Model, "-f", inputPath, "-otxt", "-of", outputBase, "-nt"}
	if l.Language != "" {
		args = append(args, "-l", l.Language)
	}
	command := exec.CommandContext(ctx, binary, args...)
	configureCommand(command)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("whisper.cpp failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	text, err := os.ReadFile(outputBase + ".txt")
	if err != nil {
		return "", fmt.Errorf("read whisper.cpp transcript: %w", err)
	}
	return strings.TrimSpace(string(text)), nil
}
