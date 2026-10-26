# Lamport × Thomas/Hunt — semantic pressure review

Scope: tarski evaluation, stratification, demand rewrite and aggregates; After pass. No demonstrated violation of least-fixed-point, frozen-answer or demand-equivalence contracts in this pass.

## Lamport: safety properties must name their assumptions

**🟠 S1 — malformed aggregates can turn a denial into an allowance in a generic consumer.** `aggregate_eval.go:247–272` marks a mixed sum unusable, then returns no tuple and no error. Given `total(U,N) :- user(U), N = sum V : { weight(U,V) }. deny(U) :- total(U,N), N > 10. allow(U) :- user(U), not deny(U).`, weight 20 denies Alice; adding weight `"invalid"` grants Alice. Both full and demand evaluation do this. This is documented Datalog behavior, not an implementation contract violation. Invariant needed at the RBAC boundary: malformed security facts never become absence of denial. Validate numeric inputs before admission; favor positive validated authorization. **Current Shearline mitigation:** `policy/policy.go:141–145` refuses aggregate plans, so this example does not bypass Shearline Load.

**🟠 S2 — negative predicates fail open over wrong kinds unless the consumer validates kinds.** `builtins.go:78–79` returns false on wrong-kind input; builtin negation then inverts it. `allow(U) :- resource(U), not prefix("secret/",U).` derives `allow(123)` for numeric resource 123. Likewise `!=` regards unlike kinds as unequal (`join.go:162–163`). This is intentional, but a policy author's intuitive "invalid implies deny" is not the engine's invariant. **Current Shearline mitigation:** core RBAC relation checks enforce strings (`policy/facts.go:45`); preserve those checks, and type-check any custom context guards. No current Shearline bypass established.

## Thomas/Hunt: tested understanding

Blue evidence: `stratify.go:35–43` rejects cycles through negation and orders negative dependencies strictly below readers; aggregate dependencies are negative edges. `demand_rewrite.go:14–18` states soundness/completeness separately. Existing randomized full-versus-demand tests exercise recursion, negation, aggregate results and mixed values. Ran `go test -run 'TestDemand_.*|TestGolden_.*|TestAggregate.*' -count=1 ./...`: PASS (0.831s).

Red evidence: standalone executable `/tmp/tarski-semantic-pressure/main.go` reproduced S1/S2 with MaxSteps=10000 and MaxBytes=10000. Full and bound-demand answers agreed in all six targeted goals. Run `cd /tmp/tarski-semantic-pressure && go run .`. These are boundary counterexamples, not proof of an exploitable Shearline bug.

## Convergence and tension

Both voices require explicit, executable consumer invariants: bad data cannot authorize, aggregate plans are rejected by Shearline, wrong-kind RBAC facts are refused, and evaluation errors produce deny. Lamport asks for end-to-end invariants; Thomas/Hunt asks for tests that demonstrate why those invariants survive real adapter inputs. The tension: changing generic engine semantics to reject mixed groups would strengthen one security expectation while breaking its documented query semantics. Keep consumer validation explicit; do not silently redefine the engine.

Verdict: semantics reviewed are suitable behind Shearline's existing admission checks; preserve and pressure-test those checks. No new core semantic must-fix found; this is bounded test evidence, not exhaustive verification.
