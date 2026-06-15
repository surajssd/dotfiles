#!/usr/bin/env bash
#
# gather_plan_context.sh — assemble review context for a PLAN file + the repo it targets.
#
# Usage:
#   gather_plan_context.sh <PLAN_PATH> <WORKDIR>
#
# Writes <WORKDIR>/context/ with:
#   plan.md                 copy of the plan under review (the primary artifact)
#   referenced-files/       real repo files the plan cites, mirrored at their repo paths
#   referenced-files.txt    embedded relpaths, one per line (drives build_prompt.sh's loop)
#   missing-references.txt  path-shaped tokens that DON'T exist in the repo (grounding aid)
#   repo-orientation.md     repo root, branch, status, recent commits, file list
#   meta.env                printf %q-quoted PLAN_PATH PLAN_NAME REPO_ROOT CURRENT_BRANCH
#
# Grounding heuristic (the load-bearing part). A token lifted from the plan is only ever
# classified "missing" when it is PATH-SHAPED — it contains a '/' or ends in a known
# source-file extension. Bare code tokens (`safe_fence()`, `set -euo pipefail`, `printf`)
# and function names are NEVER flagged. A path-shaped token is resolved against the repo
# with `git ls-files` as the authority: on-disk exact path, then a basename/suffix match
# (so a file cited by basename resolves to its real nested path), then nothing → missing.
# A "missing" path for a *plan* usually means a file the plan proposes to CREATE (expected)
# or a genuinely broken reference — build_prompt.sh and the prompt frame it that way so
# reviewers read it as a signal, not as noise.
#
# Every file lookup is anchored at REPO_ROOT (resolved via `git -C`), never at wherever the
# plan file happens to live, so the script works when invoked from any directory.

set -euo pipefail

err() {
    echo "$*" >&2
}

PLAN_PATH="${1:-}"
WORKDIR="${2:-}"

# Decision #1: a plan path is mandatory. Fail fast and loud if it's missing/unreadable.
if [ -z "${PLAN_PATH}" ] || [ ! -f "${PLAN_PATH}" ] || [ ! -r "${PLAN_PATH}" ]; then
    err "❌ plan-multi-agent-review needs a path to a readable plan file"
    err "   Usage: gather_plan_context.sh <PLAN_PATH> <WORKDIR>"
    exit 1
fi
if [ -z "${WORKDIR}" ]; then
    err "❌ gather_plan_context.sh needs a WORKDIR (got none)"
    exit 1
fi

# Make the plan path absolute so the workflow (which cd's into the repo to dispatch) can
# still find it, and so meta.env carries a usable value.
case "${PLAN_PATH}" in
/*) ;;
*) PLAN_PATH="${PWD}/${PLAN_PATH}" ;;
esac
PLAN_NAME="$(basename "${PLAN_PATH}")"

# Resolve the repo we vet the plan against and ESTABLISH it as the anchor for every
# lookup. git -C means we never depend on the caller's cwd.
REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || echo "${PWD}")"
CURRENT_BRANCH="$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo '(no branch)')"

CTX="${WORKDIR}/context"
REFDIR="${CTX}/referenced-files"
mkdir -p "${REFDIR}"

cp "${PLAN_PATH}" "${CTX}/plan.md"

# Scratch dir for all intermediates; cleaned on exit.
TMP_WORK="$(mktemp -d)"
trap 'rm -rf "${TMP_WORK}"' EXIT

# --- caps (anything dropped by a cap is logged, never silent) --------------------------
MAX_FILES=10
MAX_LINES=400
MAX_BYTES=65536  # 64 KiB per embedded file
TOTAL_MAX=262144 # 256 KiB across all embedded files
MAX_MISSING=50

# --- path-shape filter: known source-file extensions -----------------------------------
# A token is path-shaped if it contains '/' OR ends in one of these. Keep this generous;
# a missed extension just means a file gets left to live exploration, not mis-flagged.
EXT_RE='\.(sh|bash|zsh|fish|md|markdown|go|py|rb|ts|tsx|js|jsx|mjs|cjs|java|kt|kts|swift|scala|rs|c|h|cc|cpp|hpp|cs|php|pl|lua|r|m|mm|sql|proto|graphql|gql|html|css|scss|sass|less|vue|svelte|yaml|yml|json|json5|toml|ini|cfg|conf|env|tf|tfvars|hcl|txt|csv|tsv|xml|gradle|mk|cmake|lock)$'

is_path_shaped() {
    # Slash → path. Otherwise test the extension. Use a here-string, NOT `printf | grep`:
    # a piped `grep -q` closes the pipe on match and SIGPIPEs printf, which under pipefail
    # would make this function report FAILURE on a successful match.
    case "$1" in
    */*) return 0 ;;
    esac
    grep -qiE "${EXT_RE}" <<<"$1"
}

# --- 1. tokenize the plan --------------------------------------------------------------
# Paths live in inline-code spans and in H2–H4 heading tails. Pull those lines, strip URLs
# and word-leading absolute/home paths (a path that STARTS with '/' or '~' is external, not
# a repo-relative claim — but we must not chop the '/' inside scripts/run_reviewer.sh, so we
# only strip when the '/'/'~' is at a word boundary), then carve path-like runs. The token
# regex starts at an alphanumeric, so backtick/heading/ordinal punctuation falls away and a
# trailing `:line` ref (config.go:88) splits off on the ':'.
extract_tokens() {
    {
        # shellcheck disable=SC2016  # literal backticks: matching inline-code spans in the plan, not expanding vars.
        grep -oE '`[^`]+`' "${PLAN_PATH}" 2>/dev/null || true
        grep -E '^#{2,4} ' "${PLAN_PATH}" 2>/dev/null || true
    } |
        sed -E 's#https?://[^ ]+##g' |
        sed -E 's#(^|[[:space:]])[~/][^[:space:]]*##g' |
        grep -oE '[A-Za-z0-9_][A-Za-z0-9._/+-]*' || true
}

ALL_FILES="$(mktemp "${TMP_WORK}/all.XXXXXX")"
{ git -C "${REPO_ROOT}" ls-files 2>/dev/null || true; } >"${ALL_FILES}"

FOUND_LIST="$(mktemp "${TMP_WORK}/found.XXXXXX")"
MISSING_LIST="$(mktemp "${TMP_WORK}/missing.XXXXXX")"

# --- 2. classify each unique token: found (with real relpath) / missing / ignored ------
extract_tokens | sort -u | while IFS= read -r tok; do
    [ -n "${tok}" ] || continue
    # A directory reference is neither a found file nor a missing one.
    [ -d "${REPO_ROOT}/${tok}" ] && continue

    # Try to resolve the token to a real repo file FIRST, for every token regardless of
    # shape. An EXACT on-disk or ls-files match proves the file exists, so it can never be
    # a false positive — this is what lets extensionless root files like Makefile /
    # Dockerfile, cited bare, resolve as found. The looser basename/suffix match stays
    # gated behind path-shape so a bare word like `git` can't coincidentally resolve to a
    # nested file literally named `git`.
    rel=""
    if [ -f "${REPO_ROOT}/${tok}" ]; then
        # Exact path on disk (tracked or untracked) — record it verbatim.
        rel="${tok}"
    else
        # Exact ls-files line (any token — existence is proof enough).
        rel="$(awk -v t="${tok}" '$0 == t { print; exit }' "${ALL_FILES}")"
        # Basename in a nested dir: a line ending in "/<tok>". Path-shaped tokens only, so
        # bare code words can't match by coincidence. First match wins; ls-files is sorted.
        if [ -z "${rel}" ] && is_path_shaped "${tok}"; then
            rel="$(awk -v t="${tok}" '
                BEGIN { m = length(t) }
                { n = length($0); if (n > m && substr($0, n - m) == "/" t) { print; exit } }
            ' "${ALL_FILES}")"
        fi
    fi

    if [ -n "${rel}" ]; then
        printf '%s\n' "${rel}" >>"${FOUND_LIST}"
    elif is_path_shaped "${tok}"; then
        # Did not resolve, but it's PATH-SHAPED (contains '/' or ends in a source
        # extension) → a genuine missing reference. Bare code tokens like `safe_fence()`
        # or `set -euo pipefail` fall through and are ignored, never flagged as missing.
        # (if/elif with no trailing bare `&&` also keeps the loop's exit status 0, so the
        # `… | while` pipeline can't trip `set -e`/pipefail on a non-path final token.)
        printf '%s\n' "${tok}" >>"${MISSING_LIST}"
    fi
done

sort -u "${FOUND_LIST}" -o "${FOUND_LIST}"
sort -u "${MISSING_LIST}" -o "${MISSING_LIST}"

# --- 3. embed found files (mirrored at their real repo paths), honoring the caps --------
: >"${CTX}/referenced-files.txt"
total=0
count=0
dropped_count=0
dropped_size=0
while IFS= read -r rel; do
    [ -n "${rel}" ] || continue
    if [ "${count}" -ge "${MAX_FILES}" ]; then
        dropped_count=$((dropped_count + 1))
        continue
    fi
    src="${REPO_ROOT}/${rel}"
    [ -f "${src}" ] || continue

    # Truncate to MAX_LINES then MAX_BYTES. `head -c` may close the pipe early and SIGPIPE
    # the first head; `|| true` keeps pipefail from aborting (the bytes are already written).
    tmp="$(mktemp "${TMP_WORK}/ref.XXXXXX")"
    { head -n "${MAX_LINES}" "${src}" | head -c "${MAX_BYTES}" >"${tmp}"; } || true
    sz="$(wc -c <"${tmp}" | tr -d ' ')"

    if [ $((total + sz)) -gt "${TOTAL_MAX}" ]; then
        dropped_size=$((dropped_size + 1))
        rm -f "${tmp}"
        continue
    fi

    dest="${REFDIR}/${rel}"
    mkdir -p "$(dirname "${dest}")"
    mv "${tmp}" "${dest}"
    printf '%s\n' "${rel}" >>"${CTX}/referenced-files.txt"
    total=$((total + sz))
    count=$((count + 1))
done <"${FOUND_LIST}"

# --- 4. record missing path references (capped) ----------------------------------------
: >"${CTX}/missing-references.txt"
mcount=0
while IFS= read -r tok; do
    [ -n "${tok}" ] || continue
    mcount=$((mcount + 1))
    [ "${mcount}" -le "${MAX_MISSING}" ] && printf '%s\n' "${tok}" >>"${CTX}/missing-references.txt"
done <"${MISSING_LIST}"
missing_total="${mcount}"

# --- 5. repo orientation (best-effort; never aborts the gather) ------------------------
# shellcheck disable=SC2016  # literal backticks below are Markdown (code spans / ``` fences), not shell expansions.
{
    printf '# Repository orientation\n\n'
    printf -- '- Repo root: `%s`\n' "${REPO_ROOT}"
    printf -- '- Current branch: `%s`\n\n' "${CURRENT_BRANCH}"
    printf '## Working tree status (git status --short)\n\n```\n'
    { git -C "${REPO_ROOT}" status --short 2>/dev/null || true; }
    printf '```\n\n## Recent commits\n\n```\n'
    { git -C "${REPO_ROOT}" log --oneline -5 2>/dev/null || true; }
    printf '```\n\n## Tracked files (first 300)\n\n```\n'
    # Group the pipe so a SIGPIPE from `head` can't trip pipefail.
    { git -C "${REPO_ROOT}" ls-files 2>/dev/null | head -300; } || true
    printf '```\n'
} >"${CTX}/repo-orientation.md"

# --- 6. machine-readable metadata for the orchestrator ---------------------------------
{
    printf 'PLAN_PATH=%q\n' "${PLAN_PATH}"
    printf 'PLAN_NAME=%q\n' "${PLAN_NAME}"
    printf 'REPO_ROOT=%q\n' "${REPO_ROOT}"
    printf 'CURRENT_BRANCH=%q\n' "${CURRENT_BRANCH}"
} >"${CTX}/meta.env"

# --- 7. summary to stderr --------------------------------------------------------------
err "✅ Plan context gathered in ${CTX}"
err "ℹ️  embedded ${count} referenced file(s); ${missing_total} path ref(s) not found in repo"
[ "${dropped_count}" -gt 0 ] &&
    err "ℹ️  ${dropped_count} found file(s) past the ${MAX_FILES}-file cap left to live exploration"
[ "${dropped_size}" -gt 0 ] &&
    err "ℹ️  ${dropped_size} found file(s) skipped to stay under the ${TOTAL_MAX}-byte embed budget"
exit 0
