# Decisions waiting on Matt

Each item links to where its options and rationale are written. A recommendation is given for
each, and the build proceeds on it only where noted.

## Now

1. **go-quality bug.** The reusable workflow fails every consumer: its format step walks
   go-quality's own dirty fixture. tarski works around it. Card `gate-walks-own-fixture` on the
   go-quality board.

## Decided

- **Repo visibility.** Public (2026-10-26).
- **Aggregates (v0.2.0).** All ten questions settled on the recommendation
  ([design § 13](design/aggregates.md)).
- **Demand-driven evaluation (v0.3.0).** All fourteen questions settled on the recommendation
  ([design § 8](design/demand.md)). The design names one line not to cross: demand restricts what
  is derived. It never reorders a body or adds a cost model, because a query optimiser invites
  the next three asks.

## Pending, from the documentation panel (card 10)

Both come from `docs/reviews/luminary-docs-2026-10-26/00-collation.md` and are recorded here so
they are written down, not implied. The documentation follows the "for now" choice until Matt
decides.

- **T3. One `Result` type for demand, or two?** Decided for now: one type. A `Result` evaluated
  for a goal answers a narrower contract, which the `Result` godoc states and `Result.Goal`
  detects. The alternative is a separate goal result type before v1.0, while consumers are few,
  so that code written against `Result` cannot meet a relation it may not read. Matt confirms or
  reverses before v1.0.
- **T4. Is the step count a versioned unit or an instrument?** Decided for now: an instrument.
  `Result.Cost` and `MaxSteps` are exact and deterministic within a version and not stable across
  releases, patch releases included, so bounds need headroom and re-measuring after an upgrade, and
  a patch may turn an answer into `*TooLarge`. The alternative is to version the unit, so that
  any change to step accounting needs a minor bump. This is open for Matt before v1.0, and
  the answer decides whether a tight bound is the consumer's fault or the engine's.
