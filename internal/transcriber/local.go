package transcriber

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	appaudio "github.com/sawy3r/voxsteer/internal/audio"
)

// LocalWhisper runs a locally built whisper.cpp CLI. The CLI may be compiled
// with CUDA, Vulkan, Metal, or CPU support without changing this Go binary.
type LocalWhisper struct {
	Binary   string
	Model    string
	Language string
	// Threads sets whisper.cpp's worker thread count; zero picks an
	// automatic count based on the machine.
	Threads int
}

const maxWhisperOutput = 1 << 20

type cappedBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *cappedBuffer) Write(value []byte) (int, error) {
	written := len(value)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if remaining > len(value) {
			remaining = len(value)
		}
		_, _ = b.buffer.Write(value[:remaining])
	}
	if remaining < len(value) {
		b.overflow = true
	}
	return written, nil
}

func (b *cappedBuffer) String() string {
	return b.buffer.String()
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
	args := []string{"-m", l.Model, "-f", inputPath, "-np", "-nt", "-t", strconv.Itoa(l.threadCount())}
	if language := whisperLanguage(l.Language); language != "" {
		args = append(args, "-l", language)
	}
	command := exec.CommandContext(ctx, binary, args...)
	configureCommand(command)
	command.WaitDelay = 5 * time.Second
	stdout := cappedBuffer{limit: maxWhisperOutput}
	stderr := cappedBuffer{limit: maxWhisperOutput}
	command.Stdout = &stdout
	command.Stderr = &stderr
	err = command.Run()
	if stdout.overflow || stderr.overflow {
		return "", fmt.Errorf("whisper.cpp output exceeded %d bytes", maxWhisperOutput)
	}
	if err != nil {
		details := strings.TrimSpace(stderr.String())
		if details != "" {
			return "", fmt.Errorf("whisper.cpp failed: %w: %s", err, details)
		}
		return "", fmt.Errorf("whisper.cpp failed: %w", err)
	}
	text := strings.TrimSpace(stdout.String())
	if strings.Contains(strings.ToLower(stderr.String()), "error:") {
		return "", fmt.Errorf("whisper.cpp failed: %s", strings.TrimSpace(stderr.String()))
	}
	return text, nil
}

// threadCount defaults to more workers than whisper.cpp's own four, which
// leaves most of a modern CPU idle, while capping at eight so efficiency
// cores and hyperthreads are not oversubscribed.
func (l *LocalWhisper) threadCount() int {
	if l.Threads > 0 {
		return l.Threads
	}
	threads := runtime.NumCPU()
	if threads > 8 {
		threads = 8
	}
	if threads < 1 {
		threads = 1
	}
	return threads
}

func whisperLanguage(language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" || language == "auto" {
		return language
	}
	if separator := strings.IndexAny(language, "-_"); separator >= 0 {
		return language[:separator]
	}
	return language
}
