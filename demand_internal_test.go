package tarski

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// A rule rewritten for a goal records its source as the proof's rule and leaves its
// demand literal out of the proof's body (design §2.5): here the plan's own rule is
// dressed as a rewrite by hand, so the mechanism is tested apart from the rewrite.
func TestRecord_NamesTheSourceAndSkipsTheDemandLiteral(t *testing.T) {
	t.Parallel()
	schema := Schema{Arities: map[string]int{"node": 1, "score": 2}}
	plan, err := Compile(schema, mustParse(t, "d(X) :- node(X).\np(X) :- d(X), score(X, _).\n"))
	if err != nil {
		t.Fatal(err)
	}
	src := &Rule{Head: Atom{Name: "p", Args: []Term{{Var: "X"}}}, Line: 9, Text: "p(X) :- score(X, _)."}
	found := false
	for s := range plan.strata {
		for i := range plan.strata[s] {
			if r := &plan.strata[s][i]; r.Head.Name == "p" {
				r.source, r.demand, found = src, 0, true
			}
		}
	}
	if !found {
		t.Fatal("no rule for p in the plan")
	}
	base, err := NewBase(schema, Fact{"node", Tuple{String("a")}}, Fact{"score", Tuple{String("a"), Int(3)}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := Evaluate(context.Background(), plan, base, Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	p, ok := res.Proof("p", Tuple{String("a")})
	if !ok {
		t.Fatal("no proof of p(a)")
	}
	if p.Rule.Text != src.Text || p.Rule.Line != 9 {
		t.Errorf("proof rule = %q line %d, want the source %q", p.Rule.Text, p.Rule.Line, src.Text)
	}
	if len(p.Body) != 1 || p.Body[0].Relation != "score" {
		t.Errorf("proof body = %+v, want only score: the demand literal is not listed", p.Body)
	}
}

// visible names the source relation in place of a hidden one, and changes nothing else.
func TestVisible_MapsHiddenRelations(t *testing.T) {
	t.Parallel()
	demand := Plan{demand: &demandPlan{hidden: map[string]string{"m?p?b": "p"}}}
	cases := []struct {
		name string
		plan Plan
		err  error
		want string
	}{
		{"hidden name under demand", demand, &TooLarge{Bound: BoundSteps, Relation: "m?p?b"}, "p"},
		{"wrapped hidden name", demand, fmt.Errorf("x: %w", &TooLarge{Relation: "m?p?b"}), "p"},
		{"source name under demand", demand, &TooLarge{Relation: "q"}, "q"},
		{"plain plan", Plan{}, &TooLarge{Relation: "m?p?b"}, "m?p?b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var tl *TooLarge
			if !errors.As(tc.plan.visible(tc.err), &tl) || tl.Relation != tc.want {
				t.Fatalf("visible(%v) relation = %v, want %s", tc.err, tl, tc.want)
			}
		})
	}
	plain := errors.New("other")
	if got := demand.visible(plain); !errors.Is(got, plain) || got.Error() != "other" {
		t.Errorf("visible changed an error that is no *TooLarge: %v", got)
	}
}

// place keeps a body the caller ordered and marks every position over the stratum's own
// relations, a leading literal included.
func TestPlace_KeepsAnOrderedBody(t *testing.T) {
	t.Parallel()
	lit := func(name string) Literal { return Literal{Atom: &Atom{Name: name, Args: []Term{{Var: "X"}}}} }
	pr := rule{Rule: Rule{Head: Atom{Name: "p", Args: []Term{{Var: "X"}}}}, ordered: []Literal{lit("m"), lit("e"), lit("p")}, demand: 0}
	strata := place([]rule{pr}, map[string]int{"p": 1, "m": 1}, map[string]bool{"p": true, "m": true})
	if len(strata) != 2 || len(strata[1]) != 1 {
		t.Fatalf("strata = %v", strata)
	}
	got := strata[1][0]
	if len(got.ordered) != 3 || got.ordered[0].Atom.Name != "m" || fmt.Sprint(got.recursive) != "[0 2]" {
		t.Errorf("ordered %v recursive %v, want the body kept and positions [0 2]", got.ordered, got.recursive)
	}
}

// rbacUnseeded is Shearline's rbac rules without the request seeds (design §2.2).
const rbacUnseeded = `holds(P, R) :- role(P, R).
holds(P, R) :- holds(P, Q), inherits(Q, R).
permit(P, A, Pat) :- holds(P, Role), grants(Role, A, Pat).
deny(P, A, R) :- denied(P, A, R).
allow(P, A, R) :- permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R).
`

var rbacSchema = Schema{Arities: map[string]int{"role": 2, "inherits": 2, "grants": 3, "denied": 3, "resource": 1}}

// rewritten renders a demand plan's rules, stratum by stratum, as head :- ordered body.
func rewritten(d *demandPlan) []string {
	var out []string
	for s, stratum := range d.strata {
		for _, r := range stratum {
			body := make([]string, len(r.ordered))
			for i, l := range r.ordered {
				body[i] = l.text()
			}
			out = append(out, fmt.Sprintf("%d %s :- %s.", s, r.Head.text(), strings.Join(body, ", ")))
		}
	}
	return out
}

// The rewrite of design §2.2 and the adornment rules of §2.1, pinned: demand passes
// through the head's bound positions and what earlier positive atoms bind, a constant is
// bound and _ is open, a demand rule keeps only the atoms that bind what it needs, and
// the all-free adornment is the source rule with no demand relation.
func TestRewrite_Shapes(t *testing.T) {
	t.Parallel()
	graph := Schema{Arities: map[string]int{"e": 2}}
	cases := []struct {
		name, text, relation, adornment string
		schema                          Schema
		want                            []string
	}{
		{"rbac", rbacUnseeded, "allow", "bbb", rbacSchema, []string{
			"0 m?permit?bbf(P, A) :- m?allow?bbb(P, A, R).",
			"0 m?deny?bbb(P, A, R) :- m?allow?bbb(P, A, R).",
			"0 permit(P, A, Pat) :- m?permit?bbf(P, A), holds(P, Role), grants(Role, A, Pat).",
			"0 m?holds?bf(P) :- m?permit?bbf(P, A).",
			"0 deny(P, A, R) :- m?deny?bbb(P, A, R), denied(P, A, R).",
			"0 holds(P, R) :- m?holds?bf(P), role(P, R).",
			"0 holds(P, R) :- m?holds?bf(P), holds(P, Q), inherits(Q, R).",
			"1 allow(P, A, R) :- m?allow?bbb(P, A, R), permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R).",
		}},
		{"swap and constant", `r(X, Y) :- s(Y, X). s(X, Y) :- e(X, Y), t(Y, "c"). t(X, Y) :- e(X, Y), w(Y, _). w(X, Y) :- e(X, Y).`,
			"r", "bf", graph, []string{
				"0 r(X, Y) :- m?r?bf(X), s(Y, X).",
				"0 m?s?fb(X) :- m?r?bf(X).",
				`0 s(X, Y) :- m?s?fb(Y), e(X, Y), t(Y, "c").`,
				`0 m?t?bb(Y, "c") :- m?s?fb(Y).`,
				"0 t(X, Y) :- m?t?bb(X, Y), e(X, Y), w(Y, _).",
				"0 m?w?bf(Y) :- m?t?bb(X, Y).",
				"0 w(X, Y) :- m?w?bf(X), e(X, Y).",
			}},
		{"all free", `r(X, Y) :- e(X, Z), s(Z, Y). s(X, Y) :- e(X, Y).`, "r", "ff", graph, []string{
			"0 r(X, Y) :- e(X, Z), s(Z, Y).",
			"0 m?s?bf(Z) :- e(X, Z).",
			"0 s(X, Y) :- m?s?bf(X), e(X, Y).",
		}},
		{"recursion through the same binding", `p(X, Y) :- e(X, Y). p(X, Y) :- p(X, Z), e(Z, Y).`, "p", "bf", graph, []string{
			"0 p(X, Y) :- m?p?bf(X), e(X, Y).",
			"0 p(X, Y) :- m?p?bf(X), p(X, Z), e(Z, Y).",
		}},
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
			again, _ := plan.Demand(tc.relation, tc.adornment)
			if strings.Join(rewritten(again.demand), "\n") != got {
				t.Error("two rewrites of one plan for one goal shape differ")
			}
		})
	}
}
