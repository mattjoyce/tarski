package tarski

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// aggCycleText is a recursion whose demand reaches an aggregate's braces and comes back.
const aggCycleText = `p(X, Y) :- e(X, Y). p(X, Y) :- p(X, Z), c(Z, N), N > 1, e(Z, Y). ` +
	`c(Z, N) :- e(Z, _), N = count : { s(Z, W) }. s(Z, W) :- e(Z, W), e(W, Z).`

// The fallback of design §2.4, pinned: when the rewrite cannot be stratified, demand is
// dropped for the source relation of the first demand relation, by sorted name, in the
// first component holding a negative edge, and the rewrite runs again. The rewrite text
// and the order of dropping are a function of the plan and the goal shape (D5).
func TestRewrite_Fallback(t *testing.T) {
	t.Parallel()
	graph := Schema{Arities: map[string]int{"e": 2}}
	cases := []struct {
		name, text, relation, adornment string
		schema                          Schema
		dropped, seed                   string
		want                            []string
	}{
		{
			// m?r0?bb(X, Y) :- m?r1?bf(X), r1(X, Z), e(Z, Y) puts r0's demand under r1, which
			// negates r0: the component {m?r0?bb, r0, r1} holds the negation, and r0 goes.
			"demand under a negation", `r0(X, Y) :- e(X, Y). r1(X, Y) :- e(Y, X). r1(X, Y) :- r1(X, Z), e(Z, Y), not r0(X, Y).`,
			"r1", "bf", graph, "r0", "m?r1?bf", []string{
				"0 r0(X, Y) :- e(X, Y).",
				"1 r1(X, Y) :- m?r1?bf(X), e(Y, X).",
				"1 r1(X, Y) :- m?r1?bf(X), r1(X, Z), e(Z, Y), not r0(X, Y).",
			},
		},
		{
			// The cycle m?a?bf -> b -> not c -> m?c?bb -> m?b?bf -> m?a?bf: m?a?bf sorts first, so
			// the goal's own relation is derived whole and has no seed; c, below it, keeps the
			// demand b's rule passes it.
			"the goal's own demand dropped", `a(X, Y) :- e(X, Y). a(X, Y) :- b(X, Z), a(Z, Y). b(X, Z) :- e(X, Z), not c(X, Z). ` +
				`c(X, Z) :- e(X, Z), e(Z, X).`,
			"a", "bf", graph, "a", "", []string{
				"0 m?c?bb(X, Z) :- e(X, Z).",
				"0 c(X, Z) :- m?c?bb(X, Z), e(X, Z), e(Z, X).",
				"1 a(X, Y) :- e(X, Y).",
				"1 a(X, Y) :- b(X, Z), a(Z, Y).",
				"1 b(X, Z) :- e(X, Z), not c(X, Z).",
			},
		},
		{
			// An aggregate's edge is negative too: p passes demand to c, c's braces pass it to
			// s, and p reads c, so m?c?bf -> p -> c -> (aggregate) s -> m?s?bf -> m?c?bf. c goes,
			// and s keeps the demand c's whole rule passes it through the group variable.
			"demand through an aggregate cycle", aggCycleText, "p", "bf", graph, "c", "m?p?bf", []string{
				"0 m?s?bf(Z) :- e(Z, _).",
				"0 s(Z, W) :- m?s?bf(Z), e(Z, W), e(W, Z).",
				"1 p(X, Y) :- m?p?bf(X), e(X, Y).",
				"1 p(X, Y) :- m?p?bf(X), p(X, Z), c(Z, N), N > 1, e(Z, Y).",
				"1 c(Z, N) :- e(Z, _), N = count : { s(Z, W) }.",
			},
		},
		{
			// Design §1.2's example stratifies under §2.2's minimal prefix: nothing is dropped.
			"design §1.2", `y(A) :- d(A). x(A) :- b(A), not y(A). w(A) :- x(A). h(A) :- x(A), f(A), not w(A). g(A) :- w(A), h(A).`,
			"g", "b", Schema{Arities: map[string]int{"d": 1, "b": 1, "f": 1}}, "", "m?g?b", []string{
				"0 m?w?b(A) :- m?g?b(A).",
				"0 m?h?b(A) :- m?g?b(A).",
				"0 m?x?b(A) :- m?w?b(A).",
				"0 m?x?b(A) :- m?h?b(A).",
				"0 m?w?b(A) :- m?h?b(A).",
				"0 m?y?b(A) :- m?x?b(A).",
				"0 y(A) :- m?y?b(A), d(A).",
				"1 w(A) :- m?w?b(A), x(A).",
				"1 x(A) :- m?x?b(A), b(A), not y(A).",
				"2 g(A) :- m?g?b(A), w(A), h(A).",
				"2 h(A) :- m?h?b(A), x(A), not w(A), f(A).",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := Compile(tc.schema, mustParse(t, tc.text))
			if err != nil {
				t.Fatal(err)
			}
			d, err := plan.Demand(tc.relation, tc.adornment)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Join(rewritten(d.demand), "\n")
			if want := strings.Join(tc.want, "\n"); got != want {
				t.Errorf("rewrite =\n%s\nwant\n%s", got, want)
			}
			if dropped := strings.Join(d.demand.dropped, ", "); dropped != tc.dropped || d.demand.seed != tc.seed {
				t.Errorf("dropped %q seed %q, want %q and %q", dropped, d.demand.seed, tc.dropped, tc.seed)
			}
			for range 3 {
				again, _ := plan.Demand(tc.relation, tc.adornment)
				if strings.Join(rewritten(again.demand), "\n") != got || strings.Join(again.demand.dropped, ", ") != tc.dropped {
					t.Fatal("two rewrites of one plan for one goal shape differ")
				}
			}
		})
	}
}

// adornmentProgram is g(H) reading p/7 under the binding patterns masks name: bit i set
// puts the constant "a" at position i, and a clear bit a variable of its own.
func adornmentProgram(masks int) string {
	var b strings.Builder
	b.WriteString("p(A, B, C, D, E, F, G) :- s(A, B, C, D, E, F, G).\n")
	for m := 1; m <= masks; m++ {
		args := make([]string, 7)
		for i := range args {
			args[i] = fmt.Sprintf("V%d", i)
			if m&(1<<i) != 0 {
				args[i] = `"a"`
			}
		}
		fmt.Fprintf(&b, "g(H) :- n(H), p(%s).\n", strings.Join(args, ", "))
	}
	return b.String()
}

// A relation reached under more than maxAdornments binding patterns has its demand
// dropped and is derived whole; at the cap it keeps every pattern. Either way the answer
// is full evaluation's.
func TestRewrite_AdornmentCap(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"n": 1, "s": 7}}
	var facts []Fact
	for _, x := range []string{"a", "b"} {
		facts = append(facts, Fact{"n", Tuple{String(x)}})
		facts = append(facts, Fact{"s", Tuple{String(x), String("a"), String("a"), String(x), String("a"), String("a"), String("a")}})
	}
	base, err := NewBase(schema, facts...)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		masks   int
		dropped string
		hidden  int
	}{{maxAdornments, "", maxAdornments}, {maxAdornments + 1, "p", 0}} {
		t.Run(fmt.Sprint(tc.masks), func(t *testing.T) {
			plan, err := Compile(schema, mustParse(t, adornmentProgram(tc.masks)))
			if err != nil {
				t.Fatal(err)
			}
			d, err := plan.Demand("g", "f")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(d.demand.dropped, ", "); got != tc.dropped || len(d.demand.hidden) != tc.hidden {
				t.Fatalf("dropped %q with %d demand relations, want %q and %d", got, len(d.demand.hidden), tc.dropped, tc.hidden)
			}
			full, err := Evaluate(context.Background(), plan, base, Options{})
			if err != nil {
				t.Fatal(err)
			}
			res, err := Evaluate(context.Background(), d, base, Options{Goal: &Goal{Relation: "g", Args: []Term{{Var: "X"}}}})
			if err != nil {
				t.Fatal(err)
			}
			want, _ := full.Relation("g")
			got, _ := res.Relation("g")
			if got.Hash() != want.Hash() || got.Len() == 0 {
				t.Errorf("g under demand = %s, full %s", got.Bytes(), want.Bytes())
			}
		})
	}
}
