---
id: 7
status: done
---
# Building a large base: is a bulk constructor needed?

## Job story
When Anam builds attr/3 over the live record (hundreds of thousands of tuples) for a query, I want the base built without waste, so a question over payloads costs what the facts cost and no more. (Drafted by datalog-module; Anam's ask 3 in its consumer brief.)

## Narrative
- 2026-10-25: `BenchmarkNewBase_Attr` (bench_base_test.go) builds 100k and 500k attr/3 tuples. Before: 500k took about 271 ms and 349 MiB. A profile put a third of the memory in `Base.group` (one allocation per tuple copy, slices grown by doubling) and most of the rest in the dedup set and the per-position indexes.
- Fix, no API change: `group` counts per relation and copies each relation's tuples into one backing array (each tuple capped with a full slice expression so no append can bleed into its neighbour); `rebuild` pre-sizes the tuple slice and the set. benchstat, n=6: NewBase 500k -20% time, -38% memory, -18% allocations; BenchmarkEvaluate unchanged (allocations identical).
- Verdict for Anam: no bulk constructor. What remains is the dedup key per tuple and an index per argument position, which any constructor would pay. If attr/3 grows past a few million tuples, the lever is demand-driven evaluation or indexing fewer positions, not a new constructor. (by datalog-module)
