---
type: Concept
title: Rule-set identity
description: "A rule set is a named, versioned program with a sha256 over its text; Verify re-parses so the rules that ran are the rules that were hashed."
tags: [identity, hash, verify, receipt, ruleset]
timestamp: 2026-10-26T00:00:00Z
---

# What a rule set is

`RuleSet{Name, Version, Hash, Program}`. `NewRuleSet(name, version, text)` parses the text, hashes it and names the program `"name/version"`. It fails with `ErrInvalid` rather than return a partial set.

- The name is non-empty with no whitespace, control character, slash or semicolon, because `ID()` is `"name/version hash"` and decisions join several.
- The version is not negative. It should move when the rules' meaning does; the hash catches any change the version did not.
- The text is not empty, since `Verify` would refuse it.

# ID

`RuleSet.ID()` is `"name/version sha256:<hex>"`. A consumer carries it in its receipt. Shearline's `Decision.RuleSets` carries one ID per loaded set.

# Verify

`Verify` holds a set to its text, because the text is the one authoritative representation. The name and version are held to `NewRuleSet`'s rule, the hash is recomputed over `Program.Text`, and the text is parsed again and must equal `Program`. So an AST edited after `NewRuleSet`, or built by hand around someone else's hash, is `ErrInvalid`. Shearline's `policy.Load` calls it, so the ID in a decision is the hash of the rules that ran.

# The plan owns its programs

`Compile` deep-copies every program, so a caller's later edits reach no plan. A proof's rule is a further deep copy ([proof](/concepts/proof.md)), carrying `Set`, `Line` and `Text` so a receipt can name its rule.

# Hashing and aggregates

`Rule.Text` is cut from the source between the first token and the period, so an aggregate rule's canonical text is what was written, braces included. The hash is over the whole text and `Verify` re-parses and compares the program; neither reads the AST's shape. Existing rule sets' hashes and IDs are untouched by aggregates.

# Related

[Frozen bytes and answer hashes](/concepts/frozen-bytes-and-hashes.md), API: [RuleSet](/api/ruleset.md).

# Citations

[1] [docs/DESIGN.md, RuleSet, ID, Verify](../../docs/DESIGN.md)
[2] [docs/design/aggregates.md, section 2.4](../../docs/design/aggregates.md)
[3] [bundled.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/bundled.go)
