package audio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandArgsBuilds16kMonoWAVConversion(t *testing.T) {
	args := strings.Join(CommandArgs("in.m4a", "out.wav"), " ")
	for _, want := range []string{"-f WAVE", "-d LEI16@16000", "-c 1", "in.m4a", "out.wav"} {
		if !strings.Contains(args, want) {
			t.Fatalf("args %q missing %q", args, want)
		}
	}
}

func TestToTempWAVInvokesAfconvert(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.txt")
	fake := filepath.Join(dir, "afconvert")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + log + `"
last=""
for arg in "$@"; do last="$arg"; done
printf wav > "$last"
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	wav, err := New(fake).ToTempWAV(context.Background(), "memo.m4a")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(wav)
	if filepath.Ext(wav) != ".wav" {
		t.Fatalf("temp path = %q, want .wav", wav)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	for _, want := range []string{"-f\nWAVE", "-d\nLEI16@16000", "-c\n1", "memo.m4a", wav} {
		if !strings.Contains(got, want) {
			t.Errorf("fake afconvert args missing %q in\n%s", want, got)
		}
	}
}
