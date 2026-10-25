package tarski_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/tarski"
)

// A callback can retain the caller's schema maps. Every compilation phase must
// see the same input snapshot, including validation and final base requirements.
func TestCompile_SnapshotsSchemaBeforeCallbacks(t *testing.T) {
	t.Run("arities", func(t *testing.T) {
		schema := tarski.Schema{Arities: map[string]int{"node": 1}}
		schema.Checks = map[string]func(tarski.Atom) error{"node": func(tarski.Atom) error {
			schema.Arities["extra"] = 1
			schema.Arities["node"] = 2
			return nil
		}}
		plan, err := tarski.Compile(schema, parse(t, `out(X) :- node(X).`))
		if err != nil {
			t.Fatal(err)
		}
		base, err := tarski.NewBase(tarski.Schema{Arities: map[string]int{"node": 1}}, fact("node", "present"))
		if err != nil {
			t.Fatal(err)
		}
		result, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
		if err != nil {
			t.Fatalf("schema mutation changed the plan's base requirements: %v", err)
		}
		answer, err := result.Relation("out")
		if err != nil || !same(rows(answer), []string{"present"}) {
			t.Fatalf("answer = %v, %v", rows(answer), err)
		}
	})
	t.Run("checks", func(t *testing.T) {
		refused := errors.New("other relation is forbidden")
		schema := tarski.Schema{Arities: map[string]int{"node": 1, "other": 1}}
		schema.Checks = map[string]func(tarski.Atom) error{
			"node":  func(tarski.Atom) error { delete(schema.Checks, "other"); return nil },
			"other": func(tarski.Atom) error { return refused },
		}
		_, err := tarski.Compile(schema, parse(t, `out(X) :- node(X), other(X).`))
		if !errors.Is(err, tarski.ErrInvalid) || !errors.Is(err, refused) {
			t.Fatalf("a callback removed a later validator: got %v", err)
		}
	})
}

// Keep the Compile callback's argument, without a subsequent NewBase callback
// replacing it. Both immediate and retained edits must leave the plan unchanged.
func TestCompile_CheckArgumentsStayDetached(t *testing.T) {
	var retained []tarski.Term
	schema := tarski.Schema{Arities: map[string]int{"node": 1}, Checks: map[string]func(tarski.Atom) error{
		"node": func(a tarski.Atom) error {
			retained = a.Args
			a.Args[0] = tarski.Term{Const: tarski.String("not present")}
			return nil
		},
	}}
	plan, err := tarski.Compile(schema, parse(t, `out(X) :- node(X).`))
	if err != nil {
		t.Fatal(err)
	}
	retained[0] = tarski.Term{Const: tarski.String("also absent")}
	base, err := tarski.NewBase(tarski.Schema{Arities: map[string]int{"node": 1}}, fact("node", "present"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
	if err != nil {
		t.Fatal(err)
	}
	answer, err := result.Relation("out")
	if err != nil || !same(rows(answer), []string{"present"}) {
		t.Fatalf("callback mutated plan: %v, %v", rows(answer), err)
	}
}
