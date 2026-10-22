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

// A sealed base relation has an index for every position from the start, and reading it
// through candidates or an evaluation changes nothing. (The value has no add: that is
// a fact about the types, not something a test can observe.)
func TestRelation_SealedBaseHasEveryIndex(t *testing.T) {
	t.Parallel()
	b := internalBase(t, Fact{"edge", str("a", "b")}, Fact{"edge", str("a", "c")})
	r := b.relations["edge"]
	if len(r.idx) != 2 || r.arity != 2 {
		t.Fatalf("%d indexes, arity %d; want 2 and 2", len(r.idx), r.arity)
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
	if len(r.idx) != 2 || len(before) != 2 || r.len() != 2 || b.relations["edge"] != r {
		t.Error("Evaluate changed a sealed relation")
	}
}

// A derived relation comes out of Evaluate sealed without indexes, and still answers
// candidates correctly, by scanning. Sealing it builds nothing.
func TestRelation_SealedDerivedHasNoIndexAndScans(t *testing.T) {
	t.Parallel()
	plan, err := Compile(Schema{Arities: map[string]int{"edge": 2, "node": 1}}, mustParse(t, `p(X, Y) :- edge(X, Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	b := internalBase(t, Fact{"edge", str("a", "b")}, Fact{"edge", str("a", "c")}, Fact{"edge", str("b", "c")})
	res, err := Evaluate(context.Background(), plan, b, Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	p := res.relations["p"]
	if p.idx != nil || p.arity != 2 || p.len() != 3 {
		t.Fatalf("derived value: idx %v, arity %d, len %d; want no index, 2, 3", p.idx, p.arity, p.len())
	}
	if got := p.candidates(0, String("a")); len(got) != 2 || p.tuples[got[0]][1].Text() != "b" || p.tuples[got[1]][1].Text() != "c" {
		t.Errorf("candidates(0, a) = %v, want the two a-tuples in order", got)
	}
	if got := p.candidates(1, String("c")); len(got) != 2 {
		t.Errorf("candidates(1, c) = %v, want two", got)
	}
	if got := p.candidates(1, String("nothing")); len(got) != 0 || !p.has(str("b", "c")) || p.has(str("c", "b")) {
		t.Errorf("candidates(1, nothing) = %v, has wrong", got)
	}
}

// A builder indexes lazily and drops the index when it grows; sealing hands over its
// tuples and set, with indexes only when asked.
func TestRelBuilder_IndexesLazilyAndSeals(t *testing.T) {
	t.Parallel()
	b := newRelBuilder()
	b.add(str("a", "b"))
	if b.idx != nil {
		t.Error("a builder indexed before any lookup")
	}
	if got := b.candidates(0, String("a")); len(got) != 1 {
		t.Fatalf("candidates = %v, want one", got)
	}
	b.add(str("a", "c"))
	if got := b.candidates(0, String("a")); len(got) != 2 {
		t.Errorf("after growing, candidates = %v, want two: the index went stale", got)
	}
	if b.add(str("a", "c")) {
		t.Error("a duplicate was added")
	}
	plain := newRelBuilder()
	plain.add(str("x"))
	if v := plain.seal(1, false); v.idx != nil || v.len() != 1 {
		t.Errorf("seal(false) = idx %v, len %d; want none, 1", v.idx, v.len())
	}
	v := b.seal(2, true)
	if len(v.idx) != 2 || v.len() != 2 || !v.has(str("a", "b")) {
		t.Errorf("seal(true) = %d indexes, len %d", len(v.idx), v.len())
	}
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
