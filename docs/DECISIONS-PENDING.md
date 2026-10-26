# Decisions waiting on Matt

Collected 2026-10-26 by datalog-module while Matt was away. Each item links to where its options
and rationale are written. A recommendation is given for each, and the build proceeds on it only
where noted.

## Now

1. **Repo visibility.** Decided 2026-10-26: public.
2. **go-quality bug.** The reusable workflow fails every consumer: its format step walks
   go-quality's own dirty fixture. tarski works around it. Card `gate-walks-own-fixture` on the
   go-quality board.

## v0.2.0 (aggregates): decided 2026-10-26

All ten questions settled on the recommendation ([design § 13](design/aggregates.md)). What remains
is the consumer steps in [CHANGELOG § Unreleased](../CHANGELOG.md), and the tag.

## v0.3.0 (demand-driven evaluation): designed, not built

The fourteen questions in [`docs/design/demand.md`](design/demand.md) § Open questions. The
design names one line not to cross: demand restricts what is derived. It never reorders a body
or adds a cost model, because a query optimiser invites the next three asks.
