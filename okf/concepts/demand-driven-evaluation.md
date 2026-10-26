---
type: Concept
title: Demand-driven evaluation
description: "Released in v0.3.0: given a goal, a magic-sets rewrite derives only the tuples that can reach it, with the same answer bytes and hashes as full evaluation, a per-relation fallback when stratification breaks, and demand through aggregates on group keys only."
tags: [demand, magic-sets, goal, v0.3, aggregates, stratification, cost]
timestamp: 2026-10-26T00:00:00Z
---

# Status

Released as v0.3.0 on 2026-10-26 (card 9, five slices): the internals, the rewrite, the fallback, aggregates, and the evidence. `Result.Cost() (steps, bytes int)` is in the exported API (design Q8). The design is `docs/design/demand.md`; Matt's decisions are its section 8 ([demand design record](../../docs/design/demand.md)) and the evidence its section 9: Shearline's criterion met (demand over unseeded rules within 1.03× of the hand-seeded steps, the MaxBytes cliff reproduced); Anam's partly met (wide `instant` under demand costs what the hand-bounded one does and 28× less than wide full, but about 8× the steps of hand-bounded full, since with no goal position bound the demand relations for `displaced` and `before` are as large as the relations). Matt chose to ship as is; a restricted passing strategy and supplementary magic sets stay as follow-ups.

# What problem it solves

Cost should follow the question and not the whole base. Shearline seeds every rule with the request by hand, and Anam parameterises its set with `instant/1`; both smuggle a query into the rules. The Shearline module panel found what an unseeded `holds/2` costs: every decision derived every principal's roles, and the byte bound tripped near 38,000 pairs. Hand seeding is safety by discipline; demand is safety by construction.

# The invariants

The design states them before any mechanism (Lamport and Thomas/Hunt lens). Full evaluation means the stratified least fixed point M (see [least fixed point](/concepts/least-fixed-point.md)); D is demand evaluation for a goal.

| Invariant | Claim |
|---|---|
| D1, answers | The goal relation under D is M's relation filtered to the goal's constants, byte for byte and hash for hash. |
| D2, soundness | Every tuple D derives into a visible relation is in M. |
| D3, completeness per demand | For every demanded binding, every tuple of M's relation that agrees with it is in D's. |
| D4, termination | The rewritten program is Datalog; demand tuples are projections of bindings over the active domain. |
| D5, determinism | The rewrite is a function of (plan, goal shape). |
| D6, frozen bytes | No answer, hash, rule-set hash, constant text, proof or absence shape moves. |
| D7, hidden helpers | No exported surface shows a demand relation. |
| D8, stratification | The rewritten program is stratified, or demand is dropped relation by relation until it is. |
| D9, aggregates | Demand enters an aggregate's braces only through its group variables. |
| D10, bounds | `MaxBytes` and `MaxSteps` charge the rewritten evaluation, helper tuples included. |
| D11, proofs and why-not, weaker on purpose | A valid, deterministic derivation, not promised to be full evaluation's first. |

# The technique

Non-subsumptive magic sets: one hidden demand relation per (relation, adornment), and each source rule copied once per head adornment with a leading demand literal, every copy writing into the source relation under its own name. So `Result`, proofs and why-not see source names. An adornment is a string of `b` and `f` per position. A worklist in sorted order adorns each positive derived atom by which arguments are bound at that point in the rule's evaluation order. The all-free adornment means the whole relation and has no demand relation. The seed is the one tuple `Evaluate` adds before the first stratum.

Considered and rejected: relevance plus constant propagation (it cannot express set-valued demand, as in Anam's `in_force`), and full magic sets with supplementary predicates and subsumption (too many hidden relations for twenty-rule programs). The line the design names not to cross: demand restricts what is derived. It never reorders a body or adds a cost model, because a query optimiser invites the next three asks (Stonebraker's side of the Helland and Stonebraker tension).

# Fallback

Magic rewriting can break stratification even when every demand rule is positive. Where the rewrite would, demand is dropped for one relation and the rewrite runs again until it stratifies: in the first strongly connected component holding a negative edge, the source relation of the first demand relation by sorted name. A relation reached under more than `maxAdornments` = 64 binding patterns has its demand dropped the same way. A dropped relation is derived whole wherever it is reached, the goal's own included (it then has no seed), and the relations below it keep their demand. The fallback never changes an answer, it ends (each round drops a relation not dropped before, and the source program stratifies), and it is a function of the plan and the goal shape.

# Aggregates

Demand enters an aggregate's braces only through its group variables, then what earlier inner atoms bind, never a local from outside, so every group the goal needs is computed whole and its value is full evaluation's. Copies of an aggregate rule share its aggregate plans, so a group's memoised value and witnesses are one per evaluation. An aggregate's result passes no demand: a literal after the aggregate that reads the result is adorned open at that position. The design first implied it did; a demand rule would then leave the result unbound, which is unsound, and a mutation test proved it. Sound and complete, only less selective. See [aggregates](/concepts/aggregates.md).

# What a consumer sees

- `Plan.Demand(relation, adornment)` compiles a plan for one shape; `Options.Goal` carries each request's constants; `Result.Goal()` reports the goal. See [Demand and Goal in docs/DESIGN.md](../../docs/DESIGN.md).
- `Relation(goal relation)` is the filtered answer. A base relation is the base's. Any other derived relation is `ErrInvalid` with a message saying how to read it (Q2): the result is partial for them and the type is not split.
- Proofs and why-not only for the goal relation and tuples that agree with the goal's constants; `WhyEmpty` only when the goal leaves every position open.
- A goal that repeats a variable other than `_` is refused (Q14): an adornment cannot express equality.

# Evidence at v0.3.0

The property test compares demand with full evaluation on random recursive and negated programs, and fails when the demand rules are removed. After slices 3 and 4 every random program answers by demand (400 of 400, plus 300 with aggregates and 470 goal tuples), matching full evaluation in bytes, hash, proofs and why-not. `FuzzDemandMatchesFull` runs in CI. `TestDemand_CostFollowsTheGoal` shows the cost following the goal. The two consumer benchmarks (`BenchmarkShearline_Allow` over `testdata/rbac.dl` and its unseeded twin at 50,000 principals; `BenchmarkAnam_Regimen` over `testdata/anam.dl` at 900 and 6,000 entries, the design's 20,000 being infeasible for full evaluation) and the cliff test landed in slice 5; their numbers are design section 9.

# Consumers

Shearline keeps its seeds (Q10) and is told that proofs and why-not witnesses under demand may differ from full evaluation's first derivation. Anam decides its own `instant/1` (Q11).

# Related

[Bounds](/concepts/bounds.md), [stratified negation](/concepts/stratified-negation.md), [demand design record](../../docs/design/demand.md), [package tarski](/packages/tarski.md).

# Citations

[1] [docs/design/demand.md](../../docs/design/demand.md)
[2] [docs/design/demand.md, section 8, decided with Matt](../../docs/design/demand.md)
[3] [Card 9, demand-driven evaluation](../../kanban/done/9-demand-driven.md)
[4] [docs/DESIGN.md, Demand-driven evaluation](../../docs/DESIGN.md)
[5] [demand.go](../../demand.go)
[6] [demand_rewrite.go](../../demand_rewrite.go)
[7] [demand_fallback.go](../../demand_fallback.go)
