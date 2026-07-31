#!/usr/bin/env bash
set -euo pipefail

# validate-commit-msg.sh — lint a Conventional Commits message with commitlint,
# run via bun. Mirrors the project's CI (commitlint@19.8.0 +
# @commitlint/config-conventional@19.8.0 + the conventionalcommits preset), but
# validates a single drafted message (via stdin) instead of a commit range.
#
# Usage:
#   validate-commit-msg.sh <message-file>      # lint the message in a file
#   printf '%s\n' 'feat: x' | validate-commit-msg.sh   # or pipe it on stdin
#
# Exit status is commitlint's own: 0 = passes, non-zero = fails. On failure the
# verbose report (per-rule pass/fail with explanations) is printed for the
# caller to read and act on.

# Dependencies are installed once into a cache dir (outside the repo) so the
# skill directory stays clean. package.json + commitlint.config.mjs live next to
# this script as the pinned source of truth and are copied in on first run or
# whenever their versions change.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
WORK_DIR="${XDG_CACHE_HOME:-$HOME/.cache}/conventional-commits-skill"

if ! command -v bun >/dev/null 2>&1; then
    echo "❌ bun is not installed or not on PATH — install it: https://bun.sh" >&2
    exit 127
fi

# Resolve the message: a file argument, or stdin if none was given.
tmp_msg=""
cleanup() {
    # Preserve the real exit status: an EXIT trap's last command otherwise
    # becomes the script's exit code (a false [[ -n "" ]] would mask a pass).
    local rc=$?
    [[ -n "$tmp_msg" ]] && rm -f "$tmp_msg"
    return "$rc"
}
trap cleanup EXIT

if [[ $# -ge 1 && -n "${1:-}" ]]; then
    msg_file="$1"
    if [[ ! -f "$msg_file" ]]; then
        echo "❌ message file not found: $msg_file" >&2
        exit 2
    fi
else
    tmp_msg="$(mktemp)"
    msg_file="$tmp_msg"
    cat >"$msg_file"
fi

mkdir -p "$WORK_DIR"

# Sync the pinned manifest + config into the cache dir. Reinstall only when the
# manifest changed or node_modules is missing; otherwise this runs fully offline.
needs_install=0
if ! cmp -s "$SCRIPT_DIR/package.json" "$WORK_DIR/package.json"; then
    cp "$SCRIPT_DIR/package.json" "$WORK_DIR/package.json"
    needs_install=1
fi
cp "$SCRIPT_DIR/commitlint.config.mjs" "$WORK_DIR/commitlint.config.mjs"
if [[ ! -d "$WORK_DIR/node_modules" ]]; then
    needs_install=1
fi

if [[ "$needs_install" -eq 1 ]]; then
    echo "⏳ installing commitlint into $WORK_DIR (one-time)…" >&2
    (cd "$WORK_DIR" && bun install)
fi

# Feed the message on stdin (not --edit) so markdown lines beginning with '#'
# are preserved rather than stripped as git comments.
(cd "$WORK_DIR" && bun x commitlint --config commitlint.config.mjs --verbose <"$msg_file")
