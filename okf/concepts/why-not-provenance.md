---
type: Concept
title: Why-not provenance
description: "How an absent tuple explains itself: the farthest literal each candidate rule reached, a Truncated flag for the bounded walk, Blocked for a negation that held, and WhyEmpty for a relation with no tuple at all."
tags: [whynot, whyempty, absence, explanation, provenance]
timestamp: 2026-10-26T00:00:00Z
---

# The idea

A result you cannot interrogate is one you will stop trusting. `Result.WhyNot(name, t, maxDepth)` returns an `Absence` for a tuple that was not derived. It is engine-level and knows nothing of any consumer's vocabulary: for Shearline's RBAC rules it reads as "no role fact for P" or "denied by <proof>".

# Shape

| Type | Field | Meaning |
|---|---|---|
| `Absence` | `Relation`, `Goal` | The goal; a position left open is a variable |
| | `Rules []RuleFailure` | One per rule whose head unifies with the goal. Nil for a base relation ("no such fact"); empty for a derived relation no head unifies with ("no rule's head matches") |
| | `Truncated` | The walk hit a limit; what is shown is still true |
| `RuleFailure` | `Rule` | A copy, as in [proof](/concepts/proof.md) |
| | `Bound` | Bindings when the walk stopped |
| | `Failed` | The farthest literal with no match (for a negation, the one that held) |
| | `Because []Absence` | Why `Failed` found nothing, when it is a derived positive atom |
| | `Blocked *Proof` | When `Failed` is a negation that held: the proof of the tuple that blocked |
| | `Value *Const`, `Reason string` | v0.2: when `Failed` is an aggregate, its value for the group if it had one, and why it stopped the rule |

# Farthest literal

A depth-first walk over the rule's ordered body, extending bindings through every match of each literal. Matches are in relation order and rules in plan order, so the answer is deterministic. The literal reported is the one with the greatest index that any path reached and found no match, with that path's bindings. A body that matched fully would have derived the goal, so it cannot occur against the result it came from; it shows as a `RuleFailure` with a zero `Failed`.

# WhyEmpty (v0.2.0, card 8)

`Result.WhyEmpty(name, maxDepth)` is `WhyNot` with every goal position open (`_`), sharing one `explain` with it. It answers "why did my question come back empty" so an empty answer is never mistaken for "there is nothing" (Anam card 182, "silent empties are the worst failure for an assistant"). `ok` is false for a non-empty relation, an unknown one, or a run without proofs. An empty base relation reads "no such fact". The text reads like `high(_) is not derived ... stopped at V > 140 with At="2026-07-20", V=128`; the JSON writes an open position as null.

# Aggregates in why-not

The walk evaluates an aggregate over the result's relations with the evaluation's own function, charged to `whyNotSteps` (candidates, unindexed scans, groups and `sumWork`), so an aggregate cannot make the walk unbounded. The reasons it reports: an empty `min` or `max` group, a group that mixes kinds (naming the constant), a non-number summed, or a bound result that differs from the group's value. The inner body is not explained recursively in v0.2.

# Limits

| Limit | Value | Effect |
|---|---|---|
| `whyNotDepth` | 8 | Default nesting of `Because` when `maxDepth` is 0 |
| `whyNotSteps` | 1 << 16 | Candidates tried in all |

`Truncated` is set where the cut fell and on every `Absence` above it. A goal already explained in the same Absence is a cycle, not a cut: it is left with an empty `Because` and does not set `Truncated`. A consumer builtin that panics during the walk makes `WhyNot` return ok false, since its signature has no error; with a pure `Holds` this cannot happen, because the walk revisits only bindings evaluation already tried.

# Under demand

Why-not is given only for the goal's relation and tuples that agree with the goal's constants. It fails at the same literal per rule as full evaluation, with `Bound` and `Blocked` possibly naming a different witness where several exist. `WhyEmpty` works only when the goal leaves every position open.

# Related

[Proof](/concepts/proof.md), [value and builder](/concepts/value-and-builder.md) (why the walk can scan sealed relations safely), API: [WhyNot and WhyEmpty](/api/whynot-whyempty.md).

# Citations

[1] [docs/DESIGN.md, Absence, WhyNot, WhyEmpty](../../docs/DESIGN.md)
[2] [Card 8, why is this answer empty](../../kanban/done/8-why-empty.md)
[3] [whynot.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/whynot.go)
[4] [whynot_aggregate.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/whynot_aggregate.go)
[5] [whyempty_test.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/whyempty_test.go)
