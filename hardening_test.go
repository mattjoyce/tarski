package tarski_test

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// A negative bound is no bound anyone meant: it is refused with ErrInvalid before any
// join, never read as unlimited. Zero stays the documented "no bound".
func TestEvaluate_RefusesNegativeBounds(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(testSchema, parse(t, reachRules))
	if err != nil {
		t.Fatal(err)
	}
	base := newBase(t, chain(10)...)
	for _, tc := range []struct {
		name string
		opts tarski.Options
		want string
	}{
		{"steps", tarski.Options{MaxSteps: -1}, "MaxSteps"},
		{"bytes", tarski.Options{MaxBytes: -1}, "MaxBytes"},
		{"both", tarski.Options{MaxSteps: -1, MaxBytes: -1}, "MaxBytes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := tarski.Evaluate(context.Background(), plan, base, tc.opts)
			if !errors.Is(err, tarski.ErrInvalid) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Evaluate(%+v) = %v, want ErrInvalid naming %s", tc.opts, err, tc.want)
			}
			if steps, bytes := res.Cost(); steps != 0 || bytes != 0 {
				t.Fatalf("a refused evaluation charged %d steps, %d bytes", steps, bytes)
			}
		})
	}
	if _, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{}); err != nil {
		t.Fatalf("zero bounds are no bound: %v", err)
	}
}

// A constant read back from JSON is one valid JSON value in the engine's string
// domain, called directly or through json.Unmarshal. Malformed UTF-8 and a lone
// surrogate escape are refused, not read as U+FFFD; so is anything after the value.
func TestConst_UnmarshalJSONIsStrict(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"trailing close", []byte(`1]`)},
		{"trailing value", []byte(`1 2`)},
		{"trailing string", []byte(`"a""b"`)},
		{"raw invalid byte", []byte{'"', 0xff, '"'}},
		{"raw truncated rune", []byte{'"', 'x', 0xc3, '"'}},
		{"lone high surrogate", []byte(`"\ud800"`)},
		{"lone low surrogate", []byte(`"\udc00"`)},
		{"high then text", []byte(`"\ud83dx"`)},
		{"high then high", []byte(`"\ud83d\ud83d"`)},
		{"low then high", []byte(`"\ude00\ud83d"`)},
		{"short escape", []byte(`"\u12"`)},
		{"not json", []byte(`nope`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var direct tarski.Const
			if err := direct.UnmarshalJSON(tc.raw); !errors.Is(err, tarski.ErrSyntax) {
				t.Errorf("UnmarshalJSON(%q) = %v (%q), want ErrSyntax", tc.raw, err, direct.Text())
			}
			var wrapped []tarski.Const
			if err := json.Unmarshal(append(append([]byte("["), tc.raw...), ']'), &wrapped); err == nil {
				t.Errorf("json.Unmarshal([%s]) accepted %v", tc.raw, wrapped)
			}
		})
	}
	for _, tc := range []struct {
		raw  string
		want tarski.Const
	}{
		{` 1 `, tarski.Int(1)},
		{"\"\xef\xbf\xbd\"", tarski.String("�")},
		{`"�"`, tarski.String("�")},
		{`"😀"`, tarski.String("\U0001F600")},
		{`"\\ud800"`, tarski.String(`\ud800`)},
		{`"é\n"`, tarski.String("é\n")},
		{`true`, tarski.Bool(true)},
	} {
		var c tarski.Const
		if err := c.UnmarshalJSON([]byte(tc.raw)); err != nil || c.Kind() != tc.want.Kind() || c.Text() != tc.want.Text() {
			t.Errorf("UnmarshalJSON(%q) = %q, %v; want %q", tc.raw, c.Text(), err, tc.want.Text())
		}
	}
}

// MarshalJSON writes a string only from the engine's string domain: a Const built by
// String around invalid UTF-8 is refused, not written as U+FFFD, the text a different
// constant writes.
func TestConst_MarshalJSONRefusesInvalidUTF8(t *testing.T) {
	t.Parallel()
	if _, err := json.Marshal(tarski.String("\xff")); !errors.Is(err, tarski.ErrInvalid) {
		t.Fatalf("Marshal(String(\\xff)) = %v, want ErrInvalid", err)
	}
	b, err := json.Marshal(tarski.String("�"))
	if err != nil || string(b) != "\"�\"" {
		t.Fatalf("Marshal(String(U+FFFD)) = %s, %v", b, err)
	}
}

// A refused evaluation allocates for the work it was allowed, not for the size of the
// relation it would have scanned: with MaxSteps 1, a scan of 100,000 facts and one of
// 1,000 cost about the same memory. Not parallel: it reads the process's allocation
// count, which a parallel test would add to.
func TestEvaluate_ScanAllocatesNoCandidateList(t *testing.T) {
	schema := tarski.Schema{Arities: map[string]int{"edge": 1}}
	plan, err := tarski.Compile(schema, parse(t, `out(X) :- edge(X).`))
	if err != nil {
		t.Fatal(err)
	}
	allocated := func(n int) uint64 {
		facts := make([]tarski.Fact, n)
		for i := range facts {
			facts[i] = tarski.Fact{Relation: "edge", Tuple: tarski.Tuple{tarski.Int(int64(i))}}
		}
		base, err := tarski.NewBase(schema, facts...)
		if err != nil {
			t.Fatal(err)
		}
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{MaxSteps: 1})
		runtime.ReadMemStats(&after)
		if !errors.Is(err, tarski.ErrTooLarge) {
			t.Fatalf("n=%d: Evaluate = %v, want ErrTooLarge", n, err)
		}
		return after.TotalAlloc - before.TotalAlloc
	}
	small, large := allocated(1000), allocated(100000)
	if large > 2*small+16<<10 {
		t.Fatalf("a refused scan allocated %d bytes over 100,000 facts and %d over 1,000", large, small)
	}
}

// WhyNot explains only a tuple in the engine's string domain: one holding invalid
// UTF-8 could be no fact, and its Absence would have no JSON text.
func TestWhyNot_RefusesInvalidUTF8(t *testing.T) {
	t.Parallel()
	plan, err := tarski.Compile(testSchema, parse(t, reachRules))
	if err != nil {
		t.Fatal(err)
	}
	res, err := tarski.Evaluate(context.Background(), plan, newBase(t, chain(3)...), tarski.Options{Proofs: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := res.WhyNot("reaches", tarski.Tuple{tarski.String("\xff"), tarski.String("b")}, 0); ok {
		t.Fatal("WhyNot explained a tuple holding invalid UTF-8")
	}
	abs, ok := res.WhyNot("reaches", tarski.Tuple{tarski.String("\ufffd"), tarski.String("b")}, 0)
	if !ok {
		t.Fatal("WhyNot refused U+FFFD, a valid string")
	}
	if _, err := json.Marshal(abs); err != nil {
		t.Fatalf("Marshal(Absence) = %v", err)
	}
}
