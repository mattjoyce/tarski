# Concept

* [Aggregates](aggregates.md) - count, sum, min and max over a grouped body in Soufflé's syntax, stratified like negation; witnesses, empty groups, mixed kinds and the Soufflé deltas as decided.
* [Bounds](bounds.md) - A deadline, MaxBytes and MaxSteps; a crossed bound is *TooLarge, never a partial answer; what each step charges.
* [Constants and exact numbers](constants-and-exact-numbers.md) - Three scalar kinds, a UTF-8 string domain and exact decimal numbers with one canonical text.
* [Consumer builtins](consumer-builtins.md) - Guards declared in Schema.Builtins; the engine bounds the calls, the consumer keeps Holds pure.
* [Demand-driven evaluation](demand-driven-evaluation.md) - Released in v0.3.0: magic sets to a goal, same bytes as full evaluation, fallback on stratification, aggregates on group keys.
* [Frozen bytes and answer hashes](frozen-bytes-and-hashes.md) - Answer bytes, answer hash, rule-set hash and constant text are frozen and pinned by golden tests.
* [Least fixed point](least-fixed-point.md) - Why the name: Knaster-Tarski gives a Datalog program its meaning, and evaluation computes it semi-naively.
* [Proof](proof.md) - The first derivation of a tuple, shared as a DAG of values, deterministic, with aggregate groups.
* [Rule-set identity](rule-set-identity.md) - A named, versioned program with a sha256 over its text; Verify re-parses.
* [Stratified negation](stratified-negation.md) - The language, safety and stratification checks, builtins as guards, declared-and-undefined is empty.
* [The admission rule](admission-rule.md) - A feature is in only if evaluation stays pure, terminating, deterministic and boundable.
* [Value and builder](value-and-builder.md) - A relation is a builder during evaluation and a sealed value after; reads are safe by construction.
* [Why-not provenance](why-not-provenance.md) - The farthest literal each rule reached, Blocked, Truncated, and WhyEmpty for a relation with no tuple.
