package tarski_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/tarski"
)

// FuzzCompileHandBuiltProgram sends Program values that Parse never makes through
// Compile, Evaluate and Plan.Demand: Program, Rule, Literal and Aggregate are exported, so
// a caller can build any shape of them. The bytes are decoded into hand-built ASTs with
// empty names, absurd arities, literals that are several things or none, unknown
// operators, aggregates that nest or contain themselves, and negative or huge
// declarations. Nothing may panic or overflow the stack, and a refusal is ErrInvalid.
// Explore with: go test -run '^$' -fuzz '^FuzzCompileHandBuiltProgram$' -fuzztime=30s
func FuzzCompileHandBuiltProgram(f *testing.F) {
	for _, seed := range [][]byte{
		nil, {0}, {255, 255, 255, 255}, []byte("rules and literals of every kind"),
		{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9},
		{4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4, 4},
	} {
		f.Add(seed)
	}
	schema := tarski.Schema{Arities: map[string]int{"input": 1, "edge": 2}}
	base, err := tarski.NewBase(schema, tarski.Fact{Relation: "input", Tuple: tarski.Tuple{tarski.String("x")}})
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		g := &astGen{data: data}
		programs := []tarski.Program{g.program()}
		if g.pick(3) == 0 {
			programs = append(programs, g.program())
		}
		plan, err := tarski.Compile(schema, programs...)
		if err != nil {
			if !errors.Is(err, tarski.ErrInvalid) {
				t.Fatalf("Compile of a hand-built program = %v, want ErrInvalid", err)
			}
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		opts := tarski.Options{Proofs: true, MaxSteps: 1000, MaxBytes: 4096}
		_, _ = tarski.Evaluate(ctx, plan, base, opts)
		if arity, ok := plan.Arity("out"); ok && plan.Defines("out") {
			_, _ = plan.Demand("out", strings.Repeat("b", arity))
		}
	})
}

// astGen decodes fuzz bytes into AST values. Past the end of the data every choice is
// zero, so any input decodes to a program, and the same bytes always to the same one.
type astGen struct {
	data []byte
	pos  int
}

// pick is the next choice in [0, n).
func (g *astGen) pick(n int) int {
	if g.pos >= len(g.data) {
		return 0
	}
	b := int(g.data[g.pos])
	g.pos++
	return b % n
}

var (
	genNames = []string{"out", "", "input", "edge", "Up", "has space", "\xff", "count", "_", "a1", strings.Repeat("n", 300)}
	genVars  = []string{"X", "Y", "N", "_", "", "x", "1", "\xff"}
	genOps   = []string{"=", "!=", "<", "<=", ">", ">=", "", "==", "~"}
	genAggs  = []string{"count", "sum", "min", "max", "avg", ""}
	genArity = []int{1, 2, 0, -1, 3, 1 << 20}
)

func (g *astGen) program() tarski.Program {
	p := tarski.Program{Name: genNames[g.pick(len(genNames))]}
	for range g.pick(4) {
		p.Rules = append(p.Rules, g.rule())
	}
	if g.pick(2) == 0 {
		p.Declared = map[string]int{}
		for range g.pick(3) {
			p.Declared[genNames[g.pick(len(genNames))]] = genArity[g.pick(len(genArity))]
		}
	}
	return p
}

func (g *astGen) rule() tarski.Rule {
	r := tarski.Rule{Head: g.atom(), Line: g.pick(3) - 1}
	for range g.pick(5) {
		r.Body = append(r.Body, g.literal(2))
	}
	return r
}

func (g *astGen) atom() tarski.Atom {
	a := tarski.Atom{Name: genNames[g.pick(len(genNames))], Line: g.pick(3) - 1}
	for range g.pick(4) {
		a.Args = append(a.Args, g.term())
	}
	if g.pick(8) == 0 {
		a.Args = make([]tarski.Term, genArity[g.pick(len(genArity))]&0xfff) // an absurd arity
	}
	return a
}

func (g *astGen) term() tarski.Term {
	switch g.pick(5) {
	case 0:
		return tarski.Term{}
	case 1:
		return tarski.Term{Const: tarski.String(genNames[g.pick(len(genNames))])}
	case 2:
		return tarski.Term{Const: tarski.Int(int64(g.pick(5)) - 2)}
	case 3:
		return tarski.Term{Const: tarski.Bool(g.pick(2) == 0)}
	}
	return tarski.Term{Var: genVars[g.pick(len(genVars))]}
}

// literal is one body element: usually one thing, sometimes none or several, which no
// parser makes. depth bounds how far an aggregate's body can nest further aggregates.
func (g *astGen) literal(depth int) tarski.Literal {
	var l tarski.Literal
	kind := g.pick(7)
	if kind == 0 || kind == 5 || kind == 6 {
		a := g.atom()
		l.Atom, l.Negated = &a, g.pick(3) == 0
	}
	if kind == 1 || kind == 5 {
		l.Compare = &tarski.Comparison{Op: genOps[g.pick(len(genOps))], Left: g.term(), Right: g.term(), Line: g.pick(3)}
		l.Negated = l.Negated || g.pick(6) == 0
	}
	if kind == 2 || kind == 3 || kind == 6 {
		l.Aggregate = g.aggregate(depth)
	}
	return l
}

// aggregate is a count, sum, min or max with a body of its own. With some bytes the body
// holds the aggregate itself, a cycle in the AST, or a nested aggregate.
func (g *astGen) aggregate(depth int) *tarski.Aggregate {
	a := &tarski.Aggregate{Op: genAggs[g.pick(len(genAggs))], Target: g.term(), Result: g.term(), Line: g.pick(3)}
	for range g.pick(4) {
		if depth > 0 && g.pick(4) == 0 {
			a.Body = append(a.Body, g.literal(depth-1))
			continue
		}
		at := g.atom()
		a.Body = append(a.Body, tarski.Literal{Atom: &at})
	}
	if g.pick(5) == 0 {
		a.Body = append(a.Body, tarski.Literal{Aggregate: a})
	}
	return a
}
