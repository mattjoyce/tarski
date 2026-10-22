package datalog_test

import (
	"errors"
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
