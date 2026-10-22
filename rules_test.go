package datalog_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// A rule keeps the text it was written as: one line or several, a fact too, trimmed,
// its period kept, and neighbours' text left out.
func TestParse_RuleText(t *testing.T) {
	t.Parallel()
	p := parse(t, `% leading comment
kind("bp").
  up(X) :- node(X),    not edge(X, _).
multi(X, Y) :-
    edge(X, Y),
    % a comment inside stays
    X != Y.
hook/1.
last(1).`)
	want := []string{
		`kind("bp").`,
		`up(X) :- node(X),    not edge(X, _).`,
		"multi(X, Y) :-\n    edge(X, Y),\n    % a comment inside stays\n    X != Y.",
		`last(1).`,
	}
	if len(p.Rules) != len(want) {
		t.Fatalf("%d rules, want %d", len(p.Rules), len(want))
	}
	for i, r := range p.Rules {
		if r.Text != want[i] {
			t.Errorf("rule %d text = %q, want %q", i, r.Text, want[i])
		}
		if r.Set != "" {
			t.Errorf("rule %d set = %q, want none from Parse", i, r.Set)
		}
	}
	if p.Name != "" {
		t.Errorf("Parse named the program %q", p.Name)
	}
}

// NewRuleSet names its program "name/version", and Compile tags the plan's copy of
// each rule with it, leaving a bare program's rules untagged and the caller's alone.
func TestCompile_TagsRulesWithTheirSet(t *testing.T) {
	t.Parallel()
	set, err := datalog.NewRuleSet("demo", 3, "up(X) :- node(X).\n")
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	if set.Program.Name != "demo/3" {
		t.Errorf("program name = %q, want demo/3", set.Program.Name)
	}
	bare := parse(t, `down(X) :- node(X).`)
	plan, err := datalog.Compile(testSchema, set.Program, bare)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a")), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	for rel, want := range map[string]string{"up": "demo/3", "down": ""} {
		p, ok := res.Proof(rel, datalog.Tuple{datalog.String("a")})
		if !ok || p.Rule == nil || p.Rule.Set != want {
			t.Fatalf("Proof(%s a) = %+v, %v; want a rule from set %q", rel, p, ok, want)
		}
	}
	if set.Program.Rules[0].Set != "" || bare.Rules[0].Set != "" {
		t.Error("Compile tagged the caller's rules, want only the plan's copies")
	}
}

// With two sets loaded, a proof says which set each rule came from and shows the rule
// as written: the consumer's rule over the base set's derived tuple.
func TestProof_NamesTheSetAndTheText(t *testing.T) {
	t.Parallel()
	base, err := datalog.NewRuleSet("base", 1, "reach(X, Y) :- edge(X, Y).\nreach(X, Z) :- reach(X, Y), edge(Y, Z).\n")
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	extra, err := datalog.NewRuleSet("extra", 2, "island(X) :-\n  node(X),\n  not reach(X, _).\n")
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	plan, err := datalog.Compile(testSchema, base.Program, extra.Program)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	b := newBase(t, fact("node", "a"), fact("node", "z"), fact("edge", "a", "b"), fact("edge", "b", "c"))
	res, err := datalog.Evaluate(context.Background(), plan, b, datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	island, ok := res.Proof("island", datalog.Tuple{datalog.String("z")})
	if !ok || island.Rule.Set != "extra/2" || island.Rule.Text != "island(X) :-\n  node(X),\n  not reach(X, _)." {
		t.Fatalf("island proof = %+v, %v", island, ok)
	}
	reach, ok := res.Proof("reach", datalog.Tuple{datalog.String("a"), datalog.String("c")})
	if !ok || reach.Rule.Set != "base/1" || reach.Rule.Text != "reach(X, Z) :- reach(X, Y), edge(Y, Z)." {
		t.Fatalf("reach proof = %+v, %v", reach, ok)
	}
	if got := reach.Body[0].Rule; got == nil || got.Set != "base/1" || got.Text != "reach(X, Y) :- edge(X, Y)." {
		t.Errorf("the inner proof's rule = %+v, want base/1 and its own text", got)
	}
}

// Clone copies everything and shares nothing: it equals the original, and writing
// through the copy's atoms, comparisons, declarations and rules leaves the original be.
func TestProgram_Clone(t *testing.T) {
	t.Parallel()
	set, err := datalog.NewRuleSet("demo", 1, "hook/2.\nup(X) :- node(X), not edge(X, _), X != \"q\".\nkind(\"bp\").\n")
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	orig := set.Program
	orig.Rules[0].Set = "demo/1"
	want := orig.Clone()
	if !reflect.DeepEqual(orig, want) {
		t.Fatalf("Clone differs from the original:\n%+v\n%+v", orig, want)
	}
	if orig.Name == "" || orig.Text == "" || orig.Rules[0].Text == "" || orig.Rules[0].Set == "" || orig.Rules[0].Line == 0 || len(orig.Declared) == 0 {
		t.Fatal("the test program leaves a field empty, so the copy proves nothing about it")
	}
	c := orig.Clone()
	c.Name, c.Text = "x", "x"
	c.Declared["hook"] = 9
	c.Declared["new"] = 1
	c.Rules[0].Head.Name = "x"
	c.Rules[0].Head.Args[0] = datalog.Term{Var: "Z"}
	c.Rules[0].Body[0].Atom.Name = "x"
	c.Rules[0].Body[1].Atom.Args[0] = datalog.Term{Var: "Z"}
	c.Rules[0].Body[2].Compare.Op = "="
	c.Rules[0].Text, c.Rules[0].Set = "x", "x"
	c.Rules[1].Head.Args[0] = datalog.Term{Var: "Z"}
	if !reflect.DeepEqual(orig, want) {
		t.Errorf("writing through the clone changed the original:\n%+v\nwas\n%+v", orig, want)
	}
	if zero := (datalog.Program{}).Clone(); zero.Rules != nil || zero.Declared != nil {
		t.Errorf("Clone of the zero Program = %+v, want it zero", zero)
	}
}
