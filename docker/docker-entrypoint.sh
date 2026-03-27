#!/bin/sh
set -eu

CONFIG_DIR=/opt/grimoire/config

mkdir -p "$CONFIG_DIR"
exec /opt/grimoire/grimoire-bot "$@"
