#!/usr/bin/env bash
#
# Shared helpers for installer scripts. Sourced (not executed); the caller
# provides `set -euo pipefail`.

# link_tree <kind> <src_dir> <dest_dir> [skip_basename...]
#   kind=file -> symlink each entry with `ln -sf`   (glob: *)
#   kind=dir  -> symlink each subdir with `ln -sfn` (glob: */)
# Missing src_dir is a silent no-op (handles the optional private case).
link_tree() {
    local kind="$1" src="$2" dest="$3"
    shift 3
    local skip=("$@")

    [[ -d "$src" ]] || return 0
    mkdir -p "$dest"

    local glob entry base s skipit
    local ln_flags
    if [[ "$kind" == "dir" ]]; then
        ln_flags=(-sfn)
        glob="*/"
    else
        ln_flags=(-sf)
        glob="*"
    fi

    local had_nullglob=0
    shopt -q nullglob && had_nullglob=1
    shopt -s nullglob
    for entry in "$src"/$glob; do
        base="$(basename "$entry")"
        skipit=false
        if ((${#skip[@]})); then
            for s in "${skip[@]}"; do
                [[ "$base" == "$s" ]] && {
                    skipit=true
                    break
                }
            done
        fi
        [[ "$skipit" == true ]] && continue
        ln "${ln_flags[@]}" "$entry" "$dest/$base"
    done
    ((had_nullglob)) || shopt -u nullglob
}

# prune_dead_symlinks <dir>: delete broken symlinks directly under <dir>.
prune_dead_symlinks() {
    local dir="$1"
    [[ -d "$dir" ]] || return 0
    find "$dir" -maxdepth 1 -type l ! -exec test -e {} \; -delete
}
