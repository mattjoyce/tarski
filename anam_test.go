package tarski_test

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Anam is a consumer that runs its own rule set on this engine. testdata/anam.dl is
// Anam's rules/anam.dl byte for byte, copied from anam origin/master 45e4a67 on
// 2026-10-25. Refresh it when Anam's rule set moves; the test is the promise that an
// engine change breaking Anam's query fails here first (AGENTS.md § Consumers).

var anamSchema = tarski.Schema{Arities: map[string]int{"entry": 7, "attr": 3, "closes": 1}}

// anamQuery is a question Anam's docs ask: which medication statements were in force
// at each blood-pressure reading, and what words were said about a reading.
const anamQuery = `
instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
regimen(R, E) :- entry(R, "health.blood-pressure", _, _, _, _, _), in_force(E, R),
                 entry(E, T, _, _, _, _, _), T > "health.medication.", T < "health.medication/".
noted(R, Text) :- linked(Edge, R), attr(Edge, "/text", Text).
`

func anamBase(t *testing.T) tarski.Base {
	t.Helper()
	at := func(day string) string { return day + "T00:00:00.000000000Z" }
	entry := func(id, typ, day, series string) tarski.Fact {
		return fact("entry", id, typ, at(day), series, "admitted", "test", 1)
	}
	return newBaseUnder(t, anamSchema,
		entry("e1", "health.medication.started", "2026-03-01", "ramipril"),
		entry("e2", "health.medication.dose-changed", "2026-05-15", "ramipril"),
		entry("e3", "health.medication.started", "2026-05-01", "d3"),
		entry("e4", "health.medication.stopped", "2026-07-15", "d3"),
		entry("r1", "health.blood-pressure", "2026-06-01", ""),
		entry("r2", "health.blood-pressure", "2026-07-20", ""),
		entry("n1", "record.edge", "2026-06-01", ""),
		fact("attr", "n1", "/about/0", "r1"),
		fact("attr", "n1", "/text", "Before the dentist."),
		fact("closes", "health.medication.stopped"),
	)
}

func newBaseUnder(t *testing.T, schema tarski.Schema, facts ...tarski.Fact) tarski.Base {
	t.Helper()
	b, err := tarski.NewBase(schema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	return b
}

func TestAnamRuleSet_CompilesAndAnswers(t *testing.T) {
	text, err := os.ReadFile("testdata/anam.dl")
	if err != nil {
		t.Fatal(err)
	}
	set, err := tarski.NewRuleSet("anam", 2, string(text))
	if err != nil {
		t.Fatalf("NewRuleSet: %v", err)
	}
	if err := set.Verify(); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	query, err := tarski.Parse(anamQuery)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	plan, err := tarski.Compile(anamSchema, set.Program, query)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, anamBase(t), tarski.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}

	tests := []struct {
		relation string
		want     [][]string
	}{
		// At r1 (1 June) ramipril's dose change and the open D3 course stand; at r2
		// (20 July) the D3 course has been stopped, which closes its series.
		{"regimen", [][]string{{"r1", "e2"}, {"r1", "e3"}, {"r2", "e2"}}},
		{"noted", [][]string{{"r1", "Before the dentist."}}},
	}
	for _, tt := range tests {
		t.Run(tt.relation, func(t *testing.T) {
			ans, err := res.Relation(tt.relation)
			if err != nil {
				t.Fatal(err)
			}
			var got [][]string
			for _, tup := range ans.Tuples() {
				row := make([]string, len(tup))
				for i, c := range tup {
					row[i] = c.Text()
				}
				got = append(got, row)
			}
			if !slices.EqualFunc(got, tt.want, slices.Equal[[]string]) {
				t.Errorf("%s = %v, want %v", tt.relation, got, tt.want)
			}
		})
	}

	stopped := tarski.Tuple{tarski.String("r2"), tarski.String("e3")}
	if _, ok := res.Proof("regimen", tarski.Tuple{tarski.String("r1"), tarski.String("e3")}); !ok {
		t.Error("no proof for regimen(r1, e3)")
	}
	why, ok := res.WhyNot("regimen", stopped, 0)
	if !ok || len(why.Rules) == 0 {
		t.Errorf("WhyNot(regimen(r2, e3)) = %v, %v; want an explanation", why, ok)
	}
}
