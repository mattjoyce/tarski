# Thompson × Feathers: first contact with tarski v0.3.0

*Axis: trust × comprehension. Pass: first-contact. Reviewer seat: a Go engineer at a consumer
who found `github.com/mattjoyce/tarski` today and must decide, docs first and repo second,
whether to depend on it.*

## Frame

The panel's question is whether the documentation states the promises accurately, at the right
depth for a cold arrival, and whether it hides complexity or moves it onto the reader. This lens
splits that in two. Thompson asks what I take on faith and whether the repo backs it. Feathers
asks whether the documented behaviour is the observed behaviour, and whether a test would tell
me if it stopped being so.

What I ran, all read-only against the repo at `b045ed7` (tag `v0.3.0` is `d5e6261`; the one
later commit is docs-only):

| Check | Result |
|---|---|
| `go vet ./...` (go1.27.3) | exit 0 |
| `go test -count=1 ./...` | ok |
| `go test -race -shuffle=on .` | ok |
| `go test -cover .` | 97.1% of statements |
| CI on the release commit (`gh run list`) | success, both jobs |
| `go list -m -versions` via the public proxy | v0.1.0 to v0.2.2 only; `@latest` is v0.2.2 |
| `go list -m ...@v0.3.0` | resolves |
| Docs-only consumer program in a scratch module | compiles, runs, byte-identical across runs |

Independence note: one of my greps for a stale phrase also matched two lines of
`docs/reviews/luminary-docs-2026-10-26/02-ousterhout-liskov.md`. I had already logged both
loci (`eval.go:35`, `builtins.go:18`) from `go doc` before that grep. I read nothing else
under `docs/reviews/`.

## Thompson: what am I taking on faith?

**The supply chain is as small as the README says, and I checked it.**
`go.mod` is two lines, with no `require`, and there is no `go.sum`. A search for
`go:generate`, `import "C"`, build tags and cgo found nothing. Nothing in the module runs on
`go get`, `go build` or `go test` beyond Go's own toolchain. `.claude/` is untracked, so it
does not ship. README.md:29 ("standard library only") and doc.go:28 ("Nothing here reads a file
or the network") hold. The blast radius of a hostile tarski is whatever a pure function and a
consumer's own `Holds` callbacks can do inside my process, and nothing more. That is the best
answer this question can get.

**The lineage in NOTICE is accurate, and I verified it against the upstream repos.**
- `testdata/anam.dl` is byte-identical to `anam` `internal/query/rules/anam.dl` at `45e4a67`.
  It differs from Anam's current master, which has moved on.
- `testdata/rbac.dl` is byte-identical to `shearline` `policy/rbac.dl` at `c4f75ab`, and to
  its current master.
- Both upstreams are by the same author under Apache 2.0, and LICENSE is the stock Apache 2.0
  text.

Two wording issues follow.

- 🟡 **NOTICE:12 pins no commit for `anam.dl`.** It says "copied unchanged from Anam's
  internal/query/rules/anam.dl", and Anam's file has since changed. A reader who diffs against
  Anam today sees a difference and suspects tampering. The copy matches `45e4a67`, the commit
  NOTICE:9 already names for the number port. Name it at line 12 too.
- 🟡 **NOTICE:13–14 says "the rule sets under testdata/ named for Shearline are copied".**
  No file is named for Shearline. `rbac_unseeded.dl` is a derivative: its own header says the
  seeds were removed. Say "`rbac.dl` is copied unchanged from Shearline's policy/rbac.dl at
  c4f75ab; `rbac_unseeded.dl` is derived from it for the demand benchmarks."

**The CI that underwrites three README promises is weaker than the promises.**
README.md:67–68 says "the fuzz suites in CI hold that", and README.md:107–110 names the gate.
The promise that nothing panics therefore rests on CI. The workflow does run, and it is green on
`d5e6261`. What runs inside it is mutable.

- 🟠 **.github/workflows/ci.yml:30 pipes a mutable script into a shell.** It runs
  `curl ... golangci-lint/HEAD/install.sh | sh`. The binary version is pinned, but the
  installer script is fetched from `HEAD`.
- 🟠 **ci.yml:34 clones `go-quality` by the mutable branch `v1` and runs its `check.sh`.**
  The gate is the author's own repo, so its trust collapses into the author. That is fine, but
  then it is not a second witness.
- 🟡 **The actions are pinned to major tags** (`actions/checkout@v4`, `setup-go@v5`), not to
  SHAs. `permissions: contents: read` limits the damage, and that is good.

None of these touch a consumer at `go get` time. They matter because the README spends CI's
credibility. Pinning the script by SHA, and go-quality by tag or SHA, would make "CI holds that"
mean one fixed thing.

- 🟡 **The tags and commits are unsigned.** `git tag -v v0.3.0` reports no signature, and
  `%G?` is `N` on the last three commits. For a solo-author stdlib-only module, the Go checksum
  database is the practical anchor, and it will pin v0.3.0's bytes once fetched. Say so, or
  sign.
- 🟡 **The proxy does not yet list v0.3.0** (verified, transient). `go get
  github.com/mattjoyce/tarski` resolved v0.2.2 in my scratch module, and `@latest` returned
  v0.2.2. An explicit `@v0.3.0` resolves. A consumer following README.md:32 today gets
  a version without `Plan.Demand`, then finds every v0.3 doc symbol missing. This will
  self-heal. Until then, `go get github.com/mattjoyce/tarski@v0.3.0` is the line that works.
- 🟡 **README.md:54–55 says breaking a frozen thing needs "three consumers" to agree.** Author
  and consumers are one person, so a stranger gets the changelog and the golden tests, not a
  veto. That is fine for v0.x, but the line should not read as an outside check.

## Feathers: what does it actually do, and would a test tell me?

**The quick-start cannot be followed cold, because there isn't one.**
- 🔴 **README.md § Use (lines 27–36) has no code.** It gives `go get` and `go doc`, then points
  at DESIGN.md. The package has no `Example` functions either (verified: none), so pkg.go.dev
  will show none. The brief asked me to paste the README example. There is nothing to paste.
  So I wrote one from `go doc` and DESIGN.md alone: Parse, Schema with a consumer builtin,
  Compile, NewBase, Evaluate with bounds and Proofs, Relation, Hash, Cost, Proof, WhyNot,
  Demand with a Goal, and a `MaxSteps: 1` overrun. It compiled on the second try and behaved
  exactly as documented (verified):
  - the answer bytes were sorted and canonical;
  - the demand answer for `path("a", Y)` was the full answer filtered, at 12 steps against 19;
  - the overrun error named the bound and suggested a fix;
  - the proof and why-not text read well;
  - two runs gave the same output digest.

  The first compile failure was my guess at `Const.String()`; `Text()` is the accessor, and
  `go doc Const` says so. So the API is learnable from godoc. The README just makes every
  stranger rediscover the same ten-line path. One `example_test.go` with that path would also be
  a characterization test that runs in CI, so the quick-start could never rot.

**DESIGN.md is a contract written as a diff against code the reader never saw.**
- 🟠 **DESIGN.md:3–11 has two "Carried from" preambles.** The first says the text came from
  Shearline. The second says it came from Anam. Between them is a stray double blank line,
  the trace of a splice.
- 🟠 **The API block narrates change against predecessors.** It has `// unchanged`
  (DESIGN.md:16, 43, 44, 45, 86), `// Definitions parameter gone` (88), and "Anam's
  `checkDefined` is removed" (400). These words only mean something to someone who held
  Anam's `internal/query`. A stranger reads them as noise or as a hint that the real contract
  lives elsewhere. This is complexity moved onto the reader: the history is the author's, and
  the reader pays to filter it out.

**Where the documented and the actual diverge.**
- 🟠 **The step-bound wording is stale in godoc and in two DESIGN lines.** `Options.MaxSteps`
  godoc (eval.go:35) and DESIGN.md:92 say MaxSteps counts "join steps (tuples visited)".
  `Builtin` godoc (builtins.go:18) and DESIGN.md:371 say each builtin call "follows a charged
  join step". Since v0.2.1 and v0.2.2, rule applications and builtin calls are steps
  themselves (CHANGELOG.md:41–44, 62–65; DESIGN.md:405–408; README.md:79–81). The godoc is the
  first thing a stranger reads, and it is the stale copy. My overrun said "2 join steps" for
  one rule application plus one candidate. The error text carries the same stale word.
- 🟠 **DESIGN.md:269–270 calls the multi-literal `_` witness semantics "an open question for
  Matt".** CHANGELOG.md:84–86 and docs/DECISIONS-PENDING.md:16 say all aggregate questions are
  settled. One of the two is wrong, and the contract file is the one a consumer trusts.
- 🟡 **DESIGN.md:401–403 speaks of "the bundled `rbac.dl`".** tarski bundles no rule set.
  bundled.go holds `RuleSet` only, and `rbac.dl` is a test fixture. This is a Shearline
  sentence carried over unedited.
- 🟡 **DESIGN.md's `Options` block (lines 90–94) omits `Goal *Goal`.** It reappears 200 lines
  later at line 301. A reader of the block sees a struct that does not compile against v0.3.
- 🟡 **docs/DECISIONS-PENDING.md:17 points to "CHANGELOG § Unreleased"**, which no longer
  exists.
- 🟡 **The okf bundle advertises rooms that do not exist on GitHub.** okf/index.md:6, 8, 9
  and 11 link `api/`, `decisions/`, `reviews/` and `examples/`, and about 40 lines across the bundle (by grep, unverified one by one)
  point into them. Those directories are empty, so git does not track them, and every one of
  those links 404s on GitHub. okf/log.md:4 admits this. A public bundle should not ship
  dead links: drop them from the index, or fill them.

**Which promises would a test catch breaking?** I mapped each README § promise to a test.

| Promise (README.md) | Test that fails if it breaks | Verdict |
|---|---|---|
| Answer bytes and hash frozen (57–59) | golden_test.go:36 `TestGolden_AnswerBytesAndHash`; aggregate_golden_test.go:30 | held by test |
| RuleSet hash frozen | golden_test.go:81 `TestGolden_RuleSetID` | held by test |
| Constant canonical text frozen (AGENTS.md) | golden_test.go:91 `TestGolden_ConstantText` | held by test |
| Same inputs, same outputs, proofs included (60–61) | proof_test.go:197, whynot_test.go:212, base_internal_test.go:152, demand_test.go:330, aggregate tests | held, in-process only |
| Shareable across goroutines (62–63) | base_test.go:211 `TestEvaluate_ConcurrentOverOneBase`, consumer_builtins_test.go:219, under `-race` | held by test |
| Context checked (64) | aggregate_bounds_test.go:80–96 cancels inside `Holds` | held by test |
| `*TooLarge`, never partial (64–66) | aggregate_bounds_test.go, boundaries_test.go | held by test |
| No consumer input panics (67–68) | 6 fuzz targets, 30 s each in CI (ci.yml:52–56) | held for text input; see below |
| Panicking `Holds` is `ErrInvalid` (81–82) | consumer_builtins_test.go:281 | held by test |
| Demand answers equal full (CHANGELOG:12–16) | demand_property_test.go:318, FuzzDemandMatchesFull, demand_aggregate_property_test.go:157 | held by test and fuzz |
| Proof JSON bytes unchanged since v0.1 (CHANGELOG:89) | render_test.go checks keys and shape; no golden pins v0.1 proof bytes | author's word |
| Builtin purity | consumer's contract, by design | not the engine's to test; documented |

That is an unusually honest table. Almost every promise has a test that would go red.
Three places are softer.

- 🟠 **"No consumer input can make the engine panic" (README.md:67–68) is fuzzed only through
  `Parse`.** `Compile` also takes a hand-built `Program`, because the AST types are exported, and
  the v0.2.1 stack overflow came from exactly that kind of AST (CHANGELOG.md:59–61). No fuzz
  target builds ASTs directly (fuzz_test.go has no `Program{`). The fix was found by an external
  review and is pinned by a unit test, so the promise for hand-built ASTs rests on review. Either
  fuzz the AST or scope the sentence. Separately, DESIGN.md:375–376 says `Builtin.Check` and
  `Schema.Checks` panics are not recovered. That is reasonable, since they are the consumer's
  code, but the README sentence does not carve it out.
- 🟡 **CHANGELOG.md:89 says no v0.1 proof moves, but no golden test pins proof bytes.**
  Proof JSON is not on README's frozen list, so this is a changelog claim beyond the contract.
  Either pin one proof's bytes or say that proof JSON is stable but not frozen.

**Seams: can I use them from the docs alone?**
- **`Schema.Builtins`: yes.** README § Consumer builtins (72–84) is the best section in the
  repo. It splits the contract into what the engine guarantees and what the consumer
  guarantees, and it says whose fault an impure `Holds` is. I declared one from it without
  reading the engine. The only miss: no doc says that `Holds` reads values through
  `Const.Text()` and `Kind()`. Godoc for `Const` does say it.
- **`Options`: mostly.** The godoc is good, apart from the stale step wording above. README.md
  :65–67 rightly tells aggregate users to set `MaxSteps`. A newcomer still has no guidance on
  what number to start from. `Result.Cost` now exists to measure that, and README.md:97–98 says
  so for demand. A one-line "set MaxSteps from Cost on a representative base, with headroom"
  would close it.
- **`Plan.Demand`: yes, and it is the deepest piece of documentation here.** The godoc states
  the shape, the fallback, the aggregate boundary and the error cases. DESIGN.md:306–340 adds
  that a non-goal derived relation is `ErrInvalid` and that proofs may differ from full
  evaluation's. README.md:96–98 also warns that an all-open goal can cost more than a
  hand-bounded full plan. That last one is the kind of complexity that should be surfaced and
  not hidden, and it is.

**Could a stranger change this safely from the docs alone?** Mostly yes, by the test suite
rather than by the docs. Coverage is 97%, the golden tests pin the bytes, a property test and a
fuzz target pin demand against full evaluation, and AGENTS.md states the rules. A stranger who
breaks a frozen byte or a demand answer would see red. What they could break silently: proof
JSON (not pinned), the wording drift above (no test reads docs), and the hand-built-AST panic
surface.

## The two verdicts

**Is it good? (Feathers.)** Yes. The engine is well characterised. Most promises have a test
that would fail, and the seams are documented by contract rather than by example. The
documentation is the weak layer:
- no runnable quick-start and no `Example`;
- a DESIGN.md that reads as a migration diff from Anam and Shearline;
- four small stale passages, two of them in godoc;
- an okf bundle with dead rooms.

None of it misstates an answer or a hash. It costs the cold reader an hour that one example file
would save.

**Can I trust it? (Thompson.)** Yes, to the degree a stdlib-only pure function can be trusted.
That degree is high. No dependencies, nothing executes on fetch, the lineage is accurate to the
byte, and the licence is clean. The residual faith is in one author, three consumers who are
that same author, unsigned tags, and a CI whose installer and gate are fetched by mutable
reference. For a consumer, the checksum database pins the bytes once fetched, and that is enough.

## Where they converge

- **The missing example is both lenses' first finding.** Thompson wants the smallest thing he
  can account for, and Feathers wants a characterization test. An `example_test.go` is both:
  the shortest honest account of the API, run by CI, shown on pkg.go.dev.
- **README spends CI's credibility** ("the fuzz suites in CI hold that"). Thompson asks
  whether CI is a fixed witness. Feathers asks whether CI fuzzes the input that broke last time.
  Both answers are "not quite": pin the CI inputs, and fuzz hand-built ASTs.
- **Stale godoc is a trust and a comprehension defect at once.** The first text a stranger
  reads (eval.go:35, builtins.go:18) understates what MaxSteps counts.

## Where they pull apart

- **Lineage prose.** Thompson values DESIGN.md's "carried from shearline c4f75ab" lines,
  because that is provenance he can check, and he did. Feathers wants them out of the contract,
  because "// unchanged" tells a stranger nothing about behaviour. The resolution is to move the
  history to NOTICE and CHANGELOG, where it is verifiable, and keep DESIGN.md in the present
  tense.
- **Self-review as evidence.** The okf log and CHANGELOG cite an external pressure review and
  luminary audits as assurance. Feathers counts a found-and-pinned bug as real evidence.
  Thompson notes that the reviewers were agents directed by the same author, and the
  `okf/reviews/` they would live in is empty in the published tree. Their findings are
  verifiable only where a test pins them.
