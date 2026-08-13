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
# Clone-cache, die(), and inject_attribution() helpers come from lib.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
SKILLS_DIR="${REPO_DIR}/skills"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

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

# Clean up cached clones on exit.
trap clone_cache_cleanup EXIT

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
