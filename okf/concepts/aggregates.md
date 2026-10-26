---
type: Concept
title: Aggregates
description: "count, sum, min and max over a grouped body in Soufflé's syntax, stratified like negation; witnesses are the distinct solutions of the inner body with each underscore fresh; empty groups, mixed kinds and the Soufflé deltas as decided."
tags: [aggregates, count, sum, min, max, souffle, stratification, v0.2]
timestamp: 2026-10-26T00:00:00Z
---

# Shape

An aggregate is a body literal in a rule: `N = count : { body }`, `S = sum X : { body }`, `M = min X : { body }`, `M = max X : { body }`.

```
n(M, N)     :- month(M), N = count : { reading(E, At), prefix(M, At) }.
spent(M, S) :- month(M), S = sum V : { purchase(E, At, V), prefix(M, At) }.
last(M, At) :- month(M), At = max T : { reading(_, T), prefix(M, T) }.
busy(M)     :- n(M, N), N > 3.
```

- `avg` and `mean` are refused with "derive count and sum and divide in the caller". Exact division does not terminate (1/3), and a scale and rounding rule would become frozen text in every receipt.
- The operation words are not reserved: a relation may still be named `count`.
- An aggregate cannot nest, cannot be negated, and `count X` is a syntax error. The inner body is a comma list of positive atoms, negations, comparisons and builtins. Empty braces are a syntax error.
- The AST gains one additive field, `Literal.Aggregate` (nil on every other literal), and the type `Aggregate`.

# Groups

A variable of the braces that the rest of the rule also uses is a **group variable**; it must be bound by a positive atom outside the braces. A variable used only inside is a **local** variable of the aggregate. The result variable must not appear inside the braces. A head variable that occurs only inside the braces is unsafe and refused.

# Witnesses: decided semantics

A group's input is the set of **distinct solutions of the inner body, with every `_` a fresh variable**. So `count : { r(G, _) }` counts the tuples of `r` with first argument G, and `sum V : { e(E), a(E, V) }` adds V once per (E, V) witness even when two entries share an amount. A body holding only guards (`N = count : { G != "a" }`) has one empty witness when the guards hold, so the count is 1 or 0.

# Values

| Case | Value |
|---|---|
| `count` of an empty group | 0 |
| `sum` of an empty group | 0 |
| `min` or `max` of an empty group | no tuple derived |
| Group mixes kinds under `min`/`max`, or `sum` meets a non-number | no tuple for that group; why-not names the constant |
| `min`/`max` ordering | numbers by exact value, strings lexically, false before true; one kind per group |
| `sum` | exact decimal, order-free |
| Result already bound before the aggregate | an equality test, as `X = 3` is |

"How many readings in June" answers 0 only when something in the outer body says June exists: the engine has no calendar and cannot invent the month.

# Stratification

Every atom inside the braces is a negative edge, so the aggregated relations are complete before the aggregate is taken and a cycle through an aggregate is refused at `Compile` with a message saying where. The outer body may still be recursive. Several aggregates in a rule are independent; a group variable may not be another aggregate's Result (write the second aggregate in a rule of its own).

# Bounds

An aggregate's inner candidates are steps, one step is charged per group computed, and a sum's fold is priced by `sumWork` and charged before it runs. A sum wider than `maxSumDigits` places, or whose text would pass `maxExponent`, is `*TooLarge` with `BoundNumber`. A group that derives no tuple is charged no bytes, so set `MaxSteps` whenever rules use aggregates: it bounds the aggregate's memory as well as its work. See [bounds](/concepts/bounds.md).

# Proof and why-not

A proof of an aggregate tuple carries the group it was taken over, 64 witnesses kept and `GroupSize` the true count ([proof](/concepts/proof.md)). Why-not evaluates the aggregate over the result and reports its value or reason ([why-not provenance](/concepts/why-not-provenance.md)).

# Against Soufflé 2.5

Aggregate syntax is Soufflé's, so Anam's `scripts/souffle-diff.py` oracle can carry the same text on both sides. The claims were run on Soufflé 2.5 (design section 12). They match for empty groups (count and sum 0, min and max none), a fact stated twice (counted once) and `count : { r(_, _) }` (each `_` fresh). They differ in these ways, which the oracle must skip:

| Delta | Soufflé 2.5 | tarski |
|---|---|---|
| `min` or `max` over strings | refused by the type checker | allowed, lexical |
| `mean` / `avg` | `mean` exists, `avg` is a syntax error | both refused |
| `sum` over floats | floats in arbitrary order | exact |
| Multi-literal body holding `_` | `_` projected away before counting | `_` fresh |

The last row (design 12.1), over `t(1). t(2).` and `s(1,10). s(1,20). s(2,10). s(3,99). s(7,5).`:

| Body | Soufflé 2.5 | tarski |
|---|---|---|
| `count : { s(X, _) }` | 5 | 5 |
| `count : { t(X), s(X, _) }` | 2 (distinct named variables) | 3 |
| `sum X : { t(X), s(X, _) }` | 3 | 4 |
| `count : { t(X), s(X, Y) }` | 3 | 3 |

Soufflé counts every tuple for a single-atom body but projects `_` away for a multi-literal one. tarski keeps one rule for both. Named-variable witnesses everywhere would make Anam's "how many readings in June" count distinct instants rather than readings. To get Soufflé's projection in tarski, name it as a rule: `w(X) :- t(X), s(X, _).` then `count : { w(X) }`. Matt settled this on tarski's reading ([decision 5](/decisions/5-aggregate-semantics.md)).

# Consumers

- **Shearline** never uses aggregates: a decision is a tuple, not a count. `policy.Load` refuses a plan with `HasAggregates()`, and any code walking rule bodies must know a `Literal` may now be an `Aggregate`.
- **Anam** needs them first; its counting question is a test (`TestAnamRuleSet_CountsJuneReadings`). See [Anam query](/examples/anam-query.md).

# Forward compatibility with demand

The inner body is a closed unit whose free variables are exactly the group variables, and the group memo is keyed on the group key alone. That is what lets demand pass only on group keys and never trim a group ([demand-driven evaluation](/concepts/demand-driven-evaluation.md)).

# Related

API: [Aggregate and HasAggregates](/api/aggregate.md), [decision 5](/decisions/5-aggregate-semantics.md).

# Citations

[1] [docs/design/aggregates.md](../../docs/design/aggregates.md)
[2] [docs/design/aggregates.md, section 12 and 12.1](../../docs/design/aggregates.md)
[3] [docs/design/aggregates.md, section 13](../../docs/design/aggregates.md)
[4] [docs/DESIGN.md, Aggregates](../../docs/DESIGN.md)
[5] [Card 6, aggregates](../../kanban/doing/6-aggregates.md)
[6] [aggregate_eval.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/aggregate_eval.go)
[7] [anam_test.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/anam_test.go)
