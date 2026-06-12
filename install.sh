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
LEGACY_AGENT="$HOME/Library/LaunchAgents/com.matt.voicememocapture.plist"

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
  echo "==> Checking Voice Memos access"
  mkdir -p "$(dirname "$LOG")"
  touch "$LOG"
  before_size=$(stat -f%z "$LOG" 2>/dev/null || echo 0)

  "$BINARY" || true

  new_log=$(tail -c +$((before_size + 1)) "$LOG" 2>/dev/null || true)
  if printf '%s' "$new_log" | grep -q "Full Disk Access not granted"; then
    cat <<EOF
    Still blocked by macOS Full Disk Access.
    You can finish setup later by adding:
      $BINARY
    Logs: $LOG
EOF
  else
    echo "    Voice Memos access check completed. Logs: $LOG"
  fi
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
fi

echo "==> Installing LaunchAgent"
mkdir -p "$HOME/Library/LaunchAgents"
sed -e "s|__BINARY__|$BINARY|g" \
    -e "s|__WATCHDIR__|$WATCHDIR|g" \
    -e "s|__LOG__|$LOG|g" \
    "$REPO_DIR/net.mattwynne.voicememocapture.plist" > "$AGENT"

assist_full_disk_access
verify_access

# reload cleanly if already loaded, and remove the pre-rename LaunchAgent if present
launchctl unload "$AGENT" 2>/dev/null || true
if [ -f "$LEGACY_AGENT" ]; then
  launchctl unload "$LEGACY_AGENT" 2>/dev/null || true
  rm -f "$LEGACY_AGENT"
fi
launchctl load -w "$AGENT"

cat <<EOF

==> Installed.

Binary: $BINARY
LaunchAgent: $AGENT
Config: $CONFIG
Logs: $LOG
Output: $HOME/Documents/Voice Memo Transcripts
EOF
