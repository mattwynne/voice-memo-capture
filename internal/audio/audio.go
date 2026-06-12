// Package audio prepares Voice Memo audio files for local transcription.
package audio

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Converter converts source audio into the WAV format expected by whisper.cpp.
type Converter struct {
	Binary string
}

// New returns a Converter using afconvert unless another binary is supplied.
func New(binary string) Converter {
	if binary == "" {
		binary = "afconvert"
	}
	return Converter{Binary: binary}
}

// CommandArgs returns the afconvert arguments used to make a 16 kHz mono WAV.
func CommandArgs(input, output string) []string {
	return []string{"-f", "WAVE", "-d", "LEI16@16000", "-c", "1", input, output}
}

// ToTempWAV converts input to a temporary WAV file. The caller should remove
// the returned file unless it wants to keep it for debugging.
func (c Converter) ToTempWAV(ctx context.Context, input string) (string, error) {
	if c.Binary == "" {
		c.Binary = "afconvert"
	}
	out, err := os.CreateTemp("", "voice-memo-capture-*.wav")
	if err != nil {
		return "", err
	}
	outPath := out.Name()
	if err := out.Close(); err != nil {
		return "", err
	}

	cmd := exec.CommandContext(ctx, c.Binary, CommandArgs(input, outPath)...)
	combined, err := cmd.CombinedOutput()
	if err != nil {
		_ = os.Remove(outPath)
		return "", fmt.Errorf("%s %v: %w: %s", filepath.Base(c.Binary), CommandArgs(input, outPath), err, string(combined))
	}
	return outPath, nil
}
