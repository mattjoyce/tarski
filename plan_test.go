package datalog_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// Validation refuses what cannot run, before anything is read, with a message that
// says what to change.
func TestCompile_RefusesWithAReason(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, text, want string
	}{
		{"unknown relation", `a(X) :- node(X), nothing(X).`, "unknown relation nothing/1; the relations are a/1, edge/2, flag/2, node/1, score/2"},
		{"arity", `a(X) :- edge(X).`, "edge has 1 arguments, but edge is edge/2"},
		{"two arities for one head", `a(X) :- node(X). a(X, Y) :- edge(X, Y).`, "a/2, but a is a/1"},
		{"rule over a base relation", `node(X) :- edge(X, _).`, "node/1 is a base relation; a rule cannot add to it"},
		{"declaring a base relation", `edge/2.`, "edge/2 is a base relation and cannot be declared"},
		{"unbound head variable", `a(X, Y) :- node(X).`, "Y in the head of a is not bound by a positive body atom"},
		{"anonymous head", `a(_) :- node(_).`, "_ in the head of a"},
		{"unbound in negation", `a(X) :- node(X), not edge(X, P), P != "".`, "P in not edge is not bound"},
		{"unbound in comparison", `a(X) :- node(X), Y > 1.`, "Y in a comparison is not bound"},
		{"anonymous in comparison", `a(X) :- node(X), _ > 1.`, "_ in a comparison compares nothing"},
		{"unstratifiable", `a(X) :- node(X), not b(X). b(X) :- node(X), not a(X).`, "depend on each other through a negation"},
		{"schema check, body", `a(X) :- node("!x"), node(X).`, "line 1: node !x: a node name does not start with !"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := datalog.Compile(testSchema, parse(t, c.text))
			if !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
}

// What is allowed: recursion through positive atoms, negation of a lower stratum, a
// relation a later program defines, and a relation declared and never defined.
func TestCompile_Accepts(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, text string }{
		{"recursion", `r(X, Y) :- edge(X, Y). r(X, Z) :- r(X, Y), edge(Y, Z).`},
		{"negation of a lower stratum", `a(X) :- node(X), not b(X). b(X) :- edge(X, _).`},
		{"declared and used", `hook/1. a(X) :- node(X), not hook(X).`},
		{"a clean check", `a(X) :- node("fine"), node(X).`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := datalog.Compile(testSchema, parse(t, c.text)); err != nil {
				t.Errorf("Compile: %v", err)
			}
		})
	}
}

// A check runs on every atom naming its relation, in every rule, and not on the
// other relations. (A base relation is never a head, so its atoms are body atoms.)
func TestCompile_ChecksRunOnEveryAtom(t *testing.T) {
	t.Parallel()
	var seen []string
	schema := datalog.Schema{
		Arities: map[string]int{"edge": 2, "node": 1},
		Checks: map[string]func(datalog.Atom) error{
			"edge": func(a datalog.Atom) error { seen = append(seen, fmt.Sprint(a.Name, a.Line)); return nil },
		},
	}
	_, err := datalog.Compile(schema, parse(t, "a(X) :- edge(X, _), node(X), not edge(X, X).\nb(X) :- edge(X, X)."))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if got := strings.Join(seen, ","); got != "edge1,edge1,edge2" {
		t.Errorf("the edge check saw %s, want edge1,edge1,edge2", got)
	}
}

// A relation declared and defined nowhere is empty, not refused: it is the hook a rule
// set leaves for its consumer.
func TestEvaluate_DeclaredAndUndefinedIsEmpty(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{fact("node", "a"), fact("node", "b")}
	text := `hook/1. open(X) :- node(X), not hook(X). seen(X) :- hook(X).`
	if got := rows(run(t, facts, text, "open")); !same(got, []string{"a", "b"}) {
		t.Errorf("open = %v, want [a b]", got)
	}
	if got := run(t, facts, text, "seen"); got.Len() != 0 {
		t.Errorf("seen = %v, want empty", rows(got))
	}
	if got := rows(run(t, facts, text+` hook("a").`, "open")); !same(got, []string{"b"}) {
		t.Errorf("open with the hook filled = %v, want [b]", got)
	}
}

// A rule set is named, versioned and hashed over its text.
func TestNewRuleSet(t *testing.T) {
	t.Parallel()
	text := "% a comment\nhook/1.\nup(X) :- node(X), not hook(X).\n"
	rs, err := datalog.NewRuleSet("demo", 2, text)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	if rs.Name != "demo" || rs.Version != 2 {
		t.Errorf("rule set is %s v%d, want demo v2", rs.Name, rs.Version)
	}
	if !strings.HasPrefix(rs.Hash, "sha256:") || len(rs.Hash) != len("sha256:")+64 {
		t.Errorf("hash = %q", rs.Hash)
	}
	if rs.Program.Declared["hook"] != 1 || len(rs.Program.Rules) != 1 {
		t.Errorf("program = %+v, want hook/1 and one rule", rs.Program)
	}
	if want := "demo/2 " + rs.Hash; rs.ID() != want {
		t.Errorf("ID = %q, want %q", rs.ID(), want)
	}
	again, _ := datalog.NewRuleSet("demo", 2, text)
	other, _ := datalog.NewRuleSet("demo", 2, text+"\n")
	if again.Hash != rs.Hash || other.Hash == rs.Hash {
		t.Error("the hash should follow the text, byte for byte")
	}
}

func TestNewRuleSet_Refuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		version int
		text    string
		wantIs  error
	}{
		{"", 1, "a(1).", datalog.ErrInvalid},
		{"has space", 1, "a(1).", datalog.ErrInvalid},
		{"semi;colon", 1, "a(1).", datalog.ErrInvalid},
		{"slash/name", 1, "a(1).", datalog.ErrInvalid},
		{"neg", -1, "a(1).", datalog.ErrInvalid},
		{"bad", 1, "a(1)", datalog.ErrSyntax},
	}
	for _, c := range cases {
		if _, err := datalog.NewRuleSet(c.name, c.version, c.text); !errors.Is(err, c.wantIs) {
			t.Errorf("NewRuleSet(%q, %d, %q) = %v, want %v", c.name, c.version, c.text, err, c.wantIs)
		}
	}
}

// Verify holds a rule set to its hash: a built one passes, and one edited or built by
// hand does not.
func TestRuleSet_Verify(t *testing.T) {
	t.Parallel()
	rs, err := datalog.NewRuleSet("demo", 1, "up(X) :- node(X).\n")
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	if err := rs.Verify(); err != nil {
		t.Errorf("Verify on a built set = %v", err)
	}
	edited := rs
	edited.Program.Text += "% edited\n"
	forged := rs
	forged.Hash = "sha256:" + strings.Repeat("0", 64)
	for name, bad := range map[string]datalog.RuleSet{
		"edited text": edited, "forged hash": forged, "no text": {Name: "x", Hash: rs.Hash},
	} {
		if err := bad.Verify(); !errors.Is(err, datalog.ErrInvalid) {
			t.Errorf("%s: Verify = %v, want ErrInvalid", name, err)
		}
	}
}

// Plan.Arity answers for base and derived relations and for nothing else.
func TestPlan_Arity(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(testSchema, parse(t, `hook/3. up(X, Y) :- edge(X, Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	for name, want := range map[string]int{"edge": 2, "node": 1, "up": 2, "hook": 3} {
		if n, ok := plan.Arity(name); !ok || n != want {
			t.Errorf("Arity(%s) = %d, %v; want %d, true", name, n, ok, want)
		}
	}
	if n, ok := plan.Arity("nothing"); ok || n != 0 {
		t.Errorf("Arity(nothing) = %d, %v; want 0, false", n, ok)
	}
}

// A base that holds a relation the plan derives would have those facts silently
// dropped, so it is refused, naming the relation.
func TestEvaluate_RefusesABaseHoldingADerivedRelation(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(datalog.Schema{Arities: map[string]int{"a": 1}}, parse(t, `d(X) :- a(X).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := datalog.NewBase(datalog.Schema{Arities: map[string]int{"a": 1, "d": 1}}, fact("a", "y"), fact("d", "x"))
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	_, err = datalog.Evaluate(context.Background(), plan, base, datalog.Options{})
	if !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), "the base holds d, which the plan derives") {
		t.Errorf("Evaluate = %v, want ErrInvalid naming d", err)
	}
}

var errCheck = errors.New("check sentinel")

// A schema check's own error stays reachable: errors.Is finds the caller's sentinel
// through Compile and through NewBase, beside ErrInvalid.
func TestSchemaChecks_KeepTheirSentinel(t *testing.T) {
	t.Parallel()
	schema := datalog.Schema{
		Arities: map[string]int{"node": 1},
		Checks:  map[string]func(datalog.Atom) error{"node": func(datalog.Atom) error { return errCheck }},
	}
	_, err := datalog.Compile(schema, parse(t, `a(X) :- node(X).`))
	if !errors.Is(err, errCheck) || !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("Compile = %v, want the check's sentinel and ErrInvalid", err)
	}
	_, err = datalog.NewBase(schema, fact("node", "a"))
	if !errors.Is(err, errCheck) || !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("NewBase = %v, want the check's sentinel and ErrInvalid", err)
	}
}

// An empty schema says so, instead of ending a sentence on "the relations are".
func TestErrors_AnEmptySchemaIsNamed(t *testing.T) {
	t.Parallel()
	_, err := datalog.NewBase(datalog.Schema{}, fact("node", "a"))
	if err == nil || !strings.HasSuffix(err.Error(), "the schema has no relations") {
		t.Errorf("NewBase = %v, want it to end by saying the schema has no relations", err)
	}
}
