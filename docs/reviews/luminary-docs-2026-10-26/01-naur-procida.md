# Naur × Procida: tarski public docs at v0.3.0

*Axis: theory × need. Reviewed 2026-10-26 against the tree at `v0.3.0-1-gb045ed7`. That commit
adds only `okf/` to the tag.*

## Frame

The question is whether the docs state the promises accurately, at the right depth for a
consumer arriving cold, and whether they hide complexity or push it onto the reader. Naur's pass
reconciles every claim with the code. Procida's pass asks which of the four needs each page
serves. I ran `go doc -all .`, checked names, paths, constants and cross-references, ran the
named tests, and wrote a cold-start program from the docs alone.

**What held up.** It is worth saying first, because it is a lot.

- **Numbers agree.** The figures in CHANGELOG.md:26-33 match demand.md §9.1–9.2: 435 against 425
  steps, 97 against 106 µs, 28×, 59×, 8.2×, 8.4×, 4.4×, 7.1×, 3× and 1.03×. I recomputed each
  ratio from the tables. *Verified.*
- **Constants match.** `maxAdornments`, `maxProofGroup` and `maxNumberDigits` are 64,
  `whyNotDepth` is 8, `whyNotSteps` and `maxSumDigits` are 1<<16, `stepsPerDeadlineCheck` is
  1024 and `maxExponent` is 999 999 999. Each matches the source. *Verified.*
- **Every named test exists.** That covers `golden_test.go`, `aggregate_golden_test.go`,
  `anam_test.go`, `pressure_test.go`, the six fuzz targets (matching `ci.yml:55`),
  `TestDemand_CostFollowsTheGoal`, the cliff test, `BenchmarkShearline_Allow`,
  `BenchmarkAnam_Regimen`, and `BenchmarkEvaluate` at `proof_test.go:257`. The golden, Anam, cost
  and cliff tests pass with `-count=1`. *Verified.*
- **Cross-references resolve.** aggregates §13 and §12.1, and demand §8 Q11 and §9, all exist.
- **The engine code holds the "No vocabulary" rule.** A search of non-test `.go` files for
  consumer relation names finds none. *Verified.*
- **`go doc` is accurate and complete.** It is the best document in the repo.

---

## Naur: does the residue still carry the theory?

**N1 🔴 The OKF bundle (the repo's markdown knowledge bundle) points at theory that is not there.**
okf/index.md:7-11 names `api/`, `decisions/`, `examples/` and `reviews/`, and all four
directories are empty. 43 links inside `okf/` resolve to nothing. *Verified by resolving every
link.* One of them is `decisions/4-v0-freeze-policy.md`, cited as the authority for the freeze
promise at okf/packages/tarski.md:29 and okf/concepts/frozen-bytes-and-hashes.md:11. The
authority for the most important promise in the repo is unreachable. okf/log.md:4 admits the
gap, but a public bundle that admits its holes still has them. Either write the decisions, which
are the theory, or cut the index and every link to what exists.

**N2 🟠 The freeze has four different scopes, depending on which page you read.**
- **README.md:57-59** freezes answer bytes, the answer hash and the RuleSet hash. Constant text
  is not on that list.
- **CHANGELOG.md:3-4** adds "constants' canonical text" and cites README § What it promises
  consumers as its source. That section does not say it.
- **AGENTS.md § Frozen bytes** lists constant text.
- **Proof and Absence JSON** are treated as frozen by the design docs. DESIGN.md:144-145 says a
  proof without aggregates writes exactly its v0.1 bytes. demand.md:28 (D6) says no proof or
  absence shape moves. demand.md:405-406 (Q3) calls a canonical proof "a breaking change". The
  README only offers them as "Receipts as data" (README.md:69-70) and makes no freeze promise.

A consumer storing `Proof` JSON in a receipt cannot tell whether those bytes are promised. This
is the contract, so it needs one list in one place. *Verified by reading. The code freezes all of
these in practice.*

**N3 🟠 "Same inputs, same outputs" hides that a base is a sequence.** README.md:60-61 promises
that one (plan, base, options) gives the same answers, hashes, proofs and why-nots. The qualifier
is buried. DESIGN.md:131 says "a base is its fact sequence". DESIGN.md:284-286 says witness order
follows fact order, and "a proof pinned across a re-ingested base may" move. So the same facts in
a different order give the same answers and hashes, but possibly different proofs. The remedy,
"sort your standing facts", appears only as an aside about Shearline's `policy.Load`
(DESIGN.md:131, okf/concepts/proof.md:15). It is never stated as an instruction to consumers.
That is complexity moved onto the reader and then hidden from them. State it in the promise: a
set gives the same answers, and a sequence gives the same proofs.

**N4 🟠 DESIGN.md's own reconciliation rule is broken.** DESIGN.md:5-6 says "Where this file and
the code differ … fix one in the same commit." The file has drifted anyway:
- DESIGN.md:382 places `join` in `eval.go` and `walk` in `whynot.go`. They live at `join.go:10`
  and `whynot_walk.go:45`.
- DESIGN.md:402 refers to "the bundled `rbac.dl`". tarski bundles no rule set. `bundled.go` holds
  `RuleSet`, and `rbac.dl` is only a test fixture (NOTICE:12-15).
- DESIGN.md:90-94 shows `Options` without `Goal`, which the code has. It is mentioned separately
  at :301.
- DESIGN.md:270 still calls Soufflé's `_` projection "an open question for Matt". It was decided
  in aggregates.md:508-509 (§13 Q1).

*All verified against the code.*

**N5 🟠 DESIGN.md is a diff against code the reader has never seen.**
- `// unchanged` appears at :16, :43-45, :86 and :399. :88 says "Definitions parameter gone", and
  :400 says "Anam's `checkDefined` is removed". :3-7 and :10-11 are two overlapping provenance
  paragraphs.
- Because the language types are "unchanged", the contract never shows the fields of `Term`,
  `Atom`, `Literal` or `Rule`. A consumer has to go to `go doc` to learn that `Term{Var: "_"}`
  leaves a goal position open.
- DESIGN.md:7 sends the reader to Shearline's `docs/reviews/` and its "§ Decided with Matt" for
  the why. Those are in another repository. I did not verify that they are public or still at
  those anchors.
- Naur's test: if only this repo survived, the why behind the value/builder cut ("decision 6",
  DESIGN.md:392) could not be rebuilt.

**N6 🟠 The package doc is a v0.1 fossil.** doc.go:1-30 is the first thing `go doc` and
pkg.go.dev show. It never mentions proofs, why-not, aggregates, `WhyEmpty`, demand or `Cost`.
okf/packages/tarski.md:73 still cites doc.go at v0.2.2. *Verified.*

**N7 🟠 The contract names consumer vocabulary that the engine itself does not.** AGENTS.md says
"The engine never names a consumer's relations." DESIGN.md, the engine's contract, has 20 lines
naming Shearline, Anam, `rbac.dl`, `holds/2` or `policy.Load`. Examples are :110, :131, :157-161
("no role fact for P", "no grant for A"), :233, :240, :243 and :393 ("a concurrent `Policy`").
doc.go:15-16 says "a policy compiles once … evaluates per request", and the `Parse` godoc example
uses `reading`. README.md:15 uses `role` and `entry` to show they are only strings, which is fine.
The rule protects the engine's generality. When the contract reasons through one consumer's
policy, every other consumer has to translate it.

**N8 🟡 Fossils of "on master" and "doing" remain in released docs.**
- okf/log.md:9 says "Not released … Slice 5 … is not done", under the same date as :4, which
  records the release.
- okf/concepts/admission-rule.md and okf/packages/tarski.md:60 still say demand is "on master".
- okf/concepts/demand-driven-evaluation.md has the heading "Evidence on master" and cites
  `kanban/doing/9-…`, which is now in `done/`. The headers at aggregates.md:3 and demand.md:3 also
  cite `kanban/doing/`.
- aggregates.md:521 and docs/DECISIONS-PENDING.md cite "CHANGELOG § Unreleased", which no longer
  exists. DECISIONS-PENDING still says v0.3.0 is "building".
- Concept citations point at v0.2.2 blob URLs while describing v0.3.0 behaviour.

**N9 🟡 The godoc points readers at names they cannot see.** `Compile` says "(see checkShape)" and
`BoundNumber` says "(see maxSumDigits and maxExponent)". Both are unexported, so `go doc` will not
show them. Give the rule or the value instead. *Verified in `go doc -all` output.*

**N10 🟡 "No consumer input can make the engine panic" is broader than the code.** README.md:67-68
makes the claim. The code recovers only a panicking `Holds` (builtins.go:117). It does not recover
`Schema.Checks` (plan.go:293) or `Builtin.Check` (builtins.go:163). DESIGN.md:375 says so, but the
README's consumer-builtins contract (README.md:79-84) does not. Say "consumer data", and name the
two callbacks the consumer must not let panic.

**N11 🟡 demand.md's line citations have drifted.** The header at demand.md:8 pins them to "the
aggregates slice 4 commit", which is honest. But `eval.go:227` (:41) no longer points at `record`,
which now sits at eval.go:279. Freeze the doc as a dated record so nobody reads those citations as
live.

---

## Procida: which need is each page serving?

**P1 🔴 There is no tutorial, and the cold path is "read 687 lines of go doc".** README.md:27-36
"Use" gives `go get`, `go doc`, and a link to DESIGN.md, and the package has no `Example`
functions. *Verified.* I wrote the cold-start program myself. It parses, compiles, builds a base,
evaluates, then reads the Answer, the Proof, a WhyNot, and a `Demand` with `Cost`. It came to
about 30 lines and worked. My one error was guessing the shape of `Term`, because DESIGN.md:44
says "unchanged" and never shows it. The API is good enough that a tutorial is cheap. Without
one, the reader carries the beginner's success alone. Add `example_test.go` with
`ExampleEvaluate`, `ExampleResult_WhyNot` and `ExamplePlan_Demand`. They are compiled, run in CI,
and show on pkg.go.dev, so they are a residue that cannot drift. Naur would sign that too.

**P2 🟠 DESIGN.md is four quadrants on one page.** It holds reference (the Go signatures),
explanation (why Anam's lazy index raced, :392-393), history ("carried from", "unchanged", the
Shearline lineage), and maintainer internals (which unexported function in which file consults
the builtin table, :378-383). A consumer looking up the `MaxSteps` semantics has to wade through
history and internals. A maintainer finds the internals scattered through the contract. Split it:
- `go doc` is the reference.
- DESIGN.md becomes the promises and invariants only, with no history and no file names.
- A maintainer-notes page takes the internals.
- History goes to CHANGELOG and the design records.

**P3 🟠 The README buries how-tos inside the contract and the roadmap.**
- "With aggregates, set `MaxSteps`" (README.md:65-67) is an instruction placed inside a promise.
- "A consumer measures with `Result.Cost` before switching" (README.md:96-98) is the only guidance
  on when to use demand. It is a subordinate clause in the roadmap, yet it is the most important
  practical fact about v0.3.0. With every position open, demand cost about 8× the steps of
  hand-bounded full evaluation (demand.md:504-505).
- The reference is silent on cost. The `Plan.Demand` godoc says it "derives only what can reach
  the goal" with no warning, so a reader at the keyboard infers it is cheaper.

**P4 🟠 There is no how-to for adopting demand, or for choosing bounds.** The only adoption
recipes are demand.md §3.4 and §3.5. They are written for Shearline and Anam by name, cite
`policy.go` line numbers in another repo, and sit inside a design doc framed "for Matt's review".
Ductile, or a fourth consumer, has no recipe for:
- choosing an adornment;
- building `Goal.Args`, where a constant is `Term{Const: …}` and an open position is
  `Term{Var: "_"}`;
- knowing that `Relation` on a non-goal relation is `ErrInvalid`;
- comparing the two plans with `Cost`.

No page answers "what should I set `MaxBytes` and `MaxSteps` to?" either. Shearline's values are
referenced (demand.md:339, `policy.go:17-22`) but never shown.

**P5 🟠 The prose assumes a theory and never links to where it is supplied.** The README and godoc
use "least fixed point", "stratified", "magic sets" (README.md:94), "adornment", "semi-naive" and
"witness" without explanation. okf/concepts/ supplies them well, in least-fixed-point.md,
stratified-negation.md and demand-driven-evaluation.md. But nothing in README.md, doc.go or
DESIGN.md mentions `okf/`. *Verified by grep.* The explanation quadrant exists but is orphaned.
The explanations a reader can reach are 520-line design reviews with lens sections, rejected
alternatives and fourteen questions for Matt. That is a decision record, not an explanation.

**P6 🟠 There are two copies of the promise list, and they already disagree.** README.md:52-70 has
five promises. okf/packages/tarski.md:27-36 restates them and adds a sixth, "Terminating", that
the README does not promise. okf concept pages also restate DESIGN.md almost word for word: for
example, bounds.md restates DESIGN.md:95-104, and why-not-provenance.md restates DESIGN.md:152-205.
Every restatement is a place for drift. Each fact should live on one page, and the others should
link to it.

**P7 🟡 The design docs say nothing about their status.** aggregates.md:3 still opens "targeting
v0.2.0 … for Matt's review", and demand.md:3 still opens "targeting v0.3.0". Both shipped. A
reader cannot tell a live specification from a closed record. Add a one-line status header:
"Decided 2026-10-26, released in vX; this is the record, the contract is DESIGN.md."

**P8 🟡 This one is fine as it stands: CHANGELOG adoption notes.** The v0.2.0 steps for each
consumer (CHANGELOG.md:71-80) are upgrade how-tos, which is a changelog's proper job. Keep them
there rather than in DESIGN.md.

---

## Where they converge

1. **`go doc` is the truest page in the repo. Promote it.** It is current, complete and needs no
   reconciling. Both lenses want DESIGN.md slimmed to the promises and invariants `go doc` cannot
   express (N2–N5, P2). Both want Example tests as the tutorial (P1), since that is the one
   residue the compiler keeps honest.
2. **The theory is referenced but out of reach.** It sits in empty `okf/decisions/` (N1), in
   Shearline's repo (N5), and in review-framed design docs (P5, P7). Naur sees lost theory, and
   Procida sees a missing explanation quadrant. The fix serves both: short explanation pages in
   this repo, linked from the README.
3. **The freeze list and the determinism promise must be one list in one place.** Proof bytes and
   fact order are the two gaps (N2, N3, P6). This is the change consumers would actually feel.
4. **Consumer names belong in examples, not in the contract** (N7, P4). Move the Shearline and
   Anam adoption sketches into how-tos, which is what `okf/examples/` was meant to hold. The
   contract then speaks only engine.

## Where they pull apart

**T1. Keep the design records whole, or carve them up?** Naur says demand.md and aggregates.md are
the best theory residue here. They give the invariants first, the rejected techniques, the
stratification counterexample, the evidence, and the not-met criterion with its cause. Editing
them into tidy explanation pages strips the "why not the other way", which is exactly what a
successor needs to extend the engine. Procida says no reader arrives needing "fourteen open
questions for Matt". A cold consumer who wants to know why demand proofs may differ should not
have to read §0 lenses and §1.2. **Decide:** keep both. Freeze the design docs as dated,
status-headed records that are never edited again, so their line citations stop drifting (N11).
Write a short explanation page per feature that links into them. Do not average this into one
half-edited document.

**T2. The OKF bundle: fill it or prune it?** Naur says an index naming decisions that do not exist
is worse than no index, because it teaches the reader the theory is somewhere when it is not.
Write the decisions, because they are the theory. Procida says the concepts directory is already
the best explanation quadrant the repo has: ship what is real now, cut the four empty entries and
the 43 dead links, and add pages when a reader needs them. **Decide:** this is a resourcing call.
I recommend pruning today, since the repo is public as of today. Then write
`decisions/4-v0-freeze-policy.md` first, because the contract cites it.

**T3. Should the contract use a worked consumer case?** Naur says the theory of why why-not exists
is clearest through the rbac example, in sentences like "no grant for A on a held role"
(DESIGN.md:160-161), and abstract prose loses it. Procida, backed by the AGENTS.md rule, says a
reference that teaches through one consumer's vocabulary serves that consumer and makes everyone
else translate. **Decide:** the vocabulary rule is binding, so the contract stays abstract. The
rbac walk-through moves to an explanation or example page, where Naur's point is fully served.
