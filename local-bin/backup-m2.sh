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
    --exclude ".cargo" \
    --exclude ".azure" \
    --exclude ".docker" \
    --exclude ".rustup" \
    --exclude "Library" \
    --exclude "go" \
    --exclude ".kube" \
    --exclude ".vscode*" \
    --exclude ".zsh*" \
    --exclude ".oh-my-zsh" \
    --exclude ".net" \
    --exclude ".terraform.*" \
    --exclude "Applications" \
    --exclude "Pictures/Photos Library.photoslibrary" \
    --exclude "Pictures/Photo Booth Library" \
    --exclude "Desktop" \
    --exclude ".Trash" \
    --exclude ".gnupg" \
    ~/ ./

popd
