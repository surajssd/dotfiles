#!/usr/bin/env bash
set -euo pipefail
printf '%s\0' "$@" >>"$OPENSPEC_CALLS"
if [[ "$1" == "init" ]]; then
    mkdir -p openspec
    if [[ ! -e openspec/config.yaml ]]; then
        printf 'schema: spec-driven\n' >openspec/config.yaml
    fi
    if [[ "${OPENSPEC_TEST_MODE:-}" == "init-failure" ]]; then
        printf 'initialization failed\n' >&2
        exit 7
    fi
    mkdir -p openspec/changes openspec/specs
    printf 'Initialized OpenSpec\n'
    exit 0
fi
if [[ "$1" == "list" ]]; then
    if [[ "${OPENSPEC_TEST_MODE:-}" == "wrong-root" ]]; then
        printf '{"root":{"path":"/elsewhere"}}\n'
    else
        printf '{"root":{"path":"%s"}}\n' "$PWD"
    fi
    exit 0
fi
mkdir "openspec/changes/$3"
printf 'schema: spec-driven\n' >"openspec/changes/$3/.openspec.yaml"
printf '# Preserve\n' >"openspec/changes/$3/proposal.md"
if [[ "${OPENSPEC_TEST_MODE:-}" == "failure" ]]; then
    printf 'OpenSpec failed\n' >&2
    exit 7
fi
if [[ "${OPENSPEC_TEST_MODE:-}" == "metadata-exists" ]]; then
    printf 'original' >"openspec/changes/$3/plan.md"
fi
if [[ "${OPENSPEC_TEST_MODE:-}" == "wrong-path" ]]; then
    printf '{"change":{"path":"/elsewhere"}}\n'
elif [[ "${OPENSPEC_TEST_MODE:-}" == "invalid-json" ]]; then
    printf 'broken JSON\n'
else
    printf '{"change":{"path":"%s/openspec/changes/%s"}}\n' "$PWD" "$3"
fi
