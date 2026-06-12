// Package whisper runs whisper.cpp's CLI and returns plain transcript text.
package whisper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrBinaryMissing   = errors.New("whisper binary missing")
	ErrModelMissing    = errors.New("whisper model missing")
	ErrTimeout         = errors.New("whisper timed out")
	ErrEmptyTranscript = errors.New("whisper produced empty transcript")
)

type Config struct {
	Binary   string
	Model    string
	Language string
	Threads  int
	Timeout  time.Duration
}

type Runner struct {
	Config Config
}

func New(cfg Config) Runner {
	if cfg.Binary == "" {
		cfg.Binary = "whisper-cli"
	}
	if cfg.Language == "" {
		cfg.Language = "en"
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Minute
	}
	return Runner{Config: cfg}
}

// Args returns the whisper-cli arguments for a WAV input and output prefix.
func Args(cfg Config, wavPath, outPrefix string) []string {
	args := []string{"-m", cfg.Model, "-f", wavPath, "-l", cfg.Language, "-otxt", "-of", outPrefix}
	if cfg.Threads > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", cfg.Threads))
	}
	return args
}

func (r Runner) Transcribe(ctx context.Context, wavPath string) (string, error) {
	cfg := New(r.Config).Config
	if _, err := exec.LookPath(cfg.Binary); err != nil {
		return "", fmt.Errorf("%w: %s", ErrBinaryMissing, cfg.Binary)
	}
	if cfg.Model == "" {
		return "", fmt.Errorf("%w: no model configured", ErrModelMissing)
	}
	if _, err := os.Stat(cfg.Model); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: %s", ErrModelMissing, cfg.Model)
		}
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()

	prefix := filepath.Join(os.TempDir(), fmt.Sprintf("voice-memo-capture-whisper-%d", time.Now().UnixNano()))
	defer os.Remove(prefix + ".txt")
	cmd := exec.CommandContext(ctx, cfg.Binary, Args(cfg, wavPath, prefix)...)
	combined, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", ErrTimeout
	}
	if err != nil {
		return "", fmt.Errorf("whisper command failed: %w: %s", err, string(combined))
	}

	textBytes, readErr := os.ReadFile(prefix + ".txt")
	text := strings.TrimSpace(string(textBytes))
	if readErr != nil || text == "" {
		text = cleanStdout(string(combined))
	}
	if strings.TrimSpace(text) == "" {
		return "", ErrEmptyTranscript
	}
	return strings.TrimSpace(text), nil
}

func cleanStdout(s string) string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "whisper_") || strings.HasPrefix(line, "main:") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
