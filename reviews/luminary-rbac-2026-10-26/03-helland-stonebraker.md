# Helland × Stonebraker — RBAC workload audit

After pass, 2026-10-26. Reviewed tarski evaluation/resource paths and local Shearline integration. No demonstrated authorization bypass from this lens.

## Stonebraker: fit and cost

**Should-fix / medium availability: eager candidate allocation escapes fixed evaluation budgets.** `relation.go:77–80` allocates and fills an index slice for every tuple of an unbound relation; `join.go:45–49` charges the first candidate only afterwards. With `out(X) :- edge(X).`, MaxSteps=1 and MaxBytes=1, a standalone probe against this checkout allocated 8,728 bytes for 1,000 base facts and 803,352 bytes for 100,000 facts before refusing at step 2. No derived answer escaped. The settings bound charged work and derived text, not transient allocation or latency independent of base size. Lazy index construction (`builder.go:35–43,64–70`) is likewise outside accounting. Replace unbound candidate materialization with iteration and charge/check cancellation during index building. Until then cap admitted facts, arity and value lengths; preserve request-bound joins. Probe source: `/tmp/tarski-workload-probe/main.go`.

**Positive:** Shearline's `policy/rbac.dl` seeds closure with the request principal, avoiding unrelated principals' closure. `policy/policy.go:141–145` rejects aggregates for per-request decisions; lines 175–179 set both budgets and return errors before allowing. This is a well-specialized consumer over a vocabulary-free engine.

## Helland: meaning across the boundary

**Should-fix in integration / medium, conditional:** `shearline/policy/policy.go:78–82` identifies rule sets in a decision but not the standing-fact snapshot. `Answer.Hash` (`answer.go:64–65`) hashes the relation and tuples, not policy/base identity. Neither establishes that revocations are current, nor binds a cached authorization to its fact revision. At the service boundary, cache by policy+standing-facts revision and authenticated request/context; invalidate on revocation. This is an integration obligation, not an engine determinism defect.

**Conditional authorization hazards, already documented:** grants use literal prefixes (`shearline/policy/facts.go:69–73`), so a grant for `tenant/1` also matches `tenant/10`; exact deny facts do not deny descendants (lines 75–78). Context declarations validate types but do not attest or require context (`policy.go:90–93`); omitted context may disable a deny rule. Extra rule sets can add `allow` rules and bypass bundled deny (`policy.go:100–104`). Red tests should cover these exact cases. Blue controls: normalized segment-aware resources, authenticated mandatory context, trusted rule-set authorship, and service-level revocation tests. These are explicit contracts, not undisclosed bugs.

## Convergence and tension

Both favor immutable policy snapshots, bounded evaluations and rules scoped to the request. Stonebraker favors reusable indexes/materialization to keep decisions cheap; Helland requires every such cache to carry snapshot identity and revocation semantics. Decide the freshness contract before adding caching. Keep resource admission/iteration improvements inside tarski; keep tenant vocabulary, attestation and revocation ownership in Shearline's consuming service.

Verdict: suitable for trusted, bounded RBAC inputs with integration controls; do not represent Options as a process memory sandbox. Fix candidate allocation for stronger hostile-load resilience.
