package datalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

const demoRules = `up(X) :- node(X), not edge(X, _).
top(X) :- up(X).
`

func demoResult(t *testing.T) datalog.Result {
	t.Helper()
	set, err := datalog.NewRuleSet("demo", 1, demoRules)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	plan, err := datalog.Compile(testSchema, set.Program)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a"), fact("node", "b"), fact("edge", "b", "c")),
		datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return res
}

// 8. The renderers' text, written out.
func TestRender_Strings(t *testing.T) {
	t.Parallel()
	res := demoResult(t)
	proof, _ := res.Proof("top", tuple("a"))
	wantProof := `top("a")  demo/1:2  top(X) :- up(X).
  up("a")  demo/1:1  up(X) :- node(X), not edge(X, _).
    node("a")  fact
`
	if got := proof.String(); got != wantProof {
		t.Errorf("Proof.String:\n%s\nwant:\n%s", got, wantProof)
	}
	wantAbsence := `top("b") is not derived
  demo/1:2  top(X) :- up(X).
    stopped at up(X)  with X="b"
    because
      up("b") is not derived
        demo/1:1  up(X) :- node(X), not edge(X, _).
          stopped at not edge(X, _)  with X="b"
          blocked by
            edge("b", "c")  fact
`
	if got := whyNot(t, res, "top", tuple("b"), 0).String(); got != wantAbsence {
		t.Errorf("Absence.String:\n%s\nwant:\n%s", got, wantAbsence)
	}
	if got, want := whyNot(t, res, "edge", tuple("q", "r"), 0).String(), "edge(\"q\", \"r\") is not derived: no such fact\n"; got != want {
		t.Errorf("a base absence = %q, want %q", got, want)
	}
}

// A tuple shown once is not expanded again.
func TestRender_SharedSubproofIsNotRepeated(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(testSchema, parse(t, `l(X) :- node(X). both(X) :- l(X), l(X).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a")), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	p, _ := res.Proof("both", tuple("a"))
	if got := p.String(); strings.Count(got, "node(\"a\")") != 1 || !strings.Contains(got, `l("a")  (shown above)`) {
		t.Errorf("shared proof printed:\n%s", got)
	}
}

// MarshalJSON writes the documented keys and never the parser's AST.
func TestRender_JSON(t *testing.T) {
	t.Parallel()
	res := demoResult(t)
	proof, _ := res.Proof("top", tuple("a"))
	absence := whyNot(t, res, "top", tuple("b"), 0)
	allowed := map[string]bool{
		"relation": true, "tuple": true, "rule": true, "body": true, "set": true, "line": true, "text": true,
		"goal": true, "truncated": true, "rules": true, "bound": true, "failed": true, "because": true, "blocked": true,
	}
	for name, v := range map[string]json.Marshaler{"proof": proof, "absence": absence} {
		raw, err := v.MarshalJSON()
		if err != nil {
			t.Fatalf("%s: MarshalJSON: %v", name, err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatalf("%s: not JSON: %v\n%s", name, err, raw)
		}
		checkKeys(t, name, doc, allowed)
	}
	raw, _ := proof.MarshalJSON()
	want := `{"relation":"top","tuple":["a"],"rule":{"set":"demo/1","line":2,"text":"top(X) :- up(X)."},"body":[{"relation":"up","tuple":["a"],` +
		`"rule":{"set":"demo/1","line":1,"text":"up(X) :- node(X), not edge(X, _)."},"body":[{"relation":"node","tuple":["a"],"rule":null}]}]}`
	if string(raw) != want {
		t.Errorf("proof JSON:\n%s\nwant:\n%s", raw, want)
	}
	var doc map[string]any
	raw, _ = absence.MarshalJSON()
	_ = json.Unmarshal(raw, &doc)
	rules := doc["rules"].([]any)
	first := rules[0].(map[string]any)
	if doc["relation"] != "top" || doc["truncated"] != false || first["failed"] != "up(X)" || first["bound"].(map[string]any)["X"] != "b" ||
		len(first["because"].([]any)) != 1 || doc["goal"].([]any)[0] != "b" {
		t.Errorf("absence JSON: %s", raw)
	}
	open := datalog.Absence{Relation: "r", Goal: []datalog.Term{{Var: "X"}}}
	if raw, _ := open.MarshalJSON(); string(raw) != `{"relation":"r","goal":[null],"truncated":false,"rules":[]}` {
		t.Errorf("an open goal position = %s, want null", raw)
	}
}

// checkKeys walks a decoded document and holds every object key to the allowed set,
// except the variable names under "bound".
func checkKeys(t *testing.T, path string, v any, allowed map[string]bool) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		for k, sub := range x {
			if !allowed[k] {
				t.Errorf("%s has the key %q, which is no part of the format", path, k)
			}
			if k != "bound" {
				checkKeys(t, path+"."+k, sub, allowed)
			}
		}
	case []any:
		for i, sub := range x {
			checkKeys(t, fmt.Sprintf("%s[%d]", path, i), sub, allowed)
		}
	}
}

// A proof shares its sub-proofs, and the JSON must stay linear in the derivation: a
// chain where each rule reads the one below twice would otherwise double at every level.
func TestRender_SharedProofJSONIsLinear(t *testing.T) {
	const depth = 16
	var rules strings.Builder
	rules.WriteString("p0(X) :- node(X).\n")
	for i := 1; i <= depth; i++ {
		fmt.Fprintf(&rules, "p%d(X) :- p%d(X), p%d(X).\n", i, i-1, i-1)
	}
	plan, err := datalog.Compile(testSchema, parse(t, rules.String()))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a")), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	proof, ok := res.Proof(fmt.Sprintf("p%d", depth), tuple("a"))
	if !ok {
		t.Fatal("missing proof")
	}
	out, err := json.Marshal(proof)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if len(out) > 8192 {
		t.Fatalf("JSON for a %d-deep shared chain is %d bytes; it must be linear in the derivation", depth, len(out))
	}
	if n := strings.Count(string(out), `"shown":true`); n != depth {
		t.Fatalf("want %d references to a proof already written, got %d:\n%s", depth, n, out)
	}
	var back struct {
		Body []struct {
			Relation string `json:"relation"`
			Rule     *struct{}
			Shown    bool `json:"shown"`
		} `json:"body"`
	}
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(back.Body) != 2 || back.Body[0].Shown || !back.Body[1].Shown || back.Body[1].Rule != nil {
		t.Fatalf("the second read of p%d must be a reference with no rule and no body: %+v", depth-1, back.Body)
	}
}

// Whitespace inside a quoted constant is the constant's; only the whitespace between
// tokens is collapsed, and a comment is dropped.
func TestRender_RuleTextKeepsConstants(t *testing.T) {
	plan, err := datalog.Compile(testSchema, parse(t, "p(\"a  b\")  :-\n\tnode(\"a  b\"). % two spaces\n"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a  b")), datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	proof, ok := res.Proof("p", tuple("a  b"))
	if !ok {
		t.Fatal("missing proof")
	}
	got := proof.String()
	if !strings.Contains(got, `line 1  p("a  b") :- node("a  b").`) || strings.Contains(got, "%") {
		t.Fatalf("rule text altered:\n%s", got)
	}
}

// A RuleFailure with no rule is not something the engine makes, but it is a public
// value and String must not panic on it; nor must a cycle a caller built run forever.
func TestRender_HandBuiltAbsence(t *testing.T) {
	a := datalog.Absence{Relation: "p", Rules: []datalog.RuleFailure{{}}}
	if got := a.String(); !strings.Contains(got, "(no rule)") {
		t.Fatalf("want a marker for the missing rule, got %q", got)
	}
	if _, err := json.Marshal(a); err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	cyclic := datalog.Absence{Relation: "p", Rules: make([]datalog.RuleFailure, 1)}
	cyclic.Rules[0].Because = []datalog.Absence{cyclic}
	cyclic.Rules[0].Because[0].Rules = cyclic.Rules // the Because points back at its own rules
	if got := cyclic.String(); !strings.Contains(got, "(explanation truncated)") {
		t.Fatal("a cyclic absence must end in truncation")
	}
	out, err := json.Marshal(cyclic)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(out), `"truncated":true`) {
		t.Fatal("a cyclic absence must end in truncation in JSON too")
	}
	loop := datalog.Proof{Relation: "p", Body: make([]datalog.Proof, 1)}
	loop.Body[0] = loop
	loop.Body[0].Body = loop.Body // the body contains itself
	if _, err := json.Marshal(loop); err != nil {
		t.Fatalf("Marshal of a cyclic proof: %v", err)
	}
}
