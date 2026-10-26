---
type: Concept
title: Proof
description: "A proof is the first derivation of a tuple, shared as a DAG of values, deterministic, and naming its rule by set, line and text; an aggregate tuple's proof carries the group it was taken over."
tags: [proof, determinism, receipt, provenance, aggregates]
timestamp: 2026-10-26T00:00:00Z
---

# What a proof is

`Proof{Relation, Tuple, Rule, Body, Group, GroupSize}` is how one tuple came to be.

| Case | `Rule` | `Body` |
|---|---|---|
| Base fact | nil | empty |
| Derived tuple | the first rule that produced it | one `Proof` per positive body atom, in the rule's written order |

Builtins, comparisons and negations contribute no body entry. A proof is recorded only when `Options.Proofs` is on, because it touches the hot loop. `Result.Proof(name, t)` has `ok` false when the tuple is not in the result or proofs were off.

# First derivation and determinism

A tuple may have many derivations; the proof keeps the first, in evaluation order. That is stable only because evaluation is deterministic: for one (plan, base, options) every evaluation yields the same proof for every tuple. A base is its fact sequence, so a consumer that wants a proof to be a function of its content sorts its standing facts (Shearline's `policy.Load` does).

# A DAG of values

`Result.Proof` builds each (relation, tuple) proof once and reuses that value wherever it appears in a body, so the size is linear in the derivation records. The structure is finite and acyclic: a body tuple was always derived strictly before its head. The text form says "(shown above)" for a repeat; the JSON writes a later occurrence as `{relation, tuple, rule: null, shown: true}`.

# Naming its rule

`Rule` is a deep copy the holder may keep or change; the plan is not reachable through a proof. It carries `Set` ("name/version" of the rule set `Compile` took it from, empty for a bare Program), `Line` and `Text` (the rule as written). See [rule-set identity](/concepts/rule-set-identity.md).

# Aggregate groups (v0.2)

For a tuple an aggregate derived, `Group` holds one entry per witness kept, each the inner positive atoms' proofs in written order, and `GroupSize` the true number of witnesses. `maxProofGroup` = 64 witnesses are kept, in witness order, and only when `Proofs` is on; a cut `Group` still states the count. The text prints `group of 1024 (64 shown)`. Both fields are zero for every tuple no aggregate derived and `omitempty` in JSON, so a proof without aggregates writes the bytes it wrote in v0.1. With several aggregates in a rule, witnesses are concatenated in literal order and `GroupSize` sums them. Witness order follows the inner join's relation order, which follows the base's fact order; the answer and its hash do not depend on it, but a proof pinned across a re-ingested base may. See [aggregates](/concepts/aggregates.md).

# Under demand

A proof from a demand evaluation is a valid, deterministic derivation by a source rule, but it is not promised to be the one full evaluation records first, because first is an artefact of evaluation order. See [demand-driven evaluation](/concepts/demand-driven-evaluation.md).

# Rendering

`String()` is an indented derivation for a log or an operator. `MarshalJSON` emits relation, tuple, rule as `{set, line, text}`, and body or group recursively, never the parser's AST.

# Related

[Why-not provenance](/concepts/why-not-provenance.md), API: [Proof in docs/DESIGN.md](../../docs/DESIGN.md).

# Citations

[1] [docs/DESIGN.md, Proof](../../docs/DESIGN.md)
[2] [docs/design/aggregates.md, section 5](../../docs/design/aggregates.md)
[3] [proof.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/proof.go)
[4] [render.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/render.go)
