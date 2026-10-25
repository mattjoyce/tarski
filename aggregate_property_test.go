package tarski_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// The aggregates against a program without them, over random bases (seeded, so a
// failure replays). The witnesses of the group G are the tuples of
//
//	w(G, V, F) :- grp(G), item(G, V), tag(V, F), not bad(V).
//
// whose F stands for the _ inside the braces, so a value tagged twice is two witnesses.
// count is how many there are, sum the exact fold of V over them, and min and max the
// ends of their V sorted. Values are tenths, so the expected sum is exact integer
// arithmetic in tenths.
const (
	propAggregates = `
n(G, N) :- grp(G), N = count : { item(G, V), tag(V, _), not bad(V) }.
s(G, S) :- grp(G), S = sum V : { item(G, V), tag(V, _), not bad(V) }.
lo(G, M) :- grp(G), M = min V : { item(G, V), tag(V, _), not bad(V) }.
hi(G, M) :- grp(G), M = max V : { item(G, V), tag(V, _), not bad(V) }.
`
	propWitnesses = `w(G, V, F) :- grp(G), item(G, V), tag(V, F), not bad(V).`
)

// tenths writes a count of tenths as a number lexeme: 25 is 2.5, -3 is -0.3.
func tenths(n int) string {
	sign := ""
	if n < 0 {
		sign, n = "-", -n
	}
	return fmt.Sprintf("%s%d.%d", sign, n/10, n%10)
}

// parseTenths reads a canonical number text with at most one decimal place as tenths.
func parseTenths(t *testing.T, text string) int {
	t.Helper()
	neg := strings.HasPrefix(text, "-")
	whole, frac, _ := strings.Cut(strings.TrimPrefix(text, "-"), ".")
	n, err := strconv.Atoi(whole + frac + strings.Repeat("0", 1-len(frac)))
	if err != nil {
		t.Fatalf("parseTenths(%q): %v", text, err)
	}
	if neg {
		n = -n
	}
	return n
}

// propBase is a random base: up to four groups, items with values in tenths between
// -5 and 5, tags on some values (some twice) and some values bad.
func propBase(r *rand.Rand) []tarski.Fact {
	var facts []tarski.Fact
	num := func(n int) tarski.Const {
		c, err := tarski.Number(tenths(n))
		if err != nil {
			panic(err)
		}
		return c
	}
	groups := 1 + r.IntN(4)
	for g := range groups {
		facts = append(facts, tarski.Fact{Relation: "grp", Tuple: tarski.Tuple{tarski.String("g" + strconv.Itoa(g))}})
		for range r.IntN(6) {
			facts = append(facts, tarski.Fact{Relation: "item", Tuple: tarski.Tuple{tarski.String("g" + strconv.Itoa(g)), num(r.IntN(101) - 50)}})
		}
	}
	for v := -50; v <= 50; v++ {
		for f := range r.IntN(3) {
			facts = append(facts, tarski.Fact{Relation: "tag", Tuple: tarski.Tuple{num(v), tarski.String("f" + strconv.Itoa(f))}})
		}
		if r.IntN(8) == 0 {
			facts = append(facts, tarski.Fact{Relation: "bad", Tuple: tarski.Tuple{num(v)}})
		}
	}
	return facts
}

// propExpected is each group's count, sum, min and max as row strings, computed from
// the witnesses w derived without aggregates.
func propExpected(t *testing.T, w tarski.Answer, groups []string) (n, s, lo, hi []string) {
	byGroup := map[string][]int{}
	for _, tup := range w.Tuples() {
		byGroup[tup[0].Text()] = append(byGroup[tup[0].Text()], parseTenths(t, tup[1].Text()))
	}
	canon := func(lexeme string) string {
		c, err := tarski.Number(lexeme)
		if err != nil {
			t.Fatal(err)
		}
		return c.Text()
	}
	for _, g := range groups {
		vs := byGroup[g]
		total := 0
		for _, v := range vs {
			total += v
		}
		n = append(n, g+" "+strconv.Itoa(len(vs)))
		s = append(s, g+" "+canon(tenths(total)))
		if len(vs) > 0 {
			lo = append(lo, g+" "+canon(tenths(slices.Min(vs))))
			hi = append(hi, g+" "+canon(tenths(slices.Max(vs))))
		}
	}
	return n, s, lo, hi
}

func TestAggregates_AgreeWithTheProgramWithoutThem(t *testing.T) {
	t.Parallel()
	schema := tarski.Schema{Arities: map[string]int{"grp": 1, "item": 2, "tag": 2, "bad": 1}}
	plan, err := tarski.Compile(schema, parse(t, propAggregates+propWitnesses))
	if err != nil {
		t.Fatal(err)
	}
	witnesses := 0
	defer func() {
		if witnesses < 100 {
			t.Errorf("the random bases gave %d witnesses in all; the property is not being exercised", witnesses)
		}
	}()
	for seed := range uint64(60) {
		r := rand.New(rand.NewPCG(seed, 0x7a75))
		base, err := tarski.NewBase(schema, propBase(r)...)
		if err != nil {
			t.Fatal(err)
		}
		res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		answer := func(name string) tarski.Answer {
			a, err := res.Relation(name)
			if err != nil {
				t.Fatal(err)
			}
			return a
		}
		var groups []string
		for _, tup := range answer("n").Tuples() {
			groups = append(groups, tup[0].Text())
		}
		if len(groups) != base.Len("grp") {
			t.Fatalf("seed %d: n has %d groups, want one per grp fact (%d)", seed, len(groups), base.Len("grp"))
		}
		witnesses += answer("w").Len()
		n, s, lo, hi := propExpected(t, answer("w"), groups)
		for _, c := range []struct {
			name string
			want []string
		}{{"n", n}, {"s", s}, {"lo", lo}, {"hi", hi}} {
			if got := rows(answer(c.name)); !same(got, c.want) {
				t.Errorf("seed %d: %s = %s, want %s", seed, c.name, strings.Join(got, "; "), strings.Join(c.want, "; "))
			}
		}
		again, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"n", "s", "lo", "hi"} {
			a, _ := again.Relation(name)
			if a.Hash() != answer(name).Hash() {
				t.Errorf("seed %d: %s hashes differ between two evaluations", seed, name)
			}
		}
	}
}
