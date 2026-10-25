package tarski_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The frozen bytes (README § What it promises consumers). Consumers store answer
// hashes and rule-set hashes in receipts and verify against them later, so these
// literals are a contract with Shearline, Anam and Ductile, not an implementation
// detail. A change here is a breaking change: bump the version, write it in the
// changelog, and post it to every consumer before merging. Never update a literal to
// make the test pass.

const (
	goldenRules = "out(X, Y) :- v(X, Y).\n"

	goldenBytes = `{"relation":"out","tuples":[["huge",1e+100],["int",9007199254740993],` +
		`["ms",1792269840000],["neg",-12.5],["off",false],` +
		`["quote \" slash \\ tab \t é",true],["small",0.00001]]}`
	goldenAnswerHash = "sha256:13c1a5ae5b6f4c7f3015caf826ed5f71750537079b31cd7ee289af5620b32f80"
	goldenRuleSetID  = "golden/1 sha256:46f36924420c19d07b3ce0f2622dc32c4a23c63529314e596ded8d4552f85670"
)

func goldenNumber(t *testing.T, lexeme string) tarski.Const {
	t.Helper()
	c, err := tarski.Number(lexeme)
	if err != nil {
		t.Fatalf("Number(%q): %v", lexeme, err)
	}
	return c
}

func TestGolden_AnswerBytesAndHash(t *testing.T) {
	schema := tarski.Schema{Arities: map[string]int{"v": 2}}
	v := func(k string, c tarski.Const) tarski.Fact {
		return tarski.Fact{Relation: "v", Tuple: tarski.Tuple{tarski.String(k), c}}
	}
	base, err := tarski.NewBase(schema,
		v("quote \" slash \\ tab \t é", tarski.Bool(true)),
		v("int", goldenNumber(t, "9007199254740993")),
		v("ms", goldenNumber(t, "1792269840000")),
		v("neg", goldenNumber(t, "-12.50")),
		v("small", goldenNumber(t, "0.00001")),
		v("huge", goldenNumber(t, "1e100")),
		v("off", tarski.Bool(false)),
	)
	if err != nil {
		t.Fatal(err)
	}
	prog, err := tarski.Parse(goldenRules)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tarski.Compile(schema, prog)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ans, err := res.Relation("out")
	if err != nil {
		t.Fatal(err)
	}
	if got := string(ans.Bytes()); got != goldenBytes {
		t.Errorf("answer bytes moved:\n got %s\nwant %s", got, goldenBytes)
	}
	if got := ans.Hash(); got != goldenAnswerHash {
		t.Errorf("answer hash moved: got %s, want %s", got, goldenAnswerHash)
	}
	var back tarski.Answer
	if err := back.UnmarshalJSON([]byte(goldenBytes)); err != nil || back.Hash() != goldenAnswerHash {
		t.Errorf("golden bytes read back as hash %q, err %v", back.Hash(), err)
	}
}

func TestGolden_RuleSetID(t *testing.T) {
	set, err := tarski.NewRuleSet("golden", 1, goldenRules)
	if err != nil {
		t.Fatal(err)
	}
	if got := set.ID(); got != goldenRuleSetID {
		t.Errorf("rule-set ID moved: got %s, want %s", got, goldenRuleSetID)
	}
}

func TestGolden_ConstantText(t *testing.T) {
	tests := []struct {
		in   tarski.Const
		want string
	}{
		{goldenNumber(t, "1.0"), "1"},
		{goldenNumber(t, "-0"), "0"},
		{goldenNumber(t, "1000000"), "1000000"},
		{goldenNumber(t, "1e21"), "1000000000000000000000"},
		{goldenNumber(t, "0.000100"), "0.0001"},
		{goldenNumber(t, "1e100"), "1e+100"},
		{tarski.Int(-42), "-42"},
		{tarski.Bool(true), "true"},
		{tarski.String("a\"b"), `a"b`},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.in.Text(); got != tt.want {
				t.Errorf("Text() = %q, want %q", got, tt.want)
			}
		})
	}
}
