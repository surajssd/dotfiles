#!/usr/bin/env bash
#
# Vendor selected external rules (plain markdown) into this repo's rules/
# directory. Multi-source: each registry entry names its upstream repo +
# author + mode, mirroring fetch-external-skills.sh.
#   fetch    - clone upstream, copy the rule .md into rules/<name> verbatim
#              (no frontmatter or attribution injected).
#   preserve - already vendored & locally customised; only verify + report,
#              never overwrite (protects local edits).
# Fetched rules are committed; re-run to update and record the printed
# upstream SHA in the commit message. install-rules.sh symlinks them as usual.
# Clone-cache and die() helpers come from lib.sh.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(realpath "${SCRIPT_DIR}/..")"
RULES_DIR="${REPO_DIR}/rules"
# shellcheck source=/dev/null
source "${SCRIPT_DIR}/lib.sh"

# Registry: mode|repo_url|author|upstream_subpath|dest_name
# - upstream_subpath is the path to the .md within the upstream repo.
# - dest_name is the filename under rules/.
RULES=(
    "fetch|https://github.com/abatilo/vimrc|github.com/abatilo|rules/simple.md|simple.md"
    "fetch|https://github.com/abatilo/vimrc|github.com/abatilo|rules/comments.md|comments.md"
    "fetch|https://github.com/abatilo/vimrc|github.com/abatilo|rules/commit-notes.md|commit-notes.md"
    "fetch|https://github.com/abatilo/vimrc|github.com/abatilo|rules/simplified-technical-english.md|simplified-technical-english.md"
    "fetch|https://github.com/abatilo/vimrc|github.com/abatilo|rules/subtractive-engineering.md|subtractive-engineering.md"
)

# Clean up cached clones on exit.
trap clone_cache_cleanup EXIT

command -v git >/dev/null 2>&1 || die "git is required"
mkdir -p "$RULES_DIR"

for entry in "${RULES[@]}"; do
    IFS='|' read -r mode repo _ subpath name <<<"$entry"
    dest="${RULES_DIR}/${name}"

    case "$mode" in
    preserve)
        [[ -f "$dest" ]] || die "preserve rule missing: rules/${name} (expected from ${repo})"
        echo "ℹ️  Preserving rules/${name} (vendored from ${repo}; not overwritten)"
        ;;
    fetch)
        ensure_clone "$repo"
        src="$(clone_dir_for "$repo")/${subpath}"
        [[ -f "$src" ]] || die "upstream not found: ${subpath} in ${repo}"
        echo "⏳ Vendoring ${repo##*/}:${subpath} -> rules/${name} ..."
        cp "$src" "$dest"
        ;;
    *)
        die "unknown mode '${mode}' in registry entry: ${entry}"
        ;;
    esac
done

echo "✅ Processed ${#RULES[@]} registry entries"
for i in "${!CLONE_URLS[@]}"; do
    echo "ℹ️  ${CLONE_URLS[$i]} @ $(git -C "${CLONE_DIRS[$i]}" rev-parse HEAD)"
done
echo "ℹ️  Record the upstream SHA(s) above in your commit message."
echo "ℹ️  Run 'make install-rules' to symlink them into ~/.claude/rules."
