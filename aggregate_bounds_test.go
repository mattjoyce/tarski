package tarski_test

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/tarski"
)

// aggEval compiles text over schema and evaluates it over facts.
func aggEval(t *testing.T, schema tarski.Schema, facts []tarski.Fact, text string, opts tarski.Options) (tarski.Result, error) {
	t.Helper()
	plan, err := tarski.Compile(schema, parse(t, text))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := tarski.NewBase(schema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	return tarski.Evaluate(context.Background(), plan, base, opts)
}

// Every bound holds inside an aggregate: the candidates its body visits are join steps,
// its head tuples are bytes, and a sum too wide to hold exactly is refused. Each names
// the relation being derived, and none returns a partial result.
func TestEvaluate_AggregateBounds(t *testing.T) {
	t.Parallel()
	huge, err := tarski.Number("1e70000")
	if err != nil {
		t.Fatal(err)
	}
	wide := append(aggFacts(), tarski.Fact{Relation: "item", Tuple: tarski.Tuple{tarski.String("h"), huge}})
	cases := []struct {
		name, text string
		facts      []tarski.Fact
		opts       tarski.Options
		bound      string
		relation   string
	}{
		{"steps inside the braces", `n(N) :- N = count : { item(_, _), item(_, _), item(_, _) }.`, aggFacts(),
			tarski.Options{MaxSteps: 50}, tarski.BoundSteps, "n"},
		{"steps of a min over a product", `n(N) :- N = min V : { item(_, V), item(_, _), word(_) }.`, aggFacts(),
			tarski.Options{MaxSteps: 20, Proofs: true}, tarski.BoundSteps, "n"},
		{"bytes of the head tuples", `p(G, V, N) :- item(G, V), N = count : { item(_, _) }.`, aggFacts(),
			tarski.Options{MaxBytes: 20}, tarski.BoundBytes, "p"},
		{"a sum too wide to hold exactly", `s(S) :- S = sum V : { item(_, V) }.`, wide,
			tarski.Options{}, tarski.BoundNumber, "s"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res, err := aggEval(t, aggSchema, c.facts, c.text, c.opts)
			var tl *tarski.TooLarge
			if !errors.As(err, &tl) || !errors.Is(err, tarski.ErrTooLarge) {
				t.Fatalf("Evaluate = %v, want *TooLarge", err)
			}
			if tl.Bound != c.bound || tl.Relation != c.relation {
				t.Errorf("Bound, Relation = %q, %q; want %q, %q", tl.Bound, tl.Relation, c.bound, c.relation)
			}
			if !strings.Contains(err.Error(), c.relation) {
				t.Errorf("message %q does not name %s", err, c.relation)
			}
			if _, err := res.Relation(c.relation); err == nil {
				t.Error("a refused evaluation still answers")
			}
		})
	}
}

// The deadline is still seen inside an aggregate's body: a builtin there cancels the
// context on its first call, and the evaluation stops within stepsPerDeadlineCheck
// steps instead of finishing the group.
func TestEvaluate_AggregateHonoursTheDeadline(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	schema := aggSchema
	schema.Builtins = map[string]tarski.Builtin{"stop": {
		Args:  []tarski.BuiltinArg{{Name: "X"}},
		Holds: func([]tarski.Const) bool { cancel(); return true },
	}}
	plan, err := tarski.Compile(schema, parse(t, `n(N) :- N = count : { item(_, V), stop(V), item(_, _), item(_, _), item(_, _) }.`))
	if err != nil {
		t.Fatal(err)
	}
	base, err := tarski.NewBase(schema, aggFacts()...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tarski.Evaluate(ctx, plan, base, tarski.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Evaluate = %v, want context.Canceled", err)
	}
}

// aggProgram derives with every op, a recursion below and a stratum above, so a
// determinism check covers each path.
const aggProgram = `
reach(X, Y) :- edge(X, Y). reach(X, Z) :- reach(X, Y), edge(Y, Z).
n(G, N) :- grp(G), N = count : { item(G, _) }.
s(G, S) :- grp(G), S = sum V : { item(G, V), not bad(V) }.
lo(G, M) :- grp(G), M = min V : { item(G, V) }.
hi(W) :- W = max X : { word(X) }.
r(X, N) :- edge(X, _), N = count : { reach(X, _) }.
busy(G) :- n(G, N), N > 2.
`

// aggHashes evaluates aggProgram and returns each derived relation's answer hash.
func aggHashes(t *testing.T, plan tarski.Plan, base tarski.Base, opts tarski.Options) map[string]string {
	res, err := tarski.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		t.Error(err)
		return nil
	}
	out := map[string]string{}
	for _, name := range []string{"reach", "n", "s", "lo", "hi", "r", "busy"} {
		ans, err := res.Relation(name)
		if err != nil {
			t.Error(err)
			return nil
		}
		out[name] = ans.Hash()
	}
	return out
}

// Two evaluations of one plan over one base give identical answers, with proofs on or
// off, sequentially and concurrently (run under -race).
func TestEvaluate_AggregatesAreDeterministic(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(aggSchema, parse(t, aggProgram))
	if err != nil {
		t.Fatal(err)
	}
	base, err := tarski.NewBase(aggSchema, aggFacts()...)
	if err != nil {
		t.Fatal(err)
	}
	want := aggHashes(t, plan, base, tarski.Options{})
	if got := aggHashes(t, plan, base, tarski.Options{Proofs: true}); !mapsEqual(got, want) {
		t.Errorf("with proofs: %v, want %v", got, want)
	}
	var wg sync.WaitGroup
	results := make([]map[string]string, 8)
	for i := range results {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = aggHashes(t, plan, base, tarski.Options{Proofs: i%2 == 0})
		}()
	}
	wg.Wait()
	for i, got := range results {
		if !mapsEqual(got, want) {
			t.Errorf("concurrent evaluation %d: %v, want %v", i, got, want)
		}
	}
}

func mapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// Until proofs carry the group (card 6 slice 4), the proof of an aggregate-derived tuple
// is its rule and its positive atoms outside the braces, and a why-not that reaches an
// aggregate literal claims no failing literal: it says it was cut short. Neither
// panics, and both render.
func TestProofAndWhyNot_ThroughAnAggregate(t *testing.T) {
	t.Parallel()
	text := `n(G, N) :- grp(G), N = count : { item(G, _) }.
pair(G) :- grp(G), n(X, X).
busy(G) :- grp(G), n(G, 5).`
	res, err := aggEval(t, aggSchema, aggFacts(), text, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := res.Proof("n", tarski.Tuple{tarski.String("a"), tarski.Int(3)})
	if !ok || p.Rule == nil || len(p.Body) != 1 || p.Body[0].Relation != "grp" {
		t.Fatalf("Proof(n(a, 3)) = %+v, %v; want the rule with grp(a) as its body", p, ok)
	}
	if _, err := json.Marshal(p); err != nil || p.String() == "" {
		t.Errorf("proof renders: %v", err)
	}
	goals := []struct {
		relation string
		tuple    tarski.Tuple
	}{
		{"n", tarski.Tuple{tarski.String("a"), tarski.Int(4)}},
		{"pair", tarski.Tuple{tarski.String("a")}},
		{"busy", tarski.Tuple{tarski.String("a")}},
	}
	for _, g := range goals {
		a, ok := res.WhyNot(g.relation, g.tuple, 0)
		if !ok || !a.Truncated || len(a.Rules) != 1 {
			t.Fatalf("WhyNot(%s%v) = %+v, %v; want one rule, truncated", g.relation, g.tuple, a, ok)
		}
		if _, err := json.Marshal(a); err != nil || a.String() == "" {
			t.Errorf("WhyNot(%s) renders: %v", g.relation, err)
		}
		if g.relation != "n" {
			continue
		}
		if f := a.Rules[0].Failed; f.Atom != nil || f.Compare != nil || f.Aggregate != nil {
			t.Errorf("WhyNot(n) claims the failing literal %+v; want none", f)
		}
	}
}

// TestAggregate_SumWorkCharged: folding a sum aligns every addend to the group's
// width, so a few hundred witnesses of sixty-thousand-digit spread cost far more than
// their join steps. The fold is charged to MaxSteps before it runs, so a small bound
// stops it rather than the work escaping the bound (commit security review).
func TestAggregate_SumWorkCharged(t *testing.T) {
	schema := tarski.Schema{Arities: map[string]int{"v": 2}}
	var facts []tarski.Fact
	for i := range 200 {
		lexeme := "1e60000"
		if i%2 == 1 {
			lexeme = strconv.Itoa(i)
		}
		n, err := tarski.Number(lexeme)
		if err != nil {
			t.Fatal(err)
		}
		facts = append(facts, tarski.Fact{Relation: "v", Tuple: tarski.Tuple{tarski.Int(int64(i)), n}})
	}
	base, err := tarski.NewBase(schema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := tarski.Parse("total(S) :- S = sum X : { v(_, X) }.\n")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tarski.Compile(schema, prog)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// The join visits about 200 tuples; the fold is 200 addends over 60001 digits.
	_, err = tarski.Evaluate(ctx, plan, base, tarski.Options{MaxSteps: 10_000})
	var tl *tarski.TooLarge
	if !errors.As(err, &tl) || tl.Bound != tarski.BoundSteps {
		t.Fatalf("Evaluate with MaxSteps 10000 = %v, want *TooLarge on BoundSteps", err)
	}
	if _, err := tarski.Evaluate(ctx, plan, base, tarski.Options{MaxSteps: 1_000_000}); err != nil {
		t.Fatalf("Evaluate with room for the fold: %v", err)
	}
}
