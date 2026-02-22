#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "${BASH_SOURCE[0]}")"/util.sh

if ! command -v crontab &>/dev/null; then
    echo "❌ crontab is not installed."
    exit 1
fi

usage() {
    echo "Usage: $(basename "$0") <command> [args]"
    echo ""
    echo "Commands:"
    echo "  install <script-path> <cron-schedule>   Install a cron job for the given script"
    echo "  remove  <script-path>                   Remove the cron job for the given script"
    echo "  list                                    List all current user cron jobs"
    echo "  help                                    Show this help message"
    echo ""
    echo "Cron schedule format: minute hour day-of-month month day-of-week"
    echo "  Examples:"
    echo "    '*/5 * * * *'   - every 5 minutes"
    echo "    '0 * * * *'     - every hour"
    echo "    '0 0 * * *'     - every day at midnight"
    exit 1
}

validate_cron_schedule() {
    local schedule="$1"
    local field_count
    field_count=$(echo "$schedule" | awk '{print NF}')
    if [[ "$field_count" -ne 5 ]]; then
        echo "❌ Invalid cron schedule: expected 5 fields, got ${field_count}."
        echo "   Format: minute hour day-of-month month day-of-week"
        echo "   Example: */5 * * * *"
        exit 1
    fi
}

cmd_install() {
    if [[ -z "${1:-}" ]]; then
        echo "❌ Missing required argument: <script-path>"
        usage
    fi

    if [[ -z "${2:-}" ]]; then
        echo "❌ Missing required argument: <cron-schedule>"
        usage
    fi

    local script_path="$1"
    local schedule="$2"

    if [[ ! -f "$script_path" ]]; then
        echo "❌ File not found: ${script_path}"
        exit 1
    fi

    script_path="$(realpath "$script_path")"

    if [[ ! -x "$script_path" ]]; then
        echo "❌ File is not executable: ${script_path}"
        echo "   Run: chmod +x ${script_path}"
        exit 1
    fi

    if crontab -l 2>/dev/null | grep -qF "$script_path"; then
        echo "ℹ️ Cron job already exists for ${script_path}."
        exit 0
    fi

    validate_cron_schedule "$schedule"

    (
        crontab -l 2>/dev/null || true
        echo "$schedule $script_path"
    ) | crontab -

    echo "✅ Cron job installed: ${schedule} ${script_path}"
}

cmd_remove() {
    if [[ -z "${1:-}" ]]; then
        echo "❌ Missing required argument: <script-path>"
        usage
    fi

    local script_path="$1"

    if [[ -f "$script_path" ]]; then
        script_path="$(realpath "$script_path")"
    else
        # File may have been deleted, construct absolute path manually
        if [[ "$script_path" != /* ]]; then
            script_path="$(pwd)/${script_path}"
        fi
    fi

    if ! crontab -l 2>/dev/null | grep -qF "$script_path"; then
        echo "ℹ️ No cron job found for ${script_path}."
        exit 0
    fi

    crontab -l 2>/dev/null | grep -vF "$script_path" | crontab -

    echo "✅ Cron job removed for ${script_path}."
}

cmd_list() {
    local entries
    entries=$(crontab -l 2>/dev/null) || true
    if [[ -z "$entries" ]]; then
        echo "ℹ️ No cron jobs found."
    else
        echo "$entries"
    fi
}

COMMAND="${1:-}"
shift || true

case "$COMMAND" in
install)
    cmd_install "$@"
    ;;
remove)
    cmd_remove "$@"
    ;;
list)
    cmd_list
    ;;
help | --help | -h)
    usage
    ;;
*)
    usage
    ;;
esac
