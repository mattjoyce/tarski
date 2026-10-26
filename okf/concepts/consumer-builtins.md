---
type: Concept
title: Consumer builtins
description: "A consumer declares its own guards in Schema.Builtins beside the engine's; the engine guarantees termination and a bounded number of calls, the consumer guarantees Holds is pure, total, deterministic and cheap."
tags: [builtins, schema, guards, purity, contract]
timestamp: 2026-10-26T00:00:00Z
---

# Why

The engine stays vocabulary-free, so a guard in a consumer's own words (Anam's `within/3` on fact-times, an `overlaps/4` on intervals) is declared by the consumer, not added to the engine. Card 2 settled the shape; see [decision 3](/decisions/3-consumer-builtins-in-schema.md).

# Shape

```go
type Builtin struct {
    Args  []BuiltinArg                  // arity is len(Args), at least 1
    Doc   string                        // what a catalogue prints
    Check func(consts []*Const) error   // optional, at Compile: one entry per argument, nil for a variable
    Holds func(args []Const) bool       // required: the guard over bound arguments
}
type BuiltinArg struct{ Name, Doc string }
```

`Schema.Builtins map[string]Builtin` is keyed by name. `EngineBuiltins()` returns a fresh copy of the engine's own (`prefix`, `suffix`, `contains`) so a catalogue can list both. `Check` and `Holds` are tagged `json:"-"` so a catalogue marshals.

# Treated exactly as the engine's

At `Compile`, in stratification, in `Evaluate` and in `WhyNot`. Every argument must be bound by a positive body atom (`_` is refused), it binds and derives nothing, and a proof through one lists only the positive atoms. A why-not stopped by one reports it as `Failed` with no `Because`.

`Compile` refuses with `ErrInvalid` a declared name that is not a relation name, an engine builtin's name, or also a base relation in `Arities`; a declaration with no `Args` or a nil `Holds`; a builtin in a head or declaration; a wrong arity; an unbound argument. A `Check` error is wrapped in `ErrInvalid` with the line. `NewBase` holds the same Schema to the same declaration rules, since a Schema is one value.

`Compile` copies the table, `Args` cloned, so changing the map afterwards changes no plan.

# The contract splits in two

| The engine guarantees | The consumer guarantees |
|---|---|
| Termination: a builtin creates no value | `Holds` is a pure, total, deterministic function of its arguments |
| A bounded number of calls: each call is a step charged to `MaxSteps` (v0.2.2) | `Holds` is safe to call concurrently |
| A `Holds` that panics is recovered where it is called: `Evaluate` fails with `ErrInvalid` naming the builtin and arguments, `WhyNot` returns ok false, no panic escapes | One call is cheap |

An impure builtin voids "same inputs, same outputs" and the answer hash, and that is the consumer's fault, not the engine's. `Check` is not recovered, like `Schema.Checks`: it runs once at `Compile`, on the program, not on data.

Builtins are the plan's alone. A base never consults them, so a base built under a schema with other `Builtins` is compatible by shape.

# Step accounting

v0.2.1 charged a step per rule application, which bounded how many guard-only rules ran but not how many guards one body held. v0.2.2 charges each builtin call. See [bounds](/concepts/bounds.md).

# Open points recorded on the card

The field shape (`Args` against `Arity`, `[]*Const` for `Check`) was to be confirmed with the Anam and Shearline leads before a tag; the card records Anam approving the freeze of the shape before v0.1.0. Anam should drop its private table and pass `within` through `Schema.Builtins`.

# Related

[Stratified negation](/concepts/stratified-negation.md), API: [Schema and Builtins](/api/schema-builtins.md).

# Citations

[1] [Card 2, consumer builtins](../../kanban/done/2-consumer-builtins.md)
[2] [Card 5, tag v0.1.0](../../kanban/done/5-tag-v0.1.0.md)
[3] [docs/DESIGN.md, Built-ins](../../docs/DESIGN.md)
[4] [README.md, Consumer builtins](../../README.md)
[5] [builtins.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/builtins.go)
[6] [consumer_builtins_test.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/consumer_builtins_test.go)
