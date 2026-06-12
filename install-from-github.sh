#!/usr/bin/env bash
set -euo pipefail

REPO="${VMC_REPO:-mattwynne/voice-memo-capture}"
REF="${VMC_REF:-main}"
ARCHIVE_URL="https://github.com/$REPO/archive/$REF.tar.gz"
TMPDIR="$(mktemp -d)"

cleanup() {
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "error: $1 is required but was not found on PATH" >&2
    exit 1
  fi
}

need curl
need tar
need go
need make

cat <<EOF
==> Installing voice-memo-capture from GitHub
    Repository: https://github.com/$REPO
    Ref: $REF
EOF

echo "==> Downloading source"
curl -fsSL "$ARCHIVE_URL" -o "$TMPDIR/source.tar.gz"

echo "==> Unpacking"
tar -xzf "$TMPDIR/source.tar.gz" -C "$TMPDIR"
SRC_DIR="$(find "$TMPDIR" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
if [ -z "$SRC_DIR" ] || [ ! -f "$SRC_DIR/Makefile" ]; then
  echo "error: downloaded archive did not contain the expected project files" >&2
  exit 1
fi

cd "$SRC_DIR"

# curl | bash gives this script a pipe on stdin. Reattach /dev/tty for the
# repo installer so it can pause while the user grants Full Disk Access.
if [ -r /dev/tty ]; then
  make install </dev/tty
else
  echo "warning: no /dev/tty available; Full Disk Access prompt will be skipped" >&2
  make install
fi
