# Voice Memo Capture Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A Go background service for macOS that reads Apple Voice Memos, extracts Apple's on-device transcript, and writes one Markdown file per memo into a folder — run automatically by launchd.

**Architecture:** A single static Go binary, run once per invocation (scan → process → exit); launchd owns scheduling (folder-watch + hourly sweep). Internal packages with one responsibility each: `config`, `transcript` (ported byte-scanner), `memos` (read-only SQLite), `ledger` (idempotency), `output` (Markdown). The repo is published public under MIT, crediting the CC0 gist it was ported from, with GitHub Actions CI.

**Tech Stack:** Go 1.23; `modernc.org/sqlite` (pure-Go, no cgo) for read-only DB access; `github.com/BurntSushi/toml` for config; macOS launchd; GitHub Actions.

---

## Reference facts (from the source gist & live machine)

These are verified, not assumed — use them verbatim:

- Recordings dir: `~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings`
- DB file: `<recordings>/CloudRecordings.db` (SQLite)
- Core Data epoch offset: **978307200** (2001-01-01 UTC, in Unix seconds). `unix = ZDATE + 978307200`.
- Audio extensions to try: `.m4a`, `.qta` (a row's `ZPATH` may name one while the real file is the other).
- Title column: `COALESCE(ZENCRYPTEDTITLE, ZCUSTOMLABEL)`.
- Read-only open DSN: `file:<dbpath>?mode=ro`.
- Transcript sentinel inside the audio bytes: `{"attributedString":`. Apple stores the transcript JSON with a `runs` array where **even indices are the spoken-text tokens**; joining `runs[0,2,4,…]` yields the transcript text.
- The folder is **TCC-protected**: reads fail with a permission error until the binary is granted Full Disk Access.

Module path: `github.com/mattwynne/voice-memo-capture`.

Config resolution order (refinement of the spec): `$VOICE_MEMO_CAPTURE_CONFIG`, else `~/.config/voice-memo-capture/config.toml`, else built-in defaults. (Cleaner than "next to the binary.")

Install paths (no sudo): binary → `~/.local/bin/voice-memo-capture`; LaunchAgent → `~/Library/LaunchAgents/com.matt.voicememocapture.plist`.

Fixture note (refinement of the spec): the transcript test builds a **synthetic** fixture (filler bytes + valid sentinel JSON + trailing bytes) rather than committing a real recording — deterministic and avoids shipping personal audio.

---

## File structure

```
voice-memo-capture/
├── go.mod, go.sum
├── .gitignore
├── Makefile
├── LICENSE                                  # MIT
├── CREDITS.md
├── README.md
├── config.example.toml
├── .github/workflows/ci.yml
├── cmd/voice-memo-capture/main.go           # wiring, run-once, logging, FDA error
├── internal/
│   ├── config/config.go + config_test.go    # TOML load + defaults + ~ expand
│   ├── transcript/transcript.go + _test.go  # byte-scanner + flatten (ported)
│   ├── memos/memos.go + memos_test.go        # read-only DB query + path resolve
│   ├── ledger/ledger.go + ledger_test.go     # JSON processed-id set
│   └── output/output.go + output_test.go     # filename + markdown render/write
├── com.matt.voicememocapture.plist           # template w/ __BINARY__ __WATCHDIR__ __LOG__
├── install.sh
└── uninstall.sh
```

---

## Task 1: Repo bootstrap

**Files:**
- Create: `go.mod`, `.gitignore`, `LICENSE`, `CREDITS.md`, `Makefile`

- [ ] **Step 1: Initialise the module**

Run:
```bash
cd ~/git/mattwynne/voice-memo-capture
go mod init github.com/mattwynne/voice-memo-capture
```
Expected: creates `go.mod` with `module github.com/mattwynne/voice-memo-capture` and a `go 1.23` (or newer) line.

- [ ] **Step 2: Add `.gitignore`**

Create `.gitignore`:
```gitignore
/bin/
*.log
.DS_Store
```

- [ ] **Step 3: Add the MIT `LICENSE`**

Create `LICENSE`:
```
MIT License

Copyright (c) 2026 Matt Wynne

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 4: Add `CREDITS.md`**

Create `CREDITS.md`:
```markdown
# Credits

The core logic for reading Apple Voice Memos was ported from Pedram Amini's
public-domain (CC0) gist:

https://gist.github.com/pedramamini/f4efacfe7080e07e18f54e13d8243dc1

Specifically, the following were reimplemented in Go from that script:

- Querying the `CloudRecordings.db` Core Data SQLite database.
- Resolving a recording's real audio path (`.m4a` vs `.qta`).
- Extracting Apple's on-device transcript by scanning the audio file for the
  embedded `{"attributedString":` JSON and flattening its `runs` array.

The original is dedicated to the public domain under CC0; this project is
licensed under MIT (see `LICENSE`).
```

- [ ] **Step 5: Add `Makefile`**

Create `Makefile` (note: recipe lines must be TAB-indented):
```makefile
BINARY := voice-memo-capture
PKG := ./cmd/voice-memo-capture
BIN_DIR := $(HOME)/.local/bin

.PHONY: build test vet fmt install uninstall

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l -w .

install: build
	./install.sh

uninstall:
	./uninstall.sh
```

- [ ] **Step 6: Verify the module builds (empty) and commit**

Run:
```bash
go build ./... 2>&1 || true   # no packages yet is fine
git add -A
git commit -m "chore: bootstrap Go module, MIT license, credits, Makefile"
```
Expected: clean commit.

---

## Task 2: `config` package

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

- [ ] **Step 1: Add the TOML dependency**

Run:
```bash
go get github.com/BurntSushi/toml@latest
```
Expected: `go.mod`/`go.sum` updated.

- [ ] **Step 2: Write the failing test**

Create `internal/config/config_test.go`:
```go
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
	if cfg.Behavior.OnMissingTranscript != "skip" {
		t.Errorf("OnMissingTranscript = %q, want skip", cfg.Behavior.OnMissingTranscript)
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
}

func TestExpandUserExpandsTilde(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := ExpandUser("~/Documents/x")
	want := filepath.Join(home, "Documents/x")
	if got != want {
		t.Errorf("ExpandUser = %q, want %q", got, want)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/config/ -run Test -v`
Expected: FAIL — `undefined: Load` / `undefined: ExpandUser`.

- [ ] **Step 4: Write the implementation**

Create `internal/config/config.go`:
```go
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
}

// Defaults returns the documented default configuration.
func Defaults() Config {
	var c Config
	c.Output.Dir = "~/Documents/Voice Memo Transcripts"
	c.Output.FilenameFormat = "{date} {time} - {title}.md"
	c.Output.Mode = "per-memo"
	c.Audio.Handling = "link"
	c.Source.RecordingsDir = "~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"
	c.Behavior.OnMissingTranscript = "skip"
	c.Logging.File = "~/Library/Logs/voice-memo-capture.log"
	c.Logging.Level = "info"
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
```

Note: `toml.DecodeFile` only overwrites keys present in the file, so defaults survive for absent keys — exactly the merge behaviour the tests assert.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: PASS (all three tests).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(config): TOML loading with defaults and ~ expansion"
```

---

## Task 3: `transcript` package (the ported kernel)

**Files:**
- Create: `internal/transcript/transcript.go`
- Test: `internal/transcript/transcript_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/transcript/transcript_test.go`:
```go
package transcript

import (
	"os"
	"path/filepath"
	"testing"
)

// buildFixture wraps a valid transcript JSON object in filler bytes, the way
// it appears embedded in a real .m4a/.qta file.
func buildFixture(jsonObj string) []byte {
	out := []byte{0x00, 0x01, 0x02, 'f', 'r', 'e', 'e'} // leading binary filler
	out = append(out, []byte(jsonObj)...)
	out = append(out, []byte{0xFF, 0xFE, 0x00}...) // trailing binary filler
	return out
}

func writeTemp(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "memo.m4a")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestExtractReturnsJoinedTokens(t *testing.T) {
	// runs alternates token, attr-index, token, attr-index, ...
	obj := `{"attributedString":{"runs":["Hello ",0,"world",1]},"locale":"en-US"}`
	path := writeTemp(t, buildFixture(obj))

	text, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Extract reported no transcript, want one")
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestExtractNoTranscript(t *testing.T) {
	path := writeTemp(t, []byte("no transcript here at all"))
	_, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Extract reported a transcript, want none")
	}
}

func TestExtractSkipsInvalidJSONSentinelAndFindsLater(t *testing.T) {
	// First sentinel is followed by broken JSON; a later one is valid.
	bad := `{"attributedString":{"runs":[BROKEN`
	good := `{"attributedString":{"runs":["ok",0]}}`
	data := append([]byte(bad), []byte(good)...)
	path := writeTemp(t, data)

	text, ok, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || text != "ok" {
		t.Errorf("got (%q, %v), want (\"ok\", true)", text, ok)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/transcript/ -v`
Expected: FAIL — `undefined: Extract`.

- [ ] **Step 3: Write the implementation**

Create `internal/transcript/transcript.go`:
```go
// Package transcript extracts Apple's on-device Voice Memos transcript that is
// embedded as JSON inside the recording's audio file.
//
// Ported from Pedram Amini's CC0 gist (see CREDITS.md). Apple writes the
// transcript JSON into the file after on-device transcription completes. Two
// container layouts exist in the wild (older .m4a udta `tsrp` atom; newer .qta
// meta/ilst), so rather than walk both, we scan the raw bytes for the unique
// JSON sentinel and brace-balance forward to the first parseable object.
package transcript

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

var sentinel = []byte(`{"attributedString":`)

type parsed struct {
	AttributedString struct {
		Runs []json.RawMessage `json:"runs"`
	} `json:"attributedString"`
}

// Extract returns the transcript text for the audio file at path. The second
// return value is false (with nil error) when the file has no embedded
// transcript yet — the caller should skip and retry later.
func Extract(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	raw, ok := scan(data)
	if !ok {
		return "", false, nil
	}
	text, err := flatten(raw)
	if err != nil {
		return "", false, err
	}
	return text, true, nil
}

// scan finds the first byte range starting at the sentinel that forms a valid,
// brace-balanced JSON object (string-literal aware).
func scan(data []byte) ([]byte, bool) {
	from := 0
	for {
		rel := bytes.Index(data[from:], sentinel)
		if rel < 0 {
			return nil, false
		}
		start := from + rel
		depth := 0
		inStr := false
		escape := false
		for j := start; j < len(data); j++ {
			b := data[j]
			if inStr {
				switch {
				case escape:
					escape = false
				case b == 0x5C: // backslash
					escape = true
				case b == 0x22: // "
					inStr = false
				}
				continue
			}
			switch b {
			case 0x22: // "
				inStr = true
			case 0x7B: // {
				depth++
			case 0x7D: // }
				depth--
				if depth == 0 {
					candidate := data[start : j+1]
					if json.Valid(candidate) {
						return candidate, true
					}
					// invalid here; resume searching after this sentinel
					from = start + len(sentinel)
					goto nextSentinel
				}
			}
		}
		// reached EOF without closing; try after this sentinel
		from = start + len(sentinel)
	nextSentinel:
	}
}

// flatten joins the even-indexed (spoken-text) entries of the runs array.
func flatten(raw []byte) (string, error) {
	var p parsed
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", err
	}
	var sb strings.Builder
	for i := 0; i < len(p.AttributedString.Runs); i += 2 {
		var tok string
		if err := json.Unmarshal(p.AttributedString.Runs[i], &tok); err != nil {
			continue // non-string token: skip defensively
		}
		sb.WriteString(tok)
	}
	return sb.String(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/transcript/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(transcript): port tsrp byte-scanner + runs flattening from gist"
```

---

## Task 4: `ledger` package

**Files:**
- Create: `internal/ledger/ledger.go`
- Test: `internal/ledger/ledger_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ledger/ledger_test.go`:
```go
package ledger

import (
	"path/filepath"
	"testing"
)

func TestAddHasAndPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "processed.json")

	l, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if l.Has(42) {
		t.Fatal("fresh ledger should not contain id 42")
	}
	l.Add(42)
	if !l.Has(42) {
		t.Fatal("ledger should contain 42 after Add")
	}
	if err := l.Save(path); err != nil {
		t.Fatal(err)
	}

	// Reload from disk: 42 should persist, 99 should not.
	l2, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !l2.Has(42) {
		t.Error("reloaded ledger missing id 42")
	}
	if l2.Has(99) {
		t.Error("reloaded ledger unexpectedly has id 99")
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	l, err := Load(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if l.Has(1) {
		t.Error("missing-file ledger should be empty")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/ledger/ -v`
Expected: FAIL — `undefined: Load`.

- [ ] **Step 3: Write the implementation**

Create `internal/ledger/ledger.go`:
```go
// Package ledger tracks which memo IDs have already been written, so runs are
// idempotent. Stored as a small JSON file.
package ledger

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

type Ledger struct {
	ids map[int64]bool
}

type fileShape struct {
	Processed []int64 `json:"processed"`
}

// Load reads the ledger at path. A missing file yields an empty ledger.
func Load(path string) (*Ledger, error) {
	l := &Ledger{ids: map[int64]bool{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l, nil
	}
	if err != nil {
		return nil, err
	}
	var fileData fileShape
	if err := json.Unmarshal(data, &fileData); err != nil {
		return nil, err
	}
	for _, id := range fileData.Processed {
		l.ids[id] = true
	}
	return l, nil
}

func (l *Ledger) Has(id int64) bool { return l.ids[id] }

func (l *Ledger) Add(id int64) { l.ids[id] = true }

// Save writes the ledger to path, creating parent directories as needed.
func (l *Ledger) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	ids := make([]int64, 0, len(l.ids))
	for id := range l.ids {
		ids = append(ids, id)
	}
	data, err := json.MarshalIndent(fileShape{Processed: ids}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/ledger/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(ledger): JSON-backed idempotency set"
```

---

## Task 5: `output` package

**Files:**
- Create: `internal/output/output.go`
- Test: `internal/output/output_test.go`

This package owns: filename generation from the format tokens, title sanitization, and writing the per-memo Markdown file. It depends on a `Memo` value type defined here (the `memos` package will populate it in Task 6) to avoid an import cycle — see the struct below.

- [ ] **Step 1: Write the failing test**

Create `internal/output/output_test.go`:
```go
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
		Date:      time.Date(2026, 6, 12, 14, 30, 0, 0, time.UTC),
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
		"file://",
		"the transcript body",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("rendered markdown missing %q\n---\n%s", want, md)
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
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/output/ -v`
Expected: FAIL — `undefined: Memo` / `undefined: Filename`.

- [ ] **Step 3: Write the implementation**

Create `internal/output/output.go`:
```go
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
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/output/ -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(output): markdown render, token filenames, title sanitization"
```

---

## Task 6: `memos` package (read-only SQLite)

**Files:**
- Create: `internal/memos/memos.go`
- Test: `internal/memos/memos_test.go`

- [ ] **Step 1: Add the SQLite dependency**

Run:
```bash
go get modernc.org/sqlite@latest
```
Expected: `go.mod`/`go.sum` updated (pure-Go driver, no cgo).

- [ ] **Step 2: Write the failing test**

The test builds a throwaway SQLite DB with the same driver, creates the two
tables we read, inserts one row + a matching audio file, then asserts `List`
returns it with converted fields.

Create `internal/memos/memos_test.go`:
```go
package memos

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func newTestDB(t *testing.T, recordingsDir string) {
	t.Helper()
	dbPath := filepath.Join(recordingsDir, "CloudRecordings.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	stmts := []string{
		`CREATE TABLE ZFOLDER (Z_PK INTEGER PRIMARY KEY, ZENCRYPTEDNAME TEXT)`,
		`CREATE TABLE ZCLOUDRECORDING (
			Z_PK INTEGER PRIMARY KEY,
			ZDATE REAL, ZDURATION REAL,
			ZENCRYPTEDTITLE TEXT, ZCUSTOMLABEL TEXT,
			ZPATH TEXT, ZFOLDER INTEGER)`,
		`INSERT INTO ZFOLDER (Z_PK, ZENCRYPTEDNAME) VALUES (1, 'Ideas')`,
		// ZDATE 769105800 == 2025-05-15T... in Core Data epoch (2001-based).
		`INSERT INTO ZCLOUDRECORDING
			(Z_PK, ZDATE, ZDURATION, ZENCRYPTEDTITLE, ZCUSTOMLABEL, ZPATH, ZFOLDER)
			VALUES (10, 769105800.0, 12.5, 'My Memo', NULL, '20250515 010000.m4a', 1)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("setup stmt failed: %v\n%s", err, s)
		}
	}
}

func TestListReturnsConvertedMemo(t *testing.T) {
	dir := t.TempDir()
	newTestDB(t, dir)
	// create the audio file so resolveAudioPath finds it
	audio := filepath.Join(dir, "20250515 010000.m4a")
	if err := os.WriteFile(audio, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	store := New(dir)
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d memos, want 1", len(got))
	}
	m := got[0]
	if m.ID != 10 {
		t.Errorf("ID = %d, want 10", m.ID)
	}
	if m.Title != "My Memo" {
		t.Errorf("Title = %q, want My Memo", m.Title)
	}
	if m.Folder != "Ideas" {
		t.Errorf("Folder = %q, want Ideas", m.Folder)
	}
	if m.AudioPath != audio {
		t.Errorf("AudioPath = %q, want %q", m.AudioPath, audio)
	}
	// 769105800 + 978307200 = 1747413000 unix
	wantUnix := int64(769105800 + coreDataEpochOffset)
	if m.Date.Unix() != wantUnix {
		t.Errorf("Date.Unix() = %d, want %d", m.Date.Unix(), wantUnix)
	}
	if m.Duration != 12500*time.Millisecond {
		t.Errorf("Duration = %v, want 12.5s", m.Duration)
	}
}

func TestListMissingAudioLeavesPathEmpty(t *testing.T) {
	dir := t.TempDir()
	newTestDB(t, dir)
	// note: no audio file created
	got, err := New(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if got[0].AudioPath != "" {
		t.Errorf("AudioPath = %q, want empty when file absent", got[0].AudioPath)
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/memos/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 4: Write the implementation**

Create `internal/memos/memos.go`:
```go
// Package memos reads Apple Voice Memos recordings from the CloudRecordings.db
// Core Data SQLite database (read-only) and resolves their audio file paths.
package memos

import (
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"github.com/mattwynne/voice-memo-capture/internal/output"
)

const coreDataEpochOffset = 978307200 // 2001-01-01 UTC in Unix seconds

var audioExts = []string{".m4a", ".qta"}

// Memo embeds the output.Memo fields plus what we need for filtering/skip.
type Memo struct {
	output.Memo
	Folder string
}

// Store reads recordings from a given Recordings directory.
type Store struct {
	recordingsDir string
}

func New(recordingsDir string) *Store {
	return &Store{recordingsDir: recordingsDir}
}

func (s *Store) dbPath() string {
	return filepath.Join(s.recordingsDir, "CloudRecordings.db")
}

const listSQL = `
	SELECT r.Z_PK AS id,
	       r.ZDATE AS zdate,
	       r.ZDURATION AS duration,
	       COALESCE(r.ZENCRYPTEDTITLE, r.ZCUSTOMLABEL) AS title,
	       r.ZPATH AS path,
	       f.ZENCRYPTEDNAME AS folder
	FROM ZCLOUDRECORDING r
	LEFT JOIN ZFOLDER f ON r.ZFOLDER = f.Z_PK
	ORDER BY r.ZDATE DESC`

// List returns all recordings, newest first, with audio paths resolved.
func (s *Store) List() ([]Memo, error) {
	db, err := sql.Open("sqlite", "file:"+s.dbPath()+"?mode=ro")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(listSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memos []Memo
	for rows.Next() {
		var (
			id       int64
			zdate    sql.NullFloat64
			duration sql.NullFloat64
			title    sql.NullString
			path     sql.NullString
			folder   sql.NullString
		)
		if err := rows.Scan(&id, &zdate, &duration, &title, &path, &folder); err != nil {
			return nil, err
		}
		m := Memo{Folder: folder.String}
		m.ID = id
		m.Title = title.String
		m.Date = time.Unix(int64(zdate.Float64)+coreDataEpochOffset, 0).UTC()
		m.Duration = time.Duration(duration.Float64 * float64(time.Second))
		m.AudioPath = s.resolveAudioPath(path.String)
		memos = append(memos, m)
	}
	return memos, rows.Err()
}

// resolveAudioPath returns the absolute path to the recording's audio file, or
// "" if it isn't present on disk yet (e.g. not downloaded from iCloud). ZPATH
// may name one extension while the real file uses the other.
func (s *Store) resolveAudioPath(zpath string) string {
	if zpath == "" {
		return ""
	}
	direct := filepath.Join(s.recordingsDir, zpath)
	if fileExists(direct) {
		return direct
	}
	stem := zpath[:len(zpath)-len(filepath.Ext(zpath))]
	for _, ext := range audioExts {
		cand := filepath.Join(s.recordingsDir, stem+ext)
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// IsPermissionError reports whether err is the macOS TCC "operation not
// permitted" / permission-denied condition (Full Disk Access not granted).
func IsPermissionError(err error) bool {
	return errors.Is(err, fs.ErrPermission)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/memos/ -v`
Expected: PASS (both tests).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(memos): read-only DB query and audio path resolution"
```

---

## Task 7: `main.go` wiring

**Files:**
- Create: `cmd/voice-memo-capture/main.go`
- Create: `config.example.toml`

- [ ] **Step 1: Add `config.example.toml`**

Create `config.example.toml`:
```toml
# voice-memo-capture configuration.
# All keys are optional; anything omitted uses the built-in default shown here.

[output]
dir = "~/Documents/Voice Memo Transcripts"
filename_format = "{date} {time} - {title}.md"   # tokens: {date} {time} {title} {id}
mode = "per-memo"                # "per-memo" | "daily-journal"

[audio]
handling = "link"                # "link" | "copy"

[source]
recordings_dir = "~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"

[behavior]
on_missing_transcript = "skip"   # "skip" | "placeholder"

[logging]
file = "~/Library/Logs/voice-memo-capture.log"
level = "info"                   # "debug" | "info" | "warn" | "error"
```

- [ ] **Step 2: Write `main.go`**

Create `cmd/voice-memo-capture/main.go`:
```go
// Command voice-memo-capture scans Apple Voice Memos once, extracts each new
// memo's native transcript, and writes it as Markdown. launchd runs it on a
// folder-watch + hourly schedule; the process itself just runs once and exits.
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
```

- [ ] **Step 3: Build the whole program**

Run: `go build ./...`
Expected: builds with no errors. (`output.Memo` is reused by `memos.Memo` via embedding, so `m.Memo` in main is valid.)

- [ ] **Step 4: Smoke-test the binary**

Run:
```bash
go run ./cmd/voice-memo-capture
```
Expected (on this Mac, before FDA is granted): exits 0, and the log file
`~/Library/Logs/voice-memo-capture.log` contains a "Full Disk Access not
granted" line naming the binary. (After FDA is granted in Task 8 it will write
transcripts instead.)

- [ ] **Step 5: Run all tests**

Run: `go test ./...`
Expected: PASS across all packages.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(cmd): wire run-once pipeline with FDA-aware error handling"
```

---

## Task 8: launchd agent + install/uninstall scripts

**Files:**
- Create: `com.matt.voicememocapture.plist` (template)
- Create: `install.sh`
- Create: `uninstall.sh`

- [ ] **Step 1: Add the LaunchAgent template**

Create `com.matt.voicememocapture.plist` (placeholders substituted at install):
```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>com.matt.voicememocapture</string>
  <key>ProgramArguments</key>
  <array>
    <string>__BINARY__</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>StartInterval</key>
  <integer>3600</integer>
  <key>WatchPaths</key>
  <array>
    <string>__WATCHDIR__</string>
  </array>
  <key>StandardOutPath</key>
  <string>__LOG__</string>
  <key>StandardErrorPath</key>
  <string>__LOG__</string>
</dict>
</plist>
```

- [ ] **Step 2: Add `install.sh`**

Create `install.sh` (and `chmod +x install.sh`):
```bash
#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$HOME/.local/bin"
BINARY="$BIN_DIR/voice-memo-capture"
CONFIG_DIR="$HOME/.config/voice-memo-capture"
CONFIG="$CONFIG_DIR/config.toml"
LOG="$HOME/Library/Logs/voice-memo-capture.log"
WATCHDIR="$HOME/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"
AGENT="$HOME/Library/LaunchAgents/com.matt.voicememocapture.plist"

echo "==> Building"
mkdir -p "$BIN_DIR"
( cd "$REPO_DIR" && go build -o "$BINARY" ./cmd/voice-memo-capture )

echo "==> Ad-hoc code-signing (stable-ish TCC identity)"
codesign --force -s - "$BINARY"

echo "==> Installing config (if absent)"
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG" ]; then
  cp "$REPO_DIR/config.example.toml" "$CONFIG"
  echo "    wrote $CONFIG"
else
  echo "    keeping existing $CONFIG"
fi

echo "==> Installing LaunchAgent"
mkdir -p "$HOME/Library/LaunchAgents"
sed -e "s|__BINARY__|$BINARY|g" \
    -e "s|__WATCHDIR__|$WATCHDIR|g" \
    -e "s|__LOG__|$LOG|g" \
    "$REPO_DIR/com.matt.voicememocapture.plist" > "$AGENT"

# reload cleanly if already loaded
launchctl unload "$AGENT" 2>/dev/null || true
launchctl load -w "$AGENT"

cat <<EOF

==> Installed.

ONE MANUAL STEP — grant Full Disk Access:
  1. Open System Settings > Privacy & Security > Full Disk Access
  2. Click +, press Cmd+Shift+G, and paste:
       $BINARY
  3. Enable the toggle next to it.

Until you do this, the tool will log "Full Disk Access not granted" and write
nothing. Logs: $LOG
EOF
```

- [ ] **Step 3: Add `uninstall.sh`**

Create `uninstall.sh` (and `chmod +x uninstall.sh`):
```bash
#!/usr/bin/env bash
set -euo pipefail

BINARY="$HOME/.local/bin/voice-memo-capture"
AGENT="$HOME/Library/LaunchAgents/com.matt.voicememocapture.plist"

if [ -f "$AGENT" ]; then
  launchctl unload "$AGENT" 2>/dev/null || true
  rm -f "$AGENT"
  echo "Removed LaunchAgent."
fi
rm -f "$BINARY" && echo "Removed binary." || true
echo "Note: config (~/.config/voice-memo-capture) and transcripts are left in place."
echo "Remember to remove the binary from Full Disk Access in System Settings."
```

- [ ] **Step 4: Make scripts executable and commit**

Run:
```bash
chmod +x install.sh uninstall.sh
bash -n install.sh && bash -n uninstall.sh   # syntax check
git add -A
git commit -m "feat: launchd agent template, install and uninstall scripts"
```
Expected: `bash -n` prints nothing (syntax OK).

- [ ] **Step 5: Real install + grant FDA (manual, on this Mac)**

Run: `make install`
Then follow the printed Full Disk Access steps for `~/.local/bin/voice-memo-capture`.

- [ ] **Step 6: Verify end-to-end**

Run:
```bash
~/.local/bin/voice-memo-capture
ls -la ~/Documents/Voice\ Memo\ Transcripts/
tail -n 20 ~/Library/Logs/voice-memo-capture.log
```
Expected: after FDA is granted, Markdown files appear for memos that have a
native transcript, and the log shows "wrote …" lines. (No commit — this step
is verification only.)

---

## Task 9: README + CI

**Files:**
- Create: `README.md`
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Add the CI workflow**

Create `.github/workflows/ci.yml`:
```yaml
name: CI

on:
  push:
    branches: [ main ]
  pull_request:

jobs:
  build-test:
    runs-on: macos-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.23'
      - run: go vet ./...
      - run: go build ./...
      - run: go test ./...
```

- [ ] **Step 2: Add `README.md`**

Create `README.md`:
````markdown
# voice-memo-capture

![CI](https://github.com/mattwynne/voice-memo-capture/actions/workflows/ci.yml/badge.svg)

Record a voice memo on your iPhone; once it syncs to your Mac, a Markdown
transcript appears in a folder — automatically, with no app to open.

It reads Apple's **on-device** transcript that's embedded in each recording
(macOS 15+ / iOS 18+), so there's no Whisper, no cloud, and no model download.
A launchd agent runs the tool whenever the Voice Memos folder changes, plus
hourly as a safety net.

## Requirements

- macOS 15+ (for Apple's native transcripts) — developed on macOS 26.
- [Go](https://go.dev/dl/) 1.23+ to build.

## Install

```bash
git clone https://github.com/mattwynne/voice-memo-capture.git
cd voice-memo-capture
make install
```

`make install` builds the binary to `~/.local/bin/voice-memo-capture`, writes
a default config, and loads the launchd agent.

### Required: grant Full Disk Access

The Voice Memos folder is protected by macOS. You must grant the binary Full
Disk Access once:

1. System Settings → Privacy & Security → **Full Disk Access**
2. Click **+**, press **Cmd+Shift+G**, and paste:
   `~/.local/bin/voice-memo-capture`
3. Enable the toggle.

Until then the tool logs `Full Disk Access not granted` and writes nothing.

## Configuration

Config lives at `~/.config/voice-memo-capture/config.toml` (override with
`$VOICE_MEMO_CAPTURE_CONFIG`). Every key is optional and falls back to the
default shown:

| Key | Default | Meaning |
| --- | --- | --- |
| `output.dir` | `~/Documents/Voice Memo Transcripts` | Where transcripts are written |
| `output.filename_format` | `{date} {time} - {title}.md` | Tokens: `{date} {time} {title} {id}` |
| `output.mode` | `per-memo` | `per-memo` or `daily-journal` |
| `audio.handling` | `link` | `link` to original audio, or `copy` |
| `source.recordings_dir` | Voice Memos group container | Override only if Apple moves it |
| `behavior.on_missing_transcript` | `skip` | `skip` (retry later) or `placeholder` |
| `logging.file` | `~/Library/Logs/voice-memo-capture.log` | Log path |
| `logging.level` | `info` | `debug`/`info`/`warn`/`error` |

## How it runs

A launchd agent (`com.matt.voicememocapture`) triggers the tool when the
recordings folder changes and once an hour. Each run is idempotent: a JSON
ledger at `~/.local/state/voice-memo-capture/processed.json` records what's
already written, and memos whose transcript isn't ready yet are retried on the
next run. Logs go to `~/Library/Logs/voice-memo-capture.log`.

## Uninstall

```bash
make uninstall
```

This unloads the agent and removes the binary. Your config and transcripts are
left in place. Remember to remove the binary from Full Disk Access in System
Settings.

## Credits & license

The core Voice Memos reading logic was ported from Pedram Amini's public-domain
(CC0) gist — see [`CREDITS.md`](CREDITS.md). This project is licensed under the
[MIT License](LICENSE).
````

- [ ] **Step 3: Validate and commit**

Run:
```bash
go vet ./... && go build ./... && go test ./...
git add -A
git commit -m "docs: tidy README with install + config; add GitHub Actions CI"
```
Expected: all green; clean commit.

---

## Task 10: Publish the public repo

**Files:** none (publishing action)

- [ ] **Step 1: Create the public GitHub repo and push**

Run:
```bash
cd ~/git/mattwynne/voice-memo-capture
gh repo create mattwynne/voice-memo-capture \
  --public --source=. --remote=origin \
  --description "Auto-transcribe Apple Voice Memos to Markdown on macOS" \
  --push
```
Expected: repo created at `https://github.com/mattwynne/voice-memo-capture`, all commits pushed, `origin` set.

- [ ] **Step 2: Confirm CI runs green**

Run:
```bash
gh run list --limit 1
gh run watch "$(gh run list --limit 1 --json databaseId -q '.[0].databaseId')" || true
```
Expected: the CI workflow appears and finishes **success**. If it fails, read
the log (`gh run view --log-failed`), fix, commit, and push.

- [ ] **Step 3: Confirm the badge resolves**

Open `https://github.com/mattwynne/voice-memo-capture` and check the README CI
badge shows passing. (Manual visual check; no commit.)

---

## Self-review (completed by plan author)

**Spec coverage:**
- Go single static binary, run-once → Task 7 + launchd in Task 8. ✓
- Port (not vendor) the kernel → Tasks 3 & 6, credited in Task 1. ✓
- Apple-native-only transcript extraction → Task 3. ✓
- One Markdown file per memo, configurable format → Task 5. ✓
- Default output `~/Documents/Voice Memo Transcripts` → Tasks 2, 7. ✓
- `config.toml` with defaults + partial merge → Task 2. ✓
- Idempotency ledger → Task 4, used in Task 7. ✓
- launchd WatchPaths + hourly sweep → Task 8 plist. ✓
- Full Disk Access: granted to binary, graceful runtime fallback, installer guidance → Tasks 6, 7, 8. ✓
- Read-only DB, `.m4a`/`.qta` resolution, iCloud-not-downloaded skip → Task 6, handled in Task 7. ✓
- Error handling (locked DB / missing transcript / per-memo isolation) → Task 7 loop. ✓
- Testing across packages → Tasks 2–6. ✓
- MIT license + CC0 credit → Task 1. ✓
- Public repo → Task 10. ✓
- Tidy README + install instructions → Task 9. ✓
- CI → Task 9 (workflow) + Task 10 (verify green). ✓

**Refinements recorded in-plan (deviations from spec, intentional):** config
path is `~/.config/...` not "next to the binary"; transcript fixture is
synthetic rather than a committed real recording; install dir is `~/.local/bin`
(no sudo). All noted at the top under "Reference facts".

**Type consistency:** `output.Memo` is the shared value type; `memos.Memo`
embeds it and adds `Folder`; `main` passes `m.Memo` to `output.Write`. Function
names (`Load`, `Extract`, `List`, `New`, `Write`, `Filename`, `Render`,
`Has`/`Add`/`Save`, `IsPermissionError`, `ExpandUser`) are used consistently
across tasks. ✓

**Placeholder scan:** no TBD/TODO; every code step contains complete code. ✓
