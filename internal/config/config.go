// Package config loads voice-memo-capture settings from a TOML file,
// merging any present values over built-in defaults.
package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Output struct {
		Dir            string `toml:"dir"`
		FilenameFormat string `toml:"filename_format"`
		Mode           string `toml:"mode"`
	} `toml:"output"`
	Audio struct {
		Handling string `toml:"handling"`
	} `toml:"audio"`
	Source struct {
		RecordingsDir string `toml:"recordings_dir"`
	} `toml:"source"`
	Behavior struct {
		OnMissingTranscript string `toml:"on_missing_transcript"`
	} `toml:"behavior"`
	Logging struct {
		File  string `toml:"file"`
		Level string `toml:"level"`
	} `toml:"logging"`
	Launchd struct {
		CheckIntervalSeconds int `toml:"check_interval_seconds"`
	} `toml:"launchd"`
}

// Defaults returns the documented default configuration.
func Defaults() Config {
	var c Config
	c.Output.Dir = "~/Documents/Voice Memo Transcripts"
	c.Output.FilenameFormat = "{date} {time} - {title}.md"
	c.Output.Mode = "per-memo"
	c.Audio.Handling = "link"
	c.Source.RecordingsDir = "~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"
	c.Behavior.OnMissingTranscript = "placeholder"
	c.Logging.File = "~/Library/Logs/voice-memo-capture.log"
	c.Logging.Level = "info"
	c.Launchd.CheckIntervalSeconds = 300
	return c
}

// Load returns Defaults() with any values present in the TOML file at path
// merged over the top. A missing file is not an error.
func Load(path string) (Config, error) {
	cfg := Defaults()
	if path == "" {
		return cfg, nil
	}
	_, err := toml.DecodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		return Defaults(), nil
	}
	if err != nil {
		return cfg, err
	}
	return cfg, nil
}

// ExpandUser expands a leading "~/" to the user's home directory.
func ExpandUser(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(p, "~"))
		}
	}
	return p
}
