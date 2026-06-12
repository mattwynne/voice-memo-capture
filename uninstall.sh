#!/usr/bin/env bash
set -euo pipefail

BINARY="$HOME/.local/bin/voice-memo-capture"
AGENT="$HOME/Library/LaunchAgents/net.mattwynne.voicememocapture.plist"

if [ -f "$AGENT" ]; then
  launchctl unload "$AGENT" 2>/dev/null || true
  rm -f "$AGENT"
  echo "Removed LaunchAgent: $AGENT"
fi
rm -f "$BINARY" && echo "Removed binary." || true
echo "Note: config (~/.config/voice-memo-capture) and transcripts are left in place."
echo "Remember to remove the binary from Full Disk Access in System Settings."
