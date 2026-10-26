# Demand-driven evaluation: derive only what can reach the goal

> **Status:** decided 2026-10-26, released in v0.3.0. This is the design record, frozen as
> written; line citations into the code are as of the commit named below (`1053f06`, the
> aggregates-through-demand slice) and are not maintained. The contract is `docs/DESIGN.md`; the
> API is `go doc`. Notes on this record are in the Errata section at the end.

Design for card 9 (`kanban/done/9-demand-driven.md`). Written 2026-10-26 by datalog-module for
Matt's review. Binding inputs: `AGENTS.md`, `README.md`
§ The admission rule, `docs/DESIGN.md`, `docs/design/aggregates.md` § 8 and § 12.1, Shearline's
`docs/ENGINE.md` § Direction of travel 3 and `policy/rbac.dl`, the Shearline module panel
(`docs/reviews/luminary-module-2026-10-22/01-hickey-armstrong.md` A1), Anam's `rules/anam.dl` and
`docs/QUERY.md` (read from anam `origin/master`). Line numbers cite master at the aggregates
slice 4 commit.

## 0. Lenses, before any mechanism

**Lamport × Thomas/Hunt.** The invariants are the specification; the rewrite in § 2 exists only
to make them true. Full evaluation means the stratified least fixed point M(plan, base). A goal
g names a relation and fixes some of its positions. Demand evaluation D(plan, base, g) must
satisfy:

- D1, answers. `Relation(g.Relation)` under D is M's relation filtered to the tuples that agree
  with g's constants, byte for byte, hash for hash.
- D2, soundness. Every tuple D derives into a consumer-visible relation is in M.
- D3, completeness per demand. For every relation r, binding pattern α and binding β in the
  demand set of (r, α) at the fixpoint, every tuple of M's r that agrees with β is in D's r. D3
  is what makes D1 true and what lets a why-not walk read D's relations without lying.
- D4, termination. The rewritten program is Datalog: demand tuples are projections of bindings
  over the active domain, so nothing new is made and the fixpoint is reached.
- D5, determinism. The rewrite is a function of (plan, goal shape); evaluation of the rewritten
  program is a function of (rewritten plan, base, goal) exactly as today.
- D6, frozen bytes. No answer, hash, rule-set hash, constant text, proof or absence shape moves.
  A plan not compiled for a goal takes the path it takes today.
- D7, hidden helpers. No exported surface shows a demand relation or a rewritten rule: a
  `Proof.Rule` is the source rule, `TooLarge.Relation` is a source relation, `Result.Relation`
  knows no helper name.
- D8, stratification. The rewritten program is stratified, or demand is dropped, relation by
  relation, until it is. A relation whose demand is dropped is evaluated in full.
- D9, aggregates. Demand enters an aggregate's braces only through its group variables; local
  variables and the target carry none, so a group is always whole.
- D10, bounds. `MaxBytes` and `MaxSteps` charge the rewritten evaluation, helper tuples included,
  and no bound is crossed with a nil error.
- D11, proofs and why-not, weaker on purpose. A proof under D is a valid derivation (rule from the
  plan, body tuples in M, acyclic) and deterministic. It is not promised to be M's "first
  derivation" (`eval.go:227`): first is an artefact of evaluation order, and D evaluates in
  another order. Likewise an `Absence` under D names the same `Failed` literal per rule, since the
  set of bindings a walk can reach is the same (D3), but `Bound` and `Blocked` may name a
  different witness where several exist, because "the first path keeps it"
  (`whynot_walk.go:133`) depends on relation order.

**Helland × Stonebraker.** Stonebraker asks whether a bottom-up evaluator should grow a goal
directed mode at all; the consumers already do it by hand. Shearline seeds every rule with the
request (`rbac.dl:1-3`) and Anam parameterises its set with `instant/1` (`anam.dl:11`,
`QUERY.md:112`). Both are a query smuggled into the rules, and the Shearline panel found what
that costs: one unseeded `holds/2` and every decision derived every principal's roles, tripping
the byte bound near 38,000 pairs (`01-hickey-armstrong.md:65-69`, confirmed at 50,000,
`decide_test.go:216`). Hand seeding is safety by discipline; demand is safety by construction, and
the evidence that an optimiser belongs here is that the consumers already wrote half of one
badly. Helland asks what leaves the engine: a receipt holds the answer hash and the rule-set hash
(Anam `QUERY.md:87-96`, Shearline `policy.go:157-186`), and D1 keeps both unchanged, so a receipt
cut under full evaluation verifies under demand and the receipt need not say which ran. The
tension that stays live: Stonebraker is right that once a query optimiser is in, the next asks are
join ordering, supplementary predicates and derived-relation indexes. This design draws the line
at *what is derived*: demand restricts the tuples, never the order the ordered body is joined
(`stratify.go:187`), and never a cost model. Anam's lazy `attr/3` construction
(`QUERY.md:183-186`) is base-side pruning and stays in Anam: who builds the base is the consumer.

**Ousterhout × Liskov.** One method, one type, one `Options` field, one accessor (§ 3). The
Liskov cost is that a `Result` from a demand evaluation is not a substitute for a full one on any
relation but the goal and the base. The design refuses those reads loudly (§ 3.3) rather than
split the type; § 7 Q2 names the alternative.

## 1. Technique

### 1.1 The candidates

*Relevance plus constant propagation.* Keep only rules reachable from the goal and push the
goal's constants into base lookups. It handles `allow("alice", "read", "/x")` on rbac: `P` stays a
single constant through `holds(P, Q), inherits(Q, R)`. It does not handle Anam: `regimen(At,
Drug)` has no constant at all; the saving comes from `current(E, At)` being needed only for the
set of reading instants, a set-valued demand that reaches `before/2` through `in_force/2`
(`anam.dl:43`, `anam.dl:15`). Nor does it handle any binding that passes through a join
(`holds(P, R) :- member(P, G), group_role(G, R)`: the demand on `group_role` is P's groups, a
set). Constants are the degenerate case of demand sets. Rejected as insufficient.

*Full magic sets with adorned predicate copies, supplementary predicates and subsumption.* The
textbook form. Adorned copies (`holds_bf`, `holds_bb`) multiply the relations a `Result` holds and
force a mapping layer under every accessor; supplementary predicates save re-joining prefixes at
the price of more hidden relations; subsumption saves little once copies share one relation.
Rejected as oversized for twenty-rule programs and three consumers.

*Magic sets, non-subsumptive, one relation per source relation.* Recommended, specified in § 2.
Each (relation, adornment) reachable from the goal gets one demand relation; each source rule gets
one copy per adornment of its head, differing only in a leading demand literal; every copy writes
into the source relation under its own name. Sound because every copy derives true tuples (D2);
complete per demand because each copy is complete for its own demand set (D3). Shared relations
mean proofs, why-not and `Result` see source names, which is most of D7 for free.

### 1.2 Compared

- *Answers identical.* All three keep D1 when correct; only the third keeps it over Anam's
  shapes. Correctness of magic sets under stratified negation holds when the rewritten program is
  stratified, which § 2.4 enforces.
- *Worst-case size.* Adornments per relation are at most 2^arity in theory; reachable ones in
  practice are a handful (rbac: `holds_bf`, `permit_bbf`, `deny_bbb`; anam: `in_force_fb`,
  `current_fb`, `before_fb`, `displaced_bb`, `closed_bb`, `latest_bb`, `later_bf`). Rules are
  copied once per head adornment and one demand rule is made per IDB body literal per copy, so the
  rewritten program is O(rules × adornments × body length). Work: a demand rule re-joins a prefix
  of its source body, so demand work is at most (1 + k) × A times the source rule's join, k the
  IDB atoms in its body and A its adornments, and is usually a small fraction of M's work. A
  constant `maxAdornments = 64` per relation bounds the copies; past it that relation's demand is
  dropped (§ 2.4), never an error, since dropping demand never changes an answer.
- *Stratified negation.* Magic rewriting can break stratification even when every demand rule is
  positive. Over the stratified program `y(A) :- d(A). x(A) :- b(A), not y(A). w(A) :- x(A).
  h(A) :- x(A), f(A), not w(A). g(A) :- w(A), h(A).` with goal `g_b`, the demand rule for `h`
  passes `A` from `w` (`m_h(A) :- m_g(A), w(A)`), `h`'s rule demands `x` (`m_x(A) :- m_h(A)`),
  `x` negates `y`, and `y`'s demand comes from `x` (`m_y(A) :- m_x(A), b(A)`): the cycle
  `m_h → w → x → not y → m_y → m_x → m_h` holds a negation. The published remedies are
  restricted SIPS that never pass a binding through a literal of a higher stratum into a lower one
  (Balbin, Port, Ramamohanarao and Meenakshi 1991; Kemp, Srivastava and Stuckey 1995) and Ross's
  modular stratification. Rather than carry a theorem, this design checks the rewritten program
  with `stratify` and drops demand, one relation at a time, until it passes (§ 2.4). A dropped
  relation is evaluated in full and still restricts what is below it.
- *Aggregates.* The inner body is already a closed unit with its group variables as its only
  free variables (`aggregate_eval.go:25`, `aggregate_check.go:139`). Demand into an inner atom
  binds exactly the group variables that occur in it plus what earlier inner positive atoms bind;
  the target and locals are never bound from outside (D9). The memo key is the group key alone
  (`aggregate_eval.go:142-153`), so a demanded group is computed once and whole. The inner atoms'
  edges stay negative (`stratify.go:73-76`), so the fallback of § 2.4 covers an aggregate cycle
  the same way.
- *Proofs and why-not.* With shared relations a derivation record names source relations already;
  the leading demand literal must be kept off the proof stack (§ 2.5), and `derivation.rule` must
  point at the source rule, not the copy. Why-not walks source rules over the result's relations
  (`whynot.go:191`, `whynot_walk.go:85`); under D those relations are complete for every binding
  the walk reaches from a goal inside the demand set, because the walk's bindings come from head
  unification and preceding positive atoms in the same order the rewrite passes them. A goal
  outside the demand set would read partial relations, so it is refused (§ 3.3).
- *Size in this codebase.* One new file for the rewrite (adornment worklist, demand rules,
  fallback loop), about 300 lines; two fields on `rule` (`plan.go:47`); one int compare in
  `join` under `ev.proofs != nil` (`join.go:55`); seeding in `Evaluate` (`eval.go:55-57`);
  guards in `Relation`, `Proof`, `WhyNot`, `WhyEmpty`; one accessor. No change to `order`,
  `stratify`, the semi-naive loop or the aggregate evaluator.

## 2. The rewrite

### 2.1 Adornment and worklist

An adornment is a string over `{b, f}` of a relation's arity: `b` where the goal or the context
binds the position. Starting from the goal's shape, a worklist in sorted order takes each
(relation, adornment) once. For each source rule of the relation, the body in evaluation order
(`order`, `stratify.go:187`, which already places guards as soon as they are bound and keeps
positive atoms in written order) is walked left to right with a bound set initialised from the
head's `b` positions. A positive atom over a derived relation is adorned by which of its
arguments are bound at that point (constants count as bound; `_` is free), and that pair joins the
worklist; after it, its variables are bound. A negated atom is adorned the same way (safety makes
every named variable bound, `plan.go:259-264`; a `_` is `f`); it binds nothing. An aggregate
literal adorns each inner positive atom with the group variables bound plus what earlier inner
positive atoms bind, and binds its result afterwards. Base relations, builtins and comparisons
are not adorned; a base relation is read whole and indexed as today (`base.go:124`,
`relation.go:71`).

The all-free adornment means "the whole relation". It has no demand relation and its rule copy is
the source rule unchanged. This is also how a dropped relation (§ 2.4) is represented, so there is
one mechanism for "evaluated in full" and no zero-arity relation anywhere.

### 2.2 Demand relations and rules

For each (r, α) with at least one `b`, a hidden derived relation `m?r?α` of arity |b| holds the
demand set. The name holds a byte `isName` (`plan.go:131`) refuses, so no consumer name collides
and no consumer can read, declare or seed it. For each source rule `r(args) :- body` and each
adornment α of r, one copy: `r(args) :- m?r?α(bound args), body`, with `ordered` set directly to
the demand literal followed by the source rule's ordered body (not re-ordered: a guard keeps its
place after the atom that bound it in the source, which keeps `Proof.Body` order equal to the
source's). For each adorned derived literal `s_β` in that copy, one demand rule
`m?s?β(bound args of s) :- m?r?α(...), P` where P is the smallest set of positive atoms from the
prefix that binds the variables s needs, found by walking the prefix backwards from s's bound
variables and adding an atom when it binds a needed variable, then its variables to the needed
set. Guards and negations are left out of demand rules; leaving them out makes a demand set a
superset of the exact one, which is still sound and complete (D2, D3) and only costs work
downstream. For rbac with the seeds removed:

```
holds(P, R)      :- m?holds?bf(P), role(P, R).
holds(P, R)      :- m?holds?bf(P), holds(P, Q), inherits(Q, R).
m?holds?bf(P)    :- m?permit?bbf(P, A).
permit(P, A, Pat):- m?permit?bbf(P, A), holds(P, Role), grants(Role, A, Pat).
m?permit?bbf(P, A) :- m?allow?bbb(P, A, R).
m?deny?bbb(P, A, R):- m?allow?bbb(P, A, R).
deny(P, A, R)    :- m?deny?bbb(P, A, R), denied(P, A, R).
allow(P, A, R)   :- m?allow?bbb(P, A, R), permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R).
```

The seed `m?allow?bbb("alice", "read", "/x")` is the one tuple `Evaluate` adds before the first
stratum runs. Note that `m?holds?bf` is one tuple however many principals the base holds, which
is the whole point.

### 2.3 Stratifying and ordering the rewritten program

The rewritten rules go through `dependencies`, `components` and the stratum levels of
`stratify.go:13-45` unchanged; `group` (`stratify.go:155`) is split so a caller can hand it rules
whose `ordered` is already set, and `recursive` positions are computed against the new strata as
today (`stratify.go:166`). The demand literal is a derived relation and may be a recursive
position, so the semi-naive loop (`eval.go:128-162`) re-fires a copy when its demand set grows,
with no new code.

### 2.4 Fallback

If `stratify` (`stratify.go:13`) refuses the rewritten rules, find a strongly connected
component holding a negative edge (`stratify.go:102` already computes components), take the
first demand relation in it by sorted name, and drop demand for its source relation: every
adornment of it becomes all-free, its copies collapse to the source rule, and occurrences of it
in bodies are not adorned and generate no demand rule. Rebuild and check again. Each round
removes at least one demand relation, so the loop ends, in the worst case at the source program.
The same drop applies to a relation whose adornments pass `maxAdornments`. The result is a
function of the plan and the goal shape (D5).

### 2.5 Evaluation

`Plan` gains an unexported pointer to a demand plan: the goal relation and adornment, the seed's
hidden name, and the rewritten strata and `idb`. `Evaluate` (`eval.go:41`) takes the rewritten
strata when the pointer is set, after checking `Options.Goal` against the shape, creates builders
for every rewritten `idb` name (`eval.go:55-57`), adds the seed tuple to the seed's builder and
charges it through `count` (`eval.go:250`), then runs as today. `rule` gains `source *Rule` and
`demand int` (the index of the demand literal in `ordered`, or -1). `record` (`eval.go:233`)
stores `source` when set; `join` (`join.go:55-57`) does not push the demand literal's tuple onto
the proof stack when `i == r.demand`. `TooLarge.Relation` for a hidden name is replaced by its
source relation before the error returns. Helper tuples are charged bytes and steps like any
derived tuple: the memory is real and the bound is a bound.

`accepts` (`plan.go:306`) needs no change: a base cannot hold a hidden name. `Arity`, `Defines`
and `HasAggregates` answer for the source plan.

## 3. API, additive

```go
// Goal is the question a demand evaluation answers: a relation, and per position a
// constant to hold it to or a variable to leave it open.
type Goal struct {
    Relation string
    Args     []Term   // a constant binds the position; a variable (any name, "_" included) leaves it open
}

// Demand compiles the plan to answer one shape of question: goals over relation whose
// bound positions are the b's of adornment ("bbf"), a string of b and f of the relation's
// arity. The constants come later, in Options.Goal. The returned plan evaluates to exactly
// what the receiver would, filtered to the goal (D1), and derives only what can reach it.
// ErrInvalid for an unknown or base relation or a malformed adornment.
func (p Plan) Demand(relation, adornment string) (Plan, error)

type Options struct {
    MaxBytes, MaxSteps int
    Proofs            bool
    Goal              *Goal   // required on a plan from Demand, refused on any other
}

// Goal is the goal a result was evaluated for; ok is false for a full evaluation.
func (r Result) Goal() (g Goal, ok bool)
```

The shape is compiled once and shared; the constants arrive per call. A rewrite per call was
considered (Q1): it would make `Options.Goal` the whole surface, but it puts a stratification pass
in front of every request, and a plan is a value consumers share.

### 3.1 Evaluate

On a demand plan, `Options.Goal` must name the plan's relation, have its arity, hold a constant at
every `b` and a variable at every `f`; a repeated variable name (`p(X, X)`) is refused, since an
adornment cannot express equality and a rule can. Else `ErrInvalid`. On a plain plan a non-nil
`Goal` is `ErrInvalid` with "compile the plan for the goal with Plan.Demand". The context, bounds
and builtin panic handling are as today.

### 3.2 Result

`Relation(g.Relation)` returns M's relation filtered to the goal's constants, the filter applied
because recursion legitimately derives tuples outside the goal (`path(Z, Y)` for every demanded
`Z`). `Relation(base relation)` is the base's relation, complete. `Relation` of any other derived
relation is `ErrInvalid`: "the result was evaluated for allow(…); holds is partial under that goal;
evaluate without a goal to read it". Never a partial answer with a nil error. A hidden name is
"no relation", as any unknown name is.

### 3.3 Proof, WhyNot, WhyEmpty

`Proof(name, t)` is as today when `name` is the goal relation and `t` agrees with the goal's
constants; otherwise ok is false. Its `Rule` is the source rule with `Set`, `Line` and `Text`; its
`Body` lists the source body's positive atoms. `WhyNot(name, t, depth)` likewise requires the goal
relation and a `t` inside the demand set, which is what makes the walk's reads honest (D3); for
any other goal ok is false. `WhyEmpty(name, depth)` is allowed only when `name` is the goal
relation and the adornment is all `f`; a partially bound goal relation is partial by design and
has no "empty". The weaker promise on witnesses is D11.

### 3.4 Shearline

`policy.Load` keeps compiling the sets and adds `allowPlan, _ := plan.Demand("allow", "bbb")`
and `permitPlan, _ := plan.Demand("permit", "bbf")`. `Decide` (`policy.go:157`) keeps
`p.base.With(req.facts()...)` for the context facts and calls
`Evaluate(ctx, allowPlan, base, Options{MaxBytes, MaxSteps, Proofs: true, Goal: &Goal{"allow",
[]Term{{Const: principal}, {Const: action}, {Const: resource}}}})`; `Proof("allow", want)` and
`WhyNot("allow", want, 0)` (`policy.go:176-180`) are unchanged. `Permitted` (`policy.go:194`)
evaluates `permitPlan` with the third position open and reads `Relation("permit")`, which is
already this principal's and action's, so the filter at `policy.go:210` becomes a check. With
that, `rbac.dl` may keep its seeds (belt and braces; its hash does not move) or drop them in a
later version; either way an unseeded consumer rule set no longer finds the cliff. The `TooLarge`
remedy text at `policy.go:315` finally says the true thing.

### 3.5 Anam

Anam compiles the bundled set and the query per request (`QUERY.md:65-72`), so
`plan.Demand(relation, strings.Repeat("f", arity))` for the answer relation costs nothing new,
and `Evaluate` runs with every position open. `instant/1` stays: without it `before/2` is unsafe
and Anam's refusal when it is undefined (`QUERY.md:179-181`) stays in Anam, as `DESIGN.md`
§ Declared and undefined already says. What changes is that a wide `instant`, up to
`instant(At) :- entry(_, _, At, _, _, _, _)`, now costs only the instants the answer relation
reaches, because `current_fb`'s demand is the set of fact-times `in_force` passes down. The
receipt is unchanged: the answer hash is the same (D1), and the rule-set hash is the text's.

## 4. Invariants as tests

- **Property, D1.** A generator over seeded `math/rand/v2` (the pattern of
  `aggregate_property_test.go:117-175`) builds programs from templates that mix a transitive
  closure, a filter through a stratified negation of a derived relation, a count or max over a
  derived relation grouped by a base relation, comparisons and `prefix`, over random bases with
  one to six base relations. For each program and a random goal (a reachable relation, a random
  adornment, constants drawn from the active domain and, one time in four, from outside it),
  compile `Demand`, evaluate both ways with no bounds, and require: `Relation(goal)` bytes and
  hash equal full's filtered to the constants; every other derived `Relation` is `ErrInvalid`;
  base relations equal. A floor on witnesses exercised, as the aggregate property has
  (`aggregate_property_test.go:124-129`). Seeds whose program is unstratified skip.
- **Property, D11.** For each tuple of the goal answer, `Proof` is ok under both and the demand
  proof is valid: its rule is a source rule by `Text`, the head unifies with the tuple, each body
  tuple is in the full result, and the DAG is acyclic. For random absent tuples inside the demand
  set, `WhyNot` is ok under both, with equal rule counts and equal `Failed` text per rule.
- **Stratification fallback.** The program of § 1.2 compiles under `Demand("g", "b")`, answers
  equal full, and an internal test asserts which relation's demand was dropped and that the
  order of dropping is a function of the program.
- **Aggregates.** A goal binding a group key derives the group whole: `n(G, N)` under `n_bf`
  equals full's `n` for that `G`, and an internal test asserts no inner local variable is ever a
  `b`.
- **Golden.** `golden_test.go` is not edited. No new frozen bytes: a demand answer is a full
  answer.
- **Determinism.** Two demand evaluations give equal hashes and deeply equal proofs; a demand plan
  built twice has equal rewritten strata (an internal test over the rule texts).
- **Bounds.** `MaxSteps` and `MaxBytes` crossed inside a demand rule return `*TooLarge` naming the
  source relation; the 50,000-principal unseeded base under Shearline's bounds (`policy.go:17-22`)
  fails in full and answers under demand. The fuzz seeds (`fuzz_test.go:18-23`) gain a goal
  path: parse, compile, `Demand` over a random reachable shape, evaluate under tight bounds,
  nothing panics.
- **Plain path untouched.** `BenchmarkEvaluate` (`proof_test.go:257`) within noise; a test that a
  plan without `Demand` records `Options.Goal == nil` and takes the source strata.

## 5. Cost model and evidence plan

The claim is not "demand is always cheaper". Demand's work is bounded above by (1 + k) × A times
the source work for the rules it keeps (§ 1.2) and below by nothing; a goal with every position
open over a program whose rules all read whole relations gains nothing and pays the demand rules.
The claim is that for the two consumer shapes cost follows the question. Two benchmarks in
`tarski`, each with a success criterion, and both consumers' suites green:

1. **Shearline's standing base, unseeded rules.** `testdata/rbac_unseeded.dl`: the four rbac
   rules with `principal(P)`, `action(A)` and `resource(R)` removed from `holds` and `deny`,
   `permit` seeded only by what binds it. A base of 50,000 principals each with one role, a
   twelve-role inheritance chain, 200 grants, 500 denials; one goal `allow("p1", "read",
   "/doc/x")`. Success: full evaluation under `MaxBytes = 1 << 20` returns `*TooLarge` (the
   cliff, reproduced); demand under the same bounds answers; demand's steps and wall time are
   within 2× of full evaluation over today's hand-seeded `rbac.dl` with the same request facts.
   That last number is the one that says demand replaces discipline.
2. **Anam's `in_force` over a large entry base.** `testdata/anam.dl` plus a query with `instant(At)
   :- entry(_, _, At, _, _, _, _)` over 20,000 entries in 300 series, 500 of them readings; goal
   `regimen_ff`. Success: demand within 1.2× of full evaluation with the hand-bounded `instant(At)
   :- entry(_, "health.blood-pressure", At, ...)`, and at least 10× below full evaluation with
   the wide `instant`. A second query with `instant` bounded by hand shows demand within 1.2× of
   full, the no-regression case.
3. **`BenchmarkEvaluate` and `BenchmarkEvaluate_Aggregate`** within noise.

To read steps without an error, § 7 Q8 proposes `Result.Cost() (steps, bytes int)`; without it the
benchmarks measure wall time and the cliff test measures the bound.

## 6. Staged implementation

Each slice leaves `~/Projects/go-quality/check.sh .` green and `golden_test.go` untouched.

1. **Internals, no API.** `rule.source`, `rule.demand`; `record` prefers `source`; `join` skips
   the demand push; `group` split to accept pre-ordered rules; `TooLarge` relation mapping hook.
   `BenchmarkEvaluate` gate.
2. **Rewrite without fallback.** `Goal`, `Options.Goal`, `Plan.Demand`, `Result.Goal`;
   adornment worklist, demand rules, hidden names, seeding; `Relation`, `Proof`, `WhyNot`,
   `WhyEmpty` guards. `Demand` refuses with "not yet" any plan whose rewrite `stratify` refuses
   and any plan with aggregates. Property test over programs with recursion and negation that
   stratify first time.
3. **Fallback.** The drop loop and `maxAdornments`; the § 1.2 program as a test; property test
   extended to every stratified program.
4. **Aggregates.** Inner-body adornment on group keys; property with aggregates; why-not through
   an aggregate on a demand result (`whynot_aggregate.go:20` reads the result's relations, which
   are complete for the group by D3).
5. **Evidence and release.** The two benchmarks and the cliff test; `Result.Cost` if approved;
   `DESIGN.md`, `README.md` roadmap, `CHANGELOG.md`; board posts to Shearline, Anam and Ductile
   with the adoption sketches of § 3.4 and § 3.5; tag v0.3.0.

## 7. Open questions for Matt

- **Q1, API shape.** `Plan.Demand(relation, adornment)` plus `Options.Goal` (recommended), or
  `Options.Goal` alone with the rewrite run on every `Evaluate`? Recommend precompile: a plan is a
  shared value, and a stratification pass per request is the wrong place for Shearline's budget.
- **Q2, partial relations.** Refuse non-goal derived relations with `ErrInvalid` (recommended), a
  new sentinel `ErrPartial`, or a separate `GoalResult` type with only the goal methods? Recommend
  `ErrInvalid`: reading a partial relation is a programming error, the message says what to do,
  and one `Result` keeps proof and why-not on one type.
- **Q3, proof identity.** Accept D11, a valid deterministic proof that may differ from full
  evaluation's first derivation where a tuple has several (recommended), or define proofs
  canonically (smallest derivation) so both modes agree? The canonical form would move v0.1 proof
  bytes and is a breaking change; recommend accept, document, and post to Shearline since
  `Decision.Proof` is consumer-visible.
- **Q4, why-not witnesses.** The same choice for `Bound` and `Blocked`. Recommend accept.
- **Q5, stratification fallback.** Drop demand per relation until the rewrite stratifies
  (recommended), refuse `Demand` with `ErrInvalid` and make the consumer restructure, or fall
  back to the whole source program? Recommend per relation: it never changes an answer, it is a
  function of the plan, and it keeps the win below the dropped relation.
- **Q6, adornment cap.** `maxAdornments = 64` per relation with the same drop (recommended), or
  refuse? Recommend drop; `MaxSteps` remains the cost bound a consumer sets.
- **Q7, helper bytes.** Charge hidden relations' tuples to `MaxBytes` (recommended; the memory is
  real and the bound stays a bound), or exclude them so a consumer's byte budget means the same
  tuples in both modes? Recommend charge, and say so in `DESIGN.md`.
- **Q8, `Result.Cost() (steps, bytes int)`.** Add it (recommended)? It is the evidence
  instrument for § 5, and Anam's 413 message could carry the numbers. Additive, two ints.
- **Q9, proofs of non-goal tuples.** Every derived tuple under demand has a sound proof; allow
  `Proof` on them? Recommend no: keep `Proof` and `WhyNot` to one domain, the demand set, so ok
  means the same thing on both.
- **Q10, Shearline's `rbac.dl`.** Keep the request seeds in version 1 (recommended; the hash
  holds and the seeds are now redundant, not wrong) and leave an unseeded version to Shearline's
  lead on the board.
- **Q11, Anam's default `instant`.** Whether the bundled set should ship a wide `instant` once
  demand bounds it is Anam's decision; recommend raising it on the board with the § 5 numbers,
  not deciding it here.
- **Q12, version.** v0.3.0 after v0.2.0 tags (recommended, as the card says), or fold into v0.2?
  Recommend keep them apart: aggregates' open questions should not wait on this.
- **Q13, supplementary magic and subsumption.** Defer (recommended) until a benchmark shows the
  prefix re-join matters.
- **Q14, repeated goal variables.** Refuse `Goal` args that repeat a variable (recommended), or
  treat them as open and ignore the equality? Refuse: silently ignoring an equality is a wrong
  answer with confidence.

## 8. Decided with Matt (2026-10-26)

All fourteen questions in §7 were settled on the recommendation:

- **Q1, API:** precompile per goal. `Plan.Demand(relation, adornment)` returns a rewritten,
  shareable Plan, and `Options.Goal` carries each request's constants.
- **Q2, partial relations:** asking a demand Result for any derived relation other than the goal
  is `ErrInvalid`, with a message saying how to ask for it.
- **Q3 and Q4, proof and why-not identity:** answers and hashes are identical. Proofs and why-not
  witnesses are valid and deterministic, but not promised byte-identical to full evaluation's
  first derivation. This is documented, and Shearline's lead is told.
- **Q5 to Q14, accepted as recommended:**
  - Q5: when the rewrite breaks stratification, drop demand per relation.
  - Q6: cap adornments at 64 per relation and drop the rest.
  - Q7: charge helper tuples to MaxBytes.
  - Q8: add `Result.Cost`.
  - Q9: proofs and why-not only for the goal relation.
  - Q10: Shearline keeps its seeds.
  - Q11: Anam decides its own instant/1.
  - Q12: v0.3 ships separately from v0.2.
  - Q13: no supplementary magic sets yet.
  - Q14: a goal that repeats a variable is refused.

Matt asked for the build to go ahead in the five slices of §6.

## 9. Evidence (2026-10-26)

Measured on master at slice 5 on an Apple M1 with `go test -bench`; steps and bytes are
`Result.Cost`, or the `*TooLarge` where a bound tripped. The tests beside the benchmarks
(`bench_demand_test.go`, `bench_demand_anam_test.go`) hold demand's answers to full
evaluation's, hash for hash, on the same bases.

### 9.1 Shearline (§5 benchmark 1): met

| run, one decision on 50,000 principals | ns/op | steps | derived bytes |
|---|---|---|---|
| seeded `rbac.dl`, full | 106,403 | 425 | 1,438 |
| unseeded, full, `MaxBytes` 1 << 20 | 40,327,806 | 91,340 | 1,048,585, then `*TooLarge` |
| unseeded, demand `allow_bbb` | 97,250 | 435 | 1,501 |

Demand over the unseeded rules is within 1.03× of today's hand-seeded rules in steps and no
slower in wall time; the cliff is reproduced, and demand under the same bound answers. For
Shearline's shape demand replaces the seeding discipline.

### 9.2 Anam (§5 benchmark 2): partly met

The 20,000-entry ledger §5 named is infeasible for full evaluation even with the hand-bounded
`instant` (nine minutes without finishing: `displaced/2` joins every `later/2` pair to every
instant), so the benchmark runs 900 entries in 30 series (small) and 6,000 in 300 (medium).
Wide full at medium runs under `MaxSteps` 10,000,000 and reports the work when it trips.

| scale | run | ns/op | steps | derived bytes |
|---|---|---|---|---|
| small | hand-bounded, full | 87,845,917 | 494,536 | 1,319,382 |
| small | hand-bounded, demand `regimen_ff` | 388,630,625 | 4,049,131 | 3,979,456 |
| small | wide, demand | 373,542,459 | 4,048,261 | 3,979,456 |
| small | wide, full | 10,492,431,500 | 240,035,625 | 34,915,013 |
| medium | hand-bounded, full | 1,852,486,042 | 22,946,676 | 27,899,112 |
| medium | hand-bounded, demand | 13,071,357,791 | 191,762,312 | 89,231,836 |
| medium | wide, demand | 13,792,409,166 | 191,752,412 | 89,231,836 |
| medium | wide, full, `MaxSteps` 10M | 9,439,686,542 | 10,000,001, tripped | 352,114,447 |

- **Wide demand costs what hand-bounded demand costs.** Demand trims `instant` to the fact-times
  `in_force` reaches, which was §3.5's point. Met.
- **Wide demand against wide full:** 28× fewer ns and 59× fewer steps at small scale. At medium
  wide full did not finish inside 10M steps, fewer than demand spends, so the ratio there is
  unmeasured. Met where measurable.
- **Demand against full on the hand-bounded `instant`:** 8.2× the steps and 4.4× the time at
  small, 8.4× and 7.1× at medium, 3× the bytes at both. The 1.2× criterion is **not met**.

Why, from the rewritten program (`Plan.Demand("regimen", "ff")` over `anam.dl` with the
hand-bounded query): the goal binds nothing, so every binding comes sideways from body atoms.
`m?displaced?bb(E, At) :- m?current?fb(At), before(E, At)` has a tuple per `before` pair, and
`m?before?bb(F, At) :- m?displaced?bb(E, At), later(E, F)` is a join the size of full
evaluation's `displaced` join; `before` and `displaced` are then derived again under those
demand relations, and `before` under two adornments (`fb` and `bb`). That is the (1 + k) × A
bound of §1.2 paid with no selectivity to pay for it: §5's "a goal with every position open
gains nothing and pays the demand rules", except that here it gains on `instant` and pays on
everything below it.

Nothing in the engine is wrong: answers, hashes, proofs and why-not match full evaluation in
every test, and the overhead is bounded as §1.2 says. The cost is the sideways
information-passing choice of §2.1 (pass every bound variable) meeting a shape where the goal
binds nothing. Options, for Matt:

(a) Ship as is. Anam decides its own `instant/1` (Q11) and measures with `Result.Cost`; the
    wide `instant` is now affordable, and the hand-bounded one stays cheaper.
(b) A restricted passing strategy: pass only bindings reachable from the goal's bound positions
    (here none below `instant`), which would leave `displaced` and `before` whole and keep the
    `instant` trim. A change in the rewrite's meaning, so a Lamport × Thomas/Hunt pass first,
    and the property tests must still hold.
(c) Supplementary magic sets (Q13, deferred), which share the sideways joins instead of
    repeating them.

Matt chose (a) on 2026-10-26: v0.3.0 is tagged as is, and (b) and (c) stay open as
follow-ups should a consumer need them.

### 9.3 Hot loop (§5 item 3): met

`benchstat`, eight runs each, master against slice 5: bytes and allocations per op identical in
all four `BenchmarkEvaluate` cases; times within noise (geomean +1.1%; `Evaluate/proofs=true`
+4.8% at p=0.028 with a ±6% spread on the branch, for a change that copies two integers into
the Result).

## Errata (2026-10-26)

1. **Open questions are answered.** §7 lists questions "for Matt" as written. Matt's answers are
   in §8, and the evidence, including the Anam criterion that was not met and Matt's choice of
   option (a), "ship as is", on 2026-10-26, is in §9. A reader should treat §7 as the questions
   and §8 and §9 as their outcome.
2. **Status and card path.** The opening lines said "targeting v0.3.0 after v0.2.0 tags" and
   cited `kanban/doing/`. The record is decided and released, and card 9 is in `kanban/done/`.
3. **Line citations.** They name master at the commit in the status header and have drifted
   since. Find the current code by symbol name.
