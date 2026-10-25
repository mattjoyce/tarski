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

const inertMessage = "aggregates are not evaluated yet (tarski v0.2 in progress)"

// aggregateSchema holds every base relation aggregateRules read.
var aggregateSchema = tarski.Schema{Arities: map[string]int{
	"month": 1, "reading": 1, "purchase": 3, "r": 3, "bad": 1, "s": 1,
	"count": 1, "sum": 1, "min": 2, "max": 1,
}}

// The syntax lands inert: Compile refuses every aggregate, naming the rule set, the
// line and the literal, so no plan holds one before evaluation exists.
func TestCompile_RefusesEveryAggregate(t *testing.T) {
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
			_, err = tarski.Compile(aggregateSchema, set.Program)
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), inertMessage) {
				t.Fatalf("Compile = %v, want ErrInvalid containing %q", err, inertMessage)
			}
			if !strings.Contains(err.Error(), "agg/1, rule 1 (line 1") || !strings.Contains(err.Error(), " = "+c.ops[0]) {
				t.Errorf("Compile = %v, want it to name the set, the rule and the literal", err)
			}
		})
	}
}

// An aggregate built by hand that the parser would not make is refused as malformed,
// with a reason, before the inert refusal and never by a panic.
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
			if strings.Contains(err.Error(), inertMessage) {
				t.Errorf("Compile = %v, want the shape refusal, not the inert one", err)
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

// Before evaluation exists Compile refuses every aggregate, so HasAggregates is false
// for every plan Compile makes; the programs below are the existing kinds (recursion,
// negation, comparisons, builtins, Anam's rule set). Slice 3 of card 6 makes Compile
// accept aggregates, and its tests compile an aggregate rule and assert true here; until
// then aggregate_internal_test.go reaches the true branch with a hand-assembled plan.
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
