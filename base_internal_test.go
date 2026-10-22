package datalog

import (
	"context"
	"strings"
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

// The order tuples enter every derived relation is a function of the plan and the
// base, not of map order: several rules derive the same relations, in a recursion, and
// repeated evaluations agree tuple for tuple.
func TestEvaluate_DerivationOrderIsDeterministic(t *testing.T) {
	t.Parallel()
	var facts []Fact
	for i := 0; i < 8; i++ {
		facts = append(facts, Fact{"edge", str(string(rune('a'+i)), string(rune('a'+(i+1)%8)))},
			Fact{"edge", str(string(rune('a'+i)), string(rune('a'+(i+3)%8)))}, Fact{"node", str(string(rune('a' + i)))})
	}
	base := internalBase(t, facts...)
	plan, err := Compile(Schema{Arities: map[string]int{"edge": 2, "node": 1}}, mustParse(t, `
		step(X, Y) :- edge(X, Y).
		step(X, Y) :- edge(Y, X).
		reach(X, Y) :- step(X, Y).
		reach(X, Z) :- reach(X, Y), step(Y, Z).
		back(X, Y) :- reach(Y, X), node(X).
		both(X, Y) :- reach(X, Y), back(X, Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	order := func() map[string][]string {
		res, err := Evaluate(context.Background(), plan, base, Options{})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		out := map[string][]string{}
		for name := range plan.idb {
			for _, tup := range res.relations[name].tuples {
				out[name] = append(out[name], tup.key())
			}
		}
		return out
	}
	first := order()
	if len(first["both"]) == 0 {
		t.Fatal("the program derived nothing; the test proves nothing")
	}
	for i := 0; i < 25; i++ {
		got := order()
		for name, want := range first {
			if strings.Join(got[name], "|") != strings.Join(want, "|") {
				t.Fatalf("run %d: %s derived in a different order", i, name)
			}
		}
	}
}
