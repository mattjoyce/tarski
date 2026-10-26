// Package tarski is a small Datalog evaluator for questions that must have the same
// answer every time: facts in, derived relations out, each derivation checkable.
//
// The language (Parse) is small: rules with named derived relations, stratified
// negation, comparisons, facts, aggregates (count, sum, min and max over a group), and a
// declaration for a relation one program leaves to another. Compile validates programs
// against a Schema before anything is read: unsafe variables, unknown relations, wrong
// arities, negation or aggregation that cannot be stratified and any atom a schema
// check refuses are each reported with a message that says what to change. A relation a
// program declares and nothing defines is empty, which is how a rule set leaves a hook
// for its consumer to fill. Schema.Builtins lets a consumer add guards in its own
// vocabulary beside the engine's prefix, suffix and contains.
//
// The base relations are plain tuples. NewBase holds facts under a Schema, and
// Base.With adds more as a new value, sharing every relation the new facts leave
// alone, so a rule set compiles once, holds its standing facts once, and evaluates per
// question. A Base is immutable and its relations are indexed when it is built, so any
// number of Evaluate calls may share one.
//
// Evaluate runs a plan semi-naively to a fixpoint. It always terminates: there are no
// function symbols, and no built-in produces a value, so the derived relations are
// finite. A deadline on the context, a byte bound on what is derived and a step bound
// on the work to derive it stop it earlier, with the context's error or ErrTooLarge,
// never with a partial answer. Result.Cost reports the work charged, which is how a
// caller chooses the bounds: measure a representative question, then leave headroom.
// An Answer (Result.Relation) is a value: its tuples sorted, serialised one way and
// hashed, so two evaluations of one program over one set of facts answer identically.
// A RuleSet names a program, versions it and hashes its text, so an answer can say
// which rules stood behind it.
//
// A result can say why. With Options.Proofs on, Result.Proof gives the derivation of any
// tuple as a finite tree down to base facts, and Result.WhyNot explains a tuple that is
// absent: for each rule that could have derived it, the farthest literal that found
// nothing and the bindings it had there. Result.WhyEmpty does the same for a relation
// that holds nothing. Proofs and absences print as text and marshal as JSON.
//
// A question that names its constants need not pay for the whole relation. Plan.Demand
// compiles a plan for one shape of goal, such as "the first position bound", and
// Options.Goal supplies the constants per evaluation. The goal's relation is the same
// answer full evaluation gives, filtered to the goal, derived from only what can reach
// it. Selectivity comes from the bound positions, so measure with Result.Cost.
//
// Nothing here reads a file or the network, keeps state between calls, or knows what
// the relations mean. The package owns the evaluation; the caller owns the vocabulary.
package tarski
