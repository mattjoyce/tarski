---
type: Concept
title: Frozen bytes and answer hashes
description: "An Answer serialises one way and its hash is sha256 over those bytes; a RuleSet hash is sha256 over its text; these and a constant's canonical text are frozen and each is pinned by a golden test."
tags: [hash, golden, receipt, determinism, frozen, sha256]
timestamp: 2026-10-26T00:00:00Z
---

# What is frozen

| Frozen | Meaning |
|---|---|
| `Answer.Bytes()` | The one serialisation of an answer: a relation name, tuples sorted with no repeats, one width. |
| `Answer.Hash()` | `"sha256:"` plus the digest of `Bytes()`. |
| `RuleSet.Hash` | `"sha256:"` plus the digest of the rule set's text. |
| A constant's canonical text | See [constants and exact numbers](/concepts/constants-and-exact-numbers.md). |

Receipts stored by consumers depend on all of these. Anam's stored receipts hold their hashes, and Shearline's decisions carry rule-set IDs. A change to any of them is breaking: it takes a minor version bump and a changelog entry, and the three consumers have to agree ([decision 4](/decisions/4-v0-freeze-policy.md)).

# The tests that pin them

- `golden_test.go` pins an answer's exact bytes and hash (string escapes, non-ASCII, booleans, 2^53+1, epoch milliseconds, a trailing-zero decimal, a small decimal, 1e100), reads the bytes back to the same hash, pins a rule-set ID, and pins nine constants' canonical text. It was captured after the exact-numbers card so the numbers are the exact ones, and the file says never to update a literal to pass (card 4).
- `aggregate_golden_test.go` pins the new shapes aggregates add. `golden_test.go` itself did not change in v0.2.
- Later releases state it: v0.2.0, v0.2.1 and v0.2.2 each say no answer, hash or golden byte moved.

# What does not move when the engine grows

Proofs and absences gained fields in v0.2 (`Group`, `GroupSize`, `Value`, `Reason`), all `omitempty`, so a proof without aggregates marshals to the bytes it wrote in v0.1. A rule set that gains an aggregate changes its hash the way any edit does; no existing rule set's hash moves. Demand-driven evaluation holds the same line: answers and hashes are identical to full evaluation's ([demand-driven evaluation](/concepts/demand-driven-evaluation.md)).

# Same inputs, same outputs

Every walk over a map on the derivation path iterates in sorted order, since insertion order decides which derivation is first. A test evaluates twice and compares proofs deeply. An impure consumer builtin voids this and the hash, and that is the consumer's fault ([consumer builtins](/concepts/consumer-builtins.md)).

# Related

[Rule-set identity](/concepts/rule-set-identity.md), [proof](/concepts/proof.md).

# Citations

[1] [README.md, What it promises consumers](../../README.md)
[2] [Card 4, golden tests](../../kanban/done/4-frozen-bytes-test.md)
[3] [docs/design/aggregates.md, section 6](../../docs/design/aggregates.md)
[4] [golden_test.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/golden_test.go)
[5] [answer.go](https://github.com/mattjoyce/tarski/blob/v0.3.0/answer.go)
