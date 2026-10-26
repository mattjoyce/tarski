package tarski_test

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Programs whose rewrite breaks stratification answer through the fallback (design §2.4,
// decided §8 Q5): demand is dropped for a relation at a time until the rewrite stratifies,
// and every answer is still full evaluation's filtered to the goal, with valid proofs and
// the same why-not failures. The first program is the one slice 2 refused; in the second
// the goal's own relation loses its demand and is derived whole; in the third the cycle
// runs through an aggregate's braces.
func TestDemand_Fallback(t *testing.T) {
	t.Parallel()
	schema := tarski.Schema{Arities: map[string]int{"e": 2}}
	programs := []struct{ name, text, relation string }{
		{"demand under a negation", `r0(X, Y) :- e(X, Y). r1(X, Y) :- e(Y, X). r1(X, Y) :- r1(X, Z), e(Z, Y), not r0(X, Y).`, "r1"},
		{"the goal's own demand dropped", `a(X, Y) :- e(X, Y). a(X, Y) :- b(X, Z), a(Z, Y). b(X, Z) :- e(X, Z), not c(X, Z). ` +
			`c(X, Z) :- e(X, Z), e(Z, X).`, "a"},
		{"demand through an aggregate cycle", `p(X, Y) :- e(X, Y). p(X, Y) :- p(X, Z), c(Z, N), N > 1, e(Z, Y). ` +
			`c(Z, N) :- e(Z, _), N = count : { s(Z, W) }. s(Z, W) :- e(Z, W), e(W, Z).`, "p"},
	}
	for _, p := range programs {
		t.Run(p.name, func(t *testing.T) {
			plan, err := tarski.Compile(schema, parse(t, p.text))
			if err != nil {
				t.Fatal(err)
			}
			tuples := 0
			for seed := range uint64(20) {
				r := rand.New(rand.NewPCG(seed, 0xfa11))
				var facts []tarski.Fact
				for _, x := range propDomain {
					for _, y := range propDomain {
						if r.IntN(3) == 0 {
							facts = append(facts, tarski.Fact{Relation: "e", Tuple: tarski.Tuple{tarski.String(x), tarski.String(y)}})
						}
					}
				}
				base, err := tarski.NewBase(schema, facts...)
				if err != nil {
					t.Fatal(err)
				}
				full, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
				if err != nil {
					t.Fatal(err)
				}
				for _, adornment := range []string{"bf", "fb", "bb", "ff"} {
					tuples += checkFallback(t, plan, base, full, p.text, p.relation, adornment, r)
				}
			}
			if tuples < 40 {
				t.Errorf("%d goal tuples in all; the fallback is not being exercised", tuples)
			}
		})
	}
}

// checkFallback evaluates one random goal of the given shape under demand and holds it to
// full evaluation: the answer, every proof's rule and body, and why-not's failing literals.
// It returns how many tuples the answer held.
func checkFallback(t *testing.T, plan tarski.Plan, base tarski.Base, full tarski.Result, text, relation, adornment string, r *rand.Rand) int {
	t.Helper()
	dplan, err := plan.Demand(relation, adornment)
	if err != nil {
		t.Fatalf("Demand(%s, %s): %v", relation, adornment, err)
	}
	goal := tarski.Goal{Relation: relation}
	for i := range adornment {
		if adornment[i] == 'f' {
			goal.Args = append(goal.Args, tarski.Term{Var: "V" + string(rune('0'+i))})
			continue
		}
		goal.Args = append(goal.Args, tarski.Term{Const: tarski.String(propDomain[r.IntN(len(propDomain))])})
	}
	res, err := tarski.Evaluate(context.Background(), dplan, base, tarski.Options{Proofs: true, Goal: &goal})
	if err != nil {
		t.Fatal(err)
	}
	c := demandCase{text: text, goal: goal, full: full, demand: res, domain: propStrings()}
	want := filtered(t, full, goal)
	got, err := res.Relation(relation)
	if err != nil || got.Hash() != want.Hash() {
		t.Fatalf("goal %v: demand %s, full %s (%v)", goal, got.Bytes(), want.Bytes(), err)
	}
	c.checkProofs(t, 0)
	c.checkWhyNot(t, 0)
	return got.Len()
}
