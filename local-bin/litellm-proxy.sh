#!/usr/bin/env bash

set -euo pipefail

readonly TMP_LITELLM_VENV="/tmp/litellm"
readonly CLAUDE_SETTINGS_FILE="${HOME}/.claude/settings.json"
readonly LITELLM_CONFIG_FILE="${HOME}/.config/litellm/config.yaml"

function info() {
  echo "ℹ️ ${1}"
}

function source_venv() {
  # Create a virtualenv in the /tmp/litellm only if it does not exists
  if [ ! -d "${TMP_LITELLM_VENV}" ] || [ ! -f "${TMP_LITELLM_VENV}/bin/activate" ]; then
    info "Creating virtualenv in ${TMP_LITELLM_VENV}"

    # NOTE: This is a workaround for the python 3.14 issue where litellm fails
    # as follows:
    # ImportError: cannot import name 'BaseDefaultEventLoopPolicy' from 'asyncio.events'
    python3.11 -m venv "${TMP_LITELLM_VENV}"

    source "${TMP_LITELLM_VENV}/bin/activate"
    # https://github.com/BerriAI/litellm
    pip install 'litellm[proxy]'
  else
    info "Activating virtualenv in ${TMP_LITELLM_VENV}"
    source "${TMP_LITELLM_VENV}/bin/activate"
  fi
}

function reset_claude() {
  info "Resetting claude settings file at ${CLAUDE_SETTINGS_FILE}"
  rm -rf "${CLAUDE_SETTINGS_FILE}"
}

function cleanup() {
  info "Cleaning up temporary files and configurations"
  rm -rf "${TMP_LITELLM_VENV}"
  rm -rf "${CLAUDE_SETTINGS_FILE}"
  rm -rf "${LITELLM_CONFIG_FILE}"
}

function create_claude_config() {
  info "Creating claude settings file at ${CLAUDE_SETTINGS_FILE}"
  mkdir -p "$(dirname "${CLAUDE_SETTINGS_FILE}")"
  cat > "${CLAUDE_SETTINGS_FILE}" <<EOF
{
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-",
    "ANTHROPIC_BASE_URL": "http://localhost:4000",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "claude-opus-41",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4.5",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-sonnet-4.5",
    "ANTHROPIC_MODEL": "claude-sonnet-4.5",
    "ANTHROPIC_SMALL_FAST_MODEL": "claude-sonnet-4.5",
    "CLAUDE_CODE_SUBAGENT_MODEL": "claude-sonnet-4.5"
  }
}
EOF
}

function create_litellm_config() {
  info "Creating litellm config file at ${LITELLM_CONFIG_FILE}"
  mkdir -p "$(dirname "${LITELLM_CONFIG_FILE}")"
  cat > "${LITELLM_CONFIG_FILE}" <<EOF
general_settings:
  master_key: sk-
litellm_settings:
  disable_copilot_system_to_assistant: true
  drop_params: true
model_list:
- model_name: '*'
  litellm_params:
    model: github_copilot/*
    extra_headers:
      Editor-Version: vscode/1.372.0
EOF
}

function start() {
  source_venv
  create_claude_config
  create_litellm_config
  info "Starting litellm proxy server"
  litellm --config "${LITELLM_CONFIG_FILE}"
}

function usage() {
  echo "Usage: litellm-proxy.sh <subcommand>"
  echo ""
  echo "Subcommands:"
  echo "  start         Start the litellm proxy server"
  echo "  reset-claude  Reset the claude settings file"
  echo "  cleanup       Cleanup temporary files and configurations"
  echo ""
}

trap reset_claude EXIT SIGINT SIGTERM

case "${1:-}" in
start)
    shift
    start "$@"
    ;;
reset-claude)
    shift
    reset_claude
    ;;
cleanup)
    shift
    cleanup
    ;;
*)
    echo "Unknown subcommand: ${1:-}"
    usage
    exit 1
    ;;
esac
