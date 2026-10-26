package tarski_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Every refusal of docs/design/aggregates.md §3, each by a small program, with the
// sentinel and the words that say what to change.
func TestCompile_RefusesUnsafeAggregates(t *testing.T) {
	t.Parallel()
	schema := tarski.Schema{
		Arities: aggSchema.Arities,
		Checks: map[string]func(tarski.Atom) error{
			"word": func(a tarski.Atom) error {
				if !a.Args[0].IsVar() && strings.HasPrefix(a.Args[0].Const.Text(), "!") {
					return fmt.Errorf("a word does not start with !")
				}
				return nil
			},
		},
	}
	cases := []struct {
		name, text string
		sentinel   error
		want       string
	}{
		{"result inside the braces", `n(N) :- N = count : { item(N, _) }.`, tarski.ErrInvalid,
			"N is the result of count and cannot appear inside its braces"},
		{"result as the target", `n(N) :- N = sum N : { item(_, N) }.`, tarski.ErrInvalid, "N is the result of sum"},
		{"target bound by no positive atom inside", `s(G, S) :- grp(G), S = sum G : { item(_, V) }.`, tarski.ErrInvalid,
			"G in sum must be bound by a positive atom inside the braces"},
		{"target bound only by a negation inside", `s(S) :- S = max V : { item(_, W), not bad(V) }.`, tarski.ErrInvalid,
			"V in max must be bound by a positive atom inside the braces"},
		{"group variable bound only by a comparison outside", `n(N) :- grp(H), H != G, N = count : { item(G, _) }.`, tarski.ErrInvalid,
			"G is used inside the braces of count and outside them, so it groups the aggregate; bind it by a positive atom outside"},
		{"group variable bound only by a negation outside", `n(N) :- grp(H), not item(H, G), N = count : { item(G, _) }.`, tarski.ErrInvalid,
			"G is used inside the braces of count and outside them"},
		{"group variable shared with another aggregate's result", `n(N) :- M = count : { grp(_) }, N = count : { item(_, M) }.`,
			tarski.ErrInvalid, "M is used inside the braces of count and outside them"},
		{"head variable bound only inside", `n(V, N) :- N = count : { item(_, V) }.`, tarski.ErrInvalid,
			"V in the head of n occurs only inside an aggregate"},
		{"head variable bound by a comparison and inside", `n(G, N) :- G = "a", N = count : { item(G, _) }.`, tarski.ErrInvalid,
			"G in the head of n occurs only inside an aggregate"},
		{"unbound variable in a comparison inside", `n(N) :- N = count : { item(_, V), V > W }.`, tarski.ErrInvalid,
			"W in a comparison is not bound by a positive body atom (inside the braces of count"},
		{"unbound variable in a negation inside", `n(N) :- N = count : { item(_, V), not tag(V, W) }.`, tarski.ErrInvalid,
			"W in not tag is not bound"},
		{"unbound variable in a builtin inside", `n(N) :- N = count : { word(V), prefix(W, V) }.`, tarski.ErrInvalid,
			"W, argument Pattern of prefix(Pattern, Subject), must be bound by a positive body atom"},
		{"a builtin binds nothing inside", `n(N) :- N = min V : { prefix("a", V) }.`, tarski.ErrInvalid,
			"V in min must be bound by a positive atom inside the braces"},
		{"wrong arity inside", `n(N) :- N = count : { item(_) }.`, tarski.ErrInvalid, "item has 1 arguments, but item is item/2"},
		{"unknown relation inside", `n(N) :- N = count : { nope(_) }.`, tarski.ErrInvalid, "unknown relation nope/1"},
		{"schema check inside", `n(N) :- N = count : { word("!x") }.`, tarski.ErrInvalid, "a word does not start with !"},
		{"an aggregate over itself", `n(N) :- N = count : { n(_) }.`, tarski.ErrInvalid,
			"n reads itself inside an aggregate, so the program cannot be stratified"},
		{"a cycle through an aggregate", `a(X) :- item(X, _). a(N) :- b(N). b(N) :- N = count : { a(_) }.`, tarski.ErrInvalid,
			"b reads a inside an aggregate and a depends on b, so the program cannot be stratified; the aggregated relation must be complete"},
		{"a cycle through a negation inside the braces", `a(X) :- item(X, _). a(X) :- b(X). b(N) :- N = count : { item(_, V), not a(V) }.`,
			tarski.ErrInvalid, "b reads a inside an aggregate"},
		{"avg", `n(N) :- N = avg V : { item(_, V) }.`, tarski.ErrSyntax, "avg is not an aggregate here; derive count and sum"},
		{"mean", `n(N) :- N = mean V : { item(_, V) }.`, tarski.ErrSyntax, "mean is not an aggregate here"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			prog, err := tarski.Parse(c.text)
			if err == nil {
				_, err = tarski.Compile(schema, prog)
			}
			if !errors.Is(err, c.sentinel) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want %v containing %q", err, c.sentinel, c.want)
			}
		})
	}
}

// The programs a refusal points to compile once the change it names is made: the
// aggregated relation moved one stratum down, the result moved outside, the group
// variable bound outside.
func TestCompile_AcceptsTheFixedPrograms(t *testing.T) {
	t.Parallel()
	texts := []string{
		`a(X) :- item(X, _). b(N) :- N = count : { a(_) }.`,
		`a(X) :- item(X, _). c(N) :- b(N). b(N) :- N = count : { a(_) }.`,
		`n(N) :- N = count : { item(M, _) }.`,
		`n(G, N) :- grp(G), N = count : { item(G, _) }.`,
		`n(G, N) :- grp(G), item(G, N), N = count : { item(G, _) }.`,
		`n(N) :- M = count : { grp(_) }, N = count : { item(_, V) }, N > M.`,
	}
	for _, text := range texts {
		if _, err := tarski.Compile(aggSchema, parse(t, text)); err != nil {
			t.Errorf("Compile(%s) = %v, want a plan", text, err)
		}
	}
}
