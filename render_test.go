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
