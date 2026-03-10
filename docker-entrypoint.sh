#!/bin/sh
set -eu

DATA_DIR=/data
IMAGE_BIN=/opt/grimoire/grimoire-bot
RUNTIME_BIN="$DATA_DIR/grimoire-bot"

mkdir -p "$DATA_DIR"
cp "$IMAGE_BIN" "$RUNTIME_BIN"
chmod +x "$RUNTIME_BIN"

cd "$DATA_DIR"
exec "$RUNTIME_BIN" "$@"
