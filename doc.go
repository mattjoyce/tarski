// Package datalog is a small Datalog evaluator for questions that must have the same
// answer every time: facts in, derived relations out, each derivation checkable.
//
// The language (Parse) is small: rules with named derived relations, stratified
// negation, comparisons, facts, and a declaration for a relation one program leaves to
// another. Compile validates programs against a Schema before anything is read:
// unsafe variables, unknown relations, wrong arities, unstratifiable negation and any
// atom a schema check refuses are each reported with a message that says what to
// change. A relation a program declares and nothing defines is empty, which is how a
// rule set leaves a hook for its consumer to fill.
//
// The base relations are plain tuples. NewBase holds facts under a Schema, and
// Base.With adds more as a new value, sharing every relation the new facts leave
// alone, so a policy compiles once, holds its standing facts once, and evaluates per
// request. A Base is immutable and its relations are indexed when it is built, so any
// number of Evaluate calls may share one.
//
// Evaluate runs a plan semi-naively to a fixpoint. It always terminates: there are no
// function symbols, and no built-in produces a value, so the derived relations are
// finite. A deadline on the context, a byte bound on what is derived and a step bound
// on the work to derive it stop it earlier, with the context's error or ErrTooLarge.
// An Answer (Result.Relation) is a value: its tuples sorted, serialised one way and
// hashed, so two evaluations of one program over one base answer identically. A
// RuleSet names a program, versions it and hashes its text, so an answer can say
// which rules stood behind it.
//
// Nothing here reads a file or the network, keeps state between calls, or knows what
// the relations mean. The package owns the evaluation; the caller owns the vocabulary.
package datalog
