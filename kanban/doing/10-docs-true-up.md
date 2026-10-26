---
id: 10
status: doing
---
# Docs true-up after the v0.3.0 luminary panel

## Job story
When a consumer engineer finds tarski cold, I want the README, godoc and DESIGN.md to state each promise once, in the present tense, with an example to paste, so I can depend on the engine from the docs alone and never read a stale copy of a contract.

## Source
`docs/reviews/luminary-docs-2026-10-26/00-collation.md` (Naur × Procida, Ousterhout × Liskov, Thompson × Feathers; 2026-10-26).

## Must-fix (gates a v0.3.1 docs release)
- [ ] C1 `example_test.go`: `ExampleEvaluate`, `ExampleResult_WhyNot`, `ExamplePlan_Demand`; README § Use points at it.
- [ ] C2 The step unit stated once in the `Options.MaxSteps` godoc (join candidates, negation probes, rule applications per round, builtin calls, aggregate groups, sum work); `Result.Cost`, `TooLarge.Steps`, `ErrTooLarge`, `Builtin`, DESIGN.md:92 point to it; error text drops "join".
- [ ] C3 One frozen-or-not table in README § What it promises (answer bytes, answer hash, RuleSet hash and ID format, constant text, Proof JSON, Absence JSON, rendered text), each frozen row naming its test; a proof-bytes golden if proofs are frozen; CHANGELOG, AGENTS, DESIGN, design records point to it.
- [ ] C4 Prune okf/index.md and the 43 dead links; then write `okf/decisions/4-v0-freeze-policy.md` first.

## Should-fix
- [ ] C5 Determinism promise says a base is a fact sequence: same set gives same answers and hashes; same sequence gives same proofs, witnesses, why-not bindings; sort facts to pin proofs.
- [ ] C6 DESIGN.md present tense, promises and invariants only; lineage to one History paragraph; internals to a maintainer-notes page; fix :382 (join is in join.go, walk in whynot_walk.go), :402 (no bundled rbac.dl), :90-94 (Options lacks Goal), :270 (decided), eval.go:53 (name and arity only).
- [ ] C7 `Plan.Demand` godoc and DESIGN § Demand, Answers: selectivity comes from bound positions; measure with Cost; under the same bounds a demand plan can be refused where the full plan answers.
- [ ] C8 "Setting the bounds" paragraph; `Cost` and `MaxSteps` godoc state determinism within a version and instability across releases; test that Cost is independent of Options.Proofs.
- [ ] C9 Panic promise scoped to consumer data; name Schema.Checks and Builtin.Check; fuzz hand-built Program values or scope the sentence.
- [ ] C10 Consumer vocabulary out of DESIGN.md and doc.go; adoption sketches (demand.md §3.4, §3.5) to how-to pages.
- [ ] doc.go brought to v0.3 (proofs, why-not, aggregates, demand, Cost); godoc stops naming unexported identifiers.
- [ ] Design docs get a status header and are frozen as dated records; one short explanation page per feature.
- [ ] aggregates.md:98 and :299 corrected (mixed-kind group omitted from the Answer; witness cap is 64 across all of a rule's aggregates).
- [ ] NOTICE:12 names 45e4a67 for anam.dl; NOTICE:13-14 says rbac_unseeded.dl is derived.
- [ ] ci.yml pins the golangci-lint installer and go-quality by SHA or tag.

## Decisions for Matt (record before v1.0)
- T3 One `Result` type for demand plus a godoc sentence, or a `GoalResult` type.
- T4 Step count as a versioned unit (minor bump on any accounting change) or as an instrument (headroom, re-measure after upgrades).

## Narrative
- 2026-10-26: card drafted from the panel collation by datalog-module.
