---
id: 13
status: doing
---
# Harden resource accounting and constant JSON admission

## Job story
When admitting adversarial inputs to bounded evaluation, I want allocation and decoding boundaries to match their documented promises, so I can prevent avoidable availability failures and ambiguous identities.

## Evidence
See `reviews/luminary-rbac-2026-10-26/00-collation.md` and retained `.go.txt` probes.

## Proposed work
- Avoid materializing all unbound candidate indexes before charging/checking work; audit lazy index creation. Benchmark-gate hot-loop changes.
- Reject negative MaxSteps/MaxBytes; zero retains its documented unlimited meaning.
- Replace Decoder.More as an EOF check in Const.UnmarshalJSON; reject trailing delimiters on direct calls.
- Decide and document strict malformed UTF-8/surrogate admission for JSON constants before changing decoding behavior.
- Add desired-behavior regressions alongside fixes; preserve every frozen golden literal. Run the full gate and consumer suite.

## Narrative
- 2026-10-26: Filed from card 12 review. Eager allocation reproduced at ~803 KB for 100k input facts under limits of 1. Negative limits admitted 10k derived tuples. Direct JSON method accepted `1]`; malformed string encodings normalized to U+FFFD. These remain unresolved. Shearline context attestation, canonical resources and revocation freshness belong in consumer integration, not engine vocabulary.
- 2026-10-26: Card-12 merged to master; branched `card-13-pressure-review-hardening`. Matt chose strict decode only: `Const.UnmarshalJSON` rejects malformed UTF-8, lone surrogate escapes and trailing text (ErrSyntax); `MarshalJSON` refuses invalid UTF-8 (ErrInvalid); `String()` keeps its signature. Negative MaxSteps/MaxBytes are ErrInvalid; zero is unchanged.
- 2026-10-26: Allocation: `matching` now returns `hits` (an index list or "all below n"), so an unbound scan allocates nothing. Refused scan of 100k facts went from 803 KB to the same as 1k. Lazy-index audit: a derived relation's index was rebuilt from scratch on every growth, which is uncharged and can be quadratic across rounds. It is now extended incrementally, so indexing is linear in derived tuples, which are already charged to MaxBytes. Step counts are unchanged, so Shearline's fixed limits are unaffected. `BenchmarkEvaluate` interleaved n=10: geomean +0.00%, B/op −0.6%.
- 2026-10-26: Gate PASS (race/shuffle, lint, vet, govulncheck; bumblebee skipped, catalog unset). Coverage 97.1%. Shearline `./...` green against this checkout via a scratch go.work (Shearline tree untouched). Regressions are in `hardening_test.go`.
