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
