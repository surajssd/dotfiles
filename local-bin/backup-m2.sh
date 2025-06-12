#!/usr/bin/env bash
#

set -euo pipefail

# Check if this is a OSX machine and if not exit
OPERATING_SYSTEM=$(uname)
if [ "$OPERATING_SYSTEM" != "Darwin" ]; then
    echo "This script is only on OSX"
    exit 1
fi

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

HDD_NAME="${HDD_NAME:-/Volumes/mac-backup}"
DIR_NAME="${DIR_NAME:-m2-$(ssd date)}"
BACKUP_DIR_NAME="${HDD_NAME}/${DIR_NAME}"

mkdir -p "${BACKUP_DIR_NAME}"
pushd "${BACKUP_DIR_NAME}"

echo "Backing up to ${BACKUP_DIR_NAME} ..."

echo "Total backup size close to: $(du -sh ~ 2>/dev/null)"

while true; do
    echo "Size: $(du -sh ${BACKUP_DIR_NAME})"
    sleep 5
done &

rsync \
    -aq \
    --backup \
    --progress --stats \
    --exclude ".azure" \
    --exclude ".bash-git-prompt" \
    --exclude ".cache" \
    --exclude ".cargo" \
    --exclude ".docker" \
    --exclude ".gnupg" \
    --exclude ".kube" \
    --exclude ".local" \
    --exclude ".minikube" \
    --exclude ".net" \
    --exclude ".npm" \
    --exclude ".nvm" \
    --exclude ".oh-my-zsh" \
    --exclude ".ollama" \
    --exclude ".rustup" \
    --exclude ".terraform.*" \
    --exclude ".Trash" \
    --exclude ".vscode*" \
    --exclude ".zsh*" \
    --exclude "Applications" \
    --exclude "Desktop" \
    --exclude "go" \
    --exclude "Library" \
    --exclude "Pictures/Photos Library.photoslibrary" \
    --exclude "Pictures/Photo Booth Library" \
    ~/ ./

popd
