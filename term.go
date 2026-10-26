package tarski

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
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

// Const is one constant: a string, a number or a boolean. A number is an exact
// decimal: 1792269840000 stays 1792269840000 and 9007199254740993 stays itself,
// where a float64 rounded both. It keeps one canonical lexeme, so that 1 and 1.0
// are the same constant and hash the same.
type Const struct {
	kind Kind
	s    string
	num  number
}

// String makes a string constant. The engine's string domain is valid UTF-8: JSON
// would write an invalid byte and U+FFFD as the same text, so two different facts
// would share an answer and its hash. The two entry points, the lexer and
// NewBase and Base.With, refuse a string that is not valid UTF-8. String itself stays
// total and does not check, so a Const built here is checked where it enters a Base.
func String(s string) Const { return Const{kind: KindString, s: s} }

// Bool makes a boolean constant.
func Bool(b bool) Const { return Const{kind: KindBool, s: strconv.FormatBool(b)} }

// Number makes a number constant from a JSON lexeme. The value is exact: the lexeme
// is read as the decimal it is, not as a float64 that rounds it. A lexeme that is
// not a number as JSON spells one is ErrSyntax, and so is an exponent beyond nine
// digits either way. NaN and the infinities have no JSON text and are refused as
// not finite. Negative zero is zero, so that identity and comparison agree on it.
func Number(lexeme string) (Const, error) {
	c, err := newNumber(lexeme)
	if err != nil && notFinite(lexeme) {
		return Const{}, fmt.Errorf("%w: %q is not a finite number", ErrSyntax, lexeme)
	}
	return c, err
}

// notFinite reports whether s spells NaN or an infinity, so that Number can say why
// it refused one.
func notFinite(s string) bool {
	f, err := strconv.ParseFloat(s, 64)
	return err == nil && (math.IsNaN(f) || math.IsInf(f, 0))
}

// Int makes a number constant from an integer, the same constant Number gives for its
// decimal text. Every int64 is held exactly.
func Int(n int64) Const {
	d := strconv.FormatInt(n, 10)
	neg := n < 0
	if neg {
		d = d[1:]
	}
	num := canonical(neg, d, 0)
	return Const{kind: KindNumber, s: num.text(), num: num}
}

// Float makes a number constant from a float64. Its value is the shortest decimal
// that reads back as f (strconv's 'g' format at precision -1), held exactly, so
// Float(1.5) equals Number("1.5"), Float(1e21) equals Number("1e21"), and -0 is 0.
// NaN and the infinities are ErrInvalid, for the reasons Number refuses them.
func Float(f float64) (Const, error) {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return Const{}, fmt.Errorf("%w: %v is not a finite number", ErrInvalid, f)
	}
	return newNumber(strconv.FormatFloat(f, 'g', -1, 64))
}

// Kind reports the constant's kind.
func (c Const) Kind() Kind { return c.kind }

// Text is the constant as text: the string itself, the number's canonical lexeme,
// or true/false.
func (c Const) Text() string { return c.s }

// MarshalJSON writes the constant as the JSON value it came from. A string that is not
// valid UTF-8 is outside the engine's string domain (see String) and is ErrInvalid:
// JSON would write it as U+FFFD, the text of a different constant.
func (c Const) MarshalJSON() ([]byte, error) {
	if c.kind == KindString {
		if !utf8.ValidString(c.s) {
			return nil, fmt.Errorf("%w: a string constant that is not valid UTF-8 has no JSON text", ErrInvalid)
		}
		return json.Marshal(c.s)
	}
	return []byte(c.s), nil
}

// UnmarshalJSON reads a constant back from the JSON MarshalJSON wrote, so an answer
// decoded on the client holds the constants the server's did. b is exactly one JSON
// value, whitespace aside, in the engine's string domain: text that is not valid UTF-8,
// or a \u escape of half a surrogate pair, is ErrSyntax, never read as U+FFFD.
func (c *Const) UnmarshalJSON(b []byte) error {
	if !utf8.Valid(b) {
		return fmt.Errorf("%w: a constant is valid UTF-8, but %q is not", ErrSyntax, b)
	}
	if loneSurrogate(b) {
		return fmt.Errorf("%w: %s escapes half a UTF-16 surrogate pair, which is no character", ErrSyntax, b)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
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

// loneSurrogate reports whether the JSON text b holds a \u escape of a UTF-16 surrogate
// that is not one half of a pair, a high surrogate escape followed at once by a low one.
// encoding/json reads such an escape as U+FFFD, so two different texts would decode to
// one constant.
func loneSurrogate(b []byte) bool {
	high := false // the last escape read was a high surrogate, awaiting its low half
	for i := 0; i < len(b); i++ {
		r, ok := uEscape(b, i)
		switch {
		case ok && utf16.IsSurrogate(r) && r < 0xdc00:
			if high {
				return true
			}
			high = true
			i += len(`\u0000`) - 1
		case ok && utf16.IsSurrogate(r):
			if !high {
				return true
			}
			high = false
			i += len(`\u0000`) - 1
		case high:
			return true
		case b[i] == '\\':
			i++ // the escaped character: \\u is a backslash and a u, not an escape
		}
	}
	return high
}

// uEscape is the code unit of the \u escape at b[i], and false when none starts there.
func uEscape(b []byte, i int) (rune, bool) {
	if i+len(`\u0000`) > len(b) || b[i] != '\\' || b[i+1] != 'u' {
		return 0, false
	}
	n, err := strconv.ParseUint(string(b[i+2:i+6]), 16, 16)
	return rune(n), err == nil
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
// fixed-width, so that is chronological), numbers exactly by value, false before
// true.
// ok is false when the kinds differ: they are unequal, and not ordered.
func compare(a, b Const) (cmp int, ok bool) {
	if a.kind != b.kind {
		return 0, false
	}
	switch a.kind {
	case KindNumber:
		return a.num.cmp(b.num), true
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
