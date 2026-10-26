# tarski: design contract

The engine's API and the invariants evaluation keeps. This file states promises, not code: `go
doc -all` is the reference for every signature, and the status of each byte-level promise (frozen,
stable within a minor, not promised) is the table in [`README.md`](../README.md) § What it promises
consumers. How the code is organised is in [`MAINTAINERS.md`](MAINTAINERS.md). The reasoning behind
aggregates and demand is in [`design/aggregates.md`](design/aggregates.md) and
[`design/demand.md`](design/demand.md), which are dated records.

The engine names no relation of its own and no consumer's. Examples here use `edge`, `path` and
`node`.

## The API

```go
var ErrSyntax, ErrInvalid, ErrTooLarge error   // a *TooLarge also satisfies errors.Is(err, ErrTooLarge)

type Kind uint8                                // KindString, KindNumber, KindBool
type Const struct{ /* unexported */ }          // build with String, Bool, Number, Int, Float
type Tuple []Const

// Program syntax, as Parse reads it and Compile takes it.
type Term struct {
    Var   string   // "" for a constant; "_" is the anonymous variable
    Const Const
}
type Atom struct {
    Name string
    Args []Term
    Line int
}
type Comparison struct {
    Op          string   // =, !=, <, <=, > or >=
    Left, Right Term
    Line        int
}
type Literal struct {
    Atom      *Atom
    Negated   bool
    Compare   *Comparison
    Aggregate *Aggregate   // exactly one of Atom, Compare and Aggregate is set
}
type Rule struct {
    Head Atom
    Body []Literal
    Line int
    Text string   // the rule as written, final period included
    Set  string   // "name/version" of the rule set Compile took it from; "" for a bare Program
}
type Program struct {
    Rules    []Rule
    Declared map[string]int   // relations declared without rules, name/arity.
    Text     string
    Name     string           // "name/version", set by NewRuleSet; Parse leaves it empty
}
func Parse(text string) (Program, error)

type Schema struct {
    Arities  map[string]int
    Checks   map[string]func(Atom) error    // nil map or nil entry: no check
    Builtins map[string]Builtin             // nil map: the engine's builtins only
}
type Builtin struct {
    Args  []BuiltinArg                  // arity is len(Args), at least 1
    Doc   string                        // the contract a catalogue prints
    Check func(consts []*Const) error   // optional, at Compile; nil entry for a variable
    Holds func(args []Const) bool       // required: the guard over bound arguments
}
type BuiltinArg struct{ Name, Doc string }
func EngineBuiltins() map[string]Builtin    // prefix, suffix, contains; a fresh copy

type Fact struct {
    Relation string
    Tuple    Tuple
}
func NewBase(schema Schema, facts ...Fact) (Base, error)
func (b Base) With(facts ...Fact) (Base, error)
func (b Base) Len(relation string) int

func Compile(schema Schema, programs ...Program) (Plan, error)
func (p Plan) Arity(name string) (int, bool)   // base or derived
func (p Plan) Defines(name string) bool        // some rule has name as its head
func (p Plan) HasAggregates() bool
func (p Plan) Demand(relation, adornment string) (Plan, error)

type Options struct {
    MaxBytes int     // bound over derived tuples' JSON text; 0 is no bound, negative is ErrInvalid
    MaxSteps int     // bound over the work; 0 is no bound, negative is ErrInvalid; see the godoc for what a step is
    Proofs   bool    // record the first derivation of each derived tuple
    Goal     *Goal   // required on a plan from Demand, ErrInvalid on any other
}
type Goal struct {
    Relation string
    Args     []Term
}
func Evaluate(ctx context.Context, plan Plan, base Base, opts Options) (Result, error)

func (r Result) Relation(name string) (Answer, error)
func (r Result) Proof(name string, t Tuple) (Proof, bool)
func (r Result) WhyNot(name string, t Tuple, maxDepth int) (Absence, bool)
func (r Result) WhyEmpty(name string, maxDepth int) (Absence, bool)
func (r Result) Goal() (Goal, bool)
func (r Result) Cost() (steps, bytes int)

type Answer struct{ /* unexported */ }   // Tuples, Len, Bytes, Hash; marshals as Bytes

type Proof struct {
    Relation  string
    Tuple     Tuple
    Rule      *Rule       // nil for a base fact
    Body      []Proof     // the rule's positive body atoms, in written order
    Group     [][]Proof   // an aggregate's witnesses, first 64 kept
    GroupSize int         // how many witnesses the groups held, kept or not
}
type Absence struct {
    Relation  string
    Goal      []Term         // constants, or variables where the caller left a position open
    Rules     []RuleFailure  // one per candidate rule; nil for a base relation
    Truncated bool           // the walk hit its depth or step limit; what is here is still true
}
type RuleFailure struct {
    Rule    *Rule
    Bound   map[string]Const   // bindings when the walk stopped
    Failed  Literal            // the farthest literal with no match (negation: one that held)
    Because []Absence          // why Failed found nothing, when it is a derived positive atom
    Blocked *Proof             // when Failed is a negation that held: the tuple that blocked
    Value   *Const             // an aggregate's value that differed from the bound result
    Reason  string             // why an aggregate stopped the rule
}

type RuleSet struct {
    Name    string
    Version int
    Hash    string   // "sha256:<hex>" over Text
    Program Program
}
func NewRuleSet(name string, version int, text string) (RuleSet, error)
func (s RuleSet) ID() string        // "name/version hash"
func (s RuleSet) Verify() error
```

## Constants

- **Strings are valid UTF-8.** The lexer refuses a literal that is not, `NewBase` and `With`
  refuse a fact that is not, and `Compile` refuses a hand-built constant that is not. JSON would
  fold invalid bytes into U+FFFD, and two distinct constants would share an answer hash.
  `Const.UnmarshalJSON` holds JSON to the same domain: invalid UTF-8, a lone surrogate escape and
  anything after the one value are ErrSyntax, never U+FFFD. `Const.MarshalJSON` refuses a string
  that is not valid UTF-8 with ErrInvalid rather than writing U+FFFD.
- **Escapes.** A string literal reads JSON's escapes and Go's (`\/`, a surrogate pair, `\x41`,
  `\U0001F600`). A lone surrogate reads as neither.
- **Numbers are exact decimals.** `Number` reads JSON's number grammar only (no plus, leading
  zero, hex, underscore or bare point), and NaN and infinities are refused. The value is what the
  lexeme says and nothing is rounded: `9007199254740993` is itself, and `1` and
  `1.0000000000000000001` are two numbers. An exponent past nine digits is refused before any
  arithmetic. `-0` is `0`.
- **One value, one text.** `1`, `1.0` and `1e0` are one constant with one canonical text and one
  hash. The text is the plain decimal, an integer in full (`1792269840000`, `0.00001`), and the
  exact scientific form (`1e+999`) only when the plain form would pass 64 characters. Equality
  and order follow the value exactly.
- **`Int` and `Float`.** `Int` makes the number of that integer. `Float(f)` is `Number` of `f`'s
  shortest round-trip text, and refuses NaN and infinities with `ErrInvalid`.
- **Names.** A relation name is a lower-case letter then letters, digits and underscores. A
  variable is an upper-case letter or underscore then the same. `Compile` holds a hand-built
  `Program` and a `Schema` to the same rules as the lexer.

## Schema, base and plan

- **Schema.** `Arities` names the base relations a base may hold. `Checks` run on every atom
  that names the relation at `Compile`, and on every fact of the relation at `NewBase` and
  `With` (as an atom of constants), so the schema's word holds wherever a tuple enters. A check's
  error is wrapped, so `errors.Is` on the caller's sentinel works. A `Schema` that carries checks
  is not a comparable value. `Builtins` declares the consumer's guards (§ Built-ins).
- **Base.** `NewBase` refuses a fact of a relation the schema lacks, or of the wrong arity
  (`ErrInvalid`). Every relation in `Arities` exists in the base, empty or not. A base is a
  sequence of facts: `With(f...)` is observationally equal to `NewBase(schema, standing..., f...)`
  in `Len`, in every `Evaluate`, and in every answer hash. A property test holds it over random
  fact sets.
- **Plan.** `Compile` validates the programs against the schema, stratifies them, and deep-copies
  them, so a caller's later edits reach no plan. A relation a program declares and nothing defines
  evaluates as the empty relation, so a rule set can leave a hook open. A consumer that wants a
  refusal checks `Plan.Defines`; `Schema.Checks` is the wrong place, since it sees one atom and
  not reachability.
- **Compatibility of a plan and a base** is by shape only: names and arities. A base that holds a
  relation the plan derives, or lacks a relation the plan reads, is `ErrInvalid` at `Evaluate`.
  A base built under a schema with different `Checks` or `Builtins` passes, because its facts were
  checked by its own schema at `NewBase`. A caller that wants one set of checks everywhere builds
  the plan and the base from one `Schema` value.

## Evaluation

`Evaluate` computes the least fixed point of the plan over the base, stratum by stratum, and
returns every relation, base and derived. Options bound it; the context's deadline is checked on
entry and during evaluation.

- **Pure.** The result is a function of (plan, base, options). The engine reads no file or
  network and keeps no state between calls.
- **Concurrency.** `Plan`, `Base` and `Result` are values: immutable once built and safe to
  share across goroutines. Nothing a caller can hold has a write method.
- **Determinism.** One (plan, base, options) gives the same answers, hashes, proofs and why-nots
  on every run. Answers and hashes depend on the set of facts. A base is a fact sequence, and the
  first derivation found is the one a proof records, so proofs, aggregate witness order and
  why-not bindings also depend on the order the facts were given. A consumer that pins proofs
  sorts its facts before building the base. Every walk over a map on the derivation path iterates
  in sorted order.
- **Termination.** There are no function symbols and no builtin makes a value, so a derived
  tuple is built from constants of the base and the program. An aggregate makes values, but it is
  stratified like negation, so each stratum sees finitely many group keys and one value per key.
  Evaluation reaches its fixpoint.
- **Bounds fail closed.** `MaxBytes` charges approximately the JSON text of every derived tuple,
  intermediate relations included, with lengths unescaped. `MaxSteps` bounds the work, and the
  `Options.MaxSteps` godoc is the one definition of what a step is. Crossing either, or summing
  past the exact-number bounds, returns `*TooLarge`, never a partial answer. `TooLarge.Bound` says
  which, and `TooLarge.Steps` and `.Bytes` are on the scale of `Result.Cost`. An aggregate's memo
  and kept witnesses grow with the groups computed, and a group that derives no tuple is charged
  no bytes, so `MaxSteps` is what bounds an aggregate's memory: set it when rules use aggregates.
- **Cost is an instrument.** `Result.Cost` reports the steps and bytes the evaluation charged, on
  any result, with or without a goal, and it does not depend on `Options.Proofs`. Within one
  version it is exact and deterministic for one (plan, base, options). Across releases, patch
  releases included, it is not stable: step accounting grew in v0.2.1 and v0.2.2. A bound equal to
  the cost passes and one below it is refused. Choose bounds from a measurement with headroom, and
  measure again after an upgrade (README § Setting the bounds).
- **No panic on consumer data.** Parsed text, facts and hand-built `Program` values cannot make the engine panic. A `Holds`
  that panics is recovered where it is called: `Evaluate` fails with `ErrInvalid` naming the
  builtin and its arguments, and `WhyNot` and `WhyEmpty` return ok false. `Schema.Checks` and
  `Builtin.Check` run the consumer's code and are not recovered; a panic in either reaches the
  caller. The fuzz suites feed generated text and facts, and hand-built `Program` values,
  through parse, compile and evaluate.

## Built-ins

A builtin is a relation answered in Go, a guard like a comparison. Every argument must be bound
by a positive body atom, else `ErrInvalid` at compile (`_` is refused too). It binds nothing and
derives nothing, and never appears in a head, a declaration, or a schema's `Arities`. So it
creates no value, and evaluation still terminates. A proof through one lists only the positive
atoms. A why-not stopped by one reports it as `Failed`, with no `Because`.

The engine has three, over strings, with the pattern first and the subject second. A non-string
argument makes the guard false.

| Atom | True when |
|---|---|
| `prefix(P, S)` | `strings.HasPrefix(S, P)` |
| `suffix(P, S)` | `strings.HasSuffix(S, P)` |
| `contains(P, S)` | `strings.Contains(S, P)` |

A consumer declares its own in `Schema.Builtins`, keyed by name, and Compile, stratification,
`Evaluate` and `WhyNot` treat them exactly as the engine's. `Compile` refuses (`ErrInvalid`) a
declared name that is not a relation name, an engine builtin's name, or also a base relation in
`Arities`; a declaration with no `Args` or a nil `Holds`; and, as for the engine's, a head or
declaration of a builtin, a wrong arity, or an unbound argument. `Check`, when set, sees one
`*Const` per argument (nil for a variable) and its error is wrapped in `ErrInvalid` with the
line. `NewBase` holds the same `Schema` to the same declaration rules. `Compile` copies the
table, so changing the map afterwards changes no plan.

The contract splits in two.

- **The engine guarantees** termination, a bounded number of calls (each call is a step charged
  to `MaxSteps`, so `MaxSteps` bounds how often any `Holds` runs, whatever the rule bodies hold),
  and that a panicking `Holds` is an error and not a panic.
- **The consumer guarantees** that `Holds` is a pure, total, deterministic function of its
  arguments, safe to call concurrently, and cheap. An impure `Holds` voids "same inputs, same
  outputs" and the answer hash. `WhyNot` and `WhyEmpty` call `Holds` again after evaluation,
  uncharged, so the purity obligation covers explanations too.

## Proofs

`Proof` is how one tuple came to be. A base fact has `Rule` nil and no `Body`. A derived tuple
has the rule that first produced it and, in the rule's written order, the positive body atoms'
tuples, each with its own proof. Built-ins, comparisons and negations contribute no body entry.
The structure is finite and acyclic, since a body tuple was always derived strictly before its
head, and it is a DAG shared as values: each (relation, tuple) proof is built once and reused
wherever it appears, so the size is linear in the derivation records.

`Rule` is a deep copy the holder may keep or change; the plan is not reachable through a proof.
It carries `Set`, `Line` and `Text`, so a receipt can name its rule. `Group` and `GroupSize` are
zero for every tuple no aggregate derived, and a cut `Group` still states the count in
`GroupSize`.

`Result.Proof` returns ok false when the tuple is not in the result or the evaluation ran without
`Options.Proofs`. Rendering: `String` is an indented derivation for a log or an operator.
`MarshalJSON` emits relation, tuple, rule as `{set, line, text}`, and body or rules recursively,
never the parser's AST. A later occurrence of a tuple already written is
`{relation, tuple, rule: null, shown: true}`, and the text says "(shown above)", so a document
is linear in the derivation. The bytes of the JSON are frozen; the text is stable within a minor
version (README table).

## Why-not

`Absence` is why a tuple was not derived. For a goal not in the result, every rule whose head
unifies with it is walked, body in evaluation order, against the result's relations. Each rule
reports the farthest literal that found no match and the bindings it had there.

- **Farthest literal.** A depth-first walk over the rule's ordered body, extending bindings
  through every match of each literal, in relation order and rules in plan order. The literal
  reported is the one with the greatest index that any path reached and found no match, with
  that path's bindings. A body that matched fully would have derived the goal, so it cannot occur
  against the result it came from; it shows as a `RuleFailure` with a zero `Failed`.
- **Explaining further.** A positive literal over a derived relation that found nothing is
  explained recursively, to a bounded depth with a visited set, so recursion terminates. A goal
  already explained in the same `Absence` is a cycle, not a cut: it is left with an empty
  `Because` and does not set `Truncated`. A negation that held reports the proof of the tuple
  that blocked it. A failed atom over a base relation gets an `Absence` with nil `Rules`, "no such
  fact". One over a derived relation none of whose heads unify gets empty, non-nil `Rules`, "no
  rule's head matches".
- **Bounds.** The walk tries at most 65,536 candidates and goes `maxDepth` levels deep (0 means
  the default, 8). `Truncated` is set where the cut fell and on every `Absence` above it.
  Aggregates are evaluated over the result's relations by the evaluation's own function and are
  charged to the same candidate count, so an aggregate cannot make the walk unbounded.
- **Requirements.** `Options.Proofs` must be on. `WhyNot` returns ok false when the tuple is in
  the result (ask `Proof`), when the name is not a relation of the plan or the tuple is not its
  arity, or when the evaluation ran without proofs. `WhyEmpty` explains a relation that holds no
  tuple at all, every position open, and returns ok false when the relation holds a tuple.
- **Determinism.** One (plan, base, goal) gives one `Absence`. Which bindings are reported
  where several paths reach the same literal follows the base's fact order.

For `path` over `edge`, the absence of `path(a, f)` reads: the first rule stopped at
`edge(a, f)`, because there is no such fact; the second stopped at `edge(b, f)` with `X=a, Y=b, Z=f`,
for the same reason.

## Rule sets

A `RuleSet` is a named, versioned program with a hash over its text. `NewRuleSet` parses the text
and hashes it, and names the program `name/version`. The name must be non-empty with no
whitespace, control character, slash or semicolon, since an ID is `name/version hash` and a
decision may join several. The version must not be negative and the text must not be empty, since
`Verify` would refuse it. Else `ErrInvalid`.

`Verify` holds a rule set to its identity and its text: the name and version are held to
`NewRuleSet`'s rule, the hash is recomputed over `Program.Text`, and the text is parsed again and
must equal `Program`, so the rules that run are the rules that were hashed. A set built or
edited by hand that fails any of these is `ErrInvalid`. A consumer that calls `Verify` before
`Compile` carries an ID that is the hash of the rules that ran. The ID format and the hash are
frozen.

## Aggregates

The reasoning, the invariants and the comparison with Soufflé are in
[`design/aggregates.md`](design/aggregates.md). The contract in brief:

- **Shape.** `N = count : { body }`, `S = sum X : { body }`, `M = min X : { body }` and
  `M = max X : { body }`, in a rule body. `avg` and `mean` are refused with a pointer to sum and
  count. The operation words are not reserved: a relation may still be named `count`.
- **Groups.** A variable of the braces that the rest of the rule also uses groups the aggregate;
  it must be bound by a positive atom outside the braces. Variables used only inside the braces
  belong to the aggregate.
- **Witnesses.** A group's input is the set of distinct solutions of the inner body, with every
  `_` a fresh variable, for every body shape (design § 13 Q1). For a single-atom body this
  matches Soufflé. For a multi-literal body Soufflé projects `_` away first and tarski does not
  (design § 12.1).
- **Values.** count and sum of an empty group are 0. min and max of an empty group derive
  nothing. Sums are exact and order-free. min and max compare one kind (numbers exactly, strings
  lexically). A group that mixes kinds, or sums a non-number, derives no tuple, and why-not says
  which constant. The group is dropped from the answer without an error, so a consumer that
  cannot tolerate that evaluates with `Proofs` and asks `WhyNot`.
- **Result already bound.** A result variable bound before the aggregate is an equality test, as
  `X = 3` is.
- **Stratification.** Every atom inside the braces is a negative edge: the aggregated relations
  are complete before the aggregate is taken, and a cycle through an aggregate is refused at
  `Compile` with a message saying where.
- **Bounds.** Inner candidates are steps and a sum's fold is charged before it runs. A sum wider
  than 65,536 places, or whose text would pass the exponent bound, is `*TooLarge` with
  `BoundNumber`.
- **Several aggregates in a rule** are independent. A group variable may not be another
  aggregate's result (refused at `Compile`); write the second aggregate in a rule of its own.
  The 64-witness cap on a proof's `Group` is shared across all aggregates of a rule, and
  `GroupSize` is their sum.
- **Witness order in a proof** follows the inner join's relation order, which follows the base's
  fact order. The answer and its hash do not depend on it.

## Demand-driven evaluation

The reasoning, invariants first, is [`design/demand.md`](design/demand.md); the decisions are its
§ 8 and the evidence its § 9. A how-to for choosing a goal shape is
[`howto/demand.md`](howto/demand.md).

`Plan.Demand(relation, adornment)` compiles a plan to answer one shape of goal. The adornment is
one `b` or `f` per position of the relation: `b` where goals will give a constant, `f` where they
leave the position open. The constants arrive per evaluation in `Options.Goal`. `Demand` is
`ErrInvalid` for an unknown or base relation, a malformed adornment, and a plan already compiled
for a goal (call it on the plan `Compile` returned).

- **Answers.** On a demand result, `Relation(goal relation)` is full evaluation's relation
  filtered to the goal's constants, byte for byte and hash for hash. A base relation is the
  base's. Any other derived relation is `ErrInvalid` with a message saying how to read it,
  never a partial answer. A hidden helper relation is never named: not by `Relation`, `Arity`, a
  proof or `*TooLarge`. There is one `Result` type; a result evaluated for a goal answers a
  narrower contract, and `Result.Goal` tells the two apart.
- **Goals.** A goal names the plan's relation, has its arity, holds a constant at every `b` and a
  variable at every `f`. A variable other than `_` written twice is refused, since an adornment
  cannot express equality; write the equality in a rule and ask for that rule's relation. A goal
  on a plan not compiled by `Demand` is `ErrInvalid`.
- **Selectivity comes from bound positions.** A goal that binds nothing gains little and pays for
  the helper rules. Under the same bounds a demand plan can be refused where the full plan
  answers, since its helper tuples are charged to `MaxBytes` and its joins to `MaxSteps`. Measure
  both with `Result.Cost` before switching (the how-to shows how).
- **Proofs and why-not** are given only for the goal's relation and tuples that agree with the
  goal's constants. `WhyEmpty` is given only when the goal leaves every position open. A proof is
  a valid, deterministic derivation by a source rule, and it need not be the one a full plan
  records first. A why-not fails at the same literal per rule, with bindings and blocking
  witness possibly different where several exist.
- **Bounds.** Helper tuples, the goal's seed included, are charged to `MaxBytes` and their joins
  to `MaxSteps`. `Result.Cost` reports both on any result.
- **Fallback.** Where the rewrite would break stratification, demand is dropped for one relation
  and the rewrite runs again, until it stratifies: in the first strongly connected component
  holding a negative edge, the source relation of the first helper relation by sorted name. A
  relation reached under more than 64 binding patterns has its demand dropped the same way. A
  dropped relation is derived whole wherever it is reached, the goal's own included (it then has
  no seed), and the relations below it keep their demand. Fallback never changes an answer, it
  ends (each round drops a relation not dropped before), and it is a function of the plan and
  the goal shape.
- **Aggregates.** Demand enters an aggregate's braces only through its group variables, so every
  group the goal needs is computed whole and its value is full evaluation's. An aggregate's
  result passes no demand: a literal after the aggregate that reads the result is adorned open at
  that position, which is sound and complete and only less selective.

## History

tarski was cut with its history from the `datalog` package of Shearline, which had cut it from
Anam's query engine; the commits are named in [`NOTICE`](../NOTICE). What changed in each
release, and the decisions behind each, are in [`CHANGELOG.md`](../CHANGELOG.md) and the dated
records under `design/`.
