package tarski_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Frozen bytes for aggregates (docs/design/aggregates.md §6), beside golden_test.go's,
// which aggregates leave untouched. An aggregate's value is a constant made the way
// every constant is, so its text, the answer bytes and the hash are as frozen as any:
// the same rules apply. Never update a literal to make the test pass.

const (
	goldenAggregateRules = "agg(K, N, S, L, H) :- key(K), N = count : { v(K, _) }, S = sum X : { v(K, X) }, " +
		"L = min X : { v(K, X) }, H = max X : { v(K, X) }.\n" +
		"cnt(K, N) :- key(K), N = count : { v(K, _) }.\n"

	// The sum is 1e100 - 12.2 held exactly: 101 significant digits, so the text is the
	// exact scientific form, every digit kept.
	goldenAggregateBytes = `{"relation":"agg","tuples":[["a",4,` +
		`9.9999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999999878e+99,` +
		`-12.5,1e+100]]}`
	goldenAggregateHash  = "sha256:655c9dcd1db8986ff30323ec6383222b931212b2f7a9efac4e6df53831b521f5"
	goldenCountBytes     = `{"relation":"cnt","tuples":[["a",4],["e",0]]}`
	goldenAggregateSetID = "golden-aggregate/1 sha256:87a01d7deea0d01d194297711bf85ac70caed909d8b942d4334dbd5fbf2437ed"
)

func TestGolden_AggregateAnswers(t *testing.T) {
	schema := tarski.Schema{Arities: map[string]int{"key": 1, "v": 2}}
	v := func(k, lexeme string) tarski.Fact {
		return tarski.Fact{Relation: "v", Tuple: tarski.Tuple{tarski.String(k), goldenNumber(t, lexeme)}}
	}
	base, err := tarski.NewBase(schema,
		tarski.Fact{Relation: "key", Tuple: tarski.Tuple{tarski.String("a")}},
		tarski.Fact{Relation: "key", Tuple: tarski.Tuple{tarski.String("e")}},
		v("a", "0.1"), v("a", "0.2"), v("a", "-12.50"), v("a", "1e100"),
	)
	if err != nil {
		t.Fatal(err)
	}
	set, err := tarski.NewRuleSet("golden-aggregate", 1, goldenAggregateRules)
	if err != nil {
		t.Fatal(err)
	}
	if got := set.ID(); got != goldenAggregateSetID {
		t.Errorf("rule-set ID moved: got %s, want %s", got, goldenAggregateSetID)
	}
	plan, err := tarski.Compile(schema, set.Program)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
	if err != nil {
		t.Fatal(err)
	}
	agg, err := res.Relation("agg")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(agg.Bytes()); got != goldenAggregateBytes {
		t.Errorf("aggregate answer bytes moved:\n got %s\nwant %s", got, goldenAggregateBytes)
	}
	if got := agg.Hash(); got != goldenAggregateHash {
		t.Errorf("aggregate answer hash moved: got %s, want %s", got, goldenAggregateHash)
	}
	cnt, err := res.Relation("cnt")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(cnt.Bytes()); got != goldenCountBytes {
		t.Errorf("count answer bytes moved:\n got %s\nwant %s", got, goldenCountBytes)
	}
}
