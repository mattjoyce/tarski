# Decisions waiting on Matt

Collected 2026-10-26 by datalog-module while Matt was away. Each item links to where its options
and rationale are written. A recommendation is given for each, and the build proceeds on it only
where noted.

## Now

1. **Repo visibility.** `github.com/mattjoyce/tarski` is private, so consumers need `GOPRIVATE`
   and git credentials to fetch it. Recommendation: public, like Shearline and Anam:
   `gh repo edit mattjoyce/tarski --visibility public`.
2. **go-quality bug.** The reusable workflow fails every consumer: its format step walks
   go-quality's own dirty fixture. tarski works around it. Card `gate-walks-own-fixture` on the
   go-quality board.

## v0.2.0 (aggregates): built, audited, waiting on these

The ten questions in [`docs/design/aggregates.md` § 11](design/aggregates.md). The build used each
recommendation; any can change before the tag. Two carry new evidence:

- **Q1, witness semantics.** Soufflé itself is inconsistent (§ 12.1). tarski counts distinct
  solutions with `_` fresh everywhere. That matches Soufflé when every variable is named, and it
  keeps "how many readings in June" counting readings, not instants.
- **Q9, several aggregates per rule.** Allowed and independent, but a group variable may not be
  another aggregate's Result (the audit's finding 6).

Then the consumer steps in [CHANGELOG § Unreleased](../CHANGELOG.md), and the tag.

## v0.3.0 (demand-driven evaluation): designed, not built

The fourteen questions in [`docs/design/demand.md`](design/demand.md) § Open questions. The
design names one line not to cross: demand restricts what is derived. It never reorders a body
or adds a cost model, because a query optimiser invites the next three asks.
