package tarski_test

import (
	"cmp"
	"errors"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"
)

// A number is the exact decimal the fact holds. The engine kept a float64 and printed
// the float's shortest form, so 1792269840000 answered as 1.79226984e+12,
// 9007199254740993 answered as the same number as its neighbour 9007199254740992, and
// a comparison with the wrong number matched. The value is now exact: it answers as
// the fact holds it, an integer in full, and it compares exactly. Ported from Anam's
// internal/query number_test.go and number_precision_test.go, over NewBase.

// beyondFloat64 is 2^53+1, the first integer a float64 cannot hold.
const beyondFloat64 = "9007199254740993"

// num makes a number constant from a lexeme the test knows is good.
func num(t *testing.T, lexeme string) tarski.Const {
	t.Helper()
	c, err := tarski.Number(lexeme)
	if err != nil {
		t.Fatalf("Number(%q): %v", lexeme, err)
	}
	return c
}

// scores is a base of score(name, n) facts, one per lexeme, named by position.
func scores(t *testing.T, lexemes ...string) tarski.Base {
	t.Helper()
	facts := make([]tarski.Fact, len(lexemes))
	for i, l := range lexemes {
		facts[i] = tarski.Fact{Relation: "score", Tuple: tarski.Tuple{tarski.String(string(rune('a' + i))), num(t, l)}}
	}
	return newBase(t, facts...)
}

func TestNumber_TheValueTheFactHoldsIsTheValueThatAnswers(t *testing.T) {
	t.Parallel()
	cases := []struct{ lexeme, want string }{
		{"120", "120"},                     // an ordinary reading
		{"81.4", "81.4"},                   // a decimal
		{"1792269840000", "1792269840000"}, // an epoch in milliseconds, in full
		{beyondFloat64, beyondFloat64},     // beyond float64's exact range
		{"1.0000000000000000001", "1.0000000000000000001"},
		{"0.5", "0.5"},
		{"-0.5", "-0.5"},
		{"1.0", "1"},    // one value, one form
		{"1e3", "1000"}, // the same value as 1000
	}
	lexemes := make([]string, len(cases))
	want := make([]string, len(cases))
	for i, c := range cases {
		lexemes[i], want[i] = c.lexeme, c.want
	}
	ans, err := tryRun(scores(t, lexemes...), `s(S) :- score(_, S).`, "s", tarski.Options{})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	got := rows(ans)
	sort.Strings(got)
	sort.Strings(want)
	if !same(got, want) {
		t.Errorf("the numbers answered\n%v\nwant\n%v", got, want)
	}
}

// Two numbers a float64 cannot tell apart are two numbers: 1 and
// 1.0000000000000000001 are one float64 and two decimals, and so are 2^53 and 2^53+1.
func TestNumber_TwoNumbersAFloat64CannotTellApartAreNotOne(t *testing.T) {
	t.Parallel()
	ans, err := tryRun(scores(t, "1.0000000000000000001"), `s(E) :- score(E, S), S = 1.`, "s", tarski.Options{})
	if err != nil || ans.Len() != 0 {
		t.Errorf("S = 1 over 1.0000000000000000001 answered %d rows, %v; want none", ans.Len(), err)
	}
	b := newBase(t,
		tarski.Fact{Relation: "score", Tuple: tarski.Tuple{tarski.String("a"), num(t, "9007199254740992")}},
		tarski.Fact{Relation: "score", Tuple: tarski.Tuple{tarski.String("a"), num(t, beyondFloat64)}})
	if got := b.Len("score"); got != 2 {
		t.Errorf("Len(score) = %d, want 2: 2^53 and 2^53+1 collapsed", got)
	}
}

// The number the fact holds is the number a comparison matches, and only it: a fact
// holding 2^53+1 answers a query for it, and a fact holding 2^53 does not.
func TestNumber_TheNumberTheFactHoldsIsTheNumberAComparisonMatches(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		held string
		want int
	}{{beyondFloat64, 1}, {"9007199254740992", 0}} {
		for _, rule := range []string{
			`s(E) :- score(E, S), S = ` + beyondFloat64 + `.`,
			`s(E) :- score(E, ` + beyondFloat64 + `).`,
		} {
			ans, err := tryRun(scores(t, c.held), rule, "s", tarski.Options{})
			if err != nil || ans.Len() != c.want {
				t.Errorf("%s over %s answered %d rows, %v; want %d", rule, c.held, ans.Len(), err, c.want)
			}
		}
	}
}

// An exponent is bounded before any arithmetic is done on it (Anam card 224): the
// exponent's arithmetic once wrapped past the guards and panicked. The bound is nine
// digits either way: at it the number is exact and compares, one past it is refused.
func TestNumber_AnExponentBeyondTheBoundIsRefusedNotAPanic(t *testing.T) {
	t.Parallel()
	base := scores(t, "120")
	cases := []struct {
		constant string
		refused  bool
	}{
		{"1e999999999", false},
		{"1e-999999999", false},
		{"1e+999999999", false},
		{"1e1000000000", true},
		{"1e-1000000000", true},
		{"1e9223372036854775807", true}, // the panic the audit found
		{"1e-9223372036854775808", true},
		{"1e99999999999999999999", true}, // past an int altogether
	}
	for _, c := range cases {
		ans, err := tryRun(base, `big(E) :- score(E, S), S < `+c.constant+`.`, "big", tarski.Options{})
		small := strings.HasPrefix(c.constant, "1e-")
		switch {
		case c.refused && (!errors.Is(err, tarski.ErrSyntax) || !strings.Contains(err.Error(), "exponent out of range")):
			t.Errorf("%s: err = %v, want ErrSyntax, exponent out of range", c.constant, err)
		case !c.refused && err != nil:
			t.Errorf("%s: err = %v, want the query to run", c.constant, err)
		case !c.refused && small && ans.Len() != 0:
			t.Errorf("120 < %s answered %d rows", c.constant, ans.Len())
		case !c.refused && !small && ans.Len() != 1:
			t.Errorf("120 < %s answered %d rows, want 1", c.constant, ans.Len())
		}
	}
}

// The canonical text is one form per value: an integer in full, a decimal in plain
// form, and the exact scientific form only past maxNumberDigits (64) characters.
func TestNumber_CanonicalText(t *testing.T) {
	t.Parallel()
	cases := []struct{ lexeme, want string }{
		{"1792269840000", "1792269840000"},
		{"1e21", "1000000000000000000000"},
		{"1e6", "1000000"},
		{"1.5e6", "1500000"},
		{"1E+2", "100"},
		{"100.000", "100"},
		{"0.00001", "0.00001"},
		{"1.5e-7", "0.00000015"},
		{"-12.340e1", "-123.4"},
		{"0.0", "0"},
		{"-0e-5", "0"},
		{"1e63", "1" + strings.Repeat("0", 63)}, // 64 characters
		{"1e64", "1e+64"},
		{"1e-62", "0." + strings.Repeat("0", 61) + "1"}, // 64 characters
		{"1e-63", "1e-63"},
		{"-1.23e-70", "-1.23e-70"},
		{"1e999", "1e+999"},
		{"-1e999", "-1e+999"},
		{"123456789012345678901234567890.5", "123456789012345678901234567890.5"},
	}
	for _, c := range cases {
		got := num(t, c.lexeme)
		if got.Text() != c.want {
			t.Errorf("Number(%q).Text() = %q, want %q", c.lexeme, got.Text(), c.want)
		}
		if again := num(t, got.Text()); again != got {
			t.Errorf("Number(%q) read back from its text %q is a different constant", c.lexeme, got.Text())
		}
		var back tarski.Const
		if b, err := got.MarshalJSON(); err != nil || back.UnmarshalJSON(b) != nil || back != got {
			t.Errorf("Number(%q) through JSON = %q, want %q", c.lexeme, back.Text(), got.Text())
		}
	}
}

// Equal numbers are one constant and one tuple, however they are spelled.
func TestNumber_EqualNumbersAreOneTuple(t *testing.T) {
	t.Parallel()
	spellings := []string{"100", "100.0", "1e2", "1E+2", "10e1", "1000e-1", "100.00e0"}
	facts := make([]tarski.Fact, len(spellings))
	for i, s := range spellings {
		facts[i] = tarski.Fact{Relation: "score", Tuple: tarski.Tuple{tarski.String("a"), num(t, s)}}
	}
	if got := newBase(t, facts...).Len("score"); got != 1 {
		t.Errorf("Len(score) = %d, want 1: one value spelled %d ways", got, len(spellings))
	}
}

// Numbers order exactly. The answer is sorted by the engine's order, so a base of
// numbers given out of order comes back in this order.
func TestNumber_OrdersExactly(t *testing.T) {
	t.Parallel()
	sorted := []string{
		"-1e+999", "-9007199254740993", "-9007199254740992", "-1.5", "-1.0000000000000000001",
		"-1", "-0.5", "-0.00000015", "0", "1e-63", "0.00001", "0.5", "1",
		"1.0000000000000000001", "1.5", "1000000", "9007199254740992", beyondFloat64, "1e+64", "1e+999",
	}
	shuffled := append([]string(nil), sorted...)
	rand.New(rand.NewPCG(1, 2)).Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })
	facts := make([]tarski.Fact, len(shuffled))
	for i, s := range shuffled {
		facts[i] = tarski.Fact{Relation: "node", Tuple: tarski.Tuple{num(t, s)}}
	}
	ans, err := tryRun(newBase(t, facts...), `n(X) :- node(X).`, "n", tarski.Options{})
	if err != nil || !same(rows(ans), sorted) {
		t.Errorf("sorted = %v, %v\nwant %v", rows(ans), err, sorted)
	}
	lt, err := tryRun(newBase(t, facts...), `lt(X) :- node(X), X < 9007199254740993, X > 9007199254740992.`, "lt", tarski.Options{})
	if err != nil || lt.Len() != 0 {
		t.Errorf("a number strictly between 2^53 and 2^53+1 answered %v, %v", rows(lt), err)
	}
}

// Int and Float keep the order the Go values have: every int64 exactly, and every
// float64 through its shortest round-trip decimal, which is distinct per float and
// ordered as the floats are.
func TestIntAndFloat_KeepTheGoOrder(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(3, 4))
	ordered := func(a, b tarski.Const) int {
		var x, y []byte
		x, _ = a.MarshalJSON()
		y, _ = b.MarshalJSON()
		ans, err := tryRun(newBase(t,
			tarski.Fact{Relation: "node", Tuple: tarski.Tuple{a}},
			tarski.Fact{Relation: "node", Tuple: tarski.Tuple{b}}),
			`lt(X) :- node(X), node(Y), X < Y.`, "lt", tarski.Options{})
		if err != nil {
			t.Fatalf("%s < %s: %v", x, y, err)
		}
		switch {
		case ans.Len() == 0:
			return 0
		case ans.Tuples()[0][0] == a:
			return -1
		default:
			return 1
		}
	}
	for range 200 {
		i, j := int64(r.Uint64()), int64(r.Uint64())
		if got, want := ordered(tarski.Int(i), tarski.Int(j)), cmp.Compare(i, j); got != want {
			t.Errorf("Int(%d) vs Int(%d) = %d, want %d", i, j, got, want)
		}
		f, g := math.Float64frombits(r.Uint64()), math.Float64frombits(r.Uint64())
		cf, errF := tarski.Float(f)
		cg, errG := tarski.Float(g)
		if errF != nil || errG != nil {
			continue // NaN or an infinity, refused as TestIntAndFloat_AgreeWithNumber holds
		}
		if got, want := ordered(cf, cg), cmp.Compare(f, g); got != want {
			t.Errorf("Float(%v) vs Float(%v) = %d, want %d", f, g, got, want)
		}
	}
	if tarski.Int(math.MinInt64).Text() != "-9223372036854775808" || tarski.Int(math.MaxInt64).Text() != "9223372036854775807" {
		t.Errorf("Int at the limits = %q, %q", tarski.Int(math.MinInt64).Text(), tarski.Int(math.MaxInt64).Text())
	}
}

// TestNumber_TextReadsBack holds the writer to the reader at the exponent bound: a
// lexeme accepted only if its canonical text is accepted too.
func TestNumber_TextReadsBack(t *testing.T) {
	tests := []struct {
		lexeme string
		ok     bool
	}{
		{"1e999999999", true},
		{"1e-999999999", true},
		{"9.99e999999999", true},
		{"10e999999999", false},    // 1e+1000000000
		{"123e999999998", false},   // 1.23e+1000000000
		{"0.1e-999999999", false},  // 1e-1000000000
		{"0.01e-999999999", false}, // 1e-1000000001
		{"0e999999999", true},      // zero has no magnitude
	}
	for _, tt := range tests {
		t.Run(tt.lexeme, func(t *testing.T) {
			c, err := tarski.Number(tt.lexeme)
			if (err == nil) != tt.ok {
				t.Fatalf("Number(%q) err = %v, want ok=%v", tt.lexeme, err, tt.ok)
			}
			if err != nil {
				return
			}
			back, err := tarski.Number(c.Text())
			if err != nil || back != c {
				t.Errorf("Number(%q).Text() = %q reads back as %v, %v", tt.lexeme, c.Text(), back, err)
			}
		})
	}
}

// FuzzNumberRoundTrip: whatever Number accepts, its canonical text is accepted and
// names the same constant.
func FuzzNumberRoundTrip(f *testing.F) {
	for _, s := range []string{"0", "-1.5", "1e999999999", "10e999999999", "9007199254740993", "0.000001", "1E+5"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, lexeme string) {
		c, err := tarski.Number(lexeme)
		if err != nil {
			return
		}
		back, err := tarski.Number(c.Text())
		if err != nil || back != c {
			t.Fatalf("Number(%q).Text() = %q reads back as %v, %v", lexeme, c.Text(), back, err)
		}
	})
}
