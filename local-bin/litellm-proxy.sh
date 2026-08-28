#!/usr/bin/env bash

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

SCRIPT_DIR="$(dirname "$(realpath "${BASH_SOURCE[0]}")")"
readonly SCRIPT_DIR
readonly LITELLM_COMPOSE_FILE="${SCRIPT_DIR}/../configs/litellm/compose.yaml"
readonly LITELLM_COPILOT_VOLUME="litellm-copilot-data"
readonly DEFAULT_LITELLM_URL="http://litellm.orb.local:4000"
readonly DEFAULT_LITELLM_MODEL="claude-fable-5"
readonly LITELLM_URL="${LITELLM_URL:-${DEFAULT_LITELLM_URL}}"
readonly LITELLM_MODEL="${LITELLM_MODEL:-${DEFAULT_LITELLM_MODEL}}"

function info() {
    echo "ℹ️  ${1}"
}

function get_keychain_secret() {
    local service="${1}"

    if [[ "$(uname -s)" == "Darwin" ]] && command -v security &>/dev/null; then
        security find-generic-password -a "${USER}" -s "${service}" -w 2>/dev/null
        return
    fi

    return 1
}

# Each secret is read from the environment variable named ${1}, then from
# the macOS Keychain service of the same name.
function get_secret() {
    local name="${1}"
    local description="${2}"

    if [[ -n "${!name:-}" ]]; then
        printf '%s' "${!name}"
        return 0
    fi

    if get_keychain_secret "${name}"; then
        return 0
    fi

    err "❌ No ${description} is available."
    err "   Set ${name} or save it in Keychain service '${name}'."
    return 1
}

function require_docker() {
    if ! command -v docker &>/dev/null; then
        err "❌ Docker is not installed."
        return 1
    fi

    if ! docker info >/dev/null 2>&1; then
        err "❌ The Docker daemon is not running."
        return 1
    fi

    if ! docker compose version >/dev/null 2>&1; then
        err "❌ Docker Compose is not available."
        return 1
    fi
}

function run_compose() {
    local litellm_master_key
    local litellm_salt_key
    local wandb_api_key
    local wandb_qa_api_key

    require_docker
    litellm_master_key=$(get_secret LITELLM_MASTER_KEY "LiteLLM proxy key")
    litellm_salt_key=$(get_secret LITELLM_SALT_KEY "LiteLLM salt key")
    wandb_api_key=$(get_secret WANDB_API_KEY "W&B API key")
    wandb_qa_api_key=$(get_secret WANDB_QA_API_KEY "W&B QA API key")

    LITELLM_MASTER_KEY="${litellm_master_key}" \
        LITELLM_SALT_KEY="${litellm_salt_key}" \
        WANDB_API_KEY="${wandb_api_key}" \
        WANDB_QA_API_KEY="${wandb_qa_api_key}" \
        docker compose --file "${LITELLM_COMPOSE_FILE}" "$@"
}

function wait_for_health() {
    local attempt=0
    local container_health
    local device_prompt=""
    local last_device_prompt=""
    local max_attempts=120

    printf "⏳ Waiting for LiteLLM to become healthy "
    while [[ "${attempt}" -lt "${max_attempts}" ]]; do
        container_health=$(docker inspect --format '{{.State.Health.Status}}' litellm 2>/dev/null || true)
        if [[ "${container_health}" == "healthy" ]] && curl -fsS "${LITELLM_URL}/health/liveliness" >/dev/null 2>&1; then
            echo ""
            echo "✅ LiteLLM is healthy at ${LITELLM_URL}."
            return 0
        fi

        device_prompt=$(docker logs --since 5m litellm 2>&1 |
            rg -o 'Please visit https://github.com/login/device and enter code [A-Z0-9-]+ to authenticate\.' |
            tail -n 1 || true)
        if [[ -n "${device_prompt}" ]] && [[ "${device_prompt}" != "${last_device_prompt}" ]]; then
            echo ""
            echo "${device_prompt}"
            printf "⏳ Waiting for GitHub approval and LiteLLM startup "
            last_device_prompt="${device_prompt}"
        fi

        printf "."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo ""
    err "❌ LiteLLM did not become healthy within $((max_attempts * 2)) seconds."
    return 1
}

function require_healthy_proxy() {
    if ! curl -fsS "${LITELLM_URL}/health/liveliness" >/dev/null 2>&1; then
        err "❌ LiteLLM is not reachable at ${LITELLM_URL}."
        return 1
    fi
}

function start_proxy() {
    require_docker
    docker volume create "${LITELLM_COPILOT_VOLUME}" >/dev/null
    run_compose up --detach
    wait_for_health
}

function stop_proxy() {
    run_compose down
}

function restart_proxy() {
    require_docker
    docker volume create "${LITELLM_COPILOT_VOLUME}" >/dev/null
    run_compose up --detach --force-recreate
    wait_for_health
}

function proxy_status() {
    run_compose ps
}

function proxy_logs() {
    run_compose logs --follow "$@"
}

function list_models() {
    local api_key

    require_healthy_proxy
    api_key=$(get_secret LITELLM_MASTER_KEY "LiteLLM proxy key")

    curl -fsS "${LITELLM_URL}/v1/models" \
        --header "Authorization: Bearer ${api_key}" |
        jq -r '.data[]?.id' |
        sort
}

function copy_key() {
    local api_key

    if ! command -v pbcopy &>/dev/null; then
        err "❌ pbcopy is not available; the key command requires macOS."
        return 1
    fi

    api_key=$(get_secret LITELLM_MASTER_KEY "LiteLLM proxy key")
    printf '%s' "${api_key}" | pbcopy
    info "Master key copied to the clipboard. Log in to ${LITELLM_URL}/ui as 'admin'."
}

function test_model() {
    local api_key
    local model="${1}"
    local request_body
    local response

    require_healthy_proxy
    api_key=$(get_secret LITELLM_MASTER_KEY "LiteLLM proxy key")
    request_body=$(jq -n \
        --arg model "${model}" \
        '{model: $model, max_tokens: 256, messages: [{role: "user", content: "Reply with exactly ok and no punctuation."}]}')

    response=$(curl -fsS "${LITELLM_URL}/v1/messages" \
        --header "Authorization: Bearer ${api_key}" \
        --header "Content-Type: application/json" \
        --header "anthropic-version: 2023-06-01" \
        --data "${request_body}")

    echo "${response}" | jq -er '.content[] | select(.type == "text") | .text'
}

function run_claude() {
    local api_key

    require_healthy_proxy
    api_key=$(get_secret LITELLM_MASTER_KEY "LiteLLM proxy key")

    info "Launching Claude Code with ${LITELLM_MODEL} through ${LITELLM_URL}."
    env \
        -u ANTHROPIC_API_KEY \
        -u CLAUDE_CODE_USE_BEDROCK \
        -u CLAUDE_CODE_USE_FOUNDRY \
        -u CLAUDE_CODE_USE_VERTEX \
        ANTHROPIC_BASE_URL="${LITELLM_URL}" \
        ANTHROPIC_AUTH_TOKEN="${api_key}" \
        ANTHROPIC_MODEL="${LITELLM_MODEL}" \
        ANTHROPIC_DEFAULT_OPUS_MODEL="${LITELLM_MODEL}" \
        ANTHROPIC_DEFAULT_SONNET_MODEL="${LITELLM_MODEL}" \
        ANTHROPIC_DEFAULT_HAIKU_MODEL="${LITELLM_MODEL}" \
        ANTHROPIC_SMALL_FAST_MODEL="${LITELLM_MODEL}" \
        CLAUDE_CODE_SUBAGENT_MODEL="${LITELLM_MODEL}" \
        CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS=1 \
        claude --model "${LITELLM_MODEL}" "$@"
}

function usage() {
    echo "Usage: litellm-proxy.sh <subcommand> [arguments...]"
    echo ""
    echo "Subcommands:"
    echo "  start          Create or start the Compose-managed LiteLLM container"
    echo "  stop           Stop the container while preserving Copilot OAuth"
    echo "  restart        Recreate the container from checked-in configuration"
    echo "  status         Show the Compose service status"
    echo "  logs           Follow LiteLLM logs"
    echo "  models         List configured models"
    echo "  key            Copy the master key to the clipboard for the admin UI login"
    echo "  test-copilot   Test claude-sonnet-4-6 through GitHub Copilot"
    echo "  test-wandb     Test wandb/zai-org/GLM-5.2"
    echo "  claude         Launch Claude Code; remaining arguments are passed through"
    echo ""
    echo "Environment variables:"
    echo "  LITELLM_URL         Proxy URL (default: ${DEFAULT_LITELLM_URL})"
    echo "  LITELLM_MODEL       Claude Code model (default: ${DEFAULT_LITELLM_MODEL})"
    echo "  LITELLM_MASTER_KEY  Proxy key"
    echo "  LITELLM_SALT_KEY    DB encryption salt; set once and never change it"
    echo "  WANDB_API_KEY       W&B Inference key"
    echo "  WANDB_QA_API_KEY    W&B QA Inference key"
    echo ""
    echo "Each key falls back to the macOS Keychain entry whose service name"
    echo "equals the variable name."
    echo ""
    echo "Example:"
    echo "  litellm-proxy.sh claude --dangerously-skip-permissions --allow-dangerously-skip-permissions"
}

case "${1:-}" in
start)
    shift
    start_proxy "$@"
    ;;
stop)
    shift
    stop_proxy "$@"
    ;;
restart)
    shift
    restart_proxy "$@"
    ;;
status)
    shift
    proxy_status "$@"
    ;;
logs)
    shift
    proxy_logs "$@"
    ;;
models)
    shift
    list_models "$@"
    ;;
key)
    shift
    copy_key "$@"
    ;;
test-copilot)
    shift
    test_model "claude-sonnet-4-6"
    ;;
test-wandb)
    shift
    test_model "wandb/zai-org/GLM-5.2"
    ;;
claude)
    shift
    run_claude "$@"
    ;;
-h | --help | help)
    usage
    ;;
*)
    err "❌ Unknown subcommand: ${1:-<none>}"
    usage
    exit 1
    ;;
esac
