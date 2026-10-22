package datalog_test

import (
	"context"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/mattjoyce/shearline/datalog"
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
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4096 {
			t.Skip()
		}
		p, err := datalog.Parse(s)
		if err != nil {
			return
		}
		checkStringsAreText(t, p)
		schema := datalog.Schema{Arities: map[string]int{"input": 1}}
		plan, err := datalog.Compile(schema, p)
		if err != nil {
			return
		}
		base, err := datalog.NewBase(schema, datalog.Fact{Relation: "input", Tuple: datalog.Tuple{datalog.String("x")}})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, _ = datalog.Evaluate(ctx, plan, base, datalog.Options{Proofs: true, MaxSteps: 1000, MaxBytes: 4096})
	})
}

// checkStringsAreText fails if a parsed program holds a string constant that is not
// valid UTF-8, which the lexer is to refuse.
func checkStringsAreText(t *testing.T, p datalog.Program) {
	t.Helper()
	check := func(terms []datalog.Term) {
		for _, term := range terms {
			if !term.IsVar() && term.Const.Kind() == datalog.KindString && !utf8.ValidString(term.Const.Text()) {
				t.Fatalf("Parse produced the invalid string %q", term.Const.Text())
			}
		}
	}
	for _, r := range p.Rules {
		check(r.Head.Args)
		for _, l := range r.Body {
			if l.Atom != nil {
				check(l.Atom.Args)
			}
			if l.Compare != nil {
				check([]datalog.Term{l.Compare.Left, l.Compare.Right})
			}
		}
	}
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
		schema := datalog.Schema{Arities: map[string]int{"edge": 2}}
		plan, err := datalog.Compile(schema, parse(t, `reach(X,Y) :- edge(X,Y). reach(X,Z) :- reach(X,Y), edge(Y,Z).`))
		if err != nil {
			t.Fatal(err)
		}
		edges, facts := graphFrom(data)
		want := edges.closure()
		base, err := datalog.NewBase(schema, facts...)
		if err != nil {
			t.Fatal(err)
		}
		res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{Proofs: true, MaxSteps: 100000, MaxBytes: 100000})
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
		schema := datalog.Schema{Arities: map[string]int{"edge": 2, "node": 1}}
		plan, err := datalog.Compile(schema, parse(t,
			`loop(X) :- edge(X,X). safe(X) :- node(X), not loop(X). disconnected(X,Y) :- node(X), node(Y), not edge(X,Y), X != Y.`))
		if err != nil {
			t.Fatal(err)
		}
		edges, facts := graphFrom(data)
		base, err := datalog.NewBase(schema, append(nodeFacts(), facts...)...)
		if err != nil {
			t.Fatal(err)
		}
		res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{Proofs: true, MaxSteps: 100000, MaxBytes: 100000})
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
