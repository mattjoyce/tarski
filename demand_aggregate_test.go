package tarski

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// Demand through aggregates (design §2.1, D9; aggregates.md §8): demand enters the braces
// only through the group variables, so a group is computed whole for every key the goal
// needs, and a relation read inside the braces is complete for that group (D3) though
// partial elsewhere.

var demandAggSchema = Schema{Arities: map[string]int{"owns": 2, "price": 2, "person": 1}}

const demandAggText = `item(P, C) :- owns(P, C).
cheap(C) :- price(C, X), X < 10.
n(P, N) :- person(P), N = count : { item(P, C), cheap(C) }.
big(P) :- n(P, N), N > 1.
top(P, M) :- person(P), M = max X : { item(P, C), price(C, X) }.`

// The rewrite of an aggregate rule, pinned. Inner atoms are adorned from the group
// variables and the inner atoms before them only: binding the result in the goal (n/bb)
// adorns the braces exactly as leaving it open (n/bf) does, and the local C is bound
// only once item, inside the braces, binds it.
func TestRewrite_Aggregates(t *testing.T) {
	t.Parallel()
	plan, err := Compile(demandAggSchema, mustParse(t, demandAggText))
	if err != nil {
		t.Fatal(err)
	}
	inner := []string{
		"0 item(P, C) :- m?item?bf(P), owns(P, C).",
		"0 cheap(C) :- m?cheap?b(C), price(C, X), X < 10.",
	}
	cases := []struct {
		relation, adornment string
		want                []string
	}{
		{"n", "bf", append([]string{
			"0 m?item?bf(P) :- m?n?bf(P).",
			"0 m?cheap?b(C) :- m?n?bf(P), item(P, C).",
		}, append(inner, "1 n(P, N) :- m?n?bf(P), person(P), N = count : { item(P, C), cheap(C) }.")...)},
		{"n", "bb", append([]string{
			"0 m?item?bf(P) :- m?n?bb(P, N).",
			"0 m?cheap?b(C) :- m?n?bb(P, N), item(P, C).",
		}, append(inner, "1 n(P, N) :- m?n?bb(P, N), person(P), N = count : { item(P, C), cheap(C) }.")...)},
		{"top", "fb", []string{
			"0 m?item?bf(P) :- m?top?fb(M), person(P).",
			"0 item(P, C) :- m?item?bf(P), owns(P, C).",
			"1 top(P, M) :- m?top?fb(M), person(P), M = max X : { item(P, C), price(C, X) }.",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.relation+"/"+tc.adornment, func(t *testing.T) {
			d, err := plan.Demand(tc.relation, tc.adornment)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(rewritten(d.demand), "\n")
			if want := strings.Join(tc.want, "\n"); got != want {
				t.Errorf("rewrite =\n%s\nwant\n%s", got, want)
			}
			// A copy's aggregate plans move with its body: the plan at an aggregate
			// literal's place is the source's own.
			for _, stratum := range d.demand.strata {
				for _, r := range stratum {
					for i, l := range r.ordered {
						if l.Aggregate != nil && (r.aggs[i] == nil || r.aggs[i].lit != l.Aggregate) {
							t.Errorf("%s: no aggregate plan at ordered[%d]", r.Head.text(), i)
						}
					}
				}
			}
		})
	}
}

// demandAggBase: alice owns a, b and c, bob owns b and d; a, b and d are cheap.
func demandAggBase(t *testing.T) Base {
	t.Helper()
	s := String
	base, err := NewBase(demandAggSchema,
		Fact{"person", Tuple{s("alice")}}, Fact{"person", Tuple{s("bob")}},
		Fact{"owns", Tuple{s("alice"), s("a")}}, Fact{"owns", Tuple{s("alice"), s("b")}}, Fact{"owns", Tuple{s("alice"), s("c")}},
		Fact{"owns", Tuple{s("bob"), s("b")}}, Fact{"owns", Tuple{s("bob"), s("d")}},
		Fact{"price", Tuple{s("a"), Int(1)}}, Fact{"price", Tuple{s("b"), Int(2)}}, Fact{"price", Tuple{s("c"), Int(50)}},
		Fact{"price", Tuple{s("d"), Int(3)}})
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// Why-not through an aggregate on a demand Result reads the result's relations, which
// are partial (bob's items and the price of d were never derived) but complete for the
// group the goal reaches, so the walk computes alice's count as full evaluation does.
// Proof groups name witnesses the full result holds.
func TestDemand_AggregateWhyNotAndProof(t *testing.T) {
	t.Parallel()
	plan, err := Compile(demandAggSchema, mustParse(t, demandAggText))
	if err != nil {
		t.Fatal(err)
	}
	base := demandAggBase(t)
	full, err := Evaluate(context.Background(), plan, base, Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	d, err := plan.Demand("n", "bb")
	if err != nil {
		t.Fatal(err)
	}
	alice := String("alice")
	goal := &Goal{Relation: "n", Args: []Term{{Const: alice}, {Const: Int(5)}}}
	res, err := Evaluate(context.Background(), d, base, Options{Proofs: true, Goal: goal})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int{"item": 3, "cheap": 2} {
		if got := res.relations[name].len(); got != want || full.relations[name].len() <= got {
			t.Fatalf("%s under demand has %d tuples, full %d: want %d, alice's alone", name, got, full.relations[name].len(), want)
		}
	}
	tup := Tuple{alice, Int(5)}
	dn, okD := res.WhyNot("n", tup, 0)
	fn, okF := full.WhyNot("n", tup, 0)
	if !okD || !okF || len(dn.Rules) != 1 || !reflect.DeepEqual(dn.Rules[0].Failed, fn.Rules[0].Failed) ||
		dn.Rules[0].Value == nil || !dn.Rules[0].Value.same(Int(2)) || dn.Rules[0].Reason != fn.Rules[0].Reason {
		t.Fatalf("WhyNot under demand = %+v, full %+v", dn, fn)
	}

	d2, err := plan.Demand("n", "bf")
	if err != nil {
		t.Fatal(err)
	}
	res2, err := Evaluate(context.Background(), d2, base, Options{Proofs: true, Goal: &Goal{Relation: "n", Args: []Term{{Const: alice}, {Var: "N"}}}})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := res2.Proof("n", Tuple{alice, Int(2)})
	if !ok || p.GroupSize != 2 || len(p.Group) != 2 {
		t.Fatalf("proof of n(alice, 2) = %+v, %v; want a group of two", p, ok)
	}
	for _, w := range p.Group {
		for _, bt := range w {
			if _, ok := full.Proof(bt.Relation, bt.Tuple); !ok {
				t.Errorf("witness %s%v is not in the full result", bt.Relation, bt.Tuple)
			}
		}
	}
}

// One aggregate demand plan shared by goroutines: every evaluation gives the same answer
// and the same proof, group included.
func TestDemand_AggregateConcurrent(t *testing.T) {
	t.Parallel()
	plan, err := Compile(demandAggSchema, mustParse(t, demandAggText))
	if err != nil {
		t.Fatal(err)
	}
	d, err := plan.Demand("big", "b")
	if err != nil {
		t.Fatal(err)
	}
	base := demandAggBase(t)
	opts := Options{Proofs: true, Goal: &Goal{Relation: "big", Args: []Term{{Const: String("bob")}}}}
	eval := func() Proof {
		res, err := Evaluate(context.Background(), d, base, opts)
		if err != nil {
			t.Error(err)
			return Proof{}
		}
		p, _ := res.Proof("big", Tuple{String("bob")})
		return p
	}
	want := eval()
	if want.Rule == nil || len(want.Body) != 1 || want.Body[0].GroupSize != 2 {
		t.Fatalf("proof of big(bob) = %+v; want n(bob, 2) over a group of two", want)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if !reflect.DeepEqual(eval(), want) {
				t.Error("a concurrent aggregate demand evaluation differs")
			}
		})
	}
	wg.Wait()
}
