---
type: Concept
title: Least fixed point
description: "Why the package is called tarski: by Knaster-Tarski a monotone operator on sets has a least fixed point, and that is exactly what a Datalog program means; evaluation computes it semi-naively."
tags: [datalog, semantics, fixed-point, termination, knaster-tarski]
timestamp: 2026-10-26T00:00:00Z
---

# The meaning of a program

A relation is a set of tuples. A rule is set algebra: a join (intersection on shared columns), a projection (drop columns), a union (two rules with one head) and, under stratification, a difference (`not`). The rules together form an operator on sets of facts. It is monotone: more input facts never remove a derived fact, within a stratum.

By the Knaster-Tarski theorem a monotone operator on a complete lattice has a least fixed point. That least fixed point is what a Datalog program means: the smallest set of facts closed under the rules and containing the base. The engine computes it, and the package is named for Alfred Tarski. See [README](../../README.md).

# How it is computed

`Evaluate` runs the plan stratum by stratum, semi-naive within each: after the first round only the tuples new in the last round are joined again. See [stratified negation](/concepts/stratified-negation.md) for the strata.

# Why it ends

There are no function symbols, and nothing in a rule head makes a new value. A builtin is a guard that binds and derives nothing ([consumer builtins](/concepts/consumer-builtins.md)). An aggregate's value is taken over complete lower relations and is one value per group key ([aggregates](/concepts/aggregates.md)). So every stratum's active domain is the constants of the base and the program plus the finite set of aggregate values, and the fixpoint is reached. This is invariant I2 of the aggregates design and the termination clause of [the admission rule](/concepts/admission-rule.md).

# Why sets matter for the receipts

Sets have no order, so facts can arrive in any order and the answer and its hash are the same. Sets have no duplicates, so a fact stated twice changes nothing. The proof and why-not are the parts that do depend on order, which is why evaluation iterates in sorted relation order. See [proof](/concepts/proof.md).

# Related

[Admission rule](/concepts/admission-rule.md), [bounds](/concepts/bounds.md), [demand-driven evaluation](/concepts/demand-driven-evaluation.md) (which keeps the same least fixed point, filtered to a goal).

# Citations

[1] [README.md](../../README.md)
[2] [docs/design/aggregates.md, section 0 (invariants I1 to I7)](../../docs/design/aggregates.md)
[3] [docs/design/demand.md, section 0 (D1 to D11)](../../docs/design/demand.md)
[4] [doc.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/doc.go)
