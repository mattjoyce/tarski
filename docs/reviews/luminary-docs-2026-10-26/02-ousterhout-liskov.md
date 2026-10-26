# Ousterhout × Liskov: the documented contract of tarski v0.3.0

Lens: surface × substitution, After pass, aimed at the documentation rather than the code.
Reviewed 2026-10-26 at `b045ed7` (tag v0.3.0 plus the okf docs commit). Read in full: `README.md`,
`doc.go`, `docs/DESIGN.md`, `CHANGELOG.md`, `docs/design/aggregates.md`, `docs/design/demand.md`,
`AGENTS.md`, and `go doc -all .`. Checked against `eval.go`, `join.go`, `bounds.go`, `plan.go`,
`demand.go`, `demand_result.go`, `aggregate_eval.go`, `whynot_walk.go`, `golden_test.go`,
`aggregate_golden_test.go`, `render_test.go`, `aggregate_proof_test.go` and `cost_test.go`.
"Verified" below means I read the code path. Nothing in Shearline, Anam or Ductile was read, so
claims about what those consumers do are unverified.

**Frame.** Three consumers share the exported types, so every sentence in the godoc and the
README is a boundary promise. Ousterhout asks how much a cold consumer has to learn, and from how
many places, before they can use the engine safely. Liskov asks whether each promise names its
preconditions, postconditions and invariants, whether the code keeps it, and whether the code
keeps promises nobody wrote down that consumers will lean on anyway. Severity: 🔴 must-fix ·
🟠 should-fix · 🟡 consider.

## Ousterhout: is the documented interface deep?

The godoc is the best part of the surface. `Result.Relation`, `Result.WhyNot`, `Result.Cost`,
`Plan.Demand` and `Proof` each state their contract where a consumer will look, in a few
sentences. The trouble is that the godoc is only one of four places where the contract lives:
README § What it promises, the godoc, `docs/DESIGN.md`, and the decision sections of the two
design notes. Where they disagree, `DESIGN.md:5-6` makes the reader arbitrate: "the code is wrong
or this file is stale". That moves the complexity onto the reader rather than hiding it.

**O1 🔴 The unit of `MaxSteps` is defined five ways, and the one a consumer reads first is
stale.** The step is the unit of the engine's main safety bound. Its definitions:

- `eval.go:35` (godoc `Options.MaxSteps`) says "join steps (tuples visited)".
- `DESIGN.md:92` says the same.
- `bounds.go:9` (`ErrTooLarge`) and `bounds.go:25` (`TooLarge.Steps`) say "join steps". The error
  text at `bounds.go:54` prints "%d join steps".
- `eval.go:22` (godoc `Result.Cost`) says "tuples visited in joins, rule applications and builtin
  calls".
- `DESIGN.md:405-407` adds negation probes. `DESIGN.md:97-99` adds one step per aggregate group and
  the priced sum fold.

Verified in code, a step is charged for six things: a candidate visited by a positive join
(`join.go:48`), a candidate tried by a negation probe (`join.go:141`), each rule application
including every semi-naive re-application per recursive position per round (`eval.go:233`), each
builtin call (`join.go:190-193`), each aggregate group computed (`aggregate_eval.go:154`), and
`sumWork` for a sum's fold (`aggregate_eval.go:174`). No single sentence anywhere lists all six. The field doc names one of them. A consumer who sizes
`MaxSteps` from the godoc is working with a unit the godoc does not describe. Fix: put the full
list in one place, the `Options.MaxSteps` doc, and have `Cost`, `TooLarge.Steps`, `ErrTooLarge`
and `DESIGN.md` point to it. Drop "join" from the error text, or accept that a consumer may
match on it.

**O2 🟠 `DESIGN.md` is written as a diff against a history the reader does not have.**

It has two preambles naming different ancestors, Shearline at `DESIGN.md:3-7` and Anam at
`:10-11`. The API block annotates types as "// unchanged" (`:16`, `:43-45`, `:86`) and
"Definitions parameter gone" (`:88`), relative to Anam's `internal/query`, which a cold consumer
has never seen. It cites Shearline's `policy.Load` (`:110`, `:131`, `:233`) and Anam cards (`:204`)
as reasons for engine behaviour. This is obscurity in Ousterhout's sense. The contract a consumer needs is mixed in with
provenance the consumer cannot use. Fix: move the lineage into one "History" paragraph, and
state each clause as it stands today.

**O3 🟠 Demand's "when should I use it" lives in the README roadmap and design § 9, not in the
API.** `README.md:96-98` carries the one warning a consumer needs: "with every position open it
can cost more than a hand-bounded full evaluation, so a consumer measures with `Result.Cost`
before switching". The godoc of `Plan.Demand` (`demand.go:43-58`) says only that the plan
"derives only what can reach the goal". `DESIGN.md` § Demand-driven evaluation does not mention
the regression either. The evidence is in `demand.md:504-505`: demand cost 8.2× the steps and 3×
the bytes of full evaluation on Anam's hand-bounded shape. A roadmap entry is the wrong home for
an operating caveat. Fix: one sentence in the `Plan.Demand` godoc saying selectivity comes from
bound positions, an all-`f` goal can cost more, and the consumer should measure with `Cost`.

**O4 🟠 Nothing tells a consumer how to choose a bound.** README tells consumers to set
`MaxSteps` with aggregates (`README.md:65-67`, `CHANGELOG.md:79-80`), and v0.3 shipped the
instrument that makes this possible. The calibration recipe is already pinned by
`cost_test.go:20-77`: `Cost` is deterministic, a bound equal to the cost passes, and one below
it is refused. No doc states that recipe. Fix: add a short "Setting the bounds" paragraph that
says to evaluate a representative request, read `Cost`, set the bounds with headroom, and
re-measure after any upgrade (see L3).

**O5 🟡 The adornment string restates what the goal already says.** `Plan.Demand(relation,
"bbf")` followed by `Options.Goal` with constants at exactly the `b` positions means the consumer
writes the shape twice. Five of the `ErrInvalid` messages at `demand.go:122-138` exist only to
catch the two copies disagreeing. Q1 (`demand.md:396-398`) decided to precompile, with good
reasons. The docs could still shrink what the caller has to know. A helper that derives the
adornment from a template `Goal`, or one godoc example showing the pair, would do it.

---

## Liskov: are the promises stated, and kept?

**L1 🔴 The frozen set is stated three different ways, and proofs fall between them.**

| Source | What it says is frozen |
|---|---|
| `README.md:57-59` | Answer bytes, Answer hash, RuleSet hash. |
| `CHANGELOG.md:3-5` and `AGENTS.md` § The rules | Those three, plus the canonical text of constants. |
| `DESIGN.md:287`, `aggregates.md:25-26` (I4) and § 6, `demand.md:28` (D6) | Those, plus proof and why-not bytes. |

`demand.md:405-406` (Q3) says a canonical proof form "would move v0.1 proof bytes and is a
breaking change". Then D11 (`demand.md:39-45`, decided at `:445-447`) ships a mode whose proofs
differ from full evaluation's.

Verified against tests: `golden_test.go` pins answer bytes, answer hash, the RuleSet ID with its
`name/version hash` format, and constant text. Proof JSON is pinned only incidentally
(`render_test.go:104`, `aggregate_proof_test.go:153-155`), and Proof and Absence `String()` text
by `render_test.go:40-58`, with neither labelled frozen or not. A consumer that stores `Decision.Proof` in a receipt cannot tell whether that is safe across
versions. I did not verify whether Shearline does this. Fix: one canonical list in README, which
the other files point to. For each of Answer bytes, Answer hash, RuleSet hash, RuleSet ID
format, constant text, Proof JSON, Absence JSON and the rendered text, say "frozen", "stable
within a minor version", or "not promised". Then label the tests that pin each one.

**L2 🟠 "Same inputs" hides a precondition: a base is its fact sequence, not its fact set.**
`README.md:60-61` promises that one (plan, base, options) gives the same answers, hashes, proofs
and why-nots. A `Base` has no equality, and its relations keep insertion order
(`relation.go:9`, verified). Two bases built from the same facts in different orders therefore
give the same answers and hashes, but can give different proofs, different aggregate witness
order and different why-not bindings.

The only places that say so are a clause inside the `Proof` comment at `DESIGN.md:131`, "a base
is its fact sequence", and `DESIGN.md:284-286`. The second notes that "a proof pinned across a
re-ingested base may" move. Shearline sorts its facts to get around this (per `DESIGN.md:131`,
unverified). Anam and Ductile would have to find the same trap on their own. Fix: state in
README § What it promises that answers and hashes depend on the set of facts, while proofs,
witnesses and why-not bindings also depend on fact order. Tell a consumer that pins proofs to
sort its facts.

**L3 🟠 `Cost` is "an instrument, not frozen", but the docs leave out the guarantee consumers
will actually rely on.** The godoc (`eval.go:21-27`) says what `Cost` is not. It does not say what
it is: deterministic for one (plan, base, options) within a version, and exactly the counter the
bounds compare against. That property is the reason `Cost` exists, and `cost_test.go:20-77` pins
it. It is also independent of `Options.Proofs`. I verified that from the code: proof recording
and witness logging charge no step. No test compares the two cases, though, and no doc says it.
A consumer will calibrate with `Proofs: false` and run with `Proofs: true`, so this should be
written down and tested.

The flip side is not written down either. "May change between versions" means a `MaxSteps` tuned
on one version can refuse on the next. This already happened in two patch releases: v0.2.1 and
v0.2.2 both raised step counts (`CHANGELOG.md:41-44`, `:62-65`). The `Options.MaxSteps` doc does
not warn about this. Only `Cost` does. Fix: state both halves on `MaxSteps` and `Cost`. Within a
version, the count is exact and deterministic, and `Cost` is the calibration. Across versions,
the count is not stable, even across patch releases, so the consumer re-measures after an
upgrade. Also say whether `TooLarge.Steps` and `Bytes` on a refusal are on the same scale as
`Cost`. They are, verified at `join.go:78` and `eval.go:317`.

**L4 🟠 The `Evaluate` godoc promises a schema check the code does not make.** `eval.go:53` says
"A base built under a schema other than the plan's is ErrInvalid". `Plan.accepts`
(`plan.go:316-329`, verified) checks only that the base does not hold a relation the plan
derives, and that each base relation the plan reads exists with the right arity. A base built
under a schema with different `Checks` or `Builtins`, or with extra relations, is accepted.
`DESIGN.md:105-111` describes this correctly. A consumer reading the godoc would believe the
engine enforces "same Schema", and so believe its `Checks` held for every fact. They do not.
Fix: make the godoc say "compatible by name and arity only", as `DESIGN.md` does.

**L5 🟠 A demand plan substitutes for the full plan only when both stay inside the bounds.**
`Plan.Demand` (`demand.go:46-48`) promises that the plan "evaluates to exactly what the receiver
would ... filtered to the goal's constants". `README.md:94` says "Answers do not change". Both
are true when both evaluations succeed. Demand charges its helper tuples to `MaxBytes` and their
joins to `MaxSteps` (Q7, `DESIGN.md:318-319`). Under the same `Options`, a demand plan can
therefore return `*TooLarge` where the full plan answers. `demand.md:490-491` shows this on
Anam's shape: 4,049,131 steps against 494,536. In Liskov's terms, the substitute has a stronger
precondition, a larger budget. Fix: add "with the same bounds, a demand evaluation can be refused
where the full one answers" to the `Plan.Demand` godoc and to `DESIGN.md` § Demand-driven
evaluation, Answers.

**L6 🟠 A mixed-kind group drops out of the answer without a trace, despite "nothing is ignored
silently".** `aggregates.md:97-98` says "Nothing is ignored silently and nothing is coerced".
The value is not coerced, but the whole group vanishes from the `Answer`. This happens for a
`sum` over a group holding one string, and for a `min` or `max` over a group mixing kinds. The
answer for that group is the same as for a group that does not exist.

The only signal is `WhyNot`, which has three conditions. It needs `Options.Proofs`. The consumer
has to know which tuple to ask about, and for a sum it does not know the value. Under demand it
works only inside the goal. The postcondition the README and godoc should state is this: an
aggregate's answer omits any group whose values cannot be combined, and `WhyEmpty` or `WhyNot`
is how to find such a group. `DESIGN.md:271-274` states the behaviour accurately. The claim in
the design note is stronger than the behaviour. Fix the sentence, and put the omission in the
`Aggregate` godoc (`go doc`, type Aggregate), which today says nothing about empty or mixed
groups.

**L7 🟡 The witness cap is shared across several aggregates, so the per-aggregate split cannot
be recovered.** `groupOf` (`aggregate_eval.go:110-131`, verified) concatenates the witnesses of
every aggregate in the rule, cuts the total at 64, and sums the sizes. When the first aggregate
fills the 64 slots, the second contributes no witness. `GroupSize` reports only the sum, so a
receipt cannot tell how many witnesses belong to each aggregate. `aggregates.md:298-299` says "a
receipt that needs them apart can read the rule text". That is false once the cut falls or the
sizes are summed. The `Proof` godoc (`proof.go:13-17`) is accurate but silent on this case. Fix:
say "64 in all, not per aggregate" and drop the "read the rule text" claim. Alternatively, keep
per-aggregate sizes, which would be an additive field.

**L8 🟡 Stale and contradictory clauses that a cold reader will take at face value.**

- `DESIGN.md:268-270` still calls witness semantics "an open question for Matt". It was decided
  at `aggregates.md:508-509`.
- `aggregates.md:521-522` points to "CHANGELOG § Unreleased", which no longer exists.
- `builtins.go:18` and `DESIGN.md:371` say each builtin call "follows a charged join step". Since
  v0.2.2 each call is its own step (`join.go:189-193`), which is a stronger guarantee than the one
  written.
- `DESIGN.md:300` omits a refusal the godoc lists: `Demand` on a plan already compiled for a goal
  is `ErrInvalid` (`demand.go:57-62`).

**L9 🟡 The panic and purity promises are wider in README than in the code.** `README.md:68-69`
says "No consumer input can make the engine panic". `Schema.Checks` and `Builtin.Check` are not
recovered (`DESIGN.md:375-376`), so a consumer callback that panics during Compile escapes. Either
"consumer input" means data and not callbacks, in which case say so, or the claim is too wide.

Separately, the builtin contract is worded only for `Evaluate`. `WhyNot` and `WhyEmpty` call
`Holds` again after evaluation (`whynot_walk.go:57-58`, verified), and those calls are not
charged one step each. They are bounded only indirectly, by `whyNotSteps` times the body's
length. The purity obligation therefore covers the explanation as well as the answer. If `Holds`
is impure, a why-not can contradict the answer it explains. Say so in § Consumer builtins.

---

## Where the two converge

- **One home per promise.** O1, O2, L1 and L8 are the same defect seen from both sides. The
  contract is spread over four documents, and the stale copies make promises the code no longer
  keeps. Ousterhout counts that as cost to the reader. Liskov counts it as an unstated contract.
  The godoc is the most accurate surface (Cost, Relation, WhyNot, Proof), so make it the
  authority for API behaviour. Keep `DESIGN.md` for invariants only, and keep README's promise
  list as the single frozen-or-not table.
- **Write down what the code already guarantees.** The cost calibration in L3 and O4, the
  fact-order dependence in L2, and the bound asymmetry under demand in L5 are each kept by the
  code, pinned or measured, and missing from the docs. Consumers will depend on all of them. Each
  needs one sentence. None needs code.

## Where they pull apart

**The demand `Result`: one type or two?** Ousterhout endorses the decision in `demand.md:64-67`
and Q2. One `Result` type and one accessor is the smallest surface, and the refusal message
(`demand_result.go:41-43`) says how to read a non-goal relation. Liskov's objection, which the
design names itself, is that a demand `Result` is not a behavioural subtype of a full one. Code
written against `Result` that reads two derived relations works on one and fails at run time on
the other. The type checker cannot tell them apart, and `Result.Goal()` is the only way to
check. Splitting would add a type, roughly a dozen lines of godoc, and a second set of methods.
Keeping one type means every consumer helper that accepts a `Result` must document whether it
accepts a demand one. **To decide:** keep one type, which is the current state, and say in the
`Result` godoc that "a Result evaluated for a goal answers a narrower contract; check Goal()
before passing it to code that reads arbitrary relations". Or introduce a `GoalResult` before
v1.0, while consumers are still few. I lean toward keeping one type plus the sentence, because
the refusal fails closed. That choice gives up the substitutability Liskov would want, and it
should be recorded as a decision, not left implied.

**`Cost`: an instrument, or part of the contract?** Ousterhout wants it unfrozen, so that the
engine can get cheaper without a breaking change. Liskov notes that `MaxSteps` is frozen in
spirit ("fail closed and bounded") while its unit is not. A tight bound set by a consumer is
exactly the code that breaks when a patch release changes the count, as v0.2.1 and v0.2.2 did.
Two answers are possible. Version the unit, so that any change to step accounting needs a minor
bump. Or document that bounds need headroom and re-measuring after every upgrade, and accept that
a patch can turn an answer into `*TooLarge`. The panel should pick one rather than leave both
half-implied.
