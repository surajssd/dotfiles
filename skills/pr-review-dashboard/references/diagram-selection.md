# Diagram selection for PR reviews

Choose a diagram only when it answers a reviewer question faster than prose or a small table. Prefer a few narrow, evidence-backed views over a quota or one overloaded picture. Put source locations in the caption and label inferred or external behavior as such.

| Reviewer question | Best view | Use when |
|---|---|---|
| What systems or modules are affected? | Component/package diagram | Boundaries, dependencies, or ownership change across three or more components. |
| What happens in time? | UML sequence diagram | A request, reconcile, RPC, event, retry, or callback chain changes. Include the important failure or timeout branch. |
| What decisions and branches exist? | UML activity diagram | Business logic, validation, rollout, migration, or recovery has meaningful branching. |
| Which states and transitions are legal? | UML state diagram | Lifecycle, status, feature-flag phase, protocol, or migration transitions change. Show invalid and rollback transitions when relevant. |
| How do types relate? | UML class diagram | Interfaces, inheritance, composition, public types, or ownership relationships change. Do not diagram incidental structs. |
| Where does software run? | UML deployment diagram | Processes, clusters, zones, sidecars, stores, or network placement change. Mark failure domains. |
| How does data cross boundaries? | Data-flow/trust-boundary diagram | Serialization, persistence, queues, external systems, secrets, or user-controlled input changes. Mark validation and trust boundaries. |
| What changed structurally? | Before/after/diff toggle | An API, schema, module layout, or ownership model is refactored. Keep positions stable between views. |
| What is the schema contract? | Schema/ER comparison | Database, CRD, proto, event, JSON, or domain relationships change. Include cardinality, nullability, defaults, and compatibility. |
| Where should review attention go? | Change-surface or risk heatmap | A large PR spans many files or subsystems. Link hot cells to findings or evidence. |

Use HTML/CSS for compact component maps, activity flows, comparisons, heatmaps, and schema views. Use Mermaid only when automatic layout materially helps a sequence, state, class, or deployment diagram. Native inline SVG is acceptable when edge routing or spatial layout cannot be expressed clearly with the template primitives.

Do not add a diagram when the PR is a local one-function fix, the diagram would repeat the diff, relationships are unverified, or a four-row table is clearer. State that no diagram was warranted so the omission is visibly deliberate.

Every diagram must include:

- the precise question it answers;
- a legend for changed, unchanged, external, and inferred elements when those distinctions appear;
- the relevant file and line evidence;
- failure, retry, rollback, or trust boundaries when they are central to the review;
- text that remains understandable if Mermaid cannot render.
