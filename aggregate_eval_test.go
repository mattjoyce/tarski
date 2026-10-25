package tarski_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/tarski"
)

// aggSchema is the base the aggregate evaluation tests read: groups, items of a group
// with a value, tags on a value, words, a mixed-kind relation and a small graph.
var aggSchema = tarski.Schema{Arities: map[string]int{
	"grp": 1, "item": 2, "tag": 2, "bad": 1, "word": 1, "mixed": 2, "edge": 2,
}}

// aggFacts: group "a" holds 1, 2, 3; "b" holds 10, -2.5 and 2, so the value 2 is in
// both; "e" holds nothing. tag is empty, so an aggregate over it has an empty group.
func aggFacts() []tarski.Fact {
	return []tarski.Fact{
		fact("grp", "a"), fact("grp", "b"), fact("grp", "e"),
		fact("item", "a", 1), fact("item", "a", 2), fact("item", "a", 3),
		fact("item", "b", 10), fact("item", "b", -2.5), fact("item", "b", 2),
		fact("bad", 3),
		fact("word", "pear"), fact("word", "apple"), fact("word", "fig"),
		fact("mixed", "m", 1), fact("mixed", "m", "1"), fact("mixed", "n", "z"), fact("mixed", "n", "y"),
		fact("edge", "x", "y"), fact("edge", "y", "z"), fact("edge", "z", "w"),
	}
}

// aggRun compiles text over aggSchema, evaluates it over aggFacts and returns the
// relation's rows.
func aggRun(t *testing.T, text, relation string, opts tarski.Options) []string {
	t.Helper()
	plan, err := tarski.Compile(aggSchema, parse(t, text))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := tarski.NewBase(aggSchema, aggFacts()...)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	ans, err := res.Relation(relation)
	if err != nil {
		t.Fatal(err)
	}
	return rows(ans)
}

func TestEvaluate_Aggregates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, text, relation string
		want                 []string
	}{
		{"count per group, empty group 0", `n(G, N) :- grp(G), N = count : { item(G, _) }.`, "n", []string{"a 3", "b 3", "e 0"}},
		{"sum per group, empty group 0", `s(G, S) :- grp(G), S = sum V : { item(G, V) }.`, "s", []string{"a 6", "b 9.5", "e 0"}},
		{"min per group, empty group none", `lo(G, M) :- grp(G), M = min V : { item(G, V) }.`, "lo", []string{"a 1", "b -2.5"}},
		{"max per group, empty group none", `hi(G, M) :- grp(G), M = max V : { item(G, V) }.`, "hi", []string{"a 3", "b 10"}},
		{"count with no outer body: wildcards are fresh", `n(N) :- N = count : { item(_, _) }.`, "n", []string{"6"}},
		{"sum per witness: a value two groups share is added twice", `s(S) :- S = sum V : { item(G, V) }.`, "s", []string{"15.5"}},
		{"count over an empty relation is 0", `n(N) :- N = count : { tag(_, _) }.`, "n", []string{"0"}},
		{"sum over an empty relation is 0", `n(N) :- N = sum V : { tag(_, V) }.`, "n", []string{"0"}},
		{"min over an empty relation derives nothing", `n(N) :- N = min V : { tag(_, V) }.`, "n", nil},
		{"several aggregates in one rule",
			`st(G, N, S, L, H) :- grp(G), N = count : { item(G, _) }, S = sum V : { item(G, V) }, L = min V : { item(G, V) }, H = max V : { item(G, V) }.`,
			"st", []string{"a 3 6 1 3", "b 3 9.5 -2.5 10"}},
		{"over a derived relation of a lower stratum", `big(G, V) :- item(G, V), V > 1. n(G, N) :- grp(G), N = count : { big(G, _) }.`,
			"n", []string{"a 2", "b 2", "e 0"}},
		{"recursion below the aggregate",
			`reach(X, Y) :- edge(X, Y). reach(X, Z) :- reach(X, Y), edge(Y, Z). out(X, N) :- edge(X, _), N = count : { reach(X, _) }.`,
			"out", []string{"x 3", "y 2", "z 1"}},
		{"a recursive rule that aggregates a lower relation",
			`lvl(X, N) :- edge(X, _), N = count : { edge(_, _) }. lvl(Y, N) :- lvl(X, N), edge(X, Y), K = count : { edge(Y, _) }, K > 0.`,
			"lvl", []string{"x 3", "y 3", "z 3"}},
		{"negation inside", `n(G, N) :- grp(G), N = count : { item(G, V), not bad(V) }.`, "n", []string{"a 2", "b 3", "e 0"}},
		{"comparison inside", `l(M) :- M = max W : { word(W), W < "g" }.`, "l", []string{"fig"}},
		{"builtin inside", `n(N) :- N = count : { word(W), prefix("p", W) }.`, "n", []string{"1"}},
		{"builtin inside on a group variable", `n(P, N) :- word(P), N = count : { word(W), prefix(P, W) }.`, "n", []string{"apple 1", "fig 1", "pear 1"}},
		{"constant in the group key", `n(N) :- N = count : { item("a", _) }. s(S) :- S = sum V : { item("b", V) }.`, "n", []string{"3"}},
		{"constant in the group key, sum", `s(S) :- S = sum V : { item("b", V) }.`, "s", []string{"9.5"}},
		{"min and max over strings are lexical", `lo(W) :- W = min X : { word(X) }. hi(W) :- W = max X : { word(X) }. both(L, H) :- lo(L), hi(H).`,
			"both", []string{"apple pear"}},
		{"max over two kinds derives nothing; one kind still does", `m(G, X) :- mixed(G, _), X = max V : { mixed(G, V) }.`, "m", []string{"n z"}},
		{"sum over a non-number derives nothing", `s(S) :- S = sum V : { word(V) }.`, "s", nil},
		{"a result already bound is a test", `three(G) :- grp(G), item(G, N), N = count : { item(G, _) }.`, "three", []string{"a"}},
		{"a guard on the result", `busy(G) :- grp(G), N = count : { item(G, V), V > 1 }, N >= 2.`, "busy", []string{"a", "b"}},
		{"a negation on the result", `odd(G) :- grp(G), N = count : { item(G, _) }, not item(G, N).`, "odd", []string{"b", "e"}},
		{"two aggregates sharing a result name test each other", `same(G) :- grp(G), N = count : { item(G, _) }, N = count : { item(G, V), V != 99 }.`,
			"same", []string{"a", "b", "e"}},
		{"an inner local variable named like another aggregate's", `t(G, N, M) :- grp(G), N = count : { item(G, V) }, M = count : { item(V, _) }.`,
			"t", []string{"a 3 6", "b 3 6", "e 0 6"}},
		{"a body with only guards over group variables", `n(G, N) :- grp(G), N = count : { G != "a" }.`, "n", []string{"a 0", "b 1", "e 1"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			for _, proofs := range []bool{false, true} {
				if got := aggRun(t, c.text, c.relation, tarski.Options{Proofs: proofs}); !same(got, c.want) {
					t.Errorf("%s (proofs %v) = %v, want %v", c.relation, proofs, got, c.want)
				}
			}
		})
	}
}
