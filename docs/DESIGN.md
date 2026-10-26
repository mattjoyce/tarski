# tarski: design contract

The engine's API and evaluation invariants. Carried from Shearline's `docs/DESIGN.md` § Package
`datalog` at shearline `c4f75ab`, the day the engine was cut into its own module; the package is
renamed `tarski` and error messages are prefixed `tarski:`. Where this file and the code differ,
the code is wrong or this file is stale: fix one in the same commit. The history of how each
clause was decided is in Shearline's `docs/reviews/` and `docs/DESIGN.md` § Decided with Matt.


Carried from Anam's `internal/query` with the package renamed. Error messages are prefixed
`tarski:` (they were `query:` in Anam, `datalog:` in Shearline).

```go
var ErrSyntax, ErrInvalid, ErrTooLarge error          // the three; *TooLarge still Is(ErrTooLarge)

type Kind; const KindString, KindNumber, KindBool      // unchanged
type Const; func String(string) Const; func Bool(bool) Const
func Number(lexeme string) (Const, error)      // a JSON number lexeme, held exactly (else ErrSyntax); -0 is 0
func Int(int64) Const; func Float(float64) (Const, error)   // the same constants Number makes; Float(f) is Number of f's shortest round-trip text; Float refuses NaN/Inf with ErrInvalid (no lexeme to blame)
// The string domain is valid UTF-8: the lexer refuses a literal that is not,
// NewBase/With refuse a fact that is not, and Compile refuses a hand-built constant
// that is not, since JSON would fold invalid bytes into U+FFFD and two distinct
// constants would share an Answer hash. A string literal's escapes are JSON's or
// Go's: what a JSON encoder writes (\/, a surrogate pair) and what a person writes
// (\x41, \U0001F600) both read; a lone surrogate reads as neither.
// Numbers are exact decimals (ported from Anam, card 1), not float64. Number reads
// JSON's number grammar only: no plus, leading zero, hex, underscore or bare point,
// and NaN/Inf are refused as not finite. The value is what the lexeme says, nothing
// rounded: 9007199254740993 is itself, and 1 and 1.0000000000000000001 are two
// numbers. An exponent is bounded at nine digits either way (maxExponent, Anam card
// 224) and refused past it, before any arithmetic. One value has one canonical text,
// and identity, equality and order follow the value exactly: 1, 1.0 and 1e0 are one
// constant with one hash. The text is the plain decimal (an integer in full, so
// 1792269840000 and 1e21 print every digit; 0.00001, not 1e-05), and the exact
// scientific form (1e+999, 1.23e-70) only when the plain form would pass 64
// characters (maxNumberDigits). The text matches Anam's, so Anam's stored receipts
// hold; for Shearline it changes the text, and so the hash, of any number at or
// above 1e6 or below 1e-4 in magnitude that float64's shortest form wrote with an
// exponent.
// A relation name is a lower-case letter then letters, digits and underscores, and a
// variable an upper-case letter or underscore then the same, wherever it comes from:
// Compile holds a hand-built Program and a Schema to it, as the lexer holds text.
type Tuple []Const                                     // unchanged
type Term, Atom, Comparison, Literal, Rule, Program    // unchanged
func Parse(text string) (Program, error)               // unchanged

// Schema names the base relations a base may hold and, per relation, an optional
// check. The check runs on every body atom that names the relation at Compile (a
// head never names a base relation), and on every fact of the relation at
// NewBase/With (as an Atom of constants), so the schema's word holds everywhere a
// tuple enters. Its error is wrapped, so errors.Is on the caller's sentinel works. A
// Schema that carries checks is not a comparable value; keep every other field plain data.
// Builtins declares the consumer's own guards beside the engine's; see § Built-ins.
type Schema struct {
    Arities  map[string]int
    Checks   map[string]func(Atom) error              // nil map or nil entry: no check
    Builtins map[string]Builtin                       // nil map: the engine's builtins only
}

type Builtin struct {
    Args  []BuiltinArg                  // arity is len(Args), at least 1; names for catalogues and messages
    Doc   string                        // the contract a catalogue prints
    Check func(consts []*Const) error   // optional, at Compile: one entry per argument, nil for a variable
    Holds func(args []Const) bool       // required: the guard over bound arguments
}
type BuiltinArg struct{ Name, Doc string }
func EngineBuiltins() map[string]Builtin              // prefix, suffix, contains; a fresh copy

// Fact is one tuple of a base relation.
type Fact struct {
    Relation string
    Tuple    Tuple
}

// NewBase holds facts under schema. A fact of a relation the schema lacks, or of the
// wrong arity, is ErrInvalid. Every relation in schema.Arities exists, empty or not.
func NewBase(schema Schema, facts ...Fact) (Base, error)

// With is the standing base plus more facts, as a new value. Postcondition, and the
// property a test holds over random fact sets: b.With(f...) is observationally equal
// to NewBase(schema, standing..., f...) in Len, in every Evaluate, and in every
// Answer hash. Implementation: relations the new facts do not touch are shared, not
// copied; a relation they do touch is rebuilt. Same validation as NewBase.
func (b Base) With(facts ...Fact) (Base, error)

func (b Base) Len(relation string) int                 // unchanged

func Compile(schema Schema, programs ...Program) (Plan, error)   // Definitions parameter gone

type Options struct {
    MaxBytes int   // over every derived tuple's JSON text; 0 is no bound; over it, *TooLarge
    MaxSteps int   // join steps (tuples visited) over the whole evaluation; 0 is no bound; over it, *TooLarge
    Proofs   bool  // record, for each derived tuple, the first derivation that produced it
}
// MaxBytes bounds what is derived; MaxSteps bounds the work to derive it. A rule that
// joins a wide product and derives only duplicates is charged no bytes, so without
// MaxSteps its termination would rest on the caller's deadline alone. MaxSteps also
// charges an aggregate's inner candidates, one step per group computed, and a sum's
// fold priced by sumWork before it runs (§ Aggregates). An aggregate's memo and, with
// Proofs, its kept witnesses grow with the groups computed, and a group that derives no
// tuple is charged no bytes: MaxSteps, not MaxBytes, is what bounds an aggregate's memory,
// so set it whenever rules use aggregates. *TooLarge carries Bound:
// BoundBytes, BoundSteps, or BoundNumber (a sum wider than maxSumDigits or whose text
// would pass maxExponent), and its message starts "tarski:".
// A base that holds a relation the plan derives is ErrInvalid at Evaluate, as is one
// that lacks a relation the plan reads. Compatibility between a plan and a base is by
// shape only, names and arities: a base built under a schema with different Checks
// passes, because its facts were checked by its own schema at NewBase. A caller that
// wants one set of checks everywhere compiles and builds from one Schema value, as
// Shearline's policy.Load does. Builtins are the plan's alone: a base never consults
// them, so a base built under a schema with other Builtins passes too.
func Evaluate(ctx context.Context, plan Plan, base Base, opts Options) (Result, error)

func (r Result) Relation(name string) (Answer, error)
// Answer is a value with nothing to change: Relation(), Tuples() (a copy), Len(),
// Bytes() (a copy of its one serialisation), Hash() ("sha256:" over Bytes). It
// marshals as Bytes, so what is sent, stored and hashed is one thing; a zero Answer
// marshals to an error. UnmarshalJSON reads back only an answer as written (a
// relation name; tuples sorted, no repeats, one width) and seals it again.
type Answer

// Proof is how one tuple came to be. A base fact has Rule nil and no Body. A derived
// tuple has the rule that first produced it and, in the rule's written order, the
// positive body atoms' tuples, each with its own proof. Built-ins and comparisons and
// negations contribute no body entry. The structure is finite and acyclic: a body
// tuple was always derived strictly before its head. It is a DAG shared as values:
// Result.Proof builds each (relation, tuple) proof once and reuses that value wherever
// it appears in a body, so the size is linear in the derivation records.
//
// Proofs are deterministic: for one (plan, base, options) every evaluation yields the
// same proof for every tuple; a base is its fact sequence, so Shearline's policy.Load sorts
// standing facts and a proof is a function of the policy's content. Rule is a deep
// copy the holder may keep or change; the plan is not reachable through a proof. It
// carries Set ("name/version" of the rule set Compile took it from, "" for a bare
// Program), Line, and Text (the rule as written), so a receipt can name its rule.
type Proof struct {
    Relation  string
    Tuple     Tuple
    Rule      *Rule
    Body      []Proof
    Group     [][]Proof  // v0.2: an aggregate's witnesses, each the inner body's tuples; first 64 kept
    GroupSize int        // v0.2: how many witnesses the groups held, kept or not
}
// Group and GroupSize are zero for every tuple no aggregate derived, and both marshal
// omitempty, so a proof without aggregates writes exactly the bytes it wrote in v0.1.
// Witnesses are kept in witness order, a function of (plan, base), and only when
// Options.Proofs is on; a cut Group still states the count in GroupSize.
// Proof is the proof of t in relation name; ok is false when t is not in the result or
// the evaluation ran without Options.Proofs.
func (r Result) Proof(name string, t Tuple) (p Proof, ok bool)

// Absence is why a tuple was not derived: why-not provenance. For a goal not in the
// result, every rule whose head unifies with it is walked, body in evaluation order,
// against the result's relations; each rule reports the farthest literal that found no
// match and the bindings it had there. A positive literal over a derived relation that
// found nothing is explained recursively (depth-bounded, visited set, so recursion
// through holds/2 terminates). A negation that held reports the proof of the tuple that
// blocked it. A goal over a base relation with no rules is simply "no such fact".
// Options.Proofs must be on. Deterministic for one (plan, base, goal). Engine-level: it
// knows nothing of RBAC, and for rbac.dl it reads as "no role fact for P", "no grant for
// A on a held role", "no permitted prefix matches R", or "denied by <proof>".
type Absence struct {
    Relation  string
    Goal      []Term           // constants, or variables where the caller left a position open
    Rules     []RuleFailure    // one per candidate rule; empty for a base relation
    Truncated bool             // the walk hit whyNotSteps or maxDepth; what is here is still true
}
// "Farthest literal": a depth-first walk over the rule's ordered body, extending
// bindings through every match of each literal (matches in relation order, rules in
// plan order, so the answer is deterministic); the literal reported is the one with the
// greatest index that any path reached and found no match, with that path's bindings.
// A body that matches fully would have derived the goal, so it cannot occur against the
// result it came from; it shows as a RuleFailure with a zero Failed. The walk is bounded
// by whyNotSteps = 1 << 16 candidates tried (each matched literal branches) and by
// maxDepth nested Because levels; Truncated is set where the cut fell and on every
// Absence above it. A goal already explained in the same Absence is a cycle, not a cut:
// it is left with an empty Because and does not set Truncated. A failed positive atom
// over a base relation gets an Absence with nil Rules, "no such fact"; one over a derived
// relation none of whose heads unify gets empty Rules, "no rule's head matches". Derived relations
// are scanned, not indexed, since a shared Result's lazy index would race.
type RuleFailure struct {
    Rule    *Rule              // a copy, as in Proof
    Bound   map[string]Const   // bindings when the walk stopped
    Failed  Literal            // the farthest literal with no match (negation: one that held)
    Because []Absence          // why Failed found nothing, when it is a derived positive atom
    Blocked *Proof             // when Failed is a negation that held: the tuple that blocked
    Value   *Const             // v0.2: when Failed is an aggregate whose group had a value that
                               // differed from the result already bound: that value; else nil
    Reason  string             // v0.2: why the aggregate stopped the rule (empty min/max group,
                               // mixed kinds, a non-number summed, a bound result that differs)
}
// Why-not evaluates an aggregate over the result's relations with the evaluation's own
// function, charged to whyNotSteps (candidates, unindexed scans, groups and sumWork), so
// an aggregate cannot make the walk unbounded. Value and Reason marshal omitempty.
// WhyNot explains a tuple's absence. ok is false when t is in the result (ask Proof),
// when name is not a relation of the plan or t is not its arity, or when the evaluation
// ran without Options.Proofs. maxDepth bounds the recursion; 0 means the
// engine's default (whyNotDepth = 8, enough for any policy rule set, small enough that a
// pathological recursion stays cheap).
func (r Result) WhyNot(name string, t Tuple, maxDepth int) (a Absence, ok bool)
// WhyEmpty explains why a relation holds no tuple at all: the goal is every position open
// ("_"), so each rule reports the farthest literal any binding reached. ok is false when
// the relation holds a tuple, is not the plan's, or the evaluation ran without Proofs.
// For a question that came back empty (Anam card 182). v0.2, additive.
func (r Result) WhyEmpty(name string, maxDepth int) (a Absence, ok bool)

// Both Proof and Absence render: String() is an indented derivation for a log or an
// operator; MarshalJSON emits relation, tuple, rule as {set, line, text}, and body or
// rules recursively, never the parser's AST. A proof writes each (relation, tuple)
// once; a later occurrence is {relation, tuple, rule: null, shown: true}, as the text
// says "(shown above)", so a document is linear in the derivation.
func (p Proof) String() string;   func (p Proof) MarshalJSON() ([]byte, error)
func (a Absence) String() string; func (a Absence) MarshalJSON() ([]byte, error)

// RuleSet is a named, versioned program with a hash over its text.
type RuleSet struct {
    Name    string
    Version int
    Hash    string   // "sha256:<hex>" over Text
    Program Program
}
// NewRuleSet parses text and hashes it, and names the program "name/version". The name
// must be non-empty with no whitespace or control character, slash or semicolon (ID
// is "name/version hash" and decisions join several); the version must not be
// negative; the text must not be empty, since Verify would refuse it. Else ErrInvalid.
func NewRuleSet(name string, version int, text string) (RuleSet, error)
// ID is "name/version hash", the string a decision carries.
func (s RuleSet) ID() string
// Verify holds a RuleSet to its identity and its text: the name and version are
// held to NewRuleSet's rule, the hash is recomputed over Program.Text, and the text
// is parsed again and must equal Program, so the rules that run are the rules that
// were hashed. A set built or edited by hand that fails any is ErrInvalid.
// Shearline's policy.Load calls it, so the ID in a decision is the hash of the rules that ran.
// Compile also deep-copies every program, so a caller's later edits reach no plan.
func (s RuleSet) Verify() error

// Arity reports a relation's arity in the plan, base or derived, and whether it exists.
func (p Plan) Arity(name string) (int, bool)
// Defines reports whether some rule has name as its head: a relation only declared
// has an arity and no rule. Shearline's policy.Load requires both of allow/3 and permit/3.
func (p Plan) Defines(name string) bool
// HasAggregates reports whether any rule of the plan uses an aggregate, so a consumer
// that never wants them (Shearline's policy.Load) can refuse a rule set that does. v0.2.
func (p Plan) HasAggregates() bool

// Aggregate is the body literal `Result = Op Target : { Body }`, held in the additive
// field Literal.Aggregate (nil on every other literal). v0.2.
type Aggregate struct {
    Op     string   // "count", "sum", "min" or "max"
    Target Term     // the variable summed or compared; the zero Term for count
    Result Term     // the variable the aggregate's value binds
    Body   []Literal
    Line   int
}
```

### Aggregates (v0.2)

The full design, with the invariants stated first and the Soufflé comparison, is
[`docs/design/aggregates.md`](design/aggregates.md). The contract in brief:

- **Shape.** `N = count : { body }`, `S = sum X : { body }`, `M = min X : { body }`,
  `M = max X : { body }` in a rule body. `avg` and `mean` are refused with a pointer to sum and
  count. The operation words are not reserved: a relation may still be named `count`.
- **Groups.** A variable of the braces that the rest of the rule also uses groups; it must be
  bound by a positive atom outside the braces. Variables used only inside the braces belong to
  the aggregate.
- **Witnesses.** A group's input is the set of distinct solutions of the inner body, every `_` a
  fresh variable. This matches Soufflé for single-atom bodies; for multi-literal bodies Soufflé
  projects `_` away first (design §12.1), an open question for Matt.
- **Values.** count and sum of an empty group are 0; min and max of one derive nothing. Sums are
  exact (decimal, order-free). min and max compare one kind (numbers exactly, strings
  lexically); a group that mixes kinds, or sums a non-number, derives no tuple and why-not says
  which constant.
- **Result already bound.** A Result bound before the aggregate is an equality test, as `X = 3` is.
- **Stratification.** Every atom inside the braces is a negative edge: the aggregated relations
  are complete before the aggregate is taken, and a cycle through an aggregate is refused at
  Compile with a message saying where.
- **Bounds.** Inner candidates are steps; a sum's fold is charged before it runs; a sum wider
  than `maxSumDigits = 1 << 16` places, or whose text would pass `maxExponent`, is `*TooLarge`
  with `BoundNumber`.
- **Several aggregates in a rule** are independent: a group variable may not be another
  aggregate's Result (refused at Compile); write the second aggregate in a rule of its own.
- **Witness order in a proof** follows the inner join's relation order, which follows the base's
  fact order, as `Proof.Body` always has. The answer and its hash do not depend on it; a proof
  pinned across a re-ingested base may.
- **Frozen bytes.** No v0.1 answer, rule-set hash, proof or constant text moves;
  `golden_test.go` is unchanged and `aggregate_golden_test.go` pins the new shapes.

### Demand-driven evaluation (v0.3, in progress)

The design, invariants first, is [`docs/design/demand.md`](design/demand.md); Matt's decisions
are its §8. Built so far (card 9, slices 1 and 2):

```go
type Goal struct {
    Relation string
    Args     []Term   // a constant binds the position; a variable (any name, "_" included) leaves it open
}
func (p Plan) Demand(relation, adornment string) (Plan, error)   // adornment: one b or f per position
// Options.Goal *Goal: required on a plan from Demand, ErrInvalid on any other
func (r Result) Goal() (g Goal, ok bool)                          // ok false for a full evaluation
```

- **Answers.** On a demand Result, `Relation(goal relation)` is full evaluation's relation
  filtered to the goal's constants, byte for byte and hash for hash; a base relation is the
  base's; any other derived relation is `ErrInvalid` with a message saying how to read it. A
  hidden demand relation is never named: not by `Relation`, `Arity`, a proof or `*TooLarge`.
- **Goals.** A goal names the plan's relation, has its arity, a constant at every `b` and a
  variable at every `f`; a variable other than `_` written twice is refused (an adornment
  cannot express equality).
- **Proofs and why-not** are given only for the goal's relation and tuples that agree with
  the goal's constants; `WhyEmpty` only when the goal leaves every position open. A proof is a
  valid, deterministic derivation by a source rule, not promised to be the one full evaluation
  records first; a why-not fails at the same literal per rule, with bindings and blocking
  witness possibly different where several exist.
- **Bounds.** Demand relations' tuples, the goal's seed included, are charged to `MaxBytes` and
  their joins to `MaxSteps`.
- **Not yet.** `Demand` refuses, with `ErrInvalid` saying "not yet supported", a plan with
  aggregates and a plan whose rewrite cannot be stratified. Later slices lift both.

### Built-ins

A builtin is a relation answered in Go, a guard like a comparison: every argument must be
bound by a positive body atom (else `ErrInvalid` at compile; `_` is refused too), it binds
nothing, derives nothing, and never appears in a head, a declaration, or among a schema's
`Arities` (else `ErrInvalid`). So it creates no value and evaluation still terminates. A proof
through one lists only the positive atoms; a why-not stopped by one reports it as `Failed`, with
no `Because`.

The engine has three, over strings. Argument order is pattern first, subject second. A
non-string argument makes the guard false.

| Atom | True when |
|---|---|
| `prefix(P, S)` | `strings.HasPrefix(S, P)` |
| `suffix(P, S)` | `strings.HasSuffix(S, P)` |
| `contains(P, S)` | `strings.Contains(S, P)` |

A consumer declares its own in `Schema.Builtins`, keyed by name, and they are treated exactly as
the engine's at Compile, in stratification, in Evaluate and in WhyNot. Compile refuses
(`ErrInvalid`) a declared name that is not a relation name, an engine builtin's name, or also a
base relation in `Arities`; a declaration with no `Args` or a nil `Holds`; and, as for the engine's,
a head or declaration of a builtin, a wrong arity, or an unbound argument. `Check`, when set,
sees one `*Const` per argument (nil for a variable) and its error is wrapped in `ErrInvalid`
with the line. NewBase holds the same Schema to the same declaration rules, since a Schema is one
value.

The contract a consumer signs: `Holds` is a pure, total, deterministic function of its
arguments, safe to call concurrently. The engine guarantees termination (no value is created)
and bounds the number of calls (each follows a join step charged to `MaxSteps`); purity and the
cost of one call are the consumer's. An impure `Holds` voids "same inputs, same outputs" and the
hash. A `Holds` that panics is recovered where it is called: Evaluate fails with `ErrInvalid`
naming the builtin and its arguments, WhyNot returns ok false, and no panic reaches the caller.
`Check` is not recovered, like `Schema.Checks`: it runs once at Compile, on the program, not on
data.

Implementation: `builtins.go` holds `engineBuiltins` and `type builtinTable map[string]Builtin`.
Compile copies the engine's entries and the consumer's (each `Args` cloned) into one table on the
`Plan`, so a caller changing its map afterwards changes nothing a compiled plan does. `plan.go`
consults the table in `relations`/`checkAtom`/`checkRule`; `stratify.go`'s `order` treats a
builtin literal as a guard; `eval.go`'s `join` and `whynot.go`'s `walk` evaluate it where they
evaluate a `Comparison`.

### Evaluation invariants

- **Concurrency.** A `Base`, a `Plan` and a `Result` are values: immutable once built and safe
  to share across goroutines. A relation is either a value (`relation`: read methods only, no
  `add`) or a builder (`relBuilder`: the place tuples are added, held only by the evaluator while
  `Evaluate` runs). Base relations are sealed with an index per argument position; derived
  relations are sealed without indexes when `Evaluate` returns and answer lookups by scanning.
  Nothing a caller can hold has a write method, so the safety is structural (decision 6). Anam's
  lazy-index mutation on shared relations would have raced under a concurrent `Policy`.
- **Determinism.** `Evaluate` is a function of (plan, base, options): same inputs, same `Answer`
  hashes and the same `Proof` for every tuple. Every walk over a map on the derivation path
  (`merge` over the round's delta above all) iterates in sorted relation order, since insertion
  order decides which derivation is first. A test evaluates twice and compares proofs deeply.
- **Termination.** No function symbols, no built-in produces a value; evaluation reaches a
  fixpoint. The deadline is checked every `stepsPerDeadlineCheck = 1024` join steps (unchanged).
- **Declared and undefined is empty.** Anam's `checkDefined` is removed. A relation a program
  declares and nothing defines evaluates as the empty relation, so a rule set can leave a hook
  open; the bundled `rbac.dl` does not use it (its `deny/3` is defined from `denied/3`), a
  consumer set may. Anam re-adds its `instant/1` check in its own layer if it wants it;
  `Schema.Checks` is not the place (it sees one atom, not reachability).
- **Bounds are approximate on bytes, exact on steps.** `MaxBytes` charges unescaped lengths, an
  estimate of the JSON text; `MaxSteps` counts every candidate visited by a join or a negation
  probe. Neither can be crossed with a nil error.

