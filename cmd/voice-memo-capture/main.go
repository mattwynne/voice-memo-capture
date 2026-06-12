// Command voice-memo-capture scans Apple Voice Memos once, extracts each new
// memo's native transcript, and writes it as Markdown. launchd runs it on a
// folder-watch + frequent sweep schedule; the process itself just runs once and exits.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/mattwynne/voice-memo-capture/internal/config"
	"github.com/mattwynne/voice-memo-capture/internal/ledger"
	"github.com/mattwynne/voice-memo-capture/internal/memos"
	"github.com/mattwynne/voice-memo-capture/internal/output"
	"github.com/mattwynne/voice-memo-capture/internal/transcript"
)

func configPath() string {
	if p := os.Getenv("VOICE_MEMO_CAPTURE_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "voice-memo-capture", "config.toml")
}

func ledgerPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "voice-memo-capture", "processed.json")
}

func main() {
	flag.Parse()
	if err := run(); err != nil {
		log.Fatalf("voice-memo-capture: %v", err)
	}
}

func run() error {
	cfg, err := config.Load(configPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logFile, err := openLog(config.ExpandUser(cfg.Logging.File))
	if err == nil {
		log.SetOutput(logFile)
		defer logFile.Close()
	}
	log.SetFlags(log.LstdFlags)

	recordingsDir := config.ExpandUser(cfg.Source.RecordingsDir)
	outDir := config.ExpandUser(cfg.Output.Dir)

	store := memos.New(recordingsDir)
	list, err := store.List()
	if err != nil {
		if memos.IsPermissionError(err) {
			log.Printf("cannot read Voice Memos: Full Disk Access not granted. "+
				"Add this binary to System Settings > Privacy & Security > Full Disk Access: %s",
				selfPath())
			return nil // exit 0: this is a setup state, not a crash
		}
		return fmt.Errorf("listing memos: %w", err)
	}

	led, err := ledger.Load(ledgerPath())
	if err != nil {
		return fmt.Errorf("loading ledger: %w", err)
	}

	written := 0
	for _, m := range list {
		if led.Has(m.ID) {
			continue
		}
		if m.AudioPath == "" {
			log.Printf("memo %d: audio not downloaded yet, will retry", m.ID)
			continue
		}
		text, ok, err := transcript.Extract(m.AudioPath)
		if err != nil {
			log.Printf("memo %d: transcript error: %v", m.ID, err)
			continue
		}
		if !ok {
			if cfg.Behavior.OnMissingTranscript == "placeholder" {
				path, err := output.Write(outDir, cfg.Output.FilenameFormat, m.Memo, output.PlaceholderTranscript())
				if err != nil {
					log.Printf("memo %d: placeholder write failed: %v", m.ID, err)
					continue
				}
				log.Printf("memo %d: no native transcript yet, wrote placeholder %s", m.ID, path)
				continue
			}
			log.Printf("memo %d: no native transcript yet, will retry", m.ID)
			continue
		}
		path, err := output.Write(outDir, cfg.Output.FilenameFormat, m.Memo, text)
		if err != nil {
			log.Printf("memo %d: write failed: %v", m.ID, err)
			continue
		}
		led.Add(m.ID)
		written++
		log.Printf("memo %d: wrote %s", m.ID, path)
	}

	if err := led.Save(ledgerPath()); err != nil {
		return fmt.Errorf("saving ledger: %w", err)
	}
	log.Printf("done: %d new transcript(s) written", written)
	return nil
}

func openLog(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "the voice-memo-capture binary"
	}
	return p
}
