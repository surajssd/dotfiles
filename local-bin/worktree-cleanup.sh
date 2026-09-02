#!/usr/bin/env bash
#
# Interactively remove git worktrees whose branches have merged GitHub pull
# requests, together with their local branches. Never forces removal, never
# touches remote branches.

set -euo pipefail

# shellcheck source=/dev/null
source "$(dirname "$(realpath "${BASH_SOURCE[0]}")")"/util.sh

TAB=$(printf '\t')
NL='
'
GH_TIMEOUT=30
PR_VIEW_TEMPLATE='{{.number}}{{"\t"}}{{.state}}{{"\t"}}{{.headRefName}}{{"\t"}}{{.headRefOid}}{{"\t"}}{{.baseRefName}}{{"\t"}}{{with .mergeCommit}}{{.oid}}{{end}}'
PR_LIST_TEMPLATE='{{range .}}{{.number}}{{"\t"}}{{.state}}{{"\t"}}{{.headRefName}}{{"\t"}}{{.headRefOid}}{{"\t"}}{{.baseRefName}}{{"\t"}}{{with .mergeCommit}}{{.oid}}{{end}}{{"\n"}}{{end}}'

export GH_PROMPT_DISABLED=1
export GH_NO_UPDATE_NOTIFIER=1
export GH_PAGER=cat

# Physical path, so it compares equal to the physical paths git reports.
ORIG_PWD=$(pwd -P)
SCAN_ROOT=""
TIMEOUT_CMD=""
HAVE_FZF=0
TMP_DIR=""

# Repositories, parallel arrays indexed by repository id.
REPO_COUNT=0
REPO_KEY=()  # canonical common git dir, the deduplication identity
REPO_SEED=() # directory the repository was discovered from
REPO_MAIN=() # main worktree path, filled during enumeration
REPO_GH=()   # unset | ok | fail: cached gh repository resolution

# Worktree rows, parallel arrays indexed by row id. Machine data lives here;
# rendered table text is derived from it and never parsed back.
ROW_COUNT=0
R_PATH=()
R_REPOIDX=()
R_BRANCH=()
R_HEAD=()
R_PR=()
R_STATE=()
R_ACTION=()
R_REASON=()
R_DETAIL=()

SORTED_IDS=()
REMOVABLE_IDS=()
SELECTED_IDS=()

DISCOVERY_INCOMPLETE=0
UNKNOWN_TOTAL=0
REMOVED_COUNT=0
REVAL_SKIPPED=0
FAILED_COUNT=0

# Outputs of pr_lookup.
LK_STATE=UNKNOWN
LK_PR="-"
LK_REASON=""
LK_DETAIL=""

# Outputs of recheck_worktree_flags.
RC_FOUND=0
RC_LOCKED=0
RC_PRUNABLE=0

function usage() {
    echo "📖 Usage: worktree-cleanup.sh [scan-root]"
    echo ""
    echo "Recursively discovers git repositories under scan-root (default: the"
    echo "current directory), lists their linked worktrees, and interactively"
    echo "removes worktrees whose branch has a GitHub pull request that is"
    echo "MERGED or CLOSED and that contains every local commit: the PR head"
    echo "matches the local worktree HEAD exactly or contains it as an"
    echo "ancestor, or (for merged PRs) the merge commit contains it. The"
    echo "matching local branch is deleted after the worktree is removed."
    echo ""
    echo "Discovery skips these directories:"
    echo "    node_modules, .terraform, vendor, .venv, .cache"
    echo ""
    echo "A repository is included when its main checkout or any linked"
    echo "worktree is under scan-root. All worktrees of an included repository"
    echo "are then considered, even ones outside scan-root."
    echo ""
    echo "Never removed: main worktrees, and detached, dirty, locked, prunable,"
    echo "or unverified worktrees. Open pull requests never qualify."
    echo "Remote branches are never deleted."
    echo ""
    echo "Options:"
    echo "    --help, -h    ❓ Show this help message"
}

function parse_args() {
    while [[ $# -gt 0 ]]; do
        case "${1}" in
        --help | -h)
            usage
            exit 0
            ;;
        -*)
            err "❌ Unknown option: ${1}"
            usage
            exit 1
            ;;
        *)
            if [[ -n ${SCAN_ROOT} ]]; then
                err "❌ Only one scan-root argument is accepted, got extra: ${1}"
                usage
                exit 1
            fi
            SCAN_ROOT=${1}
            ;;
        esac
        shift
    done

    if [[ -z ${SCAN_ROOT} ]]; then
        SCAN_ROOT=${ORIG_PWD}
    fi
    if [[ ! -d ${SCAN_ROOT} ]]; then
        err "❌ Not a directory: ${SCAN_ROOT}"
        exit 1
    fi
    SCAN_ROOT=$(cd "${SCAN_ROOT}" && pwd -P)
}

function check_deps() {
    local dep
    for dep in git gh column; do
        if ! command -v "${dep}" >/dev/null 2>&1; then
            err "❌ Required dependency not found: ${dep}"
            exit 1
        fi
    done
    if command -v timeout >/dev/null 2>&1; then
        TIMEOUT_CMD=timeout
    elif command -v gtimeout >/dev/null 2>&1; then
        TIMEOUT_CMD=gtimeout
    else
        err "❌ Required dependency not found: timeout or gtimeout"
        exit 1
    fi
    if command -v fzf >/dev/null 2>&1; then
        HAVE_FZF=1
    fi
}

# Invoked via the EXIT trap set in make_tmp_dir.
# shellcheck disable=SC2329
function cleanup() {
    if [[ -n ${TMP_DIR} && -d ${TMP_DIR} ]]; then
        cd / && rm -rf "${TMP_DIR}"
    fi
}

function make_tmp_dir() {
    local gnu_mktemp="/opt/homebrew/opt/coreutils/libexec/gnubin/mktemp"
    if [[ -x ${gnu_mktemp} ]]; then
        TMP_DIR=$("${gnu_mktemp}" -d --suffix=.worktree-cleanup)
    else
        TMP_DIR=$(mktemp -d)
    fi
    trap cleanup EXIT
}

function run_gh() {
    local dir=${1}
    shift
    (cd "${dir}" && "${TIMEOUT_CMD}" "${GH_TIMEOUT}" gh "$@") 2>/dev/null
}

function add_repo() {
    local dir=${1}
    local inside common key i
    inside=$(git -C "${dir}" rev-parse --is-inside-work-tree 2>/dev/null) || return 0
    # Bare repositories report false here and are ignored.
    if [[ ${inside} != true ]]; then
        return 0
    fi
    common=$(git -C "${dir}" rev-parse --git-common-dir 2>/dev/null) || return 0
    key=$(cd "${dir}" 2>/dev/null && cd "${common}" 2>/dev/null && pwd -P) || return 0
    for ((i = 0; i < REPO_COUNT; i++)); do
        if [[ ${REPO_KEY[i]} == "${key}" ]]; then
            return 0
        fi
    done
    REPO_KEY[REPO_COUNT]=${key}
    REPO_SEED[REPO_COUNT]=${dir}
    REPO_MAIN[REPO_COUNT]=""
    REPO_GH[REPO_COUNT]="unset"
    REPO_COUNT=$((REPO_COUNT + 1))
}

function discover_repos() {
    err "⏳ Discovering git repositories under: $(tilde_path "${SCAN_ROOT}")"
    local gitpaths="${TMP_DIR}/git-paths"
    local find_err="${TMP_DIR}/find-errors"
    find "${SCAN_ROOT}" \
        \( -name node_modules -o -name .terraform -o -name vendor -o -name .venv -o -name .cache \) -prune \
        -o -name .git -prune -print0 >"${gitpaths}" 2>"${find_err}" || true

    if [[ -s ${find_err} ]]; then
        DISCOVERY_INCOMPLETE=1
        err "⚠️ Discovery could not read some paths, results are incomplete:"
        local line
        while IFS= read -r line; do
            err "    ${line}"
        done <"${find_err}"
    fi

    local gitpath
    while IFS= read -r -d '' gitpath; do
        add_repo "$(dirname "${gitpath}")"
    done <"${gitpaths}"

    add_repo "${SCAN_ROOT}"

    err "ℹ️ Found ${REPO_COUNT} git repositories."
}

function confirm_repo_count() {
    if ((REPO_COUNT <= 20)); then
        return 0
    fi
    err "ℹ️ Scan root $(tilde_path "${SCAN_ROOT}") contains ${REPO_COUNT} repositories."
    printf 'Continue and scan all %d repositories? [y/N] ' "${REPO_COUNT}"
    local ans=""
    read -r ans || true
    case "$(printf '%s' "${ans}" | tr '[:upper:]' '[:lower:]')" in
    y | yes) return 0 ;;
    *)
        err "✅ Cancelled before any GitHub calls."
        exit 0
        ;;
    esac
}

function add_row() {
    R_PATH[ROW_COUNT]=${1}
    R_REPOIDX[ROW_COUNT]=${2}
    R_BRANCH[ROW_COUNT]=${3}
    R_HEAD[ROW_COUNT]=${4}
    R_PR[ROW_COUNT]=${5}
    R_STATE[ROW_COUNT]=${6}
    R_ACTION[ROW_COUNT]=${7}
    R_REASON[ROW_COUNT]=${8}
    R_DETAIL[ROW_COUNT]=""
    ROW_COUNT=$((ROW_COUNT + 1))
}

function classify_record() {
    local repoidx=${1} is_main=${2} path=${3} head=${4} ref=${5}
    local detached=${6} locked=${7} prunable=${8} bare=${9}

    if [[ -z ${path} || ${bare} -eq 1 ]]; then
        return 0
    fi

    if [[ ${is_main} -eq 1 ]]; then
        REPO_MAIN[repoidx]=${path}
    fi

    case "${path}" in
    *"${NL}"*)
        err "⚠️ Skipping a worktree whose path contains a newline (unsupported)"
        return 0
        ;;
    esac

    local branch=""
    if [[ -n ${ref} ]]; then
        branch=${ref#refs/heads/}
    fi

    if [[ ${is_main} -eq 1 ]]; then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "main worktree"
        return 0
    fi
    if [[ ${prunable} -eq 1 ]]; then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "prunable, run: git worktree prune"
        return 0
    fi
    if [[ ${locked} -eq 1 ]]; then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "locked"
        return 0
    fi
    if [[ ${detached} -eq 1 ]]; then
        add_row "${path}" "${repoidx}" "" "${head}" "-" "NOT_CHECKED" "SKIP" "detached HEAD"
        return 0
    fi
    if [[ -z ${branch} ]]; then
        add_row "${path}" "${repoidx}" "" "${head}" "-" "NOT_CHECKED" "SKIP" "no branch checked out"
        return 0
    fi

    local cur_ref cur_head status_out
    if ! cur_ref=$(git -C "${path}" symbolic-ref --quiet HEAD 2>/dev/null); then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "cannot read HEAD ref"
        return 0
    fi
    if [[ ${cur_ref} != "${ref}" ]]; then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "branch changed during scan"
        return 0
    fi
    if ! cur_head=$(git -C "${path}" rev-parse HEAD 2>/dev/null); then
        add_row "${path}" "${repoidx}" "${branch}" "${head}" "-" "NOT_CHECKED" "SKIP" "cannot resolve HEAD"
        return 0
    fi
    if ! status_out=$(git -C "${path}" status --porcelain 2>/dev/null); then
        add_row "${path}" "${repoidx}" "${branch}" "${cur_head}" "-" "NOT_CHECKED" "SKIP" "git status failed"
        return 0
    fi
    if [[ -n ${status_out} ]]; then
        add_row "${path}" "${repoidx}" "${branch}" "${cur_head}" "-" "NOT_CHECKED" "SKIP" "dirty: uncommitted or untracked changes"
        return 0
    fi

    add_row "${path}" "${repoidx}" "${branch}" "${cur_head}" "-" "PENDING" "PENDING" ""
}

function enumerate_repo() {
    local repoidx=${1}
    local seed=${REPO_SEED[repoidx]}
    local listfile="${TMP_DIR}/worktree-list"
    if ! git -C "${seed}" worktree list --porcelain -z >"${listfile}" 2>/dev/null; then
        err "❌ Failed to list worktrees for repository at: $(tilde_path "${seed}")"
        DISCOVERY_INCOMPLETE=1
        return 0
    fi

    local field is_main=1
    local w_path="" w_head="" w_ref="" w_detached=0 w_locked=0 w_prunable=0 w_bare=0
    while IFS= read -r -d '' field; do
        if [[ -z ${field} ]]; then
            classify_record "${repoidx}" "${is_main}" "${w_path}" "${w_head}" "${w_ref}" \
                "${w_detached}" "${w_locked}" "${w_prunable}" "${w_bare}"
            if [[ -n ${w_path} ]]; then
                is_main=0
            fi
            w_path="" w_head="" w_ref="" w_detached=0 w_locked=0 w_prunable=0 w_bare=0
            continue
        fi
        case "${field}" in
        "worktree "*) w_path=${field#worktree } ;;
        "HEAD "*) w_head=${field#HEAD } ;;
        "branch "*) w_ref=${field#branch } ;;
        bare) w_bare=1 ;;
        detached) w_detached=1 ;;
        locked | "locked "*) w_locked=1 ;;
        prunable | "prunable "*) w_prunable=1 ;;
        esac
    done <"${listfile}"
    classify_record "${repoidx}" "${is_main}" "${w_path}" "${w_head}" "${w_ref}" \
        "${w_detached}" "${w_locked}" "${w_prunable}" "${w_bare}"
}

function resolve_repo_gh() {
    local repoidx=${1} dir=${2}
    if [[ ${REPO_GH[repoidx]} != unset ]]; then
        return 0
    fi
    if run_gh "${dir}" repo view --json nameWithOwner --template '{{.nameWithOwner}}' >/dev/null; then
        REPO_GH[repoidx]=ok
    else
        REPO_GH[repoidx]=fail
    fi
}

# Returns 0 when local_head is an ancestor of (or equal to) target, meaning
# every local commit is contained in it. When the target commit is not
# present locally, fetches fetch_ref from each remote until it appears.
function local_head_contained() {
    local dir=${1} local_head=${2} target=${3} fetch_ref=${4} remote
    if ! git -C "${dir}" cat-file -e "${target}^{commit}" 2>/dev/null; then
        while IFS= read -r remote; do
            "${TIMEOUT_CMD}" "${GH_TIMEOUT}" git -C "${dir}" fetch -q "${remote}" \
                "${fetch_ref}" </dev/null >/dev/null 2>&1 || continue
            if git -C "${dir}" cat-file -e "${target}^{commit}" 2>/dev/null; then
                break
            fi
        done < <(git -C "${dir}" remote 2>/dev/null)
    fi
    git -C "${dir}" merge-base --is-ancestor "${local_head}" "${target}" 2>/dev/null
}

# Returns 0 when every local commit is contained in the merged pull request:
# the local HEAD is an ancestor of the PR head (bots pushed commits on top),
# or of the merge commit (the local branch was moved to the merge result).
function merged_pr_contains_local() {
    local dir=${1} local_head=${2} prnum=${3} pr_head=${4} merge_oid=${5}
    if local_head_contained "${dir}" "${local_head}" "${pr_head}" "refs/pull/${prnum}/head"; then
        return 0
    fi
    if [[ -n ${merge_oid} ]]; then
        local_head_contained "${dir}" "${local_head}" "${merge_oid}" "${merge_oid}"
        return
    fi
    return 1
}

# Resolves the pull request for one branch. Sets LK_STATE, LK_PR, LK_REASON,
# and LK_DETAIL. LK_STATE is MERGED only when at least one pull request with
# the exact branch name is merged and its head either equals the local
# worktree HEAD or contains it as an ancestor.
function pr_lookup() {
    local dir=${1} branch=${2} head=${3}
    LK_STATE=UNKNOWN
    LK_PR="-"
    LK_REASON=""
    LK_DETAIL=""
    local note=""

    local prnum=""
    prnum=$(git -C "${dir}" config --get "branch.${branch}.prNumber" 2>/dev/null) || prnum=""
    case "${prnum}" in
    '' | *[!0-9]*) prnum="" ;;
    esac

    local out p_num p_state p_href p_hoid p_base p_moid
    if [[ -n ${prnum} ]]; then
        if out=$(run_gh "${dir}" pr view "${prnum}" \
            --json number,state,headRefName,headRefOid,baseRefName,mergeCommit \
            --template "${PR_VIEW_TEMPLATE}"); then
            IFS=${TAB} read -r p_num p_state p_href p_hoid p_base p_moid <<<"${out}"
            if [[ ${p_href} == "${branch}" && ${p_hoid} == "${head}" ]]; then
                LK_STATE=${p_state}
                LK_PR="#${p_num}(${p_base})"
                LK_DETAIL="PR #${p_num} -> ${p_base} [${p_state}]"
                case "${p_state}" in
                MERGED) LK_REASON="PR #${p_num} merged into ${p_base}" ;;
                CLOSED) LK_REASON="PR #${p_num} closed without merge" ;;
                *) LK_REASON="PR #${p_num} is ${p_state}" ;;
                esac
                return 0
            fi
            if [[ ${p_href} == "${branch}" && ${p_state} == MERGED ]] &&
                merged_pr_contains_local "${dir}" "${head}" "${p_num}" "${p_hoid}" "${p_moid}"; then
                LK_STATE=MERGED
                LK_PR="#${p_num}(${p_base})"
                LK_DETAIL="PR #${p_num} -> ${p_base} [MERGED, contains local HEAD]"
                LK_REASON="PR #${p_num} merged into ${p_base}, all local commits are in the merged PR"
                return 0
            fi
            if [[ ${p_href} == "${branch}" && ${p_state} == CLOSED ]] &&
                local_head_contained "${dir}" "${head}" "${p_hoid}" "refs/pull/${p_num}/head"; then
                LK_STATE=CLOSED
                LK_PR="#${p_num}(${p_base})"
                LK_DETAIL="PR #${p_num} -> ${p_base} [CLOSED, contains local HEAD]"
                LK_REASON="PR #${p_num} closed without merge, all local commits are in the PR"
                return 0
            fi
            note="stale breadcrumb PR #${prnum} ignored"
        else
            note="breadcrumb PR #${prnum} lookup failed, ignored"
        fi
    fi

    local list
    if ! list=$(run_gh "${dir}" pr list --state all --head "${branch}" --limit 501 \
        --json number,state,headRefName,headRefOid,baseRefName,mergeCommit \
        --template "${PR_LIST_TEMPLATE}"); then
        LK_REASON="PR search failed or timed out${note:+ (${note})}"
        return 0
    fi

    local total=0 merged_pr="" merged_detail="" match_pr="" match_detail="" match_states=""
    local contained any_contained=0
    while IFS=${TAB} read -r p_num p_state p_href p_hoid p_base p_moid; do
        if [[ -z ${p_num} ]]; then
            continue
        fi
        total=$((total + 1))
        if [[ ${p_href} != "${branch}" ]]; then
            continue
        fi
        contained=0
        if [[ ${p_hoid} != "${head}" ]]; then
            if [[ ${p_state} == MERGED ]]; then
                if ! merged_pr_contains_local "${dir}" "${head}" "${p_num}" "${p_hoid}" "${p_moid}"; then
                    continue
                fi
            elif ! local_head_contained "${dir}" "${head}" "${p_hoid}" "refs/pull/${p_num}/head"; then
                continue
            fi
            contained=1
        fi
        if [[ ${contained} -eq 1 ]]; then
            any_contained=1
            match_pr="${match_pr}${match_pr:+,}#${p_num}(${p_base})"
            match_detail="${match_detail}${match_detail:+${NL}}PR #${p_num} -> ${p_base} [${p_state}, contains local HEAD]"
        else
            match_pr="${match_pr}${match_pr:+,}#${p_num}(${p_base})"
            match_detail="${match_detail}${match_detail:+${NL}}PR #${p_num} -> ${p_base} [${p_state}]"
        fi
        if [[ ${p_state} == MERGED ]]; then
            merged_pr="${merged_pr}${merged_pr:+,}#${p_num}(${p_base})"
            if [[ ${contained} -eq 1 ]]; then
                merged_detail="${merged_detail}${merged_detail:+${NL}}PR #${p_num} -> ${p_base} [MERGED, contains local HEAD]"
            else
                merged_detail="${merged_detail}${merged_detail:+${NL}}PR #${p_num} -> ${p_base} [MERGED]"
            fi
        else
            match_states="${match_states},${p_state}"
        fi
    done <<<"${list}"

    if ((total >= 501)); then
        LK_REASON="branch matches more than 500 PRs, refusing to decide${note:+ (${note})}"
        return 0
    fi
    if [[ -z ${match_pr} ]]; then
        if ((total == 0)); then
            LK_REASON="no PR found for branch${note:+ (${note})}"
        else
            LK_REASON="no PR matches both branch name and HEAD${note:+ (${note})}"
        fi
        return 0
    fi
    if [[ -n ${merged_pr} ]]; then
        LK_STATE=MERGED
        LK_PR=${merged_pr}
        LK_DETAIL=${merged_detail}
        if [[ ${any_contained} -eq 1 ]]; then
            LK_REASON="merged: ${merged_pr}, all local commits are in the merged PR${note:+ (${note})}"
        else
            LK_REASON="merged: ${merged_pr}${note:+ (${note})}"
        fi
        return 0
    fi
    case "${match_states}," in
    *,OPEN,*)
        LK_STATE=OPEN
        LK_PR=${match_pr}
        LK_DETAIL=${match_detail}
        LK_REASON="PR ${match_pr} is OPEN, not merged${note:+ (${note})}"
        return 0
        ;;
    esac
    LK_STATE=CLOSED
    LK_PR=${match_pr}
    LK_DETAIL=${match_detail}
    if [[ ${any_contained} -eq 1 ]]; then
        LK_REASON="closed without merge: ${match_pr}, all local commits are in the PR${note:+ (${note})}"
    else
        LK_REASON="closed without merge: ${match_pr}${note:+ (${note})}"
    fi
}

function resolve_prs() {
    local id pending=0
    for ((id = 0; id < ROW_COUNT; id++)); do
        if [[ ${R_ACTION[id]} == PENDING ]]; then
            pending=$((pending + 1))
        fi
    done
    if ((pending == 0)); then
        return 0
    fi

    err "⏳ Resolving pull requests for ${pending} candidate branches..."
    local n=0 path branch repoidx
    for ((id = 0; id < ROW_COUNT; id++)); do
        if [[ ${R_ACTION[id]} != PENDING ]]; then
            continue
        fi
        n=$((n + 1))
        path=${R_PATH[id]}
        branch=${R_BRANCH[id]}
        repoidx=${R_REPOIDX[id]}
        err "⏳ [${n}/${pending}] ${branch} ($(tilde_path "${path}"))"

        resolve_repo_gh "${repoidx}" "${path}"
        if [[ ${REPO_GH[repoidx]} == fail ]]; then
            R_PR[id]="-"
            R_STATE[id]=UNKNOWN
            R_ACTION[id]=SKIP
            R_REASON[id]="gh could not resolve a GitHub repository from the remotes"
            continue
        fi

        pr_lookup "${path}" "${branch}" "${R_HEAD[id]}"
        R_PR[id]=${LK_PR}
        R_STATE[id]=${LK_STATE}
        R_REASON[id]=${LK_REASON}
        R_DETAIL[id]=${LK_DETAIL}
        if [[ ${LK_STATE} == MERGED || ${LK_STATE} == CLOSED ]]; then
            R_ACTION[id]=REMOVE
        else
            R_ACTION[id]=SKIP
        fi
    done
}

function sort_rows() {
    SORTED_IDS=()
    if ((ROW_COUNT == 0)); then
        return 0
    fi
    local id rank sortfile="${TMP_DIR}/sort-input"
    : >"${sortfile}"
    for ((id = 0; id < ROW_COUNT; id++)); do
        rank=1
        if [[ ${R_ACTION[id]} == REMOVE ]]; then
            rank=0
        fi
        printf '%d\t%s\t%s\t%s\t%d\n' "${rank}" "${R_STATE[id]}" \
            "${REPO_MAIN[${R_REPOIDX[id]}]}" "${R_PATH[id]}" "${id}" >>"${sortfile}"
    done
    while IFS= read -r id; do
        SORTED_IDS[${#SORTED_IDS[@]}]=${id}
    done < <(sort -t "${TAB}" -k1,1n -k2,2 -k3,3 -k4,4 <"${sortfile}" | awk -F '\t' '{print $NF}')
}

function tilde_path() {
    case "${1}" in
    "${HOME}") echo "~" ;;
    "${HOME}"/*) echo "~${1#"${HOME}"}" ;;
    *) echo "${1}" ;;
    esac
}

# Relative to the directory the command was run from when inside it,
# otherwise the tilde form.
function display_path() {
    case "${1}" in
    "${ORIG_PWD}") echo "." ;;
    "${ORIG_PWD}"/*) echo "${1#"${ORIG_PWD}"/}" ;;
    *) tilde_path "${1}" ;;
    esac
}

function state_display() {
    case "${1}" in
    MERGED) echo "🟣 MERGED" ;;
    OPEN) echo "🟢 OPEN" ;;
    CLOSED) echo "🔴 CLOSED" ;;
    UNKNOWN) echo "❓ UNKNOWN" ;;
    NOT_CHECKED) echo "⚪ NOT_CHECKED" ;;
    *) echo "${1}" ;;
    esac
}

function render_table() {
    local i id repo_display
    echo ""
    {
        printf 'WORKTREE\tREPO\tBRANCH\tPR\tPR_STATE\tACTION\tREASON\n'
        for ((i = 0; i < ${#SORTED_IDS[@]}; i++)); do
            id=${SORTED_IDS[i]}
            repo_display=${REPO_MAIN[${R_REPOIDX[id]}]}
            repo_display=${repo_display//${NL}/ }
            printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
                "$(tilde_path "${R_PATH[id]}")" \
                "$(display_path "${repo_display}")" \
                "${R_BRANCH[id]:--}" \
                "${R_PR[id]}" \
                "$(state_display "${R_STATE[id]}")" \
                "${R_ACTION[id]}" \
                "${R_REASON[id]}"
        done
    } | column -t -s "${TAB}"
    echo ""
}

function collect_removable() {
    REMOVABLE_IDS=()
    local i id
    for ((i = 0; i < ${#SORTED_IDS[@]}; i++)); do
        id=${SORTED_IDS[i]}
        if [[ ${R_ACTION[id]} == REMOVE ]]; then
            REMOVABLE_IDS[${#REMOVABLE_IDS[@]}]=${id}
        fi
    done
}

function build_previews() {
    local dir="${TMP_DIR}/previews" i id path
    mkdir -p "${dir}"
    for ((i = 0; i < ${#REMOVABLE_IDS[@]}; i++)); do
        id=${REMOVABLE_IDS[i]}
        path=${R_PATH[id]}
        {
            echo "Branch: ${R_BRANCH[id]}"
            echo ""
            echo "Verified pull requests:"
            echo "${R_DETAIL[id]}"
            echo ""
            echo "Latest commits:"
            git -C "${path}" log --oneline -5 2>/dev/null || echo "(git log failed)"
        } >"${dir}/${id}"
    done
}

function select_with_fzf() {
    build_previews
    local i id sel line
    local infile="${TMP_DIR}/fzf-input"
    : >"${infile}"
    for ((i = 0; i < ${#REMOVABLE_IDS[@]}; i++)); do
        id=${REMOVABLE_IDS[i]}
        printf '%d\t%s\t%s\t%s\t%s\n' "${id}" "$(state_display "${R_STATE[id]}")" \
            "$(tilde_path "${R_PATH[id]}")" "${R_BRANCH[id]}" "${R_PR[id]}" >>"${infile}"
    done
    sel=$(column -t -s "${TAB}" <"${infile}" | fzf --multi --reverse --height=50% \
        --header="TAB toggles, ctrl-a selects all, ctrl-d clears, ENTER confirms, ESC cancels (PR_STATE  WORKTREE  BRANCH  PR)" \
        --bind "ctrl-a:select-all,ctrl-d:deselect-all" \
        --with-nth=2.. \
        --preview="cat '${TMP_DIR}/previews/'{1}" \
        --preview-window=right:35%) || true
    if [[ -z ${sel} ]]; then
        return 0
    fi
    while IFS= read -r line; do
        id=${line%%[[:space:]]*}
        SELECTED_IDS[${#SELECTED_IDS[@]}]=${id}
    done <<<"${sel}"
}

function select_numbered() {
    local i id n=${#REMOVABLE_IDS[@]}
    echo "Removable worktrees:"
    for ((i = 0; i < n; i++)); do
        id=${REMOVABLE_IDS[i]}
        printf '%3d) %s  %s  [%s  %s]\n' "$((i + 1))" "$(state_display "${R_STATE[id]}")" \
            "$(tilde_path "${R_PATH[id]}")" "${R_BRANCH[id]}" "${R_PR[id]}"
    done
    printf 'Enter numbers to remove (space or comma separated, empty cancels): '
    local line=""
    read -r line || true
    line=${line//,/ }
    if [[ -z ${line// /} ]]; then
        return 0
    fi
    local toks=() tok chosen=" "
    read -r -a toks <<<"${line}"
    for tok in "${toks[@]}"; do
        case "${tok}" in
        '' | *[!0-9]*)
            err "❌ Invalid selection, not a number: ${tok}"
            exit 1
            ;;
        esac
        tok=$((10#${tok}))
        if ((tok < 1 || tok > n)); then
            err "❌ Selection out of range: ${tok}"
            exit 1
        fi
        case "${chosen}" in
        *" ${tok} "*)
            err "❌ Duplicate selection: ${tok}"
            exit 1
            ;;
        esac
        chosen="${chosen}${tok} "
        SELECTED_IDS[${#SELECTED_IDS[@]}]=${REMOVABLE_IDS[$((tok - 1))]}
    done
}

function confirm_removal() {
    local i id path inside=0
    echo ""
    echo "Selected worktrees:"
    for ((i = 0; i < ${#SELECTED_IDS[@]}; i++)); do
        id=${SELECTED_IDS[i]}
        path=${R_PATH[id]}
        echo "    $(tilde_path "${path}")"
        case "${ORIG_PWD}" in
        "${path}" | "${path}"/*) inside=1 ;;
        esac
    done
    if ((inside == 1)); then
        echo "Your shell is inside a selected worktree and will be left in a removed directory."
    fi
    printf '🗑️ Remove these %d worktrees and their local branches? [y/N] ' "${#SELECTED_IDS[@]}"
    local ans=""
    read -r ans || true
    case "$(printf '%s' "${ans}" | tr '[:upper:]' '[:lower:]')" in
    y | yes) return 0 ;;
    *)
        err "✅ Cancelled, no changes made."
        exit 0
        ;;
    esac
}

function reval_skip() {
    err "ℹ️ Skipping $(tilde_path "${1}"): ${2}"
    REVAL_SKIPPED=$((REVAL_SKIPPED + 1))
}

function fail_item() {
    err "❌ $(tilde_path "${1}"): ${2}"
    FAILED_COUNT=$((FAILED_COUNT + 1))
}

# Returns 0 when the branch ref is still checked out in any worktree of the
# repository, or when that cannot be verified.
function branch_still_checked_out() {
    local main=${1} ref=${2}
    local listfile="${TMP_DIR}/recheck-branches" field
    if ! git -C "${main}" worktree list --porcelain -z >"${listfile}" 2>/dev/null; then
        return 0
    fi
    while IFS= read -r -d '' field; do
        if [[ ${field} == "branch ${ref}" ]]; then
            return 0
        fi
    done <"${listfile}"
    return 1
}

function recheck_worktree_flags() {
    local main=${1} path=${2}
    local listfile="${TMP_DIR}/recheck-flags" field cur in_target=0
    RC_FOUND=0
    RC_LOCKED=0
    RC_PRUNABLE=0
    git -C "${main}" worktree list --porcelain -z >"${listfile}" 2>/dev/null || return 0
    while IFS= read -r -d '' field; do
        case "${field}" in
        "worktree "*)
            cur=${field#worktree }
            if [[ ${cur} == "${path}" ]]; then
                RC_FOUND=1
                in_target=1
            else
                in_target=0
            fi
            ;;
        locked | "locked "*)
            if ((in_target == 1)); then
                RC_LOCKED=1
            fi
            ;;
        prunable | "prunable "*)
            if ((in_target == 1)); then
                RC_PRUNABLE=1
            fi
            ;;
        esac
    done <"${listfile}"
}

function branch_config_exists() {
    local main=${1} branch=${2} name
    while IFS= read -r name; do
        case "${name}" in
        "branch.${branch}."*) return 0 ;;
        esac
    done < <(git -C "${main}" config --local --list --name-only 2>/dev/null)
    return 1
}

function remove_one() {
    local id=${1}
    local path=${R_PATH[id]} branch=${R_BRANCH[id]} repoidx=${R_REPOIDX[id]}
    local expect_head=${R_HEAD[id]}
    local main=${REPO_MAIN[repoidx]}
    local ref="refs/heads/${branch}"

    err "⏳ Removing: $(tilde_path "${path}")"

    if [[ ! -d ${path} ]]; then
        reval_skip "${path}" "worktree directory no longer exists"
        return 0
    fi
    local cur_ref
    if ! cur_ref=$(git -C "${path}" symbolic-ref --quiet HEAD 2>/dev/null); then
        reval_skip "${path}" "cannot read HEAD ref anymore"
        return 0
    fi
    if [[ ${cur_ref} != "${ref}" ]]; then
        reval_skip "${path}" "branch changed to ${cur_ref}"
        return 0
    fi
    local cur_head
    if ! cur_head=$(git -C "${path}" rev-parse HEAD 2>/dev/null); then
        reval_skip "${path}" "cannot resolve HEAD anymore"
        return 0
    fi
    if [[ ${cur_head} != "${expect_head}" ]]; then
        reval_skip "${path}" "HEAD moved from ${expect_head} to ${cur_head}"
        return 0
    fi
    local status_out
    if ! status_out=$(git -C "${path}" status --porcelain 2>/dev/null); then
        reval_skip "${path}" "git status failed"
        return 0
    fi
    if [[ -n ${status_out} ]]; then
        reval_skip "${path}" "became dirty"
        return 0
    fi
    recheck_worktree_flags "${main}" "${path}"
    if ((RC_FOUND == 0)); then
        reval_skip "${path}" "no longer listed as a worktree"
        return 0
    fi
    if ((RC_LOCKED == 1)); then
        reval_skip "${path}" "became locked"
        return 0
    fi
    if ((RC_PRUNABLE == 1)); then
        reval_skip "${path}" "became prunable"
        return 0
    fi

    pr_lookup "${path}" "${branch}" "${cur_head}"
    if [[ ${LK_STATE} != MERGED && ${LK_STATE} != CLOSED ]]; then
        reval_skip "${path}" "fresh PR check did not confirm a merged or closed PR: ${LK_REASON}"
        return 0
    fi

    local git_err="${TMP_DIR}/git-error"
    if ! git -C "${main}" worktree remove "${path}" 2>"${git_err}"; then
        fail_item "${path}" "worktree remove failed, branch preserved: $(cat "${git_err}")"
        return 0
    fi
    err "✅ Removed worktree: $(tilde_path "${path}")"

    if branch_still_checked_out "${main}" "${ref}"; then
        fail_item "${path}" "branch ${branch} is still checked out in another worktree, branch preserved"
        return 0
    fi

    if ! git -C "${main}" update-ref -d "${ref}" "${cur_head}" 2>"${git_err}"; then
        fail_item "${path}" "branch deletion failed, branch preserved: $(cat "${git_err}")"
        return 0
    fi
    err "✅ Deleted branch: ${branch}"

    if branch_config_exists "${main}" "${branch}"; then
        if ! git -C "${main}" config --local --remove-section "branch.${branch}" 2>"${git_err}"; then
            fail_item "${path}" "branch config cleanup failed: $(cat "${git_err}")"
            return 0
        fi
    fi

    REMOVED_COUNT=$((REMOVED_COUNT + 1))
}

function print_summary() {
    local id known_skips=0 unknown=0
    for ((id = 0; id < ROW_COUNT; id++)); do
        if [[ ${R_ACTION[id]} != SKIP ]]; then
            continue
        fi
        if [[ ${R_STATE[id]} == UNKNOWN ]]; then
            unknown=$((unknown + 1))
        else
            known_skips=$((known_skips + 1))
        fi
    done
    UNKNOWN_TOTAL=${unknown}
    echo ""
    echo "Summary:"
    echo "    ✅ Removed worktrees and branches: ${REMOVED_COUNT}"
    echo "    ℹ️ Known skipped worktrees: ${known_skips}"
    echo "    ℹ️ Worktrees with unknown PR information: ${unknown}"
    echo "    ℹ️ Skipped during final revalidation: ${REVAL_SKIPPED}"
    echo "    ❌ Failed or partially completed removals: ${FAILED_COUNT}"
}

function compute_exit() {
    local rc=0
    if ((DISCOVERY_INCOMPLETE == 1)); then
        err "❌ Discovery was incomplete."
        rc=1
    fi
    if ((UNKNOWN_TOTAL > 0 || REVAL_SKIPPED > 0 || FAILED_COUNT > 0)); then
        rc=1
    fi
    exit "${rc}"
}

function main() {
    parse_args "$@"
    check_deps
    make_tmp_dir
    discover_repos

    if ((REPO_COUNT == 0)); then
        err "ℹ️ No git repositories found, nothing to do."
        compute_exit
    fi
    confirm_repo_count

    err "⏳ Enumerating worktrees..."
    local i
    for ((i = 0; i < REPO_COUNT; i++)); do
        enumerate_repo "${i}"
    done
    if ((ROW_COUNT == 0)); then
        err "ℹ️ No worktrees found, nothing to do."
        print_summary
        compute_exit
    fi

    resolve_prs
    sort_rows
    render_table
    collect_removable

    if ((${#REMOVABLE_IDS[@]} == 0)); then
        err "ℹ️ No removable worktrees."
        print_summary
        compute_exit
    fi

    if ((HAVE_FZF == 1)); then
        select_with_fzf
    else
        select_numbered
    fi
    if ((${#SELECTED_IDS[@]} == 0)); then
        err "✅ Nothing selected, no changes made."
        exit 0
    fi

    confirm_removal

    cd "${TMP_DIR}"
    for ((i = 0; i < ${#SELECTED_IDS[@]}; i++)); do
        remove_one "${SELECTED_IDS[i]}"
    done

    print_summary
    compute_exit
}

main "$@"
