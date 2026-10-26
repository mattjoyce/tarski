package tarski_test

import (
	"context"
	"fmt"
	"log"

	"github.com/mattjoyce/tarski"
)

// exampleRules is the program the examples share: a node reaches another along edges.
const exampleRules = `
path(X, Y) :- edge(X, Y).
path(X, Z) :- path(X, Y), edge(Y, Z).
`

// exampleMust stops an example at the first error; a real caller handles it.
func exampleMust[T any](v T, err error) T {
	if err != nil {
		log.Fatal(err)
	}
	return v
}

// exampleSetup compiles exampleRules over a small graph: a chain a to d, and a
// separate edge e to f.
func exampleSetup() (tarski.Plan, tarski.Base) {
	schema := tarski.Schema{Arities: map[string]int{"edge": 2}}
	prog := exampleMust(tarski.Parse(exampleRules))
	plan := exampleMust(tarski.Compile(schema, prog))
	edge := func(from, to string) tarski.Fact {
		return tarski.Fact{Relation: "edge", Tuple: tarski.Tuple{tarski.String(from), tarski.String(to)}}
	}
	base := exampleMust(tarski.NewBase(schema,
		edge("a", "b"), edge("b", "c"), edge("c", "d"), edge("e", "f")))
	return plan, base
}

// Evaluate runs a compiled plan over a base. The bounds make it fail closed: a
// derivation that crosses one is an error, never a partial answer. The answer is a
// value with bytes and a hash that depend only on the plan and the facts.
func ExampleEvaluate() {
	// A schema names the base relations and their arities. Parse reads the rules,
	// Compile checks them against the schema, NewBase holds the facts.
	schema := tarski.Schema{Arities: map[string]int{"edge": 2}}
	prog := exampleMust(tarski.Parse(exampleRules))
	plan := exampleMust(tarski.Compile(schema, prog))
	edge := func(from, to string) tarski.Fact {
		return tarski.Fact{Relation: "edge", Tuple: tarski.Tuple{tarski.String(from), tarski.String(to)}}
	}
	base := exampleMust(tarski.NewBase(schema,
		edge("a", "b"), edge("b", "c"), edge("c", "d"), edge("e", "f")))

	opts := tarski.Options{MaxSteps: 10_000, MaxBytes: 1 << 20}
	res := exampleMust(tarski.Evaluate(context.Background(), plan, base, opts))

	path := exampleMust(res.Relation("path"))
	fmt.Println(path.Len(), "paths")
	fmt.Println(string(path.Bytes()))
	fmt.Println(path.Hash())

	// Cost is what the evaluation charged against the bounds. Read it on a
	// representative input to choose MaxSteps and MaxBytes, with headroom.
	steps, bytes := res.Cost()
	fmt.Println(steps > 0, bytes > 0)

	// A bound too small for the work is an error that says which bound was crossed.
	_, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{MaxSteps: 3})
	fmt.Println(err != nil)
	// Output:
	// 7 paths
	// {"relation":"path","tuples":[["a","b"],["a","c"],["a","d"],["b","c"],["b","d"],["c","d"],["e","f"]]}
	// sha256:ed8b58af89167cc75d5b8a4ffda32a45e4d0b4fb4bd436d3124f9d093dd67cc0
	// true true
	// true
}

// With Options.Proofs on, Proof shows how a tuple was derived and WhyNot explains one
// that was not: the farthest literal each candidate rule reached.
func ExampleResult_WhyNot() {
	plan, base := exampleSetup()
	res := exampleMust(tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true}))

	ac := tarski.Tuple{tarski.String("a"), tarski.String("c")}
	proof, ok := res.Proof("path", ac)
	fmt.Println(ok)
	fmt.Print(proof)

	// There is no path from a to f. WhyNot walks each rule that could derive it.
	af := tarski.Tuple{tarski.String("a"), tarski.String("f")}
	absence, ok := res.WhyNot("path", af, 0)
	fmt.Println(ok)
	fmt.Print(absence)
	// Output:
	// true
	// path("a", "c")  line 3  path(X, Z) :- path(X, Y), edge(Y, Z).
	//   path("a", "b")  line 2  path(X, Y) :- edge(X, Y).
	//     edge("a", "b")  fact
	//   edge("b", "c")  fact
	// true
	// path("a", "f") is not derived
	//   line 2  path(X, Y) :- edge(X, Y).
	//     stopped at edge(X, Y)  with X="a", Y="f"
	//     because
	//       edge("a", "f") is not derived: no such fact
	//   line 3  path(X, Z) :- path(X, Y), edge(Y, Z).
	//     stopped at edge(Y, Z)  with X="a", Y="b", Z="f"
	//     because
	//       edge("b", "f") is not derived: no such fact
}

// Plan.Demand compiles a plan for one shape of question: here path with its first
// position bound. The constants arrive per evaluation in Options.Goal, a constant
// where the position is bound and a variable (the anonymous "_" included) where it
// is open. The goal's relation is the same answer full evaluation gives, filtered to
// the goal, usually at less cost.
func ExamplePlan_Demand() {
	plan, base := exampleSetup()
	demand := exampleMust(plan.Demand("path", "bf"))

	goal := &tarski.Goal{Relation: "path", Args: []tarski.Term{
		{Const: tarski.String("e")},
		{Var: "_"},
	}}
	res := exampleMust(tarski.Evaluate(context.Background(), demand, base, tarski.Options{Goal: goal}))
	path := exampleMust(res.Relation("path"))
	fmt.Println(string(path.Bytes()))

	// A Result evaluated for a goal answers only the goal's relation. Check Goal
	// before reading anything else from it.
	g, ok := res.Goal()
	fmt.Println(g.Relation, ok)

	// Cost follows the question: a goal that binds a position does less work than
	// the full plan. Measure with Cost; a goal that binds nothing may cost more.
	full := exampleMust(tarski.Evaluate(context.Background(), plan, base, tarski.Options{}))
	fullSteps, _ := full.Cost()
	demandSteps, _ := res.Cost()
	fmt.Println(demandSteps < fullSteps)
	// Output:
	// {"relation":"path","tuples":[["e","f"]]}
	// path true
	// true
}
