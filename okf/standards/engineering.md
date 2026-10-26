---
type: Standard
title: Engineering standards
description: "The Go standards tarski holds to and the gates CI runs: the go-quality gate, an 80 percent coverage floor, bounded fuzz runs, golden tests and a benchmark gate."
tags: [standard, go, ci, testing, lint, fuzz]
timestamp: 2026-10-26T00:00:00Z
---

# Standards

| Area | Standard |
|---|---|
| Language | Go 1.25 or later, standard library only. No dependency, ever: the engine's claim is absolute. |
| Gate | go-quality (`~/Projects/go-quality/check.sh .`): gofmt and goimports, vet, golangci-lint (errorlint, funlen 60, revive, forbidigo, gosec), govulncheck, `go test -race -shuffle=on`. A non-zero exit means not done. |
| Coverage | At least 80 percent, enforced in CI. |
| Fuzz | Six fuzz targets, 30 seconds each in CI: `FuzzParseCompileEvaluate`, `FuzzTransitiveClosure`, `FuzzNegationAndRepeatedVariables`, `FuzzNumberRoundTrip`, `FuzzNumberAdd`, `FuzzDemandMatchesFull`. They hold the promise that no consumer input makes the engine panic. |
| Size | Functions under about 60 lines, files under about 400. Card 2 records `whynot.go` over budget at 510 lines; it was later split (commit a85dd90). |
| Tests | Table-driven, real components, never a mock of the engine. |
| Errors | Wrap with `%w`, compare with `errors.Is` and `errors.As`; do not log and return the same error. |
| Docs | Exported names documented. Defaults are named constants with a reason; invariants are constants with a reason they are not configurable. |
| Determinism | Every walk over a map on the derivation path iterates in sorted order. |
| Frozen bytes | A golden test pins each frozen byte; the file says never to update a literal to pass. See [frozen bytes and answer hashes](/concepts/frozen-bytes-and-hashes.md). |
| Benchmark gate | A change to the hot loop (`BenchmarkEvaluate`) is kept only within noise. Card 1 held a 15 percent line. |
| Commits | Conventional subjects (`feat:`, `fix:`, `docs:`, `test:`, `chore:`), gate green before any commit that touches Go. |
| Work | Cards under `./kanban/` (backlog, doing, done), each with a job story; a branch per card. |

# Tests that hold the contract

- `golden_test.go` pins answer bytes, hashes, a rule-set ID and constants' canonical text; `aggregate_golden_test.go` pins the aggregate shapes.
- `anam_test.go` compiles and evaluates Anam's unmodified `rules/anam.dl` ([Anam query](../../anam_test.go)).
- `pressure_test.go` holds the regressions from the pressure review.
- Property tests compare demand evaluation with full evaluation in bytes and hash ([demand-driven evaluation](/concepts/demand-driven-evaluation.md)).

# The reusable-workflow workaround

The CI `quality` job does not call go-quality's reusable workflow. That workflow checks go-quality out inside the module, and its format step then walks into go-quality's own deliberately dirty fixture, so it fails every consumer. Instead the job installs the gate's tools at the versions `go-check.yml@v1` pins (goimports v0.44.0, govulncheck v1.3.0, golangci-lint v2.11.4), clones go-quality at tag `v1` outside the workspace, and runs its `check.sh`. The ci.yml comment says to return to the reusable workflow once fixed. The bug is filed as the go-quality card `gate-walks-own-fixture`. Card 5 records v0.1.0 going green only after this workaround.

# Mentors

Every design review and code audit runs through the luminaries skill. See [AGENTS.md mentors](../../AGENTS.md).

# Citations

[1] [AGENTS.md](../../AGENTS.md)
[2] [CI workflow](../../.github/workflows/ci.yml)
[3] [README.md, Engineering](../../README.md)
[4] [Card 5, tag v0.1.0](../../kanban/done/5-tag-v0.1.0.md)
[5] [docs/DECISIONS-PENDING.md](../../docs/DECISIONS-PENDING.md)
