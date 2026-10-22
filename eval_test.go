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
	if !errors.Is(err, datalog.ErrTooLarge) || !errors.As(err, &big) || big.Relation != "pairs" || big.MaxBytes != 200 || big.Bound != datalog.BoundBytes || !strings.HasPrefix(err.Error(), "datalog: ") {
		t.Fatalf("Evaluate = %v, want ErrTooLarge at pairs over 200", err)
	}
	if ans, err := tryRun(base, text, "few", datalog.Options{}); err != nil || ans.Len() != 1 {
		t.Errorf("unbounded = %v, %d tuples; want one", err, ans.Len())
	}
	if _, err := tryRun(base, `few(A) :- node(A), A = "n1".`, "few", datalog.Options{MaxBytes: 200}); err != nil {
		t.Errorf("a derivation under the bound = %v, want none", err)
	}
}

// Options.Proofs is accepted and a derived tuple then has a proof (see proof_test.go).
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
	if p, ok := res.Proof("n", datalog.Tuple{datalog.String("a")}); !ok || p.Rule == nil {
		t.Error("a derived tuple has no proof under Options.Proofs")
	}
}

// A bound on bytes cannot see work that derives little: a cross product whose every
// row is a duplicate. Options.MaxSteps stops it, and the refusal says which bound.
func TestEvaluate_MaxSteps(t *testing.T) {
	t.Parallel()
	var facts []datalog.Fact
	for i := 0; i < 30; i++ {
		facts = append(facts, fact("node", fmt.Sprint("n", i)))
	}
	base := newBase(t, facts...)
	text := `dup(A) :- node(A), node(B), node(C).`
	_, err := tryRun(base, text, "dup", datalog.Options{MaxBytes: 1 << 20, MaxSteps: 1000})
	var big *datalog.TooLarge
	if !errors.Is(err, datalog.ErrTooLarge) || !errors.As(err, &big) {
		t.Fatalf("Evaluate = %v, want ErrTooLarge", err)
	}
	if big.MaxSteps != 1000 || big.Steps != 1001 || big.Relation != "dup" || big.Bound != "MaxSteps" || !strings.HasPrefix(err.Error(), "datalog: ") {
		t.Errorf("TooLarge = %+v, want 1001 steps over 1000 at dup", *big)
	}
	for _, want := range []string{"1001 join steps", "Options.MaxSteps", "more selective"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	if ans, err := tryRun(base, text, "dup", datalog.Options{MaxSteps: 100000}); err != nil || ans.Len() != 30 {
		t.Errorf("under the bound = %v, %d tuples; want 30", err, ans.Len())
	}
	if _, err := tryRun(base, text, "dup", datalog.Options{}); err != nil {
		t.Errorf("no bound = %v, want none", err)
	}
}

// The deadline is looked at before the first join, not only every few thousand steps,
// so a small evaluation on a context that is already done fails.
func TestEvaluate_CancelledContextFailsASmallEvaluation(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(testSchema, parse(t, `n(X) :- node(X).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base := newBase(t, fact("node", "a"), fact("node", "b"), fact("node", "c"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = datalog.Evaluate(ctx, plan, base, datalog.Options{})
	if !errors.Is(err, context.Canceled) || !strings.Contains(err.Error(), "datalog: evaluation stopped") {
		t.Errorf("Evaluate on a cancelled context = %v, want context.Canceled from datalog: evaluation stopped", err)
	}
	expired, stop := context.WithTimeout(context.Background(), 0)
	defer stop()
	if _, err := datalog.Evaluate(expired, plan, base, datalog.Options{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Evaluate past its deadline = %v, want context.DeadlineExceeded", err)
	}
}

// A bound crossed by the very last tuple of the last round still ends in *TooLarge.
// Round one derives r(1); round two derives r(2), the last candidate of its join and
// the crossing tuple; round three's only rule asks for r(1) in the delta, finds none,
// and visits nothing, so no join is left to notice. Evaluate must.
func TestEvaluate_MaxBytesCrossedByTheLastTuple(t *testing.T) {
	t.Parallel()
	base := newBase(t, fact("node", 1), fact("edge", 1, 2))
	text := `r(1) :- node(1). r(Y) :- r(1), edge(1, Y).`
	if ans, err := tryRun(base, text, "r", datalog.Options{}); err != nil || ans.Len() != 2 {
		t.Fatalf("unbounded = %v, %d tuples; want r(1) and r(2)", err, ans.Len())
	}
	// Each r tuple is charged 3 bytes: 6 in all, so a bound of 5 is crossed by r(2).
	_, err := tryRun(base, text, "r", datalog.Options{MaxBytes: 5})
	if !errors.Is(err, datalog.ErrTooLarge) {
		t.Errorf("Evaluate over MaxBytes = %v, want ErrTooLarge", err)
	}
}

// A negation probe is work like a join and is charged like one: scanning a hub's
// many edges for one that is not there is stopped by MaxSteps.
func TestEvaluate_MaxStepsCoversNegationProbes(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{fact("node", "n0")}
	for i := 0; i < 500; i++ {
		facts = append(facts, fact("edge", "hub", fmt.Sprint("n", i)))
	}
	base := newBase(t, facts...)
	text := `ok(X) :- node(X), not edge("hub", "zzz").`
	_, err := tryRun(base, text, "ok", datalog.Options{MaxSteps: 100})
	if !errors.Is(err, datalog.ErrTooLarge) {
		t.Errorf("a 500-candidate probe under MaxSteps 100 = %v, want ErrTooLarge", err)
	}
	if ans, err := tryRun(base, text, "ok", datalog.Options{MaxSteps: 10000}); err != nil || ans.Len() != 1 {
		t.Errorf("under the bound = %v, %d tuples; want 1", err, ans.Len())
	}
}

// An Answer is a value its holder owns: writing through its tuples or its bytes does
// not reach the frozen base every other evaluation reads.
func TestAnswer_DoesNotAliasTheBase(t *testing.T) {
	t.Parallel()
	base := newBase(t, fact("edge", "a", "b"), fact("edge", "b", "c"))
	text := `e(X, Y) :- edge(X, Y).`
	first, err := tryRun(base, text, "e", datalog.Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	hash, before := first.Hash(), rows(first)
	first.Tuples[0][0] = datalog.String("tampered")
	first.Tuples[1] = nil
	first.Bytes()[0] = 'X'
	again, err := tryRun(base, text, "e", datalog.Options{})
	if err != nil || again.Hash() != hash || !same(rows(again), before) {
		t.Errorf("after writing through an answer: %v, hash %s (was %s), rows %v (were %v)",
			err, again.Hash(), hash, rows(again), before)
	}
	if again.Bytes()[0] != '{' || first.Bytes()[0] != '{' {
		t.Error("Bytes() is not a copy: a write to it changed an answer")
	}
}
