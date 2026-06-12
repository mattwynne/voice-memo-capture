#!/usr/bin/env bash
set -euo pipefail

BINARY="$HOME/.local/bin/voice-memo-capture"
AGENT="$HOME/Library/LaunchAgents/net.mattwynne.voicememocapture.plist"
LEGACY_AGENT="$HOME/Library/LaunchAgents/com.matt.voicememocapture.plist"

for plist in "$AGENT" "$LEGACY_AGENT"; do
  if [ -f "$plist" ]; then
    launchctl unload "$plist" 2>/dev/null || true
    rm -f "$plist"
    echo "Removed LaunchAgent: $plist"
  fi
done
rm -f "$BINARY" && echo "Removed binary." || true
echo "Note: config (~/.config/voice-memo-capture) and transcripts are left in place."
echo "Remember to remove the binary from Full Disk Access in System Settings."
