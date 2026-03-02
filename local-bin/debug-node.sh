#!/usr/bin/env bash

set -euo pipefail

# Parse arguments: support --env KEY=VALUE flags before the positional node name.
ENV_VARS=()
NODE_NAME=""

while [[ $# -gt 0 ]]; do
    case "${1}" in
    --env)
        if [[ $# -lt 2 ]]; then
            echo "❌ --env requires a KEY=VALUE argument"
            exit 1
        fi
        ENV_VARS+=("${2}")
        shift 2
        ;;
    --env=*)
        ENV_VARS+=("${1#--env=}")
        shift
        ;;
    -h | --help)
        echo "📖 Usage:"
        echo "  debug-node.sh [--env KEY=VALUE ...] <node-name>"
        exit 0
        ;;
    -*)
        echo "❌ Unknown flag: ${1}"
        echo "📖 Usage:"
        echo "  debug-node.sh [--env KEY=VALUE ...] <node-name>"
        exit 1
        ;;
    *)
        if [[ -n "${NODE_NAME}" ]]; then
            echo "❌ Unexpected argument: ${1}"
            echo "📖 Usage:"
            echo "  debug-node.sh [--env KEY=VALUE ...] <node-name>"
            exit 1
        fi
        NODE_NAME="${1}"
        shift
        ;;
    esac
done

if [[ -z "${NODE_NAME}" ]]; then
    echo "📖 Usage:"
    echo "  debug-node.sh [--env KEY=VALUE ...] <node-name>"
    exit 1
fi

# This is simplest way to get a shell on a node without needing to worry about
# the pod name length limit of kubectl debug. It creates a debug pod with the
# same settings as "kubectl debug node/<name> --profile=sysadmin" but with a
# custom name, waits for it to be ready, attaches to it, then cleans up after.
#
# kubectl debug "node/${1}" --image=quay.io/surajd/ubuntu:latest --profile=sysadmin -it -- chroot /host /bin/bash

# kubectl debug auto-generates a pod name like "node-debugger-<nodename>-<suffix>"
# which exceeds the 63-char DNS label limit for long node names, and there's no
# flag to override it. Instead, create the debug pod manually with the same
# settings as "kubectl debug node/<name> --profile=sysadmin".
POD_NAME="node-debugger-${NODE_NAME:0:43}-$(head -c 3 /dev/urandom | xxd -p | head -c 5)"

POD_MANIFEST="$(mktemp)"
trap 'rm -f "${POD_MANIFEST}"' EXIT

cat >"${POD_MANIFEST}" <<EOF
apiVersion: v1
kind: Pod
metadata:
  name: ${POD_NAME}
spec:
  nodeName: ${NODE_NAME}
  hostPID: true
  hostNetwork: true
  restartPolicy: Never
  containers:
  - name: debugger
    image: mcr.microsoft.com/mirror/docker/library/ubuntu:24.04
    stdin: true
    tty: true
    command: ["chroot", "/host", "/bin/bash"]$(
    if [[ ${#ENV_VARS[@]} -gt 0 ]]; then
        printf '\n    env:'
        for env_var in "${ENV_VARS[@]}"; do
            env_key="${env_var%%=*}"
            env_val="${env_var#*=}"
            printf '\n    - name: %s\n      value: "%s"' "${env_key}" "${env_val}"
        done
    fi
)
    securityContext:
      privileged: true
    volumeMounts:
    - name: host-root
      mountPath: /host
  volumes:
    - name: host-root
      hostPath:
        path: /
EOF

# Create the pod, poll until ready, attach, then clean up.
kubectl apply -f "${POD_MANIFEST}"

echo "⏳ Waiting for pod/${POD_NAME} to be ready..."
while true; do
    PHASE="$(kubectl get pod "${POD_NAME}" -o jsonpath='{.status.phase}' 2>/dev/null || echo "Unknown")"
    CONDITIONS="$(kubectl get pod "${POD_NAME}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null || echo "Unknown")"

    if [[ "${CONDITIONS}" == "True" ]]; then
        echo "✅ Pod is Ready (phase: ${PHASE}). Attaching..."
        break
    fi

    # Check for terminal failure states.
    if [[ "${PHASE}" == "Failed" || "${PHASE}" == "Succeeded" ]]; then
        echo "❌ ERROR: Pod entered terminal phase '${PHASE}' without becoming Ready."
        kubectl logs "${POD_NAME}" --tail=20 2>/dev/null || true
        kubectl delete pod "${POD_NAME}" --wait=false 2>/dev/null || true
        exit 1
    fi

    echo "🔄 Status: phase=${PHASE}, ready=${CONDITIONS}"
    sleep 5
done

kubectl attach -it "${POD_NAME}" -c debugger
kubectl delete pod "${POD_NAME}" --wait=false
