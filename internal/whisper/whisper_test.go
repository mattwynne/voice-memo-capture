package whisper

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestArgsIncludesModelInputTextOutputAndThreads(t *testing.T) {
	args := strings.Join(Args(Config{Model: "model.bin", Language: "en", Threads: 4}, "memo.wav", "out"), " ")
	for _, want := range []string{"-m model.bin", "-f memo.wav", "-l en", "-otxt", "-of out", "-t 4"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
}

func fakeExecutable(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "whisper-cli")
	if runtime.GOOS == "windows" {
		t.Skip("shell script fake not supported")
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func model(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "model.bin")
	if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTranscribeSuccessReadsOutputFile(t *testing.T) {
	fake := fakeExecutable(t, `prefix=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-of" ]; then shift; prefix="$1"; fi
  shift || true
done
printf 'hello from whisper\n' > "$prefix.txt"
`)
	got, err := New(Config{Binary: fake, Model: model(t)}).Transcribe(context.Background(), "memo.wav")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello from whisper" {
		t.Fatalf("got %q", got)
	}
}

func TestTranscribeCommandFailed(t *testing.T) {
	fake := fakeExecutable(t, "echo boom >&2\nexit 7\n")
	_, err := New(Config{Binary: fake, Model: model(t)}).Transcribe(context.Background(), "memo.wav")
	if err == nil || !strings.Contains(err.Error(), "whisper command failed") {
		t.Fatalf("err = %v", err)
	}
}

func TestTranscribeEmptyOutput(t *testing.T) {
	fake := fakeExecutable(t, "exit 0\n")
	_, err := New(Config{Binary: fake, Model: model(t)}).Transcribe(context.Background(), "memo.wav")
	if !errors.Is(err, ErrEmptyTranscript) {
		t.Fatalf("err = %v, want ErrEmptyTranscript", err)
	}
}

func TestTranscribeTimeout(t *testing.T) {
	fake := fakeExecutable(t, "sleep 2\n")
	_, err := New(Config{Binary: fake, Model: model(t), Timeout: 10 * time.Millisecond}).Transcribe(context.Background(), "memo.wav")
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
}

func TestTranscribeMissingModel(t *testing.T) {
	fake := fakeExecutable(t, "exit 0\n")
	_, err := New(Config{Binary: fake, Model: filepath.Join(t.TempDir(), "missing.bin")}).Transcribe(context.Background(), "memo.wav")
	if !errors.Is(err, ErrModelMissing) {
		t.Fatalf("err = %v, want ErrModelMissing", err)
	}
}
