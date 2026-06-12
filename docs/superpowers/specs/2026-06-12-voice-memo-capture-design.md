# Voice Memo Capture — Design

**Date:** 2026-06-12
**Status:** Approved (pending spec review)
**Language:** Go

## Goal

A hands-off background service on macOS that takes Apple Voice Memos —
recorded on iPhone, synced to the Mac via iCloud — and writes a plain-text
transcript of each one into a folder, with zero ongoing effort from the user.
Record on the phone, and the transcript appears.

## Background / Why this is non-trivial

Voice Memos are stored at:

```
~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings/
```

containing `.m4a` / `.qta` audio files and a `CloudRecordings.db` SQLite
database (key table `ZCLOUDRECORDING`: `ZPATH`, `ZCUSTOMLABEL`, `ZDATE`,
`ZDURATION`, `ZFOLDER`).

This folder is **TCC-protected**. Confirmed on this machine:

```
$ ls ~/Library/Group\ Containers/group.com.apple.VoiceMemos.shared/Recordings/
ls: .../Recordings/: Operation not permitted
```

Any process reading it must be granted **Full Disk Access** in System
Settings. This is the core obstacle and the reason the user could previously
only drag-and-drop files out of the Voice Memos app.

On macOS 15+ / iOS 18+ (this machine is Darwin 25.3 / macOS 26), Apple
transcribes memos **on-device** and embeds the transcript inside the audio
file as a `tsrp` MP4 atom (JSON with timing + locale). We extract that native
transcript directly — no Whisper, no cloud, no model downloads.

## Decisions (from brainstorming)

| Decision | Choice |
| --- | --- |
| Language | **Go** — single static binary with its own TCC identity |
| Reuse strategy | **Port** the ~90-line read-only kernel from pedramamini's CC0 script (credit retained); we do not vendor Python |
| Output target | Plain Markdown folder, one file per memo |
| Transcription | **Apple native only** — no Whisper dependency |
| Trigger | launchd **WatchPaths** (instant) + configurable sweep interval (default 5 minutes) |
| Configuration | All tunables in a `config.toml` |
| Publishing | **Public** GitHub repo under `mattwynne` |
| License | **MIT** for our code, crediting the original gist |
| Docs / CI | Tidy README with install instructions; GitHub Actions CI |

### Licensing note

The upstream gist is **CC0** (public domain dedication), which places no
conditions on reuse. We are therefore free to license our Go derivative under
**MIT**. We credit the original gist (link in `CREDITS.md` and `README.md`) as
the author requested, though CC0 imposes no legal obligation to do so.

### Why Go (vs Python / Swift)

- A compiled binary gets its **own Full Disk Access identity** instead of
  granting FDA to a general `python3` that every script would inherit.
- The kernel we reuse is small and portable: the transcript extractor
  deliberately **avoids parsing MP4 containers** — it byte-scans for a JSON
  sentinel and brace-balances — so it ports cleanly with no MP4 library.
- Go gives the simplest toolchain (`go build` → one static binary, no Xcode)
  and pure-Go dependencies keep the binary static (no cgo).
- Trade-off accepted vs Swift: Go stays headless (no future menu-bar/
  notification UI), and a bare binary's TCC grant is keyed to its code hash,
  so a rebuild may require re-granting FDA (see Full Disk Access section).

## Architecture

A single Go binary, **run once per invocation**: scan → process → exit.
launchd owns the scheduling (watch + interval); the binary itself is not a
long-running process. This keeps it simple and naturally idempotent.

Internal packages, each with one responsibility:

- `internal/config` — load `config.toml`, merge over documented defaults.
- `internal/memos` — open `CloudRecordings.db` read-only, query recordings,
  resolve each memo's real audio path (`.m4a` vs `.qta`).
- `internal/transcript` — **ported kernel**: byte-scan the audio file for the
  `tsrp` JSON sentinel, brace-balance, parse, and flatten `runs[::2]` to text.
- `internal/ledger` — JSON idempotency ledger of processed memo IDs.
- `internal/output` — render and write the per-memo Markdown file.
- `cmd/voice-memo-capture` — entrypoint wiring the above together.

### Repo layout (`~/git/mattwynne/voice-memo-capture`)

```
voice-memo-capture/
├── README.md                       # overview, install, FDA step, config, uninstall
├── LICENSE                         # MIT (our code)
├── CREDITS.md                      # link back to pedramamini's gist (CC0)
├── .github/
│   └── workflows/
│       └── ci.yml                  # GitHub Actions: build, vet, test
├── go.mod
├── go.sum
├── Makefile                        # build / test / install / uninstall
├── config.example.toml             # documented defaults, copied on install
├── cmd/
│   └── voice-memo-capture/
│       └── main.go                 # entrypoint: load config, run once, exit
├── internal/
│   ├── config/        config.go        # TOML load + defaults
│   ├── memos/         memos.go         # read-only DB query + path resolution
│   ├── transcript/    transcript.go    # tsrp byte-scanner + JSON (ported)
│   │                  transcript_test.go
│   │                  testdata/sample_with_tsrp.m4a   # committed fixture
│   ├── ledger/        ledger.go        # idempotency state
│   └── output/        output.go        # markdown writer
├── install.sh                      # build, install binary, load agent, print FDA steps
├── uninstall.sh                    # unload + remove agent, remove binary
└── net.mattwynne.voicememocapture.plist # LaunchAgent template
```

### Dependencies (both pure Go — static binary, no cgo)

- `modernc.org/sqlite` — read-only access to `CloudRecordings.db`.
- `github.com/BurntSushi/toml` — parse `config.toml` (Go has no stdlib TOML).

Everything else (byte scanning, `encoding/json`, file IO, Markdown text) is
standard library.

## Data flow

```
New memo syncs from iPhone → appears in the Recordings folder
        │
launchd fires (WatchPaths on the folder, OR the configured sweep interval)
        │
voice-memo-capture (runs once):
  1. load config.toml (fall back to defaults for missing keys)
  2. open CloudRecordings.db read-only → (id, title, date, duration, path, folder)
  3. for each memo not in the ledger:
       - resolve audio path; if not downloaded from iCloud yet → skip
       - byte-scan audio for the tsrp transcript JSON, flatten to text
       - if transcript missing → skip, DO NOT ledger (retry next run)
       - else → write <output>/{date} {time} - {title}.md
                add memo id to the ledger
  4. exit
        │
Markdown file appears in the configured output folder
```

## Configuration (`config.toml`)

```toml
[output]
dir = "~/Documents/Voice Memo Transcripts"
filename_format = "{date} {time} - {title}.md"   # tokens: {date} {time} {title} {id}
mode = "per-memo"                # "per-memo" | "daily-journal"

[audio]
handling = "link"                # "link" | "copy"

[source]
recordings_dir = "~/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"

[behavior]
on_missing_transcript = "placeholder"   # "placeholder" | "skip"

[logging]
file = "~/Library/Logs/voice-memo-capture.log"
level = "info"                   # "debug" | "info" | "warn" | "error"
```

- Config path: next to the binary, or via `$VOICE_MEMO_CAPTURE_CONFIG`.
- Missing keys fall back to these documented defaults; a partial or empty
  config still works.
- `install.sh` copies `config.example.toml` → `config.toml` on first run if
  none exists.

### Output file format (per memo)

```markdown
# {title}

- Date: {YYYY-MM-DD HH:MM}
- Duration: {mm:ss}
- Audio: [{filename}](file:///abs/path/to/original.m4a)

{transcript text}
```

## Full Disk Access (the one manual step)

The binary reads a TCC-protected folder, so **the installed binary itself**
must be granted Full Disk Access. `install.sh`:

1. builds and installs the binary to a stable path (e.g.
   `/usr/local/bin/voice-memo-capture`),
2. ad-hoc code-signs it (`codesign -s -`),
3. prints that exact path and walks the user through:

   > System Settings → Privacy & Security → Full Disk Access → add
   > `/usr/local/bin/voice-memo-capture`

**Caveat (honest):** for a bare ad-hoc-signed binary, TCC keys the grant to
the code hash, so **rebuilding the binary may require re-adding it** to the
FDA list. For a tool rebuilt rarely this is a minor one-off. Escaping it would
mean a Developer ID signature or wrapping in a `.app` bundle — out of scope
for v1.

If FDA is absent at runtime, the binary detects the `operation not permitted`
error, logs a clear "grant Full Disk Access to `<path>`" message, and exits 0.

## Idempotency & state

A JSON ledger at `~/.local/state/voice-memo-capture/processed.json` records
which memo IDs have been written. Re-runs skip ledgered memos. A memo with no
transcript yet is **not** ledgered, so the next sweep retries it once Apple
finishes on-device transcription. Safe to run any number of times.

## Error handling

| Condition | Behavior |
| --- | --- |
| Folder unreadable (FDA missing) | Log clear "grant Full Disk Access" message; exit 0 |
| `CloudRecordings.db` locked (app writing) | Open read-only / retry; log at debug, retry next run |
| Audio not yet downloaded from iCloud | Skip memo, retry next run |
| Missing/malformed `tsrp` transcript | Skip that memo, log, continue with others |
| Any per-memo error | Isolated — never aborts the whole batch |

All activity logged to the configured log file.

## Testing

Go tests (`go test ./...`), no dependency on the real library:

- Filename generation from `filename_format` tokens, including title
  sanitization (slashes, length).
- Ledger skip logic: a ledgered id is not rewritten; an un-ledgered id is.
- Transcript extraction against a small **committed fixture** audio file
  containing a `tsrp` atom (`internal/transcript/testdata/`).
- Config loading: partial config merges over defaults; missing file uses
  defaults.

## Publishing, Docs & CI

**Public repo.** Created public on GitHub under `mattwynne/voice-memo-capture`
(via `gh repo create`), pushed from the existing local history.

**`LICENSE`** — standard MIT text, `Copyright (c) 2026 Matt Wynne`.

**`CREDITS.md`** — credits pedramamini's gist (CC0) and explains which logic
was ported (DB query, audio-path resolution, `tsrp` extractor).

**`README.md`** — tidy and skimmable:
- One-paragraph what/why (record on phone → transcript appears in a folder).
- Requirements (macOS 15+ for native transcripts; Go to build).
- Install: `git clone` → `make install` (builds, installs binary, loads the
  LaunchAgent, copies `config.example.toml`).
- **The Full Disk Access step**, called out prominently with the exact path,
  since nothing works without it.
- Configuration reference (the `config.toml` keys).
- How it runs (launchd watch + configurable sweep), where logs go.
- Uninstall: `make uninstall`.
- Credit + link to the original gist.

**CI (`.github/workflows/ci.yml`)** — GitHub Actions on push and PR:
- `runs-on: macos-latest` (the transcript fixture test and stdlib paths are
  macOS-oriented; keeps CI representative of the target platform).
- Steps: `actions/setup-go`, `go vet ./...`, `go build ./...`,
  `go test ./...`.
- A status badge in the README.

## Out of scope (YAGNI)

- Whisper / any non-native transcription.
- LLM summaries or tagging.
- Logseq / Obsidian integration (plain Markdown only; can point other tools
  at the folder later).
- Editing memos, renaming, or writing back to the Voice Memos DB.
- Menu-bar UI, notifications, Developer ID signing, `.app` bundle.

## Credits

Logic ported from
[pedramamini's Voice Memos gist](https://gist.github.com/pedramamini/f4efacfe7080e07e18f54e13d8243dc1)
(CC0): the `CloudRecordings.db` querying, audio-path resolution, and the
`tsrp` byte-scanning transcript extractor are reimplemented in Go from that
script.
