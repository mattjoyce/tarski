package tarski_test

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/tarski"
)

// rbacRules is Shearline's rbac rules without the request seeds: unseeded, full
// evaluation derives every principal's roles, and demand only the asking one's.
const rbacRules = `holds(P, R) :- role(P, R).
holds(P, R) :- holds(P, Q), inherits(Q, R).
permit(P, A, Pat) :- holds(P, Role), grants(Role, A, Pat).
deny(P, A, R) :- denied(P, A, R).
allow(P, A, R) :- permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R).
`

var rbacTestSchema = tarski.Schema{Arities: map[string]int{"role": 2, "inherits": 2, "grants": 3, "denied": 3, "resource": 1}}

// rbacBase has principals p0..p(n-1), each holding r(i%3), a chain r0 -> r1 -> r2, a
// read grant on /doc/ for r2 and a write grant on /doc/a for r0, two resources, and
// p1 denied read on /doc/b.
func rbacBase(t testing.TB, n int) tarski.Base {
	t.Helper()
	s := func(vs ...string) tarski.Tuple {
		out := make(tarski.Tuple, len(vs))
		for i, v := range vs {
			out[i] = tarski.String(v)
		}
		return out
	}
	facts := []tarski.Fact{
		{Relation: "inherits", Tuple: s("r0", "r1")}, {Relation: "inherits", Tuple: s("r1", "r2")},
		{Relation: "grants", Tuple: s("r2", "read", "/doc/")}, {Relation: "grants", Tuple: s("r0", "write", "/doc/a")},
		{Relation: "resource", Tuple: s("/doc/a")}, {Relation: "resource", Tuple: s("/doc/b")},
		{Relation: "denied", Tuple: s("p1", "read", "/doc/b")},
	}
	for i := range n {
		facts = append(facts, tarski.Fact{Relation: "role", Tuple: s("p"+strconv.Itoa(i), "r"+strconv.Itoa(i%3))})
	}
	b, err := tarski.NewBase(rbacTestSchema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// rbacPlans is the rbac plan and its demand plan for allow/bbb.
func rbacPlans(t testing.TB) (plan, demand tarski.Plan) {
	t.Helper()
	prog, err := tarski.Parse(rbacRules)
	if err != nil {
		t.Fatal(err)
	}
	plan, err = tarski.Compile(rbacTestSchema, prog)
	if err != nil {
		t.Fatal(err)
	}
	demand, err = plan.Demand("allow", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	return plan, demand
}

// strGoal is a goal over relation: a "?"-prefixed argument is a variable, any other a
// string constant.
func strGoal(relation string, args ...string) *tarski.Goal {
	g := &tarski.Goal{Relation: relation}
	for _, a := range args {
		if v, ok := strings.CutPrefix(a, "?"); ok {
			g.Args = append(g.Args, tarski.Term{Var: v})
			continue
		}
		g.Args = append(g.Args, tarski.Term{Const: tarski.String(a)})
	}
	return g
}

func TestDemand_Refuses(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	agg, err := tarski.Compile(rbacTestSchema, parse(t, `n(P, N) :- role(P, _), N = count : { role(P, _) }.`))
	if err != nil {
		t.Fatal(err)
	}
	// The demand on r0 comes from r1, which r1's rule reads before it negates r0: the
	// rewrite's cycle m?r0?bb -> r1 -> not r0 -> m?r0?bb runs through a negation.
	unstrat, err := tarski.Compile(tarski.Schema{Arities: map[string]int{"e": 2}}, parse(t,
		`r0(X, Y) :- e(X, Y). r1(X, Y) :- e(Y, X). r1(X, Y) :- r1(X, Z), e(Z, Y), not r0(X, Y).`))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name                string
		plan                tarski.Plan
		relation, adornment string
		want                string
	}{
		{"unknown relation", plan, "nope", "b", "unknown relation nope"},
		{"base relation", plan, "role", "bf", "base relation"},
		{"adornment too short", plan, "allow", "bb", "adornment"},
		{"adornment letter", plan, "allow", "bbx", "adornment"},
		{"already for a goal", demand, "allow", "bbb", "already compiled"},
		{"aggregates", agg, "n", "bf", "not yet supported"},
		{"rewrite unstratified", unstrat, "r1", "bf", "not yet supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.plan.Demand(tc.relation, tc.adornment)
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Demand(%s, %s) = %v, want ErrInvalid naming %q", tc.relation, tc.adornment, err, tc.want)
			}
		})
	}
}

func TestEvaluate_GoalMustFitThePlan(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	base := rbacBase(t, 3)
	open, err := plan.Demand("permit", "bff")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		plan tarski.Plan
		goal *tarski.Goal
		want string
	}{
		{"goal on a plain plan", plan, strGoal("allow", "p0", "read", "/doc/a"), "Plan.Demand"},
		{"no goal on a demand plan", demand, nil, "set Options.Goal"},
		{"another relation", demand, strGoal("permit", "p0", "read", "/doc/a"), "permit/3"},
		{"wrong arity", demand, strGoal("allow", "p0", "read"), "allow/2"},
		{"open where bound", demand, strGoal("allow", "p0", "?A", "/doc/a"), "leaves position 2 open"},
		{"bound where open", open, strGoal("permit", "p0", "read", "?X"), "binds position 2"},
		{"repeated variable", open, strGoal("permit", "p0", "?X", "?X"), "repeats the variable X"},
		{"bad utf-8", demand, strGoal("allow", "p0", "read", "\xff"), "UTF-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tarski.Evaluate(context.Background(), tc.plan, base, tarski.Options{Goal: tc.goal})
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Evaluate = %v, want ErrInvalid naming %q", err, tc.want)
			}
		})
	}
	// "_" leaves positions open as often as it is written.
	if _, err := tarski.Evaluate(context.Background(), open, base, tarski.Options{Goal: strGoal("permit", "p0", "?_", "?_")}); err != nil {
		t.Errorf("a goal with _ twice: %v", err)
	}
}

// A demand Result answers the goal, the base, proofs and why-not inside the goal
// (design §3.2-3.3, §8 Q2 and Q9)...
func TestDemand_ResultAnswersTheGoal(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	base := rbacBase(t, 6)
	full, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"p0", "read", "/doc/b"}, {"p1", "read", "/doc/b"}, {"p2", "read", "/doc/a"}, {"zz", "read", "/doc/a"}} {
		goal := strGoal("allow", args...)
		res, err := tarski.Evaluate(context.Background(), demand, base, tarski.Options{Proofs: true, Goal: goal})
		if err != nil {
			t.Fatal(err)
		}
		got, err := res.Relation("allow")
		if err != nil {
			t.Fatal(err)
		}
		want := filtered(t, full, *goal)
		if got.Hash() != want.Hash() {
			t.Errorf("%v: allow = %s, want %s", args, got.Bytes(), want.Bytes())
		}
		tup := goalTuple(goal)
		_, inFull := full.Proof("allow", tup)
		p, ok := res.Proof("allow", tup)
		if ok != inFull || ok && (p.Rule == nil || !strings.HasPrefix(p.Rule.Text, "allow(P, A, R) :-") || len(p.Body) != 2) {
			t.Errorf("%v: Proof = %+v, %v; full has it %v", args, p, ok, inFull)
		}
		if _, ok := res.WhyNot("allow", tup, 0); ok == inFull {
			t.Errorf("%v: WhyNot ok = %v with the tuple derived = %v", args, ok, inFull)
		}
		g, ok := res.Goal()
		if !ok || !reflect.DeepEqual(g, *goal) {
			t.Errorf("Goal() = %v, %v", g, ok)
		}
		g.Args[0].Var = "changed"
		if again, _ := res.Goal(); again.Args[0].Var != "" {
			t.Error("Goal() shares its Args with the result")
		}
	}
}

// ...and refuses everything else loudly.
func TestDemand_ResultRefusesOutsideTheGoal(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	base := rbacBase(t, 6)
	full, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), demand, base, tarski.Options{Proofs: true, Goal: strGoal("allow", "p0", "read", "/doc/b")})
	if err != nil {
		t.Fatal(err)
	}
	refusals := []struct {
		name string
		do   func() bool
	}{
		{"Relation of a non-goal derived relation", func() bool {
			_, err := res.Relation("holds")
			return errors.Is(err, tarski.ErrInvalid) && strings.Contains(err.Error(), "evaluate without a goal") &&
				strings.Contains(err.Error(), `allow("p0", "read", "/doc/b")`)
		}},
		{"Relation of a hidden name", func() bool {
			_, err := res.Relation("m?holds?bf")
			return errors.Is(err, tarski.ErrInvalid) && strings.Contains(err.Error(), "no relation")
		}},
		{"Proof outside the goal relation", func() bool { _, ok := res.Proof("holds", goalTuple(strGoal("h", "p0", "r0"))); return !ok }},
		{"Proof outside the goal's constants", func() bool { _, ok := res.Proof("allow", goalTuple(strGoal("a", "p3", "read", "/doc/b"))); return !ok }},
		{"WhyNot outside the goal", func() bool {
			_, ok := res.WhyNot("allow", goalTuple(strGoal("a", "p1", "read", "/doc/b")), 0)
			return !ok
		}},
		{"WhyNot of another relation", func() bool { _, ok := res.WhyNot("holds", goalTuple(strGoal("h", "p0", "zz")), 0); return !ok }},
		{"WhyEmpty on a bound goal", func() bool { _, ok := res.WhyEmpty("allow", 0); return !ok }},
		{"Goal of a full result", func() bool { _, ok := full.Goal(); return !ok }},
	}
	for _, tc := range refusals {
		if !tc.do() {
			t.Errorf("%s: answered", tc.name)
		}
	}
	for _, name := range []string{"role", "grants"} {
		a, _ := full.Relation(name)
		if b, err := res.Relation(name); err != nil || a.Hash() != b.Hash() {
			t.Errorf("base relation %s under demand: %v", name, err)
		}
	}
}

// goalTuple is the goal's constants as a tuple.
func goalTuple(g *tarski.Goal) tarski.Tuple {
	out := make(tarski.Tuple, len(g.Args))
	for i, a := range g.Args {
		out[i] = a.Const
	}
	return out
}

// With every position open the goal's relation is whole, so WhyEmpty explains it when
// it is empty, and Relation is full evaluation's.
func TestDemand_OpenGoal(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(rbacTestSchema, parse(t, rbacRules+`lonely(P) :- role(P, R), not inherits(R, _), grants(R, "none", _).`))
	if err != nil {
		t.Fatal(err)
	}
	demand, err := plan.Demand("lonely", "f")
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), demand, rbacBase(t, 6), tarski.Options{Proofs: true, Goal: strGoal("lonely", "?P")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := res.Relation("lonely")
	if err != nil || a.Len() != 0 {
		t.Fatalf("lonely = %v, %v; want empty", a, err)
	}
	why, ok := res.WhyEmpty("lonely", 0)
	if !ok || len(why.Rules) != 1 || why.Rules[0].Failed.Atom == nil || why.Rules[0].Failed.Atom.Name != "grants" {
		t.Errorf("WhyEmpty = %+v, %v; want the grants literal", why, ok)
	}
	if _, ok := res.WhyEmpty("holds", 0); ok {
		t.Error("WhyEmpty of a relation other than the goal's answered")
	}
}

// Demand derives only what reaches the goal: a step bound that full evaluation crosses
// over two thousand principals, a demand evaluation for one of them stays far inside.
func TestDemand_CostFollowsTheGoal(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	base := rbacBase(t, 2000)
	opts := tarski.Options{MaxSteps: 200, MaxBytes: 4096}
	if _, err := tarski.Evaluate(context.Background(), plan, base, opts); !errors.Is(err, tarski.ErrTooLarge) {
		t.Fatalf("full evaluation under %d steps = %v, want *TooLarge", opts.MaxSteps, err)
	}
	opts.Goal = strGoal("allow", "p1999", "read", "/doc/a")
	res, err := tarski.Evaluate(context.Background(), demand, base, opts)
	if err != nil {
		t.Fatalf("demand evaluation under %d steps: %v", opts.MaxSteps, err)
	}
	if a, _ := res.Relation("allow"); a.Len() != 1 {
		t.Errorf("allow = %s, want p1999's read of /doc/a", a.Bytes())
	}
}

// Bounds hold on a demand evaluation as on any: crossed is *TooLarge and no Result, and
// the relation named is one the caller wrote, never a hidden one (design D7, D10).
func TestDemand_Bounds(t *testing.T) {
	t.Parallel()
	_, demand := rbacPlans(t)
	base := rbacBase(t, 50)
	goal := strGoal("allow", "p2", "read", "/doc/a")
	for _, opts := range []tarski.Options{{MaxBytes: 1}, {MaxBytes: 40}, {MaxSteps: 1}, {MaxSteps: 3}, {MaxSteps: 6}} {
		opts.Goal, opts.Proofs = goal, true
		res, err := tarski.Evaluate(context.Background(), demand, base, opts)
		var tl *tarski.TooLarge
		if !errors.As(err, &tl) || !errors.Is(err, tarski.ErrTooLarge) {
			t.Fatalf("%+v: err = %v, want *TooLarge", opts, err)
		}
		if strings.Contains(tl.Relation, "?") || tl.Relation == "" || strings.Contains(err.Error(), "?") {
			t.Errorf("%+v: TooLarge names %q: %v", opts, tl.Relation, err)
		}
		if _, err := res.Relation("allow"); err == nil {
			t.Errorf("%+v: a partial Result came back with the error", opts)
		}
	}
	// The seed itself is charged: one byte is crossed at it, named as the goal's relation.
	_, err := tarski.Evaluate(context.Background(), demand, base, tarski.Options{MaxBytes: 1, Goal: goal})
	var tl *tarski.TooLarge
	if !errors.As(err, &tl) || tl.Relation != "allow" || tl.Tuples != 1 {
		t.Errorf("MaxBytes 1 = %v, want the seed charged as allow", err)
	}
}

// Same plan, base and goal: same answers and the same proofs, evaluated twice or by
// many goroutines at once on one shared plan and base.
func TestDemand_Deterministic(t *testing.T) {
	t.Parallel()
	_, demand := rbacPlans(t)
	base := rbacBase(t, 30)
	opts := tarski.Options{Proofs: true, Goal: strGoal("allow", "p5", "read", "/doc/b")}
	eval := func() (string, tarski.Proof) {
		res, err := tarski.Evaluate(context.Background(), demand, base, opts)
		if err != nil {
			t.Error(err)
			return "", tarski.Proof{}
		}
		a, _ := res.Relation("allow")
		p, _ := res.Proof("allow", goalTuple(opts.Goal))
		return a.Hash(), p
	}
	wantHash, wantProof := eval()
	if wantProof.Rule == nil {
		t.Fatal("p5 may read /doc/b: no proof")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			h, p := eval()
			if h != wantHash || !reflect.DeepEqual(p, wantProof) {
				t.Error("a concurrent demand evaluation differs")
			}
		})
	}
	wg.Wait()
}

// Demand returns a new plan and leaves its receiver as it was: the source plan still
// evaluates in full, and the demand plan answers Arity, Defines and HasAggregates as
// the source does.
func TestDemand_LeavesTheSourcePlan(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	res, err := tarski.Evaluate(context.Background(), plan, rbacBase(t, 6), tarski.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if h, err := res.Relation("holds"); err != nil || h.Len() == 0 {
		t.Errorf("full holds after Demand = %v, %v", h, err)
	}
	if n, ok := demand.Arity("holds"); !ok || n != 2 || !demand.Defines("permit") || demand.HasAggregates() {
		t.Error("the demand plan does not answer for the source plan")
	}
	if _, ok := demand.Arity("m?holds?bf"); ok {
		t.Error("the demand plan names a hidden relation")
	}
}
