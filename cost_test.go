package tarski_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/tarski"
)

func costOf(t *testing.T, plan tarski.Plan, base tarski.Base, opts tarski.Options) (steps, bytes int) {
	t.Helper()
	res, err := tarski.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return res.Cost()
}

// Cost reports the counters the bounds use: a bound set to the cost passes, and one
// below it is refused, on a full plan, with proofs, and on a demand plan.
func TestResult_CostIsWhatTheBoundsCharge(t *testing.T) {
	t.Parallel()
	reachPlan, err := tarski.Compile(testSchema, parse(t, reachRules))
	if err != nil {
		t.Fatal(err)
	}
	reachDemand, err := reachPlan.Demand("reaches", "bf")
	if err != nil {
		t.Fatal(err)
	}
	rbac, rbacDemand := rbacPlans(t)
	cases := []struct {
		name string
		plan tarski.Plan
		base tarski.Base
		opts tarski.Options
	}{
		{"full", reachPlan, newBase(t, chain(40)...), tarski.Options{}},
		{"full with proofs", reachPlan, newBase(t, chain(40)...), tarski.Options{Proofs: true}},
		{"demand", reachDemand, newBase(t, chain(40)...), tarski.Options{Goal: strGoal("reaches", "n5", "?Y")}},
		{"rbac full", rbac, rbacBase(t, 30), tarski.Options{}},
		{"rbac demand", rbacDemand, rbacBase(t, 30), tarski.Options{Goal: strGoal("allow", "p1", "read", "/doc/a")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			steps, bytes := costOf(t, tc.plan, tc.base, tc.opts)
			if steps <= 0 || bytes <= 0 {
				t.Fatalf("Cost = %d, %d, want both positive", steps, bytes)
			}
			if s2, b2 := costOf(t, tc.plan, tc.base, tc.opts); s2 != steps || b2 != bytes {
				t.Errorf("second run Cost = %d, %d, want %d, %d", s2, b2, steps, bytes)
			}
			okOpts := tc.opts
			okOpts.MaxSteps, okOpts.MaxBytes = steps, bytes
			if _, err := tarski.Evaluate(context.Background(), tc.plan, tc.base, okOpts); err != nil {
				t.Errorf("bounds equal to the cost: %v, want success", err)
			}
			for _, bound := range []struct {
				name  string
				apply func(*tarski.Options)
				want  string
			}{
				{"steps", func(o *tarski.Options) { o.MaxSteps = steps - 1 }, tarski.BoundSteps},
				{"bytes", func(o *tarski.Options) { o.MaxBytes = bytes - 1 }, tarski.BoundBytes},
			} {
				o := tc.opts
				bound.apply(&o)
				var tl *tarski.TooLarge
				_, err := tarski.Evaluate(context.Background(), tc.plan, tc.base, o)
				if !errors.As(err, &tl) || tl.Bound != bound.want {
					t.Errorf("one %s under the cost: %v, want TooLarge on %s", bound.name, err, bound.want)
				}
			}
		})
	}
}

// Over an empty base nothing is derived, so no bytes are charged; each rule application
// still costs one step (v0.2.1: every rule application is a step), so the two reach rules
// cost two.
func TestResult_CostOverAnEmptyBaseIsOneStepPerRule(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(testSchema, parse(t, reachRules))
	if err != nil {
		t.Fatal(err)
	}
	if s, b := costOf(t, plan, newBase(t), tarski.Options{}); s != 2 || b != 0 {
		t.Errorf("Cost over an empty base = %d, %d, want 2 (one step per rule application), 0", s, b)
	}
}

// Cost follows the question: the demand plan charges less than the full one for a
// goal that reaches a sliver of the base.
func TestResult_DemandCostsLessThanFull(t *testing.T) {
	t.Parallel()
	plan, demand := rbacPlans(t)
	base := rbacBase(t, 300)
	fs, fb := costOf(t, plan, base, tarski.Options{})
	ds, db := costOf(t, demand, base, tarski.Options{Goal: strGoal("allow", "p1", "read", "/doc/a")})
	if ds >= fs || db >= fb {
		t.Errorf("demand cost = %d steps, %d bytes; full = %d, %d; want demand lower on both", ds, db, fs, fb)
	}
}
