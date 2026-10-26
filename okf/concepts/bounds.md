---
type: Concept
title: Bounds
description: "Evaluation is bounded by a context deadline, MaxBytes and MaxSteps; a crossed bound is *TooLarge, never a partial answer; steps are charged per join candidate, rule application, builtin call and aggregate group."
tags: [bounds, maxbytes, maxsteps, deadline, toolarge, fail-closed]
timestamp: 2026-10-26T00:00:00Z
---

# Three bounds

| Bound | Where | Exactness |
|---|---|---|
| Deadline | the context passed to `Evaluate` | Checked on entry and every `stepsPerDeadlineCheck` = 1024 steps. A context already done fails at once, before any join. |
| `Options.MaxBytes` | over every derived tuple's JSON text | Approximate: it charges unescaped lengths. 0 is no bound. |
| `Options.MaxSteps` | the work to derive | Exact: each charge below is a step. 0 is no bound. |

`MaxBytes` bounds what is derived; `MaxSteps` bounds the work. A rule that joins a wide product and derives only duplicates is charged no bytes, so without `MaxSteps` its termination would rest on the caller's deadline alone. Neither bound can be crossed with a nil error: a guard runs after the last stratum so the final tuple cannot slip past.

# What a step is

Steps rose twice in the pressure review's wake, so the current rule is worth stating exactly.

| Charge | Since |
|---|---|
| Every candidate visited by a join or a negation probe | v0.1.0 (carried from Shearline) |
| Every aggregate inner candidate, plus one step for each group computed | v0.2.0 |
| A sum's fold, priced by `sumWork` and charged before it runs | v0.2.0 (second security review, see sum security reviews) |
| Every rule application | v0.2.1 |
| Every builtin call, engine or consumer's | v0.2.2 |

Comparisons stay free. The effect of the last two: a rule with no positive atom, called a guard-only rule, used to run its guards with no join step charged, and a body of 1000 builtin calls ran all of them under `MaxSteps: 1`. Now `MaxSteps` bounds how often any `Holds` runs, whatever the rule bodies hold, which is what the README promises consumers of [consumer builtins](/concepts/consumer-builtins.md). Step counts rise by one per rule application and per builtin call; no answer changes. See the pressure review.

# Aggregates and memory

An aggregate's memo, and with `Proofs` its kept witnesses, grow with the groups computed. A group that derives no tuple is charged no bytes. So `MaxSteps`, not `MaxBytes`, is what bounds an aggregate's memory, and a consumer whose rules use aggregates should set it. The luminary audit added this to the docs (luminary audit).

# The third bound: numbers

A sum wider than `maxSumDigits` = 1 << 16 aligned decimal places, or whose text would pass `maxExponent`, is `*TooLarge` with `Bound` = `BoundNumber`. No option moves it; it is an invariant with a reason: it refuses the sum before the allocation, so the bound cannot be crossed on the way to being checked. The check is order-free: it looks at the whole set's width up front and the exponent on the result only, so the refusal is a function of the set and not of fold order.

# The refusal

`*TooLarge{Bound, Relation, Tuples, Bytes, MaxBytes, Steps, MaxSteps}` `Is(ErrTooLarge)`. `Bound` is `BoundBytes`, `BoundSteps` or `BoundNumber`. `Relation` is the one being derived. The message starts `tarski:` and names the remedy. Asking again unchanged fails again. v0.2.0 added an unexported field to `TooLarge`, so an unkeyed `TooLarge{...}` literal in consumer code no longer compiles.

# Under demand

Demand relations' tuples, the goal's seed included, are charged to `MaxBytes` and their joins to `MaxSteps`. A hidden demand relation is never named in a `*TooLarge`. See [demand-driven evaluation](/concepts/demand-driven-evaluation.md).

# Why-not has its own budget

The why-not walk is bounded by `whyNotSteps` = 1 << 16 candidates and `whyNotDepth` = 8. See [why-not provenance](/concepts/why-not-provenance.md).

# Related

[Package tarski](/packages/tarski.md), API: [Evaluate and Options in docs/DESIGN.md](../../docs/DESIGN.md).

# Citations

[1] [docs/DESIGN.md, Options and Evaluation invariants](../../docs/DESIGN.md)
[2] [CHANGELOG.md, v0.2.0 to v0.2.2](../../CHANGELOG.md)
[3] [README.md, What it promises consumers](../../README.md)
[4] [bounds.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/bounds.go)
[5] [sum.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/sum.go)
[6] [join.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/join.go)
