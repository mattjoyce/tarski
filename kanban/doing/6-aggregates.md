---
id: 6
status: doing
---
# Aggregates: count, sum, min, max over a grouped body (v0.2)

## Job story
When Anam asks how many readings fell in June or what the month's spend totals, I want to write it as one rule with an aggregate, so the count comes from the facts with a receipt instead of being worked out by the caller. (Drafted by datalog-module for Matt; Anam card 213, shearline docs/ENGINE.md § Direction of travel 2.)

## Constraints (from Matt and the consumers)
- Stratified like negation: an aggregate over a relation that reaches itself is refused at Compile.
- Deterministic: exact decimals make sum order-independent; the answer bytes and hash are a function of (plan, base).
- Charged to the bounds like any derived relation.
- Syntax close to Soufflé's (`N = count : { ... }`), so Anam's scripts/souffle-diff.py can check answers.
- Proof of an aggregate tuple is the group it was taken over. Why-not explains an empty or absent group.
- Shearline's policy never uses them, and its Load may refuse a rule set that does.
- Must stay compatible with demand-driven evaluation later: demand passes only grouping keys and never trims a group.
- avg: decide (defer, or a named scale and rounding rule).

## Plan
1. Design doc `docs/design/aggregates.md`: invariants first (Lamport × Thomas/Hunt Before pass), admission check (Helland × Stonebraker).
2. Implement on a branch, gate green, golden tests unchanged.

## Narrative
- 2026-10-25: design written to `docs/design/aggregates.md` (fable rung, Lamport × Thomas/Hunt and Helland × Stonebraker passes). Eleven sections, five slices. Its §11 lists ten questions for Matt. Matt was away and asked for work to continue, so the build proceeds on the design's recommendation for each, recorded here as assumptions. v0.2.0 is not tagged until Matt has reviewed them; any can change before the tag.
  - Witness semantics: Soufflé's distinct solutions of the inner body.
  - Mixed kinds in a group: no tuple, with why-not naming the constant.
  - Empty groups: count and sum give 0; min and max derive nothing.
  - Proof group capped at 64 witnesses, with GroupSize.
  - Sum overflow: `*TooLarge` with a new `BoundNumber`.
  - min/max over strings allowed. avg deferred, refused with a message. `Plan.HasAggregates` only. Several aggregates per rule allowed. Slice 2 lands the syntax inert.
  - Soufflé claims marked in §7 are to be checked against a real Soufflé before slice 3. (by datalog-module)
- 2026-10-25: slice 1 (exact sum) and slice 2 (inert syntax) merged. Slice 3 (compile, stratify, evaluate) built. Before merging it, a differential against Soufflé 2.5 found Soufflé counts multi-literal aggregate bodies over distinct named variables, while single-atom bodies count every tuple. tarski counts distinct solutions with `_` fresh everywhere. Recorded in design §12.1. Question 1 stays open for Matt, with evidence. The commit security review found a sum's fold was not charged to MaxSteps; fixed (sumWork, stepN) with a test that fails without it. Slice 3 deviated from my brief in one place, in the design's favour: a result already bound before its aggregate is an equality test, not a refusal. (by datalog-module)
