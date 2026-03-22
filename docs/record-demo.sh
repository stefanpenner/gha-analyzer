#!/bin/sh
# Records a demo of otel-explorer and converts to SVG.
#
# Usage: ./docs/record-demo.sh
#
# Prerequisites:
#   npm install -g svg-term-cli
#
# The Python driver creates a properly-sized pty, spawns otel-explorer,
# sends scripted keystrokes, and writes an asciinema v2 .cast file.
# No asciinema binary needed.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CAST_FILE="${SCRIPT_DIR}/demo.cast"
SVG_FILE="${SCRIPT_DIR}/demo.svg"
BINARY="otel-explorer"

# Ensure binary is built
if ! command -v "$BINARY" >/dev/null 2>&1; then
  echo "Building ${BINARY}..."
  (cd "${SCRIPT_DIR}/.." && go build -o "${BINARY}" ./cmd/otel-explorer)
  PATH="${SCRIPT_DIR}/..:${PATH}"
fi

echo "Recording demo..."
python3 "${SCRIPT_DIR}/demo-driver.py" "$CAST_FILE"

echo "Converting to SVG..."
svg-term --in "$CAST_FILE" --out "$SVG_FILE" \
  --window \
  --no-cursor

echo "Done: ${SVG_FILE}"
