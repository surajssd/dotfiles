#!/usr/bin/env bash

set -euo pipefail

readonly SESSIONS_CONFIG="${HOME}/.config/openclaw/sessions.yaml"
readonly STATE_DIR="${HOME}/.custom-openclaw-setup"
readonly CONTAINER_PREFIX="openclaw"
readonly DEFAULT_IMAGE="ghcr.io/surajssd/dotfiles/openclaw:latest"
readonly CONTAINER_GATEWAY_PORT=18789
readonly CONTAINER_BRIDGE_PORT=18790
readonly PROXY_PID_DIR="${STATE_DIR}/proxy"
readonly DEFAULT_PROXY_PORT=11080
function detect_vmnet_gateway() {
    local subnet
    subnet="$(container network ls 2>/dev/null | awk 'NR==2 {print $3}')"
    if [[ -z "${subnet}" ]]; then
        echo ""
        return
    fi
    # Convert subnet to gateway: 192.168.65.0/24 → 192.168.65.1
    # Strip the network and CIDR suffix, then append .1
    local base
    base="${subnet%.*}"
    echo "${base}.1"
}

VMNET_GATEWAY="$(detect_vmnet_gateway)"
readonly VMNET_GATEWAY

function info() {
    echo "${1}"
}

function err() {
    echo "❌ ${1}" >&2
    exit 1
}

function preflight_checks() {
    if [[ "$(uname -s)" != "Darwin" ]]; then
        err "This script is only supported on macOS (Darwin)."
    fi

    if ! command -v container &>/dev/null; then
        err "'container' CLI is not installed. Install it with: brew install container"
    fi

    if ! container system status &>/dev/null; then
        err "container system is not running. Start it with: container system start"
    fi

    if ! command -v yq &>/dev/null; then
        err "'yq' is not installed. Install it with: brew install yq"
    fi

    if [[ ! -f "${SESSIONS_CONFIG}" ]]; then
        err "Sessions config not found at ${SESSIONS_CONFIG}"
    fi
}

function validate_session_name() {
    local name="${1:-}"
    if [[ -z "${name}" ]]; then
        err "Session name is required."
    fi

    if [[ ! "${name}" =~ ^[a-zA-Z0-9][a-zA-Z0-9_-]*$ ]]; then
        err "Invalid session name '${name}'. Use only letters, digits, hyphens, and underscores."
    fi

    # Verify the session exists in the config file
    local exists
    exists="$(yq "has(\"${name}\")" "${SESSIONS_CONFIG}")"
    if [[ "${exists}" != "true" ]]; then
        err "Session '${name}' not found in ${SESSIONS_CONFIG}"
    fi
}

function container_name() {
    echo "${CONTAINER_PREFIX}-${1}"
}

function ensure_state_dirs() {
    local session_name="${1}"
    local config_dir="${STATE_DIR}/${session_name}/config"

    # Host-side config dir (bind-mounted for host access to openclaw.json)
    mkdir -p "${config_dir}/identity"
    mkdir -p "${config_dir}/agents/main/agent"
    mkdir -p "${config_dir}/agents/main/sessions"
    mkdir -p "${config_dir}/workspace"

    # Case-sensitive container volumes for home and linuxbrew
    local home_vol="${CONTAINER_PREFIX}-${session_name}-home"
    local brew_vol="${CONTAINER_PREFIX}-${session_name}-linuxbrew"
    if ! container volume ls 2>/dev/null | grep -q "${home_vol}"; then
        container volume create "${home_vol}" -s 20G >/dev/null
    fi
    if ! container volume ls 2>/dev/null | grep -q "${brew_vol}"; then
        container volume create "${brew_vol}" -s 20G >/dev/null
    fi
}

function read_session_config() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"

    image="$(yq ".${session_name}.image // \"${DEFAULT_IMAGE}\"" "${yaml}")"
    cpus="$(yq ".${session_name}.resources.cpus // 2" "${yaml}")"
    memory="$(yq ".${session_name}.resources.memory // \"2g\"" "${yaml}")"
    gateway_port="$(yq ".${session_name}.ports.gateway" "${yaml}")"
    bridge_port="$(yq ".${session_name}.ports.bridge" "${yaml}")"

    if [[ "${gateway_port}" == "null" || -z "${gateway_port}" ]]; then
        err "ports.gateway is required for session '${session_name}' in ${yaml}"
    fi
    if [[ "${bridge_port}" == "null" || -z "${bridge_port}" ]]; then
        err "ports.bridge is required for session '${session_name}' in ${yaml}"
    fi
}

function build_mount_args() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"
    local config_dir="${STATE_DIR}/${session_name}/config"
    local home_vol="${CONTAINER_PREFIX}-${session_name}-home"
    local brew_vol="${CONTAINER_PREFIX}-${session_name}-linuxbrew"

    mount_args=()

    # Case-sensitive container volumes for home and linuxbrew (ext4 in the VM).
    # The entrypoint seeds these from image seed copies on first run.
    mount_args+=("--mount" "type=volume,source=${home_vol},target=/home/node")
    mount_args+=("--mount" "type=volume,source=${brew_vol},target=/home/linuxbrew")

    # Bind-mount the config dir on top of the home volume for host-side access
    # to openclaw.json (needed by start, info, config commands).
    mount_args+=("--mount" "type=bind,source=${config_dir},target=/home/node/.openclaw")
    mount_args+=("--mount" "type=bind,source=${config_dir}/workspace,target=/home/node/.openclaw/workspace")

    # Extra mounts from YAML
    local mount_count
    mount_count="$(yq ".${session_name}.mounts | length // 0" "${yaml}")"
    for ((i = 0; i < mount_count; i++)); do
        local src tgt ro
        src="$(yq ".${session_name}.mounts[${i}].source" "${yaml}")"
        tgt="$(yq ".${session_name}.mounts[${i}].target" "${yaml}")"
        ro="$(yq ".${session_name}.mounts[${i}].readonly // false" "${yaml}")"

        if [[ "${src}" == "null" || "${tgt}" == "null" ]]; then
            err "Mount at index ${i} for session '${session_name}' is missing source or target."
        fi

        local mount_spec="type=bind,source=${src},target=${tgt}"
        if [[ "${ro}" == "true" ]]; then
            mount_spec+=",readonly"
        fi
        mount_args+=("--mount" "${mount_spec}")
    done

    # Skills directories mount (optional)
    # Each subdirectory from the listed skills folders is mounted individually
    # into the standard OpenClaw skills path so they are discovered natively
    local skills_count
    skills_count="$(yq ".${session_name}.skills | length // 0" "${yaml}")"
    if [[ "${skills_count}" -gt 0 ]]; then
        for ((i = 0; i < skills_count; i++)); do
            local skills_dir
            skills_dir="$(yq ".${session_name}.skills[${i}]" "${yaml}")"
            if [[ -z "${skills_dir}" || "${skills_dir}" == "null" ]]; then
                continue
            fi
            if [[ ! -d "${skills_dir}" ]]; then
                err "Skills directory '${skills_dir}' does not exist."
            fi
            for subdir in "${skills_dir}"/*/; do
                [[ -d "${subdir}" ]] || continue
                local skill_name
                skill_name="$(basename "${subdir}")"
                mount_args+=("--mount" "type=bind,source=${subdir},target=/home/node/.openclaw/workspace/skills/${skill_name},readonly")
            done
        done
    fi
}

function build_env_args() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"

    env_args=()
    env_args+=("-e" "HOME=/home/node")
    env_args+=("-e" "TERM=xterm-256color")
    env_args+=("-e" "NODE_OPTIONS=--max-old-space-size=3072")

    # Default TZ from system if not overridden
    local system_tz
    system_tz="$(readlink /etc/localtime | sed 's|.*/zoneinfo/||')"
    env_args+=("-e" "TZ=${system_tz}")

    # Add session-specific env vars from YAML (these can override defaults like TZ)
    local env_count
    env_count="$(yq ".${session_name}.env | length // 0" "${yaml}")"
    if [[ "${env_count}" -gt 0 ]]; then
        while IFS= read -r env_line; do
            env_args+=("-e" "${env_line}")
        done < <(yq ".${session_name}.env | to_entries | .[] | .key + \"=\" + (.value | tostring)" "${yaml}")
    fi
}

# ---------------------------------------------------------------------------
# HTTP proxy management
#
# macOS Network Extensions only intercept traffic from host user-space apps.
# Container traffic via vmnet NAT is not intercepted, causing timeouts to
# certain endpoints. Running an HTTP proxy on the host lets container
# traffic originate from host user-space where it is properly handled.
# ---------------------------------------------------------------------------

function proxy_enabled() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"
    local enabled
    enabled="$(yq ".${session_name}.proxy.enabled // false" "${yaml}")"
    [[ "${enabled}" == "true" ]]
}

function proxy_port() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"
    local port
    port="$(yq ".${session_name}.proxy.port // ${DEFAULT_PROXY_PORT}" "${yaml}")"
    echo "${port}"
}

function proxy_config_file() {
    local session_name="${1}"
    echo "${PROXY_PID_DIR}/${session_name}.conf"
}

function start_proxy() {
    local session_name="${1}"

    if ! proxy_enabled "${session_name}"; then
        return
    fi

    if ! command -v tinyproxy &>/dev/null; then
        info "⚠️  'tinyproxy' not installed — proxy support disabled. Install with: brew install tinyproxy"
        return
    fi

    if [[ -z "${VMNET_GATEWAY}" ]]; then
        info "⚠️  Could not detect vmnet gateway IP — proxy support disabled. Ensure a bridge interface exists."
        return
    fi

    local port
    port="$(proxy_port "${session_name}")"
    local pid_file="${PROXY_PID_DIR}/${session_name}.pid"
    local conf_file
    conf_file="$(proxy_config_file "${session_name}")"

    mkdir -p "${PROXY_PID_DIR}"

    # Check if proxy is already running
    if [[ -f "${pid_file}" ]]; then
        local existing_pid
        existing_pid="$(cat "${pid_file}")"
        if kill -0 "${existing_pid}" 2>/dev/null; then
            info "🔌 HTTP proxy already running (PID ${existing_pid}) on ${VMNET_GATEWAY}:${port}"
            return
        fi
        # Stale PID file — clean up
        rm -f "${pid_file}"
    fi

    # Derive the /24 subnet from the gateway IP (e.g. 192.168.65.1 → 192.168.65.0/24)
    local subnet
    subnet="$(echo "${VMNET_GATEWAY}" | sed 's/\.[0-9]*$/.0\/24/')"

    # Generate a minimal tinyproxy config
    cat >"${conf_file}" <<EOF
Port ${port}
Listen ${VMNET_GATEWAY}
Timeout 600
LogLevel Critical
MaxClients 100
DisableViaHeader Yes
Allow ${subnet}
EOF

    info "⏳ Starting HTTP proxy on ${VMNET_GATEWAY}:${port}..."
    tinyproxy -d -c "${conf_file}" &
    local proxy_pid=$!

    # Give it a moment to bind
    sleep 1
    if ! kill -0 "${proxy_pid}" 2>/dev/null; then
        info "❌ Failed to start HTTP proxy on ${VMNET_GATEWAY}:${port}"
        return
    fi

    echo "${proxy_pid}" >"${pid_file}"
    info "✅ HTTP proxy running (PID ${proxy_pid}) on ${VMNET_GATEWAY}:${port}"
}

function stop_proxy() {
    local session_name="${1}"
    local pid_file="${PROXY_PID_DIR}/${session_name}.pid"

    if [[ ! -f "${pid_file}" ]]; then
        return
    fi

    local proxy_pid
    proxy_pid="$(cat "${pid_file}")"
    if kill -0 "${proxy_pid}" 2>/dev/null; then
        kill "${proxy_pid}" 2>/dev/null || true
        info "✅ HTTP proxy stopped (PID ${proxy_pid})"
    fi
    rm -f "${pid_file}"
}

function inject_proxy_env() {
    local session_name="${1}"
    local cname="${2}"

    if ! proxy_enabled "${session_name}"; then
        return
    fi

    if ! command -v tinyproxy &>/dev/null || [[ -z "${VMNET_GATEWAY}" ]]; then
        return
    fi

    local port
    port="$(proxy_port "${session_name}")"
    local proxy_url="http://${VMNET_GATEWAY}:${port}"

    # Derive the /24 subnet from the gateway IP
    local subnet
    subnet="$(echo "${VMNET_GATEWAY}" | sed 's/\.[0-9]*$/.0\/24/')"

    # Write proxy environment file inside the container
    container exec "${cname}" bash -c "cat > /home/node/.proxy_env << PROXYEOF
export http_proxy=${proxy_url}
export HTTP_PROXY=${proxy_url}
export https_proxy=${proxy_url}
export HTTPS_PROXY=${proxy_url}
export no_proxy=localhost,127.0.0.1,${subnet}
export NO_PROXY=localhost,127.0.0.1,${subnet}
PROXYEOF" 2>/dev/null || true

    # Install into /etc/profile.d (login shells), /home/node/.profile (user login),
    # and /etc/bash.bashrc (interactive non-login shells).
    # Note: ~/.bashrc is often a read-only symlink on container volumes.
    container exec "${cname}" bash -c '
        sudo cp /home/node/.proxy_env /etc/profile.d/proxy.sh 2>/dev/null || true
        if ! grep -q "\.proxy_env" /home/node/.profile 2>/dev/null; then
            echo "" >> /home/node/.profile
            echo "# HTTP proxy" >> /home/node/.profile
            echo "[ -f ~/.proxy_env ] && source ~/.proxy_env" >> /home/node/.profile
        fi
        if ! grep -q "\.proxy_env" /etc/bash.bashrc 2>/dev/null; then
            echo "" | sudo tee -a /etc/bash.bashrc > /dev/null
            echo "# HTTP proxy" | sudo tee -a /etc/bash.bashrc > /dev/null
            echo "[ -f /home/node/.proxy_env ] && . /home/node/.proxy_env" | sudo tee -a /etc/bash.bashrc > /dev/null
        fi
    ' 2>/dev/null || true

    info "🔌 Proxy env injected: ${proxy_url}"
}

function cmd_proxy_status() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    if ! proxy_enabled "${session_name}"; then
        info "🔌 Proxy is not enabled for session '${session_name}'"
        info "   Add 'proxy: { enabled: true }' to ${SESSIONS_CONFIG}"
        return
    fi

    local port
    port="$(proxy_port "${session_name}")"
    local pid_file="${PROXY_PID_DIR}/${session_name}.pid"

    echo ""
    echo "  Session:  ${session_name}"
    echo "  Bind:     ${VMNET_GATEWAY}:${port}"

    if [[ -f "${pid_file}" ]]; then
        local proxy_pid
        proxy_pid="$(cat "${pid_file}")"
        if kill -0 "${proxy_pid}" 2>/dev/null; then
            echo "  Status:   🟢 Running (PID ${proxy_pid})"
        else
            echo "  Status:   🔴 Dead (stale PID ${proxy_pid})"
        fi
    else
        echo "  Status:   ⚪ Not started"
    fi
    echo ""
}

function cmd_setup() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"
    ensure_state_dirs "${session_name}"

    local image cpus memory gateway_port bridge_port
    read_session_config "${session_name}"

    local mount_args=()
    build_mount_args "${session_name}"

    local env_args=()
    build_env_args "${session_name}"

    local host_uid host_gid
    host_uid="$(id -u)"
    host_gid="$(id -g)"

    info "🔧 Running onboarding for session '${session_name}'..."
    container run \
        --rm -it \
        --cpus "${cpus}" --memory 4g \
        --uid "${host_uid}" --gid "${host_gid}" \
        "${mount_args[@]}" \
        "${env_args[@]}" \
        "${image}" \
        openclaw onboard --mode local --no-install-daemon

    info "⚙️  Setting gateway mode to local..."
    container run \
        --rm \
        --cpus "${cpus}" --memory 4g \
        --uid "${host_uid}" --gid "${host_gid}" \
        "${mount_args[@]}" \
        "${env_args[@]}" \
        "${image}" \
        openclaw config set gateway.mode local

    info "⚙️  Allowing Control UI access..."
    container run \
        --rm \
        --cpus "${cpus}" --memory 4g \
        --uid "${host_uid}" --gid "${host_gid}" \
        "${mount_args[@]}" \
        "${env_args[@]}" \
        "${image}" \
        openclaw config set gateway.controlUi.dangerouslyAllowHostHeaderOriginFallback true

    info "✅ Setup complete for session '${session_name}'."
    echo ""
    cmd_start "${session_name}"
}

function wait_for_healthy() {
    local session_name="${1}"
    local cname="${2}"
    local gateway_port="${3}"

    local health_url="http://127.0.0.1:${gateway_port}/healthz"
    local max_attempts=30
    local attempt=0
    printf "⏳ Waiting for gateway to become healthy "
    while [[ "${attempt}" -lt "${max_attempts}" ]]; do
        if curl -sf "${health_url}" >/dev/null 2>&1; then
            echo ""
            info "✅ Container '${cname}' is running and healthy."
            return
        fi
        printf "."
        sleep 2
        attempt=$((attempt + 1))
    done

    echo ""
    echo "⚠️  Gateway did not become healthy within $((max_attempts * 2))s."
    info "📋 Check logs for errors:"
    echo "  openclaw.sh logs ${session_name}"
}

function cmd_start() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"
    ensure_state_dirs "${session_name}"

    # Verify that setup has been completed (onboarding creates openclaw.json)
    local config_json="${STATE_DIR}/${session_name}/config/openclaw.json"
    if [[ ! -f "${config_json}" ]]; then
        echo "❌ Session '${session_name}' has not been set up yet. Run setup first:" >&2
        echo "  openclaw.sh setup ${session_name}" >&2
        exit 1
    fi

    local cname
    cname="$(container_name "${session_name}")"

    local image cpus memory gateway_port bridge_port
    read_session_config "${session_name}"

    # Reuse existing container to preserve state/progress.
    if container ls 2>/dev/null | grep -q "${cname}"; then
        info "🟢 Container '${cname}' is already running."
        start_proxy "${session_name}"
        inject_proxy_env "${session_name}" "${cname}"
        print_connection_info "${session_name}"
        return
    elif container ls -a 2>/dev/null | grep -q "${cname}"; then
        info "♻️  Restarting existing container '${cname}'"
        container start "${cname}"
        wait_for_healthy "${session_name}" "${cname}" "${gateway_port}"
        start_proxy "${session_name}"
        inject_proxy_env "${session_name}" "${cname}"
        print_connection_info "${session_name}"
        return
    fi

    local mount_args=()
    build_mount_args "${session_name}"

    local env_args=()
    build_env_args "${session_name}"

    # Map container user to host UID/GID so bind-mounted files have correct ownership
    local host_uid host_gid
    host_uid="$(id -u)"
    host_gid="$(id -g)"

    info "🚀 Starting OpenClaw session '${session_name}' as container '${cname}'"
    container run \
        -d \
        --cpus "${cpus}" --memory "${memory}" \
        --uid "${host_uid}" --gid "${host_gid}" \
        -p "${gateway_port}:${CONTAINER_GATEWAY_PORT}" \
        -p "${bridge_port}:${CONTAINER_BRIDGE_PORT}" \
        "${mount_args[@]}" \
        "${env_args[@]}" \
        --name "${cname}" \
        "${image}" \
        openclaw gateway --bind lan --port "${CONTAINER_GATEWAY_PORT}"

    wait_for_healthy "${session_name}" "${cname}" "${gateway_port}"
    start_proxy "${session_name}"
    inject_proxy_env "${session_name}" "${cname}"
    print_connection_info "${session_name}"
}

function print_connection_info() {
    local session_name="${1}"
    local yaml="${SESSIONS_CONFIG}"
    local gateway_port
    gateway_port="$(yq ".${session_name}.ports.gateway" "${yaml}")"

    echo ""

    # Read gateway token from config
    local config_json="${STATE_DIR}/${session_name}/config/openclaw.json"
    local token=""
    if [[ -f "${config_json}" ]] && command -v jq &>/dev/null; then
        token="$(jq -r '.gateway.auth.token // empty' "${config_json}" 2>/dev/null || true)"
    fi

    if [[ -n "${token}" ]]; then
        echo "  🌐 Dashboard: http://localhost:${gateway_port}/#token=${token}"
    else
        echo "  🌐 Dashboard: http://127.0.0.1:${gateway_port}/"
    fi
    echo "  💚 Health:    http://127.0.0.1:${gateway_port}/healthz"
    echo ""
    info "📋 To view logs:"
    echo "  openclaw.sh logs ${session_name}"
    echo ""
    info "📱 To approve a device:"
    echo "  openclaw.sh exec ${session_name} openclaw devices approve"
}

function cmd_info() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    local cname
    cname="$(container_name "${session_name}")"

    if ! container ls 2>/dev/null | grep -q "${cname}"; then
        err "Session '${session_name}' is not running. Start it with: openclaw.sh start ${session_name}"
    fi

    print_connection_info "${session_name}"
}

function cmd_stop() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    local cname
    cname="$(container_name "${session_name}")"

    stop_proxy "${session_name}"

    if container ls -a 2>/dev/null | grep -q "${cname}"; then
        info "🛑 Stopping container '${cname}'"
        container stop "${cname}" 2>/dev/null || true
        info "✅ Container '${cname}' stopped"
    else
        info "⚠️  Container '${cname}' is not running"
    fi
}

function cmd_config() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    local config_path="${STATE_DIR}/${session_name}/config/openclaw.json"
    if [[ -f "${config_path}" ]]; then
        echo "${config_path}"
    else
        err "Config file not found: ${config_path}. Run 'openclaw.sh setup ${session_name}' first"
    fi
}

function cmd_restart() {
    local session_name="${1:-}"
    info "🔄 Restarting session '${session_name}'"
    cmd_stop "${session_name}"
    cmd_start "${session_name}"
}

function cmd_remove() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    local cname
    cname="$(container_name "${session_name}")"

    stop_proxy "${session_name}"

    if container ls -a 2>/dev/null | grep -q "${cname}"; then
        info "🗑️  Stopping and removing container '${cname}'"
        container stop "${cname}" 2>/dev/null || true
        container rm "${cname}"
        info "✅ Container '${cname}' removed"
        info "💾 State preserved in ${STATE_DIR}/${session_name}/"
    else
        info "⚠️  Container '${cname}' does not exist"
    fi
}

function cmd_logs() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"

    local cname
    cname="$(container_name "${session_name}")"
    container logs -f "${cname}"
}

function cmd_exec() {
    local session_name="${1:-}"
    preflight_checks
    validate_session_name "${session_name}"
    shift

    local cname
    cname="$(container_name "${session_name}")"

    # Default to login bash shell if no command specified
    if [[ $# -eq 0 ]]; then
        set -- bash -l
    fi

    # If proxy is enabled and a non-bash command is being run, wrap it so that
    # the proxy env vars (ALL_PROXY, HTTP_PROXY, etc.) are available.
    if proxy_enabled "${session_name}" && [[ "${1}" != "bash" ]]; then
        container exec -it "${cname}" bash -lc "$*"
    else
        container exec -it "${cname}" "$@"
    fi
}

function cmd_status() {
    local session_name="${1:-}"
    preflight_checks

    if [[ -n "${session_name}" ]]; then
        validate_session_name "${session_name}"
        local cname
        cname="$(container_name "${session_name}")"

        if container ls 2>/dev/null | grep -q "${cname}"; then
            info "🟢 Session '${session_name}' is running"
            container ls 2>/dev/null | head -1
            container ls 2>/dev/null | grep "${cname}"
        elif container ls -a 2>/dev/null | grep -q "${cname}"; then
            info "🔴 Session '${session_name}' exists but is stopped"
        else
            info "⚪ Session '${session_name}' has no container (not started yet)"
        fi
    else
        info "📦 All OpenClaw containers:"
        container ls -a 2>/dev/null | head -1
        container ls -a 2>/dev/null | grep "${CONTAINER_PREFIX}-" || echo "  (none)"
    fi
}

function cmd_list() {
    preflight_checks

    local sessions
    sessions="$(yq 'keys | .[]' "${SESSIONS_CONFIG}")"

    if [[ -z "${sessions}" ]]; then
        echo "No sessions defined."
        return
    fi

    # Collect rows first to compute column widths
    local -a names=() ports=() statuses=()
    while IFS= read -r session_name; do
        local cname gateway_port
        cname="$(container_name "${session_name}")"
        gateway_port="$(yq ".${session_name}.ports.gateway // \"?\"" "${SESSIONS_CONFIG}")"

        local status_str="NotCreated"
        if container ls 2>/dev/null | grep -q "${cname}"; then
            status_str="Running"
        elif container ls -a 2>/dev/null | grep -q "${cname}"; then
            status_str="Stopped"
        fi

        names+=("${session_name}")
        ports+=("${gateway_port}")
        statuses+=("${status_str}")
    done <<<"${sessions}"

    # Compute column widths (minimum = header length)
    local name_w=4 port_w=4 status_w=6
    for ((i = 0; i < ${#names[@]}; i++)); do
        ((${#names[i]} > name_w)) && name_w=${#names[i]}
        ((${#ports[i]} > port_w)) && port_w=${#ports[i]}
        ((${#statuses[i]} > status_w)) && status_w=${#statuses[i]}
    done

    # Print header and rows
    printf "%-${name_w}s   %-${port_w}s   %-${status_w}s\n" "NAME" "PORT" "STATUS"
    for ((i = 0; i < ${#names[@]}; i++)); do
        printf "%-${name_w}s   %-${port_w}s   %-${status_w}s\n" "${names[i]}" "${ports[i]}" "${statuses[i]}"
    done
}

function usage() {
    echo "🐾 openclaw.sh — Manage OpenClaw gateway containers"
    echo ""
    echo "Usage: openclaw.sh <subcommand> [session-name]"
    echo ""
    echo "Sessions are defined in: ${SESSIONS_CONFIG}"
    echo "Persistent state is stored in: ${STATE_DIR}/<session>/"
    echo ""
    echo "Subcommands:"
    echo "  🔧 setup        <name>        Run initial onboarding for a new session"
    echo "  🚀 start        <name>        Start an OpenClaw session container"
    echo "  🛑 stop         <name>        Stop an OpenClaw session container"
    echo "  🔄 restart      <name>        Restart an OpenClaw session container"
    echo "  🗑️ remove       <name>        Stop and remove an OpenClaw session container"
    echo "  📋 logs         <name>        Follow logs of an OpenClaw session container"
    echo "  🐚 exec         <name> [cmd]  Exec into a running OpenClaw session container"
    echo "  ⚙️ config       <name>        Show config file path for a session"
    echo "  🔗 info         <name>        Show connection info for a session"
    echo "  🔍 status       [name]        Show status of one or all OpenClaw containers"
    echo "  🔌 proxy-status <name>        Show HTTP proxy status for a session"
    echo "  📝 list                       List all defined sessions and their status"
    echo ""
}

case "${1:-}" in
setup)
    shift
    cmd_setup "$@"
    ;;
start)
    shift
    cmd_start "$@"
    ;;
stop)
    shift
    cmd_stop "$@"
    ;;
restart)
    shift
    cmd_restart "$@"
    ;;
remove)
    shift
    cmd_remove "$@"
    ;;
logs)
    shift
    cmd_logs "$@"
    ;;
exec)
    shift
    cmd_exec "$@"
    ;;
info)
    shift
    cmd_info "$@"
    ;;
config)
    shift
    cmd_config "$@"
    ;;
status)
    shift
    cmd_status "${1:-}"
    ;;
proxy-status)
    shift
    cmd_proxy_status "$@"
    ;;
list)
    shift
    cmd_list
    ;;
help | --help | -h)
    usage
    ;;
*)
    echo "Unknown subcommand: ${1:-}"
    usage
    exit 1
    ;;
esac
