---
id: 2
status: backlog
blocks: [5]
---
# Consumer-declared builtins through Schema

## Job story
When a consumer needs a guard that speaks its own vocabulary (Anam's `within/3` on fact-times, an `overlaps/4` on intervals), I want to declare it in the Schema I compile against, so the engine stays vocabulary-free and the consumer keeps its words. (Drafted by datalog-module for Matt.)

## Scope
`Schema.Builtins map[string]Builtin`, shaped like Anam's table (anam origin/master `internal/query/builtin.go`): arity, an optional plan-time check of constant arguments, `Holds(args []Const) bool`, and a `Doc` for catalogues. Consulted everywhere the engine builtins are (plan, stratify, eval, whynot). `prefix`, `suffix` and `contains` stay engine entries. A schema that redeclares an engine name, or names a relation that is also a base relation, is ErrInvalid at Compile. Every argument must be bound by a positive atom, so a builtin creates no values.

## Acceptance
- A test declares a consumer builtin and evaluates with it, including in a negated rule and in why-not.
- README states the contract: a builtin is a pure total function of its arguments, or the hash is the consumer's fault.
- Gate green; `docs/DESIGN.md` § Built-ins updated.
