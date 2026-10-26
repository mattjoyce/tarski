# Changelog

What is frozen, what is stable within a minor version and what is not promised is one table, in
README § What it promises consumers. Any change to a frozen row is breaking and is listed here
under its version.

## Unreleased

Hardening from the RBAC pressure review (card 13, `reviews/luminary-rbac-2026-10-26/`). No
answer, hash, proof, golden byte or step count changes for any input that was accepted and
meant what it said.

- **Negative bounds are refused.** `Evaluate` returns ErrInvalid for a negative
  `Options.MaxSteps` or `Options.MaxBytes`, which every check had read as no bound. Zero is
  still no bound.
- **Strict JSON constants.** `Const.UnmarshalJSON` refuses invalid UTF-8, a lone surrogate
  escape and trailing text (`1]` called directly) with ErrSyntax; each used to decode, the first
  two as U+FFFD. `Const.MarshalJSON` refuses a string that is not valid UTF-8 with ErrInvalid.
  Answers never held one, since every entry point already refuses them; `Result.WhyNot` now
  refuses such a tuple too (ok false), so an Absence never holds one either.
- **A join's memory follows its steps.** An unbound literal no longer builds a list of every
  candidate index before the first step is charged: a refused scan of 100,000 facts under
  `MaxSteps: 1` allocated about 800 KB and now allocates what one over 1,000 does. A derived
  relation's lazy index is extended as the relation grows instead of rebuilt, so indexing costs
  work in proportion to the tuples derived. `BenchmarkEvaluate` is unchanged within noise.

## v0.3.1 — 2026-10-26

Documentation release (card 10), from the luminary panel on v0.3.0
(`docs/reviews/luminary-docs-2026-10-26/`). No answer, hash, proof or golden byte changes. The
items below are the panel's ten convergences, in its order.

- **C1, an example to start from.** `example_test.go` holds `ExampleEvaluate`,
  `ExampleResult_WhyNot` and `ExamplePlan_Demand`. They run in CI and show on pkg.go.dev, and
  `README.md` § Use points at them.
- **C2, one definition of a step.** The `Options.MaxSteps` godoc lists what a step is, once.
  `Result.Cost`, `TooLarge`, `ErrTooLarge`, `Builtin` and `docs/DESIGN.md` point to it, and the
  stale "join steps" wording is gone from `eval.go`, `bounds.go` and `builtins.go`.
- **C3, one table of what is frozen.** `README.md` § What it promises consumers now holds a
  single table (frozen, stable within a minor, not promised) with the test that pins each row.
  `CHANGELOG.md`, `AGENTS.md` and `docs/DESIGN.md` point to it. A new golden,
  `TestGolden_ProofAndAbsenceJSON` in `proof_golden_test.go`, pins Proof and Absence JSON, which
  no test had pinned byte for byte.
- **C4, no dead links in the okf bundle.** `okf/index.md` no longer lists the `api/`,
  `examples/` and `reviews/` directories, which are not in the repository, and the pages under
  `okf/` no longer link into them. `okf/decisions/` now holds the freeze policy the contract cites.
- **C5, a base is a fact sequence.** The determinism promise in `README.md` and `docs/DESIGN.md`
  now says answers and hashes depend on the set of facts, while proofs, witness order and why-not
  bindings also depend on fact order, so a consumer that pins proofs sorts its facts.
- **C6, `docs/DESIGN.md` in the present tense.** The "carried from" preambles and the
  "unchanged" markers are gone, and the API block shows the real fields of `Term`, `Atom`,
  `Literal`, `Rule` and `Options`. The file names and unexported internals moved to the new
  `docs/MAINTAINERS.md`. Five stale statements are corrected: where `join` and `walk` live, a
  bundled rule set that does not exist, the aggregate `_` question that was decided, the unit of
  a step, and the refusal of `Demand` on a plan already compiled for a goal. Lineage is one
  History paragraph pointing at `NOTICE` and this file.
- **C7, demand's caveat where the API is read.** `Plan.Demand` godoc and `docs/DESIGN.md` §
  Demand-driven evaluation say that selectivity comes from the bound positions, that helper
  tuples are charged, and that under the same bounds a demand plan can be refused where the full
  plan answers.
- **C8, how to choose a bound.** `README.md` § Setting the bounds and the `Result.Cost` godoc say
  to measure a representative request, leave headroom, and measure again after an upgrade, since
  step counts rose in v0.2.1 and v0.2.2. The step count is an instrument: exact and
  deterministic within a version, not stable across releases.
- **C9, a narrower panic promise.** "No consumer input can make the engine panic" now reads
  "no consumer data": text, facts and hand-built programs. `Schema.Checks` and `Builtin.Check`
  are named as the consumer callbacks that are not recovered. `FuzzCompileHandBuiltProgram` in
  `fuzz_program_test.go` covers hand-built programs.
- **C10, no consumer vocabulary in the contract.** `docs/DESIGN.md` speaks only of the engine,
  with `edge`, `path` and `node` for examples. The consumer sketches left it, and the guidance
  they held, without consumer names, is `docs/howto/demand.md`. `NOTICE` now names the commit
  of each copied fixture and says `rbac_unseeded.dl` is derived, not copied.
- **Builtin refusals name the argument and show the signature (card 11).** A variable a
  builtin cannot bind: `Other, argument B of within(A, B, Days), must be bound by a positive
  body atom`. The wrong arity: `within has 2 arguments, but within takes 3: within(A, B, Days)`.
  A Check refusal carries the signature. Message text only; no answer, hash or golden changes.

## v0.3.0 — 2026-10-26

Demand-driven evaluation (card 9; the design is `docs/design/demand.md`, Matt's decisions its
§8, the evidence its §9). Additive: no answer, hash or golden byte changes.

- **`Plan.Demand(relation, adornment)`** compiles a plan to answer one shape of goal;
  `Options.Goal` carries each evaluation's constants and `Result.Goal` reports them. The goal's
  relation is full evaluation's filtered to the goal's constants, byte for byte and hash for
  hash; the hidden demand relations are never named. Proofs and why-not are given for the goal
  relation only: valid and deterministic, not promised byte-identical to full evaluation's
  first derivation.
- **Fallback.** Where the magic rewrite would break stratification, or a relation is reached
  under more than 64 binding patterns, demand is dropped for that relation and it is derived
  whole. The answer is the same; only the work differs.
- **Aggregates under demand.** Demand enters the braces through the group variables only, so
  a group is never trimmed and its value is full evaluation's.
- **`Result.Cost() (steps, bytes int)`**: the steps and derived bytes the evaluation charged,
  on any Result. An instrument for comparing plans, not a frozen value and no part of an
  Answer or its hash.
- **Evidence** (design §9). Shearline's shape: demand over unseeded rules costs what the
  hand-seeded policy costs (435 against 425 steps, 97 µs against 106 µs per decision on a
  50,000-principal base) where unseeded full evaluation hits the `MaxBytes` cliff. Anam's
  shape: demand over the wide `instant` costs what demand over the hand-bounded one costs and
  28× less than full evaluation over the wide one, but about 8× the steps of full evaluation
  over the hand-bounded `instant`, because with every goal position open the demand relations
  for `displaced` and `before` are as large as the relations. Anam keeps its own `instant/1`
  (design §8 Q11) and can measure with `Result.Cost`.
- Hot loop: `BenchmarkEvaluate` bytes and allocations per op identical; times within noise.

## v0.2.2 — 2026-10-26

Two more findings from the same pressure review, after it replayed v0.2.1. No answer, hash or
golden byte changes.

- **Every builtin call costs a step.** v0.2.1 charged a step per rule application, which bounded
  how many guard-only rules ran but not how many guards one body held: a rule of 1000 builtin
  calls ran them all under `MaxSteps: 1`. Each builtin call is now a step, so `MaxSteps` bounds
  `Holds` calls directly. Comparisons stay free. Step counts rise by one per builtin call.
- **Duplicates keep no storage.** v0.1.0's faster `NewBase` (card 7) sized storage from the input
  count and shared one backing array per relation, so 300,000 identical facts kept one tuple and
  about 38 MB. A relation whose input held duplicates is now compacted to what it keeps.
- Benchmarks: bytes and allocations per op are identical to v0.2.1; timings were too noisy on the
  build machine to compare and the changed paths are not on the benchmarks' hot loop.

## v0.2.1 — 2026-10-26

Fixes from an external pressure review of v0.2.0 (board agent tarski-pressure). No answer,
hash or golden byte changes.

- **No panic on an absurd arity.** `NewBase` sized its preallocation by arity times count before
  checking fact widths, so a few short facts under a huge declared arity panicked. It now counts
  only well-formed facts, so the room it makes is bounded by the input.
- **No stack overflow on a cyclic aggregate.** A hand-built `Aggregate` whose body holds itself
  made `Compile` recurse forever while copying the program. A nested aggregate, which Compile
  refuses, is now copied one level only, and the refusal is reached.
- **Guard-only rules are bounded.** A rule with no positive atom called its guards (and a consumer
  builtin's `Holds`) with no join step charged. Every rule application now costs a step, so
  `MaxSteps` bounds the calls as README § Consumer builtins promises. Step counts rise by one per
  rule application; answers are unchanged.

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
