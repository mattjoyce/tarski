package datalog_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/mattjoyce/shearline/datalog"
)

// Number takes finite numbers only. NaN would compare equal to everything, and Inf has
// no JSON text, so neither becomes a constant; a lexeme that is no number at all says so.
func TestNumber_RefusesWhatIsNotFinite(t *testing.T) {
	t.Parallel()
	for _, lexeme := range []string{"NaN", "nan", "Inf", "-inf", "+Infinity", "1e999", "-1e999"} {
		_, err := datalog.Number(lexeme)
		if !errors.Is(err, datalog.ErrSyntax) || !strings.Contains(err.Error(), "not a finite number") {
			t.Errorf("Number(%q) = %v, want ErrSyntax, not a finite number", lexeme, err)
		}
	}
	if _, err := datalog.Number("twelve"); !errors.Is(err, datalog.ErrSyntax) || !strings.Contains(err.Error(), "is not a number") {
		t.Errorf("Number(twelve) = %v, want ErrSyntax, is not a number", err)
	}
	var c datalog.Const
	if err := c.UnmarshalJSON([]byte(`NaN`)); err == nil {
		t.Error("a constant read back from NaN was accepted")
	}
}

// Negative zero is zero: one lexeme, one identity, and an index probe for 0 finds -0
// as the comparison already said it should.
func TestNumber_NegativeZeroIsZero(t *testing.T) {
	t.Parallel()
	for _, lexeme := range []string{"-0", "-0.0", "0", "-0e5"} {
		c, err := datalog.Number(lexeme)
		if err != nil || c.Text() != "0" {
			t.Errorf("Number(%q) = %q, %v; want 0", lexeme, c.Text(), err)
		}
	}
	negative, _ := datalog.Number("-0")
	base := newBase(t, datalog.Fact{Relation: "score", Tuple: datalog.Tuple{datalog.String("a"), negative}})
	text := `probe(E) :- score(E, 0). cmp(E) :- score(E, X), X = 0. lit(E) :- score(E, -0).`
	for _, rel := range []string{"probe", "cmp", "lit"} {
		ans, err := tryRun(base, text, rel, datalog.Options{})
		if err != nil || ans.Len() != 1 {
			t.Errorf("%s over a -0 fact = %v, %d tuples; want one", rel, err, ans.Len())
		}
	}
}

// A string with a NUL in it cannot make two different tuples one: both survive.
func TestTuple_NulDoesNotCollapseTuples(t *testing.T) {
	t.Parallel()
	b := newBase(t, fact("edge", "a\x000", "b"), fact("edge", "a", "\x000b"), fact("edge", "a\x00", "0b"))
	if got := b.Len("edge"); got != 3 {
		t.Fatalf("Len(edge) = %d, want 3: tuples with NUL collapsed", got)
	}
	ans, err := tryRun(b, `e(X, Y) :- edge(X, Y).`, "e", datalog.Options{})
	if err != nil || ans.Len() != 3 {
		t.Errorf("derived %d tuples, %v; want 3", ans.Len(), err)
	}
}

// Int and Float make the constants Number would make from the same text, so a Go
// value and a parsed literal are one constant.
func TestIntAndFloat_AgreeWithNumber(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		made datalog.Const
		text string
	}{
		{datalog.Int(14), "14"}, {datalog.Int(-3), "-3"}, {datalog.Int(0), "0"},
		{datalog.Int(1 << 62), "4611686018427387904"},
	} {
		want, err := datalog.Number(c.text)
		if err != nil || c.made != want || c.made.Text() != want.Text() {
			t.Errorf("Int made %q, Number(%q) = %q, %v", c.made.Text(), c.text, want.Text(), err)
		}
	}
	for f, text := range map[float64]string{1: "1", 1.5: "1.5", -2.25: "-2.25", 1e21: "1e21", 146.0: "146"} {
		got, err := datalog.Float(f)
		want, _ := datalog.Number(text)
		if err != nil || got != want || got.Kind() != datalog.KindNumber {
			t.Errorf("Float(%v) = %q, %v; want %q", f, got.Text(), err, want.Text())
		}
	}
	negZero := math.Copysign(0, -1)
	if z, err := datalog.Float(negZero); err != nil || z.Text() != "0" || z != datalog.Int(0) {
		t.Errorf("Float(-0.0) = %q, %v; want 0, the same constant as Int(0)", z.Text(), err)
	}
	for _, f := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, err := datalog.Float(f); !errors.Is(err, datalog.ErrInvalid) {
			t.Errorf("Float(%v) = %v, want ErrInvalid", f, err)
		}
	}
}
