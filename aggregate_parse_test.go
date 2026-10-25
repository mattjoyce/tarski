package tarski_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// aggregateRules are rules holding aggregates in every form the parser reads. Each is
// one rule, written as its own source text, so its Rule.Text is the whole string.
var aggregateRules = []struct {
	name, text string
	ops        []string // the operation of each aggregate literal, in body order
	targets    []string // each aggregate's target variable, "" for count
	inner      []int    // each aggregate's body length
}{
	{"count", `n(M, N) :- month(M), N = count : { reading(M, _) }.`, []string{"count"}, []string{""}, []int{1}},
	{"count with no outer body", `n(N) :- N = count : { reading(_) }.`, []string{"count"}, []string{""}, []int{1}},
	{"sum", `s(M, S) :- month(M), S = sum V : { purchase(E, At, V), prefix(M, At) }.`, []string{"sum"}, []string{"V"}, []int{2}},
	{"min", `first(M, T) :- month(M), T = min X : { reading(X), prefix(M, X) }.`, []string{"min"}, []string{"X"}, []int{2}},
	{"max", `last(M, T) :- month(M), T = max X : { reading(X), prefix(M, X) }.`, []string{"max"}, []string{"X"}, []int{2}},
	{"negation and comparisons inside", `q(M, N) :- month(M), N = count : { r(M, X, V), not bad(X), V > 3, X != "z", V <= 1e3 }.`,
		[]string{"count"}, []string{""}, []int{5}},
	{"several aggregates", `st(M, N, S, L) :- month(M), N = count : { r(M, _) }, S = sum V : { r(M, V) }, L = max V : { r(M, V) }.`,
		[]string{"count", "sum", "max"}, []string{"", "V", "V"}, []int{1, 1, 1}},
	{"aggregate then a guard on its result", `busy(M) :- month(M), N = count : { r(M, _) }, N > 3.`, []string{"count"}, []string{""}, []int{1}},
	{"compact", `n(N):-N=count:{r(X),s(X)}.`, []string{"count"}, []string{""}, []int{2}},
	{"across lines with a comment", "n(M, N) :- month(M),\n  N = count : {   % per month\n    r(M, _)\n  }.", []string{"count"}, []string{""}, []int{1}},
	{"operation words as relation names", `n(N) :- count(N), N = count : { count(_), sum(_), min(_, _), max(_) }.`,
		[]string{"count"}, []string{""}, []int{4}},
}

// Every aggregate form parses to the AST the design names, and its rule's Text is the
// source as written, braces and comments included.
func TestParse_Aggregates(t *testing.T) {
	t.Parallel()
	for _, c := range aggregateRules {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p := parse(t, c.text)
			if len(p.Rules) != 1 {
				t.Fatalf("%d rules, want 1", len(p.Rules))
			}
			r := p.Rules[0]
			if r.Text != c.text {
				t.Errorf("Text = %q, want the source %q", r.Text, c.text)
			}
			var aggs []*tarski.Aggregate
			for _, l := range r.Body {
				if l.Aggregate != nil {
					if l.Atom != nil || l.Compare != nil || l.Negated {
						t.Errorf("an aggregate literal also holds %+v", l)
					}
					aggs = append(aggs, l.Aggregate)
				}
			}
			if len(aggs) != len(c.ops) {
				t.Fatalf("%d aggregates, want %d", len(aggs), len(c.ops))
			}
			for i, a := range aggs {
				if a.Op != c.ops[i] || a.Target.Var != c.targets[i] || len(a.Body) != c.inner[i] {
					t.Errorf("aggregate %d = %s %q over %d literals, want %s %q over %d",
						i, a.Op, a.Target.Var, len(a.Body), c.ops[i], c.targets[i], c.inner[i])
				}
				if !a.Result.IsVar() || a.Result.Var == "_" || a.Line < 1 {
					t.Errorf("aggregate %d result %+v line %d", i, a.Result, a.Line)
				}
			}
		})
	}
}

// Parsing a rule's Text gives back the same rule, and its Text again: the text is the
// rule's one representation, as RuleSet.Verify relies on.
func TestParse_AggregateTextRoundTrip(t *testing.T) {
	t.Parallel()
	for _, c := range aggregateRules {
		first := parse(t, c.text).Rules[0]
		again := parse(t, first.Text).Rules[0]
		if !reflect.DeepEqual(first, again) {
			t.Errorf("%s: Parse(Text) = %+v, want %+v", c.name, again, first)
		}
		if again.Text != first.Text {
			t.Errorf("%s: Text moved: %q, then %q", c.name, first.Text, again.Text)
		}
	}
}

// A malformed aggregate is ErrSyntax naming what to write instead.
func TestParse_AggregateErrors(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, text, want string }{
		{"missing braces", `n(N) :- N = count : r(X).`, `expected "{" to open the body of count, got "r"`},
		{"missing closing brace", `n(N) :- N = count : { r(X) .`, `expected "," or "}" in the body of count, got "."`},
		{"unterminated body", `n(N) :- N = count : { r(X)`, `got "end of program"`},
		{"missing colon", `n(N) :- N = count { r(X) }.`, `expected ":" after count, got "{"`},
		{"missing colon after target", `s(S) :- S = sum X { r(X) }.`, `expected ":" after sum X`},
		{"missing variable after sum", `s(S) :- S = sum : { r(X) }.`, `expected the variable sum is taken over, as in S = sum X`},
		{"constant after max", `s(S) :- S = max 3 : { r(X) }.`, `expected the variable max is taken over`},
		{"anonymous target", `s(S) :- S = min _ : { r(X) }.`, `expected the variable min is taken over`},
		{"count with a variable", `n(N) :- N = count X : { r(X) }.`, `count takes no variable; write N = count : { ... }`},
		{"aggregate in a head", `n(count : { r(X) }) :- r(X).`, `an aggregate cannot be an argument`},
		{"sum in a head", `n(sum X : { r(X) }).`, `an aggregate cannot be an argument`},
		{"aggregate in a body atom", `n(X) :- r(X, count : { s(_) }).`, `an aggregate cannot be an argument`},
		{"empty body", `n(N) :- N = count : { }.`, `the body of count is empty`},
		{"nested", `n(N) :- N = count : { r(X), M = count : { s(X) } }.`, `an aggregate cannot nest`},
		{"negated", `n(N) :- r(N), not count : { s(_) }.`, `an aggregate cannot be negated`},
		{"result after", `n(N) :- r(N), count : { s(_) } = N.`, `an aggregate's result comes first: write N = count`},
		{"constant result", `n(1) :- 1 = count : { r(_) }.`, `the result of count must be a named variable`},
		{"anonymous result", `n(1) :- _ = sum X : { r(X) }.`, `the result of sum must be a named variable`},
		{"result by another operator", `n(N) :- r(N), N < count : { r(_) }.`, `an aggregate binds its result with =, not <`},
		{"avg", `a(A) :- A = avg X : { r(X) }.`, `avg is not an aggregate here; derive count and sum and divide in the caller`},
		{"mean", `a(A) :- A = mean X : { r(X) }.`, `mean is not an aggregate here`},
		{"stray colon", `n(X) :- r(X) : s(X).`, `expected ".", got ":"`},
		{"stray brace", `n(X) :- { r(X) }.`, `expected a variable or a constant, got "{"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := tarski.Parse(c.text)
			if !errors.Is(err, tarski.ErrSyntax) || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("Parse(%q) = %v, want ErrSyntax containing %q", c.text, err, c.want)
			}
			if !strings.Contains(err.Error(), "line 1:") {
				t.Errorf("Parse(%q) = %v, want it to name the line", c.text, err)
			}
		})
	}
}

// count, sum, min, max, avg and mean are words, not keywords: a v0.1 program using
// them as relation names parses, compiles and answers as it did.
func TestParse_OperationWordsStayRelationNames(t *testing.T) {
	t.Parallel()
	schema := tarski.Schema{Arities: map[string]int{"count": 1, "sum": 2}}
	text := `min(X) :- count(X), not max(X). max(X) :- sum(X, Y), Y > 1. avg(X) :- min(X). mean(X) :- avg(X), X = "a".`
	plan, err := tarski.Compile(schema, parse(t, text))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if plan.HasAggregates() {
		t.Error("HasAggregates = true for a program with no aggregate")
	}
	base := newBaseUnder(t, schema, fact("count", "a"), fact("count", "b"), fact("sum", "b", 2))
	res, err := tarski.Evaluate(t.Context(), plan, base, tarski.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	for rel, want := range map[string]int{"min": 1, "max": 1, "avg": 1, "mean": 1} {
		ans, err := res.Relation(rel)
		if err != nil || ans.Len() != want {
			t.Errorf("%s: %v tuples, err %v; want %d", rel, ans.Len(), err, want)
		}
	}
}
