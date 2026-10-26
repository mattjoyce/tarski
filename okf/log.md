# Directory Update Log

## 2026-10-26
* **Release**: v0.3.0. [Demand-driven evaluation](/concepts/demand-driven-evaluation.md) shipped with `Result.Cost`; evidence in `docs/design/demand.md` section 9 (Shearline met, Anam partly met, hot loop within noise). Cards 6 and 9 closed. This bundle is committed to the repository as is: `api/`, `decisions/`, `examples/` and `reviews/` are still empty, though `index.md` names them.
* **Creation**: Established this bundle for [tarski](/packages/tarski.md), modelled on the bundle Shearline kept for the engine while it lived there. Written from the code at v0.2.2 and the repository's master, which is ahead of v0.2.2.
* **Release**: v0.2.2. Every builtin call costs a step; a relation whose input held duplicates is compacted so duplicates keep no storage. See [bounds](/concepts/bounds.md) and [the pressure review](/reviews/pressure-review-2026-10-26.md).
* **Release**: v0.2.1. No panic on an absurd arity, no stack overflow on a cyclic aggregate, and every rule application costs a step. See [the pressure review](/reviews/pressure-review-2026-10-26.md).
* **Release**: v0.2.0. [Aggregates](/concepts/aggregates.md) (`count`, `sum`, `min`, `max`), [`Result.WhyEmpty`](/api/whynot-whyempty.md), a faster `NewBase`. Aggregate semantics settled with Matt ([decision 5](/decisions/5-aggregate-semantics.md)).
* **Addition**: [Demand-driven evaluation](/concepts/demand-driven-evaluation.md) built on master for v0.3 (card 9, slices 1 to 4): `Goal`, `Plan.Demand`, `Options.Goal`, `Result.Goal`. Not released; the design questions were settled with Matt ([decision 6](/decisions/6-demand-decisions.md)). Slice 5 (evidence benchmarks, release docs, the tag) is not done at the time of writing.
* **Review**: The [luminary audit](/reviews/luminary-audit-2026-10-26.md) of v0.1.0 to master found no high findings.

## 2026-10-25
* **Release**: v0.1.0. The engine cut from Shearline's `datalog` package at its commit c4f75ab into its own module, history carried, renamed `tarski`. Exact decimal numbers, consumer builtins, Anam's rule set as a test, golden tests for the frozen bytes. See [package tarski](/packages/tarski.md).
* **Review**: Two commit security reviews on exact sums, during card 6; see [sum security reviews](/reviews/sum-security-reviews-2026-10-25.md).
