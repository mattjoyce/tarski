package tarski_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The proof of an aggregate-derived tuple carries the group it was taken over
// (docs/design/aggregates.md §5.1-§5.3): Group holds the kept witnesses, each the
// positive atoms inside the braces with their proofs, and GroupSize the whole size.

// aggProofText is the program the proof tests share. from and r are derived, so a
// group over r has witnesses with proofs of their own.
const aggProofText = `n(G, N) :- grp(G), N = count : { item(G, _) }.
s(G, S) :- grp(G), S = sum X : { item(G, X) }.
lo(G, L) :- grp(G), L = min X : { item(G, X) }.
hi(G, H) :- grp(G), H = max X : { item(G, X) }.
two(G, N, S) :- grp(G), N = count : { item(G, _) }, S = sum X : { item(G, X) }.
top(G, X, N) :- item(G, X), N = count : { item(G, _) }.
twice(G, S) :- grp(G), S = sum X : { item(G, X), item(G, X) }.
r(X, Y) :- edge(X, Y).
r(X, Z) :- r(X, Y), edge(Y, Z).
from(X) :- edge(X, _).
reach(X, N) :- from(X), N = count : { r(X, _) }.`

func aggProofResult(t *testing.T, facts []tarski.Fact) tarski.Result {
	t.Helper()
	res, err := aggEval(t, aggSchema, facts, aggProofText, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// witnessTuples is each kept witness as `rel["a",1] rel["a",1]`, in order.
func witnessTuples(p tarski.Proof) []string {
	out := make([]string, len(p.Group))
	for i, w := range p.Group {
		parts := make([]string, len(w))
		for j, sub := range w {
			raw, _ := json.Marshal(sub.Tuple)
			parts[j] = sub.Relation + string(raw)
		}
		out[i] = strings.Join(parts, " ")
	}
	return out
}

func TestProof_AggregateGroup(t *testing.T) {
	t.Parallel()
	res := aggProofResult(t, aggFacts())
	a, e := tarski.String("a"), tarski.String("e")
	num := func(s string) tarski.Const {
		c, err := tarski.Number(s)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	items := []string{`item["a",1]`, `item["a",2]`, `item["a",3]`}
	cases := []struct {
		name     string
		relation string
		tuple    tarski.Tuple
		size     int
		want     []string
		text     string // a line String must hold
	}{
		{"count", "n", tarski.Tuple{a, tarski.Int(3)}, 3, items, "  group of 3\n    item(\"a\", 1)  fact\n"},
		{"sum", "s", tarski.Tuple{a, tarski.Int(6)}, 3, items, "  group of 3\n"},
		{"min", "lo", tarski.Tuple{a, tarski.Int(1)}, 3, items, "  group of 3\n"},
		{"max", "hi", tarski.Tuple{a, tarski.Int(3)}, 3, items, "  group of 3\n"},
		{"empty count", "n", tarski.Tuple{e, tarski.Int(0)}, 0, []string{}, "  grp(\"e\")  fact\n  group of 0\n"},
		{"empty sum", "s", tarski.Tuple{e, tarski.Int(0)}, 0, []string{}, "  group of 0\n"},
		{"two aggregates concatenate", "two", tarski.Tuple{a, tarski.Int(3), tarski.Int(6)}, 6,
			append(append([]string{}, items...), items...), "  group of 6\n"},
		{"several atoms per witness", "twice", tarski.Tuple{tarski.String("b"), num("9.5")}, 3,
			[]string{`item["b",10] item["b",10]`, `item["b",-2.5] item["b",-2.5]`, `item["b",2] item["b",2]`},
			"    item(\"b\", 10)  fact\n    item(\"b\", 10)  (shown above)\n"},
		{"derived witnesses", "reach", tarski.Tuple{tarski.String("x"), tarski.Int(3)}, 3,
			[]string{`r["x","y"]`, `r["x","z"]`, `r["x","w"]`}, "      r(\"x\", \"y\")  (shown above)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, ok := res.Proof(tc.relation, tc.tuple)
			if !ok {
				t.Fatalf("Proof(%s%v) not found", tc.relation, tc.tuple)
			}
			if p.GroupSize != tc.size {
				t.Errorf("GroupSize = %d, want %d", p.GroupSize, tc.size)
			}
			if got := witnessTuples(p); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("group = %q, want %q", got, tc.want)
			}
			if s := p.String(); !strings.Contains(s, tc.text) {
				t.Errorf("String() =\n%s\nwant it to hold\n%s", s, tc.text)
			}
		})
	}
}

// A group over 64 witnesses keeps the first 64, in witness order, and states its size.
func TestProof_AggregateGroupCap(t *testing.T) {
	t.Parallel()
	facts := []tarski.Fact{fact("grp", "g")}
	for i := range 100 {
		facts = append(facts, fact("item", "g", i))
	}
	res := aggProofResult(t, facts)
	p, ok := res.Proof("n", tarski.Tuple{tarski.String("g"), tarski.Int(100)})
	if !ok {
		t.Fatal("Proof(n(g, 100)) not found")
	}
	if len(p.Group) != 64 || p.GroupSize != 100 {
		t.Fatalf("len(Group), GroupSize = %d, %d; want 64, 100", len(p.Group), p.GroupSize)
	}
	for i, w := range p.Group {
		if len(w) != 1 || w[0].Tuple[1] != tarski.Int(int64(i)) {
			t.Fatalf("witness %d = %v, want item(g, %d)", i, w, i)
		}
	}
	if !strings.Contains(p.String(), "  group of 100 (64 shown)\n") {
		t.Errorf("String() does not say how many were shown:\n%s", p.String())
	}
	raw, err := json.Marshal(p)
	if err != nil || !strings.Contains(string(raw), `"group_size":100`) {
		t.Errorf("JSON = %s, %v; want group_size 100", raw, err)
	}
}

// A tuple shared by the body and a witness, or by two witnesses, is one Proof value,
// and the JSON writes it once: the second time it is a reference.
func TestProof_AggregateGroupIsShared(t *testing.T) {
	t.Parallel()
	res := aggProofResult(t, aggFacts())
	p, ok := res.Proof("top", tarski.Tuple{tarski.String("a"), tarski.Int(1), tarski.Int(3)})
	if !ok {
		t.Fatal("Proof(top(a, 1, 3)) not found")
	}
	if !reflect.DeepEqual(p.Body[0], p.Group[0][0]) {
		t.Errorf("body item(a, 1) and witness item(a, 1) differ: %+v, %+v", p.Body[0], p.Group[0][0])
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	want := `"body":[{"relation":"item","tuple":["a",1],"rule":null}],` +
		`"group":[[{"relation":"item","tuple":["a",1],"rule":null,"shown":true}],` +
		`[{"relation":"item","tuple":["a",2],"rule":null}],[{"relation":"item","tuple":["a",3],"rule":null}]],"group_size":3}`
	if !strings.HasSuffix(string(raw), want) {
		t.Errorf("JSON =\n%s\nwant it to end\n%s", raw, want)
	}
	reach, _ := res.Proof("reach", tarski.Tuple{tarski.String("x"), tarski.Int(3)})
	if !reflect.DeepEqual(reach.Group[1][0].Body[0], reach.Group[0][0]) {
		t.Error("r(x, y) inside r(x, z)'s proof is not the witness r(x, y)'s proof")
	}
	raw, _ = json.Marshal(reach)
	if n := strings.Count(string(raw), `"tuple":["x","y"],"rule":{`); n != 1 {
		t.Errorf("r(x, y) is written out %d times, want once:\n%s", n, raw)
	}
}

// Two evaluations give the same proofs, value and bytes, groups included.
func TestProof_AggregateDeterministic(t *testing.T) {
	t.Parallel()
	goals := []struct {
		relation string
		tuple    tarski.Tuple
	}{
		{"two", tarski.Tuple{tarski.String("b"), tarski.Int(3), mustNumber(t, "9.5")}},
		{"reach", tarski.Tuple{tarski.String("x"), tarski.Int(3)}},
	}
	first, second := aggProofResult(t, aggFacts()), aggProofResult(t, aggFacts())
	for _, g := range goals {
		p, ok1 := first.Proof(g.relation, g.tuple)
		q, ok2 := second.Proof(g.relation, g.tuple)
		if !ok1 || !ok2 || p.GroupSize == 0 {
			t.Fatalf("Proof(%s%v) = %v, %v, size %d", g.relation, g.tuple, ok1, ok2, p.GroupSize)
		}
		pj, _ := json.Marshal(p)
		qj, _ := json.Marshal(q)
		if !reflect.DeepEqual(p, q) || string(pj) != string(qj) || p.String() != q.String() {
			t.Errorf("Proof(%s%v) differs between evaluations:\n%s\n%s", g.relation, g.tuple, pj, qj)
		}
	}
}

// A proof of a tuple no aggregate derived has no group, and its JSON names none.
func TestProof_NoAggregateNoGroup(t *testing.T) {
	t.Parallel()
	res := aggProofResult(t, aggFacts())
	p, ok := res.Proof("r", tarski.Tuple{tarski.String("x"), tarski.String("z")})
	if !ok || p.Group != nil || p.GroupSize != 0 {
		t.Fatalf("Proof(r(x, z)) = %+v, %v; want no group", p, ok)
	}
	raw, _ := json.Marshal(p)
	if strings.Contains(string(raw), "group") || strings.Contains(p.String(), "group") {
		t.Errorf("a proof with no aggregate mentions a group: %s", raw)
	}
}

func mustNumber(t *testing.T, s string) tarski.Const {
	t.Helper()
	c, err := tarski.Number(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// BenchmarkEvaluate_Aggregate measures what recording a group's witnesses costs: the
// same aggregate program with proofs off and on.
func BenchmarkEvaluate_Aggregate(b *testing.B) {
	facts := []tarski.Fact{}
	for g := range 50 {
		facts = append(facts, fact("grp", fmt.Sprint("g", g)))
		for i := range 200 {
			facts = append(facts, fact("item", fmt.Sprint("g", g), i))
		}
	}
	base, err := tarski.NewBase(aggSchema, facts...)
	if err != nil {
		b.Fatal(err)
	}
	prog, err := tarski.Parse("n(G, N) :- grp(G), N = count : { item(G, _) }.\ns(G, S) :- grp(G), S = sum X : { item(G, X) }.\n")
	if err != nil {
		b.Fatal(err)
	}
	plan, err := tarski.Compile(aggSchema, prog)
	if err != nil {
		b.Fatal(err)
	}
	for _, proofs := range []bool{false, true} {
		b.Run(fmt.Sprintf("proofs=%v", proofs), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: proofs}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
