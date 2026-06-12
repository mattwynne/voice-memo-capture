// Package output renders and writes the per-memo Markdown file.
package output

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Memo is the data the output layer needs about a recording. The memos package
// populates and passes this value.
type Memo struct {
	ID        int64
	Date      time.Time
	Duration  time.Duration
	Title     string
	AudioPath string // absolute path to the resolved audio file
}

// Filename renders the format string for a memo. Supported tokens:
// {date} -> YYYY-MM-DD, {time} -> HHMM, {title} -> sanitized title, {id}.
func Filename(format string, m Memo) string {
	local := m.Date.Local()
	r := strings.NewReplacer(
		"{date}", local.Format("2006-01-02"),
		"{time}", local.Format("1504"),
		"{title}", sanitizeTitle(m.Title),
		"{id}", fmt.Sprintf("%d", m.ID),
	)
	return r.Replace(format)
}

func sanitizeTitle(title string) string {
	t := strings.TrimSpace(title)
	if t == "" {
		t = "Untitled"
	}
	// macOS path separator is '/'; ':' is also reserved in Finder. Replace both.
	t = strings.ReplaceAll(t, "/", "-")
	t = strings.ReplaceAll(t, ":", "-")
	// collapse newlines/tabs and trim length
	t = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, t)
	if len(t) > 120 {
		t = strings.TrimSpace(t[:120])
	}
	return t
}

// Render returns the Markdown document for a memo and its transcript.
func Render(m Memo, transcript string) string {
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = "Untitled"
	}
	audioURL := (&url.URL{Scheme: "file", Path: m.AudioPath}).String()
	filename := filepath.Base(m.AudioPath)
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "- Date: %s\n", m.Date.Local().Format("2006-01-02 15:04"))
	fmt.Fprintf(&b, "- Duration: %s\n", formatDuration(m.Duration))
	fmt.Fprintf(&b, "- Audio: [%s](%s)\n\n", filename, audioURL)
	b.WriteString(strings.TrimSpace(transcript))
	b.WriteString("\n")
	return b.String()
}

func formatDuration(d time.Duration) string {
	total := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// Write renders the memo and writes it under dir, returning the file path.
func Write(dir, format string, m Memo, transcript string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, Filename(format, m))
	if err := os.WriteFile(path, []byte(Render(m, transcript)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
