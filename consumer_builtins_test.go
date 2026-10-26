package tarski_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mattjoyce/tarski"
)

// shorterThan(A, B) is a consumer builtin with no meaning to the engine: true when A's
// text is shorter than B's. Its Check refuses an empty constant, which no row of the
// tests can be shorter than, to prove the plan-time hook.
var shorterThan = tarski.Builtin{
	Args: []tarski.BuiltinArg{{Name: "A", Doc: "a constant"}, {Name: "B", Doc: "a constant"}},
	Doc:  "true when A's text is shorter than B's",
	Check: func(consts []*tarski.Const) error {
		if b := consts[1]; b != nil && b.Text() == "" {
			return errEmptyBound
		}
		return nil
	},
	Holds: func(args []tarski.Const) bool { return len(args[0].Text()) < len(args[1].Text()) },
}

var errEmptyBound = errors.New("nothing is shorter than the empty string")

var wordSchema = tarski.Schema{
	Arities:  map[string]int{"word": 1, "pair": 2},
	Builtins: map[string]tarski.Builtin{"shorter": shorterThan},
}

var wordFacts = []tarski.Fact{
	fact("pair", "ab", "abcd"), fact("pair", "abc", "a"), fact("pair", "xy", "zw"),
	fact("word", "a"), fact("word", "abc"),
}

// evalWords compiles text over wordSchema and evaluates it over wordFacts.
func evalWords(t *testing.T, schema tarski.Schema, text string, opts tarski.Options) (tarski.Result, error) {
	t.Helper()
	plan, err := tarski.Compile(schema, parse(t, text))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := tarski.NewBase(schema, wordFacts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	return tarski.Evaluate(context.Background(), plan, base, opts)
}

func relationRows(t *testing.T, res tarski.Result, name string) []string {
	t.Helper()
	ans, err := res.Relation(name)
	if err != nil {
		t.Fatalf("Relation(%s): %v", name, err)
	}
	return rows(ans)
}

// A consumer builtin is evaluated as an engine guard is: positively, negated, inside a
// relation another rule negates, and in a recursive rule, which it does not enter the
// stratification of.
func TestConsumerBuiltin_Evaluates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, text, rel string
		want            []string
	}{
		{"positive", `up(X, Y) :- pair(X, Y), shorter(X, Y).`, "up", []string{"ab abcd"}},
		{"written first", `up(X, Y) :- shorter(X, Y), pair(X, Y).`, "up", []string{"ab abcd"}},
		{"negated", `down(X, Y) :- pair(X, Y), not shorter(X, Y).`, "down", []string{"abc a", "xy zw"}},
		{"constant arguments", `w(X) :- word(X), shorter("", X).`, "w", []string{"a", "abc"}},
		{"under a negated relation", `
			up(X, Y) :- pair(X, Y), shorter(X, Y).
			flat(X, Y) :- pair(X, Y), not up(X, Y).`, "flat", []string{"abc a", "xy zw"}},
		{"in a recursion", `
			grow(X) :- word(X).
			grow(Y) :- grow(X), pair(X, Y), shorter(X, Y).`, "grow", []string{"a", "abc"}},
		{"beside an engine builtin", `p(X, Y) :- pair(X, Y), shorter(X, Y), prefix(X, Y).`, "p", []string{"ab abcd"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			res, err := evalWords(t, wordSchema, c.text, tarski.Options{})
			if err != nil {
				t.Fatalf("Evaluate: %v", err)
			}
			if got := relationRows(t, res, c.rel); !same(got, c.want) {
				t.Errorf("%s = %v, want %v", c.rel, got, c.want)
			}
		})
	}
}

// A builtin is a guard, so a proof through it lists only the positive atoms; and a rule
// stopped by it reports the builtin as the farthest literal, with nothing beneath it.
func TestConsumerBuiltin_ProofAndWhyNot(t *testing.T) {
	t.Parallel()
	res, err := evalWords(t, wordSchema, `up(X, Y) :- pair(X, Y), shorter(X, Y).`, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	p, ok := res.Proof("up", tuple("ab", "abcd"))
	if !ok {
		t.Fatal("no proof of up(ab, abcd)")
	}
	if len(p.Body) != 1 || p.Body[0].Relation != "pair" {
		t.Errorf("proof body = %+v, want the pair fact alone", p.Body)
	}
	a := whyNot(t, res, "up", tuple("abc", "a"), 0)
	if len(a.Rules) != 1 {
		t.Fatalf("why-not rules = %+v, want one", a.Rules)
	}
	rf := a.Rules[0]
	if got := failed(rf); got != "shorter(X, Y)" || rf.Bound["X"].Text() != "abc" || rf.Bound["Y"].Text() != "a" {
		t.Errorf("failed at %q with %v, want the builtin shorter(X, Y) with X abc and Y a", got, rf.Bound)
	}
	if rf.Failed.Atom == nil || rf.Failed.Atom.Name != "shorter" || rf.Because != nil || rf.Blocked != nil {
		t.Errorf("failure = %+v, want the builtin with no Because and no Blocked", rf)
	}
}

// Compile refuses a builtin declaration it could not use, and a program that uses a
// consumer builtin where an engine builtin would be refused, each with ErrInvalid and
// a message that says what to change.
func TestConsumerBuiltin_Refusals(t *testing.T) {
	t.Parallel()
	with := func(name string, b tarski.Builtin) tarski.Schema {
		return tarski.Schema{Arities: wordSchema.Arities, Builtins: map[string]tarski.Builtin{name: b}}
	}
	noHolds := shorterThan
	noHolds.Holds = nil
	noArgs := shorterThan
	noArgs.Args = nil
	const ok = `up(X, Y) :- pair(X, Y).`
	cases := []struct {
		name   string
		schema tarski.Schema
		text   string
		want   string
	}{
		{"redeclares an engine builtin", with("prefix", shorterThan), ok, "prefix is an engine builtin"},
		{"is also a base relation", with("word", shorterThan), ok, "word is both a base relation and a builtin"},
		{"an upper-case name", with("Shorter", shorterThan), ok, `"Shorter" is not a builtin name`},
		{"an empty name", with("", shorterThan), ok, `"" is not a builtin name`},
		{"a name with a dash", with("short-er", shorterThan), ok, `"short-er" is not a builtin name`},
		{"no arguments", with("shorter", noArgs), ok, "builtin shorter has no arguments"},
		{"no Holds", with("shorter", noHolds), ok, "builtin shorter has no Holds"},
		{"a head", wordSchema, `shorter(X, Y) :- pair(X, Y).`, "shorter is a built-in and cannot be the head of a rule"},
		{"a declaration", wordSchema, `shorter/2.`, "shorter is a built-in and cannot be declared"},
		{"wrong arity", wordSchema, `w(X) :- word(X), shorter(X).`, "shorter has 1 arguments, but shorter takes 2: shorter(A, B)"},
		{"an unbound argument", wordSchema, `w(X) :- word(X), shorter(X, Y).`, "Y, argument B of shorter(A, B), must be bound by a positive body atom"},
		{"bound only by a negation", wordSchema, `w(X) :- word(X), not pair(X, Y), shorter(X, Y).`, "Y in not pair is not bound"},
		{"anonymous", wordSchema, `w(X) :- word(X), shorter(X, _).`, "_ in shorter tests nothing"},
		{"a constant its Check refuses", wordSchema, `w(X) :- word(X), shorter(X, "").`, "shorter(A, B): nothing is shorter than the empty string"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := tarski.Compile(c.schema, parse(t, c.text))
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
	// The Check's own error is still there to be found.
	_, err := tarski.Compile(wordSchema, parse(t, `w(X) :- word(X), shorter(X, "").`))
	if !errors.Is(err, errEmptyBound) {
		t.Errorf("Compile = %v, want it to wrap the Check's error", err)
	}
	// A schema is one value: NewBase refuses a builtin declaration Compile would.
	if _, err := tarski.NewBase(with("prefix", shorterThan)); !errors.Is(err, tarski.ErrInvalid) {
		t.Errorf("NewBase = %v, want ErrInvalid", err)
	}
}

// The plan copies the builtin table at Compile: a caller changing its map, or the
// declaration's Args, afterwards changes nothing the plan derives.
func TestConsumerBuiltin_Snapshot(t *testing.T) {
	t.Parallel()
	decl := shorterThan
	decl.Args = []tarski.BuiltinArg{{Name: "A"}, {Name: "B"}}
	schema := tarski.Schema{Arities: wordSchema.Arities, Builtins: map[string]tarski.Builtin{"shorter": decl}}
	plan, err := tarski.Compile(schema, parse(t, `up(X, Y) :- pair(X, Y), shorter(X, Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	always := decl
	always.Holds = func([]tarski.Const) bool { return true }
	schema.Builtins["shorter"] = always
	decl.Args[0], decl.Args = tarski.BuiltinArg{Name: "changed"}, decl.Args[:1]
	base, err := tarski.NewBase(wordSchema, wordFacts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if got := relationRows(t, res, "up"); !same(got, []string{"ab abcd"}) {
		t.Errorf("up = %v after the caller's map changed, want ab abcd", got)
	}
	delete(schema.Builtins, "shorter")
	if res, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{}); err != nil {
		t.Fatalf("Evaluate after delete: %v", err)
	}
	if got := relationRows(t, res, "up"); !same(got, []string{"ab abcd"}) {
		t.Errorf("up = %v after the caller deleted the builtin, want ab abcd", got)
	}
}

// One plan with a consumer builtin, shared by concurrent evaluations, gives one answer
// and one hash. Run under -race, this also holds the table to being read only.
func TestConsumerBuiltin_Concurrent(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(wordSchema, parse(t, `
		up(X, Y) :- pair(X, Y), shorter(X, Y).
		down(X, Y) :- pair(X, Y), not shorter(X, Y).`))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := tarski.NewBase(wordSchema, wordFacts...)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	const n = 16
	hashes := make([]string, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{Proofs: true})
			if err != nil {
				t.Errorf("Evaluate: %v", err)
				return
			}
			if _, ok := res.WhyNot("up", tuple("abc", "a"), 0); !ok {
				t.Error("WhyNot was not ok")
			}
			ans, err := res.Relation("down")
			if err != nil {
				t.Errorf("Relation: %v", err)
				return
			}
			hashes[i] = ans.Hash()
		}()
	}
	wg.Wait()
	for i := 1; i < n; i++ {
		if hashes[i] != hashes[0] {
			t.Fatalf("evaluation %d hashed %s, evaluation 0 %s", i, hashes[i], hashes[0])
		}
	}
}

// A Holds that panics fails closed: Evaluate returns ErrInvalid naming the builtin and
// its arguments, WhyNot gives no explanation, and no panic reaches the caller.
func TestConsumerBuiltin_PanicFailsClosed(t *testing.T) {
	t.Parallel()
	fragile := shorterThan
	fragile.Holds = func(args []tarski.Const) bool { return args[0].Text()[2] == 'c' } // index out of range on "ab"
	schema := tarski.Schema{Arities: wordSchema.Arities, Builtins: map[string]tarski.Builtin{"fragile": fragile}}
	_, err := evalWords(t, schema, `p(X, Y) :- pair(X, Y), fragile(X, Y).`, tarski.Options{})
	if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), `builtin fragile("ab", "abcd") panicked`) {
		t.Errorf("Evaluate = %v, want ErrInvalid naming fragile and its arguments", err)
	}

	// A pure Holds meets in WhyNot only bindings the evaluation already tried, so a
	// first panic there is an impure builtin's; it still must not escape. This one is
	// armed after the evaluation.
	var armed atomic.Bool
	impure := shorterThan
	impure.Holds = func(args []tarski.Const) bool {
		if armed.Load() {
			panic("armed")
		}
		return shorterThan.Holds(args)
	}
	schema.Builtins = map[string]tarski.Builtin{"impure": impure}
	res, err := evalWords(t, schema, `q(X, Y) :- pair(X, Y), impure(X, Y).`, tarski.Options{Proofs: true})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	armed.Store(true)
	if a, ok := res.WhyNot("q", tuple("abc", "a"), 0); ok {
		t.Errorf("WhyNot = %v, ok; want no explanation when a builtin panics", a)
	}
}

// EngineBuiltins lists the engine's own, as a copy a catalogue may change freely, and
// a catalogue can marshal it.
func TestEngineBuiltins_Catalogue(t *testing.T) {
	t.Parallel()
	got := tarski.EngineBuiltins()
	for _, name := range []string{"prefix", "suffix", "contains"} {
		b, ok := got[name]
		if !ok || len(b.Args) != 2 || b.Holds == nil || b.Doc == "" {
			t.Errorf("EngineBuiltins()[%s] = %+v, want two named arguments, a Doc and a Holds", name, b)
		}
	}
	if len(got) != 3 {
		t.Errorf("EngineBuiltins() has %d entries, want 3", len(got))
	}
	text, err := json.Marshal(got["prefix"])
	if err != nil || !strings.Contains(string(text), `"name":"Pattern"`) {
		t.Errorf("json.Marshal(prefix) = %s, %v", text, err)
	}
	got["prefix"].Args[0].Name = "changed"
	delete(got, "suffix")
	again := tarski.EngineBuiltins()
	if again["prefix"].Args[0].Name != "Pattern" || len(again) != 3 {
		t.Errorf("changing a copy changed the engine's table: %+v", again)
	}
	res, err := evalWords(t, wordSchema, `p(X, Y) :- pair(X, Y), suffix(X, Y).`, tarski.Options{})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if rows := relationRows(t, res, "p"); len(rows) != 0 {
		t.Errorf("p = %v, want none", rows)
	}
}
