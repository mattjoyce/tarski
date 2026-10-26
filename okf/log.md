# Directory Update Log

## 2026-10-26
* **Decision**: [The v0 freeze policy](/decisions/4-v0-freeze-policy.md) written down (Matt, on the documentation panel's C3): the frozen set, the stable set, step counts as an instrument, and the test that pins each frozen item.
* **Pruned**: the index no longer names `api/`, `examples/` or `reviews/`, which never existed in the published tree. All 43 dead links retargeted to the code, the design records or the README, or unlinked. Fossils fixed: "on master", "kanban/doing", blob URLs pinned at v0.2.2 now at v0.3.0.
* **Release**: v0.3.0. [Demand-driven evaluation](/concepts/demand-driven-evaluation.md) shipped with `Result.Cost`; evidence in `docs/design/demand.md` section 9 (Shearline met, Anam partly met, hot loop within noise). Cards 6 and 9 closed. Pruned the same day: the index named `api/`, `examples/` and `reviews/`, which were never written, so it no longer does, and every link into them now points at the code or a design record.
* **Creation**: Established this bundle for [tarski](/packages/tarski.md), modelled on the bundle Shearline kept for the engine while it lived there. Written from the code at v0.2.2 and the repository's master at that time, which was ahead of v0.2.2.
* **Release**: v0.2.2. Every builtin call costs a step; a relation whose input held duplicates is compacted so duplicates keep no storage. See [bounds](/concepts/bounds.md) and the pressure review.
* **Release**: v0.2.1. No panic on an absurd arity, no stack overflow on a cyclic aggregate, and every rule application costs a step. See the pressure review.
* **Release**: v0.2.0. [Aggregates](/concepts/aggregates.md) (`count`, `sum`, `min`, `max`), [`Result.WhyEmpty` in docs/DESIGN.md](../docs/DESIGN.md), a faster `NewBase`. Aggregate semantics settled with Matt ([aggregates design record](../docs/design/aggregates.md)).
* **Addition**: [Demand-driven evaluation](/concepts/demand-driven-evaluation.md) built for v0.3 (card 9, slices 1 to 4): `Goal`, `Plan.Demand`, `Options.Goal`, `Result.Goal`. At that time not released; the design questions were settled with Matt ([demand design record](../docs/design/demand.md)). At that time slice 5 (evidence benchmarks, release docs, the tag) was not done; it landed in v0.3.0 above.
* **Review**: The luminary audit of v0.1.0 to the master of the time found no high findings.

## 2026-10-25
* **Release**: v0.1.0. The engine cut from Shearline's `datalog` package at its commit c4f75ab into its own module, history carried, renamed `tarski`. Exact decimal numbers, consumer builtins, Anam's rule set as a test, golden tests for the frozen bytes. See [package tarski](/packages/tarski.md).
* **Review**: Two commit security reviews on exact sums, during card 6; see sum security reviews.
