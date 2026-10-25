package tarski_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// schemaFor is a schema holding every relation p's bodies read and its heads do not
// define, at the arity it is read at, so each parsed form compiles on its own.
func schemaFor(p tarski.Program) tarski.Schema {
	heads, arities := map[string]bool{}, map[string]int{}
	builtins := tarski.EngineBuiltins()
	var body func([]tarski.Literal)
	body = func(ls []tarski.Literal) {
		for _, l := range ls {
			if l.Aggregate != nil {
				body(l.Aggregate.Body)
			}
			if l.Atom == nil {
				continue
			}
			if _, builtin := builtins[l.Atom.Name]; !builtin {
				arities[l.Atom.Name] = len(l.Atom.Args)
			}
		}
	}
	for _, r := range p.Rules {
		heads[r.Head.Name] = true
		body(r.Body)
	}
	for name := range heads {
		delete(arities, name)
	}
	return tarski.Schema{Arities: arities}
}

// Every form the parser reads compiles once its relations exist, and the plan says it
// aggregates; the rule set holding it verifies.
func TestCompile_AcceptsEveryParsedAggregate(t *testing.T) {
	t.Parallel()
	for _, c := range aggregateRules {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			set, err := tarski.NewRuleSet("agg", 1, c.text)
			if err != nil {
				t.Fatalf("NewRuleSet: %v", err)
			}
			if err := set.Verify(); err != nil {
				t.Errorf("Verify: %v", err)
			}
			plan, err := tarski.Compile(schemaFor(set.Program), set.Program)
			if err != nil {
				t.Fatalf("Compile = %v, want a plan", err)
			}
			if !plan.HasAggregates() {
				t.Error("HasAggregates = false for a plan holding an aggregate")
			}
		})
	}
}

// An aggregate built by hand that the parser would not make is refused as malformed,
// with a reason, before any safety check walks it, and never by a panic.
func TestCompile_RefusesMalformedAggregates(t *testing.T) {
	t.Parallel()
	x, n := tarski.Term{Var: "X"}, tarski.Term{Var: "N"}
	atom := &tarski.Atom{Name: "s", Args: []tarski.Term{x}}
	body := []tarski.Literal{{Atom: atom}}
	agg := func(op string, target, result tarski.Term, body ...tarski.Literal) *tarski.Aggregate {
		return &tarski.Aggregate{Op: op, Target: target, Result: result, Body: body}
	}
	count := agg("count", tarski.Term{}, n, body...)
	cmp := &tarski.Comparison{Op: "=", Left: x, Right: x}
	cases := []struct {
		name string
		lit  tarski.Literal
		want string
	}{
		{"avg", tarski.Literal{Aggregate: agg("avg", x, n, body...)}, "avg is not an aggregate here"},
		{"unknown operation", tarski.Literal{Aggregate: agg("median", x, n, body...)}, `operation "median" is not one of`},
		{"constant result", tarski.Literal{Aggregate: agg("count", tarski.Term{}, atomTerm("a"), body...)}, "result of count must be a named variable"},
		{"anonymous result", tarski.Literal{Aggregate: agg("count", tarski.Term{}, tarski.Term{Var: "_"}, body...)}, "result of count"},
		{"bad result name", tarski.Literal{Aggregate: agg("count", tarski.Term{}, tarski.Term{Var: "n"}, body...)}, `"n" is not a variable name`},
		{"count with a target", tarski.Literal{Aggregate: agg("count", x, n, body...)}, "count takes no target"},
		{"sum without a target", tarski.Literal{Aggregate: agg("sum", tarski.Term{}, n, body...)}, "target of sum must be a named variable"},
		{"empty body", tarski.Literal{Aggregate: agg("count", tarski.Term{}, n)}, "the body of count is empty"},
		{"nested", tarski.Literal{Aggregate: agg("count", tarski.Term{}, n, tarski.Literal{Aggregate: count})}, "an aggregate cannot nest"},
		{"empty inner literal", tarski.Literal{Aggregate: agg("count", tarski.Term{}, n, tarski.Literal{})}, "count, body literal 1: it is neither"},
		{"negated", tarski.Literal{Aggregate: count, Negated: true}, "an aggregate cannot be negated"},
		{"atom and aggregate", tarski.Literal{Atom: atom, Aggregate: count}, "both an atom and an aggregate"},
		{"comparison and aggregate", tarski.Literal{Compare: cmp, Aggregate: count}, "both a comparison and an aggregate"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			head := tarski.Atom{Name: "out", Args: []tarski.Term{atomTerm("x")}}
			prog := tarski.Program{Rules: []tarski.Rule{{Head: head, Body: []tarski.Literal{{Atom: atom}, c.lit}}}}
			_, err := tarski.Compile(tarski.Schema{Arities: map[string]int{"s": 1}}, prog)
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
}

// Clone shares nothing with the original down through an aggregate's body: editing
// the clone's aggregate leaves the original as parsed.
func TestProgramClone_DeepCopiesAggregates(t *testing.T) {
	t.Parallel()
	text := `q(M, N) :- month(M), N = count : { r(M, X, V), not bad(X), V > 3 }.`
	orig := parse(t, text)
	want := parse(t, text)
	clone := orig.Clone()
	if !reflect.DeepEqual(clone, orig) {
		t.Fatalf("Clone = %+v, want a value equal to the original", clone)
	}
	agg := clone.Rules[0].Body[1].Aggregate
	agg.Op, agg.Result.Var = "sum", "Z"
	agg.Body[0].Atom.Name, agg.Body[0].Atom.Args[0].Var = "changed", "Changed"
	agg.Body[1].Atom.Name = "changed"
	agg.Body[2].Compare.Op = "<"
	agg.Body = append(agg.Body, tarski.Literal{Atom: &tarski.Atom{Name: "extra"}})
	if !reflect.DeepEqual(orig, want) {
		t.Errorf("editing the clone moved the original: %+v", orig.Rules[0].Body[1].Aggregate)
	}
}

// HasAggregates is false for every plan of the kinds a v0.1 program makes (recursion,
// negation, comparisons, builtins, Anam's rule set); TestCompile_AcceptsEveryParsedAggregate
// holds it true for every compiled aggregate form.
func TestPlan_HasAggregatesIsFalseForExistingPrograms(t *testing.T) {
	t.Parallel()
	anam, err := os.ReadFile("testdata/anam.dl")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		schema tarski.Schema
		text   string
	}{
		{"empty", tarski.Schema{}, ``},
		{"recursion", tarski.Schema{Arities: map[string]int{"edge": 2}}, `reach(X, Y) :- edge(X, Y). reach(X, Z) :- reach(X, Y), edge(Y, Z).`},
		{"negation and comparison", tarski.Schema{Arities: map[string]int{"node": 1, "edge": 2}},
			`loop(X) :- edge(X, X). safe(X) :- node(X), not loop(X), X != "z".`},
		{"builtin", tarski.Schema{Arities: map[string]int{"node": 1}}, `n(X) :- node(X), prefix("a", X).`},
		{"anam", anamSchema, string(anam) + anamQuery},
	}
	for _, c := range cases {
		plan, err := tarski.Compile(c.schema, parse(t, c.text))
		if err != nil {
			t.Fatalf("%s: Compile: %v", c.name, err)
		}
		if plan.HasAggregates() {
			t.Errorf("%s: HasAggregates = true, want false", c.name)
		}
	}
	if (tarski.Plan{}).HasAggregates() {
		t.Error("the zero Plan has aggregates")
	}
}

// A rule set's hash is sha256 over its text and nothing else, so adding the aggregate
// syntax moves no existing hash; a set holding an aggregate hashes the same way.
func TestRuleSet_HashIsStillTheTextHash(t *testing.T) {
	t.Parallel()
	texts := []string{
		goldenRules,
		`reach(X, Y) :- edge(X, Y). reach(X, Z) :- reach(X, Y), edge(Y, Z).`,
		`min(X) :- count(X), not max(X).`,
		aggregateRules[0].text,
	}
	for _, text := range texts {
		set, err := tarski.NewRuleSet("h", 1, text)
		if err != nil {
			t.Fatalf("NewRuleSet(%q): %v", text, err)
		}
		sum := sha256.Sum256([]byte(text))
		if want := "sha256:" + hex.EncodeToString(sum[:]); set.Hash != want {
			t.Errorf("Hash(%q) = %s, want %s", text, set.Hash, want)
		}
	}
}
