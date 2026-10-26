package tarski_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The demand property (docs/design/demand.md §4, D1): over random programs with
// recursion and stratified negation, random bases and random goals, the goal's relation
// on a demand Result is full evaluation's filtered to the goal's constants, byte for
// byte and hash for hash; every other derived relation is refused; base relations are
// the base's; every proof is a valid derivation and every why-not fails at the literal
// full evaluation's does (D11). Seeds replay.

var propDemandSchema = tarski.Schema{Arities: map[string]int{"e": 2, "n": 1, "f": 1}}

// propDomain is the active domain of the random bases; "zz" is outside it.
var propDomain = []string{"n0", "n1", "n2", "n3", "n4"}

// propProgram is a random program over e/2, n/1 and f/1: derived r0..r3 of arity two,
// each from one to three rules that chain base and lower relations, may recurse through
// itself, and may negate e or a strictly lower relation, so the program stratifies; and
// q/1 over one of them, through a negation and an open position.
func propProgram(r *rand.Rand) string {
	var b strings.Builder
	for i := range 4 {
		head := "r" + strconv.Itoa(i)
		pos := func() string { // a positive relation for ri: e, a lower one, or itself
			if k := r.IntN(i + 2); k <= i {
				return "r" + strconv.Itoa(k)
			}
			return "e"
		}
		for range 1 + r.IntN(3) {
			var body []string
			switch r.IntN(4) {
			case 0:
				body = append(body, pos()+"(X, Y)")
			case 1:
				body = append(body, pos()+"(X, Z)", pos()+"(Z, Y)")
			case 2:
				body = append(body, pos()+"(Y, X)")
			default:
				body = append(body, pos()+"(X, Y)", "e(Y, "+strconv.Quote(propDomain[r.IntN(len(propDomain))])+")")
			}
			if r.IntN(3) == 0 {
				neg := "e"
				if i > 0 && r.IntN(2) == 0 {
					neg = "r" + strconv.Itoa(r.IntN(i))
				}
				body = append(body, "not "+neg+"(X, Y)")
			}
			if r.IntN(4) == 0 {
				body = append(body, "not f(Y)")
			}
			if r.IntN(5) == 0 {
				body = append(body, "X != Y")
			}
			if r.IntN(8) == 0 {
				body = append(body, `prefix("n", X)`)
			}
			fmt.Fprintf(&b, "%s(X, Y) :- %s.\n", head, strings.Join(body, ", "))
		}
	}
	k := strconv.Itoa(r.IntN(4))
	fmt.Fprintf(&b, "q(X) :- n(X), not r%s(X, X).\nq(X) :- r%s(X, _).\n", k, k)
	return b.String()
}

// propDemandBase is a random base over the domain.
func propDemandBase(r *rand.Rand) []tarski.Fact {
	var facts []tarski.Fact
	for _, x := range propDomain {
		facts = append(facts, tarski.Fact{Relation: "n", Tuple: tarski.Tuple{tarski.String(x)}})
		if r.IntN(4) == 0 {
			facts = append(facts, tarski.Fact{Relation: "f", Tuple: tarski.Tuple{tarski.String(x)}})
		}
		for _, y := range propDomain {
			if r.IntN(4) == 0 {
				facts = append(facts, tarski.Fact{Relation: "e", Tuple: tarski.Tuple{tarski.String(x), tarski.String(y)}})
			}
		}
	}
	return facts
}

// propRelations is the derived relations of propProgram, with their arities.
var propRelations = []relArity{{"r0", 2}, {"r1", 2}, {"r2", 2}, {"r3", 2}, {"q", 1}}

// relArity is a relation a goal may name, with its arity.
type relArity struct {
	name  string
	arity int
}

// propGoal is a random goal over one of rels: a random adornment, and constants from
// the domain or, one time in four, from outside it.
func propGoal(r *rand.Rand, rels []relArity) (relation, adornment string, goal tarski.Goal) {
	pick := rels[r.IntN(len(rels))]
	relation, arity := pick.name, pick.arity
	goal.Relation = relation
	for i := range arity {
		if r.IntN(2) == 0 {
			adornment += "f"
			goal.Args = append(goal.Args, tarski.Term{Var: "V" + strconv.Itoa(i)})
			continue
		}
		adornment += "b"
		c := "zz"
		if r.IntN(4) != 0 {
			c = propDomain[r.IntN(len(propDomain))]
		}
		goal.Args = append(goal.Args, tarski.Term{Const: tarski.String(c)})
	}
	return relation, adornment, goal
}

// filtered is full evaluation's answer for the goal's relation, filtered to its
// constants, sealed as an Answer through its own wire form.
func filtered(t *testing.T, full tarski.Result, goal tarski.Goal) tarski.Answer {
	t.Helper()
	all, err := full.Relation(goal.Relation)
	if err != nil {
		t.Fatal(err)
	}
	keep := []tarski.Tuple{}
	for _, tup := range all.Tuples() {
		if agrees(goal, tup) {
			keep = append(keep, tup)
		}
	}
	b, err := json.Marshal(struct {
		Relation string         `json:"relation"`
		Tuples   []tarski.Tuple `json:"tuples"`
	}{goal.Relation, keep})
	if err != nil {
		t.Fatal(err)
	}
	var a tarski.Answer
	if err := json.Unmarshal(b, &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// agrees reports whether tup holds the goal's constants.
func agrees(goal tarski.Goal, tup tarski.Tuple) bool {
	for i, a := range goal.Args {
		if !a.IsVar() && (a.Const.Kind() != tup[i].Kind() || a.Const.Text() != tup[i].Text()) {
			return false
		}
	}
	return true
}

// demandCase is one seed's program, base and goal, evaluated in full and under demand,
// with the program's derived and base relations and the constants a why-not tries.
type demandCase struct {
	text          string
	goal          tarski.Goal
	full, demand  tarski.Result
	derived, base []string
	domain        []tarski.Const
}

// propStrings is the domain as constants.
func propStrings() []tarski.Const {
	out := make([]tarski.Const, len(propDomain))
	for i, x := range propDomain {
		out[i] = tarski.String(x)
	}
	return out
}

// newDemandCase builds and evaluates the case for a seed. Every program the generator
// makes stratifies, so Demand compiles every one: a rewrite that breaks stratification
// answers through the fallback (design §2.4).
func newDemandCase(t *testing.T, seed uint64) (c demandCase) {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, 0xde3a))
	c.text = propProgram(r)
	c.derived, c.base, c.domain = []string{"r0", "r1", "r2", "r3", "q"}, []string{"e", "n", "f"}, propStrings()
	plan, err := tarski.Compile(propDemandSchema, parse(t, c.text))
	if err != nil {
		t.Fatalf("seed %d: Compile: %v\n%s", seed, err, c.text)
	}
	base, err := tarski.NewBase(propDemandSchema, propDemandBase(r)...)
	if err != nil {
		t.Fatal(err)
	}
	relation, adornment, goal := propGoal(r, propRelations)
	c.goal = goal
	dplan, err := plan.Demand(relation, adornment)
	if err != nil {
		t.Fatalf("seed %d: Demand(%s, %s): %v\n%s", seed, relation, adornment, err, c.text)
	}
	if c.full, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true}); err != nil {
		t.Fatalf("seed %d: full: %v", seed, err)
	}
	if c.demand, err = tarski.Evaluate(context.Background(), dplan, base, tarski.Options{Proofs: true, Goal: &goal}); err != nil {
		t.Fatalf("seed %d: demand: %v\n%s", seed, err, c.text)
	}
	return c
}

// checkAnswers holds the case to D1 and to the refusals of §3.2. It returns how many
// tuples the goal's answer held.
func (c demandCase) checkAnswers(t *testing.T, seed uint64) int {
	t.Helper()
	want := filtered(t, c.full, c.goal)
	got, err := c.demand.Relation(c.goal.Relation)
	if err != nil {
		t.Fatalf("seed %d: demand Relation(%s): %v", seed, c.goal.Relation, err)
	}
	if string(got.Bytes()) != string(want.Bytes()) || got.Hash() != want.Hash() {
		t.Fatalf("seed %d: goal %v\n%s\ndemand %s\nfull   %s", seed, c.goal, c.text, got.Bytes(), want.Bytes())
	}
	for _, name := range c.derived {
		if name == c.goal.Relation {
			continue
		}
		if _, err := c.demand.Relation(name); !errors.Is(err, tarski.ErrInvalid) {
			t.Fatalf("seed %d: Relation(%s) on a demand result = %v, want ErrInvalid", seed, name, err)
		}
	}
	for _, name := range c.base {
		a, _ := c.full.Relation(name)
		b, err := c.demand.Relation(name)
		if err != nil || a.Hash() != b.Hash() {
			t.Fatalf("seed %d: base relation %s differs under demand: %v", seed, name, err)
		}
	}
	return got.Len()
}

// checkProofs holds every proof of the goal's answer to D11's validity: a source rule,
// and body tuples full evaluation holds.
func (c demandCase) checkProofs(t *testing.T, seed uint64) {
	t.Helper()
	got, _ := c.demand.Relation(c.goal.Relation)
	for _, tup := range got.Tuples() {
		p, ok := c.demand.Proof(c.goal.Relation, tup)
		if !ok || p.Rule == nil {
			t.Fatalf("seed %d: no proof of %s%v under demand", seed, c.goal.Relation, tup)
		}
		if !strings.Contains(c.text, p.Rule.Text) {
			t.Fatalf("seed %d: proof rule %q is not a source rule", seed, p.Rule.Text)
		}
		for _, body := range p.Body {
			if _, ok := c.full.Proof(body.Relation, body.Tuple); !ok {
				t.Fatalf("seed %d: proof body %s%v is not in the full result", seed, body.Relation, body.Tuple)
			}
		}
		if p.GroupSize < len(p.Group) {
			t.Fatalf("seed %d: proof of %s%v keeps %d witnesses of a group of %d", seed, c.goal.Relation, tup, len(p.Group), p.GroupSize)
		}
		for _, w := range p.Group {
			for _, wt := range w {
				if _, ok := c.full.Proof(wt.Relation, wt.Tuple); !ok {
					t.Fatalf("seed %d: proof witness %s%v is not in the full result", seed, wt.Relation, wt.Tuple)
				}
			}
		}
	}
}

// checkWhyNot compares why-not under demand and in full for tuples of the goal's
// relation that agree with the goal and were not derived: the same rules, and the same
// failing literal in each.
func (c demandCase) checkWhyNot(t *testing.T, seed uint64) {
	t.Helper()
	for _, tup := range candidates(c.goal, c.domain) {
		if _, in := c.full.Proof(c.goal.Relation, tup); in {
			continue
		}
		d, okD := c.demand.WhyNot(c.goal.Relation, tup, 0)
		f, okF := c.full.WhyNot(c.goal.Relation, tup, 0)
		if !okD || !okF || len(d.Rules) != len(f.Rules) {
			t.Fatalf("seed %d: WhyNot(%v) ok %v/%v rules %d/%d", seed, tup, okD, okF, len(d.Rules), len(f.Rules))
		}
		for i := range d.Rules {
			if d.Rules[i].Rule.Text != f.Rules[i].Rule.Text || !reflect.DeepEqual(d.Rules[i].Failed, f.Rules[i].Failed) {
				t.Fatalf("seed %d: WhyNot(%v) rule %d fails at %+v under demand, %+v in full\n%s",
					seed, tup, i, d.Rules[i].Failed, f.Rules[i].Failed, c.text)
			}
		}
	}
}

// candidates is every tuple over domain that agrees with the goal's constants.
func candidates(goal tarski.Goal, domain []tarski.Const) []tarski.Tuple {
	out := []tarski.Tuple{{}}
	for _, a := range goal.Args {
		var next []tarski.Tuple
		for _, prefix := range out {
			if !a.IsVar() {
				next = append(next, append(append(tarski.Tuple{}, prefix...), a.Const))
				continue
			}
			for _, x := range domain {
				next = append(next, append(append(tarski.Tuple{}, prefix...), x))
			}
		}
		out = next
	}
	return out
}

func TestDemand_AgreesWithFullEvaluation(t *testing.T) {
	t.Parallel()
	tuples := 0
	for seed := range uint64(400) {
		c := newDemandCase(t, seed)
		tuples += c.checkAnswers(t, seed)
		c.checkProofs(t, seed)
		c.checkWhyNot(t, seed)
	}
	t.Logf("400 programs evaluated under demand, %d goal tuples", tuples)
	if tuples < 300 {
		t.Errorf("%d goal tuples in all; the property is not being exercised", tuples)
	}
}

// FuzzDemandMatchesFull is the property above, and its aggregate form
// (demand_aggregate_property_test.go), over seeds the fuzzer picks.
func FuzzDemandMatchesFull(f *testing.F) {
	for _, s := range []uint64{0, 1, 7, 42, 1 << 40} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, seed uint64) {
		for _, c := range []demandCase{newDemandCase(t, seed), newAggDemandCase(t, seed)} {
			c.checkAnswers(t, seed)
			c.checkProofs(t, seed)
		}
	})
}
