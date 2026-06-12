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
a default config, helps you grant Full Disk Access, verifies access, and loads
the launchd agent.

### Required: grant Full Disk Access

The Voice Memos folder is protected by macOS. You must grant the binary Full
Disk Access once. The installer opens System Settings and reveals the binary;
when prompted:

1. In System Settings → Privacy & Security → **Full Disk Access**, click **+**.
2. Press **Cmd+Shift+G** and paste:
   `~/.local/bin/voice-memo-capture`
3. Click **Open**.
4. Enable the toggle for `voice-memo-capture`.
5. Return to Terminal and press Return so the installer can verify access.

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
