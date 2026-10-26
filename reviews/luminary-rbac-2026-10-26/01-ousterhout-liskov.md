# Ousterhout × Liskov — surface × substitution

Scope: repository After/code-audit pass, RBAC boundaries. Static review with decoder candidates subsequently confirmed by the pressure-test runner.

## Ousterhout: does the boundary hide enough?

- **Should-fix integration hazard:** shape compatibility is not validation provenance. `plan.go:320–332` accepts a Base with matching names/arities even if constructed without the plan schema's Checks. `builtins.go:77–79,122` makes a wrong-kind string guard false and its negation true. Thus `allow(X) :- subject(X), not prefix("blocked/", X).` admits numeric X when ingestion omitted type checks. This is documented engine behavior, not an engine correctness bug. Shearline should own one schema/base constructor, validate identifier kinds, and test malformed inputs against negated guards.
- **Should-fix integration hazard:** declarations without definitions are valid empty hooks (`plan.go:67–72`). A missing deny hook can make a negated denial succeed. Require expected policy relations to be `Defines`-true before activation. Test missing policy modules, not only malformed syntax.
- **Should-fix hardening:** negative bounds silently mean unlimited (`join.go:95`, `eval.go:350`), although Options documents zero as the unlimited sentinel. Reject negatives on Evaluate entry; Shearline should require positive configured limits.

## Liskov: does the promise survive adverse inputs?

- **Confirmed low-severity validation gap:** `Const.UnmarshalJSON` uses `Decoder.More()` as an EOF check (`term.go:103–115`). Direct calls with `1]` or `true}` accepts trailing invalid input (confirmed for `1]`). Standard `json.Unmarshal` checks overall syntax first, so normal wrapper use is protected. Replace More with a second Decode requiring EOF; test the public method directly.
- **Consider input consistency:** JSON strings pass through encoding/json replacement decoding (`term.go:107,114–115`). Invalid UTF-8 and lone surrogate escapes become U+FFFD before NewBase sees them, unlike Parse's rejection. This is normalization before identity, not two distinct internal constants colliding. Strict receipt/identifier ingestion should reject malformed encoding before decoding; add JSON-path tests.
- **Positive:** Compile clones AST/maps before callbacks (`plan.go:77–86`); callback atom arguments are detached (`plan.go:292`); base tuples are copied before validation (`base.go:102–119`); Proof copies rules (`proof.go:105–117`); answers clone tuples and bytes (`answer.go:49–56,84–100`). No immutability defect found in these paths.
- **Positive:** demand results refuse unrelated derived relations rather than exposing partial answers (`demand_result.go:37–45`); goal arguments are copied (`demand.go:140,204`).

## Convergence and tension

Both lenses favor one consumer policy-admission boundary that validates schema, required definitions, positive bounds, and rule identity before evaluation. Ousterhout favors hiding these requirements in a small Shearline facade; Liskov insists the facade preserve explicit errors and distinguish engine validity from policy safety. Keep the generic engine small; do not mistake generic Datalog's documented false/empty semantics for RBAC fail-closed semantics.

Verdict: engine boundaries are carefully defended, but production RBAC readiness depends on consumer admission checks. No demonstrated unauthorized grant under a correctly typed and complete policy was found by this static pass.
