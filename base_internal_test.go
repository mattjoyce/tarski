package datalog

import (
	"context"
	"testing"
)

func internalBase(t *testing.T, facts ...Fact) Base {
	t.Helper()
	b, err := NewBase(Schema{Arities: map[string]int{"edge": 2, "node": 1}}, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	return b
}

func str(vs ...string) Tuple {
	t := make(Tuple, len(vs))
	for i, v := range vs {
		t[i] = String(v)
	}
	return t
}

// With shares the relations the new facts leave alone and rebuilds the ones they touch.
func TestBase_WithSharesUntouchedRelations(t *testing.T) {
	t.Parallel()
	standing := internalBase(t, Fact{"edge", str("a", "b")}, Fact{"node", str("a")})
	more, err := standing.With(Fact{"edge", str("b", "c")})
	if err != nil {
		t.Fatalf("With: %v", err)
	}
	if more.relations["node"] != standing.relations["node"] {
		t.Error("an untouched relation was copied, want shared")
	}
	if more.relations["edge"] == standing.relations["edge"] {
		t.Error("a touched relation was shared, want rebuilt")
	}
	if got := len(standing.relations["edge"].tuples); got != 1 {
		t.Errorf("the standing edge relation holds %d tuples after With, want 1", got)
	}
	again, err := standing.With(Fact{"node", str("z")})
	if err != nil {
		t.Fatalf("With: %v", err)
	}
	if again.relations["edge"] != standing.relations["edge"] {
		t.Error("edge was copied by a With that only touched node")
	}
}

// A frozen relation has an index for every position from the start, and reading it
// through candidates changes nothing.
func TestRelation_FrozenIndexesAreComplete(t *testing.T) {
	t.Parallel()
	b := internalBase(t, Fact{"edge", str("a", "b")}, Fact{"edge", str("a", "c")})
	r := b.relations["edge"]
	if !r.frozen || len(r.idx) != 2 {
		t.Fatalf("frozen = %v with %d indexes, want true with 2", r.frozen, len(r.idx))
	}
	if got := r.candidates(0, String("a")); len(got) != 2 {
		t.Errorf("candidates(0, a) = %v, want two", got)
	}
	if got := r.candidates(1, String("zzz")); len(got) != 0 {
		t.Errorf("candidates(1, zzz) = %v, want none", got)
	}
	plan, err := Compile(Schema{Arities: map[string]int{"edge": 2, "node": 1}},
		mustParse(t, `p(X, Y) :- edge(X, Y), edge("a", Y). q(X) :- p(X, "c").`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	before := r.idx
	if _, err := Evaluate(context.Background(), plan, b, Options{}); err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if len(r.idx) != 2 || len(before) != 2 || len(r.tuples) != 2 {
		t.Error("Evaluate changed a frozen relation")
	}
}

// Adding to a frozen relation is a bug in this package, and says so loudly.
func TestRelation_AddToFrozenPanics(t *testing.T) {
	t.Parallel()
	r := internalBase(t).relations["node"]
	defer func() {
		if recover() == nil {
			t.Error("add to a frozen relation did not panic")
		}
	}()
	r.add(str("a"))
}

func mustParse(t *testing.T, text string) Program {
	t.Helper()
	p, err := Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}
