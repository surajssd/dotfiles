#!/usr/bin/env bash
#
# Vendor selected external skills into this repo's flat skills/ directory.
# Multi-source: each registry entry names its upstream repo + author + mode.
#   fetch    - clone upstream, copy the skill dir flat into skills/<name>,
#              excluding repo infrastructure (.git, .github, .claude-plugin),
#              and merge license + author attribution into its SKILL.md.
#   preserve - already vendored & locally customised; only verify + report,
#              never overwrite (protects local edits).
# Fetched skills are committed; re-run to update and record the printed
# upstream SHA in the commit message. install-skills.sh symlinks them as usual.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
SKILLS_DIR="${REPO_DIR}/skills"

# Registry: mode|repo_url|author|upstream_subpath|dest_name
# - upstream_subpath is the path within the upstream repo (used by fetch).
# - dest_name is the flattened directory under skills/.
SKILLS=(
    "fetch|https://github.com/mattpocock/skills|github.com/mattpocock|skills/productivity/grilling|grilling"
    "fetch|https://github.com/mattpocock/skills|github.com/mattpocock|skills/engineering/domain-modeling|domain-modeling"
    "fetch|https://github.com/mattpocock/skills|github.com/mattpocock|skills/engineering/grill-with-docs|grill-with-docs"
    "fetch|https://github.com/blader/humanizer|github.com/blader|.|humanizer"
    "preserve|https://github.com/bastos/skills|github.com/bastos|conventional-commits|conventional-commits"
)

die() {
    echo "❌ $*" >&2
    exit 1
}

# Cached clones, tracked as parallel indexed arrays (bash 3.2 has no
# associative arrays). CLONE_URLS[i] was cloned into CLONE_DIRS[i].
CLONE_URLS=()
CLONE_DIRS=()
TMP_DIRS=()
cleanup() {
    local d
    for d in "${TMP_DIRS[@]:-}"; do
        [[ -n "$d" ]] && rm -rf "$d"
    done
}
trap cleanup EXIT

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

# Merge license + author attribution into a SKILL.md frontmatter block.
# Idempotent and merge-aware (safe for upstreams that already carry some of
# these keys, e.g. blader/humanizer ships license: MIT and metadata.version).
# - If license: is absent, add `license: MIT` (default) before the closing ---.
# - If a metadata: block exists, inject `  author: <author>` as its first
#   child unless an author: key is already present.
# - If no metadata: block exists, create one with `  author: <author>`.
# Fresh copy each run => no duplicates. Works on bash 3.2 (no mapfile).
inject_attribution() {
    local file="$1" author="$2" license="${3:-MIT}"
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
    delim=0
    for line in "${out[@]}"; do
        if [[ "$line" =~ ^---[[:space:]]*$ ]]; then
            ((++delim))
            if ((delim == 2)); then
                ((has_license)) || printf 'license: %s\n' "$license" >>"${file}.new"
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

command -v git >/dev/null 2>&1 || die "git is required"

for entry in "${SKILLS[@]}"; do
    IFS='|' read -r mode repo author subpath name <<<"$entry"
    dest="${SKILLS_DIR}/${name}"

    case "$mode" in
    preserve)
        [[ -d "$dest" ]] || die "preserve skill missing: skills/${name} (expected from ${repo})"
        echo "ℹ️  Preserving skills/${name} (vendored from ${repo}; not overwritten)"
        ;;
    fetch)
        ensure_clone "$repo"
        src="$(clone_dir_for "$repo")/${subpath}"
        [[ -d "$src" ]] || die "upstream not found: ${subpath} in ${repo}"
        [[ -f "${src}/SKILL.md" ]] || die "no SKILL.md in ${subpath}"
        echo "⏳ Vendoring ${repo##*/}:${subpath} -> skills/${name} ..."
        rm -rf "$dest"
        mkdir -p "$dest"
        # Copy the upstream skill dir, excluding repo infrastructure that
        # never belongs in a vendored skill (.git, .github, .claude-plugin).
        # tar handles both subpath copies and repo-root copies (subpath=.) and
        # is a no-op when the excluded dirs don't exist.
        (cd "$src" && tar --exclude='./.git' --exclude='./.github' \
            --exclude='./.claude-plugin' -cf - .) | tar -xf - -C "$dest"
        if compgen -G "${dest}/scripts/*.sh" >/dev/null 2>&1; then
            chmod +x "${dest}"/scripts/*.sh
        fi
        inject_attribution "${dest}/SKILL.md" "$author"
        ;;
    *)
        die "unknown mode '${mode}' in registry entry: ${entry}"
        ;;
    esac
done

echo "✅ Processed ${#SKILLS[@]} registry entries"
for i in "${!CLONE_URLS[@]}"; do
    echo "ℹ️  ${CLONE_URLS[$i]} @ $(git -C "${CLONE_DIRS[$i]}" rev-parse HEAD)"
done
echo "ℹ️  Record the upstream SHA(s) above in your commit message."
echo "ℹ️  Run 'make install-skills' to symlink them into the agent skills paths."
