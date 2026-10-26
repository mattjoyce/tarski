---
id: 13
status: backlog
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
