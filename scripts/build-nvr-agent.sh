#!/usr/bin/env bash
# Builds nvr-agent for Windows 7 x64. Go 1.21+ binaries do not run on Windows 7,
# so this refuses any toolchain other than Go 1.20.x.
#   go install golang.org/dl/go1.20.14@latest && go1.20.14 download
#   GO=go1.20.14 bash scripts/build-nvr-agent.sh [goos] [goarch]
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$ROOT/agent/nvr-agent"
GO="${GO:-go1.20.14}"
GOOS_TARGET="${1:-windows}"
GOARCH_TARGET="${2:-amd64}"
VERSION="${VERSION:-$(git -C "$ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)}"

if ! command -v "$GO" >/dev/null 2>&1; then
  for candidate in "$HOME/go/bin/$GO" "$(go env GOPATH 2>/dev/null)/bin/$GO"; do
    if [ -x "$candidate" ]; then GO="$candidate"; break; fi
  done
fi
GO_VERSION="$(GOTOOLCHAIN=local "$GO" env GOVERSION)"
case "$GO_VERSION" in
  go1.20|go1.20.*) ;;
  *) echo "need Go 1.20.x for Windows 7 support, got $GO_VERSION (set GO=go1.20.14)" >&2; exit 1 ;;
esac

OUT="$ROOT/build/nvr-agent-$GOOS_TARGET-$GOARCH_TARGET"
rm -rf "$OUT"
mkdir -p "$OUT"

EXE="nvr-agent"
[ "$GOOS_TARGET" = "windows" ] && EXE="nvr-agent.exe"

(cd "$SRC" && GOTOOLCHAIN=local CGO_ENABLED=0 GOOS="$GOOS_TARGET" GOARCH="$GOARCH_TARGET" \
  "$GO" build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o "$OUT/$EXE" .)

cp "$SRC/nvr-agent.example.json" "$SRC/README.md" "$OUT/"
if [ "$GOOS_TARGET" = "windows" ]; then
  for bat in start.bat probe.bat; do
    sed 's/\r$//; s/$/\r/' "$SRC/deploy/$bat" > "$OUT/$bat"
  done
  mkdir -p "$OUT/ffmpeg/bin"
  echo "put ffmpeg.exe and ffprobe.exe here (see README.md)" > "$OUT/ffmpeg/bin/README.txt"
fi

echo "built $OUT ($GO_VERSION, $VERSION)"
