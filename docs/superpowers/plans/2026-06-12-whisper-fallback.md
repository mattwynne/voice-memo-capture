# Whisper Fallback Iteration Plan

Goal: stop relying solely on Apple Voice Memos native transcripts. Keep Apple's embedded transcript as the fast path when it works, but fall back to local Whisper transcription for memos whose Apple transcript is missing, pending, or unparsable.

## Product behaviour

- For each memo:
  1. Try Apple embedded transcript first.
  2. If Apple transcript exists and parses, write the final Markdown as today.
  3. If Apple transcript is missing or malformed, use local Whisper if configured.
  4. If Whisper succeeds, write Markdown with transcript and metadata noting it came from Whisper.
  5. If neither Apple nor Whisper succeeds, write/keep the pending placeholder and retry later.
- Do not ledger a memo until a real transcript has been written.
- Placeholder files remain visible while work is pending.
- Re-runs may overwrite placeholders with real transcripts.
- Existing Apple-transcribed files should not be overwritten unless explicitly requested later.

## Implementation choice

Use `whisper.cpp` via its CLI (`whisper-cli`) rather than embedding Whisper in Go.

Reasons:
- Fastest reliable iteration.
- Avoids cgo/Metal binding complexity in this Go app.
- Lets users choose/install models independently.
- Works well on Apple Silicon with Homebrew whisper.cpp builds.

Expected external tools:
- `whisper-cli` from `whisper.cpp`.
- `afconvert`, already present on macOS, to convert `.m4a`/`.qta` to 16 kHz mono WAV if needed.

## Config

Add optional config section:

```toml
[whisper]
binary = "whisper-cli"
model = "~/.local/share/voice-memo-capture/models/ggml-base.en.bin"
language = "en"
threads = 0              # 0 = whisper.cpp default
when = "apple-missing"   # "apple-missing" | "apple-error" | "always"
timeout_seconds = 1800
keep_wav = false
```

Enablement rule:
- Whisper is enabled if a `[whisper]` section with a model path is present.
- No separate `enabled` flag.
- Existing installs remain Apple-only until the installer writes `[whisper]`.

## Installer UX

Add install options:

```bash
./install.sh                         # interactive: asks about Whisper, default yes
./install.sh --with-whisper
./install.sh --without-whisper
./install.sh --with-whisper --whisper-model base.en
./install.sh --with-whisper --whisper-model small.en
```

Interactive installer should ask:

```text
Enable local Whisper fallback? [Y/n]
```

Default is **Yes** for interactive installs.

Non-interactive installs should not prompt. They should enable Whisper only when `--with-whisper` is passed.

Installer should:
- Detect `whisper-cli`.
- If missing, print:
  `brew install whisper-cpp`
- Create model directory:
  `~/.local/share/voice-memo-capture/models`
- Download selected ggml model if missing.
- Add/update the `[whisper]` section in `~/.config/voice-memo-capture/config.toml` when Whisper is selected.
- Run a lightweight validation:
  - `whisper-cli --help` works.
  - model file exists.
  - `afconvert` exists.

Do not make Homebrew install automatically in the first iteration; explain the command and fail clearly.

## New packages

### `internal/audio`

Responsibilities:
- Convert source audio to temporary WAV for Whisper.
- Use macOS `afconvert`:
  - input: `.m4a` or `.qta`
  - output: temp `.wav`
  - format: 16 kHz mono PCM
- Clean up temp WAV unless `keep_wav = true`.

Tests:
- Unit-test command construction.
- Integration test can use a fake `afconvert` shell script to prove invocation.

### `internal/whisper`

Responsibilities:
- Run `whisper-cli` with configured binary/model/language/timeout.
- Prefer stdout/plain text output or `-otxt` output file, whichever is more stable for whisper.cpp.
- Return transcript text.
- Distinguish:
  - binary missing
  - model missing
  - timeout
  - command failed
  - empty transcript

Tests:
- Use fake executable scripts in temp dirs:
  - successful transcript
  - non-zero exit
  - empty output
  - timeout
  - missing model

### `internal/transcription`

Optional orchestration package to avoid bloating `main.go`.

Responsibilities:
- Given a memo/audio path and config, choose:
  - Apple only
  - Apple then Whisper fallback
  - Whisper always
- Return:
  - transcript text
  - source: `apple` or `whisper`
  - status: success, pending, error

## Markdown output changes

Add transcript source to metadata:

```markdown
- Transcript: Apple Voice Memos
```

or:

```markdown
- Transcript: Whisper (local)
```

Pending placeholder should say:

```markdown
_Transcript pending._

Apple has not produced a native transcript yet, and local Whisper has not completed successfully. This file will be overwritten automatically when a transcript is ready.
```

## Main loop changes

Current flow:
- Apple transcript succeeds → write + ledger.
- Apple missing → placeholder or skip.
- Apple error → log and continue.

New flow:
- Apple transcript succeeds → write source=Apple + ledger.
- Apple missing and `[whisper]` is configured → run Whisper.
- Apple parse error and configured `whisper.when` includes errors → run Whisper.
- Whisper succeeds → write source=Whisper + ledger.
- Whisper fails/missing/timeout → write placeholder, do not ledger.

Important: preserve per-memo isolation. A failed Whisper run should not stop other memos.

## Model recommendation

Start with `base.en` as installer default:
- Reasonably small.
- Fast enough on Apple Silicon.
- Better than waiting forever for Apple.

Document alternatives:
- `tiny.en`: fastest, lower quality.
- `base.en`: default.
- `small.en`: better, slower/larger.

## First implementation tasks

1. Add config fields and tests for optional `[whisper]` partial TOML merge.
2. Add `internal/audio` command wrapper with fake-command tests.
3. Add `internal/whisper` command wrapper with fake-command tests.
4. Extend `internal/output` to include transcript source metadata.
5. Refactor `main.go` into a small orchestration function that tries Apple then Whisper.
6. Add installer flags and README docs for `--with-whisper`.
7. Validate locally on one real memo whose Apple transcript is pending.
8. Push and verify CI.

## Open decisions

- Should Whisper be enabled by default for new curl installs, or only with `--with-whisper`?
  - Decision: interactive installer prompts with default Yes; non-interactive curl installs require `--with-whisper`.
- Should Whisper overwrite Apple transcripts if Apple later becomes available?
  - Recommendation: no for first iteration. Ledgered Whisper transcripts are final unless we add a `--refresh` mode later.
- Should long memos be transcribed in background concurrently?
  - Recommendation: no for first iteration. Keep run-once sequential semantics for launchd simplicity.
