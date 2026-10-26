---
type: Concept
title: Value and builder
description: "A relation is a builder while evaluation runs and a sealed value afterwards, never both; Plan, Base and Result hold values only, so concurrent reads are safe by construction."
tags: [concurrency, immutability, relation, design]
timestamp: 2026-10-26T00:00:00Z
---

# The cut

The seam is mutability, not where a relation came from.

| Phase | Type | Can add tuples | Indexes |
|---|---|---|---|
| During evaluation | `relBuilder` | yes, held only by the evaluator | built lazily |
| After `Evaluate` returns, or `NewBase` or `With` | `relation` | no write method | base relations: one per argument position; derived: none, scanned |

# Consequences

- **Concurrency.** A `Plan`, a `Base` and a `Result` are immutable once built and safe to share across goroutines, so any number of `Evaluate` calls may run on one plan and one base. `Proof`, `WhyNot` and `WhyEmpty` only read values. Derived relations are scanned, not indexed, since a shared Result's lazy index would race.
- **Sharing in `With`.** `Base.With` shares every relation the new facts leave untouched and rebuilds only the touched ones.
- **Answers.** `Answer.Tuples` and `Answer.Bytes` return copies, so no caller aliases a sealed relation.
- **Consumer builtins.** `Compile` copies the builtin table (each `Args` cloned), so a caller changing its map afterwards changes nothing a compiled plan does.
- **Demand.** A plan from `Plan.Demand` is a value, shareable as any plan is.

# Why

Anam's lazy index mutated shared base relations, which would race under a concurrent consumer. The first fix in Shearline was a `frozen` bit; the cut by value and builder replaced it (Shearline decision 6, recorded in Shearline's own bundle). The refactor was gated by the benchmark and kept only within noise.

# Related

[Bounds](/concepts/bounds.md), API: [NewBase and With in docs/DESIGN.md](../../docs/DESIGN.md).

# Citations

[1] [docs/DESIGN.md, Concurrency invariant](../../docs/DESIGN.md)
[2] [relation.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/relation.go)
[3] [builder.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/builder.go)
[4] [base.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/base.go)
