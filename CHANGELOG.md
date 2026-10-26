# Changelog

Answer bytes, answer hashes, rule-set hashes and constants' canonical text are frozen
(README § What it promises consumers; `golden_test.go`). Any change to them is breaking and
is listed here under its version.

## v0.2.0 — 2026-10-26

Aggregates (card 6). The semantics were settled with Matt (design § 13).

Before a consumer adopts v0.2 (Go resolves the Shearline/Anam/Ductile diamond to one tarski, so
Anam taking v0.2 moves Shearline too):

- **Shearline:** refuse aggregates in `policy.Load` with `plan.HasAggregates()`, and teach any
  code that walks rule bodies that a `Literal` may now be an `Aggregate` (neither `Atom` nor
  `Compare` set).
- **Anam:** `scripts/souffle-diff.py` must skip min or max over strings and any multi-literal
  aggregate body holding `_`, where Soufflé differs (design § 12, § 12.1).
- **Everyone with aggregates:** set `MaxSteps`; it bounds an aggregate's memory as well as its
  work.

- **Aggregates.** `N = count : { body }`, `S = sum X : { ... }`, `min`, `max`, in Soufflé's
  syntax. Stratified like negation, exact sums, empty count and sum are 0, empty min and max
  derive nothing, mixed kinds derive no tuple. `avg` and `mean` are refused. Witnesses are the
  distinct solutions of the inner body with `_` fresh, which differs from Soufflé for
  multi-literal bodies (design § 12.1).
- **Additive API.** `Aggregate`, `Literal.Aggregate`, `Plan.HasAggregates`, `BoundNumber`,
  `Proof.Group` and `Proof.GroupSize`, `RuleFailure.Value` and `RuleFailure.Reason`. The new JSON
  fields are omitempty, so no v0.1 proof, answer or hash moves; `golden_test.go` is unchanged.
- **`TooLarge` gained an unexported field.** An unkeyed `TooLarge{...}` literal in consumer code
  no longer compiles (go vet already flags unkeyed literals).
- **Bounds.** An aggregate's inner candidates are steps, and a sum's fold is priced by
  `sumWork` and charged before it runs (two commit security reviews).
- **`Result.WhyEmpty`** (card 8) explains a relation that holds no tuple at all, every
  position open, for a question that came back empty (Anam card 182). Additive.
- **Performance.** `NewBase` copies each relation into one backing array: 500k facts build in
  20% less time and 38% less memory (card 7). A wide sum parses each addend once: 85 times
  faster on 200 addends spanning 60,000 digits.

## v0.1.0 — 2026-10-25

The engine cut from Shearline (`shearline/datalog` at c4f75ab) into its own module, with its
history, and renamed `tarski`.

- **Carried unchanged:** Parse, Compile, NewBase, Base.With, Evaluate with Options{MaxBytes,
  MaxSteps, Proofs}, Answer, Proof, WhyNot/Absence, RuleSet/NewRuleSet/Verify, stratified
  negation, the string built-ins `prefix`, `suffix` and `contains`.
- **Exact decimal numbers** (card 1), ported from Anam. Numbers are held and compared exactly.
  Canonical text matches Anam's, so Anam's receipts keep their hashes. *Breaking against
  Shearline v0.1.0's engine:* numbers at or above 1e6, or below 1e-4, now print in full
  (`1000000`, `0.00001`) instead of exponent form, which changes those answers' hashes. A
  leading-zero literal such as `01` is refused. A number is accepted only if its canonical text
  reads back.
- **Consumer builtins** (card 2): `Schema.Builtins` declares guard builtins in the shape of
  Anam's table, and `EngineBuiltins()` lists the engine's own. A panicking `Holds` is an error,
  never a panic.
- Error messages are prefixed `tarski:`.
- Tests: Anam's `rules/anam.dl` compiles and answers (card 3); golden bytes and hashes (card
  4); a number round-trip fuzz target.
