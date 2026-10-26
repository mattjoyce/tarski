---
type: Decision
title: The v0 freeze policy
description: "Which bytes tarski freezes from the first tag, which outputs are only stable, which are an instrument, what a breaking change is, and the golden test that pins each frozen item."
tags: [freeze, golden, hash, proof, determinism, semver]
timestamp: 2026-10-26T00:00:00Z
---

# Decision

Decided by Matt on 2026-10-26, on the documentation panel's finding C3, which found the frozen set stated three different ways. This page is the one authority; [README § What it promises consumers](../../README.md) carries the table and points here. The freeze itself dates from the first tag, v0.1.0 (2026-10-25); this page records its exact extent.

# What is frozen

A frozen item does not change in any release. Consumers store these: Anam keeps receipts that hold answer hashes, and Shearline's decisions carry rule-set IDs.

| Item | Pinned by |
|---|---|
| Answer bytes (`Answer.Bytes`) | `TestGolden_AnswerBytesAndHash`, `TestGolden_AggregateAnswers` |
| Answer hash (`sha256:` over those bytes) | `TestGolden_AnswerBytesAndHash` |
| `RuleSet` hash (`sha256:` over the rule set's text) | `TestRuleSet_HashIsStillTheTextHash`, `TestGolden_RuleSetID` |
| `RuleSet` ID format (`name/version hash`) | `TestGolden_RuleSetID` |
| A constant's canonical text | `TestGolden_ConstantText` |
| Proof JSON (`Proof.MarshalJSON`) | `TestGolden_ProofAndAbsenceJSON` (proof_golden_test.go); `TestRender_JSON` also compares a literal |
| Absence JSON (`Absence.MarshalJSON`) | `TestGolden_ProofAndAbsenceJSON`; also `TestRender_JSON`, `TestWhyNot_AggregateJSONRoundTrip` |

Proof JSON and Absence JSON are frozen as formats: the keys, their meaning and their encoding. What a given proof records is a separate matter, below.

# Stable, but not frozen

These may change in a minor release, with a changelog line, and consumers should not store or compare them.

- The rendered text of `Proof.String()` and `Absence.String()`. Pinned by `TestRender_Strings` and `ExampleResult_WhyNot`, so a change is deliberate and visible.
- Error message text. Match with `errors.Is` and `errors.As`.
- Which derivation a proof records. It is deterministic for one (plan, base fact sequence, options). It is not promised across plans: a demand plan may record a different derivation than the full plan. Pinned within a plan by `TestProof_Deterministic`.

# An instrument, not a promise

Step and byte counts (`Result.Cost`, the figures in `*TooLarge`) are exact and deterministic within one version. They are not stable across releases, patch releases included, because a release may make evaluation cheaper or charge a new kind of work. A consumer sizing `MaxSteps` measures against the version it ships. See [bounds](/concepts/bounds.md).

# What a breaking change is

A change to any frozen item, or to the meaning of an exported name. It takes a minor version bump while the major is 0, a changelog entry under the version, and agreement from the consumers that import tarski (Shearline, Anam, Ductile), posted on the board before the merge. A golden literal is never edited to make a test pass; if one must move, the change is a breaking one and is made as such.

# Related

[Frozen bytes and answer hashes](/concepts/frozen-bytes-and-hashes.md), [proof](/concepts/proof.md), [why-not provenance](/concepts/why-not-provenance.md), [package tarski](/packages/tarski.md).

# Citations

[1] [README.md, What it promises consumers](../../README.md)
[2] [AGENTS.md, The rules](../../AGENTS.md)
[3] [CHANGELOG.md](../../CHANGELOG.md)
[4] [golden_test.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/golden_test.go)
[5] [render_test.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/render_test.go)
