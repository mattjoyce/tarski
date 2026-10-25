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
