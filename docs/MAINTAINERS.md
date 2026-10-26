# Maintainer notes

How the code is organised. This page is for someone changing the engine; a consumer needs only
the [contract](DESIGN.md). It names unexported identifiers, so it can drift: where it and the
code differ, the code is right.

## Where things live

| File | Holds |
|---|---|
| `lex.go`, `parse.go` | The lexer and parser, the AST types (`Term`, `Atom`, `Literal`, `Rule`, `Program`) and `Parse`. |
| `aggregate.go` | The `Aggregate` type and its syntax. `aggregate_check.go` checks its variables at `Compile`. |
| `shape.go` | The structural check of a hand-built `Program`, run before `Compile` traverses anything. |
| `plan.go` | `Schema`, `Plan`, `Compile`, and the per-rule checks (arity, safety, builtins, schema checks). |
| `stratify.go` | Stratification into strata, and body ordering (`order`) so a guard runs once its arguments are bound. |
| `eval.go` | `Result`, `Options`, `Evaluate`, and the semi-naive driver: rounds, `merge`, `apply`, proof recording. |
| `join.go` | The join (`join`), negation probes (`exists`), comparisons and guards, and step and deadline accounting (`step`, `stepN`). |
| `aggregate_eval.go`, `sum.go` | Aggregate evaluation with its per-evaluation memo, and the exact sum with its priced fold. |
| `relation.go`, `builder.go` | The two relation kinds, below. |
| `base.go`, `answer.go` | `Base`, `Fact`, `NewBase`, `With`; `Answer` and its canonical bytes and hash. |
| `term.go`, `number.go` | `Const`, `Kind`, and the exact decimal arithmetic behind `Number` and canonical text. |
| `render.go`, `proof.go` | Text and JSON rendering; `Proof` and its construction from the derivation records. |
| `whynot.go`, `whynot_walk.go`, `whynot_unify.go`, `whynot_aggregate.go` | `Absence`, the farthest-literal walk, head unification, and aggregates inside the walk. |
| `builtins.go` | `Builtin`, the engine's three, the builtin table and the `Holds` recovery. |
| `bundled.go` | `RuleSet` (the file name is a fossil: tarski bundles no rule set). |
| `bounds.go` | `TooLarge`, the bound names, `ErrTooLarge`. |
| `demand.go`, `demand_rewrite.go`, `demand_fallback.go`, `demand_result.go` | The demand API, the magic-set rewrite, the fallback when the rewrite would not stratify, and what a demand `Result` will answer. |

## Relations: values and builders

A relation is either a value (`relation`: read methods only, no `add`) or a builder
(`relBuilder`: the place tuples are added, held only by the evaluator while `Evaluate` runs, and
by `NewBase` while it makes each base relation). A builder gives up its tuples to become a value.
Nothing a caller can hold has a write method, so the concurrency promise is structural.

Base relations are sealed with an index per argument position. Derived relations are sealed
without indexes when `Evaluate` returns and answer lookups by scanning. The why-not walk also
scans derived relations and never indexes them.

## Why indexes are built where they are

The engine this one descends from indexed a relation lazily, on first read, and mutated the
relation to do it. Shared across goroutines that would have raced. So the rule now is that only
a builder indexes lazily, for the joins that read it mid-evaluation, and a builder is never
shared; a relation a caller can reach is sealed first and read-only. Building a base relation
with all its indexes up front is the price, and `NewBase` copies each relation into one backing
array to keep it small.

## How a builtin is consulted

`Compile` copies the engine's three builtins and the schema's into one `builtinTable` (each
`Args` cloned) and stores it on the `Plan`. Every later consultation reads that table, never the
schema's map:

- `plan.go` consults it in `relations`, `checkAtom` and `checkRule`, to classify a name and
  check arity, boundness and the declaration rules.
- `stratify.go` consults it in `order` (via `isGuard`), which places a builtin literal as a
  guard after the atoms that bind its arguments.
- `join.go` evaluates it in `guard`, where a comparison is also evaluated: one step is charged
  first, then `Holds` runs under a deferred `recover` (in `builtins.go`) that turns a panic into
  `ErrInvalid`.
- `whynot_walk.go` evaluates it in the walker, where it evaluates a comparison, and reports a
  failing builtin as `Failed` with no `Because`.

`Schema.Checks` and `Builtin.Check` run at `Compile` and `NewBase` and are not recovered.

## Determinism

The derivation path iterates maps only in sorted order: `merge` over a round's delta above all,
since insertion order decides which derivation is first and a proof records the first. A test
evaluates twice and compares proofs deeply. A new map walk on the path needs a sort.

## Tests that carry promises

The tests behind the README table are `golden_test.go`, `aggregate_golden_test.go`,
`render_test.go`, `cost_test.go` and `proof_test.go`. `example_test.go` holds the runnable
examples. `bench_demand_test.go` and `bench_demand_anam_test.go` reproduce the evidence in
`design/demand.md` § 9. `anam_test.go` compiles and answers a consumer's rule set, kept under
`testdata/`; the fixtures there are copies named in the `NOTICE`.
