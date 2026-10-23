package datalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// These tests hold the fixes from the red/blue file audit of 2026-10-22
// (docs/reviews/file-red-blue-2026-10-22). Each names its finding.

// red10/blue13: an Answer is a value. What marshals is Bytes, and what is read back
// must be an answer as written: sorted, no repeats, one width, a relation name.
func TestAnswer_MarshalsAsBytesAndReadsBackOnlyAnAnswer(t *testing.T) {
	t.Parallel()
	a, err := tryRun(newBase(t, fact("edge", "b", "c"), fact("edge", "a", "b")), `e(X, Y) :- edge(X, Y).`, "e", datalog.Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	out, err := json.Marshal(a)
	if err != nil || string(out) != string(a.Bytes()) {
		t.Fatalf("Marshal = %s, %v; want Bytes %s", out, err, a.Bytes())
	}
	if a.Relation() != "e" || a.Len() != 2 || a.Tuples()[0][0].Text() != "a" {
		t.Fatalf("accessors: %s %d %v", a.Relation(), a.Len(), a.Tuples())
	}
	var back datalog.Answer
	if err := json.Unmarshal(out, &back); err != nil || back.Hash() != a.Hash() {
		t.Fatalf("round trip: %v, hash %s want %s", err, back.Hash(), a.Hash())
	}
	if _, err := json.Marshal(datalog.Answer{}); !errors.Is(err, datalog.ErrInvalid) {
		t.Fatalf("a zero Answer marshals to an error, got %v", err)
	}
	for _, bad := range []string{
		`{"relation":"e","tuples":[["b"],["a"]]}`,     // unsorted
		`{"relation":"e","tuples":[["a"],["a"]]}`,     // repeated
		`{"relation":"e","tuples":[["a"],["a","b"]]}`, // ragged
		`{"relation":"e","tuples":[[]]}`,              // empty tuple
		`{"relation":"e","tuples":[null]}`,            // null tuple
		`{"relation":"e","tuples":null}`,              // no tuples
		`{"relation":"not a name","tuples":[["a"]]}`,  // not a relation name
		`{"relation":"e\u0000x","tuples":[["a"]]}`,    // a NUL in the name
		`{"relation":"","tuples":[["a"]]}`,            // no name
	} {
		var b datalog.Answer
		if err := json.Unmarshal([]byte(bad), &b); !errors.Is(err, datalog.ErrSyntax) {
			t.Errorf("Unmarshal(%s) = %v, want ErrSyntax", bad, err)
		}
	}
}

// red01/blue09: a string literal reads as JSON or as Go: both are what the doc says.
func TestParse_StringEscapes(t *testing.T) {
	t.Parallel()
	for src, want := range map[string]string{
		`p("\/").`:                 "/",
		`p("😀").`:                  "😀",
		`p("é").`:                  "é",
		`p("\x41").`:               "A",
		`p("\U0001F600").`:         "😀",
		`p("a\tb\n").`:             "a\tb\n",
		`p("\"quoted\" \\ back").`: `"quoted" \ back`,
	} {
		p, err := datalog.Parse(src)
		if err != nil {
			t.Errorf("Parse(%s): %v", src, err)
			continue
		}
		if got := p.Rules[0].Head.Args[0].Const.Text(); got != want {
			t.Errorf("Parse(%s) = %q, want %q", src, got, want)
		}
	}
	for _, src := range []string{`p("\ud83d").`, `p("\ude00").`, `p("\q").`, `p("\xff").`, `p("\u12").`} {
		if _, err := datalog.Parse(src); !errors.Is(err, datalog.ErrSyntax) {
			t.Errorf("Parse(%s) = %v, want ErrSyntax", src, err)
		}
	}
}

// red06/blue19/red07/blue01: a program built by hand is held to what the parser
// would make, so a name holds no NUL, a relation has an argument, a variable is a
// variable, and a string is UTF-8.
func TestCompile_HandBuiltShape(t *testing.T) {
	t.Parallel()
	c := func(s string) datalog.Term { return datalog.Term{Const: datalog.String(s)} }
	v := func(n string) datalog.Term { return datalog.Term{Var: n} }
	rule := func(head datalog.Atom, body ...datalog.Literal) datalog.Program {
		return datalog.Program{Rules: []datalog.Rule{{Head: head, Body: body}}}
	}
	cases := map[string]datalog.Program{
		"NUL in a head name":        rule(datalog.Atom{Name: "a\x00b", Args: []datalog.Term{c("x")}}),
		"upper-case head name":      rule(datalog.Atom{Name: "Node", Args: []datalog.Term{c("x")}}),
		"zero-arity head":           rule(datalog.Atom{Name: "p"}),
		"zero-arity body atom":      rule(datalog.Atom{Name: "p", Args: []datalog.Term{v("X")}}, datalog.Literal{Atom: &datalog.Atom{Name: "node", Args: []datalog.Term{v("X")}}}, datalog.Literal{Atom: &datalog.Atom{Name: "q"}}),
		"invalid UTF-8 in the head": rule(datalog.Atom{Name: "p", Args: []datalog.Term{c("\xff")}}),
		"invalid UTF-8 in the body": rule(datalog.Atom{Name: "p", Args: []datalog.Term{v("X")}}, datalog.Literal{Atom: &datalog.Atom{Name: "node", Args: []datalog.Term{v("X")}}}, datalog.Literal{Compare: &datalog.Comparison{Op: "=", Left: v("X"), Right: c("\xff")}}),
		"lower-case variable":       rule(datalog.Atom{Name: "p", Args: []datalog.Term{v("x")}}, datalog.Literal{Atom: &datalog.Atom{Name: "node", Args: []datalog.Term{v("x")}}}),
		"declared NUL name":         {Declared: map[string]int{"a\x00b": 1}},
	}
	for name, prog := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := datalog.Compile(testSchema, prog); !errors.Is(err, datalog.ErrInvalid) {
				t.Fatalf("Compile = %v, want ErrInvalid", err)
			}
		})
	}
	bad := datalog.Schema{Arities: map[string]int{"a\x00b": 1}}
	if _, err := datalog.Compile(bad); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("a schema with a NUL in a name: %v, want ErrInvalid", err)
	}
	if _, err := datalog.NewBase(bad); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("a base with a NUL in a name: %v, want ErrInvalid", err)
	}
}

// red16/blue18: a schema check sees a copy of the atom; keeping and changing it
// changes nothing the plan derives.
func TestCompile_CheckSeesACopy(t *testing.T) {
	t.Parallel()
	var kept []datalog.Term
	schema := datalog.Schema{
		Arities: map[string]int{"node": 1},
		Checks:  map[string]func(datalog.Atom) error{"node": func(a datalog.Atom) error { kept = a.Args; return nil }},
	}
	plan, err := datalog.Compile(schema, parse(t, `out("yes") :- node("allowed").`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := datalog.NewBase(schema, fact("node", "other"))
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	kept[0] = datalog.Term{Const: datalog.String("other")}
	res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if out, _ := res.Relation("out"); out.Len() != 0 {
		t.Fatal("the check's atom was the plan's own; changing it changed the rule")
	}
}

// blue02: what a check approved is what is stored, though the check changed the
// caller's tuple under it.
func TestNewBase_StoresWhatWasChecked(t *testing.T) {
	t.Parallel()
	tuple := datalog.Tuple{datalog.String("safe")}
	schema := datalog.Schema{
		Arities: map[string]int{"node": 1},
		Checks: map[string]func(datalog.Atom) error{"node": func(datalog.Atom) error {
			tuple[0] = datalog.String("\xff")
			return nil
		}},
	}
	for _, build := range map[string]func() (datalog.Base, error){
		"NewBase": func() (datalog.Base, error) {
			return datalog.NewBase(schema, datalog.Fact{Relation: "node", Tuple: tuple})
		},
		"With": func() (datalog.Base, error) {
			b, err := datalog.NewBase(schema)
			if err != nil {
				return b, err
			}
			return b.With(datalog.Fact{Relation: "node", Tuple: tuple})
		},
	} {
		tuple[0] = datalog.String("safe")
		b, err := build()
		if err != nil {
			t.Fatalf("build: %v", err)
		}
		res, err := datalog.Evaluate(context.Background(), datalog.Plan{}, b, datalog.Options{})
		if err != nil {
			t.Fatalf("Evaluate: %v", err)
		}
		if a, _ := res.Relation("node"); a.Tuples()[0][0].Text() != "safe" {
			t.Fatalf("stored %q after checking \"safe\"", a.Tuples()[0][0].Text())
		}
	}
}

// red12/blue04: Number reads what JSON writes and nothing wider; a constant is one
// JSON value.
func TestNumber_JSONGrammar(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"+1", "01", ".5", "1.", "0x1p0", "1_000", "1e", "-", ""} {
		if _, err := datalog.Number(bad); !errors.Is(err, datalog.ErrSyntax) {
			t.Errorf("Number(%q) = %v, want ErrSyntax", bad, err)
		}
	}
	for _, good := range []string{"0", "-0", "1", "1.5", "1e3", "1E+3", "-2.5e-3", "123456789"} {
		if _, err := datalog.Number(good); err != nil {
			t.Errorf("Number(%q) = %v", good, err)
		}
	}
	var c datalog.Const
	for _, bad := range []string{"1 2", "true garbage", `"a" "b"`} {
		if err := c.UnmarshalJSON([]byte(bad)); !errors.Is(err, datalog.ErrSyntax) {
			t.Errorf("UnmarshalJSON(%q) = %v, want ErrSyntax", bad, err)
		}
	}
}

// red04/blue11: the constructor and Verify hold a set's identity to one rule, and a
// set with no text is refused where it is made, not where it is verified.
func TestRuleSet_IdentityHeldByVerify(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"bad\vname", "bad\fname", "bad name", "bad name", "bad\x00name", "a/b", "a;b", ""} {
		if _, err := datalog.NewRuleSet(name, 1, `p("x").`); !errors.Is(err, datalog.ErrInvalid) {
			t.Errorf("NewRuleSet(%q) = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := datalog.NewRuleSet("empty", 0, ""); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("NewRuleSet with no text = %v, want ErrInvalid", err)
	}
	s, err := datalog.NewRuleSet("ok", 1, `p("x").`)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	for _, edit := range []func(*datalog.RuleSet){
		func(s *datalog.RuleSet) { s.Name = "" },
		func(s *datalog.RuleSet) { s.Name = "a/b" },
		func(s *datalog.RuleSet) { s.Name = "a;b" },
		func(s *datalog.RuleSet) { s.Name = "a\nb" },
		func(s *datalog.RuleSet) { s.Version = -1 },
	} {
		edited := s
		edited.Program = s.Program.Clone()
		edit(&edited)
		edited.Program.Name = edited.Name + "/" + strings.TrimLeft(string(rune('0'+edited.Version%10)), "")
		if edited.Version < 0 {
			edited.Program.Name = edited.Name + "/-1"
		}
		if err := edited.Verify(); !errors.Is(err, datalog.ErrInvalid) {
			t.Errorf("Verify of a set with identity %q/%d = %v, want ErrInvalid", edited.Name, edited.Version, err)
		}
	}
}

// red14/blue12: a why-not walk over a derived relation scans it, since a derived
// relation has no index, and the scan is charged to the walk's budget.
func TestWhyNot_ScanIsCharged(t *testing.T) {
	t.Parallel()
	const n = 70000 // over whyNotSteps
	facts := make([]datalog.Fact, n)
	for i := range facts {
		facts[i] = fact("node", "n"+strings.Repeat("x", i%7)+string(rune('a'+i%26))+itoa(i))
	}
	plan, err := datalog.Compile(testSchema, parse(t, `big(X) :- node(X). q("m") :- big("missing").`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, facts...), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	a, ok := res.WhyNot("q", tuple("m"), 0)
	if !ok {
		t.Fatal("q(\"m\") should be absent")
	}
	if !a.Truncated {
		t.Fatalf("a scan of %d derived tuples is over the budget and must truncate the explanation: %s", n, a)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for ; i > 0; i /= 10 {
		b = append([]byte{byte('0' + i%10)}, b...)
	}
	return string(b)
}
