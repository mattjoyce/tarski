---
type: Go Package
title: Package tarski
description: "A small deterministic Datalog engine for Go: facts in, derived facts out, every answer hashed, every derivation checkable; it owns evaluation and no vocabulary."
resource: github.com/mattjoyce/tarski
tags: [go, engine, datalog, tarski]
timestamp: 2026-10-26T00:00:00Z
---

# What it owns

Evaluation. The caller owns the vocabulary. Nothing here reads a file or the network, keeps state between calls, or knows what a relation means: `role`, `entry` and `grants` are strings to it. Pre-1.0. Go 1.25 or later, standard library only. Licence Apache 2.0. Error messages are prefixed `tarski:` (they were `datalog:` in Shearline and `query:` in Anam).

The engine is a function from a plan (compiled rules) and a base (a set of facts) to derived relations. On request it also returns a proof for a derived tuple and a why-not for an absent one.

| Area | Owns |
|---|---|
| Language | `Parse`, `Program`, `Rule`, `Atom`, `Literal`, `Comparison`, `Term`, `Aggregate` |
| Validation | `Compile` against a `Schema`: arities, safety, stratification, schema checks, builtin declarations |
| Data | `Const`, `Tuple`, `Fact`, `Base` (`NewBase`, `With`, `Len`) |
| Evaluation | `Evaluate`, `Options`, `Result`, `Answer` |
| Provenance | `Proof`, `Result.Proof`, `Absence`, `RuleFailure`, `Result.WhyNot`, `Result.WhyEmpty` |
| Identity | `RuleSet`, `NewRuleSet`, `ID`, `Verify` |
| Errors | `ErrSyntax`, `ErrInvalid`, `ErrTooLarge`, `*TooLarge` |
| Demand (v0.3.0) | `Goal`, `Plan.Demand`, `Options.Goal`, `Result.Goal`, `Result.Cost` |

# What it promises consumers

These hold from the first tag. Breaking any of them takes a minor version bump and a changelog entry, and three consumers have to agree. See [decision 4](/decisions/4-v0-freeze-policy.md).

- **Answer bytes are frozen.** See [frozen bytes and answer hashes](/concepts/frozen-bytes-and-hashes.md).
- **Same inputs, same outputs.** One (plan, base, options) gives the same answers, hashes, proofs and why-nots on every run.
- **Values, not places.** `Plan`, `Base` and `Result` are immutable once built. See [value and builder](/concepts/value-and-builder.md).
- **Fail closed and bounded.** A crossed bound is `*TooLarge`, never a partial answer. See [bounds](/concepts/bounds.md).
- **Receipts as data.** `Proof`, `Absence` and `RuleFailure` are exported structs that render as indented text and marshal to compact JSON.
- **Terminating.** No function symbols, and nothing a builtin or aggregate does makes a new value outside what the data holds. See [the admission rule](/concepts/admission-rule.md).

# Who consumes it

| Consumer | Use |
|---|---|
| Shearline | The policy layer: one decision per request over a few hundred standing facts, with proofs and why-not as the receipt. Refuses rule sets that use aggregates. See [Shearline policy](/examples/shearline-policy.md). |
| Anam | The query: one evaluation per question over a ledger. Its unmodified `rules/anam.dl` is a test here. Needs aggregates, and later RBAC via Shearline. See [Anam query](/examples/anam-query.md). |
| Ductile | Config-time checks. Ductile's scopes were modelled as a Shearline policy, which runs on this engine. |

# The diamond dependency

Anam and Ductile import Shearline as well as tarski, and Shearline imports tarski. Go resolves that diamond to one tarski version, with Shearline's `go.mod` setting the floor. Two consequences follow, both in `AGENTS.md`.

- Every release must pass Shearline's policy suite, and Anam's `rules/anam.dl` test (`anam_test.go`) must stay green.
- A consumer taking a new tarski moves the others with it. The v0.2.0 changelog says so: Anam adopting v0.2 moves Shearline too, which is why Shearline had to refuse aggregates in `policy.Load` before the tag.
- The exported types (`Fact`, `RuleSet`, `Const`) are shared currency between consumers, so every change is a boundary change. See [decision 7](/decisions/7-mentors.md).

# Lineage

Cut on 2026-10-25 from Shearline's `datalog` package (Shearline commit c4f75ab), which had been cut from Anam's `internal/query`. History was carried both times. Exact numbers and the builtin table were ported from Anam (origin/master 45e4a67). The NOTICE file records this.

# Files

`parse.go`, `lex.go`, `shape.go` (language), `plan.go`, `stratify.go` (compile), `base.go`, `relation.go`, `builder.go` (data), `eval.go`, `join.go`, `bounds.go`, `builtins.go` (evaluation), `number.go`, `sum.go` (exact numbers), `aggregate*.go` (aggregates), `proof.go`, `whynot*.go`, `render.go` (provenance), `answer.go`, `bundled.go` (rule sets), `demand*.go` (demand, on master), `term.go` (constants).

# Related

[Engineering standards](/standards/engineering.md), [the admission rule](/concepts/admission-rule.md), [least fixed point](/concepts/least-fixed-point.md).

# Citations

[1] [README.md](../../README.md)
[2] [AGENTS.md, Consumers](../../AGENTS.md)
[3] [NOTICE](../../NOTICE)
[4] [docs/DESIGN.md](../../docs/DESIGN.md)
[5] [CHANGELOG.md](../../CHANGELOG.md)
[6] [doc.go at v0.2.2](https://github.com/mattjoyce/tarski/blob/v0.2.2/doc.go)
