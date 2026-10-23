package datalog

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The hooks are proved with a fake, so they do not depend on what the real built-ins
// do. This test registers shorter(A, B), true when A is shorter than B, to prove every
// hook: where it is refused, how it is ordered, and how it is evaluated. It is not parallel and removes the fake when it
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
			t.Errorf("%s holds %d tuples, want %d: %v", rel, ans.Len(), n, ans.Tuples())
		}
	}
}

func num(t *testing.T, lexeme string) Const {
	t.Helper()
	c, err := Number(lexeme)
	if err != nil {
		t.Fatalf("Number(%q): %v", lexeme, err)
	}
	return c
}

// okRows pairs every (pattern, subject) row with the built-in under test and reports
// which rows the rule derived, as "pattern|subject" in text form.
func okRows(t *testing.T, body string, rows ...Tuple) map[string]bool {
	t.Helper()
	schema := Schema{Arities: map[string]int{"p": 2}}
	facts := make([]Fact, len(rows))
	for i, r := range rows {
		facts[i] = Fact{"p", r}
	}
	base, err := NewBase(schema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	plan, err := Compile(schema, mustParse(t, "ok(P, S) :- p(P, S), "+body+"."))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := Evaluate(context.Background(), plan, base, Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	ans, err := res.Relation("ok")
	if err != nil {
		t.Fatalf("Relation: %v", err)
	}
	got := map[string]bool{}
	for _, tup := range ans.Tuples() {
		got[tup[0].Text()+"|"+tup[1].Text()] = true
	}
	return got
}

func TestBuiltins_StringTests(t *testing.T) {
	t.Parallel()
	rows := []Tuple{
		str("time", "timeline/a"),
		str("line", "timeline/a"),
		str("/a", "timeline/a"),
		str("zzz", "timeline/a"),
		str("timeline/a", "time"), // pattern longer than subject
		str("", "timeline/a"),
		str("", ""),
		str("Time", "timeline/a"), // case matters
	}
	cases := []struct {
		name, body string
		want       []string
	}{
		{"prefix", "prefix(P, S)", []string{"time|timeline/a", "|timeline/a", "|"}},
		{"suffix", "suffix(P, S)", []string{"/a|timeline/a", "|timeline/a", "|"}},
		{"contains", "contains(P, S)", []string{"time|timeline/a", "line|timeline/a", "/a|timeline/a", "|timeline/a", "|"}},
		{"not prefix", "not prefix(P, S)", []string{"line|timeline/a", "/a|timeline/a", "zzz|timeline/a", "timeline/a|time", "Time|timeline/a"}},
		{"not contains", "not contains(P, S)", []string{"zzz|timeline/a", "timeline/a|time", "Time|timeline/a"}},
		{"constant pattern", `prefix("time", S)`, []string{"time|timeline/a", "line|timeline/a", "/a|timeline/a", "zzz|timeline/a", "|timeline/a", "Time|timeline/a", "timeline/a|time"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := okRows(t, c.body, rows...)
			want := map[string]bool{}
			for _, w := range c.want {
				want[w] = true
			}
			if len(got) != len(want) {
				t.Errorf("derived %v, want %v", got, want)
			}
			for w := range want {
				if !got[w] {
					t.Errorf("missing %q; derived %v", w, got)
				}
			}
		})
	}
}

func TestBuiltins_NonStringIsFalse(t *testing.T) {
	t.Parallel()
	rows := []Tuple{
		{String("a"), num(t, "1")},
		{num(t, "1"), String("a")},
		{num(t, "1"), num(t, "1")},
		{Bool(true), String("true")},
		{String("t"), Bool(true)},
	}
	for _, name := range []string{"prefix", "suffix", "contains"} {
		for _, neg := range []string{"", "not "} {
			got := okRows(t, neg+name+"(P, S)", rows...)
			// A non-string argument makes the built-in false, so under not every
			// such row holds: negation flips the answer, it does not refuse it.
			if neg == "" && len(got) != 0 {
				t.Errorf("%s over non-strings derived %v, want none", name, got)
			}
			if neg != "" && len(got) != len(rows) {
				t.Errorf("not %s over non-strings derived %v, want all %d rows", name, got, len(rows))
			}
		}
	}
}

func TestBuiltins_Refused(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"p": 2, "q": 1}}
	cases := []struct{ name, text, want string }{
		{"unbound pattern", `a(S) :- q(S), prefix(P, S).`, "P in prefix must be bound by a positive body atom"},
		{"unbound subject", `a(P) :- q(P), suffix(P, S).`, "S in suffix must be bound by a positive body atom"},
		{"unbound under not", `a(P) :- q(P), not contains(P, S).`, "S in contains must be bound"},
		{"anonymous", `a(S) :- q(S), contains("x", _).`, "_ in contains tests nothing"},
		{"head", `prefix(P, S) :- p(P, S).`, "prefix is a built-in and cannot be the head"},
		{"head suffix", `suffix(P, S) :- p(P, S).`, "suffix is a built-in and cannot be the head"},
		{"declaration", `contains/2.`, "contains is a built-in and cannot be declared"},
		{"too few arguments", `a(S) :- q(S), prefix(S).`, "prefix has 1 arguments"},
		{"too many arguments", `a(S) :- q(S), suffix(S, S, S).`, "suffix has 3 arguments"},
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
	for _, name := range []string{"prefix", "suffix", "contains"} {
		bad := Schema{Arities: map[string]int{name: 2}}
		if _, err := Compile(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s in a Schema: Compile = %v, want ErrInvalid", name, err)
		}
		if _, err := NewBase(bad); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s in a Schema: NewBase = %v, want ErrInvalid", name, err)
		}
	}
}

// A built-in guards the recursive step; the fixpoint is still reached.
func TestBuiltins_InRecursiveRule(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"start": 1, "edge": 2}}
	base, err := NewBase(schema,
		Fact{"start", str("a1")},
		Fact{"edge", str("a1", "a2")}, Fact{"edge", str("a2", "b3")},
		Fact{"edge", str("b3", "a4")}, Fact{"edge", str("a2", "a5")}, Fact{"edge", str("a5", "a1")})
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	plan, err := Compile(schema, mustParse(t, `
		reach(X) :- start(X).
		reach(Y) :- reach(X), edge(X, Y), prefix("a", Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := Evaluate(context.Background(), plan, base, Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	ans, err := res.Relation("reach")
	if err != nil {
		t.Fatalf("Relation: %v", err)
	}
	got := map[string]bool{}
	for _, tup := range ans.Tuples() {
		got[tup[0].Text()] = true
	}
	if len(got) != 3 || !got["a1"] || !got["a2"] || !got["a5"] {
		t.Errorf("reach = %v, want a1, a2, a5 (b3 is blocked, so a4 is unreachable)", got)
	}
}

// Negation mirrors comparisons, where mismatched kinds are unequal and != holds: a
// number is prefixed by nothing, so not prefix is true for it.
func TestBuiltins_NegatedNonStringHolds(t *testing.T) {
	t.Parallel()
	got := okRows(t, "not prefix(P, S)", Tuple{String("1"), num(t, "1")}, str("1", "12"))
	if len(got) != 1 || !got["1|1"] {
		t.Errorf("not prefix derived %v, want only the number-subject row 1|1", got)
	}
}
