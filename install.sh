#!/usr/bin/env bash
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="$HOME/.local/bin"
BINARY="$BIN_DIR/voice-memo-capture"
CONFIG_DIR="$HOME/.config/voice-memo-capture"
CONFIG="$CONFIG_DIR/config.toml"
LOG="$HOME/Library/Logs/voice-memo-capture.log"
WATCHDIR="$HOME/Library/Group Containers/group.com.apple.VoiceMemos.shared/Recordings"
AGENT="$HOME/Library/LaunchAgents/net.mattwynne.voicememocapture.plist"
INTERVAL="${VMC_CHECK_INTERVAL_SECONDS:-300}"

usage() {
  cat <<EOF
Usage: ./install.sh [--interval SECONDS]

Options:
  --interval SECONDS   Safety-net sweep interval for launchd. Default: 300.
  -h, --help           Show this help.

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
  echo "==> Checking Voice Memos Full Disk Access"
  mkdir -p "$(dirname "$LOG")"
  touch "$LOG"
  before_size=$(stat -f%z "$LOG" 2>/dev/null || echo 0)

  "$BINARY" || true

  new_log=$(tail -c +$((before_size + 1)) "$LOG" 2>/dev/null || true)
  if printf '%s' "$new_log" | grep -q "Full Disk Access not granted"; then
    cat <<EOF
    ❌ Full Disk Access is still blocked.

    Add this exact binary, then enable its toggle:
      $BINARY

    Logs: $LOG
EOF
    return 1
  fi

  echo "    ✅ Full Disk Access check passed. Logs: $LOG"
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

echo "==> Installing LaunchAgent"
echo "    sweep interval: ${INTERVAL}s"
mkdir -p "$HOME/Library/LaunchAgents"
sed -e "s|__BINARY__|$BINARY|g" \
    -e "s|__WATCHDIR__|$WATCHDIR|g" \
    -e "s|__LOG__|$LOG|g" \
    -e "s|<integer>300</integer>|<integer>$INTERVAL</integer>|g" \
    "$REPO_DIR/net.mattwynne.voicememocapture.plist" > "$AGENT"

assist_full_disk_access
verify_access_with_retry

# reload cleanly if already loaded
launchctl unload "$AGENT" 2>/dev/null || true
launchctl load -w "$AGENT"

cat <<EOF

==> Installed.

Binary: $BINARY
LaunchAgent: $AGENT
Config: $CONFIG
Logs: $LOG
Output: $HOME/Documents/Voice Memo Transcripts
EOF
