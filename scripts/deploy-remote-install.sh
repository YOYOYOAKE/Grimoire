#!/usr/bin/env bash
set -euo pipefail

DEPLOY_DIR="/opt/grimoire"
DEPLOY_TMP_BIN="${DEPLOY_DIR}/grimoire-bot.new"
DEPLOY_BIN="${DEPLOY_DIR}/grimoire-bot"
DEPLOY_UNIT_TEMPLATE="${DEPLOY_DIR}/grimoire-bot.service"
SYSTEMD_UNIT="/etc/systemd/system/grimoire-bot.service"
DEPLOY_RESTART_CMD="${DEPLOY_RESTART_CMD:-}"

run_privileged() {
	if [[ "$(id -u)" -eq 0 ]]; then
		"$@"
		return
	fi

	if command -v sudo >/dev/null 2>&1; then
		sudo "$@"
		return
	fi

	echo "sudo is required to manage ${SYSTEMD_UNIT}" >&2
	exit 1
}

mkdir -p "$DEPLOY_DIR"

if [[ ! -f "$DEPLOY_TMP_BIN" ]]; then
	echo "uploaded binary not found: $DEPLOY_TMP_BIN" >&2
	exit 1
fi

if [[ ! -f "$SYSTEMD_UNIT" ]]; then
	if [[ ! -f "$DEPLOY_UNIT_TEMPLATE" ]]; then
		echo "uploaded systemd unit not found: $DEPLOY_UNIT_TEMPLATE" >&2
		exit 1
	fi

	run_privileged install -m 644 "$DEPLOY_UNIT_TEMPLATE" "$SYSTEMD_UNIT"
	run_privileged systemctl daemon-reload
	run_privileged systemctl enable grimoire-bot
fi

chmod 755 "$DEPLOY_TMP_BIN"
mv -f "$DEPLOY_TMP_BIN" "$DEPLOY_BIN"

if [[ -n "$DEPLOY_RESTART_CMD" ]]; then
	bash -lc "$DEPLOY_RESTART_CMD"
fi
