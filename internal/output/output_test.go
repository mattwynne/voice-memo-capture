package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sampleMemo() Memo {
	return Memo{
		ID:        7,
		Date:      time.Date(2026, 6, 12, 14, 30, 0, 0, time.Local),
		Duration:  95 * time.Second,
		Title:     "Idea about / the thing",
		AudioPath: "/tmp/Recordings/20260612 143000.m4a",
	}
}

func TestFilenameTokensAndSanitization(t *testing.T) {
	name := Filename("{date} {time} - {title}.md", sampleMemo())
	want := "2026-06-12 1430 - Idea about - the thing.md"
	if name != want {
		t.Errorf("Filename = %q, want %q", name, want)
	}
	if strings.Contains(name, "/") {
		t.Errorf("filename must not contain '/': %q", name)
	}
}

func TestFilenameUntitledFallback(t *testing.T) {
	m := sampleMemo()
	m.Title = ""
	name := Filename("{date} {time} - {title}.md", m)
	if !strings.Contains(name, "Untitled") {
		t.Errorf("empty title should fall back to Untitled, got %q", name)
	}
}

func TestRenderContainsHeaderAndTranscript(t *testing.T) {
	md := Render(sampleMemo(), "the transcript body")
	for _, want := range []string{
		"# Idea about / the thing",
		"- Date: 2026-06-12 14:30",
		"- Duration: 01:35",
		"- Transcript: Apple Voice Memos",
		"file://",
		"the transcript body",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestPlaceholderTranscriptExplainsPendingState(t *testing.T) {
	body := PlaceholderTranscript()
	for _, want := range []string{"Transcript pending", "local Whisper", "overwritten automatically"} {
		if !strings.Contains(body, want) {
			t.Errorf("placeholder missing %q: %s", want, body)
		}
	}
}

func TestWriteCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, "{date} {time} - {title}.md", sampleMemo(), "body")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir {
		t.Errorf("file written to %q, want dir %q", path, dir)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "body") {
		t.Error("written file missing transcript body")
	}
}
