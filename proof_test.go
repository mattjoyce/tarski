package datalog_test

import (
	"context"
	"fmt"
	"reflect"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// evalProofs compiles text over the test schema and evaluates it over facts.
func evalProofs(t testing.TB, facts []datalog.Fact, text string, opts datalog.Options) datalog.Result {
	t.Helper()
	b, err := datalog.NewBase(testSchema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	prog, err := datalog.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	plan, err := datalog.Compile(testSchema, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := datalog.Evaluate(context.Background(), plan, b, opts)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	return res
}

func tup(f datalog.Fact) datalog.Tuple { return f.Tuple }

func chain(n int) []datalog.Fact {
	var facts []datalog.Fact
	for i := 0; i < n; i++ {
		facts = append(facts, fact("edge", fmt.Sprint("n", i), fmt.Sprint("n", i+1)))
	}
	return facts
}

const reachRules = `
reaches(X, Y) :- edge(X, Y).
reaches(X, Z) :- reaches(X, Y), edge(Y, Z).
`

func TestProof_BaseFactIsALeaf(t *testing.T) {
	t.Parallel()
	f := fact("edge", "a", "b")
	res := evalProofs(t, []datalog.Fact{f}, reachRules, datalog.Options{Proofs: true})
	p, ok := res.Proof("edge", tup(f))
	if !ok || p.Rule != nil || p.Body != nil || p.Relation != "edge" || !reflect.DeepEqual(p.Tuple, tup(f)) {
		t.Errorf("Proof(edge a b) = %+v, %v, want a leaf", p, ok)
	}
}

func TestProof_OneStepNamesTheRuleAndTheBodyTuple(t *testing.T) {
	t.Parallel()
	e := fact("edge", "a", "b")
	res := evalProofs(t, []datalog.Fact{e}, "\nreaches(X, Y) :- edge(X, Y).\n", datalog.Options{Proofs: true})
	p, ok := res.Proof("reaches", tup(fact("reaches", "a", "b")))
	if !ok {
		t.Fatal("no proof of reaches(a, b)")
	}
	if p.Rule == nil || p.Rule.Line != 2 {
		t.Fatalf("Rule = %+v, want the rule on line 2", p.Rule)
	}
	if len(p.Body) != 1 || p.Body[0].Relation != "edge" || !reflect.DeepEqual(p.Body[0].Tuple, tup(e)) || p.Body[0].Rule != nil {
		t.Errorf("Body = %+v, want the edge(a, b) leaf", p.Body)
	}
}

// Transitive closure over a chain of 50: every proof is finite, bottoms out in base
// facts only, and names no tuple twice on one path.
func TestProof_ThroughRecursion(t *testing.T) {
	t.Parallel()
	res := evalProofs(t, chain(50), reachRules, datalog.Options{Proofs: true})
	p, ok := res.Proof("reaches", tup(fact("reaches", "n0", "n50")))
	if !ok {
		t.Fatal("no proof of reaches(n0, n50)")
	}
	leaves := 0
	var walk func(p datalog.Proof, path map[string]bool)
	walk = func(p datalog.Proof, path map[string]bool) {
		k := p.Relation + "/" + fmt.Sprint(p.Tuple)
		if path[k] {
			t.Fatalf("%s repeats on a root-to-leaf path", k)
		}
		path[k] = true
		defer delete(path, k)
		if p.Rule == nil {
			leaves++
			if p.Relation != "edge" {
				t.Errorf("leaf %s is not a base fact", k)
			}
			return
		}
		if len(p.Body) == 0 {
			t.Errorf("derived %s has no body", k)
		}
		for _, b := range p.Body {
			walk(b, path)
		}
	}
	walk(p, map[string]bool{})
	if leaves != 50 {
		t.Errorf("leaves = %d, want the 50 edges of the chain", leaves)
	}
}

// Negation, a comparison and a built-in guard the rule and contribute no body entry;
// the positive atoms come in the order written, whatever order the guards run in.
func TestProof_ListsOnlyPositiveAtoms(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{
		fact("node", "ax"), fact("node", "bx"), fact("score", "ax", 5), fact("score", "bx", 1),
		fact("edge", "ax", "bx"),
	}
	res := evalProofs(t, facts, `
		pick(N, S) :- score(N, S), S > 2, not edge(N, "zz"), prefix("a", N), node(N).
	`, datalog.Options{Proofs: true})
	p, ok := res.Proof("pick", tup(fact("pick", "ax", 5)))
	if !ok {
		t.Fatal("no proof of pick(ax, 5)")
	}
	if len(p.Body) != 2 || p.Body[0].Relation != "score" || p.Body[1].Relation != "node" {
		t.Errorf("Body = %+v, want score then node only", p.Body)
	}
	if _, ok := res.Proof("pick", tup(fact("pick", "bx", 1))); ok {
		t.Error("a tuple that was not derived has a proof")
	}
}

func TestProof_Absent(t *testing.T) {
	t.Parallel()
	facts := chain(2)
	off := evalProofs(t, facts, reachRules, datalog.Options{})
	for _, c := range []struct{ rel, a, b string }{{"edge", "n0", "n1"}, {"reaches", "n0", "n2"}} {
		if _, ok := off.Proof(c.rel, tup(fact(c.rel, c.a, c.b))); ok {
			t.Errorf("Proofs off: %s(%s, %s) has a proof", c.rel, c.a, c.b)
		}
	}
	on := evalProofs(t, facts, reachRules, datalog.Options{Proofs: true})
	for _, c := range []struct{ rel, a, b string }{{"reaches", "n2", "n0"}, {"edge", "x", "y"}, {"nothing", "a", "b"}} {
		if _, ok := on.Proof(c.rel, tup(fact(c.rel, c.a, c.b))); ok {
			t.Errorf("%s(%s, %s) is not in the result but has a proof", c.rel, c.a, c.b)
		}
	}
}

// A program fact is a rule with no body: derived, so it names its rule.
func TestProof_ProgramFact(t *testing.T) {
	t.Parallel()
	res := evalProofs(t, nil, `kind("bp").`, datalog.Options{Proofs: true})
	p, ok := res.Proof("kind", tup(fact("kind", "bp")))
	if !ok || p.Rule == nil || p.Body != nil {
		t.Errorf("Proof(kind bp) = %+v, %v, want a rule with no body", p, ok)
	}
}

// A tuple first derived in round 2 or later (the semi-naive delta path) has a proof
// whose body tuples are all proved, and no tuple repeats on a path. Its body tuples
// come from earlier rounds, so the longer the path the deeper the proof.
func TestProof_DeltaRounds(t *testing.T) {
	t.Parallel()
	res := evalProofs(t, chain(6), reachRules, datalog.Options{Proofs: true})
	ans, err := res.Relation("reaches")
	if err != nil {
		t.Fatal(err)
	}
	for _, tu := range ans.Tuples {
		p, ok := res.Proof("reaches", tu)
		if !ok {
			t.Fatalf("no proof of reaches%v", tu)
		}
		var check func(p datalog.Proof, path map[string]bool)
		check = func(p datalog.Proof, path map[string]bool) {
			k := p.Relation + fmt.Sprint(p.Tuple)
			if path[k] {
				t.Fatalf("%s repeats on a path", k)
			}
			path[k] = true
			defer delete(path, k)
			for _, b := range p.Body {
				if sub, ok := res.Proof(b.Relation, b.Tuple); !ok || !reflect.DeepEqual(sub, b) {
					t.Errorf("body tuple %s%v of %s is not proved as itself", b.Relation, b.Tuple, k)
				}
				check(b, path)
			}
		}
		check(p, map[string]bool{})
	}
}

func TestProof_Deterministic(t *testing.T) {
	t.Parallel()
	facts := append(chain(12), fact("edge", "n3", "n9"), fact("edge", "n9", "n2"), fact("edge", "n0", "n5"))
	text := reachRules + `
		twice(X, Y) :- reaches(X, Y), reaches(Y, X).
		both(X) :- twice(X, _), node(X).
	`
	a := evalProofs(t, facts, text, datalog.Options{Proofs: true})
	b := evalProofs(t, facts, text, datalog.Options{Proofs: true})
	for _, rel := range []string{"reaches", "twice", "both"} {
		ans, err := a.Relation(rel)
		if err != nil {
			t.Fatal(err)
		}
		for _, tu := range ans.Tuples {
			pa, oka := a.Proof(rel, tu)
			pb, okb := b.Proof(rel, tu)
			if !oka || !okb || !reflect.DeepEqual(pa, pb) {
				t.Fatalf("proof of %s%v differs between two evaluations", rel, tu)
			}
		}
	}
}

// In a diamond a -> b -> d and a -> c -> d, the two paths to d share one proof of
// reach(d): both references have equal content, and a repeat query gives the same.
func TestProof_SharedSubproof(t *testing.T) {
	t.Parallel()
	facts := []datalog.Fact{fact("edge", "a", "b"), fact("edge", "a", "c"), fact("edge", "b", "d"), fact("edge", "c", "d")}
	res := evalProofs(t, facts, `
		reach(X) :- edge("a", X).
		reach(Z) :- reach(Y), edge(Y, Z).
		meet(X) :- reach(X), edge("b", X), edge("c", X).
		both(X, Y) :- edge("a", X), edge("a", Y), reach(X), reach(Y), X != Y.
	`, datalog.Options{Proofs: true})
	p, ok := res.Proof("both", tup(fact("both", "b", "c")))
	if !ok || len(p.Body) != 4 {
		t.Fatalf("Proof(both b c) = %+v, %v", p, ok)
	}
	// reach(b) is referenced at Body[2]; the same value as a direct query.
	direct, _ := res.Proof("reach", tup(fact("reach", "b")))
	if !reflect.DeepEqual(p.Body[2], direct) {
		t.Errorf("shared sub-proof differs from the direct one:\n%+v\n%+v", p.Body[2], direct)
	}
	d, ok := res.Proof("reach", tup(fact("reach", "d")))
	if !ok || len(d.Body) != 2 {
		t.Fatalf("Proof(reach d) = %+v, %v", d, ok)
	}
	again, _ := res.Proof("reach", tup(fact("reach", "d")))
	if !reflect.DeepEqual(d, again) {
		t.Error("two queries of one tuple differ")
	}
	// The two derivations of d's parents are distinct values of one relation, but a
	// tuple reached twice inside one Proof call is one value.
	m, _ := res.Proof("meet", tup(fact("meet", "d")))
	if len(m.Body) != 3 || !reflect.DeepEqual(m.Body[0], d) {
		t.Errorf("meet(d) body[0] = %+v, want the proof of reach(d)", m.Body)
	}
}

func BenchmarkEvaluate(b *testing.B) {
	for _, proofs := range []bool{false, true} {
		b.Run(fmt.Sprintf("proofs=%v", proofs), func(b *testing.B) {
			base, err := datalog.NewBase(testSchema, chain(150)...)
			if err != nil {
				b.Fatal(err)
			}
			prog, _ := datalog.Parse(reachRules)
			plan, err := datalog.Compile(testSchema, prog)
			if err != nil {
				b.Fatal(err)
			}
			opts := datalog.Options{Proofs: proofs}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := datalog.Evaluate(context.Background(), plan, base, opts); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// A holder who rewrites a proof's rule deeply does not reach the plan: the next
// evaluation on it gives the same answer and the same proofs.
func TestProof_RuleIsACopy(t *testing.T) {
	t.Parallel()
	b, err := datalog.NewBase(testSchema, append(chain(4), fact("node", "n1"), fact("score", "n1", 3))...)
	if err != nil {
		t.Fatal(err)
	}
	prog := parse(t, reachRules+`pick(N) :- node(N), score(N, S), S > 1, not edge(N, "zz").`)
	plan, err := datalog.Compile(testSchema, prog)
	if err != nil {
		t.Fatal(err)
	}
	opts := datalog.Options{Proofs: true}
	eval := func() datalog.Result {
		res, err := datalog.Evaluate(context.Background(), plan, b, opts)
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	want := eval()
	wantAns, _ := want.Relation("reaches")
	wantPick, _ := want.Proof("pick", tup(fact("pick", "n1")))
	wantReach, _ := want.Proof("reaches", tup(fact("reaches", "n0", "n4")))

	scribble := func(r *datalog.Rule) {
		r.Head.Name, r.Head.Args[0].Var, r.Line = "x", "Q", -1
		for i := range r.Body {
			r.Body[i].Negated = !r.Body[i].Negated
			if a := r.Body[i].Atom; a != nil {
				a.Name = "x"
				a.Args[0].Var = "Q"
			}
			if c := r.Body[i].Compare; c != nil {
				c.Op = "<"
			}
		}
	}
	for _, p := range []datalog.Proof{wantPick, wantReach, wantReach.Body[0]} {
		scribble(p.Rule)
	}

	got := eval()
	if ans, _ := got.Relation("reaches"); ans.Hash() != wantAns.Hash() {
		t.Error("answer changed after a proof's rule was rewritten")
	}
	again, _ := got.Proof("pick", tup(fact("pick", "n1")))
	if again.Rule.Head.Name != "pick" || again.Rule.Line != 4 || again.Rule.Body[0].Negated {
		t.Errorf("the plan's rule was reached through a proof: %+v", again.Rule)
	}
	if reach, _ := got.Proof("reaches", tup(fact("reaches", "n0", "n4"))); reach.Rule.Head.Name != "reaches" || reach.Rule.Body[0].Atom.Name != "reaches" {
		t.Errorf("the plan's rule was reached through a proof: %+v", reach.Rule)
	}
}
