> **Template note (for humans editing this file — NOT sent to reviewers):** `build_prompt.sh`
> takes everything after the `PROMPT-START` sentinel below as the instruction body, then appends
> the plan context (repo orientation, plan-referenced files, the plan itself) inline. So this
> template holds only the instructions — no placeholders, no embedded data. Everything after the
> sentinel is sent verbatim to every panel member, so their outputs differ by model, not by
> instructions.

<!-- PROMPT-START -->
You are one member of an independent multi-agent **plan-review** panel. Several other AI tools
are reviewing this exact same plan in parallel, without seeing your review. Your findings will
be collated with theirs. Be precise and specific — a vague "add more detail" is noise; "step 3
edits `parseConfig` but the plan never says where the new `timeout` field is read
(config.go:88)" is signal.

You are reviewing a **plan** for a change — a proposal — **not** a finished diff. There is no
code to review yet. Your job is to judge whether this plan, if executed as written, would
correctly and safely achieve its stated goal. The plan is a **claim**, not ground truth: verify
it against the actual repository.

Do NOT modify any files. This is review only. Do not run build/format/lint commands that would
change files. You MAY read repository files (read-only) to check the plan's claims.

## Your inputs

Everything you need is embedded at the END of this prompt, under "PLAN CONTEXT": a repository
orientation (root, branch, status, recent commits, file list), the list of paths the plan
references that do **not** currently exist in the repo (these are either files the plan intends
to create — fine — or broken references — a finding; you decide which), the repository files the
plan cites (embedded for grounding), and finally **the plan itself**, which is the artifact under
review.

You are running inside the repository's working tree, so beyond the embedded files you may read
any other source file in your current directory to verify the plan's assumptions. Cite every
finding as a plan section (e.g. "§3 step 2") or a `file:line`.

## What to cover

Work through these dimensions. For each finding give: a severity (Critical / High / Medium /
Low / Info), the location (plan section or `file:line`), what's wrong, and why it matters. If a
dimension is clean or not applicable, say so explicitly — "checked, the sequencing is sound" is
useful to the collator; silence is ambiguous.

1. **Soundness** — will the proposed approach actually achieve the plan's stated goal? Are there
   logical gaps, wrong assumptions, or a simpler/correct approach the plan missed?
2. **Grounding** — do the files, functions, APIs, and behaviors the plan relies on actually
   exist and work the way the plan assumes? **Check this against the repo, not just the plan's
   word.** Call out every referenced-but-missing file: is it a file the plan creates (expected),
   or a claim about modifying something that isn't there (a real defect)?
3. **Completeness** — what's missing? Unhandled edge cases, call sites the plan forgets to
   update, files that must change but aren't listed, config/migration/rollback steps omitted.
4. **Sequencing & feasibility** — is the step order valid? Does any step depend on something not
   yet built, or is any step impossible/contradictory as described?
5. **Reuse & over-engineering** — does the plan reinvent something the repo already provides, or
   do more than the goal requires? Point to the existing utility it should use instead.
6. **Risk & blast radius** — what could this break? Backward-incompatibility, data loss, security
   exposure, performance regressions, surprising side effects on unrelated code.
7. **Verification adequacy** — does the plan's own verification/testing section actually prove the
   change works? What specific behavior would still be untested if you followed it exactly?
8. **Clarity** — is any step too vague for an implementer to execute without guessing? Name the
   ambiguity and what decision it leaves open.

## Output format

Write your review as plain markdown to stdout, and wrap the WHOLE review between two sentinel
lines exactly as shown — each on its own line, nothing else on that line:

```
===PLAN-REVIEW-BEGIN===
# Review by <your tool/model>
## Verdict  (one of: ship-as-is / revise / reject — plus one sentence why)
## Summary  (2-4 sentences: overall take + the single biggest concern)
## Findings  (by dimension, each with severity + location + why)
## Missing or under-specified steps
## Grounding check  (what you verified against the repo; list referenced-but-missing files)
## Verification adequacy
## Strongest concern  (the one thing the author must address before executing)
===PLAN-REVIEW-END===
```

The sentinels matter: several tools in this panel print tool-call traces and progress chatter to
stdout, and the collator uses the sentinels to extract your actual review from that noise. Emit
them verbatim, with your entire review in between.

Do not hedge everything into oblivion — commit to a verdict and a severity. If you're unsure, say
what you'd need to check to be sure. Your value to the panel is specific, falsifiable claims about
whether this plan will work.
