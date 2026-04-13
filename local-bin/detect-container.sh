#!/usr/bin/env bash

# shellcheck disable=SC2329
# SC2329: Functions are invoked indirectly via the checks array in main()

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

VERBOSE=false

check_container_env() {
    # systemd-nspawn sets container=systemd-nspawn, Podman sets container=podman
    [[ -n "${container:-}" ]]
}

check_dockerenv() {
    # Docker creates /.dockerenv at container start
    [[ -f /.dockerenv ]]
}

check_containerenv() {
    # Podman creates /run/.containerenv
    [[ -f /run/.containerenv ]]
}

check_cgroup() {
    # cgroup v1: container runtime names appear in /proc/self/cgroup
    if [[ -f /proc/self/cgroup ]]; then
        grep -qE 'docker|kubepods|lxc|containerd|libpod' /proc/self/cgroup 2>/dev/null && return 0
    fi
    # cgroup v2: /proc/self/cgroup may just show 0::/ so check mountinfo instead
    if [[ -f /proc/self/mountinfo ]]; then
        grep -qE 'docker|kubepods|containerd|libpod' /proc/self/mountinfo 2>/dev/null && return 0
    fi
    return 1
}

check_mount_namespace() {
    # Different mount namespace inodes between PID 1 and self means we are namespaced
    local pid1_inode self_inode
    pid1_inode=$(stat -c %i /proc/1/ns/mnt 2>/dev/null) || return 1
    self_inode=$(stat -c %i /proc/self/ns/mnt 2>/dev/null) || return 1
    [[ "$pid1_inode" != "$self_inode" ]]
}

check_pid1() {
    # If PID 1 is not a known init system, we are likely in a container
    local pid1_name
    pid1_name=$(ps -p 1 -o comm= 2>/dev/null) || return 1
    # Strip path prefix (macOS returns /sbin/launchd instead of launchd)
    pid1_name="${pid1_name##*/}"
    case "$pid1_name" in
    systemd | init | linuxrc | launchd) return 1 ;;
    *) return 0 ;;
    esac
}

usage() {
    echo "Usage: ${0} [--verbose|-v] [--help|-h]"
    echo ""
    echo "Detect if running inside a container."
    echo "Exits 0 if in a container, 1 otherwise."
    echo ""
    echo "Options:"
    echo "  -v, --verbose  Print which check matched"
    echo "  -h, --help     Show this help message"
}

main() {
    while [[ $# -gt 0 ]]; do
        case "${1:-}" in
        --verbose | -v)
            VERBOSE=true
            shift
            ;;
        --help | -h)
            usage
            exit 0
            ;;
        *)
            err "❌ Unknown option: ${1}"
            usage
            exit 1
            ;;
        esac
    done

    local checks=(
        check_container_env
        check_dockerenv
        check_containerenv
        check_cgroup
        check_mount_namespace
        check_pid1
    )

    for check in "${checks[@]}"; do
        if "$check"; then
            [[ "$VERBOSE" == true ]] && echo "✅ Container detected via: ${check}"
            exit 0
        fi
    done

    [[ "$VERBOSE" == true ]] && echo "ℹ️ Not running inside a container"
    exit 1
}

main "$@"
