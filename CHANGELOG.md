# Changelog

Answer bytes, answer hashes, rule-set hashes and constants' canonical text are frozen
(README § What it promises consumers; `golden_test.go`). Any change to them is breaking and
is listed here under its version.

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
