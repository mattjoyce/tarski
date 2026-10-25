---
id: 1
status: backlog
blocks: [5]
---
# Exact decimal numbers

## Job story
When Anam answers a question over epoch milliseconds or an id above 2^53, I want each number held and printed exactly, so two different values never share an answer and stored receipts keep their hashes. (Drafted by datalog-module for Matt.)

## Scope
Port Anam's exact decimal (anam origin/master `internal/query/number.go`, `number_test.go`, `number_precision_test.go`) into `Const`. Keep the signatures of `Number`, `Int` and `Float`. Keep the `maxExponent` guard (Anam card 224). Define `Float`'s text as the shortest round-trip form re-parsed, so `Float(1.5)` equals `Number("1.5")`. Shearline's `-0` folds to `0`, which stays.

## Acceptance
- `9007199254740993` answers as itself; two numbers a float64 cannot tell apart stay two.
- `1792269840000` prints as an integer, not `1.79226984e+12`. `term_test.go`'s `1e21` expectation changes to the integer form.
- Anam's number tests pass here. Gate green. `docs/DESIGN.md` "Numbers are float64" is rewritten.

## Narrative
Done on branch `card-1-exact-numbers` (commit 0fe2a2e), not merged.

- `number.go` (new) holds Anam's exact decimal from origin/master `internal/query/number.go`: canonical digits plus a power of ten, `maxNumberDigits` 64, `maxExponent` 999 999 999. `Const` carries it in place of the float64. The canonical text matches Anam's byte for byte, so Anam's stored receipts keep their hashes. The key format didn't change: it is still built on `Const.s`, and since the text is canonical, equal numbers dedup and hash as one.
- I made two changes to the port. (1) `parseNumber` refuses a leading zero (`01`) and so enforces JSON grammar itself. Anam accepted `01` and Shearline refused it, and the card's contract keeps Shearline's grammar. `isJSONNumber` is gone. (2) `magnitude` compares canonical digits without zero padding. They end in a non-zero digit, so the order is the same and there's no allocation.
- `Int` builds the decimal straight from the int64, so every int64 is exact. `Float(f)` is `Number(strconv.FormatFloat(f,'g',-1,64))`. NaN/Inf: `Float` returns ErrInvalid, and `Number` returns ErrSyntax "not a finite number" (the message comes from a ParseFloat check on the error path only).
- Tests: `number_test.go` ports Anam's number and precision tests over `NewBase`/`Evaluate`: 2^53+1 answers as itself, 2^53 and 2^53+1 stay two tuples, 1 and 1.0000000000000000001 stay two, and the exponent bound refuses past nine digits. It adds a canonical-text table with JSON and text round-trips, spelling-dedup, exact ordering, and a randomised check that `Int`/`Float` keep Go's order. In `term_test.go`, the `1e21` expectation is now the integer text, every Float case pins its text too, and `1e999`/`-1e999` left the "not finite" list.
- Found: **1e999 is now accepted.** Shearline refused it because a float64 overflows; an exact decimal holds it (`1e+999`). Anam's bound test requires `1e999999999` to run, so the card settles it.
- Found: **Shearline-visible text change is wider than 1e21.** float64's shortest `'g'` form writes an exponent for any magnitude ≥ 1e6 or < 1e-4 (`1e+06`, `1.234567e+06`, `1e-05`), and those now print plain (`1000000`, `1234567`, `0.00001`). The integer half is the card's own acceptance (1792269840000). The small-decimal half follows from matching Anam's text. Shearline's repo pins no such text. This needs a board post before merge (AGENTS.md: a change a consumer would feel).
- Benchmark (`BenchmarkEvaluate`, count 5, M1, noisy machine): proofs=false median 8.07 → 8.33 ms (+3%), proofs=true 10.87 → 11.39 ms (+5%). Allocs unchanged, B/op +8.5% (the `Const` grows from 32 to 56 bytes). Within noise/the 15% line.
- Gate: RESULT: PASS (vulncheck skipped). Coverage 95.4% → 95.7%.
