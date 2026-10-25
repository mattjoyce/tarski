package tarski_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

func whyEmptyResult(t *testing.T, proofs bool, facts ...tarski.Fact) tarski.Result {
	t.Helper()
	schema := tarski.Schema{Arities: map[string]int{"reading": 2, "noted": 1}}
	prog, err := tarski.Parse(`
summer(At, V) :- reading(At, V), At >= "2026-06", At < "2026-09".
high(At) :- summer(At, V), V > 140.
quiet(At) :- reading(At, _), not noted(At).
`)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := tarski.Compile(schema, prog)
	if err != nil {
		t.Fatal(err)
	}
	base, err := tarski.NewBase(schema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: proofs})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestWhyEmpty(t *testing.T) {
	res := whyEmptyResult(t, true,
		fact("reading", "2026-04-10", 146),
		fact("reading", "2026-07-20", 128),
		fact("noted", "2026-04-10"),
		fact("noted", "2026-07-20"),
	)
	tests := []struct {
		name       string
		relation   string
		ok         bool
		wantFailed func(tarski.Literal) bool // the farthest literal of the first rule
		wantInText string
	}{
		{"window misses the high reading", "high", true, func(l tarski.Literal) bool { return l.Compare != nil && l.Compare.Op == ">" }, "high(_) is not derived"},
		{"every reading noted", "quiet", true, func(l tarski.Literal) bool { return l.Negated && l.Atom != nil && l.Atom.Name == "noted" }, "blocked by"},
		{"summer is not empty", "summer", false, nil, ""},
		{"base relation not empty", "reading", false, nil, ""},
		{"unknown relation", "nope", false, nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, ok := res.WhyEmpty(tt.relation, 0)
			if ok != tt.ok {
				t.Fatalf("WhyEmpty(%s) ok = %v, want %v", tt.relation, ok, tt.ok)
			}
			if !ok {
				return
			}
			if len(a.Rules) == 0 {
				t.Fatalf("WhyEmpty(%s) has no rule failures: %s", tt.relation, a)
			}
			if !tt.wantFailed(a.Rules[0].Failed) {
				t.Errorf("first rule stopped at %+v, not the literal expected\n%s", a.Rules[0].Failed, a)
			}
			if !strings.Contains(a.String(), tt.wantInText) {
				t.Errorf("text lacks %q:\n%s", tt.wantInText, a)
			}
		})
	}
}

func TestWhyEmpty_BaseRelationAndNoProofs(t *testing.T) {
	res := whyEmptyResult(t, true)
	a, ok := res.WhyEmpty("reading", 0)
	if !ok || a.Rules != nil {
		t.Fatalf("WhyEmpty over an empty base relation = %v, %v; want ok with nil Rules (no such fact)", a, ok)
	}
	if _, ok := whyEmptyResult(t, false).WhyEmpty("high", 0); ok {
		t.Error("WhyEmpty without Options.Proofs answered; want ok false")
	}
}
