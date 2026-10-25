package tarski

import (
	"reflect"
	"testing"
)

// An aggregate literal renders in source form, and that form parses back to the same
// literal: the text a why-not prints is a text the parser reads.
func TestAggregateText_RoundTrips(t *testing.T) {
	t.Parallel()
	cases := []struct{ src, want string }{
		{`N = count : {r(M,_)}`, `N = count : { r(M, _) }`},
		{`S = sum V : { p(E, At, V), prefix(M, At) }`, `S = sum V : { p(E, At, V), prefix(M, At) }`},
		{`T = min X : { r(X), not bad(X), X >= "2026" }`, `T = min X : { r(X), not bad(X), X >= "2026" }`},
		{`T = max X : { r(X, 1.50, true) }`, `T = max X : { r(X, 1.5, true) }`},
	}
	for _, c := range cases {
		lit := mustAggregate(t, c.src)
		if got := lit.text(); got != c.want {
			t.Errorf("text(%s) = %q, want %q", c.src, got, c.want)
		}
		again := mustAggregate(t, lit.text())
		lit.Aggregate.Line, again.Aggregate.Line = 0, 0
		if !reflect.DeepEqual(again, lit) {
			t.Errorf("Parse(text(%s)) = %+v, want %+v", c.src, again.Aggregate, lit.Aggregate)
		}
	}
}

// mustAggregate parses src as the one body literal of a rule and returns it.
func mustAggregate(t *testing.T, src string) Literal {
	t.Helper()
	p, err := Parse("h(1) :- " + src + ".")
	if err != nil {
		t.Fatalf("Parse(%s): %v", src, err)
	}
	lit := p.Rules[0].Body[0]
	if lit.Aggregate == nil {
		t.Fatalf("Parse(%s): %+v is not an aggregate", src, lit)
	}
	return lit
}

// HasAggregates is true when a plan's rule holds an aggregate. Compile cannot make such
// a plan yet (it refuses every aggregate), so the plan is assembled by hand; slice 3 of
// card 6 replaces this with a compiled one.
func TestHasAggregates_TrueForAPlanHoldingOne(t *testing.T) {
	t.Parallel()
	p, err := Parse(`n(N) :- s(X), N = count : { s(X) }. m(X) :- s(X).`)
	if err != nil {
		t.Fatal(err)
	}
	plain := Plan{strata: [][]rule{{{Rule: p.Rules[1]}}}}
	if plain.HasAggregates() {
		t.Error("HasAggregates = true for a plan with no aggregate")
	}
	with := Plan{strata: [][]rule{{{Rule: p.Rules[1]}}, {{Rule: p.Rules[0]}}}}
	if !with.HasAggregates() {
		t.Error("HasAggregates = false for a plan holding an aggregate")
	}
}
