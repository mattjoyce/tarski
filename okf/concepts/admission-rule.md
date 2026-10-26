---
type: Concept
title: The admission rule
description: "A feature belongs in the engine only if evaluation with it is still a pure function of (plan, base), terminating, deterministic down to the answer bytes and hash, and boundable by Options."
tags: [admission, design, scope, helland, stonebraker]
timestamp: 2026-10-26T00:00:00Z
---

# The rule

A feature belongs in the engine only if evaluation, with it, is still:

1. a pure function from (plan, base) to a set,
2. terminating,
3. deterministic, down to the bytes of the canonical answer and its hash,
4. boundable by `Options`, both in bytes derived and in work done.

# What stays out, and why

| Stays out | Fails |
|---|---|
| Arithmetic in rule heads | 2: it makes new values |
| Regular expressions | 4: their cost is unbounded by the input |
| Floating-point sums in arbitrary order | 3 |
| Any state carried between evaluations | 1 |
| `avg` | needs a scale and a rounding rule, which would become frozen text in every receipt |
| Function symbols, a file format, a network face, any vocabulary of its own | the engine owns evaluation and the caller owns the vocabulary |

A string builtin such as `prefix` passes all four. So do consumer builtins, on the consumer's word that `Holds` is pure ([consumer builtins](/concepts/consumer-builtins.md)). Aggregates pass because exact decimals make `sum` order-free and every aggregated relation is complete before the aggregate runs ([aggregates](/concepts/aggregates.md)). Demand passes because it changes what is derived and never an answer ([demand-driven evaluation](/concepts/demand-driven-evaluation.md)).

# Helland and Stonebraker

The check before anything is admitted is the third mentor pair's: does the engine still fit its workload, and does the feature belong in the engine at all or in a consumer? The aggregates design took four operations and refused `avg`: nothing that needs division, ordering of results or a scale rule enters the engine. Anam's lazy `attr/3` construction is base-side pruning and stays in Anam, since who builds the base is the consumer. See [decision 7](/decisions/7-mentors.md).

# The roadmap it gates

Each step ships only when a consumer needs it: aggregates (released v0.2.0), demand-driven evaluation (built on master), and `under/3`, a prefix match that respects segment separators, when a consumer's names need it. Not planned: arithmetic, regex, function symbols, mutable state, a file format, a network face, or any vocabulary of its own.

# Related

[Least fixed point](/concepts/least-fixed-point.md), [package tarski](/packages/tarski.md).

# Citations

[1] [README.md, The admission rule and Roadmap](../../README.md)
[2] [AGENTS.md, The rules](../../AGENTS.md)
[3] [docs/design/aggregates.md, sections 0 and 1.6](../../docs/design/aggregates.md)
