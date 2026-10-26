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
three string built-ins (`prefix`, `suffix`, `contains`), beside which a consumer may declare its
own guards. Since v0.2 there are also aggregates in Soufflé's syntax:
`N = count : { ... }`, and `sum`, `min` and `max` over a variable, stratified like negation. There
are no function symbols and no arithmetic in rule heads, so every program terminates.

Status: pre-1.0. Cut on 2026-10-25 from [Shearline](https://github.com/mattjoyce/shearline),
which had cut it from [Anam](https://github.com/mattjoyce/anam), with the history carried both
times. Its consumers are Shearline's policy layer, Anam's query, and Ductile's config-time checks.

## Use

Go 1.25 or later, standard library only.

```
go get github.com/mattjoyce/tarski@v0.3.0
go doc -all github.com/mattjoyce/tarski
```

Start from the runnable examples: `ExampleEvaluate`, `ExampleResult_WhyNot` and
`ExamplePlan_Demand` are on pkg.go.dev, and `go test -run Example` runs them. They are compiled
and checked in CI, so they cannot drift from the code. The API contract and the evaluation
invariants are in [`docs/DESIGN.md`](docs/DESIGN.md); choosing a goal shape for demand-driven
evaluation is in [`docs/howto/demand.md`](docs/howto/demand.md); the ideas behind the engine
are explained in [`okf/concepts/`](okf/concepts/index.md).

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

These hold from the first tag. Breaking a frozen row takes a minor version bump and a changelog
entry, and the consumers that import tarski have to agree.

| What | Status | Pinned by |
|---|---|---|
| Answer bytes (an `Answer` serialises one way) | frozen | `TestGolden_AnswerBytesAndHash`, `TestGolden_AggregateAnswers` |
| Answer hash (sha256 over those bytes) | frozen | `TestGolden_AnswerBytesAndHash` |
| `RuleSet` hash (sha256 over its text) | frozen | `TestRuleSet_HashIsStillTheTextHash`, `TestGolden_RuleSetID` |
| `RuleSet` ID format (`name/version hash`) | frozen | `TestGolden_RuleSetID` |
| Constant canonical text | frozen | `TestGolden_ConstantText` |
| Proof JSON bytes | frozen | `TestGolden_ProofAndAbsenceJSON` |
| Absence JSON bytes | frozen | `TestGolden_ProofAndAbsenceJSON` |
| Rendered text (`Proof.String`, `Absence.String`) | stable within a minor version | `TestRender_Strings`, `ExampleResult_WhyNot` |
| Which derivation a proof records | not promised across plans: deterministic for one (plan, base sequence, options), and a demand plan may record a different one than the full plan | `TestProof_Deterministic` |
| Step and byte counts (`Result.Cost`, `TooLarge`) | not promised across releases, patch releases included; exact and deterministic within a version | `TestResult_CostIsWhatTheBoundsCharge` |

The rest of the contract:

- **Same inputs, same outputs.** One (plan, base, options) gives the same answers, hashes,
  proofs and why-nots on every run. A base is a fact sequence, not a set. Answers and hashes
  depend only on the set of facts. Proofs, witness order and why-not bindings also depend on
  the order the facts were given, because the first derivation found is the one recorded. A
  consumer that pins proofs sorts its facts before building the base.
- **Values, not places.** `Plan`, `Base` and `Result` are immutable once built and safe to share
  across goroutines, so any number of `Evaluate` calls may run on one plan and one base.
- **Terminating.** Evaluation always ends. Rules have no function symbols and no builtin makes
  a value, so a derived tuple is built from constants of the base and the program. An aggregate
  adds values, but it is stratified like negation, so each stratum sees finitely many group
  keys and one value per key ([`docs/design/aggregates.md`](docs/design/aggregates.md) I2).
- **Fail closed and bounded.** The context is checked on entry and during evaluation. Crossing
  `MaxBytes` or `MaxSteps`, or summing past the exact-number bounds, returns `*TooLarge`, never a
  partial answer. With aggregates, set `MaxSteps`: it bounds an aggregate's memory as well as its
  work, since a group that derives nothing is charged no bytes.
- **No panic on consumer data.** Text given to `Parse`, facts given to `NewBase` or `With`, and
  a hand-built `Program` given to `Compile` cannot make the engine panic, and a panicking
  `Builtin.Holds` is recovered and returned as an error. Two consumer callbacks are not
  recovered: `Schema.Checks` and `Builtin.Check` run the consumer's code, and a panic in either
  reaches the caller. The fuzz suites cover generated text, facts and number lexemes
  (`FuzzParseCompileEvaluate`, `FuzzNumberRoundTrip` and two graph oracles) and hand-built
  `Program` values (`FuzzCompileHandBuiltProgram`).
- **Receipts as data.** `Proof`, `Absence` and `RuleFailure` are exported structs. They render
  as indented text and marshal to compact JSON.

### Setting the bounds

`MaxSteps` and `MaxBytes` should come from a measurement. Evaluate a representative request
with no bound, read `Result.Cost`, and set the bounds with headroom above what it reports.
What a step is lives in one place, the `Options.MaxSteps` godoc. Step accounting grew in v0.2.1
and v0.2.2, so a bound tuned tight can refuse after a patch upgrade: re-measure after any
upgrade, and treat `*TooLarge` on a request that used to pass as a reason to re-measure before
anything else. A bound equal to the measured cost passes and one below it is refused
(`TestResult_CostIsWhatTheBoundsCharge`).

## Consumer builtins

A consumer that needs a guard in its own vocabulary declares it in `Schema.Builtins` beside the
engine's (`EngineBuiltins` lists those for a catalogue). It is a guard like `prefix`: every
argument is bound by a positive body atom, it binds and derives nothing, and Compile copies the
table, so changing the map afterwards changes no plan.

The contract splits in two. **The engine guarantees** termination (a builtin creates no value),
a bounded number of calls (each call is a step charged to `MaxSteps`), and that a `Holds`
which panics fails the evaluation with `ErrInvalid` rather than escaping. **The consumer
guarantees** that `Holds` is a pure, total, deterministic function of its arguments, safe to
call concurrently, and that one call is cheap. An impure builtin voids "same inputs, same
outputs" and the answer hash, and that is the consumer's fault, not the engine's.

## Roadmap

Each step ships only when a consumer actually needs it.

1. **Aggregates.** `count`, `sum`, `min` and `max` over a grouped body, stratified like negation
   and charged to the bounds. Anam needs them first. Released in v0.2.0
   ([`docs/design/aggregates.md`](docs/design/aggregates.md) § 13).
2. **Demand-driven evaluation.** Given a goal, derive only the tuples that can reach it (magic
   sets). Answers do not change, so every existing test and hash still holds. Released in
   v0.3.0 ([`docs/design/demand.md`](docs/design/demand.md), evidence § 9). Cost follows the
   bound positions of the goal, so measure with `Result.Cost` before switching. How to
   choose a goal shape, and the case where it costs more, is in
   [`docs/howto/demand.md`](docs/howto/demand.md).
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
