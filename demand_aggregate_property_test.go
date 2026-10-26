package tarski_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The demand property through aggregates (docs/design/demand.md §4, D1 and D9): random
// programs that count, sum, take the min or the max over lower strata, with recursion and
// negation below and above the aggregates, answer under demand exactly what full
// evaluation answers filtered to the goal, byte for byte and hash for hash, with valid
// proofs, proof groups whose witnesses full evaluation holds, and the same why-not
// failures. A group demand trimmed would change a count, a sum or an extreme, and so an
// answer here. Seeds replay.

var propAggSchema = tarski.Schema{Arities: map[string]int{"e": 2, "n": 1, "f": 1, "w": 2}}

// propAggRelations is the derived relations of propAggProgram, with their arities.
var propAggRelations = append(slices.Clone(propRelations), relArity{"a0", 2}, relArity{"a1", 2}, relArity{"t", 2},
	relArity{"u", 1}, relArity{"m", 2})

// propAggOps are the aggregate operations the generator draws from.
var propAggOps = []string{"count", "sum", "min", "max"}

// propAggregate is a random aggregate literal binding N over inner, grouped by G (the
// position of G in inner given by first): count over inner, or sum, min or max over
// the w weight of inner's other position, sometimes with a negation inside the braces.
func propAggregate(r *rand.Rand, inner, g string, first bool) string {
	atom := inner + "(" + g + ", Y)"
	if !first {
		atom = inner + "(Y, " + g + ")"
	}
	body := []string{atom}
	op := propAggOps[r.IntN(len(propAggOps))]
	target := ""
	if op != "count" {
		body = append(body, "w(Y, W)")
		target = " W"
	}
	switch r.IntN(4) {
	case 0:
		body = append(body, "not f(Y)")
	case 1:
		body = append(body, "not r0(Y, Y)")
	}
	return fmt.Sprintf("N = %s%s : { %s }", op, target, strings.Join(body, ", "))
}

// propAggProgram is propProgram with two aggregate relations over it, a0 and a1 (a1 may
// aggregate over a0), and above them t (a join of aggregates, or an atom reading an
// aggregate's result; maybe recursive), u (a negation of what is above an aggregate)
// and m (an aggregate over t and a0).
func propAggProgram(r *rand.Rand) string {
	var b strings.Builder
	b.WriteString(propProgram(r))
	for i := range 2 {
		head := "a" + strconv.Itoa(i)
		inner := "r" + strconv.Itoa(r.IntN(4))
		switch {
		case i == 1 && r.IntN(3) == 0:
			op := propAggOps[1+r.IntN(3)]
			fmt.Fprintf(&b, "%s(X, N) :- n(X), N = %s V : { a0(X, V) }.\n", head, op)
		case r.IntN(3) == 0:
			fmt.Fprintf(&b, "%s(X, N) :- e(X, Z), %s.\n", head, propAggregate(r, inner, "Z", true))
		default:
			fmt.Fprintf(&b, "%s(X, N) :- n(X), %s.\n", head, propAggregate(r, inner, "X", r.IntN(2) == 0))
		}
	}
	switch r.IntN(3) {
	case 0:
		b.WriteString("t(X, Y) :- a0(X, N), a1(Y, N).\n")
	case 1:
		b.WriteString("t(X, Y) :- a0(X, N), e(X, Y), N > 0.\n")
	default: // a derived atom reading an aggregate's result
		b.WriteString("t(X, Y) :- n(X), N = count : { r1(X, _) }, a1(Y, N).\n")
	}
	if r.IntN(2) == 0 {
		b.WriteString("t(X, Y) :- t(X, Z), e(Z, Y).\n")
	}
	if r.IntN(2) == 0 {
		b.WriteString("u(X) :- n(X), not t(X, X).\n")
	} else {
		b.WriteString("u(X) :- n(X), not a1(X, 0).\n")
	}
	switch op := propAggOps[r.IntN(len(propAggOps))]; op {
	case "count":
		b.WriteString("m(X, M) :- n(X), M = count : { t(X, Y), a0(Y, _) }.\n")
	default:
		fmt.Fprintf(&b, "m(X, M) :- n(X), M = %s V : { t(X, Y), a0(Y, V) }.\n", op)
	}
	return b.String()
}

// propAggBase is propDemandBase with weights: most nodes weigh a number from 0 to 3, some
// two, and now and then one weighs a string, which no sum takes.
func propAggBase(r *rand.Rand) []tarski.Fact {
	facts := propDemandBase(r)
	for _, x := range propDomain {
		for range r.IntN(3) {
			facts = append(facts, tarski.Fact{Relation: "w", Tuple: tarski.Tuple{tarski.String(x), tarski.Int(int64(r.IntN(4)))}})
		}
		if r.IntN(10) == 0 {
			facts = append(facts, tarski.Fact{Relation: "w", Tuple: tarski.Tuple{tarski.String(x), tarski.String("heavy")}})
		}
	}
	return facts
}

// propAggDomain is the constants a why-not over an aggregate program tries.
func propAggDomain() []tarski.Const {
	out := propStrings()
	for i := range 4 {
		out = append(out, tarski.Int(int64(i)))
	}
	return out
}

// newAggDemandCase builds and evaluates the aggregate case for a seed.
func newAggDemandCase(t *testing.T, seed uint64) (c demandCase) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 0xa66))
	c.text = propAggProgram(r)
	c.base, c.domain = []string{"e", "n", "f", "w"}, propAggDomain()
	for _, rel := range propAggRelations {
		c.derived = append(c.derived, rel.name)
	}
	plan, err := tarski.Compile(propAggSchema, parse(t, c.text))
	if err != nil {
		t.Fatalf("seed %d: Compile: %v\n%s", seed, err, c.text)
	}
	base, err := tarski.NewBase(propAggSchema, propAggBase(r)...)
	if err != nil {
		t.Fatal(err)
	}
	relation, adornment, goal := propGoal(r, propAggRelations)
	c.goal = goal
	dplan, err := plan.Demand(relation, adornment)
	if err != nil {
		t.Fatalf("seed %d: Demand(%s, %s): %v\n%s", seed, relation, adornment, err, c.text)
	}
	if c.full, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true}); err != nil {
		t.Fatalf("seed %d: full: %v", seed, err)
	}
	if c.demand, err = tarski.Evaluate(context.Background(), dplan, base, tarski.Options{Proofs: true, Goal: &goal}); err != nil {
		t.Fatalf("seed %d: demand: %v\n%s", seed, err, c.text)
	}
	return c
}

func TestDemand_AggregatesAgreeWithFullEvaluation(t *testing.T) {
	t.Parallel()
	tuples, above := 0, 0
	for seed := range uint64(300) {
		c := newAggDemandCase(t, seed)
		n := c.checkAnswers(t, seed)
		c.checkProofs(t, seed)
		c.checkWhyNot(t, seed)
		tuples += n
		if !strings.HasPrefix(c.goal.Relation, "r") && c.goal.Relation != "q" {
			above += n
		}
	}
	t.Logf("300 aggregate programs evaluated under demand, %d goal tuples, %d of them at or above an aggregate", tuples, above)
	if tuples < 300 || above < 150 {
		t.Errorf("%d goal tuples, %d at or above an aggregate; the property is not being exercised", tuples, above)
	}
}
