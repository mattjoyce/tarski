---
id: 4
status: backlog
blocks: [5]
---
# Golden tests for frozen bytes

## Job story
When anyone touches serialisation, numbers or hashing, I want a golden test to fail, so a consumer's stored receipts cannot be broken by accident. (Drafted by datalog-module for Matt.)

## Scope
Pin exact `Answer.Bytes()` and `Answer.Hash()` for an answer holding strings with escapes, booleans, small and large integers, decimals and negative numbers; pin a `RuleSet.Hash` and a constant's canonical text. Land after card 1, so the pinned numbers are the exact ones.

## Narrative
- 2026-10-25: `golden_test.go` pins an answer's exact bytes and hash (string escapes, non-ASCII, booleans, 2^53+1, epoch milliseconds, a trailing-zero decimal, a small decimal, 1e100), reads the bytes back to the same hash, pins a RuleSet ID, and pins nine constants' canonical text. Captured after card 1, so the numbers are the exact ones; card 2 did not move them. The file says never to update a literal to pass. (by datalog-module)
