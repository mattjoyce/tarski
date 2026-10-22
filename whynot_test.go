package datalog_test

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// rbacSchema and rbacText are the shape of policy/rbac.dl, so the why-not walk is held
// to the case it exists for without datalog importing policy.
var rbacSchema = datalog.Schema{Arities: map[string]int{
	"principal": 1, "action": 1, "resource": 1, "role": 2, "inherits": 2, "grants": 3, "denied": 3,
}}

const rbacText = `
deny(P, A, R) :- principal(P), action(A), resource(R), denied(P, A, R).
holds(P, R) :- principal(P), role(P, R).
holds(P, R) :- holds(P, Q), inherits(Q, R).
permit(P, A, Pat) :- principal(P), action(A), holds(P, Role), grants(Role, A, Pat).
allow(P, A, R) :- permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R).
`

// rbacResult evaluates the rbac-shaped rules, with proofs on, over a request for tok to
// read timeline/a and the given standing facts.
func rbacResult(t *testing.T, standing ...datalog.Fact) datalog.Result {
	t.Helper()
	set, err := datalog.NewRuleSet("rbac", 1, rbacText)
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	plan, err := datalog.Compile(rbacSchema, set.Program)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	facts := append([]datalog.Fact{fact("principal", "tok"), fact("action", "read"), fact("resource", "timeline/a")}, standing...)
	base, err := datalog.NewBase(rbacSchema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return res
}

func tuple(vs ...string) datalog.Tuple {
	t := make(datalog.Tuple, len(vs))
	for i, v := range vs {
		t[i] = datalog.String(v)
	}
	return t
}

var allowGoal = tuple("tok", "read", "timeline/a")

func whyNot(t *testing.T, res datalog.Result, name string, tup datalog.Tuple, depth int) datalog.Absence {
	t.Helper()
	a, ok := res.WhyNot(name, tup, depth)
	if !ok {
		t.Fatalf("WhyNot(%s) was not ok", name)
	}
	return a
}

// failed is a rule failure's literal as text, through the renderer's own form.
func failed(rf datalog.RuleFailure) string {
	rf.Rule = &datalog.Rule{}
	a := datalog.Absence{Relation: "x", Rules: []datalog.RuleFailure{rf}}
	for _, line := range strings.Split(a.String(), "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "stopped at "); ok {
			if i := strings.Index(rest, "  with "); i >= 0 {
				rest = rest[:i]
			}
			return rest
		}
	}
	return ""
}

// 1. No role fact: allow stops at permit, permit at holds, and holds at role, which is a
// base relation with nothing to say but that there is no such fact.
func TestWhyNot_NoRole(t *testing.T) {
	t.Parallel()
	a := whyNot(t, rbacResult(t), "allow", allowGoal, 0)
	if len(a.Rules) != 1 || failed(a.Rules[0]) != "permit(P, A, Pat)" || a.Truncated {
		t.Fatalf("allow: %d rules, failed %q, truncated %v", len(a.Rules), failed(a.Rules[0]), a.Truncated)
	}
	permit := a.Rules[0].Because[0]
	if permit.Relation != "permit" || failed(permit.Rules[0]) != "holds(P, Role)" || permit.Goal[2].IsVar() != true {
		t.Fatalf("permit absence = %s", permit)
	}
	holds := permit.Rules[0].Because[0]
	if holds.Relation != "holds" || len(holds.Rules) != 2 || holds.Goal[0].IsVar() || !holds.Goal[1].IsVar() {
		t.Fatalf("holds absence = %s", holds)
	}
	if got := failed(holds.Rules[0]); got != "role(P, R)" {
		t.Errorf("holds rule 1 stopped at %q, want role(P, R)", got)
	}
	role := holds.Rules[0].Because[0]
	if role.Relation != "role" || len(role.Rules) != 0 || role.Goal[0].IsVar() || !role.Goal[1].IsVar() {
		t.Errorf("role absence = %+v, want a base relation, goal [tok, open]", role)
	}
	if got := failed(holds.Rules[1]); got != "holds(P, Q)" || len(holds.Rules[1].Because) != 0 {
		t.Errorf("holds rule 2 stopped at %q with %d because, want the revisit left unexplained", got, len(holds.Rules[1].Because))
	}
	// One level deep is not enough to reach role: the cut is reported.
	if shallow := whyNot(t, rbacResult(t), "allow", allowGoal, 1); !shallow.Truncated || !shallow.Rules[0].Because[0].Truncated {
		t.Errorf("a depth of 1 should truncate the walk: %s", shallow)
	}
}

// 2. A role with no grant for the action: permit stops at grants, with Role bound.
func TestWhyNot_NoGrant(t *testing.T) {
	t.Parallel()
	a := whyNot(t, rbacResult(t, fact("role", "tok", "reader"), fact("grants", "reader", "write", "timeline/")), "allow", allowGoal, 0)
	permit := a.Rules[0].Because[0]
	rf := permit.Rules[0]
	if failed(rf) != "grants(Role, A, Pat)" || rf.Bound["Role"].Text() != "reader" || rf.Bound["A"].Text() != "read" {
		t.Errorf("permit stopped at %q with %v, want grants with Role=reader", failed(rf), rf.Bound)
	}
}

// 3. Grants that match no prefix: allow stops at prefix, once, though permit matched two
// tuples; Pat shows the first path's binding.
func TestWhyNot_NoPrefixMatch(t *testing.T) {
	t.Parallel()
	res := rbacResult(t, fact("role", "tok", "reader"), fact("grants", "reader", "read", "other/"), fact("grants", "reader", "read", "zzz/"))
	a := whyNot(t, res, "allow", allowGoal, 0)
	if len(a.Rules) != 1 {
		t.Fatalf("%d rule failures, want one", len(a.Rules))
	}
	rf := a.Rules[0]
	if failed(rf) != "prefix(Pat, R)" || rf.Bound["Pat"].Text() != "other/" || rf.Bound["R"].Text() != "timeline/a" || len(rf.Because) != 0 {
		t.Errorf("allow stopped at %q with %v, because %d; want prefix with Pat=other/", failed(rf), rf.Bound, len(rf.Because))
	}
	if _, ok := res.Proof("permit", tuple("tok", "read", "zzz/")); !ok {
		t.Error("permit should have derived both grants, so the single failure above is the first path's")
	}
}

// 4. An explicit deny: the failing literal is the negation that held, with the proof of
// the deny tuple, whose leaves include the denied fact.
func TestWhyNot_Denied(t *testing.T) {
	t.Parallel()
	res := rbacResult(t, fact("role", "tok", "reader"), fact("grants", "reader", "read", "timeline/"), fact("denied", "tok", "read", "timeline/a"))
	a := whyNot(t, res, "allow", allowGoal, 0)
	rf := a.Rules[0]
	if failed(rf) != "not deny(P, A, R)" || rf.Blocked == nil || rf.Blocked.Relation != "deny" {
		t.Fatalf("allow stopped at %q, blocked %+v; want not deny with a proof", failed(rf), rf.Blocked)
	}
	var leaf bool
	for _, b := range rf.Blocked.Body {
		leaf = leaf || (b.Relation == "denied" && b.Rule == nil)
	}
	if !leaf {
		t.Errorf("the blocking proof's body lacks the denied fact: %s", rf.Blocked)
	}
}

// 5. Recursion through holds ends without truncation at the default depth, with a long
// inheritance chain behind it.
func TestWhyNot_RecursionTerminates(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{fact("role", "tok", "r0")}
	for i := 0; i < 12; i++ {
		facts = append(facts, fact("inherits", fmt.Sprint("r", i), fmt.Sprint("r", i+1)))
	}
	res := rbacResult(t, facts...)
	a := whyNot(t, res, "holds", tuple("tok", "nothing"), 0)
	if a.Truncated || len(a.Rules) != 2 {
		t.Fatalf("holds absence truncated %v with %d rules, want two and no truncation", a.Truncated, len(a.Rules))
	}
	if got := failed(a.Rules[1]); got != "inherits(Q, R)" || a.Rules[1].Bound["Q"].Text() != "r0" {
		t.Errorf("rule 2 stopped at %q with %v, want inherits with Q=r0", got, a.Rules[1].Bound)
	}
}

// 6. A walk wider than the step limit stops, says so, and still returns what it found.
func TestWhyNot_StepLimit(t *testing.T) {
	t.Parallel()
	schema := datalog.Schema{Arities: map[string]int{"big": 1, "tag": 1}}
	var facts []datalog.Fact
	for i := 0; i < 45; i++ {
		facts = append(facts, datalog.Fact{Relation: "big", Tuple: tuple(fmt.Sprint("n", i))})
	}
	facts = append(facts, datalog.Fact{Relation: "tag", Tuple: tuple("t")})
	plan, err := datalog.Compile(schema, parse(t, `z(X) :- big(A), big(B), big(C), tag(X), X = "never".`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := datalog.NewBase(schema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, base, datalog.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	a := whyNot(t, res, "z", tuple("zzz"), 0)
	if !a.Truncated || len(a.Rules) != 1 {
		t.Errorf("a walk of ~90,000 candidates: truncated %v, %d rules; want truncated, one rule", a.Truncated, len(a.Rules))
	}
}

// 7. The same question twice gets the same answer, tuple for tuple and rule for rule.
func TestWhyNot_Deterministic(t *testing.T) {
	t.Parallel()
	res := rbacResult(t, fact("role", "tok", "reader"), fact("grants", "reader", "read", "other/"), fact("grants", "reader", "read", "zzz/"),
		fact("inherits", "reader", "viewer"))
	first := whyNot(t, res, "allow", allowGoal, 0)
	for i := 0; i < 20; i++ {
		if again := whyNot(t, res, "allow", allowGoal, 0); !reflect.DeepEqual(first, again) {
			t.Fatalf("run %d: the absence changed:\n%s\nvs\n%s", i, first, again)
		}
	}
}

// WhyNot answers only about a tuple that is absent, from a result that kept proofs.
func TestWhyNot_Refusals(t *testing.T) {
	t.Parallel()
	res := rbacResult(t, fact("role", "tok", "reader"))
	for name, c := range map[string]struct {
		rel string
		tup datalog.Tuple
	}{
		"present":        {"holds", tuple("tok", "reader")},
		"unknown":        {"nothing", tuple("a")},
		"wrong arity":    {"holds", tuple("tok")},
		"present (base)": {"role", tuple("tok", "reader")},
	} {
		if _, ok := res.WhyNot(c.rel, c.tup, 0); ok {
			t.Errorf("%s: WhyNot was ok", name)
		}
	}
	plan, _ := datalog.Compile(testSchema, parse(t, `n(X) :- node(X).`))
	off, err := datalog.Evaluate(context.Background(), plan, newBase(t, fact("node", "a")), datalog.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if _, ok := off.WhyNot("n", tuple("b"), 0); ok {
		t.Error("WhyNot was ok without Options.Proofs")
	}
}

// A Result may be asked about from many goroutines at once: the walk reads the derived
// relations by scanning, never by building an index inside them.
func TestWhyNot_ConcurrentOverOneResult(t *testing.T) {
	t.Parallel()
	res := rbacResult(t, fact("role", "tok", "reader"), fact("grants", "reader", "read", "other/"))
	want := whyNot(t, res, "allow", allowGoal, 0).String()
	var wg sync.WaitGroup
	got := make([]string, 8)
	for i := range got {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if a, ok := res.WhyNot("allow", allowGoal, 0); ok {
				got[i] = a.String()
			}
		}()
	}
	wg.Wait()
	for i, g := range got {
		if g != want {
			t.Errorf("goroutine %d got a different explanation", i)
		}
	}
}
