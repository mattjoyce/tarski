package datalog_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// These tests came from an adversarial audit of the public API: values a caller can
// construct, or change after handing over, that the parser would never produce.

// compileAndRun compiles programs over an empty schema and returns the named relation's
// rows, for the audit cases that need no facts.
func compileAndRun(t *testing.T, relation string, programs ...datalog.Program) []string {
	t.Helper()
	plan, err := datalog.Compile(datalog.Schema{}, programs...)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := datalog.NewBase(datalog.Schema{})
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	ans, err := res.Relation(relation)
	if err != nil {
		t.Fatalf("Relation: %v", err)
	}
	return rows(ans)
}

// A plan owns its rules: changing the Program after Compile, in a head, a body atom, a
// comparison or a declaration, changes nothing the plan derives.
func TestCompile_PlanIsDetachedFromTheCallersProgram(t *testing.T) {
	t.Parallel()
	prog := parse(t, `allow("alice", "read", "alice/private"). pick(X) :- allow(X, _, _), X != "mallory". open/1.`)
	plan, err := datalog.Compile(datalog.Schema{}, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	prog.Rules[0].Head.Args[2].Const = datalog.String("bob/private")
	prog.Rules[1].Body[0].Atom.Args[0] = datalog.Term{Const: datalog.String("zed")}
	prog.Rules[1].Body[1].Compare.Right = datalog.Term{Const: datalog.String("alice")}
	prog.Rules[1].Body[1].Compare.Op = "="
	prog.Declared["open"] = 7
	base, _ := datalog.NewBase(datalog.Schema{})
	res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	allow, _ := res.Relation("allow")
	pick, _ := res.Relation("pick")
	if got := rows(allow); !same(got, []string{"alice read alice/private"}) {
		t.Errorf("allow = %v after the caller edited its program, want the original fact", got)
	}
	if got := rows(pick); !same(got, []string{"alice"}) {
		t.Errorf("pick = %v after the caller edited its program, want [alice]", got)
	}
}

// Verify holds the program that runs to the text that was hashed: an edited AST, a
// changed name or a changed declaration is refused, though the hash still matches.
func TestRuleSet_VerifyHoldsTheASTToTheText(t *testing.T) {
	t.Parallel()
	const text = "open/1.\nallow(\"alice\", \"read\", \"alice/private\").\nok(X) :- allow(X, _, _), X != \"q\".\n"
	fresh := func() datalog.RuleSet {
		set, err := datalog.NewRuleSet("custom", 1, text)
		if err != nil {
			t.Fatalf("NewRuleSet: %v", err)
		}
		return set
	}
	if err := fresh().Verify(); err != nil {
		t.Fatalf("Verify on an untouched set = %v", err)
	}
	if p := fresh().Program.Clone(); p.Name == "" {
		t.Fatal("setup: the program carries no name")
	}
	edits := map[string]func(*datalog.RuleSet){
		"head constant": func(s *datalog.RuleSet) { s.Program.Rules[0].Head.Args[2].Const = datalog.String("bob/private") },
		"body operator": func(s *datalog.RuleSet) { s.Program.Rules[1].Body[1].Compare.Op = "=" },
		"dropped rule":  func(s *datalog.RuleSet) { s.Program.Rules = s.Program.Rules[:1] },
		"declaration":   func(s *datalog.RuleSet) { s.Program.Declared["open"] = 2 },
		"program name":  func(s *datalog.RuleSet) { s.Program.Name = "other/9" },
		"set tag":       func(s *datalog.RuleSet) { s.Program.Rules[0].Set = "other/9" },
		"rule text":     func(s *datalog.RuleSet) { s.Program.Rules[0].Text = "allow(\"bob\")." },
	}
	for name, edit := range edits {
		set := fresh()
		edit(&set)
		if err := set.Verify(); !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), "does not match its text") {
			t.Errorf("%s: Verify = %v, want ErrInvalid, program does not match its text", name, err)
		}
	}
	renamed := fresh()
	renamed.Name = "other" // the set's identity moved; the program still says custom/1
	if err := renamed.Verify(); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("a set renamed after NewRuleSet: Verify = %v, want ErrInvalid", err)
	}
	broken := fresh()
	broken.Program.Text = "not a program (" // hash is wrong too, but the text must parse to be compared
	broken.Hash = "sha256:" + strings.Repeat("0", 64)
	if err := broken.Verify(); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("unparseable text: Verify = %v, want ErrInvalid", err)
	}
}

func atomTerm(s string) datalog.Term { return datalog.Term{Const: datalog.String(s)} }

// A program built by hand that is not a program is refused with a reason, never a panic.
func TestCompile_RefusesMalformedASTs(t *testing.T) {
	t.Parallel()
	head := datalog.Atom{Name: "out", Args: []datalog.Term{atomTerm("x")}}
	cmp := &datalog.Comparison{Op: "=", Left: atomTerm("a"), Right: atomTerm("a")}
	atom := &datalog.Atom{Name: "input", Args: []datalog.Term{{Var: "X"}}}
	rule := func(body ...datalog.Literal) datalog.Program {
		return datalog.Program{Rules: []datalog.Rule{{Head: head, Body: body}}}
	}
	cases := []struct {
		name string
		prog datalog.Program
		want string
	}{
		{"empty literal", rule(datalog.Literal{}), "neither an atom nor a comparison"},
		{"negated empty literal", rule(datalog.Literal{Negated: true}), "neither an atom nor a comparison"},
		{"atom and comparison", rule(datalog.Literal{Atom: atom, Compare: cmp}), "both an atom and a comparison"},
		{"negated comparison", rule(datalog.Literal{Compare: cmp, Negated: true}), "a comparison cannot be negated"},
		{"head without a name", datalog.Program{Rules: []datalog.Rule{{Head: datalog.Atom{Args: head.Args}}}}, "no relation name"},
		{"body atom without a name", rule(datalog.Literal{Atom: &datalog.Atom{Args: []datalog.Term{{Var: "X"}}}}), "no relation name"},
		{"declaration without a name", datalog.Program{Declared: map[string]int{"": 1}}, "declares a relation with no name"},
		{"declaration of arity zero", datalog.Program{Declared: map[string]int{"hook": 0}}, "hook/0"},
		{"declaration of negative arity", datalog.Program{Declared: map[string]int{"hook": -2}}, "hook/-2"},
		{"unknown operator", rule(datalog.Literal{Compare: &datalog.Comparison{Op: "bogus", Left: atomTerm("a"), Right: atomTerm("b")}}), `operator "bogus"`},
		{"empty operator", rule(datalog.Literal{Compare: &datalog.Comparison{Left: atomTerm("a"), Right: atomTerm("b")}}), `operator ""`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Compile panicked: %v", r)
				}
			}()
			_, err := datalog.Compile(datalog.Schema{Arities: map[string]int{"input": 1}}, c.prog)
			if !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
	// The empty string is a constant, and a zero Term is it: nothing to refuse there.
	empty := datalog.Program{Rules: []datalog.Rule{{Head: datalog.Atom{Name: "out", Args: []datalog.Term{{}}}}}}
	if got := compileAndRun(t, "out", empty); !same(got, []string{""}) {
		t.Errorf("a zero Term in a head = %v, want the empty string", got)
	}
}

// Every operator the language has still works, and an unknown one derives nothing
// because it never compiles.
func TestCompile_ComparisonOperators(t *testing.T) {
	t.Parallel()
	for op, want := range map[string]int{"=": 0, "!=": 1, "<": 1, "<=": 1, ">": 0, ">=": 0} {
		prog := parse(t, `out("x") :- 1 = 2.`)
		prog.Rules[0].Body[0].Compare.Op = op
		if got := compileAndRun(t, "out", prog); len(got) != want {
			t.Errorf("1 %s 2 derived %d tuples, want %d", op, len(got), want)
		}
	}
	prog := parse(t, `out("allowed") :- 2 >= 1.`)
	prog.Rules[0].Body[0].Compare.Op = "bogus"
	if _, err := datalog.Compile(datalog.Schema{}, prog); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("an unknown operator compiled: %v", err)
	}
}

// The engine's strings are valid UTF-8, enforced where they enter: JSON would turn
// 0xff and U+FFFD into the same bytes, so two facts would share one answer hash.
func TestStrings_InvalidUTF8IsRefusedAtEntry(t *testing.T) {
	t.Parallel()
	invalid := string([]byte{'a', 0xff})
	_, err := datalog.NewBase(testSchema, fact("node", invalid))
	if !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), "not valid UTF-8") {
		t.Errorf("NewBase with 0xff = %v, want ErrInvalid, not valid UTF-8", err)
	}
	if _, err := newBase(t, fact("node", "a")).With(fact("edge", "ok", invalid)); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("With 0xff = %v, want ErrInvalid", err)
	}
	for _, valid := range []string{"�", "é", "日本", "a\x00b", ""} {
		if _, err := datalog.NewBase(testSchema, fact("node", valid)); err != nil {
			t.Errorf("NewBase with %q = %v, want it accepted", valid, err)
		}
	}
	// Numbers and booleans have no string to check.
	if _, err := datalog.NewBase(testSchema, fact("score", "a", 1), fact("flag", "a", true)); err != nil {
		t.Errorf("NewBase with a number and a boolean = %v", err)
	}
}

// The lexer refuses a string literal that is not valid UTF-8, written raw or by escape,
// and reads valid ones whole.
func TestParse_InvalidUTF8StringLiteral(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{
		"raw byte":        "a(\"\xff\").",
		"raw lone lead":   "a(\"x\xc3\").",
		"hex escape":      `a("\xff").`,
		"overlong escape": `a("\xc0\xaf").`,
	} {
		if _, err := datalog.Parse(text); !errors.Is(err, datalog.ErrSyntax) || !strings.Contains(err.Error(), "UTF-8") {
			t.Errorf("%s: Parse = %v, want ErrSyntax naming UTF-8", name, err)
		}
	}
	for _, text := range []string{`a("é").`, `a("é").`, `a("�").`, "a(\"\xef\xbf\xbd\")."} {
		if _, err := datalog.Parse(text); err != nil {
			t.Errorf("Parse(%q) = %v, want it accepted", text, err)
		}
	}
}
