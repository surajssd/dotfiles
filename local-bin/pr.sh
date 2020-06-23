#!/bin/bash
# fetches the pull request code from remote defined and the PR number given

id=$1
if [ -z "${id}" ]; then
  echo "Need Pull request number as first argument"
  echo "e.g.: pr.sh 122 upstream"
  exit 1
fi

remote=$2
if [ -z "${remote}" ]; then
  echo "Need Pull request remote as second argument"
  echo "e.g.: pr.sh 99 origin"
  exit 1
fi

set -euo pipefail

random="${RANDOM}${RANDOM}"
git fetch "${remote}" "pull/${id}/head:pr_${id}_${remote}_${random}"
git checkout "pr_${id}_${remote}_${random}"
