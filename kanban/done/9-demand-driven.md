---
id: 9
status: done
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
- 2026-10-26: Matt settled all fourteen questions on the recommendation (design §8) and asked for the build to go ahead. Slices 1 to 5 follow §6. (by datalog-module)
- 2026-10-26: slices 1 and 2 merged (Opus agent). The property test passes on 348 of 400 random recursive and negated programs, matching full evaluation filtered to the goal in bytes and hash, and it fails when the demand rules are removed. Deviations: design §1.2's counterexample stratifies under §2.2's minimal prefix, so the refusal test uses a program that really breaks; the worklist is first-come in plan order, deterministic and pinned by exact-text rewrite tests; hidden relations are left out of the Result entirely. Matt asked that no more work run on Fable; agents run on Opus. (by datalog-module)
- 2026-10-26: slices 3 and 4 merged (Opus). The fallback drops demand per relation until the rewrite stratifies, and caps adornments at 64. Every random program now answers by demand: 400 of 400, plus 300 with aggregates and 470 goal tuples, matching full evaluation in bytes, hash, proofs and why-not. Demand passes into an aggregate only through its group keys. Deviation: an aggregate's result passes no demand. Design §2.1 implied it does, but a demand rule would then leave the result unbound, which is unsound, and a mutation test proved it. Matt: Sonnet by default from here; review comes later and from other models. (by datalog-module)
- 2026-10-26: slice 5 finished by datalog-module (the agent building it died with the work uncommitted in its worktree; reclaimed, committed, master's v0.2.1 and v0.2.2 merged in, one test expectation moved: since v0.2.1 an empty base costs one step per rule application). `Result.Cost`, the Shearline cliff test and benchmark over `testdata/rbac.dl` and its unseeded twin, the Anam benchmark. Evidence in design §9: Shearline met (demand within 1.03× of the hand-seeded steps, the cliff reproduced); Anam partly met (wide demand costs what hand-bounded demand costs and 28× less than wide full, but 8× the steps of hand-bounded full: with every goal position open the demand relations for displaced and before are as large as the relations); hot loop within noise. CHANGELOG under Unreleased, README roadmap, DESIGN. **v0.3.0 tag held** for Matt's call on §9.2's options (a) ship as is, (b) restricted passing strategy, (c) supplementary magic sets. (by datalog-module)
- 2026-10-26: Matt chose option (a): v0.3.0 tagged as is; (b) restricted passing strategy and (c) supplementary magic sets stay as follow-ups if a consumer needs them. Card closed. (by datalog-module)
