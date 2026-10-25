# Aggregates: count, sum, min, max over a grouped body

Design for card 6 (`kanban/doing/6-aggregates.md`), targeting v0.2.0. Written 2026-10-25 by
datalog-module for Matt's review. Binding inputs: `AGENTS.md`, `README.md` § The admission rule,
`docs/DESIGN.md`, Shearline's `docs/ENGINE.md` § Direction of travel 2, Anam's `docs/QUERY.md`
and `scripts/souffle-diff.py` (both read from anam `origin/master`). Anam card 213 is not on this
machine and was not read. Line numbers cite the tree at v0.1.0.

## 0. Two lenses, before any mechanism

**Lamport × Thomas/Hunt.** The invariants below are the specification. Code that cannot be
traced to one of them is coincidence.

- I1, meaning. A program means the stratified least fixed point it means today. Inside a stratum
  that holds an aggregate rule, every relation the aggregate reads is complete, because it sits
  in a strictly lower stratum. So each aggregate literal is a fixed function of its group key for
  the whole stratum, the stratum's operator stays monotone in the relations it derives, and the
  least fixed point exists and is unique.
- I2, termination. The values an aggregate can produce are finite: group keys range over
  constants of complete relations, and one key has one value. So each stratum's active domain is
  finite and semi-naive iteration reaches its fixpoint.
- I3, determinism. count is a cardinality, sum is exact and so associative and commutative,
  min and max follow a total order within a kind. None depends on iteration order. Witness order
  (for proofs) is relation order, which `eval.go:147-159` already makes a function of the input.
- I4, frozen bytes. A program with no aggregate literal parses to the same AST, compiles to the
  same plan, and gives the same answers, hashes, proofs and why-nots byte for byte.
- I5, bounds. Every candidate visited inside an aggregate's body is a step; every head tuple is
  charged bytes; a sum's width is bounded by a constant. No bound can be crossed with a nil error.
- I6, safety. Every head variable is bound by an outer positive atom or is the result of an
  aggregate. Every group variable is bound before its aggregate runs.
- I7, acyclic proofs. Witnesses come from lower strata, so a proof DAG stays acyclic.

**Helland × Stonebraker.** Stonebraker asks whether a logic engine should do analytics at all:
Anam's query is one evaluation over thousands of entries, and `QUERY.md:211` already says "ask
for the rows and count them". That works until the count feeds another rule ("months with more
than three readings"), which no caller can do from outside. Helland asks what the number means
once it leaves: a count the caller computed is not in the receipt; a count the engine derived
is pinned by the answer hash. The two agree on count, sum, min and max over sets, and they
disagree past that. Stonebraker would stop before sum ("the engine is not a column store");
Helland wants every number a receipt carries to be derived inside. The recommendation takes the
four ops and refuses avg (§1.6), and names the line: nothing that needs division, ordering of
results, or a scale rule enters the engine. The proof of a large group (§5) is where the
tension stays live: Lamport wants the whole group, Helland wants a bounded, self-describing
document. The design bounds the group and says so in the document.

## 1. Semantics

### 1.1 Shape

An aggregate is a body literal `R = op [X] : { inner }` in a rule whose other body literals are
the outer body. Variables fall into three classes. A **group variable** appears in `inner` and
also in the outer body or the head. A **local variable** appears only in `inner`. The **result**
`R` appears nowhere in `inner`. The head may use group variables, variables bound by the outer
body, and `R`; a head variable that occurs only inside `inner` is unsafe (§3).

For a binding of the group variables g, the **witness set** W(g) is the set of distinct
assignments to the variables of `inner`, with every `_` read as a fresh variable, that satisfy
`inner` against the complete relations. Equivalently, W(g) is the set of distinct tuple
combinations, one tuple per positive inner atom, that satisfy the inner negations, comparisons
and builtins. The two readings are the same set: an assignment fixes each atom's tuple, and two
combinations that differ at some atom differ at a variable or wildcard position there, since a
constant position cannot differ. This is Soufflé's reading: it counts solutions of the body, so
`count : { r(G, _) }` counts the tuples of `r` with first argument g, and `sum V : { e(E),
a(E, V) }` adds `V` once per (E, V) witness even when two entries share one amount.

### 1.2 The four operations

- `count : { inner }` is |W(g)|, an integer constant via `Int` (`term.go:65`).
- `sum X : { inner }` is the exact sum of X over W(g). Every X must be a number (§1.4).
- `min X : { inner }` and `max X : { inner }` are the least and greatest X over W(g) under
  `compare` (`term.go:163`): strings lexically, numbers by value, false before true. Every X
  must be of one kind (§1.4).

### 1.3 Empty groups

Soufflé gives `count` and `sum` the value 0 over an empty body and derives nothing for `min`
and `max`. tarski matches it. So `n(M, N) :- month(M), N = count : { reading(M, _) }` derives
`n("2026-06", 0)` when `month("2026-06")` holds and no reading does, and `n(N) :- N = count :
{ reading(_) }` derives `n(0)` over an empty base, since the one group with no group variables
exists. A rule with `min` or `max` derives no tuple for an empty group, and why-not says so
(§5.4). The practical consequence: "how many readings in June" answers 0 only when something in
the outer body says June exists. That is the right place for the fact: the engine has no
calendar and cannot invent the month.

### 1.4 Kinds and mixed groups

Kinds are data, so they cannot be refused at Compile. The rule follows the engine's existing
treatment of kinds: comparisons across kinds are false (`eval.go:343-364`) and a wrong-kind
builtin argument is false (`builtins.go:65-83`). A `sum` over a group holding a non-number, or a
`min`/`max` over a group holding two kinds, **derives no tuple for that group**, and why-not
reports the offending constant (§5.4). Nothing is ignored silently and nothing is coerced. The
alternative, failing the whole evaluation with `ErrInvalid`, punishes a ledger query for one
stray string; the alternative of skipping non-numbers answers a wrong total with confidence.
Open question Q2.

### 1.5 Duplicates

Set semantics is kept at the relation level: a head tuple is derived once however many groups
or witnesses produce it. The aggregate's input is the witness set of §1.1, which is a set, so
nothing is double counted; it is simply a set of assignments, not a set of X values.

### 1.6 avg

Deferred. Exact division does not terminate (1/3), so avg needs a scale and a rounding rule,
and that rule becomes frozen text in every receipt. count and sum are both derivable and pinned,
so a consumer can compute an average at any scale it likes from the receipt. The parser
recognises `avg` and `mean` in aggregate position and refuses them with "avg is not an
aggregate here; derive count and sum and divide in the caller", so a Soufflé program that uses
`mean` fails loudly.

## 2. Syntax

### 2.1 Surface

Soufflé's form, with the braces:

```
n(M, N)  :- month(M), N = count : { reading(E, At), prefix(M, At) }.
spent(M, S) :- month(M), S = sum V : { purchase(E, At, V), prefix(M, At) }.
last(M, At) :- month(M), At = max T : { reading(_, T), prefix(M, T) }.
busy(M) :- n(M, N), N > 3.
```

The inner body is a comma list of the literals a body may hold today: positive atoms,
negations, comparisons, builtins. An aggregate inside an inner body is refused at parse
("an aggregate cannot nest; name the inner aggregate's relation and aggregate over it"). Several
aggregates in one outer body are allowed and independent.

### 2.2 Lexer

`lex.go:77` lists the punctuation. Add `:`, `{` and `}`, with `:-` kept ahead of `:` in the
list so the longer match wins. No valid v0.1 program contains any of the three (each was
"unexpected" at `lex.go:83`), so I4 holds at the lexer. `count`, `sum`, `min` and `max` are not
reserved: they stay valid relation names and are recognised only in the position `Var = name
[Var] :` followed by `{`. A v0.1 program with a relation `count/1` still parses and means what
it meant.

### 2.3 Parser and AST

`parser.literal` (`parse.go:189-220`) reads a leading variable then an operator. Extend the `=`
branch: after `Var =`, if the next tokens are a name in the aggregate set and then either `:`
(count) or a variable then `:`, read an aggregate; otherwise fall through to the comparison as
today. Then `{`, a literal list through `parser.literal` with aggregates refused, `}`.

The AST change is one additive field on `Literal` (`parse.go:39-43`) and one new type:

```go
type Literal struct {
    Atom      *Atom
    Negated   bool
    Compare   *Comparison
    Aggregate *Aggregate   // new; nil for every literal Parse made before v0.2
}

// Aggregate is R = Op Target : { Body }. Target is the zero Term for count.
type Aggregate struct {
    Op     string    // "count", "sum", "min", "max"
    Target Term
    Result Term
    Body   []Literal
    Line   int
}
```

`checkLiteralShape` (`shape.go:59-78`) requires exactly one of Atom, Compare, Aggregate; a
negated aggregate is refused ("compare the result instead"); the Op is one of the four; Result
and Target are variables and not `_`; Body is non-empty and holds no aggregate. `Rule.clone`
and `copyLiteral` (`parse.go:298`, `proof.go:82-93`) deep-copy `Aggregate.Body`. `Literal.text`
(`render.go:43-53`) renders `N = count : { a(X), b(X) }` in source form. `renamed`
(`whynot_unify.go:106`) renames inside `Aggregate.Body` and the Result and Target terms.

### 2.4 Rule text and hashing

`Rule.Text` is cut from the source between the first token and the period (`parse.go:184`), so
an aggregate rule's canonical text is what was written, braces included; `oneLine`
(`render.go:83`) needs no change. `RuleSet.Hash` is sha256 over the whole text (`bundled.go:99`)
and `Verify` re-parses and compares the Program (`bundled.go:63`). Neither reads the AST's
shape, so every existing rule set's hash and ID are untouched, and a rule set that gains an
aggregate changes its hash the way any edit does.

## 3. Compile checks

All in `checkRule` (`plan.go:191-227`) and `checkGuard` (`plan.go:232-261`), each with a
message that says what to change.

- The inner atoms join the `atoms` list so `checkAtom` holds them to arities, builtin arity and
  `Check`, and `Schema.Checks` (`plan.go:265-280`). Anam's pointer check therefore runs on
  `attr` atoms inside a group exactly as outside.
- Result is treated as bound from the aggregate on, for the head and for later guards. If an
  outer positive atom also binds it, the aggregate is an equality test, as `X = 3` is.
- Result must not occur in `Body`: "N is the result of the aggregate and cannot appear inside
  it".
- Target (sum, min, max) must be bound by a positive inner atom: "X in sum must be bound by a
  positive atom inside the braces". count has no target; `count X` is a syntax error.
- Inner negations, comparisons and builtins are held to inner safety: each variable is bound by
  a positive inner atom or is a group variable bound by an outer positive atom.
- A head variable that occurs only inside a body: "X in the head of n appears only inside the
  aggregate and is local to the group; bind it in the outer body or aggregate over it".
- A group variable must be bound by an outer positive atom, not only by the head: the usual
  head-safety message already covers this.
- Builtins and base relations cannot be aggregated over as heads; nothing changes there. An
  inner body may read base relations, derived relations and builtins alike.

**Stratification.** `dependencies` (`stratify.go:54-67`) adds, for each derived relation named
by a positive or negated atom inside an aggregate body, an edge marked `negative`, so the
relation lands in a strictly lower stratum, as negation does. The cycle message is specific:
"n reads r inside an aggregate and r depends on n, so the program cannot be stratified; the
aggregated relation must be complete before it is counted. Move the rules that read n out of
r's definition". The edge type gains a `kind` so the message can tell negation from aggregate.
The outer body keeps its normal edges, so a rule may be recursive through its outer body while
aggregating a lower relation; `group` (`stratify.go:131`) marks no aggregate literal as
`recursive`, since its inputs never change inside the stratum.

**Ordering.** `order` (`stratify.go:143-177`) treats an aggregate as a guard with a binding
side effect: it is placed at the earliest point where all its group variables are bound, and
once placed its Result is added to `bound` and pending guards are flushed again. The inner body
is ordered by the same function, with the group variables pre-bound, and stored on the plan's
`rule` beside `ordered` (`plan.go:43-47`) as `aggOrdered [][]Literal`, indexed like `ordered`.

**Plan.HasAggregates.** One additive method, `func (p Plan) HasAggregates() bool`, true when
any rule has an aggregate literal. Shearline's `requireRelations` (shearline
`policy/policy.go:291-309`) gains one case that refuses a plan with aggregates; `policy.Load`
compiles before it decides, so a `Plan` method is enough and no `Program` method is needed.

## 4. Evaluation

**Where.** A new case in `join` (`eval.go:240-292`) for `l.Aggregate != nil`, after the guard
case so the hot path pays one nil check. It computes the value for the current group key,
binds Result (or compares it when already bound), recurses, and unbinds. Because the aggregate
reads lower strata only, the semi-naive loop (`eval.go:107-141`) needs no change: an aggregate
literal is never a delta position.

**Group key and memo.** The key is the keys of the group variables' bindings in order, under
the literal's identity. The evaluator holds `aggMemo map[string]aggValue`, filled once per
(rule, literal, key) and valid for the whole evaluation because the inputs are complete. The
memo is what a demand-driven evaluator will later hit with the same keys (§8).

**Computing.** The inner body runs through `join` itself over `aggOrdered`, with `env`
extended by the group bindings, emitting once per witness. Each candidate visited is `step`
(`eval.go:296`), so an aggregate over a wide product is charged as the join it is, and the
deadline is seen every `stepsPerDeadlineCheck` steps. One extra step is charged per group
evaluation so an aggregate over nothing is not free. The head tuple is charged by `count`
(`eval.go:223`) as any derived tuple is, so a sum's text is what MaxBytes sees.

**count** keeps an integer. **min/max** keep the running extreme and its kind; a second kind
marks the group mixed. **sum** folds `number.add`:

```go
// add is the exact sum of two numbers: digits aligned to the smaller exponent, added as
// big integers (math/big is stdlib), then canonical. It is associative and commutative,
// so a fold over a set is the same number in every order.
func (n number) add(o number) (number, error)
```

Alignment can make a huge mantissa: `1e999999999 + 1e-999999999` spans two billion digits.
`maxSumDigits = 1 << 16` bounds the aligned width; it is an invariant with a reason (a text
that wide would be refused by any sane MaxBytes, and the constant stops the allocation before
the bound is seen), not an option. A sum whose aligned width passes it, or whose canonical
`sciExponent` passes `maxExponent` (`number.go:40,82`) so its text would not read back, fails
the evaluation with `*TooLarge{Bound: BoundNumber}`, a new constant beside `BoundBytes` and
`BoundSteps` (`bounds.go:15-18`), message "a sum needs N digits, over the bound of M; add
smaller numbers or fewer of them". Fail closed, never a rounded total. The result goes through
`canonical` (`number.go:121`) so its text obeys the same rules as a parsed number: 1.5 + 1.5 is
`3`, and the golden constants' text (`golden_test.go:91-113`) is untouched because no existing
constructor changes.

**Determinism.** The value is a function of W(g) (I3). The memo removes any dependence on how
often a group is met. Witness order for proofs is the inner join's order, which is relation
order over sealed and merged relations, already deterministic.

## 5. Proof and why-not

### 5.1 Proof

`Proof` (`proof.go:15-20`) is frozen; two fields are added, both zero for every non-aggregate
proof and `omitempty` in JSON, so existing proof bytes do not move:

```go
type Proof struct {
    Relation string
    Tuple    Tuple
    Rule     *Rule
    Body     []Proof
    Group     [][]Proof // new: one entry per witness kept, each the inner positive atoms' proofs in written order
    GroupSize int       // new: the whole group's size, so a truncated Group still states the count
}
```

`Body` keeps the outer positive atoms as today. `derivation` (`eval.go:192-200`) gains the
same two, filled by `record` from the aggregate's memo entry when a head tuple is first
derived. With several aggregates in a rule the witnesses are concatenated in literal order and
`GroupSize` sums them; a receipt that needs them apart can read the rule text. `builder.build`
(`proof.go:45-62`) builds each witness tuple's proof through the same memo, so shared facts stay
one value (I7 holds: witnesses are lower-stratum tuples).

### 5.2 Size

A count over 10^5 entries would carry 10^5 witnesses. `maxProofGroup = 64` witnesses are kept,
in witness order, and `GroupSize` carries the true size. The text form prints `group of 1024
(64 shown)` and the JSON carries `"group": [...]` and `"group_size": 1024`. This is the
Helland compromise named in §0: the document is bounded and says what it left out. Open
question Q4 on the constant, and whether 0 (size only) should be the default.

### 5.3 Rendering

`Proof.write` (`render.go:137-154`) prints after the rule line one indented `group of N` line,
then each kept witness's tuples at depth+2 with `(shown above)` dedupe as today. `jsonProof`
(`render.go:235-241`) gains `Group [][]jsonProof` and `GroupSize int`, both `omitempty`.

### 5.4 Why-not

The walker (`whynot_walk.go:42-75`) gains a case for an aggregate literal: compute the value
over the result's relations for the bound group (the same function as evaluation, reading
`Result.relations`). If Result is bound by head unification and differs, or the group is empty
under min/max, or the group is mixed, the literal fails and `record` keeps it. `RuleFailure`
(`whynot.go:45-66`) gains one additive field:

```go
// Value is the aggregate's value for the group when Failed is an aggregate literal
// and the group had one; nil for an empty or mixed group, which Reason explains.
Value  *Const
Reason string // "" unless Failed is an aggregate: "the group is empty", "the group mixes kinds: 12 and \"12\""
```

Text: `stopped at N = count : { ... }  with M="2026-06", the aggregate is 3`. JSON: `"value"`
and `"reason"`, `omitempty`. A failed outer positive atom inside the walk is explained as today;
the inner body is not explained recursively in v0.2 (an empty group's cause would need a
why-not per inner atom over an open goal; noted for later, not blocking).

## 6. Frozen bytes

- Answer serialisation and hash (`answer.go:44-53`) are not touched; an aggregate's value is a
  `Const` made by `Int` or `canonical`, the constructors every existing text goes through.
- `RuleSet.Hash` is over text (`bundled.go:99`); existing texts are unchanged.
- Parse of any v0.1 text yields the v0.1 AST plus a nil `Aggregate` pointer, and Compile of
  that AST runs the v0.1 checks because every new branch is behind `l.Aggregate != nil`.
- Proof and Absence JSON gain only `omitempty` fields, so proofs of non-aggregate tuples
  marshal to the same bytes.
- `golden_test.go` is not edited. A second golden test pins an aggregate answer, a sum text and
  an aggregate rule set ID as new constants.

## 7. Consumers

**Shearline.** Policy never uses aggregates. `policy.Load` adds `plan.HasAggregates()` to its
refusal list with "policy rules do not aggregate; a decision is one tuple, not a count". No
Shearline hash moves, since its rule texts do not change.

**Anam.** Syntax is Soufflé's for the four ops, so `souffle-diff.py` can carry an aggregate
question with the same text on both sides. Deltas the oracle's author must know:

1. Soufflé is typed. `sum` over `attrf` sums floats in arbitrary order; tarski sums exactly.
   The script compares numbers at nine significant digits (`normalise`), which absorbs it for
   ordinary totals and not for pathological ones.
2. A mixed-kind group derives nothing in tarski; Soufflé never sees the mix because `attr`
   and `attrf` are split. An aggregate over `attr` values needs a kind guard in the Anam text
   or the oracle will disagree.
3. Soufflé's manual documents `min` and `max` over numeric types; tarski orders strings too
   (a `max` fact-time is the natural "latest"). Check the Soufflé build before translating one.
4. `mean` has no tarski form (§1.6).
5. Empty groups: count and sum 0, min and max none, on both sides. Verify against the Soufflé
   build in use; the script should pin it with one question over a month with no readings.

`QUERY.md:211` ("No aggregates") and the count-over-a-month example at `QUERY.md:342` are
Anam's to rewrite once v0.2.0 tags.

**Ductile.** Config-time checks with no aggregates; nothing it marshals changes bytes.

## 8. Forward compatibility with demand-driven evaluation

Magic sets rewrites a rule to carry a demand predicate over its bound head positions. For an
aggregate rule the demand may bind group variables and nothing else: the result is unknown at
demand time, and any demand pushed inside the braces on a local variable would trim a group
and change a count. The design keeps three things so v0.3 is a rewrite and not a redesign:

- The inner body is a closed unit with its own ordering (`aggOrdered`) whose free variables
  are exactly the group variables. A rewriter can lift it to an anonymous relation
  `agg_k(G..., X, locals...)` without re-deriving which variables cross the boundary.
- The memo is keyed on the group key alone, so demand that arrives as a set of keys hits the
  same function evaluation does. Nothing in the value depends on how the key was reached.
- Aggregated relations are complete before any aggregate runs (strict stratum), so a
  demand-restricted evaluation that derives a lower relation only for the demanded keys must
  derive it fully for those keys; the rewriter must treat the inner body's relations as
  "needed whole for each demanded group", never partially. This is the same discipline
  negation already forces on magic sets, and the design avoids anything stronger.

Avoid: aggregates whose group key includes the result, aggregates reading the same stratum,
and any caching keyed on outer bindings beyond the group key.

## 9. Test plan

Table-driven throughout, real engine, no mocks.

- Parse: each op, count with and without outer body, several aggregates, refusals (nested,
  negated, `_` result, `count X`, `avg`, bare `:`), `Text` round trip, `count/1` still a
  relation.
- Compile: every message in §3, each by a one-rule program; the aggregate cycle; the same
  program with the aggregate moved one stratum down compiles.
- Evaluate: per op, groups with one, many and no witnesses; wildcards counted as witnesses;
  shared values summed per witness; mixed kinds derive nothing; strings under min and max;
  Result pre-bound as a test; recursive outer body; builtins and `Schema.Checks` inside braces.
- Property tests over random small bases: `count` equals `Len` of the group relation written
  without aggregates with all inner variables and fresh wildcard names in its head; `sum`
  equals a fold of `number.add` over that relation; `min`/`max` equal the ends of that answer
  sorted by `less`; two evaluations give equal hashes and deeply equal proofs.
- `number.add`: table (carries, signs, exponent alignment, zero), and `FuzzNumberAdd` for
  commutativity, associativity, identity, `cmp` consistency, and text that reads back; the
  width and exponent bounds refuse where specified.
- Bounds: MaxSteps crossed inside a group is `*TooLarge{BoundSteps}` with the head relation
  named; a sum past `maxSumDigits` is `BoundNumber`.
- Proof: group content and order, `GroupSize` over the cap, `(shown above)` across witnesses,
  JSON and text golden. Why-not: wrong value, empty min group, mixed group, with `Value` and
  `Reason` pinned.
- Golden: new constants for an aggregate answer, sum text and rule-set ID; the existing three
  untouched. `FuzzParseCompileEvaluate` seeds (`fuzz_test.go:18-23`) gain aggregate programs.
- `anam_test.go` gains a counting question over the same base. `BenchmarkEvaluate` within noise.

## 10. Staged implementation

Each slice leaves the gate green and the golden test untouched.

1. `number.add`, `maxSumDigits`, `BoundNumber`, tests and fuzz. No API change.
2. Lexer tokens, `Aggregate` type, parser, shape check, clone, text rendering,
   `Plan.HasAggregates`. Compile refuses every aggregate with "aggregates are not evaluated
   yet" so the surface lands inert. Shearline can adopt the refusal here.
3. Compile checks, stratification, ordering, evaluation of all four ops without proofs,
   property tests, additive golden.
4. Proof `Group`/`GroupSize`, why-not `Value`/`Reason`, rendering, tests.
5. `DESIGN.md`, `README.md`, `CHANGELOG.md`, board posts to the three consumers, Anam
   counting test, tag v0.2.0.

## 11. Open questions for Matt

- **Q1, witness semantics.** Count distinct body solutions, wildcards fresh (Soufflé's
  reading, §1.1), or distinct assignments to named variables only? Recommend Soufflé's: it is
  what `count : { r(G, _) }` plainly means and what the oracle computes.
- **Q2, mixed kinds.** No tuple for the group with why-not explaining (recommended), fail the
  evaluation with `ErrInvalid`, or sum numbers and skip the rest? Recommend no tuple: it is
  the engine's existing treatment of kinds and it is visible.
- **Q3, empty groups.** Match Soufflé (count and sum 0, min and max none)? Recommend yes; the
  oracle depends on it and 0 is the honest count of nothing.
- **Q4, proof size.** `maxProofGroup = 64` kept witnesses plus `GroupSize`? Alternatives: 0
  (size only) or an `Options` field. Recommend the constant at 64; an option would make the
  proof bytes depend on options, which they do not today.
- **Q5, sum bound error.** New `BoundNumber` under `*TooLarge` (recommended, it is a size
  refusal and `errors.Is(err, ErrTooLarge)` keeps working for consumers), or `ErrInvalid`?
- **Q6, min/max over strings.** Allow (recommended, Anam's fact-times) at the cost of one
  oracle delta, or numbers only to match Soufflé exactly?
- **Q7, avg.** Defer with a refusing keyword (recommended), or pick a scale now?
- **Q8, HasAggregates placement.** `Plan` only (recommended) or also `Program`?
- **Q9, several aggregates per rule.** Allow (recommended, no extra machinery) or one?
- **Q10, slice 2 inert landing.** Ship the syntax with Compile refusing, so Shearline's refusal
  can be adopted before evaluation exists? Recommend yes; it also lets Anam's oracle text be
  prepared against a parser that exists.
