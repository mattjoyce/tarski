package tarski_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/mattjoyce/tarski"
)

// These invariants exercise the consumer-shaped rules in demand_test.go: cycles
// terminate, denial wins, unrelated principals cannot borrow grants, and replacing
// a snapshot without a role revokes access. Both evaluation strategies must agree.
func TestPressure_RBACCycleAndRevocation(t *testing.T) {
	plan, demand := rbacPlans(t)
	cases := []struct {
		name, principal, resource string
		revoked, want             bool
	}{
		{"inherited through cycle", "p0", "/doc/a", false, true},
		{"explicit denial", "p0", "/doc/b", false, false},
		{"unrelated principal", "p1", "/doc/a", false, false},
		{"outside prefix", "p0", "/secret/a", false, false},
		{"revoked snapshot", "p0", "/doc/a", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				base := pressureRBACBase(t, tc.revoked, reverse)
				goal := strGoal("allow", tc.principal, "read", tc.resource)
				for _, mode := range []struct {
					plan tarski.Plan
					goal *tarski.Goal
				}{{plan, nil}, {demand, goal}} {
					res, err := tarski.Evaluate(context.Background(), mode.plan, base, tarski.Options{
						Goal: mode.goal, Proofs: true, MaxSteps: 10000, MaxBytes: 10000,
					})
					if err != nil {
						t.Fatal(err)
					}
					pressureDecision(t, res, goal, tc.want)
				}
			}
		})
	}
}

func pressureRBACBase(t *testing.T, revoked, reverse bool) tarski.Base {
	t.Helper()
	fact := func(name string, args ...string) tarski.Fact {
		tuple := make(tarski.Tuple, len(args))
		for i, arg := range args {
			tuple[i] = tarski.String(arg)
		}
		return tarski.Fact{Relation: name, Tuple: tuple}
	}
	facts := []tarski.Fact{
		fact("inherits", "r0", "r1"), fact("inherits", "r1", "r0"),
		fact("grants", "r1", "read", "/doc/"), fact("denied", "p0", "read", "/doc/b"),
		fact("resource", "/doc/a"), fact("resource", "/doc/b"), fact("resource", "/secret/a"),
	}
	if !revoked {
		facts = append(facts, fact("role", "p0", "r0"), fact("role", "p0", "r0"))
	}
	if reverse {
		for i, j := 0, len(facts)-1; i < j; i, j = i+1, j-1 {
			facts[i], facts[j] = facts[j], facts[i]
		}
	}
	base, err := tarski.NewBase(rbacTestSchema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func pressureDecision(t *testing.T, res tarski.Result, goal *tarski.Goal, want bool) {
	t.Helper()
	tuple := make(tarski.Tuple, len(goal.Args))
	for i, arg := range goal.Args {
		tuple[i] = arg.Const
	}
	answer, err := res.Relation("allow")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, got := range answer.Tuples() {
		found = found || reflect.DeepEqual(got, tuple)
	}
	_, proved := res.Proof("allow", tuple)
	_, absent := res.WhyNot("allow", tuple, 0)
	if found != want || proved != want || absent == want {
		t.Fatalf("goal %v: membership=%v proof=%v absence=%v, want allow=%v", tuple, found, proved, absent, want)
	}
}

func TestPressure_RBACRefusalsExposeNoAnswer(t *testing.T) {
	plan, demand := rbacPlans(t)
	base := pressureRBACBase(t, false, false)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := []struct {
		name string
		ctx  context.Context
		opts tarski.Options
		want error
	}{
		{"steps", context.Background(), tarski.Options{MaxSteps: 1}, tarski.ErrTooLarge},
		{"bytes", context.Background(), tarski.Options{MaxBytes: 1}, tarski.ErrTooLarge},
		{"cancelled", cancelled, tarski.Options{}, context.Canceled},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, proofs := range []bool{false, true} {
				for i, p := range []tarski.Plan{plan, demand} {
					opts := tc.opts
					opts.Proofs = proofs
					if i == 1 {
						opts.Goal = strGoal("allow", "p0", "read", "/doc/a")
					}
					res, err := tarski.Evaluate(tc.ctx, p, base, opts)
					if !errors.Is(err, tc.want) || !reflect.DeepEqual(res, tarski.Result{}) {
						t.Fatalf("refusal: result=%v error=%v, want zero result and %v", res, err, tc.want)
					}
				}
			}
		})
	}
}
