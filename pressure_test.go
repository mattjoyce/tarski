package tarski_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Regressions from an external pressure review of v0.2.0 (board agent tarski-pressure,
// 2026-10-26). Each failed before the fix in the same commit.

// An absurd arity with a few short facts must be refused, not make the base ask for an
// allocation sized by arity times count before the facts are checked.
func TestPressure_AbsurdArityIsRefusedNotAllocated(t *testing.T) {
	arity := int(^uint(0)>>1)/2 + 1
	_, err := tarski.NewBase(tarski.Schema{Arities: map[string]int{"r": arity}},
		tarski.Fact{Relation: "r", Tuple: tarski.Tuple{tarski.Int(1)}},
		tarski.Fact{Relation: "r", Tuple: tarski.Tuple{tarski.Int(2)}})
	if !errors.Is(err, tarski.ErrInvalid) {
		t.Fatalf("NewBase with a short fact under a huge arity = %v, want ErrInvalid", err)
	}
}

// A hand-built aggregate whose body holds itself must be refused by Compile, not
// overflow the stack while Compile deep-copies the program.
func TestPressure_CyclicAggregateIsRefused(t *testing.T) {
	a := &tarski.Aggregate{Op: "count", Result: tarski.Term{Var: "N"}}
	a.Body = []tarski.Literal{{Aggregate: a}}
	prog := tarski.Program{Rules: []tarski.Rule{{
		Head: tarski.Atom{Name: "r", Args: []tarski.Term{{Var: "N"}}},
		Body: []tarski.Literal{{Aggregate: a}},
	}}}
	if _, err := tarski.Compile(tarski.Schema{}, prog); !errors.Is(err, tarski.ErrInvalid) {
		t.Fatalf("Compile of a cyclic aggregate = %v, want ErrInvalid", err)
	}
	if c := prog.Clone(); c.Rules[0].Body[0].Aggregate == a {
		t.Error("Clone shares the top-level aggregate; want a copy")
	}
}

// Rules with no positive atom call their guards without a join step; each application
// is now a step, so MaxSteps bounds how often a consumer builtin is called.
func TestPressure_GuardOnlyRulesAreBounded(t *testing.T) {
	calls := 0
	schema := tarski.Schema{Builtins: map[string]tarski.Builtin{"guard": {
		Args:  []tarski.BuiltinArg{{Name: "X"}},
		Holds: func([]tarski.Const) bool { calls++; return true },
	}}}
	prog, err := tarski.Parse(strings.Repeat("r(\"x\") :- guard(\"x\").\n", 1000))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tarski.Compile(schema, prog)
	if err != nil {
		t.Fatal(err)
	}
	base, err := tarski.NewBase(schema)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{MaxSteps: 10})
	var tl *tarski.TooLarge
	if !errors.As(err, &tl) || tl.Bound != tarski.BoundSteps {
		t.Fatalf("1000 guard-only rules under MaxSteps 10 = %v (calls %d), want *TooLarge on steps", err, calls)
	}
	if calls > 10 {
		t.Errorf("builtin called %d times under MaxSteps 10", calls)
	}
}
