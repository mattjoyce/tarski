# Collation: luminary panel on the tarski documentation at v0.3.0

Reviewed 2026-10-26 at `b045ed7` (tag v0.3.0 plus the okf docs commit). Artifact: README.md,
doc.go, docs/DESIGN.md, CHANGELOG.md, docs/design/aggregates.md, docs/design/demand.md, NOTICE and
the okf/ bundle. Three pairs ran blind to each other: **Naur × Procida** (theory × need, both
passes), **Ousterhout × Liskov** (surface × substitution, After pass aimed at the documented
contract), **Thompson × Feathers** (trust × comprehension, first contact as a consumer engineer).
The question judged: does the documentation state the promises accurately, is it the right depth
for a consumer arriving cold, and does it hide complexity or move it onto the reader?

What held up, from all three: every benchmark figure, constant, named test and section
cross-reference checked is accurate; go.mod has no requires and nothing runs on fetch; NOTICE's
lineage is byte-accurate against both upstreams; coverage is 97.1%; the golden, property, fuzz and
cliff tests would go red on almost every README promise; `go doc` is the truest page in the repo.
The code is in good shape. The documentation around it is not.

## Verdict table

| Pair | Axis | One-line verdict |
|---|---|---|
| **Naur × Procida** | theory × need | The theory is referenced but out of reach: the okf index points at 43 dead links including the freeze-policy decision the contract cites, and there is no tutorial, so each stranger rediscovers the same 30-line path. |
| **Ousterhout × Liskov** | surface × substitution | The contract has four homes that disagree: the step unit is defined five ways and the frozen set three, and the copy a consumer reads first (the godoc) is the stale one. |
| **Thompson × Feathers** | trust × comprehension | Trustworthy to a high degree and good, with the docs the weak layer: no example to paste, DESIGN.md written as a diff against code the reader never saw, and CI's credibility spent on mutable inputs. |

## The shared meta-finding

**The contract has no single home, and the copy a cold reader hits first is the stale one.**
Naur × Procida reached it through reconciliation (N2, N4, N6: four freeze scopes, DESIGN.md's
broken same-commit rule, a v0.1 package doc). Ousterhout × Liskov reached it through the boundary
(O1, L1, L8: five definitions of a step, three of the frozen set, a godoc that promises a schema
check the code does not make). Thompson × Feathers reached it as a stranger (stale step wording at
eval.go:35 and builtins.go:18 is "the first text a stranger reads"). Three independent lenses, one
locus: README § What it promises, the godoc, DESIGN.md and the design records each carry a
different version of the same promise. The fix is structural, not editorial: one home per
promise, and every other page a pointer.

## Where the pairs converge

**C1 🔴 No tutorial, no `Example` functions.** N×P P1, T×F first 🔴, O×L O5 (asks for a godoc
example of the Demand/Goal pair). Two reviewers independently wrote a cold-start program from
`go doc` alone; both worked within two compiles; both stumbled on the same thing (the shape of
`Term` / `Const.Text()`). **Fix:** `example_test.go` with `ExampleEvaluate`,
`ExampleResult_WhyNot`, `ExamplePlan_Demand`. Compiled, run in CI, shown on pkg.go.dev: a residue
that cannot drift and a characterization test at once. README § Use points at it.

**C2 🔴 The unit of `MaxSteps` is stale where it is read first.** O×L O1 (verified six kinds of
step in code: join candidates, negation probes, rule applications per round, builtin calls,
aggregate groups, sum work), T×F 🟠 (eval.go:35, builtins.go:18, DESIGN.md:92 and :371 predate
v0.2.1/v0.2.2; the error text still prints "join steps"), N×P L8-equivalent via the builtins
wording. **Fix:** the full list once, in the `Options.MaxSteps` godoc; `Result.Cost`,
`TooLarge.Steps`, `ErrTooLarge`, `Builtin` and DESIGN.md:92 point to it; the error text drops
"join".

**C3 🔴 What is frozen is stated three ways and proofs fall between them.** N×P N2, O×L L1, T×F
(CHANGELOG:89 claims v0.1 proof bytes unchanged; no golden test pins proof bytes). README freezes
answer bytes, answer hash, rule-set hash. CHANGELOG and AGENTS add constant text and cite README
for it. DESIGN.md, aggregates I4 and demand D6 treat proof and why-not bytes as frozen; then D11
ships demand proofs that differ. **Fix:** one table in README § What it promises, one row each for
answer bytes, answer hash, RuleSet hash and ID format, constant text, Proof JSON, Absence JSON,
rendered text: frozen / stable within a minor / not promised. Name the test that pins each frozen
row; add a golden for proof bytes if they are frozen. CHANGELOG, AGENTS, DESIGN and the design
records point to the table.

**C4 🔴 The okf bundle ships 43 dead links on a public repo.** N×P N1 (verified every link; the
dead `decisions/4-v0-freeze-policy.md` is cited as the authority for the freeze promise by two
okf pages), T×F 🟡 (empty dirs are untracked so the links 404 on GitHub). **Fix:** today, cut
`api/`, `decisions/`, `examples/`, `reviews/` from okf/index.md and every link into them; then
write `decisions/4-v0-freeze-policy.md` first, since the contract cites it. See T2.

**C5 🟠 "Same inputs, same outputs" hides that a base is a fact sequence.** N×P N3, O×L L2.
Same facts in another order give the same answers and hashes but can give different proofs,
witness order and why-not bindings (DESIGN.md:131, :284-286). "Sort your facts" appears only as a
Shearline aside. **Fix:** one sentence in README's determinism promise: answers and hashes depend
on the set of facts; proofs, witnesses and why-not bindings also on their order; a consumer that
pins proofs sorts its facts.

**C6 🟠 DESIGN.md is a diff against code the reader never saw, and it has drifted.** N×P N4/N5,
O×L O2, T×F 🟠. Two "carried from" preambles (:3-11); `// unchanged` at :16, :43-45, :86, :399;
"Definitions parameter gone" (:88); `join` placed in eval.go and `walk` in whynot.go (:382, wrong);
"the bundled rbac.dl" (:402, tarski bundles none); `Options` block without `Goal` (:90-94); the
`_` witness question called open (:270) after it was decided (aggregates §13 Q1); the
`Evaluate` godoc's "built under a schema other than the plan's is ErrInvalid" (eval.go:53) where
the code checks name and arity only (L4). **Fix:** DESIGN.md in the present tense, promises and
invariants only; lineage into one History paragraph pointing at NOTICE and CHANGELOG; file names
and unexported internals to a maintainer-notes page; the five stale loci corrected.

**C7 🟠 Demand's one operating caveat is not where the API is read.** O×L O3/L5, N×P P3/P4, T×F
(notes README:96-98 has it). The `Plan.Demand` godoc promises "exactly what the receiver would"
with no mention that helper tuples are charged, that an all-open goal cost 8× the steps of
hand-bounded full on Anam's shape (demand.md §9.2), or that under the same bounds a demand plan
can be refused where the full plan answers (Liskov: a stronger precondition). **Fix:** two
sentences in the `Plan.Demand` godoc and in DESIGN.md § Demand, Answers: selectivity comes from
bound positions; measure with `Cost`; the same bounds may refuse the demand plan.

**C8 🟠 No page says how to choose a bound, and `Cost`'s real guarantee is unwritten.** O×L
O4/L3, N×P P4, T×F (Options seam: "no guidance on what number to start from"). `cost_test.go`
pins the recipe (deterministic within a version; a bound equal to `Cost` passes, one below is
refused) and the code makes `Cost` independent of `Options.Proofs`, but no doc says any of it,
and no test pins the Proofs independence. v0.2.1 and v0.2.2 both raised step counts, so a tuned
bound can refuse after a patch upgrade; nothing warns of it. **Fix:** a "Setting the bounds"
paragraph (README or DESIGN): evaluate a representative request, read `Cost`, set bounds with
headroom, re-measure after any upgrade. `Cost` and `MaxSteps` godoc state both halves. Add the
Proofs-independence test. See T4.

**C9 🟠 "No consumer input can make the engine panic" is wider than the code and the fuzz.**
N×P N10, O×L L9, T×F 🟠. `Schema.Checks` and `Builtin.Check` panics are not recovered
(DESIGN.md:375); only `Parse` input is fuzzed, while the v0.2.1 stack overflow came from a
hand-built AST. **Fix:** say "consumer data", name the two callbacks the consumer must not let
panic, and either fuzz hand-built `Program` values or scope the sentence to parsed text.

**C10 🟠 The contract names consumer vocabulary the engine does not.** N×P N7 (20 lines in
DESIGN.md naming Shearline, Anam, rbac, policy.Load), O×L O2, T×F ("bundled rbac.dl" is a
Shearline sentence carried over). AGENTS.md's rule binds the engine; the docs should obey it too.
**Fix:** the Shearline and Anam adoption sketches (demand.md §3.4, §3.5) and the rbac walk-through
move to how-to or example pages; DESIGN.md speaks only engine.

## The genuine cross-pair tensions

**T1 Freeze the design records whole, or carve them into explanation pages?** (Naur × Procida.)
Naur: demand.md and aggregates.md are the best theory residue in the repo (invariants first,
rejected techniques, the counterexample, the not-met criterion with its cause); editing them
strips the "why not the other way" a successor needs. Procida: no reader arrives needing fourteen
questions for Matt and a lens section. **Decision for the owner:** both lenses accept the same
resolution, so this one is settled rather than averaged: freeze each design doc as a dated,
status-headed record never edited again (fixes N11's drifting line citations), and write one
short explanation page per feature that links into it.

**T2 The okf bundle: fill it or prune it?** (Naur × Procida, with Thompson's "self-review as
evidence".) Naur: an index naming decisions that do not exist is worse than no index. Procida:
the concepts directory is already the best explanation quadrant the repo has; ship what is real
and cut the rest. Thompson: the reviews the bundle cites as assurance were agents directed by the
same author, and `okf/reviews/` is empty in the published tree, so their findings count only
where a test pins them. **Decision for the owner:** a resourcing call. Panel recommendation: prune
today (the repo went public today), then write the freeze-policy decision first.

**T3 One `Result` type for demand, or two?** (Ousterhout × Liskov.) Ousterhout: one type, one
accessor, the refusal fails closed with a message that says how to read the relation. Liskov: a
demand `Result` is not a behavioural subtype; code written against `Result` that reads two derived
relations works on one and fails at run time on the other, and only `Goal()` can tell them apart.
**Decision for the owner:** keep one type and add the sentence to the `Result` godoc ("a Result
evaluated for a goal answers a narrower contract; check Goal() before passing it to code that
reads arbitrary relations"), or introduce a `GoalResult` before v1.0 while consumers are few.
The lens leans to one type plus the sentence, recorded as a decision, not left implied.

**T4 Is the step count a versioned unit or an instrument?** (Ousterhout × Liskov; Thompson ×
Feathers on CI as the witness.) Ousterhout: unfrozen, so the engine can get cheaper without a
breaking change. Liskov: `MaxSteps` is frozen in spirit ("fail closed and bounded") while its unit
moved in two patch releases; a tight bound is exactly the consumer code that breaks. **Decision
for the owner:** either any change to step accounting needs a minor bump (version the unit), or
the docs say bounds need headroom and re-measuring after every upgrade and a patch may turn an
answer into `*TooLarge`. One or the other, written down.

**T5 Lineage prose in the contract.** (Thompson × Feathers, Naur × Procida, Ousterhout × Liskov
all touch it.) Thompson values "carried from shearline c4f75ab" because he checked it. Feathers,
Procida and Ousterhout want it out of the contract because `// unchanged` tells a stranger
nothing. Resolved the same way by all three: provenance to NOTICE and CHANGELOG where it is
verifiable; DESIGN.md in the present tense. Settled, not a live tension.

**T6 A worked consumer case in the contract.** (Naur × Procida.) Naur: why-not is clearest through
"no grant for A on a held role". Procida and AGENTS.md: a reference that teaches through one
consumer's vocabulary makes every other consumer translate. Resolved by the binding rule: the
contract stays abstract; the rbac walk-through moves to an example page.

## Single-lens signals

**Naur × Procida.** doc.go is a v0.1 fossil (no proofs, why-not, aggregates, demand, Cost) and is
what pkg.go.dev shows first (N6). Godoc points at unexported names `checkShape`, `maxSumDigits`,
`maxExponent` that `go doc` cannot show (N9). Two copies of the promise list already disagree
(okf/packages/tarski.md adds "Terminating") and okf concept pages restate DESIGN.md nearly word
for word (P6). Design docs carry no status header, so a reader cannot tell a live spec from a
closed record (P7). demand.md's line citations have drifted (N11). Fossils of "on master",
"doing", "CHANGELOG § Unreleased" in okf/log.md:9, okf/packages/tarski.md:60, aggregates.md:521,
DECISIONS-PENDING.md (N8).

**Ousterhout × Liskov.** A sum over a mixed-kind group drops the whole group from the Answer
despite aggregates.md:98 "nothing is ignored silently"; the only signal is WhyNot with Proofs on
(L6). The 64-witness cap is shared across all aggregates in a rule and GroupSize is their sum, so
aggregates.md:299 "read the rule text to split them" is false (L7). `WhyNot` and `WhyEmpty` call
`Holds` again after evaluation, uncharged, so the purity obligation covers the explanation too
(L9b). The adornment string restates the goal; five ErrInvalid messages exist to catch the two
copies disagreeing (O5). DESIGN.md:300 omits the "already compiled for a goal" refusal (L8).

**Thompson × Feathers.** NOTICE:12 pins no commit for anam.dl (it matches 45e4a67, which NOTICE:9
already names) and NOTICE:13-14 calls rbac_unseeded.dl "copied" when it is a derivative. CI
pipes golangci-lint's installer from HEAD into sh (ci.yml:30) and clones go-quality by mutable
branch v1 (ci.yml:34); actions pinned to major tags; tags and commits unsigned. The proxy did not
yet list v0.3.0, so README's literal `go get` fetched v0.2.2 without `Plan.Demand` (transient).
README:54-55's "three consumers must agree" reads as an outside check when author and consumers
are one person. The promise-to-test table: every README promise has a red-on-break test except
proof JSON bytes (author's word) and hand-built-AST panics (review, not fuzz).

## Build verdict

**Ship-with-fixes.** v0.3.0 is already out and nothing in the panel misstates an answer or a
hash; every finding is a documentation defect. The gate for a v0.3.1 docs release:

1. C1: `example_test.go` (three Examples), README § Use points at it.
2. C2: the step unit, once, in `Options.MaxSteps`; the other five places point to it; error text
   drops "join".
3. C3: the frozen-or-not table in README; a proof-bytes golden if proofs are frozen.
4. C4: prune okf/index.md and the dead links today.

Then the should-fixes (C5 to C10) as one docs card, and T3 and T4 as two decisions Matt records
before v1.0. Each pair's full findings: `01-naur-procida.md`, `02-ousterhout-liskov.md`,
`03-thompson-feathers.md`.
