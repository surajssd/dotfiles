#!/usr/bin/env bash
#
# Shared helpers for installer scripts. Sourced (not executed); the caller
# provides `set -euo pipefail`.
#
# Two groups of helpers live here:
#   1. Symlink helpers (link_tree, prune_dead_symlinks) - used by install-*.sh.
#   2. Vendoring helpers (die, clone cache, inject_attribution) - used by
#      fetch-external-*.sh scripts. A fetch script enables clone cleanup with
#      `trap clone_cache_cleanup EXIT`; installer scripts that only need the
#      symlink helpers are unaffected (no trap is set here).

die() {
    echo "❌ $*" >&2
    exit 1
}

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

# --- Vendoring helpers (used by fetch-external-*.sh) ---

# Cached clones, tracked as parallel indexed arrays (bash 3.2 has no
# associative arrays). CLONE_URLS[i] was cloned into CLONE_DIRS[i].
CLONE_URLS=()
CLONE_DIRS=()
TMP_DIRS=()

# Remove every cached clone on exit. Fetch scripts opt in with
# `trap clone_cache_cleanup EXIT`.
clone_cache_cleanup() {
    local d
    for d in "${TMP_DIRS[@]:-}"; do
        [[ -n "$d" ]] && rm -rf "$d"
    done
}

# Echo the cached clone dir for a URL, or return non-zero if not cloned yet.
# Parallel indexed arrays (not an associative array) keep this bash 3.2
# compatible, since macOS still ships bash 3.2 as /bin/bash.
clone_dir_for() {
    local url="$1" i
    ((${#CLONE_URLS[@]})) || return 1
    for i in "${!CLONE_URLS[@]}"; do
        if [[ "${CLONE_URLS[$i]}" == "$url" ]]; then
            printf '%s\n' "${CLONE_DIRS[$i]}"
            return 0
        fi
    done
    return 1
}

# Clone a repo once (cached by URL).
ensure_clone() {
    local url="$1" dir
    clone_dir_for "$url" >/dev/null && return 0
    dir="$(mktemp -d)"
    TMP_DIRS+=("$dir")
    echo "⏳ Cloning ${url} ..."
    git clone --depth 1 "$url" "$dir" >/dev/null 2>&1 || die "failed to clone ${url}"
    CLONE_URLS+=("$url")
    CLONE_DIRS+=("$dir")
}

# inject_attribution <file> <author> [license]
#   Merge license + author attribution into a markdown file's frontmatter.
#   Idempotent and merge-aware (safe for upstreams that already carry some of
#   these keys, e.g. blader/humanizer ships license: MIT and metadata.version).
#   - license="" omits the license line entirely (for unlicensed upstreams
#     like abatilo/vimrc); otherwise license defaults to MIT.
#   - If frontmatter exists: inject `license: <value>` (when absent & non-empty)
#     and `metadata.author` (when absent) before the closing ---.
#   - If no frontmatter: prepend a fresh block carrying the same fields.
#   Fresh write each run => no duplicates. Bash 3.2 compatible (no mapfile).
inject_attribution() {
    local file="$1" author="$2" license="${3-MIT}"
    local has_license=0 has_metadata=0 has_author=0 in_metadata=0 delim=0
    local -a out=()
    while IFS= read -r line; do
        if [[ "$line" =~ ^---[[:space:]]*$ ]]; then
            ((++delim))
            in_metadata=0
        fi
        if ((delim == 1)); then
            [[ "$line" =~ ^license: ]] && has_license=1
            if [[ "$line" =~ ^metadata: ]]; then
                has_metadata=1
                in_metadata=1
            fi
            if ((in_metadata)) && [[ "$line" =~ ^[[:space:]]+author: ]]; then
                has_author=1
            fi
        fi
        out+=("$line")
    done <"$file"

    : >"${file}.new"

    # No frontmatter block (fewer than 2 delimiters): prepend a fresh one.
    if ((delim < 2)); then
        printf '%s\n' '---' >>"${file}.new"
        [[ -n "$license" ]] && printf 'license: %s\n' "$license" >>"${file}.new"
        printf 'metadata:\n  author: %s\n' "$author" >>"${file}.new"
        printf '%s\n' '---' >>"${file}.new"
    fi

    delim=0
    for line in "${out[@]}"; do
        if [[ "$line" =~ ^---[[:space:]]*$ ]]; then
            ((++delim))
            if ((delim == 2)); then
                ((has_license)) || [[ -z "$license" ]] || printf 'license: %s\n' "$license" >>"${file}.new"
                if ((has_metadata)); then
                    ((has_author)) || printf '  author: %s\n' "$author" >>"${file}.new"
                else
                    printf 'metadata:\n  author: %s\n' "$author" >>"${file}.new"
                fi
            fi
        fi
        printf '%s\n' "$line" >>"${file}.new"
    done
    mv "${file}.new" "$file"
}
