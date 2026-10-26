package tarski

import (
	"errors"
	"strings"
	"testing"
)

// within(A, B, Days) is a consumer builtin with three named arguments, to pin how a
// refusal names the argument and shows the signature.
var within = Builtin{
	Args: []BuiltinArg{{Name: "A"}, {Name: "B"}, {Name: "Days"}},
	Check: func(consts []*Const) error {
		if d := consts[2]; d != nil && d.Text() == "0" {
			return errors.New("Days must not be zero")
		}
		return nil
	},
	Holds: func([]Const) bool { return true },
}

// TestBuiltins_RefusalsNameTheArgument pins the exact text of the three builtin
// refusals, for a consumer builtin and an engine builtin.
func TestBuiltins_RefusalsNameTheArgument(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"p": 3, "q": 1}, Builtins: map[string]Builtin{"within": within}}
	cases := []struct{ name, text, want string }{
		{"consumer safety", `a(X) :- p(X, Y, 1), within(X, Other, 1).`,
			"line 1: Other, argument B of within(A, B, Days), must be bound by a positive body atom; a built-in binds nothing"},
		{"consumer safety names the first of repeats", `a(X) :- q(X), within(X, O, O).`,
			"O, argument B of within(A, B, Days), must be bound"},
		{"consumer arity", `a(X) :- q(X), within(X, X).`,
			"within has 2 arguments, but within takes 3: within(A, B, Days)"},
		{"consumer check", `a(X) :- q(X), within(X, X, 0).`,
			"within(A, B, Days): Days must not be zero"},
		{"engine safety", `a(S) :- q(S), prefix(P, S).`,
			"line 1: P, argument Pattern of prefix(Pattern, Subject), must be bound by a positive body atom; a built-in binds nothing"},
		{"engine safety names the first of repeats", `a(S) :- q(S), contains(P, P).`,
			"P, argument Pattern of contains(Pattern, Subject), must be bound"},
		{"engine arity", `a(S) :- q(S), suffix(S).`,
			"suffix has 1 arguments, but suffix takes 2: suffix(Pattern, Subject)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := Compile(schema, mustParse(t, c.text))
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
}
