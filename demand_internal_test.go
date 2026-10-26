package tarski

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// A rule rewritten for a goal records its source as the proof's rule and leaves its
// demand literal out of the proof's body (design §2.5): here the plan's own rule is
// dressed as a rewrite by hand, so the mechanism is tested apart from the rewrite.
func TestRecord_NamesTheSourceAndSkipsTheDemandLiteral(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"node": 1, "score": 2}}
	plan, err := Compile(schema, mustParse(t, "d(X) :- node(X).\np(X) :- d(X), score(X, _).\n"))
	if err != nil {
		t.Fatal(err)
	}
	src := &Rule{Head: Atom{Name: "p", Args: []Term{{Var: "X"}}}, Line: 9, Text: "p(X) :- score(X, _)."}
	found := false
	for s := range plan.strata {
		for i := range plan.strata[s] {
			if r := &plan.strata[s][i]; r.Head.Name == "p" {
				r.source, r.demand, found = src, 0, true
			}
		}
	}
	if !found {
		t.Fatal("no rule for p in the plan")
	}
	base, err := NewBase(schema, Fact{"node", Tuple{String("a")}}, Fact{"score", Tuple{String("a"), Int(3)}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Evaluate(context.Background(), plan, base, Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := res.Proof("p", Tuple{String("a")})
	if !ok {
		t.Fatal("no proof of p(a)")
	}
	if p.Rule.Text != src.Text || p.Rule.Line != 9 {
		t.Errorf("proof rule = %q line %d, want the source %q", p.Rule.Text, p.Rule.Line, src.Text)
	}
	if len(p.Body) != 1 || p.Body[0].Relation != "score" {
		t.Errorf("proof body = %+v, want only score: the demand literal is not listed", p.Body)
	}
}

// visible names the source relation in place of a hidden one, and changes nothing else.
func TestVisible_MapsHiddenRelations(t *testing.T) {
	t.Parallel()
	demand := Plan{demand: &demandPlan{hidden: map[string]string{"m?p?b": "p"}}}
	cases := []struct {
		name string
		plan Plan
		err  error
		want string
	}{
		{"hidden name under demand", demand, &TooLarge{Bound: BoundSteps, Relation: "m?p?b"}, "p"},
		{"wrapped hidden name", demand, fmt.Errorf("x: %w", &TooLarge{Relation: "m?p?b"}), "p"},
		{"source name under demand", demand, &TooLarge{Relation: "q"}, "q"},
		{"plain plan", Plan{}, &TooLarge{Relation: "m?p?b"}, "m?p?b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tl *TooLarge
			if !errors.As(tc.plan.visible(tc.err), &tl) || tl.Relation != tc.want {
				t.Fatalf("visible(%v) relation = %v, want %s", tc.err, tl, tc.want)
			}
		})
	}
	plain := errors.New("other")
	if got := demand.visible(plain); !errors.Is(got, plain) || got.Error() != "other" {
		t.Errorf("visible changed an error that is no *TooLarge: %v", got)
	}
}

// place keeps a body the caller ordered and marks every position over the stratum's own
// relations, a leading literal included.
func TestPlace_KeepsAnOrderedBody(t *testing.T) {
	t.Parallel()
	lit := func(name string) Literal { return Literal{Atom: &Atom{Name: name, Args: []Term{{Var: "X"}}}} }
	pr := rule{Rule: Rule{Head: Atom{Name: "p", Args: []Term{{Var: "X"}}}}, ordered: []Literal{lit("m"), lit("e"), lit("p")}, demand: 0}
	strata := place([]rule{pr}, map[string]int{"p": 1, "m": 1}, map[string]bool{"p": true, "m": true})
	if len(strata) != 2 || len(strata[1]) != 1 {
		t.Fatalf("strata = %v", strata)
	}
	got := strata[1][0]
	if len(got.ordered) != 3 || got.ordered[0].Atom.Name != "m" || fmt.Sprint(got.recursive) != "[0 2]" {
		t.Errorf("ordered %v recursive %v, want the body kept and positions [0 2]", got.ordered, got.recursive)
	}
}
