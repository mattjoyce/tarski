package tarski_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Frozen bytes for proofs and absences (README § What it promises consumers).
// Proof.MarshalJSON and Absence.MarshalJSON are a contract with the consumers that store
// or display them, beside the answer bytes of golden_test.go. A change to a literal
// here is a breaking change: bump the version, write it in the changelog, and post it to
// every consumer before merging. Never update a literal to make the test pass. The
// rendered String text is stable but not frozen, and is not pinned here.

const goldenProofRules = `
path(X, Y) :- edge(X, Y).
path(X, Z) :- path(X, Y), edge(Y, Z).
twice(X, Y) :- path(X, Y), path(X, Y).
safe(X) :- node(X), not blocked(X).
deg(X, N) :- node(X), N = count : { edge(X, _) }.
none(X) :- node(X), blocked(X), edge(X, X).
`

var goldenProofs = map[string]string{
	"proof of a recursive tuple":    `{"relation":"path","tuple":["a","c"],"rule":{"set":"golden-proof/1","line":3,"text":"path(X, Z) :- path(X, Y), edge(Y, Z)."},"body":[{"relation":"path","tuple":["a","b"],"rule":{"set":"golden-proof/1","line":2,"text":"path(X, Y) :- edge(X, Y)."},"body":[{"relation":"edge","tuple":["a","b"],"rule":null}]},{"relation":"edge","tuple":["b","c"],"rule":null}]}`,
	"proof with a shared sub-proof": `{"relation":"twice","tuple":["a","b"],"rule":{"set":"golden-proof/1","line":4,"text":"twice(X, Y) :- path(X, Y), path(X, Y)."},"body":[{"relation":"path","tuple":["a","b"],"rule":{"set":"golden-proof/1","line":2,"text":"path(X, Y) :- edge(X, Y)."},"body":[{"relation":"edge","tuple":["a","b"],"rule":null}]},{"relation":"path","tuple":["a","b"],"rule":null,"shown":true}]}`,
	"proof of an aggregate tuple":   `{"relation":"deg","tuple":["a",2],"rule":{"set":"golden-proof/1","line":6,"text":"deg(X, N) :- node(X), N = count : { edge(X, _) }."},"body":[{"relation":"node","tuple":["a"],"rule":null}],"group":[[{"relation":"edge","tuple":["a","b"],"rule":null}],[{"relation":"edge","tuple":["a","d"],"rule":null}]],"group_size":2}`,
	"absence behind a base fact":    `{"relation":"path","goal":["a","z"],"truncated":false,"rules":[{"rule":{"set":"golden-proof/1","line":2,"text":"path(X, Y) :- edge(X, Y)."},"bound":{"X":"a","Y":"z"},"failed":"edge(X, Y)","because":[{"relation":"edge","goal":["a","z"],"truncated":false,"rules":[]}]},{"rule":{"set":"golden-proof/1","line":3,"text":"path(X, Z) :- path(X, Y), edge(Y, Z)."},"bound":{"X":"a","Y":"b","Z":"z"},"failed":"edge(Y, Z)","because":[{"relation":"edge","goal":["b","z"],"truncated":false,"rules":[]}]}]}`,
	"absence blocked by a negation": `{"relation":"safe","goal":["b"],"truncated":false,"rules":[{"rule":{"set":"golden-proof/1","line":5,"text":"safe(X) :- node(X), not blocked(X)."},"bound":{"X":"b"},"failed":"not blocked(X)","blocked":{"relation":"blocked","tuple":["b"],"rule":null}}]}`,
	"absence of an aggregate value": `{"relation":"deg","goal":["a",5],"truncated":false,"rules":[{"rule":{"set":"golden-proof/1","line":6,"text":"deg(X, N) :- node(X), N = count : { edge(X, _) }."},"bound":{"N":5,"X":"a"},"failed":"N = count : { edge(X, _) }","value":2,"reason":"the aggregate is 2, not 5"}]}`,
	"absence of a relation (empty)": `{"relation":"none","goal":[null],"truncated":false,"rules":[{"rule":{"set":"golden-proof/1","line":7,"text":"none(X) :- node(X), blocked(X), edge(X, X)."},"bound":{"X":"b"},"failed":"edge(X, X)","because":[{"relation":"edge","goal":["b","b"],"truncated":false,"rules":[]}]}]}`,
}

func goldenProofResult(t *testing.T) tarski.Result {
	t.Helper()
	schema := tarski.Schema{Arities: map[string]int{"edge": 2, "node": 1, "blocked": 1}}
	set, err := tarski.NewRuleSet("golden-proof", 1, goldenProofRules)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tarski.Compile(schema, set.Program)
	if err != nil {
		t.Fatal(err)
	}
	edge := func(a, b string) tarski.Fact {
		return tarski.Fact{Relation: "edge", Tuple: tarski.Tuple{tarski.String(a), tarski.String(b)}}
	}
	unary := func(rel, a string) tarski.Fact {
		return tarski.Fact{Relation: rel, Tuple: tarski.Tuple{tarski.String(a)}}
	}
	base, err := tarski.NewBase(schema,
		edge("a", "b"), edge("b", "c"), edge("a", "d"),
		unary("node", "a"), unary("node", "b"), unary("blocked", "b"),
	)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestGolden_ProofAndAbsenceJSON(t *testing.T) {
	res := goldenProofResult(t)
	s := func(vals ...string) tarski.Tuple {
		out := make(tarski.Tuple, len(vals))
		for i, v := range vals {
			out[i] = tarski.String(v)
		}
		return out
	}
	proof := func(rel string, tp tarski.Tuple) any {
		p, ok := res.Proof(rel, tp)
		if !ok {
			t.Fatalf("Proof(%s %v): not ok", rel, tp)
		}
		return p
	}
	whyNot := func(rel string, tp tarski.Tuple) any {
		a, ok := res.WhyNot(rel, tp, 0)
		if !ok {
			t.Fatalf("WhyNot(%s %v): not ok", rel, tp)
		}
		return a
	}
	five, err := tarski.Number("5")
	if err != nil {
		t.Fatal(err)
	}
	empty, ok := res.WhyEmpty("none", 0)
	if !ok {
		t.Fatal("WhyEmpty(none): not ok")
	}
	got := map[string]any{
		"proof of a recursive tuple":    proof("path", s("a", "c")),
		"proof with a shared sub-proof": proof("twice", s("a", "b")),
		"proof of an aggregate tuple":   proof("deg", tarski.Tuple{tarski.String("a"), tarski.Int(2)}),
		"absence behind a base fact":    whyNot("path", s("a", "z")),
		"absence blocked by a negation": whyNot("safe", s("b")),
		"absence of an aggregate value": whyNot("deg", tarski.Tuple{tarski.String("a"), five}),
		"absence of a relation (empty)": empty,
	}
	for name, want := range goldenProofs {
		b, err := json.Marshal(got[name])
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(b) != want {
			t.Errorf("%s: JSON bytes moved:\n got %s\nwant %s", name, b, want)
		}
	}
}
