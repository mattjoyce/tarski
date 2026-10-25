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
