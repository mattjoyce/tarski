---
id: 12
status: done
---
# RBAC luminary and red/blue pressure review

## Job story
When using tarski for Shearline RBAC, I want independent contract, semantics and workload reviews plus adversarial probes, so I can identify authorization and availability weaknesses before deployment.

## Scope
Review and evidence only; no engine fixes. Three mandatory mentor pairs. Run quality gate, coverage, bounded fuzzing, and Shearline policy suite against this checkout.

## Narrative
- 2026-10-26: Clean starting tree; branch card-12-rbac-pressure-review. Full quality gate passed; supply-chain catalogue check skipped because BUMBLEBEE_CATALOG is unset.
- 2026-10-26: Completed three independent mentor reviews and red/blue probes. Report: `reviews/luminary-rbac-2026-10-26/00-collation.md`. Confirmed eager candidate allocation outside limits, negative limits disabling bounds, and JSON decoder normalization/trailing-delimiter gaps. Context omission and resource interpretation are conditional consumer hazards; no core authorization bypass established.
- 2026-10-26: At user request added `rbac_pressure_test.go` for cyclic inheritance, denial, principal isolation, replacement-snapshot revocation, full/demand membership and proof/absence availability, and zero results on bounded/cancelled refusal. Full gate passed after addition. Existing coverage 97.2%; three short fuzz runs and local Shearline policy race/shuffle suite passed. No engine fixes or frozen-byte changes. Follow-up hardening is card 13.
