package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsWhenFileMissing(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("Load returned error for missing file: %v", err)
	}
	if cfg.Output.Mode != "per-memo" {
		t.Errorf("Output.Mode = %q, want per-memo", cfg.Output.Mode)
	}
	if cfg.Audio.Handling != "link" {
		t.Errorf("Audio.Handling = %q, want link", cfg.Audio.Handling)
	}
	if cfg.Behavior.OnMissingTranscript != "placeholder" {
		t.Errorf("OnMissingTranscript = %q, want placeholder", cfg.Behavior.OnMissingTranscript)
	}
}

func TestPartialConfigMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[output]\nmode = \"daily-journal\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Output.Mode != "daily-journal" {
		t.Errorf("Output.Mode = %q, want daily-journal (from file)", cfg.Output.Mode)
	}
	if cfg.Audio.Handling != "link" {
		t.Errorf("Audio.Handling = %q, want link (default)", cfg.Audio.Handling)
	}
	if cfg.WhisperEnabled() {
		t.Error("WhisperEnabled true without a configured model")
	}
}

func TestPartialWhisperConfigMergesOverDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("[whisper]\nmodel = \"~/models/base.bin\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.WhisperEnabled() {
		t.Fatal("WhisperEnabled false with a configured model")
	}
	if cfg.Whisper.Binary != "whisper-cli" {
		t.Errorf("Whisper.Binary = %q, want whisper-cli", cfg.Whisper.Binary)
	}
	if cfg.Whisper.Language != "en" {
		t.Errorf("Whisper.Language = %q, want en", cfg.Whisper.Language)
	}
	if cfg.Whisper.When != "apple-missing" {
		t.Errorf("Whisper.When = %q, want apple-missing", cfg.Whisper.When)
	}
	if cfg.Whisper.TimeoutSeconds != 1800 {
		t.Errorf("Whisper.TimeoutSeconds = %d, want 1800", cfg.Whisper.TimeoutSeconds)
	}
}

func TestExpandUserExpandsTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := ExpandUser("~/Documents/x")
	want := filepath.Join(home, "Documents/x")
	if got != want {
		t.Errorf("ExpandUser = %q, want %q", got, want)
	}
}
