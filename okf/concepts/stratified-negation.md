---
type: Concept
title: Stratified negation
description: "What the language is and is not: rules, comparisons, negation and aggregates stratified into strictly ordered layers, builtins as guards, and declared-but-undefined relations that evaluate empty."
tags: [datalog, language, negation, stratification, builtins, termination]
timestamp: 2026-10-26T00:00:00Z
---

# The language

Rules, facts, comparisons (`=`, `!=`, `<`, `<=`, `>`, `>=`), negation, aggregates and relation declarations. A relation name is a lower-case letter then letters, digits and underscores. A variable is an upper-case letter or underscore then the same. `_` is anonymous. Constants are double-quoted strings, numbers, `true` and `false`.

```
% a comment runs to the end of the line
name/arity.                                    declares a relation another program defines
head(X, "c", 1).                               a fact
head(X, Y) :- rel(X, Z), not other(Z), Y > 3.  a rule
n(M, N) :- month(M), N = count : { reading(_, At), prefix(M, At) }.   an aggregate (v0.2)
```

# What it is not

- No function symbols and no arithmetic in rule heads. Nothing makes a value the data did not hold, which is the termination argument ([least fixed point](/concepts/least-fixed-point.md)).
- No regular expressions, no state between evaluations, no floating-point sums. See [the admission rule](/concepts/admission-rule.md).
- No attribute-style payloads. Facts are tuples of scalars; a consumer flattens its payloads into relations such as Anam's `attr/3`.
- No `avg`. It is refused with a pointer to `sum` and `count`. See [aggregates](/concepts/aggregates.md).

# Safety and stratification

`Compile` refuses, before anything is read, with a message that says what to change:

| Problem | Result |
|---|---|
| Unsafe variable (in the head, a negated atom, a comparison or a builtin, not bound by a positive body atom) | `ErrInvalid` |
| Unknown relation, wrong arity | `ErrInvalid` |
| `_` in a head | `ErrInvalid` |
| A rule whose head names a base relation | `ErrInvalid` |
| A builtin as a head, declaration or schema entry | `ErrInvalid` |
| Negation that cannot be stratified (a cycle through `not`) | `ErrInvalid` |
| A cycle through an aggregate, with a message saying where | `ErrInvalid` |
| An atom a schema check refuses | the check's error, wrapped |

Every atom inside an aggregate's braces is a negative edge, so the aggregated relation lands in a strictly lower stratum and is complete before the aggregate is taken. The rule may still be recursive through its outer body.

# Builtins as guards

A builtin is a relation answered in Go, a guard like a comparison. Every argument must be bound by a positive body atom (`_` is refused), it binds nothing, derives nothing, and never appears in a head, a declaration or `Schema.Arities`. The engine has three, over strings, pattern first and subject second; a non-string argument makes the guard false.

| Atom | True when |
|---|---|
| `prefix(P, S)` | `strings.HasPrefix(S, P)` |
| `suffix(P, S)` | `strings.HasSuffix(S, P)` |
| `contains(P, S)` | `strings.Contains(S, P)` |

A consumer adds its own: [consumer builtins](/concepts/consumer-builtins.md).

# Declared and undefined is empty

A relation a program declares and nothing defines evaluates as the empty relation. That is how a rule set leaves a hook open for a consumer: Anam's `anam.dl` declares `instant/1` and the query defines it. Programs given to `Compile` are read as one, so a later program defines what an earlier one declared. The caller that reads such a relation checks both that it has an arity and that something defines it (`Plan.Arity`, `Plan.Defines`).

# Related

[Aggregates](/concepts/aggregates.md), [demand-driven evaluation](/concepts/demand-driven-evaluation.md) (which must preserve stratification or fall back), API: [Parse and Compile in docs/DESIGN.md](../../docs/DESIGN.md).

# Citations

[1] [docs/DESIGN.md, Built-ins and Evaluation invariants](../../docs/DESIGN.md)
[2] [docs/design/aggregates.md, section 3](../../docs/design/aggregates.md)
[3] [testdata/anam.dl](../../testdata/anam.dl)
[4] [plan.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/plan.go)
[5] [stratify.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/stratify.go)
