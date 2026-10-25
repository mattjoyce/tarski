# AGENTS.md: tarski engineering contract

Binding for humans and coding agents alike. Three things sit above the mechanics: the admission
rule in `README.md`, the API contract in `docs/DESIGN.md`, and the mentors below.

## Mentors

Every design review and code audit runs through the `luminaries` skill. Each pair is used for a
specific kind of change, and each pair's tension is reported, not averaged.

- **Ousterhout × Liskov** (*surface × substitution*) is the standing lens, applied to every
  change. Three consumers (Shearline, Anam and Ductile) share tarski's exported types, so every
  change is a boundary change.
  - Ousterhout asks whether the API stays small and deep: is complexity hidden, or only moved
    onto the caller?
  - Liskov asks whether the promise holds under substitution: frozen answer bytes, the
    determinism contract, consumer builtins honouring purity, and demand-driven evaluation
    returning exactly what full evaluation returns.
- **Lamport × Thomas/Hunt** (*system × self*) applies to any change in meaning: aggregates,
  demand-driven evaluation, stratification, numbers. Write the invariants before the code. That
  means the least fixed point, termination, determinism, and "every existing hash still holds".
  Then know why the code works, never just that it works.
- **Helland × Stonebraker** (*specialization × semantics*) applies before something is admitted
  to the engine. Does the engine still fit its workload? Does the feature belong in the engine at
  all, or in a consumer?

Use the luminaries *Before* pass on a design and the *After* pass on a diff.

## The rules

- **Admission rule.** A feature is in only if evaluation stays a pure, terminating,
  deterministic, boundable function of (plan, base). See `README.md`.
- **Stdlib only.** No dependency, ever. The engine's claim is absolute.
- **No vocabulary.** The engine never names a consumer's relations or concepts (role, entry,
  grant, facet, token, fact-time). Consumer words live in consumer rule sets and consumer
  builtins.
- **Frozen bytes.** Answer serialisation, `Answer.Hash`, `RuleSet.Hash` and the canonical text of
  a constant are frozen. A change to any of them is breaking. A test pins each one.
- **Deterministic.** Every walk over a map on the derivation path iterates in sorted order.
- **Fail closed.** Every bound crossed is an error and never a partial answer. Never panic on
  consumer input.

## Go standards

The gate is `~/Projects/go-quality/check.sh .`. A non-zero exit means not done. On top of it:

- Errors wrap with `%w` and are compared with `errors.Is` / `errors.As`. Don't log and return
  the same error.
- Exported names are documented. Defaults are named constants with a reason; invariants are
  constants with a reason they are not configurable.
- Functions stay under about 60 lines and files under about 400.
- Tests are table-driven, use real components and never mock the engine. Coverage stays above 80
  percent.
- `go test -race -shuffle=on` passes. A race failure blocks the merge.
- Benchmark-gate any change to the hot loop (`BenchmarkEvaluate`): keep it only within noise.

## Consumers

Shearline, Anam and Ductile import tarski. Anam and Ductile also import Shearline, and Go resolves
that diamond to one tarski version, with Shearline's `go.mod` setting the floor. So:

- Every release passes Shearline's policy suite, and Anam's `rules/anam.dl` test (`anam_test.go`)
  stays green.
- Consumer leads file asks on the board. They do not commit here.
- A change a consumer would feel gets posted on the board before it merges.

## Work

- Cards live in `./kanban/` (backlog, doing, done), one markdown card per item. Each card has a
  `## Job story`: "When [situation], I want to [motivation], so I can [outcome]."
- Keep the board true as you go. Write findings into the card while they are fresh.
- Commit in logical chunks with conventional subjects (`feat:`, `fix:`, `docs:`, `test:`,
  `chore:`). Run the gate green before any commit that touches Go.
- Branch per card (`card-<n>-<slug>`). Use a worktree when another agent is in the tree.
- Coordinate through the agent board (`board`). This repo's agent is `datalog-module`.
