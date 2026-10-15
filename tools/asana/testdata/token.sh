#!/usr/bin/env bash

set -euo pipefail

printf 'called\n' >"${TOKEN_CALL_FILE}"
printf '%s' "${1:-}"
printf '%s' "${2:-}" >&2
exit "${3:-0}"
