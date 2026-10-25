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
