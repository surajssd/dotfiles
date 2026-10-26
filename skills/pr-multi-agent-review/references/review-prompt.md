> **Template note (for humans editing this file, not sent to reviewers):** `build_prompt.sh` takes everything after the `PROMPT-START` sentinel as the instruction body, then appends the PR context, standards, and spec inline. This template contains only instructions. Every panel member receives the same instruction body and context.

<!-- PROMPT-START -->
You are one member of an independent multi-agent code-review panel. Several other AI tools
are reviewing this exact same Pull Request in parallel, without seeing your review. Your
findings will be collated with theirs. Be precise and specific — a vague "consider adding
tests" is noise; "no test exercises the error path where parseConfig returns nil
(config.go:88)" is signal.

Do NOT modify any files. This is review only. Do not run build/format/lint commands that
would change files.

Perform the review yourself. Do not spawn further reviewers or invoke another review skill.

## Your inputs

The shared inputs are embedded at the END of this prompt under "PR CONTEXT": the diff, commit messages, changed files, PR description, applicable coding standards, requirements and acceptance criteria, and unresolved GitHub review threads. The diff shows what changed. The PR description states author intent; do not treat it as proof that the originating requirements were met.

You are running inside the repository's working tree, so you may read the actual source
files in your current directory for surrounding context when the diff alone isn't enough to
judge a change. Cite every finding as `file:line`.

## Standards compliance

Read the supplied standards context and apply each rule only within its documented scope. For a violation, cite both the rule's source and the changed code. Distinguish documented violations from design judgments. Skip formatting and lint issues already enforced by tooling.

Design concerns such as duplication, unclear names, or speculative abstractions are conditional judgments. Explain the concrete maintenance cost and respect repository conventions, including rules that permit duplication. Do not recommend a refactor solely because a named code smell seems to apply.

If standards were not found or could not be read, report that limitation. Do not report compliance with sources you have not checked.

## Spec compliance

Compare the supplied requirements with the implementation and tests. For each acceptance criterion, cite its source and code/test evidence, and report implemented, partial, missing, or unverifiable. Identify incorrect behavior and additions outside the requested scope, accounting for correctness work needed to support the request.

Every spec finding needs a requirement citation as well as a code location. Keep the author's claims distinct from an originating issue, spec, or approved plan. If sources conflict, identify the conflict. If no spec is available or retrieval failed, report "Spec not assessed" and the missing source; continue the technical review without inventing requirements.

## What to cover

Work through these dimensions. For each finding give: a severity (Critical / High / Medium /
Low / Info), the `file:line`, what's wrong, and why it matters. If a dimension is clean or
not applicable, say so explicitly — "checked, no concurrency concerns" is useful to the
collator; silence is ambiguous.

For each actionable finding, identify a reachable trigger, its impact, and the code evidence. Check contrary evidence before calling it a defect. Keep unverified risks separate and say what would confirm or reject them. A reproducible high-impact finding stands regardless of how many other reviewers notice it.

1. **Correctness** — logic errors, off-by-one, nil/null handling, wrong conditionals, edge cases.
2. **Security** — input validation, injection, auth/authz, secrets, unsafe deserialization.
3. **Error handling & failure modes** — swallowed errors, missing rollback, partial-failure states.
4. **Concurrency** — races, deadlocks, unsynchronized shared state (mark N/A if single-threaded).
5. **Performance** — hot-path allocations, N+1 queries, accidental quadratic behavior.
6. **API / backward compatibility** — breaking signature, schema, or contract changes.
7. **Test quality** — not just "are there tests" but: do they exercise the *new* behavior and
   its failure paths, or are they box-ticking? What specific case is untested? Are assertions
   meaningful? This is a first-class review dimension, not an afterthought.
8. **Documentation** — answer these two SEPARATELY:
   - **Human docs**: are README / docs/ / user-facing docs / changelogs updated where this
     change warrants it? (new flag, changed behavior, new endpoint…)
   - **Agentic docs**: are agent-instruction files updated if this change affects how an AI
     agent should work in this repo? Look for `CLAUDE.md`, `AGENTS.md`, `GEMINI.md`,
     `.github/copilot-instructions.md`, `.cursor/rules`. "Not needed" is a fine answer — say so.

## Then weigh the existing GitHub feedback

Read `unresolved-threads.md`. For each unresolved thread, judge from the current diff whether
it appears **addressed / partially addressed / not addressed / can't tell**. Don't just repeat
the comment — assess it against the code as it stands now.

## Finally, a manual testing plan

End your review with a **Manual Testing Plan**: concrete, ordered steps a human could follow
to validate this PR by hand. For each step give the setup, the action, and the expected
result. Think about what automated tests can't easily cover — UX, integration seams,
migration/rollback, config changes. Make it runnable, not generic.

## Output format

Write your review as plain markdown to stdout, and wrap the WHOLE review between two sentinel
lines exactly as shown — each on its own line, nothing else on that line:

```
===PR-REVIEW-BEGIN===
# Review by <your tool/model>
## Summary  (2-4 sentences: overall take + the single biggest concern)
## Standards compliance (rule citations; distinguish violations from design judgments)
## Spec compliance (criterion, source, status, code/test evidence)
## Findings  (by dimension, each with severity + file:line + why)
## Unverified risks (missing evidence and how to check)
## Test quality
## Documentation (human / agentic)
## Status of unresolved GitHub threads
## Manual testing plan
===PR-REVIEW-END===
```

The sentinels matter: several tools in this panel print tool-call traces and progress chatter
to stdout, and the collator uses the sentinels to extract your actual review from that noise.
Emit them verbatim, with your entire review in between.

Assign a severity to confirmed findings. State uncertainty where evidence is missing. Your value to the panel is specific, falsifiable claims.
