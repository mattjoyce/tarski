---
id: 11
status: done
---
# Builtin refusals name the argument and show the signature

## Job story
When a rule uses a consumer builtin wrongly (an unbound variable, the wrong arity, a constant its Check refuses), I want the refusal to name the argument by the name the consumer declared and to show the builtin's signature, so I can fix the rule from the message alone.

## Source
anam-phone-log, board ask 20261026T104735Z, after Anam moved to v0.3.0 (anam 480d0b6). Anam's old engine said "argument B" and "within takes 3 arguments, within(A, B, Days)"; tarski says "Other in within must be bound by a positive body atom". Not a v0.2.2 to v0.3.0 regression: no builtin message changed between the tags. A gap against what Anam had.

## Scope
- The safety refusal for a variable in a builtin atom names the argument: "Other, argument B of within(A, B, Days), must be bound by a positive body atom".
- The arity refusal shows the signature: "within takes 3 arguments, within(A, B, Days); the rule gives 2".
- A Check refusal keeps the consumer's text and prefixes the argument name and signature.
- Engine builtins (prefix, suffix, contains) get the same shape with their own argument names.
- Message text is not frozen (README § What it promises), so this is additive; tests pin the new shape.

## Narrative
- 2026-10-26: filed from Anam's ask by datalog-module. Target v0.3.1.
- 2026-10-26: implemented in card-11 worktree. Added Builtin.signature and Builtin.argumentText (builtins.go), used by the safety refusal (plan.go checkGuard), the arity refusal and the Check prefix (checkBuiltinAtom). Engine builtins share the path, so they read prefix(Pattern, Subject). Final shapes: `line 4: Other, argument B of within(A, B, Days), must be bound by a positive body atom; a built-in binds nothing`; `within has 2 arguments, but within takes 3: within(A, B, Days)`; `within(A, B, Days): <consumer's Check text>`. Check returns only an error, so the signature is added and the consumer's text kept. Repeated variable names the first unbound argument. Pinned in builtin_refusal_test.go; older tests updated. No answer byte, hash or golden test touched.
