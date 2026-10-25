package tarski

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// number is an exact decimal: the significant digits of a value and the power of
// ten they are scaled by. It keeps what the fact holds. 1792269840000 stays
// 1792269840000, where a float64 rounded it to 1.79226984e+12, and
// 9007199254740993 stays itself, where a float64 made it the same number as its
// neighbour. It compares exactly, so two numbers a float64 cannot tell apart stay
// two numbers. Ported from Anam's internal/query (origin/master).
//
// The form is canonical: the digits carry no leading or trailing zero and the
// exponent carries the scale. One value, one form: 1 and 1.0 are one number and
// hash the same, and zero has one sign.
type number struct {
	neg    bool
	digits string // the significant digits; "0" for zero
	exp    int    // the value is sign * digits * 10^exp
}

// maxNumberDigits is the floor on how long a number's plain decimal form may be.
// A lexeme can hold a short exponent that expands to a gigabyte of digits
// ("1e999999999"), and the form is built before the answer's own size bound is
// applied; past the floor the text is the exact scientific form, which is short
// and just as exact. It is a floor, not a setting: any value of it is arbitrary,
// and it matches Anam's so that the text Anam's stored receipts hashed holds.
const maxNumberDigits = 64

// maxExponent bounds the exponent a lexeme may write, either way. It is an
// invariant, not a setting: it keeps the arithmetic on the exponent (the digits'
// count added to it, the scientific form's place) inside an int, where an exponent
// near the int's own limit wrapped past the guards and panicked (Anam card 224).
// Nine digits keeps "1e999999999" exact, and a value beyond it is refused as out of
// range before any arithmetic is done.
const maxExponent = 999_999_999

// errNotANumber and errExponentRange are why parseNumber refused a lexeme. Number
// wraps them under ErrSyntax.
var (
	errNotANumber    = errors.New("is not a number as JSON spells one")
	errExponentRange = errors.New("has an exponent out of range")
)

// parseNumber reads a JSON number lexeme into an exact decimal: an optional minus,
// an integer part with no leading zero, an optional fraction, an optional exponent.
// Nothing else is read (no plus, hex, underscore or bare point), since the
// canonical text is JSON and what comes in is held to what goes out. The value is
// what the lexeme says; nothing is rounded away.
func parseNumber(lexeme string) (number, error) {
	i := 0
	neg := false
	if i < len(lexeme) && lexeme[i] == '-' {
		neg = true
		i++
	}
	intPart, i, ok := digitRun(lexeme, i)
	if !ok || (len(intPart) > 1 && intPart[0] == '0') {
		return number{}, errNotANumber
	}
	frac := ""
	if i < len(lexeme) && lexeme[i] == '.' {
		if frac, i, ok = digitRun(lexeme, i+1); !ok {
			return number{}, errNotANumber
		}
	}
	exp, i, err := exponent(lexeme, i)
	if err != nil {
		return number{}, err
	}
	if i != len(lexeme) {
		return number{}, errNotANumber
	}
	return canonical(neg, intPart+frac, exp-len(frac)), nil
}

// exponent reads the optional exponent at i, bounded by maxExponent before any
// arithmetic is done on it, and where it ends.
func exponent(s string, i int) (exp, next int, err error) {
	if i >= len(s) || (s[i] != 'e' && s[i] != 'E') {
		return 0, i, nil
	}
	i++
	sign := 1
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		if s[i] == '-' {
			sign = -1
		}
		i++
	}
	es, i, ok := digitRun(s, i)
	if !ok {
		return 0, i, errNotANumber
	}
	e, convErr := strconv.Atoi(es)
	if convErr != nil || e > maxExponent {
		return 0, i, errExponentRange
	}
	return sign * e, i, nil
}

// canonical is the one form of sign * d * 10^exp: leading zeros dropped, trailing
// zeros moved into the exponent, and zero unsigned.
func canonical(neg bool, d string, exp int) number {
	d = strings.TrimLeft(d, "0")
	if d == "" {
		return number{digits: "0"}
	}
	trimmed := strings.TrimRight(d, "0")
	return number{neg: neg, digits: trimmed, exp: exp + len(d) - len(trimmed)}
}

// digitRun reads the run of decimal digits at i, where it ends, and whether there
// was at least one.
func digitRun(s string, i int) (string, int, bool) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[start:i], i, i > start
}

// text is the number's canonical lexeme: the exact decimal value, an integer in
// full, and the exact scientific form only when the plain one would pass
// maxNumberDigits.
func (n number) text() string {
	if n.digits == "0" {
		return "0"
	}
	s, ok := n.plain()
	if !ok {
		s = n.scientific()
	}
	if n.neg {
		return "-" + s
	}
	return s
}

// plain is the decimal expansion of the value, and whether it is short enough to
// print.
func (n number) plain() (string, bool) {
	if n.exp >= 0 {
		if len(n.digits)+n.exp > maxNumberDigits {
			return "", false
		}
		return n.digits + strings.Repeat("0", n.exp), true
	}
	place := len(n.digits) + n.exp
	switch {
	case place > 0:
		if len(n.digits)+1 > maxNumberDigits {
			return "", false
		}
		return n.digits[:place] + "." + n.digits[place:], true
	case place == 0:
		if len(n.digits)+2 > maxNumberDigits {
			return "", false
		}
		return "0." + n.digits, true
	default:
		if 2-place+len(n.digits) > maxNumberDigits {
			return "", false
		}
		return "0." + strings.Repeat("0", -place) + n.digits, true
	}
}

// scientific is the exact value in scientific form, for a value whose plain form
// is too long to print.
func (n number) scientific() string {
	mantissa := n.digits[:1]
	if len(n.digits) > 1 {
		mantissa += "." + n.digits[1:]
	}
	e := len(n.digits) - 1 + n.exp
	if e < 0 {
		return mantissa + "e-" + strconv.Itoa(-e)
	}
	return mantissa + "e+" + strconv.Itoa(e)
}

// cmp orders two numbers exactly: by sign, then by magnitude.
func (n number) cmp(o number) int {
	ns, os := n.sign(), o.sign()
	if ns != os {
		if ns < os {
			return -1
		}
		return 1
	}
	if ns == 0 {
		return 0
	}
	m := n.magnitude(o)
	if n.neg {
		return -m
	}
	return m
}

// sign is the number's sign: -1, 0 or 1.
func (n number) sign() int {
	if n.digits == "0" {
		return 0
	}
	if n.neg {
		return -1
	}
	return 1
}

// magnitude compares the absolute values of two numbers that are not zero: by the
// place of the leading digit, then digit by digit. Anam padded the shorter run with
// zeros first; canonical digits end in a non-zero digit, so a run that is a prefix
// of a longer one is already the smaller value and the padding is not needed.
func (n number) magnitude(o number) int {
	nPlace := len(n.digits) + n.exp
	oPlace := len(o.digits) + o.exp
	if nPlace != oPlace {
		if nPlace < oPlace {
			return -1
		}
		return 1
	}
	return strings.Compare(n.digits, o.digits)
}

// newNumber makes a number constant from a lexeme, the one place a number constant
// is made, so every constructor agrees on its value and its text.
func newNumber(lexeme string) (Const, error) {
	n, err := parseNumber(lexeme)
	if err != nil {
		return Const{}, fmt.Errorf("%w: %q %w", ErrSyntax, lexeme, err)
	}
	return Const{kind: KindNumber, s: n.text(), num: n}, nil
}
