package datalog_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// The evaluator reaches a fixpoint through recursion (semi-naive: a path found in
// one round is joined again in the next), stops, and removes duplicates.
func TestEvaluate_RecursionReachesTheFixpoint(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{
		fact("edge", "a", "b"), fact("edge", "b", "c"), fact("edge", "c", "d"), fact("edge", "d", "b"), // a cycle b-c-d
		fact("edge", "a", "c"), // a second way to c: no duplicate in the answer
	}
	ans := run(t, facts, `
		reaches(X, Y) :- edge(X, Y).
		reaches(X, Z) :- reaches(X, Y), edge(Y, Z).
	`, "reaches")
	want := []string{"a b", "a c", "a d", "b b", "b c", "b d", "c b", "c c", "c d", "d b", "d c", "d d"}
	if got := rows(ans); !same(got, want) {
		t.Errorf("reaches = %v, want %v", got, want)
	}
}

// Comparisons: numbers numerically, strings lexically, and a mixed pair is unequal
// and unordered. A constant in a body atom selects, and facts in a program count.
func TestEvaluate_Comparisons(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{
		fact("score", "r1", 118), fact("score", "r2", 146.0), fact("score", "r3", 9), fact("score", "r4", "high"),
		fact("flag", "r1", true), fact("flag", "r2", false),
	}
	cases := []struct {
		name, text, relation string
		want                 []string
	}{
		{"numeric order", `high(E) :- score(E, S), S > 140.`, "high", []string{"r2"}},
		{"not lexical", `low(E) :- score(E, S), S < 100.`, "low", []string{"r3"}},
		{"146.0 is 146", `exact(E) :- score(E, 146).`, "exact", []string{"r2"}},
		{"at least", `ge(E) :- score(E, S), S >= 118.`, "ge", []string{"r1", "r2"}},
		{"at most", `le(E) :- score(E, S), S <= 118, S != 9.`, "le", []string{"r1"}},
		{"strings order lexically", `s(E) :- score(E, S), S > "a".`, "s", []string{"r4"}},
		{"string vs number", `odd(E) :- score(E, S), S != "118".`, "odd", []string{"r1", "r2", "r3", "r4"}},
		{"unordered kinds", `never(E) :- score(E, S), S < "z", S > 1000.`, "never", nil},
		{"mixed kinds are unequal", `eq(E) :- score(E, S), S = "118".`, "eq", nil},
		{"booleans", `on(E) :- flag(E, true).`, "on", []string{"r1"}},
		{"facts in the program", `kind("bp"). kind("sleep"). k(X) :- kind(X), X != "sleep".`, "k", []string{"bp"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := rows(run(t, facts, c.text, c.relation)); !same(got, c.want) {
				t.Errorf("%s = %v, want %v", c.relation, got, c.want)
			}
		})
	}
}

// Negation reads a complete lower stratum: nodes with no outgoing edge, and the ones
// that cannot reach such a node's cycle, computed after the reachability they negate.
func TestEvaluate_StratifiedNegation(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{
		fact("node", "a"), fact("node", "b"), fact("node", "c"), fact("node", "d"),
		fact("edge", "a", "b"), fact("edge", "b", "c"),
	}
	ans := run(t, facts, `
		reaches(X, Y) :- edge(X, Y).
		reaches(X, Z) :- reaches(X, Y), edge(Y, Z).
		sink(X) :- node(X), not edge(X, _).
		cut(X) :- node(X), not reaches("a", X), X != "a".
		top(X) :- sink(X), not cut(X).
	`, "top")
	if got := rows(ans); !same(got, []string{"c"}) {
		t.Errorf("top = %v, want [c]", got)
	}
}

// An answer is a value: sorted, serialised once, hashed; and the same program over
// the same facts hashes the same, in whatever order the facts came.
func TestAnswer_IsAValue(t *testing.T) {
	t.Parallel()
	text := `ids(E, N) :- score(E, N).`
	first := run(t, []datalog.Fact{fact("score", "b", 2), fact("score", "a", 1)}, text, "ids")
	second := run(t, []datalog.Fact{fact("score", "a", 1), fact("score", "b", 2)}, text, "ids")
	if first.Hash() != second.Hash() {
		t.Error("two evaluations of one program over the same facts hash differently")
	}
	want := `{"relation":"ids","tuples":[["a",1],["b",2]]}`
	if string(first.Bytes()) != want {
		t.Errorf("answer bytes = %s, want %s", first.Bytes(), want)
	}
	if !strings.HasPrefix(first.Hash(), "sha256:") {
		t.Errorf("hash = %s, want sha256:…", first.Hash())
	}
	if _, err := tryRun(newBase(t), text, "nothing", datalog.Options{}); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("an unnamed relation = %v, want ErrInvalid", err)
	}
	var back datalog.Answer
	if err := back.UnmarshalJSON(first.Bytes()); err != nil || back.Hash() != first.Hash() {
		t.Errorf("an answer read back = %v, hash %s, want %s", err, back.Hash(), first.Hash())
	}
}

// A cancelled context stops an evaluation rather than letting it run on.
func TestEvaluate_HonoursTheDeadline(t *testing.T) {
	t.Parallel()
	var facts []datalog.Fact
	for i := 0; i < 200; i++ {
		facts = append(facts, fact("node", fmt.Sprint("n", i)))
	}
	plan, err := datalog.Compile(testSchema, parse(t, `triples(A, B, C) :- node(A), node(B), node(C).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := datalog.Evaluate(ctx, plan, newBase(t, facts...), datalog.Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("Evaluate on a cancelled context = %v, want context.Canceled", err)
	}
}

// Options.MaxBytes bounds what is derived, intermediate relations included, and the
// refusal names the relation that crossed it.
func TestEvaluate_MaxBytes(t *testing.T) {
	t.Parallel()
	var facts []datalog.Fact
	for i := 0; i < 20; i++ {
		facts = append(facts, fact("node", fmt.Sprint("n", i)))
	}
	base := newBase(t, facts...)
	text := `pairs(A, B) :- node(A), node(B). few(A) :- node(A), A = "n1".`
	_, err := tryRun(base, text, "few", datalog.Options{MaxBytes: 200})
	var big *datalog.TooLarge
	if !errors.Is(err, datalog.ErrTooLarge) || !errors.As(err, &big) || big.Relation != "pairs" || big.MaxBytes != 200 {
		t.Fatalf("Evaluate = %v, want ErrTooLarge at pairs over 200", err)
	}
	if ans, err := tryRun(base, text, "few", datalog.Options{}); err != nil || ans.Len() != 1 {
		t.Errorf("unbounded = %v, %d tuples; want one", err, ans.Len())
	}
	if _, err := tryRun(base, `few(A) :- node(A), A = "n1".`, "few", datalog.Options{MaxBytes: 200}); err != nil {
		t.Errorf("a derivation under the bound = %v, want none", err)
	}
}

// Options.Proofs is accepted; recording is pending, so Proof reports false.
func TestEvaluate_ProofsFlagIsAccepted(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(testSchema, parse(t, `n(X) :- node(X).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a")), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if _, ok := res.Proof("n", datalog.Tuple{datalog.String("a")}); ok {
		t.Error("Proof reported a derivation before recording exists")
	}
}
