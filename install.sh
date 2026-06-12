#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$HOME/.local/bin"
BINARY="$BIN_DIR/voice-memo-capture"
CONFIG_DIR="$HOME/.config/voice-memo-capture"
CONFIG="$CONFIG_DIR/config.toml"
LOG="$HOME/Library/Logs/voice-memo-capture.log"
WATCHDIR="$HOME/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"
LABEL="net.mattwynne.voicememocapture"
AGENT="$HOME/Library/LaunchAgents/$LABEL.plist"
INTERVAL="${VMC_CHECK_INTERVAL_SECONDS:-300}"
WHISPER_CHOICE="auto"
WHISPER_MODEL="base.en"
MODEL_DIR="$HOME/.local/share/voice-memo-capture/models"

usage() {
  cat <<EOF
Usage: ./install.sh [--interval SECONDS] [--with-whisper|--without-whisper] [--whisper-model MODEL]

Options:
  --interval SECONDS       Safety-net sweep interval for launchd. Default: 300.
  --with-whisper           Configure local whisper.cpp fallback.
  --without-whisper        Do not configure local Whisper fallback.
  --whisper-model MODEL    Whisper model to download: tiny.en, base.en, or small.en. Default: base.en.
  -h, --help               Show this help.

Environment:
  VMC_CHECK_INTERVAL_SECONDS   Alternative way to set --interval.
EOF
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --interval)
      if [ "$#" -lt 2 ]; then
        echo "error: --interval requires a value" >&2
        exit 2
      fi
      INTERVAL="$2"
      shift 2
      ;;
    --interval=*)
      INTERVAL="${1#--interval=}"
      shift
      ;;
    --with-whisper)
      WHISPER_CHOICE="yes"
      shift
      ;;
    --without-whisper)
      WHISPER_CHOICE="no"
      shift
      ;;
    --whisper-model)
      if [ "$#" -lt 2 ]; then
        echo "error: --whisper-model requires a value" >&2
        exit 2
      fi
      WHISPER_MODEL="$2"
      shift 2
      ;;
    --whisper-model=*)
      WHISPER_MODEL="${1#--whisper-model=}"
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "error: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if ! [[ "$INTERVAL" =~ ^[0-9]+$ ]] || [ "$INTERVAL" -le 0 ]; then
  echo "error: --interval must be a positive number of seconds" >&2
  exit 2
fi

case "$WHISPER_MODEL" in
  tiny.en|base.en|small.en) ;;
  *)
    echo "error: --whisper-model must be one of: tiny.en, base.en, small.en" >&2
    exit 2
    ;;
esac

configure_whisper_choice() {
  if [ "$WHISPER_CHOICE" != "auto" ]; then
    return
  fi
  if [ -t 0 ]; then
    cat <<EOF

Apple's native Voice Memos transcripts can stay pending for a long time or fail
silently. Local Whisper fallback can transcribe those memos on this Mac instead,
using whisper.cpp and a downloaded model. It is private/local, but uses CPU/GPU
time and disk space for the model.
EOF
    printf "Enable local Whisper fallback? [Y/n]: "
    read -r answer
    case "${answer:-Y}" in
      y|Y|yes|YES) WHISPER_CHOICE="yes" ;;
      n|N|no|NO) WHISPER_CHOICE="no" ;;
      *) WHISPER_CHOICE="yes" ;;
    esac
  else
    WHISPER_CHOICE="no"
  fi
}

install_whisper_config() {
  [ "$WHISPER_CHOICE" = "yes" ] || return 0

  echo "==> Configuring local Whisper fallback"
  if ! command -v whisper-cli >/dev/null 2>&1; then
    cat <<EOF

whisper-cli was not found. It comes from Homebrew's whisper-cpp package:
  brew install whisper-cpp
EOF
    if [ -t 0 ] && command -v brew >/dev/null 2>&1; then
      printf "Install whisper-cpp with Homebrew now? [Y/n]: "
      read -r answer
      case "${answer:-Y}" in
        y|Y|yes|YES)
          brew install whisper-cpp
          ;;
        *)
          echo "Skipping Whisper setup. You can rerun later with: ./install.sh --with-whisper"
          WHISPER_CHOICE="no"
          return 0
          ;;
      esac
    else
      cat <<EOF >&2
error: cannot continue Whisper setup without whisper-cli.
Install it, then rerun:
  brew install whisper-cpp
  ./install.sh --with-whisper
EOF
      exit 1
    fi
  fi
  if ! command -v whisper-cli >/dev/null 2>&1; then
    echo "error: whisper-cli still was not found after install attempt" >&2
    exit 1
  fi
  whisper_binary="$(command -v whisper-cli)"
  if ! "$whisper_binary" --help >/dev/null 2>&1; then
    echo "error: whisper-cli --help failed" >&2
    exit 1
  fi
  if ! command -v afconvert >/dev/null 2>&1; then
    echo "error: afconvert was not found; it is required to prepare audio for Whisper" >&2
    exit 1
  fi
  if ! command -v curl >/dev/null 2>&1; then
    echo "error: curl was not found; it is required to download the Whisper model" >&2
    exit 1
  fi

  mkdir -p "$MODEL_DIR"
  model_file="$MODEL_DIR/ggml-$WHISPER_MODEL.bin"
  if [ ! -f "$model_file" ]; then
    cat <<EOF

Whisper needs a GGML model file. The installer will download:
  ggml-$WHISPER_MODEL.bin

to:
  $model_file
EOF
    curl -fL --progress-bar "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-$WHISPER_MODEL.bin" -o "$model_file"
  else
    echo "    model already exists: $model_file"
  fi
  if [ ! -f "$model_file" ]; then
    echo "error: model file missing after download: $model_file" >&2
    exit 1
  fi

  python3 - "$CONFIG" "$model_file" "$whisper_binary" <<'PY'
from pathlib import Path
import re
import sys
path = Path(sys.argv[1])
model = sys.argv[2]
text = path.read_text() if path.exists() else ""
binary = sys.argv[3]
section = f'''[whisper]\nbinary = "{binary}"\nmodel = "{model}"\nlanguage = "en"\nthreads = 0\nwhen = "apple-missing"\ntimeout_seconds = 1800\nkeep_wav = false\n'''
if re.search(r'(?ms)^\[whisper\]\n.*?(?=^\[|\Z)', text):
    text = re.sub(r'(?ms)^\[whisper\]\n.*?(?=^\[|\Z)', section + "\n", text)
else:
    if text and not text.endswith("\n"):
        text += "\n"
    text += "\n" + section
path.write_text(text)
print(f"    updated {path}: [whisper]")
PY
}

assist_full_disk_access() {
  if [ "${VMC_SKIP_FDA_PROMPT:-}" = "1" ] || [ ! -t 0 ]; then
    cat <<EOF
==> Full Disk Access still required
    Add this binary in System Settings > Privacy & Security > Full Disk Access:
      $BINARY
EOF
    return
  fi

  cat <<EOF

==> Full Disk Access required

macOS protects the Voice Memos folder. The installer cannot grant this
permission for you, but it can open the right places.

In the Full Disk Access window:
  1. Click +
  2. Press Cmd+Shift+G
  3. Paste this exact path:
       $BINARY
  4. Click Open
  5. Turn on the toggle for voice-memo-capture

Opening System Settings and revealing the binary now...
EOF

  open -R "$BINARY" 2>/dev/null || true
  open "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles" 2>/dev/null || true

  printf "\nPress Return once Full Disk Access is enabled (or press Return to skip for now): "
  read -r _
}

verify_access() {
  echo "==> Checking Voice Memos Full Disk Access via launchd"
  mkdir -p "$(dirname "$LOG")"
  touch "$LOG"
  before_size=$(stat -f%z "$LOG" 2>/dev/null || echo 0)

  # Verify the same execution context that will run in normal operation. A
  # direct Terminal/shell run can report a false TCC denial even when launchd is
  # allowed to read the Voice Memos container.
  launchctl kickstart -k "gui/$(id -u)/$LABEL" 2>/dev/null || true
  sleep 2

  new_log=$(tail -c +$((before_size + 1)) "$LOG" 2>/dev/null || true)
  if printf '%s' "$new_log" | grep -q "Full Disk Access not granted"; then
    cat <<EOF
    ❌ Full Disk Access is still blocked for the LaunchAgent.

    Add this exact binary, then enable its toggle:
      $BINARY

    Logs: $LOG
EOF
    return 1
  fi

  if [ -z "$new_log" ]; then
    cat <<EOF
    ⚠️  The LaunchAgent did not write a verification log entry.

    Check manually:
      launchctl print gui/$(id -u)/$LABEL
      tail -n 50 $LOG
EOF
    return 1
  fi

  echo "    ✅ launchd Full Disk Access check passed. Logs: $LOG"
  return 0
}

verify_access_with_retry() {
  if verify_access; then
    return 0
  fi

  if [ "${VMC_SKIP_FDA_PROMPT:-}" = "1" ] || [ ! -t 0 ]; then
    return 0
  fi

  while true; do
    printf "\nOpen Full Disk Access again and retry? [Y/n]: "
    read -r answer
    case "${answer:-Y}" in
      y|Y|yes|YES)
        assist_full_disk_access
        if verify_access; then
          return 0
        fi
        ;;
      n|N|no|NO)
        echo "    Continuing install, but transcripts will not update until FDA is fixed."
        return 0
        ;;
      *)
        echo "    Please answer y or n."
        ;;
    esac
  done
}

echo "==> Building"
mkdir -p "$BIN_DIR"
( cd "$REPO_DIR" && go build -o "$BINARY" ./cmd/voice-memo-capture )

echo "==> Ad-hoc code-signing (stable-ish TCC identity)"
codesign --force -s - "$BINARY"

configure_whisper_choice

echo "==> Installing config (if absent)"
mkdir -p "$CONFIG_DIR"
if [ ! -f "$CONFIG" ]; then
  cp "$REPO_DIR/config.example.toml" "$CONFIG"
  echo "    wrote $CONFIG"
else
  echo "    keeping existing $CONFIG"
  # Migrate the old generated default so existing installs get pending files.
  # If the user customized this line differently, leave it alone.
  python3 - "$CONFIG" <<'PY'
from pathlib import Path
import sys
path = Path(sys.argv[1])
text = path.read_text()
old = 'on_missing_transcript = "skip"   # "skip" | "placeholder"'
new = 'on_missing_transcript = "placeholder"   # "placeholder" | "skip"'
if old in text:
    path.write_text(text.replace(old, new))
    print(f"    migrated {path}: on_missing_transcript = placeholder")
PY
fi

install_whisper_config

echo "==> Installing LaunchAgent"
echo "    sweep interval: ${INTERVAL}s"
mkdir -p "$HOME/Library/LaunchAgents"
sed -e "s|__BINARY__|$BINARY|g" \
    -e "s|__WATCHDIR__|$WATCHDIR|g" \
    -e "s|__LOG__|$LOG|g" \
    -e "s|<integer>300</integer>|<integer>$INTERVAL</integer>|g" \
    "$REPO_DIR/net.mattwynne.voicememocapture.plist" > "$AGENT"

assist_full_disk_access

# reload cleanly if already loaded, then verify the launchd execution context
launchctl unload "$AGENT" 2>/dev/null || true
launchctl load -w "$AGENT"
verify_access_with_retry

cat <<EOF

==> Installed.

Binary: $BINARY
LaunchAgent: $AGENT
Config: $CONFIG
Logs: $LOG
Output: $HOME/Documents/Voice Memo Transcripts
EOF
