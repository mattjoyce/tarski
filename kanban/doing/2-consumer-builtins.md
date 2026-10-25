---
id: 2
status: backlog
blocks: [5]
---
# Consumer-declared builtins through Schema

## Job story
When a consumer needs a guard that speaks its own vocabulary (Anam's `within/3` on fact-times, an `overlaps/4` on intervals), I want to declare it in the Schema I compile against, so the engine stays vocabulary-free and the consumer keeps its words. (Drafted by datalog-module for Matt.)

## Scope
`Schema.Builtins map[string]Builtin`, shaped like Anam's table (anam origin/master `internal/query/builtin.go`): arity, an optional plan-time check of constant arguments, `Holds(args []Const) bool`, and a `Doc` for catalogues. Consulted everywhere the engine builtins are (plan, stratify, eval, whynot). `prefix`, `suffix` and `contains` stay engine entries. A schema that redeclares an engine name, or names a relation that is also a base relation, is ErrInvalid at Compile. Every argument must be bound by a positive atom, so a builtin creates no values.

## Acceptance
- A test declares a consumer builtin and evaluates with it, including in a negated rule and in why-not.
- README states the contract: a builtin is a pure total function of its arguments, or the hash is the consumer's fault.
- Gate green; `docs/DESIGN.md` § Built-ins updated.

## Narrative
Branch `card-2-consumer-builtins`, commits `2413f72` (feat) and `4114423` (docs).

**Shape.** `Schema.Builtins map[string]Builtin`, where `Builtin{Args []BuiltinArg; Doc string; Check func(consts []*Const) error; Holds func(args []Const) bool}` and `BuiltinArg{Name, Doc}`. This follows Anam's `internal/query/builtin.go` rather than the brief's `Arity int` + `Check func([]Term) error`: arity is `len(Args)` (one source of truth, and the names feed catalogues and messages), and `Check` takes Anam's `[]*Const` with nil for a variable, so `checkWithin` ports unchanged. Dropped from Anam: `Name` (the map key is the name) and `BuiltinArg.Bound` (tarski requires every argument bound, so it would always be true). `Check`/`Holds` are tagged `json:"-"` so a catalogue marshals. `EngineBuiltins()` returns a fresh deep copy (map and `Args`) of prefix/suffix/contains, now expressed as the same `Builtin` type with docs.

**Plumbing.** The package-level map is gone from every hook. Compile builds a `builtinTable` (engine + consumer, `Args` cloned) after `maps.Clone(schema.Builtins)` and stores it on the `Plan`; `relations`, `checkRule`/`checkGuard`/`checkAtom`, `stratify`→`group`→`order`/`isGuard`, `join` and the why-not `walk` all read the plan's table. The hot loop does the same one map lookup per literal it did before, now on `ev.builtins` instead of the global. `checkSchema` calls `checkBuiltins`, so NewBase refuses the same bad declarations (a Schema is one value); Base/Plan compatibility in Evaluate stays by shape (builtins are the plan's alone).

**Panic policy.** Recover, fail closed, `ErrInvalid`: a `Holds` panic is recovered in `builtinTable.holds`, the frame that calls it, and returned as `ErrInvalid: builtin name(args) panicked: v`. Evaluate returns it (no partial result); WhyNot, whose signature has no error, returns ok=false. First cut re-panicked a typed value up to a deferred recover in Evaluate, which kept engine-bug panics distinct; the gate's forbidigo bans `panic`, so the error is threaded instead (guardHolds now returns `(bool, error)`). Cost: a defer per builtin call, engine ones included; outside the reach benchmark's path. `Check` is not recovered, matching `Schema.Checks` (compile-time, on the program, not on data). With a pure Holds, WhyNot only revisits bindings evaluation already tried, so a first panic there means an impure builtin; the test arms one after Evaluate.

**Tests.** `consumer_builtins_test.go` (black-box): positive, written first, negated, constant args, under a negated relation, in a recursion, beside an engine builtin; proof body omits the builtin; why-not reports `shorter(X, Y)` as Failed with no Because; every Compile refusal (engine name, base relation, bad names, no Args, nil Holds, head, declaration, arity, unbound, negation-only binding, `_`, Check refusal with the cause still `errors.Is`); NewBase refusal; snapshot (replace and delete map entry, mutate Args, after Compile); 16 concurrent Evaluate+WhyNot with equal hashes under -race; panic in Evaluate and in WhyNot; EngineBuiltins copy/marshal. `TestBuiltins_Hooks` keeps its intent but declares its fake via `Schema.Builtins` instead of mutating the global, so it is now parallel. Coverage 95.6%.

**Gate.** `SKIP_VULNCHECK=1 check.sh .` RESULT: PASS. Benchmark: the machine was loaded (other agents), so I ran old/new test binaries interleaved six times; min ns/op proofs=false 7.85 ms old vs 7.94 ms new, proofs=true 10.50 vs 10.20; allocs identical (94021 / 128231). Within noise.

**Open.** (1) Field shape (Args vs Arity; `[]*Const` vs `[]Term` vs `Atom` for Check) is the public-API call to confirm with Anam/Shearline leads before a tag. (2) WhyNot cannot say *why* it returned false after a builtin panic without an API change. (3) `whynot.go` was already over 400 lines (499) and is now 510. (4) Anam should drop its private table and pass `within` through `Schema.Builtins` once this lands; its `Bound` field goes away.
