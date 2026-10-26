---
id: 9
status: doing
---
# Demand-driven evaluation (v0.3): derive only what can reach the goal

## Job story
When Anam asks a question that touches a sliver of the ledger, or Shearline decides one request, I want the engine to derive only the tuples that can reach the answer, so cost follows the question and not the whole base, and rule sets are safe by construction rather than by hand-seeding (rbac.dl's request seeding, Anam's instant/1). (Drafted by datalog-module for Matt; shearline docs/ENGINE.md § Direction of travel 3, Anam's brief.)

## Constraints
- Semantics unchanged: every existing answer, hash and golden test holds; demand evaluation is substitutable for full evaluation (Liskov).
- Stratified negation and aggregates: demand passes only on grouping keys and never trims a group; the rewrite must preserve stratification or fall back per stratum.
- Proofs and why-not hide the rewrite's helper relations; why-not needs the goal inside the demand set.
- Opt-in through Options or a goal argument; additive API only.

## Plan
1. Design doc `docs/design/demand.md` (fable rung; Lamport × Thomas/Hunt and Helland × Stonebraker passes). Design only until Matt reviews it.

## Narrative
- 2026-10-26: design written to `docs/design/demand.md` (fable rung). Recommends non-subsumptive magic sets, one hidden demand relation per (relation, adornment) writing into the source relation, so names in Result, proofs and why-not stay the source's. On a stratification break the rewrite drops demand per relation and re-stratifies. Additive API: `Goal`, `Plan.Demand`, `Options.Goal`, `Result.Goal`. Fourteen questions for Matt in its § Open questions. No code until he reviews it. (by datalog-module)
