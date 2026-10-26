# RBAC red/blue review — 2026-10-26

Reviewed tarski `f37ffea230435ee8dd8bab29d6b57bdacee4da16` and local Shearline policy integration `e3fddf55095438e6ef03d5780b11281cfefeb671`. Three independent After passes asked whether this engine is a defensible foundation for RBAC. Verdict: **suitable behind controlled Shearline admission; harden before treating it as a hostile-input sandbox.** No core authorization bypass was established. This is bounded audit evidence, not exhaustive verification.

| Pair | Axis | Verdict |
|---|---|---|
| [Ousterhout × Liskov](01-ousterhout-liskov.md) | surface × substitution | Strong copying and result boundaries; admission obligations and decoder gaps remain. |
| [Lamport × Thomas/Hunt](02-lamport-pragmatic.md) | system × self | Full/demand behavior agrees; malformed data can defeat negative authorization unless rejected at ingestion. |
| [Helland × Stonebraker](03-helland-stonebraker.md) | specialization × semantics | Request-scoped RBAC fits; evaluation bounds are not process-memory limits. |

## Shared finding

**Datalog fail-closed execution does not imply fail-closed policy design.** Boundary and semantic reviewers independently found that false/empty predicates can satisfy a negated condition. The workload reviewer found the same obligation at context/resource admission. Shearline already supplies important protections: one schema, rule-set verification, required allow/permit definitions, typed facts, aggregate rejection, request-seeded rules, fixed positive limits and deny on evaluation error.

## Ranked red findings and blue controls

| Priority | Finding and evidence | Required control / proposed fix |
|---|---|---|
| High if context is caller-controlled | A custom deny at hour >=22 denies with hour=23 but allows when hour is omitted. Reproduced through Shearline; `policy/policy.go:90` explicitly makes context optional. Documented integration hazard. | Require and attest security context before Decide; positively require security prerequisites. Test missing, spoofed and stale context in the service. |
| High if resource interpretation differs | Grant `docs/` permits `docs/../secrets/x`. Reproduced through Shearline. `policy/rbac.dl:19` uses literal prefix; the engine does not normalize paths. Exploitation requires downstream normalization/resolution to a different protected resource. | Authorize the same canonical resource the operation uses; reject traversal where applicable and use delimiter-aware scopes. Plain names need no filesystem semantics. |
| Medium availability | `relation.go:77` materializes all unbound candidates before `join.go:49` charges them. MaxSteps=1/MaxBytes=1 still allocated ~803 KB for 100k facts versus ~8.7 KB for 1k, then correctly refused. Lazy index construction is also uncharged. | Iterate candidates without a full temporary slice; check/charge expensive index construction. Meanwhile cap base sizes, arities, value lengths and concurrency. |
| Medium configuration hardening | Negative MaxSteps/MaxBytes bypass all `>0` checks (`join.go:95`, `eval.go:350`). Repro derives 10,000 tuples with both limits -1. Shearline's fixed positive constants mitigate this. | Reject negative options with ErrInvalid; preserve zero as the documented unlimited sentinel. |
| Medium at strict identifier ingestion | `json.Unmarshal` into Const accepts malformed UTF-8 and a lone surrogate, replacing both with U+FFFD (`term.go:107`). No internal hash collision demonstrated: normalization occurs first. | Reject malformed encodings before identity construction; define a consistent strict JSON admission contract. |
| Low API validation | Calling `Const.UnmarshalJSON([]byte("1]"))` directly succeeds (`term.go:110`); Decoder.More is not an EOF check. Ordinary json.Unmarshal rejects the surrounding malformed JSON. | Require EOF after the first value; add direct-method regression cases. |

Convergence C1: Ousterhout/Liskov + Lamport/Thomas/Hunt favor typed policy admission, preserving schema checks and rejecting incomplete required policy modules. An optional empty deny hook is not inherently a defect; only require modules that deployment policy expects.

Convergence C2: all pairs distinguish the engine's guarantees from end-to-end authorization. Keep attestation, resource identity, tenant isolation and revocation freshness explicit in the consuming service.

## Tensions retained

- Ousterhout wants one small consumer facade hiding validation complexity; Liskov requires its errors and preconditions to remain explicit.
- Lamport wants invalid security data to prevent authorization; query semantics intentionally omit mixed-type aggregate groups. Shearline's aggregate rejection resolves this for RBAC without changing generic query behavior.
- Stonebraker favors indexes/caches for cheap decisions; Helland requires snapshot identity and revocation freshness. Bind any decision cache to policy, standing-fact revision and authenticated request/context. No service cache was audited here.

## Single-lens signals

Boundary lens: public decoder strictness. Semantics lens: mixed-type aggregate poisoning and negated string guards reproduce in raw tarski; Shearline rejects aggregate policies and wrong-kind RBAC facts. Workload lens: eager allocation outside work charges and the need to identify fact snapshots in external caches/receipts.

## Blue evidence and retained tests

- Full go-quality gate passed before changes: formatting, vet, lint, govulncheck, race/shuffle. Supply-chain catalogue scan was skipped (BUMBLEBEE_CATALOG unset).
- Coverage: 97.2%. Anam bundled rule tests and frozen golden tests passed as part of the suite.
- Shearline policy suite passed with race/shuffle and a temporary module replacing tarski with this exact checkout.
- Bounded fuzzing, 15 seconds each / two workers: demand equivalence 43,958 executions; hand-built programs 697,229; parse/compile/evaluate 219,190. No failures; this short budget does not establish exhaustive safety.
- New `rbac_pressure_test.go`: cycle inheritance, explicit denial, unrelated-principal isolation, outside-prefix denial, role removal in a replacement snapshot; full/demand membership, proof and absence checks over reversed/duplicate facts. Step/byte/cancellation refusals must return a zero Result, with proofs both on and off.
- Final gate after adding tests: PASS, including race/shuffle; supply-chain catalogue scan still skipped. Ousterhout/Liskov After review found no blocking issue in the test diff. Cancellation probe covers cancellation before entry; proof assertions check availability, not explanation content.
- No engine implementation or frozen literals changed. Hot-loop benchmark gate is inapplicable because the hot loop was not changed.

## Reproduce unresolved behaviors

The `.go.txt` artifacts are deliberately outside the package test suite: they demonstrate current weaknesses/semantic hazards, not desired safety assertions. Copy to a temporary module and run; do not count successful reproduction as remediation.

```sh
mkdir -p /tmp/tarski-review-repro
cd /tmp/tarski-review-repro
go mod init review
go mod edit -go=1.25
go mod edit -require=github.com/mattjoyce/tarski@v0.3.1
go mod edit -require=github.com/mattjoyce/shearline@v0.0.0
go mod edit -replace=github.com/mattjoyce/tarski=/Users/mattjoyce/Projects/datalog
go mod edit -replace=github.com/mattjoyce/shearline=/Users/mattjoyce/Projects/shearline
cp /Users/mattjoyce/Projects/datalog/reviews/luminary-rbac-2026-10-26/pressure_test.go.txt pressure_test.go
go test -race -v .
go test -race -shuffle=on github.com/mattjoyce/shearline/policy
# Run these singly; each is a standalone main.
cp /Users/mattjoyce/Projects/datalog/reviews/luminary-rbac-2026-10-26/allocation_probe.go.txt /tmp/allocation_probe.go
go run /tmp/allocation_probe.go
```

For the allocation and semantic probes, copy the respective `.go.txt` to a temporary `.go` file and run `go run /tmp/allocation_probe.go` or `go run /tmp/semantic_probe.go` from that temporary module.

Build verdict: **ship with integration controls for trusted, bounded RBAC; hold deployments that accept unvalidated context, ambiguous resource identifiers, arbitrary rules, or unbounded inputs.** Track engine hardening separately; no fixes are claimed by this review.
