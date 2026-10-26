package tarski_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The evidence for demand-driven evaluation (design/demand.md § 5). Two shapes, each with
// a test that demand answers what full evaluation answers and a benchmark that reports
// the cost: this file is Shearline's, bench_demand_anam_test.go is Anam's.
//
// testdata/rbac.dl is Shearline's policy/rbac.dl byte for byte, copied from shearline
// 085d076 (the last commit to touch it) on 2026-10-26; testdata/rbac_unseeded.dl is the
// same rules with the request seeds removed.

// shearBytesBound is the per-request byte bound the cliff test sets: Shearline's order of
// magnitude for a decision (design § 5).
const shearBytesBound = 1 << 20

var shearSchema = tarski.Schema{Arities: map[string]int{
	"principal": 1, "action": 1, "resource": 1,
	"role": 2, "inherits": 2, "grants": 3, "denied": 3,
}}

// shearRequests are the decisions the tests and benchmarks ask: allowed, denied by a
// denial, and not granted.
var shearRequests = []struct {
	name                        string
	principal, action, resource string
	allowed                     bool
}{
	{"allowed", "p1", "read", "/doc/x", true},
	{"denied", "p1", "read", "/doc/secret", false},
	{"not granted", "p1", "admin", "/doc/x", false},
}

func readRules(tb testing.TB, path string) tarski.Program {
	tb.Helper()
	text, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	prog, err := tarski.Parse(string(text))
	if err != nil {
		tb.Fatalf("Parse %s: %v", path, err)
	}
	return prog
}

func compileRules(tb testing.TB, schema tarski.Schema, path string) tarski.Plan {
	tb.Helper()
	plan, err := tarski.Compile(schema, readRules(tb, path))
	if err != nil {
		tb.Fatalf("Compile %s: %v", path, err)
	}
	return plan
}

func strFact(relation string, args ...string) tarski.Fact {
	t := make(tarski.Tuple, len(args))
	for i, a := range args {
		t[i] = tarski.String(a)
	}
	return tarski.Fact{Relation: relation, Tuple: t}
}

// shearBase is Shearline's standing base: n principals holding one role each, a twelve-role
// inheritance chain, 200 grants (one of them the read grant on /doc/ at the chain's end),
// and 500 denials (one of them p1's on /doc/secret).
func shearBase(tb testing.TB, n int) tarski.Base {
	tb.Helper()
	const chain, grants, denials = 12, 200, 500
	role := func(i int) string { return "r" + strconv.Itoa(i%chain) }
	actions := []string{"read", "write", "delete"}
	facts := make([]tarski.Fact, 0, n+chain+grants+denials)
	for i := 0; i < chain-1; i++ {
		facts = append(facts, strFact("inherits", role(i), role(i+1)))
	}
	for i := range n {
		facts = append(facts, strFact("role", "p"+strconv.Itoa(i), role(i)))
	}
	facts = append(facts, strFact("grants", role(chain-1), "read", "/doc/"))
	for k := 0; k < grants-1; k++ {
		facts = append(facts, strFact("grants", role(k), actions[k%3], "/area/"+strconv.Itoa(k)+"/"))
	}
	facts = append(facts, strFact("denied", "p1", "read", "/doc/secret"))
	for k := 0; k < denials-1; k++ {
		facts = append(facts, strFact("denied", "p"+strconv.Itoa(k*97%n), "read", "/area/"+strconv.Itoa(k%grants)+"/x"))
	}
	base, err := tarski.NewBase(shearSchema, facts...)
	if err != nil {
		tb.Fatal(err)
	}
	return base
}

// shearRequestBase is the standing base plus one request's facts, as Shearline's Decide
// builds it.
func shearRequestBase(tb testing.TB, base tarski.Base, principal, action, resource string) tarski.Base {
	tb.Helper()
	b, err := base.With(strFact("principal", principal), strFact("action", action), strFact("resource", resource))
	if err != nil {
		tb.Fatal(err)
	}
	return b
}

func shearGoal(principal, action, resource string) *tarski.Goal {
	return strGoal("allow", principal, action, resource)
}

// shearPlans compiles the seeded rules, the unseeded rules, and the unseeded rules for
// the goal allow/bbb.
func shearPlans(tb testing.TB) (seeded, unseeded, demand tarski.Plan) {
	tb.Helper()
	seeded = compileRules(tb, shearSchema, "testdata/rbac.dl")
	unseeded = compileRules(tb, shearSchema, "testdata/rbac_unseeded.dl")
	demand, err := unseeded.Demand("allow", "bbb")
	if err != nil {
		tb.Fatal(err)
	}
	return seeded, unseeded, demand
}

func evaluateAllow(tb testing.TB, plan tarski.Plan, base tarski.Base, opts tarski.Options) (tarski.Answer, tarski.Result, error) {
	tb.Helper()
	res, err := tarski.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		return tarski.Answer{}, tarski.Result{}, err
	}
	ans, err := res.Relation("allow")
	if err != nil {
		tb.Fatal(err)
	}
	return ans, res, nil
}

// The cliff: unseeded rules evaluated in full cross MaxBytes, which is why Shearline
// seeds its rules by hand. Demand over the same rules, the same bounds and the same base
// answers, and answers what the seeded rules answer.
func TestShearline_UnseededFullHitsTheCliffDemandDoesNot(t *testing.T) {
	t.Parallel()
	seeded, unseeded, demand := shearPlans(t)
	standing := shearBase(t, 50_000)
	bounds := tarski.Options{MaxBytes: shearBytesBound}
	for _, rq := range shearRequests {
		t.Run(rq.name, func(t *testing.T) {
			base := shearRequestBase(t, standing, rq.principal, rq.action, rq.resource)
			var tl *tarski.TooLarge
			if _, _, err := evaluateAllow(t, unseeded, base, bounds); !errors.As(err, &tl) || tl.Bound != tarski.BoundBytes {
				t.Fatalf("unseeded full under MaxBytes = %v, want TooLarge on %s (the cliff)", err, tarski.BoundBytes)
			}
			want, _, err := evaluateAllow(t, seeded, base, bounds)
			if err != nil {
				t.Fatalf("seeded full: %v", err)
			}
			dopts := bounds
			dopts.Goal = shearGoal(rq.principal, rq.action, rq.resource)
			got, _, err := evaluateAllow(t, demand, base, dopts)
			if err != nil {
				t.Fatalf("unseeded demand under the same bound: %v", err)
			}
			if (got.Len() == 1) != rq.allowed {
				t.Errorf("demand allow has %d tuples, want allowed=%v", got.Len(), rq.allowed)
			}
			// The seeded relation holds the request's tuple; the demand relation is filtered
			// to the goal. For a single-request base they are the same relation.
			if got.Hash() != want.Hash() {
				t.Errorf("demand answer %s differs from seeded full %s", got.Hash(), want.Hash())
			}
		})
	}
}

// BenchmarkShearline_Allow is design § 5 benchmark 1: one decision on a standing base of
// 50,000 principals. Success: demand over the unseeded rules within 2x of full over the
// seeded rules (ns/op and steps/op). Unseeded full is the cliff: it runs under MaxBytes and
// reports the time and the work done when the bound trips (unbounded it derives 169 MB in
// 23 s on an M1, measured once on 2026-10-26).
func BenchmarkShearline_Allow(b *testing.B) {
	seeded, unseeded, demand := shearPlans(b)
	standing := shearBase(b, 50_000)
	rq := shearRequests[0]
	goal := shearGoal(rq.principal, rq.action, rq.resource)
	runs := []struct {
		name string
		plan tarski.Plan
		opts tarski.Options
	}{
		{"seeded-full", seeded, tarski.Options{}},
		{"unseeded-full-to-the-cliff", unseeded, tarski.Options{MaxBytes: shearBytesBound}},
		{"unseeded-demand", demand, tarski.Options{Goal: goal}},
	}
	for _, run := range runs {
		b.Run(run.name, func(b *testing.B) {
			b.ReportAllocs()
			var steps, bytes int
			for range b.N {
				base := shearRequestBase(b, standing, rq.principal, rq.action, rq.resource)
				res, err := tarski.Evaluate(context.Background(), run.plan, base, run.opts)
				var tl *tarski.TooLarge
				switch {
				case errors.As(err, &tl) && run.opts.MaxBytes > 0:
					steps, bytes = tl.Steps, tl.Bytes
				case err != nil:
					b.Fatal(err)
				default:
					steps, bytes = res.Cost()
				}
			}
			b.ReportMetric(float64(steps), "steps/op")
			b.ReportMetric(float64(bytes), "bytes/op")
		})
	}
}
