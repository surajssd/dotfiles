#!/usr/bin/env bash

set -euo pipefail

# If user doesn't provide the node name as an argument then exit
if [ $# -ne 1 ]; then
    echo "📖 Usage:"
    echo "  debug-node.sh <node-name>"
    exit 1
fi

# This is simplest way to get a shell on a node without needing to worry about
# the pod name length limit of kubectl debug. It creates a debug pod with the
# same settings as "kubectl debug node/<name> --profile=sysadmin" but with a
# custom name, waits for it to be ready, attaches to it, then cleans up after.
#
# kubectl debug "node/${1}" --image=quay.io/surajd/ubuntu:latest --profile=sysadmin -it -- chroot /host /bin/bash
NODE_NAME="${1}"

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
    command: ["chroot", "/host", "/bin/bash"]
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
