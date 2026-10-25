# tarski

A small, deterministic Datalog engine for Go. Facts in, derived facts out, every answer hashed,
every derivation checkable.

Named for Alfred Tarski: by the Knaster–Tarski theorem a monotone operator on sets has a least
fixed point, and that least fixed point is exactly what a Datalog program means. The engine
computes it.

## What it is

A function. It takes a plan (compiled rules) and a base (a set of facts) and returns the derived
relations. On request it also returns a proof for any tuple that was derived and a why-not for
any tuple that was not. It reads no file and no network, holds no state between calls, and
knows no vocabulary: `role`, `entry` and `grants` are just strings to it.

The language is small: rules, facts, stratified negation, comparisons, relation declarations, and
three string built-ins (`prefix`, `suffix`, `contains`). There are no function symbols and no
arithmetic in rule heads, so every program terminates.

Status: pre-1.0. Cut on 2026-10-25 from [Shearline](https://github.com/mattjoyce/shearline),
which had cut it from [Anam](https://github.com/mattjoyce/anam), with the history carried both
times. Its consumers are Shearline's policy layer, Anam's query, and Ductile's config-time checks.

## Use

Go 1.25 or later, standard library only.

```
go get github.com/mattjoyce/tarski
go doc github.com/mattjoyce/tarski
```

The API contract and the evaluation invariants are in [`docs/DESIGN.md`](docs/DESIGN.md).

## The admission rule

A feature belongs in the engine only if evaluation, with it, is still:

1. a pure function from (plan, base) to a set,
2. terminating,
3. deterministic, down to the bytes of the canonical answer and its hash,
4. boundable by `Options`, both in bytes derived and in work done.

So these stay out: arithmetic in rule heads (it makes new values, which breaks rule 2), regular
expressions (their cost is unbounded by the input), floating-point sums in arbitrary order (rule
3), and any state carried between evaluations (rule 1). A string built-in such as `prefix` passes
all four.

## What it promises consumers

These hold from the first tag. Breaking any of them takes a minor version bump and a changelog
entry, and three consumers have to agree.

- **Answer bytes are frozen.** An `Answer` serialises one way. Its hash is sha256 over those
  bytes, and a `RuleSet` hash is sha256 over its text. Receipts stored by consumers depend on
  both.
- **Same inputs, same outputs.** One (plan, base, options) gives the same answers, hashes,
  proofs and why-nots on every run.
- **Values, not places.** `Plan`, `Base` and `Result` are immutable once built and safe to share
  across goroutines, so any number of `Evaluate` calls may run on one plan and one base.
- **Fail closed and bounded.** The context is checked on entry and during evaluation. Crossing
  `MaxBytes` or `MaxSteps` returns `*TooLarge`, never a partial answer. No consumer input can
  make the engine panic, and the fuzz suites in CI hold that.
- **Receipts as data.** `Proof`, `Absence` and `RuleFailure` are exported structs. They render
  as indented text and marshal to compact JSON.

## Roadmap

Each step ships only when a consumer actually needs it.

1. **Aggregates.** `count`, `sum`, `min` and `max` over a grouped body, stratified like negation
   and charged to the bounds. Anam needs them first.
2. **Demand-driven evaluation.** Given a goal, derive only the tuples that can reach it (magic
   sets). Answers do not change, so every existing test and hash still holds.
3. **`under/3`**, a prefix match that respects segment separators, when a consumer's names need
   it.

Not planned: arithmetic, regex, function symbols, mutable state, a file format, a network face,
or any vocabulary of its own.

## Engineering

The gate is [go-quality](https://github.com/mattjoyce/go-quality): gofmt and goimports, vet,
golangci-lint (errorlint, funlen 60, revive, forbidigo, gosec), govulncheck, and
`go test -race -shuffle=on`. CI also enforces an 80 percent coverage floor and runs each fuzz
suite for a bounded time. Run the gate locally with:

```
~/Projects/go-quality/check.sh .
```

## Licence

Apache License 2.0. See `LICENSE`.
