package datalog_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// testSchema is a small graph: edge/2 between nodes, node/1, score/2 of a node, and
// flag/2. node refuses a constant that starts with "!", to show a schema check runs.
var testSchema = datalog.Schema{
	Arities: map[string]int{"edge": 2, "node": 1, "score": 2, "flag": 2},
	Checks: map[string]func(datalog.Atom) error{
		"node": func(a datalog.Atom) error {
			if !a.Args[0].IsVar() && strings.HasPrefix(a.Args[0].Const.Text(), "!") {
				return fmt.Errorf("node %s: a node name does not start with !", a.Args[0].Const.Text())
			}
			return nil
		},
	},
}

// fact builds a Fact from Go values: a string, an int or a bool for each argument.
func fact(relation string, args ...any) datalog.Fact {
	t := make(datalog.Tuple, len(args))
	for i, a := range args {
		switch v := a.(type) {
		case string:
			t[i] = datalog.String(v)
		case int:
			t[i], _ = datalog.Number(strconv.Itoa(v))
		case float64:
			t[i], _ = datalog.Number(strconv.FormatFloat(v, 'f', -1, 64))
		case bool:
			t[i] = datalog.Bool(v)
		default:
			panic(fmt.Sprintf("fact: unsupported %T", a))
		}
	}
	return datalog.Fact{Relation: relation, Tuple: t}
}

func newBase(t *testing.T, facts ...datalog.Fact) datalog.Base {
	t.Helper()
	b, err := datalog.NewBase(testSchema, facts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	return b
}

func parse(t *testing.T, text string) datalog.Program {
	t.Helper()
	p, err := datalog.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	return p
}

// run compiles text over the test schema, evaluates it over facts and returns relation.
func run(t *testing.T, facts []datalog.Fact, text, relation string) datalog.Answer {
	t.Helper()
	ans, err := tryRun(newBase(t, facts...), text, relation, datalog.Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return ans
}

func tryRun(base datalog.Base, text, relation string, opts datalog.Options) (datalog.Answer, error) {
	prog, err := datalog.Parse(text)
	if err != nil {
		return datalog.Answer{}, err
	}
	plan, err := datalog.Compile(testSchema, prog)
	if err != nil {
		return datalog.Answer{}, err
	}
	res, err := datalog.Evaluate(context.Background(), plan, base, opts)
	if err != nil {
		return datalog.Answer{}, err
	}
	return res.Relation(relation)
}

// rows renders an answer as one string per tuple, for comparing against a table.
func rows(a datalog.Answer) []string {
	out := make([]string, 0, a.Len())
	for _, tup := range a.Tuples {
		parts := make([]string, len(tup))
		for i, c := range tup {
			parts[i] = c.Text()
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A base holds what it was given: every schema relation exists, empty or not, and a
// duplicate fact counts once.
func TestNewBase_HoldsFacts(t *testing.T) {
	t.Parallel()
	b := newBase(t, fact("node", "a"), fact("node", "b"), fact("node", "a"), fact("edge", "a", "b"))
	for rel, want := range map[string]int{"node": 2, "edge": 1, "score": 0, "flag": 0, "nothing": 0} {
		if got := b.Len(rel); got != want {
			t.Errorf("Len(%s) = %d, want %d", rel, got, want)
		}
	}
	if got := (datalog.Base{}).Len("node"); got != 0 {
		t.Errorf("the zero Base holds %d nodes, want 0", got)
	}
}

// What a base refuses, with the reason, instead of holding a fact nothing can read.
func TestNewBase_Refuses(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		schema datalog.Schema
		facts  []datalog.Fact
		want   string
	}{
		{"unknown relation", testSchema, []datalog.Fact{fact("nothing", "a")}, "a fact of nothing, which the schema does not have"},
		{"wrong arity", testSchema, []datalog.Fact{fact("edge", "a")}, "edge has 1 arguments, but edge is edge/2"},
		{"empty fact", testSchema, []datalog.Fact{{Relation: "node"}}, "node has 0 arguments"},
		{"check refuses a fact", testSchema, []datalog.Fact{fact("node", "ok"), fact("node", "!bad")}, "a fact of node: node !bad: a node name does not start with !"},
		{"zero arity relation", datalog.Schema{Arities: map[string]int{"flag": 0}}, nil, "needs at least one argument"},
		{"check without relation", datalog.Schema{Arities: map[string]int{"a": 1}, Checks: map[string]func(datalog.Atom) error{"b": nil}}, nil, "a check for b"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := datalog.NewBase(c.schema, c.facts...)
			if !errors.Is(err, datalog.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("NewBase = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
}

// The caller may reuse the tuple it passed: the base holds its own copy.
func TestNewBase_CopiesTuples(t *testing.T) {
	t.Parallel()
	f := fact("edge", "a", "b")
	b := newBase(t, f)
	f.Tuple[1] = datalog.String("z")
	if got := rows(run2(t, b, `e(X, Y) :- edge(X, Y).`, "e")); !same(got, []string{"a b"}) {
		t.Errorf("edge = %v, want [a b]", got)
	}
}

func run2(t *testing.T, b datalog.Base, text, relation string) datalog.Answer {
	t.Helper()
	ans, err := tryRun(b, text, relation, datalog.Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return ans
}

// With adds facts as a new value: the old base is unchanged, the new one holds both,
// and a refused fact refuses the whole call.
func TestBase_With(t *testing.T) {
	t.Parallel()
	standing := newBase(t, fact("edge", "a", "b"), fact("node", "a"))
	more, err := standing.With(fact("edge", "b", "c"), fact("edge", "a", "b"), fact("score", "a", 1))
	if err != nil {
		t.Fatalf("With: %v", err)
	}
	if standing.Len("edge") != 1 || standing.Len("score") != 0 {
		t.Errorf("the standing base moved: %d edges, %d scores", standing.Len("edge"), standing.Len("score"))
	}
	if more.Len("edge") != 2 || more.Len("score") != 1 || more.Len("node") != 1 {
		t.Errorf("With holds %d edges, %d scores, %d nodes; want 2, 1, 1", more.Len("edge"), more.Len("score"), more.Len("node"))
	}
	got := rows(run2(t, more, `path(X, Y) :- edge(X, Y). path(X, Z) :- path(X, Y), edge(Y, Z).`, "path"))
	if want := []string{"a b", "a c", "b c"}; !same(got, want) {
		t.Errorf("path = %v, want %v", got, want)
	}
	if _, err := standing.With(fact("edge", "a")); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("With a wrong-arity fact = %v, want ErrInvalid", err)
	}
	none, err := standing.With()
	if err != nil || none.Len("edge") != 1 {
		t.Errorf("With nothing = %v, %d edges; want the same base", err, none.Len("edge"))
	}
}

// One Base, many concurrent evaluations: each sees the same answer and, under -race,
// none writes to a shared relation.
func TestEvaluate_ConcurrentOverOneBase(t *testing.T) {
	t.Parallel()
	var facts []datalog.Fact
	for i := 0; i < 30; i++ {
		facts = append(facts, fact("edge", fmt.Sprint("n", i), fmt.Sprint("n", i+1)), fact("node", fmt.Sprint("n", i)))
	}
	base := newBase(t, facts...)
	text := `path(X, Y) :- edge(X, Y). path(X, Z) :- path(X, Y), edge(Y, Z). dead(X) :- node(X), not edge(X, "n3"), path(X, "n20").`
	want, err := tryRun(base, text, "path", datalog.Options{})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var wg sync.WaitGroup
	hashes := make([]string, 16)
	for i := range hashes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each goroutine layers its own request fact over the shared base.
			b, err := base.With(fact("flag", fmt.Sprint("g", i), true))
			if err != nil {
				return
			}
			if ans, err := tryRun(b, text, "path", datalog.Options{}); err == nil {
				hashes[i] = ans.Hash()
			}
		}()
	}
	wg.Wait()
	for i, h := range hashes {
		if h != want.Hash() {
			t.Errorf("evaluation %d hashed %q, want %q", i, h, want.Hash())
		}
	}
}

// A base is only for the plan compiled over its schema: another one is refused, not
// read as empty.
func TestEvaluate_RefusesAForeignBase(t *testing.T) {
	t.Parallel()
	plan, err := datalog.Compile(testSchema, parse(t, `n(X) :- node(X).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	other, err := datalog.NewBase(datalog.Schema{Arities: map[string]int{"node": 2, "edge": 2, "score": 2, "flag": 2}})
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	for name, b := range map[string]datalog.Base{"zero": {}, "wrong arity": other} {
		if _, err := datalog.Evaluate(context.Background(), plan, b, datalog.Options{}); !errors.Is(err, datalog.ErrInvalid) {
			t.Errorf("%s base: Evaluate = %v, want ErrInvalid", name, err)
		}
	}
}

// The schema's check holds on With as well, so a request's facts meet the same word as
// the standing ones did.
func TestBase_WithRunsTheChecks(t *testing.T) {
	t.Parallel()
	standing := newBase(t, fact("node", "a"))
	if _, err := standing.With(fact("node", "!late")); !errors.Is(err, datalog.ErrInvalid) {
		t.Errorf("With a fact the check refuses = %v, want ErrInvalid", err)
	}
	if _, err := standing.With(fact("node", "fine")); err != nil {
		t.Errorf("With a fact the check allows = %v", err)
	}
}
