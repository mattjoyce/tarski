package datalog

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Kind is the type of a constant. Datalog sees the three scalar kinds JSON has, and
// nothing else: no null, no list, no object.
type Kind uint8

// The kinds, in the order constants of different kinds sort.
const (
	KindString Kind = iota
	KindNumber
	KindBool
)

// Const is one constant: a string, a number or a boolean. Numbers are float64, as
// JSON readers commonly take them, and keep one canonical lexeme so that 1 and 1.0
// are the same constant and hash the same.
type Const struct {
	kind Kind
	s    string
	n    float64
}

// String makes a string constant. The engine's string domain is valid UTF-8: JSON
// would write an invalid byte and U+FFFD as the same text, so two different facts
// would share an answer and its hash. The two entry points, the lexer and
// NewBase and Base.With, refuse a string that is not valid UTF-8. String itself stays
// total and does not check, so a Const built here is checked where it enters a Base.
func String(s string) Const { return Const{kind: KindString, s: s} }

// Bool makes a boolean constant.
func Bool(b bool) Const { return Const{kind: KindBool, s: strconv.FormatBool(b)} }

// Number makes a number constant from a JSON lexeme. A lexeme that is not a finite
// number is refused: NaN would equal everything under compare, and Inf has no JSON
// text. Negative zero is zero, so that identity and comparison agree on it.
func Number(lexeme string) (Const, error) {
	n, err := strconv.ParseFloat(lexeme, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return Const{}, fmt.Errorf("%w: %q is not a number", ErrSyntax, lexeme)
	}
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return Const{}, fmt.Errorf("%w: %q is not a finite number", ErrSyntax, lexeme)
	}
	if !isJSONNumber(lexeme) {
		return Const{}, fmt.Errorf("%w: %q is not a number as JSON spells one", ErrSyntax, lexeme)
	}
	return number(n), nil
}

// isJSONNumber reports whether s is a number as JSON spells one: an optional minus,
// an integer part with no leading zero, an optional fraction, an optional exponent.
// Go's ParseFloat reads more (a plus, a leading zero, hex, underscores, a bare point)
// and the canonical lexeme is JSON, so what comes in is held to what goes out.
func isJSONNumber(s string) bool {
	i := 0
	if i < len(s) && s[i] == '-' {
		i++
	}
	digits := func() int {
		start := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		return i - start
	}
	switch {
	case i < len(s) && s[i] == '0':
		i++
	case digits() == 0:
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if digits() == 0 {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if digits() == 0 {
			return false
		}
	}
	return i == len(s)
}

// Int makes a number constant from an integer, the same constant Number gives for its
// decimal text. Like every number here it is a float64 underneath, so an integer
// beyond 2^53 is held as the nearest one that fits.
func Int(n int64) Const { return number(float64(n)) }

// Float makes a number constant from a float64, the same constant Number gives for the
// value's text: Float(1) equals Number("1"), and -0 is 0. NaN and the infinities are
// ErrInvalid, for the reasons Number refuses them.
func Float(f float64) (Const, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Const{}, fmt.Errorf("%w: %v is not a finite number", ErrInvalid, f)
	}
	return number(f), nil
}

// number is the one place a number constant is made from a finite float64, so every
// constructor agrees on its lexeme and on zero having one sign.
func number(n float64) Const {
	if n == 0 {
		n = 0 // not -0
	}
	return Const{kind: KindNumber, s: strconv.FormatFloat(n, 'g', -1, 64), n: n}
}

// Kind reports the constant's kind.
func (c Const) Kind() Kind { return c.kind }

// Text is the constant as text: the string itself, the number's canonical lexeme,
// or true/false.
func (c Const) Text() string { return c.s }

// MarshalJSON writes the constant as the JSON value it came from.
func (c Const) MarshalJSON() ([]byte, error) {
	if c.kind == KindString {
		return json.Marshal(c.s)
	}
	return []byte(c.s), nil
}

// UnmarshalJSON reads a constant back from the JSON MarshalJSON wrote, so an answer
// decoded on the client holds the constants the server's did.
func (c *Const) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("%w: a constant is one JSON value, but %s goes on after the first", ErrSyntax, b)
	}
	switch t := v.(type) {
	case string:
		*c = String(t)
	case bool:
		*c = Bool(t)
	case json.Number:
		n, err := Number(t.String())
		if err != nil {
			return err
		}
		*c = n
	default:
		return fmt.Errorf("%w: a constant is a string, a number or a boolean, not %s", ErrSyntax, b)
	}
	return nil
}

// key is the constant's identity for sets and joins. The kind prefix keeps the
// string "1" apart from the number 1, and the length prefix makes the key
// self-delimiting, so a tuple's key is the concatenation of its constants' and a NUL
// (or any byte) inside a string cannot move a boundary.
func (c Const) key() string {
	var b strings.Builder
	b.Grow(keyOverhead + len(c.s))
	c.writeKey(&b)
	return b.String()
}

// keyOverhead is the room a constant's key needs beyond its text: the kind byte, a
// length of up to three digits, and the colon. It is only a size hint for Grow.
const keyOverhead = 5

// same reports whether two constants are one constant, as their keys would say, without
// building either key.
func (c Const) same(o Const) bool { return c.kind == o.kind && c.s == o.s }

// writeKey writes the constant's key to b, the one place the format is made, so a
// tuple's key is built in a single buffer instead of one string per constant.
func (c Const) writeKey(b *strings.Builder) {
	var digits [20]byte
	b.WriteByte(byte('0' + c.kind))
	b.Write(strconv.AppendInt(digits[:0], int64(len(c.s)), 10))
	b.WriteByte(':')
	b.WriteString(c.s)
}

// compare orders two constants of one kind: strings lexically (fact-times are
// fixed-width, so that is chronological), numbers numerically, false before true.
// ok is false when the kinds differ: they are unequal, and not ordered.
func compare(a, b Const) (cmp int, ok bool) {
	if a.kind != b.kind {
		return 0, false
	}
	switch a.kind {
	case KindNumber:
		switch {
		case a.n < b.n:
			return -1, true
		case a.n > b.n:
			return 1, true
		}
		return 0, true
	default:
		return strings.Compare(a.s, b.s), true
	}
}

// less is the total order over constants an answer is sorted by: by kind, then by
// the kind's own order.
func less(a, b Const) bool {
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	cmp, _ := compare(a, b)
	return cmp < 0
}

// Tuple is one row of a relation.
type Tuple []Const

func (t Tuple) key() string {
	var b strings.Builder
	size := 0
	for _, c := range t {
		size += keyOverhead + len(c.s)
	}
	b.Grow(size)
	for _, c := range t {
		c.writeKey(&b)
	}
	return b.String()
}

// tupleLess orders tuples positionally.
func tupleLess(a, b Tuple) bool {
	for i := range a {
		if i >= len(b) {
			return false
		}
		if less(a[i], b[i]) {
			return true
		}
		if less(b[i], a[i]) {
			return false
		}
	}
	return len(a) < len(b)
}
