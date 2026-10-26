package tarski_test

import (
	"context"
	"errors"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/mattjoyce/tarski"
)

// Anam's shape (design/demand.md § 5, benchmark 2): testdata/anam.dl with a query that
// defines instant/1, over a large entry base. The wide instant is the one Anam may want
// to write, every fact-time there is; the hand-bounded instant is today's discipline,
// only the blood-pressure readings' fact-times. Demand over the wide one should cost what
// full evaluation over the bounded one does.

const (
	anamRegimen = `
regimen(R, E) :- entry(R, "health.blood-pressure", _, _, _, _, _), in_force(E, R),
                 entry(E, T, _, _, _, _, _), T > "health.medication.", T < "health.medication/".
`
	anamWide = `instant(At) :- entry(_, _, At, _, _, _, _).` + anamRegimen
	anamHand = `instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).` + anamRegimen
)

// anamScale is a ledger: entries in total, every readingEvery-th a blood-pressure reading,
// the rest medication statements spread over series.
type anamScale struct{ entries, series, readingEvery int }

var (
	anamMedium = anamScale{entries: 6_000, series: 300, readingEvery: 60} // 100 readings
	anamSmall  = anamScale{entries: 900, series: 30, readingEvery: 30}    // 30 readings
	anamTiny   = anamScale{entries: 300, series: 12, readingEvery: 30}    // 10 readings, for the tests
)

// anamLedger builds the base. Fact-times are fixed-width and strictly increasing with the
// entry index, so every instant is distinct. A series starts, changes dose, and every
// third series ends in a stop, which closes it.
func anamLedger(tb testing.TB, sc anamScale) tarski.Base {
	tb.Helper()
	start := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	seen := make([]int, sc.series)
	facts := make([]tarski.Fact, 0, sc.entries+1)
	facts = append(facts, strFact("closes", "health.medication.stopped"))
	perSeries := sc.entries * (sc.readingEvery - 1) / sc.readingEvery / sc.series
	for i := range sc.entries {
		at := start.Add(time.Duration(i) * 37 * time.Minute).Format("2006-01-02T15:04:05.000000000Z")
		id, typ, series := "e"+strconv.Itoa(i), "health.blood-pressure", ""
		if i%sc.readingEvery != 0 {
			s := i % sc.series
			series = "s" + strconv.Itoa(s)
			typ = "health.medication.dose-changed"
			switch {
			case seen[s] == 0:
				typ = "health.medication.started"
			case seen[s] >= perSeries-1 && s%3 == 0:
				typ = "health.medication.stopped"
			}
			seen[s]++
		}
		n, _ := tarski.Number("1")
		facts = append(facts, tarski.Fact{Relation: "entry", Tuple: tarski.Tuple{
			tarski.String(id), tarski.String(typ), tarski.String(at), tarski.String(series),
			tarski.String("admitted"), tarski.String("bench"), n,
		}})
	}
	base, err := tarski.NewBase(anamSchema, facts...)
	if err != nil {
		tb.Fatal(err)
	}
	return base
}

// anamPlan compiles testdata/anam.dl with a query.
func anamPlan(tb testing.TB, query string) tarski.Plan {
	tb.Helper()
	text, err := os.ReadFile("testdata/anam.dl")
	if err != nil {
		tb.Fatal(err)
	}
	set, err := tarski.NewRuleSet("anam", 2, string(text))
	if err != nil {
		tb.Fatal(err)
	}
	q, err := tarski.Parse(query)
	if err != nil {
		tb.Fatal(err)
	}
	plan, err := tarski.Compile(anamSchema, set.Program, q)
	if err != nil {
		tb.Fatal(err)
	}
	return plan
}

func regimenOf(tb testing.TB, plan tarski.Plan, base tarski.Base, opts tarski.Options) (tarski.Answer, tarski.Result, error) {
	tb.Helper()
	res, err := tarski.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		return tarski.Answer{}, res, err
	}
	ans, err := res.Relation("regimen")
	if err != nil {
		tb.Fatal(err)
	}
	return ans, res, nil
}

// Demand answers what full evaluation answers on Anam's shape, for the wide instant and
// the hand-bounded one, answer for answer.
func TestAnam_DemandEqualsFull(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		query string
		scale anamScale
	}{
		{"wide instant, tiny ledger", anamWide, anamTiny},
		{"hand-bounded instant, tiny ledger", anamHand, anamTiny},
		{"hand-bounded instant, small ledger", anamHand, anamSmall},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			base, plan := anamLedger(t, tc.scale), anamPlan(t, tc.query)
			demand, err := plan.Demand("regimen", "ff")
			if err != nil {
				t.Fatal(err)
			}
			want, full, err := regimenOf(t, plan, base, tarski.Options{})
			if err != nil {
				t.Fatalf("full: %v", err)
			}
			got, dres, err := regimenOf(t, demand, base, tarski.Options{Goal: strGoal("regimen", "?R", "?E")})
			if err != nil {
				t.Fatalf("demand: %v", err)
			}
			if want.Len() == 0 {
				t.Fatal("the ledger answers nothing; the comparison would be vacuous")
			}
			if got.Hash() != want.Hash() {
				t.Errorf("demand regimen %s (%d tuples) differs from full %s (%d)", got.Hash(), got.Len(), want.Hash(), want.Len())
			}
			fs, _ := full.Cost()
			ds, _ := dres.Cost()
			t.Logf("%d regimen tuples; steps: full %d, demand %d", want.Len(), fs, ds)
		})
	}
}

// BenchmarkAnam_Regimen is design § 5 benchmark 2. Success: demand on the wide instant
// within 1.2x of full on the hand-bounded one, and at least 10x below full on the wide
// one. Run it with -benchtime=1x: one operation takes seconds to a minute.
//
// The design sized the ledger at 20,000 entries in 300 series. At that size full
// evaluation with the hand-bounded instant is itself infeasible (displaced/2 joins every
// later/2 pair to every instant; it did not finish in nine minutes), so the benchmark
// runs a ledger of 6,000 entries (the medium scale) where it does, and a small one of 900
// where the wide instant's full evaluation also finishes. Wide full at the medium scale
// runs under a 10M-step MaxSteps (a 100M bound ran past fifteen minutes) and reports the work
// done when it trips, which is far below what the demand run costs, so only the small scale
// can show the ratio against wide full.
func BenchmarkAnam_Regimen(b *testing.B) {
	scales := []struct {
		name         string
		scale        anamScale
		wideFullStep int // 0: unbounded
	}{
		{"small", anamSmall, 0},
		{"medium", anamMedium, 10_000_000},
	}
	goal := strGoal("regimen", "?R", "?E")
	for _, sc := range scales {
		base := anamLedger(b, sc.scale)
		wide, hand := anamPlan(b, anamWide), anamPlan(b, anamHand)
		wideDemand, err := wide.Demand("regimen", "ff")
		if err != nil {
			b.Fatal(err)
		}
		handDemand, err := hand.Demand("regimen", "ff")
		if err != nil {
			b.Fatal(err)
		}
		runs := []struct {
			name string
			plan tarski.Plan
			opts tarski.Options
		}{
			{"hand-bounded-full", hand, tarski.Options{}},
			{"hand-bounded-demand", handDemand, tarski.Options{Goal: goal}},
			{"wide-demand", wideDemand, tarski.Options{Goal: goal}},
			{"wide-full", wide, tarski.Options{MaxSteps: sc.wideFullStep}},
		}
		for _, run := range runs {
			b.Run(sc.name+"/"+run.name, func(b *testing.B) {
				b.ReportAllocs()
				var steps, bytes int
				for range b.N {
					res, err := tarski.Evaluate(context.Background(), run.plan, base, run.opts)
					var tl *tarski.TooLarge
					switch {
					case errors.As(err, &tl) && run.opts.MaxSteps > 0:
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
}
