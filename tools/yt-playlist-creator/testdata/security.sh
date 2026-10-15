#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' CALL "$@" >>"${SECURITY_CALLS_FILE}"
printf '%s' "${SECURITY_DIAGNOSTIC:-}" >&2

case "${1:-}" in
-i)
    IFS= read -r command
    printf '%s\n' "$command" >"${SECURITY_INPUT_FILE}"
    if [[ "${SECURITY_BLOCK:-}" == write ]]; then
        exec /bin/sleep 30
    fi
    if [[ "${SECURITY_WRITE_EXIT:-0}" != 0 ]]; then
        exit "${SECURITY_WRITE_EXIT}"
    fi
    if [[ "${SECURITY_SKIP_WRITE:-0}" == 1 ]]; then
        exit 0
    fi
    [[ "$command" == 'add-generic-password -U -a '* ]]
    [[ "$command" == *' -s YT_PLAYLIST_CREATOR_OAUTH -D "api key" -X '* ]]
    printf '%s\n' "${command##* -X }" >"${SECURITY_STORED_FILE}"
    ;;
find-generic-password)
    if [[ "${SECURITY_BLOCK:-}" == read ]]; then
        exec /bin/sleep 30
    fi
    if [[ "${SECURITY_READ_EXIT:-0}" != 0 ]]; then
        exit "${SECURITY_READ_EXIT}"
    fi
    while IFS= read -r line || [[ -n "$line" ]]; do
        printf '%s\n' "$line"
    done <"${SECURITY_STORED_FILE}"
    ;;
*)
    exit 64
    ;;
esac
