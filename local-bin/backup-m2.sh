#!/usr/bin/env bash
#

set -euo pipefail

trap "trap - SIGTERM && kill -- -$$" SIGINT SIGTERM EXIT

hdd="/Volumes/backup-osx"
dir="m2-$(date.sh)"
backup_dir="${hdd}/${dir}"

mkdir -p "${backup_dir}"
pushd "${backup_dir}"

echo "Backing up to ${backup_dir} ..."

while true; do
    echo "Size: $(du -sh ${backup_dir})"
    sleep 1
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
    ~/ ./

popd
