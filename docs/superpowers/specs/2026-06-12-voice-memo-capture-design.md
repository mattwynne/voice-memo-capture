# Voice Memo Capture — Design

**Date:** 2026-06-12
**Status:** Approved (pending spec review)

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
| Code structure | New own repo; vendor pedramamini's CC0 script with credit |
| Output target | Plain Markdown folder, one file per memo |
| Transcription | **Apple native only** — no Whisper dependency |
| Trigger | launchd **WatchPaths** (instant) + **hourly sweep** (safety net) |
| Configuration | All tunables in a `config.toml` (stdlib `tomllib`) |

## Architecture

Two parts:

1. **`voice_memo_capture.py`** — our orchestration script (dependency-free,
   Python 3.11+ stdlib only). Reads config, queries the DB, extracts native
   transcripts, writes Markdown, maintains an idempotency ledger.
2. **launchd LaunchAgent** — keeps the script running unattended: fires on
   folder change and on an hourly interval.

It vendors **`voice_memos.py`** from pedramamini's gist (CC0) for the
DB-query + `tsrp`-atom-extraction logic, rather than reimplementing it.

### Repo layout (`~/git/mattwynne/voice-memo-capture`)

```
voice-memo-capture/
├── README.md                       # setup + the Full Disk Access step
├── LICENSE                         # CC0 (matches upstream)
├── CREDITS.md                      # link back to pedramamini's gist
├── config.example.toml             # documented defaults, copied on install
├── vendor/
│   └── voice_memos.py              # pedramamini's CC0 script, vendored as-is
├── voice_memo_capture.py           # our script: extraction → markdown
├── install.sh                      # writes + loads LaunchAgent, prints FDA steps
├── uninstall.sh                    # unloads + removes the agent
├── com.matt.voicememocapture.plist # LaunchAgent template
└── tests/
    └── test_capture.py             # unit tests w/ committed fixtures
```

## Data flow

```
New memo syncs from iPhone → appears in the Recordings folder
        │
launchd fires (WatchPaths on the folder, OR the hourly sweep)
        │
voice_memo_capture.py:
  1. load config.toml (fall back to defaults for missing keys)
  2. query CloudRecordings.db → (id, title, date, duration, path, folder)
  3. for each memo not in the ledger:
       - resolve audio path; if not downloaded from iCloud yet → skip
       - extract Apple native transcript (tsrp atom) via vendored code
       - if transcript missing → skip, DO NOT ledger (retry next run)
       - else → write <output>/{date} {time} - {title}.md
                add memo id to the ledger
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
on_missing_transcript = "skip"   # "skip" | "placeholder"

[logging]
file = "~/Library/Logs/voice-memo-capture.log"
level = "info"                   # "debug" | "info" | "warn" | "error"
```

- Config is located next to the script, or via `$VOICE_MEMO_CAPTURE_CONFIG`.
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

The capture service reads a TCC-protected folder, so the **Python interpreter
that launchd runs** must have Full Disk Access. `install.sh` prints the exact
binary path to add and walks the user through:

> System Settings → Privacy & Security → Full Disk Access → add `<python3 path>`

This is unavoidable for any tool touching this folder. Everything else is
automatic. If FDA is absent at runtime, the service detects the
`Operation not permitted` error, logs a clear instruction, and exits cleanly.

## Idempotency & state

A JSON ledger at `~/.local/state/voice-memo-capture/processed.json` records
which memo IDs have been written. Re-runs skip ledgered memos. A memo with no
transcript yet is **not** ledgered, so the next sweep retries it once Apple
finishes on-device transcription. Safe to run any number of times.

## Error handling

| Condition | Behavior |
| --- | --- |
| Folder unreadable (FDA missing) | Log clear "grant Full Disk Access" message; exit 0 |
| `CloudRecordings.db` locked (app writing) | Catch, log at debug, retry next run |
| Audio not yet downloaded from iCloud | Skip memo, retry next run |
| Missing/malformed `tsrp` transcript atom | Skip that memo, log, continue with others |
| Any per-memo exception | Isolated — never aborts the whole batch |

All activity logged to the configured log file.

## Testing

Unit tests (`python3 -m unittest`), no dependency on the real library:

- Filename generation from `filename_format` tokens, including title
  sanitization (slashes, length).
- Ledger skip logic: a ledgered id is not rewritten; an un-ledgered id is.
- Native transcript extraction against a small **committed fixture** file
  containing a `tsrp` atom.
- Config loading: partial config merges over defaults; missing file uses
  defaults.

## Out of scope (YAGNI)

- Whisper / any non-native transcription.
- LLM summaries or tagging.
- Logseq / Obsidian integration (plain Markdown only; can point other tools
  at the folder later).
- Editing memos, renaming, or writing back to the Voice Memos DB.

## Credits

Built on [pedramamini's Voice Memos gist](https://gist.github.com/pedramamini/f4efacfe7080e07e18f54e13d8243dc1)
(CC0). The vendored `voice_memos.py` provides DB querying and `tsrp`-atom
transcript extraction.
