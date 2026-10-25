package tarski_test

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mattjoyce/tarski"
)

// The fuzz tests run their seeds under plain `go test`; to explore, name one:
//
//	go test ./datalog -run '^$' -fuzz '^FuzzTransitiveClosure$' -fuzztime=30s

// FuzzParseCompileEvaluate sends arbitrary text through the whole pipeline under tight
// bounds. Nothing may panic, and a program that parses holds only valid UTF-8 strings.
func FuzzParseCompileEvaluate(f *testing.F) {
	seeds := []string{
		`out("x").`, `out(X) :- input(X).`, `out(X) :- input(X), not out(X).`, `out/1.`, "\xff", "",
		`out(X) :- input(X), X >= "a", X != "b".`, `out("é日本é\x00").`, "out(\"\xff\").", `out("\xff").`,
		`a(X) :- input(X). b(X) :- a(X), not c(X). c(X) :- b(X).`, `out(1e999).`, `out(-0).`,
		`out(N) :- N = count : { input(_) }.`, `out(X, S) :- input(X), S = sum V : { input(V), not out(V, _), V > 1 }.`,
		`out(X, L) :- input(X), L = max Y : { input(Y), prefix(X, Y) }, M = min Y : { input(Y) }.`,
		`count(X) :- input(X). sum(N) :- N = count : { count(_) }.`, `out(N) :- N = avg X : { input(X) }.`,
		`out(N) :- N = count : { M = count : { input(_) } }.`, `out(N) :- N = count : { }.`,
		`out(X, N) :- input(X), N = count : { input(Y), Y != X }.`, `out(S) :- S = sum X : { input(X) }.`,
		`a(X) :- input(X). out(X, N) :- a(X), N = min Y : { a(Y), not input(Y) }.`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			t.Skip()
		}
		p, err := tarski.Parse(s)
		if err != nil {
			return
		}
		checkStringsAreText(t, p)
		schema := tarski.Schema{Arities: map[string]int{"input": 1}}
		plan, err := tarski.Compile(schema, p)
		// What the parser makes passes the shape check, so a parsed aggregate is never
		// reported as malformed (checkShape's refusals name "program 1, rule N").
		if holdsAggregate(p) && err != nil && strings.Contains(err.Error(), "program 1, rule ") {
			t.Fatalf("Compile of a parsed aggregate = %v, want no shape refusal", err)
		}
		if err != nil {
			return
		}
		base, err := tarski.NewBase(schema, tarski.Fact{Relation: "input", Tuple: tarski.Tuple{tarski.String("x")}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, _ = tarski.Evaluate(ctx, plan, base, tarski.Options{Proofs: true, MaxSteps: 1000, MaxBytes: 4096})
	})
}

// checkStringsAreText fails if a parsed program holds a string constant that is not
// valid UTF-8, which the lexer is to refuse.
func checkStringsAreText(t *testing.T, p tarski.Program) {
	t.Helper()
	check := func(terms []tarski.Term) {
		for _, term := range terms {
			if !term.IsVar() && term.Const.Kind() == tarski.KindString && !utf8.ValidString(term.Const.Text()) {
				t.Fatalf("Parse produced the invalid string %q", term.Const.Text())
			}
		}
	}
	var body func([]tarski.Literal)
	body = func(ls []tarski.Literal) {
		for _, l := range ls {
			if l.Atom != nil {
				check(l.Atom.Args)
			}
			if l.Compare != nil {
				check([]tarski.Term{l.Compare.Left, l.Compare.Right})
			}
			if l.Aggregate != nil {
				body(l.Aggregate.Body)
			}
		}
	}
	for _, r := range p.Rules {
		check(r.Head.Args)
		body(r.Body)
	}
}

// holdsAggregate reports whether any rule of p has an aggregate literal.
func holdsAggregate(p tarski.Program) bool {
	for _, r := range p.Rules {
		for _, l := range r.Body {
			if l.Aggregate != nil {
				return true
			}
		}
	}
	return false
}

// FuzzTransitiveClosure checks reach/2 against Floyd-Warshall on graphs of up to eight
// nodes: every pair is derived exactly when the oracle says, with a proof, and every
// pair that is not has a why-not.
func FuzzTransitiveClosure(f *testing.F) {
	f.Add([]byte{0, 1, 1, 2, 2, 0})
	f.Add([]byte{0, 0})
	f.Add([]byte{0, 1, 0, 1, 0, 1})
	f.Add([]byte{0, 1, 1, 2, 2, 3, 3, 4, 4, 5, 5, 6, 6, 7})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			t.Skip()
		}
		schema := tarski.Schema{Arities: map[string]int{"edge": 2}}
		plan, err := tarski.Compile(schema, parse(t, `reach(X,Y) :- edge(X,Y). reach(X,Z) :- reach(X,Y), edge(Y,Z).`))
		if err != nil {
			t.Fatal(err)
		}
		edges, facts := graphFrom(data)
		want := edges.closure()
		base, err := tarski.NewBase(schema, facts...)
		if err != nil {
			t.Fatal(err)
		}
		res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true, MaxSteps: 100000, MaxBytes: 100000})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < oracleNodes; i++ {
			for j := 0; j < oracleNodes; j++ {
				p, got := res.Proof("reach", ints(i, j))
				if got != want[i][j] {
					t.Fatalf("reach(%d,%d)=%v want %v", i, j, got, want[i][j])
				}
				if got && p.Rule == nil {
					t.Fatalf("reach(%d,%d) is derived but its proof is a base fact", i, j)
				}
				if _, ok := res.WhyNot("reach", ints(i, j), 4); ok == got {
					t.Fatalf("WhyNot(reach(%d,%d)) ok=%v with the tuple derived=%v", i, j, ok, got)
				}
			}
		}
	})
}

// FuzzNegationAndRepeatedVariables checks a repeated variable in a body atom
// (edge(X,X)) and stratified negation over it against the matrix directly.
func FuzzNegationAndRepeatedVariables(f *testing.F) {
	f.Add([]byte{0, 1, 1, 1, 2, 3})
	f.Add([]byte{0, 0})
	f.Add([]byte{3, 3, 3, 3})
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 128 {
			t.Skip()
		}
		schema := tarski.Schema{Arities: map[string]int{"edge": 2, "node": 1}}
		plan, err := tarski.Compile(schema, parse(t,
			`loop(X) :- edge(X,X). safe(X) :- node(X), not loop(X). disconnected(X,Y) :- node(X), node(Y), not edge(X,Y), X != Y.`))
		if err != nil {
			t.Fatal(err)
		}
		edges, facts := graphFrom(data)
		base, err := tarski.NewBase(schema, append(nodeFacts(), facts...)...)
		if err != nil {
			t.Fatal(err)
		}
		res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true, MaxSteps: 100000, MaxBytes: 100000})
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < oracleNodes; i++ {
			if _, safe := res.Proof("safe", ints(i)); safe == edges[i][i] {
				t.Fatalf("safe(%d)=%v but the node has a self-loop=%v", i, safe, edges[i][i])
			}
			for j := 0; j < oracleNodes; j++ {
				_, got := res.Proof("disconnected", ints(i, j))
				if want := i != j && !edges[i][j]; got != want {
					t.Fatalf("disconnected(%d,%d)=%v want %v", i, j, got, want)
				}
			}
		}
	})
}
