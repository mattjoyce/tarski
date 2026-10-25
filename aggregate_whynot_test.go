package tarski_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// Why-not through an aggregate (docs/design/aggregates.md §5.4): the walk computes
// the aggregate for the group over the result's relations, and a group that gives no
// value, or a value other than the one bound, is the failing literal with a Reason
// and, for a wrong value, the Value it had.

const aggWhyNotText = `n(G, N) :- grp(G), N = count : { item(G, _) }.
s(G, S) :- grp(G), S = sum X : { mixed(G, X) }.
lo(G, L) :- grp(G), L = min X : { item(G, X) }.
hi(G, H) :- grp(G), H = max X : { mixed(G, X) }.
big(G) :- grp(G), N = count : { item(G, _) }, N > 5.
busy(G) :- grp(G), n(G, 5).
quiet(G) :- grp(G), not n(G, 3).
pair(G) :- grp(G), n(X, X).`

func TestWhyNot_Aggregate(t *testing.T) {
	t.Parallel()
	facts := append(aggFacts(), fact("grp", "m"), fact("grp", "n"))
	res, err := aggEval(t, aggSchema, facts, aggWhyNotText, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	a, m, n := tarski.String("a"), tarski.String("m"), tarski.String("n")
	three := tarski.Int(3)
	cases := []struct {
		name     string
		relation string
		tuple    tarski.Tuple
		failed   string
		value    *tarski.Const
		reason   string
		text     string // a line String must hold
	}{
		{"pre-bound result differs", "n", tarski.Tuple{a, tarski.Int(4)}, "N = count : { item(G, _) }", &three,
			"the aggregate is 3, not 4",
			`stopped at N = count : { item(G, _) }  with G="a", N=4: the aggregate is 3, not 4`},
		{"empty group under min", "lo", tarski.Tuple{tarski.String("e"), tarski.Int(1)}, "L = min X : { item(G, X) }", nil,
			"the group is empty, so it has no min", ""},
		{"mixed kinds under max", "hi", tarski.Tuple{m, tarski.Int(1)}, "H = max X : { mixed(G, X) }", nil,
			`the group mixes kinds: 1 and "1", so it has no max`, ""},
		{"sum over a non-number", "s", tarski.Tuple{n, tarski.Int(0)}, "S = sum X : { mixed(G, X) }", nil,
			`the group holds "z", which is not a number, so it cannot be summed`, ""},
		{"missing group key", "n", tarski.Tuple{tarski.String("zz"), tarski.Int(0)}, "grp(G)", nil, "",
			`grp("zz") is not derived: no such fact`},
		{"aggregate binds and the walk goes on", "big", tarski.Tuple{a}, "N > 5", nil, "",
			`stopped at N > 5  with G="a", N=3`},
		{"negated aggregate relation blocked", "quiet", tarski.Tuple{a}, "not n(G, 3)", nil, "",
			"        group of 3\n          item(\"a\", 1)  fact\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			why, ok := res.WhyNot(tc.relation, tc.tuple, 0)
			if !ok || why.Truncated || len(why.Rules) != 1 {
				t.Fatalf("WhyNot = %v, %v; want one rule, not truncated", why, ok)
			}
			rf := why.Rules[0]
			if got := failedText(t, why); got != tc.failed {
				t.Errorf("Failed = %q, want %q", got, tc.failed)
			}
			if !reflect.DeepEqual(rf.Value, tc.value) || rf.Reason != tc.reason {
				t.Errorf("Value, Reason = %v, %q; want %v, %q", rf.Value, rf.Reason, tc.value, tc.reason)
			}
			if !strings.Contains(why.String(), tc.text) {
				t.Errorf("String() =\n%s\nwant it to hold\n%s", why.String(), tc.text)
			}
		})
	}
	// A blocked proof carries its group as Proof does.
	why, _ := res.WhyNot("quiet", tarski.Tuple{a}, 0)
	if b := why.Rules[0].Blocked; b == nil || b.GroupSize != 3 || len(b.Group) != 3 {
		t.Errorf("Blocked = %+v, want n(a, 3) with its group of 3", b)
	}
}

// failedText is the failing literal as the JSON writes it.
func failedText(t *testing.T, a tarski.Absence) string {
	t.Helper()
	var doc struct {
		Rules []struct {
			Failed string `json:"failed"`
		} `json:"rules"`
	}
	raw, err := json.Marshal(a)
	if err != nil || json.Unmarshal(raw, &doc) != nil || len(doc.Rules) == 0 {
		t.Fatalf("Absence JSON %s: %v", raw, err)
	}
	return doc.Rules[0].Failed
}

// A failure found through a sub-goal reaches the aggregate too: busy(a) needs n(a, 5),
// whose count is 3, and pair(a) needs n(X, X), whose head equates the group key with
// the count, so the walk holds the result to the key.
func TestWhyNot_AggregateBecause(t *testing.T) {
	t.Parallel()
	res, err := aggEval(t, aggSchema, aggFacts(), aggWhyNotText, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		relation, reason string
	}{
		{"busy", "the aggregate is 3, not 5"},
		{"pair", `the aggregate is 3, not "a"`},
	}
	for _, tc := range cases {
		why, ok := res.WhyNot(tc.relation, tarski.Tuple{tarski.String("a")}, 0)
		if !ok || why.Truncated || len(why.Rules) != 1 || len(why.Rules[0].Because) != 1 {
			t.Fatalf("WhyNot(%s(a)) = %v, %v", tc.relation, why, ok)
		}
		sub := why.Rules[0].Because[0]
		if len(sub.Rules) != 1 || sub.Rules[0].Reason != tc.reason {
			t.Errorf("WhyNot(%s(a)) because = %v; want reason %q", tc.relation, sub, tc.reason)
		}
	}
}

// Value and Reason travel in the JSON, value as a constant and reason as text, and
// read back as what they were.
func TestWhyNot_AggregateJSONRoundTrip(t *testing.T) {
	t.Parallel()
	res, err := aggEval(t, aggSchema, aggFacts(), aggWhyNotText, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	why, _ := res.WhyNot("n", tarski.Tuple{tarski.String("a"), tarski.Int(4)}, 0)
	raw, err := json.Marshal(why)
	if err != nil {
		t.Fatal(err)
	}
	want := `"failed":"N = count : { item(G, _) }","value":3,"reason":"the aggregate is 3, not 4"}]}`
	if !strings.HasSuffix(string(raw), want) {
		t.Errorf("JSON =\n%s\nwant it to end\n%s", raw, want)
	}
	var back struct {
		Rules []struct {
			Value  *tarski.Const `json:"value"`
			Reason string        `json:"reason"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(raw, &back); err != nil || len(back.Rules) != 1 {
		t.Fatalf("Unmarshal: %v, %+v", err, back)
	}
	got := back.Rules[0]
	if got.Value == nil || *got.Value != *why.Rules[0].Value || got.Reason != why.Rules[0].Reason {
		t.Errorf("round trip = %v, %q; want %v, %q", got.Value, got.Reason, why.Rules[0].Value, why.Rules[0].Reason)
	}
	// A failure that is not an aggregate writes neither key.
	busy, _ := res.WhyNot("big", tarski.Tuple{tarski.String("a")}, 0)
	if raw, _ := json.Marshal(busy); strings.Contains(string(raw), `"value"`) || strings.Contains(string(raw), `"reason"`) {
		t.Errorf("JSON of a non-aggregate failure names value or reason: %s", raw)
	}
}

// The walk's budget bounds an aggregate inside it: the candidates its body visits, the
// relations it scans, and a sum's fold. Each case is a group whose work passes the
// budget though the evaluation met it, and each why-not says it was cut short and
// claims no failing literal.
func TestWhyNot_AggregateBudget(t *testing.T) {
	t.Parallel()
	wide := []tarski.Fact{fact("grp", "g")}
	for i := range 70_000 {
		wide = append(wide, fact("item", "g", i))
	}
	var spread []tarski.Fact
	for i := range 200 {
		lexeme := "1e60000"
		if i%2 == 1 {
			lexeme = fmt.Sprint(i)
		}
		spread = append(spread, tarski.Fact{Relation: "item", Tuple: tarski.Tuple{tarski.Int(int64(i)), mustNumber(t, lexeme)}})
	}
	cases := []struct {
		name, text string
		facts      []tarski.Fact
		goal       tarski.Tuple
	}{
		{"candidates", "c(G, N) :- grp(G), N = count : { item(G, _) }.", wide,
			tarski.Tuple{tarski.String("g"), tarski.Int(1)}},
		{"scan of a derived relation", "d(X, Y) :- item(X, Y).\nc(G, N) :- grp(G), N = count : { d(_, \"none\") }.", wide,
			tarski.Tuple{tarski.String("g"), tarski.Int(1)}},
		{"sum fold", "c(G, S) :- grp(G), S = sum X : { item(_, X) }.", append(spread, fact("grp", "g")),
			tarski.Tuple{tarski.String("g"), tarski.Int(1)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := aggEval(t, aggSchema, tc.facts, tc.text, tarski.Options{Proofs: true})
			if err != nil {
				t.Fatal(err)
			}
			why, ok := res.WhyNot("c", tc.goal, 0)
			if !ok || !why.Truncated || len(why.Rules) != 1 {
				t.Fatalf("WhyNot = %v, %v; want one rule, truncated", why, ok)
			}
			if f := why.Rules[0]; f.Failed.Aggregate != nil || f.Failed.Atom != nil || f.Reason != "" {
				t.Errorf("a cut walk claims a failure: %+v", f)
			}
		})
	}
}
