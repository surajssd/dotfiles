#!/usr/bin/env bash
# Cleans up the docker environment

set -euo pipefail

function lite() {
    echo "🧹 Cleaning up old containers and untagged images..."
    docker rm $(docker ps -a | grep Exited | awk '{print $1}') || true
    docker rmi $(docker images | grep none | awk '{print $3}') || true
}

function full() {
    lite
    docker buildx prune -af
    echo "🚀 Cleaning up all unused containers, images, networks, and volumes..."
    docker system prune -a --volumes -f
}

function usage() {
    echo "📖 Usage: ${0} [Options]"
    echo ""
    echo "Options:"
    echo "    --lite, -l    🧹 Perform a light cleanup (remove old containers and untagged images)"
    echo "    --full, -f    🚀 Perform a full cleanup (remove all unused containers, images, networks, and volumes)"
    echo "    --help, -h    ❓ Show this help message"
}

function main() {
    if [[ $# -eq 0 ]]; then
        usage
        exit 1
    fi

    while [[ $# -gt 0 ]]; do
        case "${1:-}" in
        --lite | -l)
            lite
            exit 0
            ;;
        --full | -f)
            full
            exit 0
            ;;
        --help | help | -h)
            usage
            exit 0
            ;;
        *)
            echo "❌ Unknown option: ${1}"
            usage
            exit 1
            ;;
        esac
    done
}

main "$@"
