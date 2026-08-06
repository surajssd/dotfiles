# Evidence-led review method

Use this checklist to review the change, not merely describe it. Record evidence for every affected axis and mark an axis `Not applicable` only after checking it.

## 1. Reconstruct the change contract

Write down five things before judging the implementation:

1. Author-stated intent from the PR body, issue, and commit messages.
2. Observable behavior the diff actually adds, removes, or changes.
3. Invariants that must remain true, including compatibility promises.
4. Explicit non-goals and intentionally unchanged paths.
5. Rollout, migration, and rollback expectations.

Treat author intent as context, not proof. Call out any mismatch between intent, implementation, tests, and documentation.

## 2. Trace the impact, not just the changed lines

- Read every changed production file and its relevant surrounding code.
- Search for callers, implementers, consumers, configuration keys, serialized fields, and tests. Inspect unchanged code when it participates in the changed behavior.
- Compare important code at the merge base and at `HEAD`; do not attribute a pre-existing issue to the PR.
- Follow success, empty, boundary, failure, retry, timeout, cancellation, and rollback paths.
- Identify state ownership and mutation: who reads, writes, caches, retries, and cleans up each value.
- Check behavior across process, service, persistence, queue, API, and trust boundaries.
- Separate generated, vendored, lockfile, fixture, and formatting churn from authored logic, while still checking meaningful dependency or generated-output changes.

## 3. Cover the engineering axes

| Axis | Questions to answer |
|---|---|
| Correctness | Does each realistic input produce the intended result? Are nil, empty, boundary, partial, duplicate, and stale states handled? |
| Contracts and compatibility | Do APIs, schemas, events, flags, defaults, CLIs, config, and error semantics remain compatible? |
| Data integrity and migration | Are mixed-version states, retries, idempotency, partial failure, rollback, and cleanup safe? |
| Concurrency | Are ownership, ordering, atomicity, races, deadlocks, cancellation, and retries sound? |
| Reliability | Are failures contained, surfaced, retried appropriately, and recoverable without loops or silent corruption? |
| Security and privacy | Are authentication, authorization, validation, injection, secrets, logging, dependency, and trust-boundary risks addressed? |
| Performance and resources | Does the change affect hot paths, algorithmic cost, fan-out, allocations, I/O, rate limits, connection lifetimes, or unbounded growth? |
| Observability and operations | Can operators detect, diagnose, roll out, and roll back the new behavior? Are metrics, logs, traces, alerts, and runbooks adequate? |
| Maintainability and design | Does the code fit existing abstractions, keep responsibilities clear, avoid needless coupling, and remain understandable? |
| Scope and change hygiene | Is the PR focused, reviewable, free of unrelated refactors, and clear about generated or mechanical churn? |
| Tests | Do tests prove behavior and failure modes rather than mirror the implementation? Are important regressions and negative paths covered? |
| Documentation and UX | Are user-facing behavior, API docs, examples, accessibility, i18n, and upgrade notes updated where relevant? |
| Dependencies and delivery | Are version changes, licenses, provenance, generated artifacts, build/release wiring, and deployment ordering safe? |

Add domain-specific axes such as financial precision, protocol conformance, scheduling fairness, or model quality when the change requires them.

## 4. Evaluate verification quality

Build a behavior-to-test matrix. Include the normal path, meaningful boundaries, each important failure mode, compatibility or migration states, and rollback. For each row, record the test or check, its level, whether it ran, and the remaining gap.

Do not use test-line count as evidence of quality. Look for weak assertions, implementation-coupled tests, missing negative cases, nondeterminism, unrealistic mocks, golden files that are not semantically checked, and tests that would pass before the change.

Run the smallest relevant existing checks first, then broader checks in proportion to risk and cost. Record the exact command, exit status, and result. Never imply a command or CI job passed if it was not observed.

## 5. Admit only defensible findings

Before reporting a finding, verify all of the following:

- The PR introduces or materially worsens it.
- A concrete trigger or execution path can reach it.
- The impact matters to a user, operator, caller, data contract, or maintainer.
- Exact code evidence supports it, and contrary evidence was checked.
- The recommendation is specific enough to act on.
- A test or verification step can demonstrate the fix when practical.

Use questions for genuine missing context; do not disguise speculation as a defect. If no actionable findings survive this gate, say so plainly and list only residual risks or unverified assumptions.

## 6. Classify consistently

### Priority

- **Blocker** — likely security breach, data loss/corruption, outage, or fundamental contract break; do not merge as written.
- **Major** — realistic correctness, compatibility, reliability, or operability defect that should be fixed before merge.
- **Minor** — bounded edge case, test gap, maintainability issue, or operational weakness worth addressing.
- **Note** — non-blocking suggestion or question; never present it as a defect.

### Confidence

- **High** — directly demonstrated by code, a reproducible command, or an authoritative contract.
- **Medium** — strongly supported, but one environmental or external assumption remains.
- **Low** — plausible question that needs author or runtime confirmation; keep it out of blocking findings.

### Recommendation

- **Request changes** when any blocker or major finding remains.
- **Comment** when only minor findings, questions, or material unverified risks remain.
- **Approve** when no actionable findings remain and verification is proportionate to the risk.

This is a local recommendation, not a GitHub review action. Never post or submit it unless the user explicitly asks.
