package datalog

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The built-in map is empty at rest. This test registers a fake, shorter(A, B), true
// when A is shorter than B, to prove every hook: where it is refused, how it is
// ordered, and how it is evaluated. It is not parallel and removes the fake when it
// ends: Go runs the parallel tests only after every sequential one has finished, so
// none reads the map while it is changed.
func TestBuiltins_Hooks(t *testing.T) {
	builtins["shorter"] = builtin{arity: 2, eval: func(args []Const) bool {
		return len(args[0].Text()) < len(args[1].Text())
	}}
	defer delete(builtins, "shorter")

	schema := Schema{Arities: map[string]int{"word": 1, "pair": 2}}
	refused := []struct{ name, text, want string }{
		{"in a head", `shorter(X, Y) :- pair(X, Y).`, "shorter is a built-in and cannot be the head of a rule"},
		{"declared", `shorter/2.`, "shorter is a built-in and cannot be declared"},
		{"wrong arity", `a(X) :- word(X), shorter(X).`, "shorter has 1 arguments, but shorter is shorter/2"},
		{"unbound argument", `a(X) :- word(X), shorter(X, Y).`, "Y in shorter must be bound by a positive body atom"},
		{"bound only by a built-in", `a(X) :- word(X), shorter(X, Y), shorter(Y, X).`, "Y in shorter must be bound"},
		{"bound only by a negation", `a(X) :- word(X), not pair(X, Y), shorter(X, Y).`, "Y in not pair is not bound"},
		{"anonymous", `a(X) :- word(X), shorter(X, _).`, "_ in shorter tests nothing"},
	}
	for _, c := range refused {
		_, err := Compile(schema, mustParse(t, c.text))
		if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: Compile = %v, want ErrInvalid containing %q", c.name, err, c.want)
		}
	}
	if _, err := Compile(Schema{Arities: map[string]int{"shorter": 2}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a built-in in the schema: Compile = %v, want ErrInvalid", err)
	}
	if _, err := NewBase(Schema{Arities: map[string]int{"shorter": 2}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a built-in in the schema: NewBase = %v, want ErrInvalid", err)
	}

	// The built-in is written before the atoms that bind its arguments, and still
	// runs after them; a negated one is the opposite guard; a constant needs no atom.
	base, err := NewBase(schema,
		Fact{"pair", str("ab", "abcd")}, Fact{"pair", str("abc", "a")}, Fact{"pair", str("", "x")}, Fact{"word", str("aa")})
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	prog := mustParse(t, `
		up(X, Y) :- shorter(X, Y), pair(X, Y).
		down(X, Y) :- pair(X, Y), not shorter(X, Y).
		fixed(X) :- word(X), shorter("a", "bb").
		none(X) :- word(X), shorter("bb", "a").`)
	plan, err := Compile(schema, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := Evaluate(context.Background(), plan, base, Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	want := map[string]int{"up": 2, "down": 1, "fixed": 1, "none": 0}
	for rel, n := range want {
		ans, err := res.Relation(rel)
		if err != nil {
			t.Fatalf("Relation(%s): %v", rel, err)
		}
		if ans.Len() != n {
			t.Errorf("%s holds %d tuples, want %d: %v", rel, ans.Len(), n, ans.Tuples)
		}
	}
}
