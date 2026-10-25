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

// renamed reaches inside an aggregate: its result, its target and its body take the
// canonical names, and the original body is untouched.
func TestRenamed_RenamesInsideAnAggregate(t *testing.T) {
	t.Parallel()
	body := mustParse(t, `h(B, C) :- g(B), C = sum B : { item(B, V), V > B, not bad(B) }.`).Rules[0].Body
	before := body[1].text()
	got := renamed(body, map[string]string{"B": "A", "C": "A"})
	if want := "A = sum A : { item(A, V), V > A, not bad(A) }"; got[1].text() != want || got[0].text() != "g(A)" {
		t.Errorf("renamed = %s, %s; want g(A), %s", got[0].text(), got[1].text(), want)
	}
	if body[1].text() != before {
		t.Errorf("renamed changed the original: %s", body[1].text())
	}
}

// HasAggregates is true exactly when a compiled plan's rule holds an aggregate.
func TestHasAggregates_CompiledPlans(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"s": 1}}
	cases := []struct {
		text string
		want bool
	}{
		{`m(X) :- s(X).`, false},
		{`n(N) :- s(X), N = count : { s(X) }. m(X) :- s(X).`, true},
		{`m(X) :- s(X). n(N) :- N = count : { m(_) }.`, true},
	}
	for _, c := range cases {
		plan, err := Compile(schema, mustParse(t, c.text))
		if err != nil {
			t.Fatalf("Compile(%s): %v", c.text, err)
		}
		if got := plan.HasAggregates(); got != c.want {
			t.Errorf("HasAggregates(%s) = %v, want %v", c.text, got, c.want)
		}
	}
}

// An aggregate is placed as soon as its group variables are bound, the guards on its
// result follow it, and its body is ordered on its own with the group variables bound,
// so a guard inside runs right after the atom that binds it.
func TestOrder_PlacesAnAggregateLikeAGuardThatBinds(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"grp": 1, "item": 2}}
	text := `n(G, N) :- N > 1, N = count : { V > 1, item(G, V) }, grp(G), N != 7.`
	plan, err := Compile(schema, mustParse(t, text))
	if err != nil {
		t.Fatal(err)
	}
	r := plan.strata[0][0]
	var got []string
	for _, l := range r.ordered {
		got = append(got, l.text())
	}
	want := []string{"grp(G)", "N = count : { V > 1, item(G, V) }", "N > 1", "N != 7"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ordered = %q, want %q", got, want)
	}
	a := r.aggs[1]
	if a == nil || !reflect.DeepEqual(a.group, []string{"G"}) {
		t.Fatalf("aggs[1] = %+v, want the count grouped by G", a)
	}
	var inner []string
	for _, l := range a.inner.ordered {
		inner = append(inner, l.text())
	}
	if want := []string{"item(G, V)", "V > 1"}; !reflect.DeepEqual(inner, want) {
		t.Errorf("inner ordered = %q, want %q", inner, want)
	}
	if r.aggs[0] != nil || len(r.recursive) != 0 {
		t.Errorf("aggs = %v, recursive = %v; want only position 1 to aggregate and none recursive", r.aggs, r.recursive)
	}
}
