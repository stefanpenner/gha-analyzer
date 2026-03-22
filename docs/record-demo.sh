#!/bin/sh
# Records an asciinema demo of otel-explorer and converts to SVG.
#
# Usage: ./docs/record-demo.sh
#
# Prerequisites:
#   brew install asciinema
#   npm install -g svg-term-cli
#
# The script uses expect(1) to drive the TUI with timed keystrokes,
# producing a deterministic recording every time.

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
CAST_FILE="${SCRIPT_DIR}/demo.cast"
SVG_FILE="${SCRIPT_DIR}/demo.svg"
BINARY="otel-explorer"
COLS=120
ROWS=35

# Ensure binary is built
if ! command -v "$BINARY" >/dev/null 2>&1; then
  echo "Building ${BINARY}..."
  (cd "${SCRIPT_DIR}/.." && go build -o "${BINARY}" ./cmd/otel-explorer)
  PATH="${SCRIPT_DIR}/..:${PATH}"
fi

echo "Recording demo..."
asciinema rec "$CAST_FILE" \
  --overwrite \
  --cols "$COLS" \
  --rows "$ROWS" \
  --output-format asciicast-v2 \
  --command "expect -f ${SCRIPT_DIR}/demo-driver.exp"

# Patch header dimensions — asciinema headless mode forces 80x24 in the header
# but expect's stty_init creates the pty at the correct size, so the content
# is rendered correctly. We just need the header to match.
python3 -c "
import json, sys
lines = open('$CAST_FILE').readlines()
header = json.loads(lines[0])
header['width'] = $COLS
header['height'] = $ROWS
lines[0] = json.dumps(header) + '\n'
open('$CAST_FILE', 'w').writelines(lines)
"

echo "Converting to SVG..."
svg-term --in "$CAST_FILE" --out "$SVG_FILE" \
  --window \
  --no-cursor

echo "Done: ${SVG_FILE}"
