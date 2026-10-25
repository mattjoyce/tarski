package tarski

import (
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// maxSumDigits bounds the aligned width of an exact sum: the number of decimal places
// from the lowest digit of any addend to the highest. Alignment is what makes a sum
// exact, and it is also what can make it huge: 1e999999999 + 1e-999999999 spans two
// billion digits, though each addend's text is short. It is an invariant, not an
// option. A text that wide would be refused by any sane Options.MaxBytes, but only
// after it was built; this constant refuses the sum before the allocation, so the
// bound cannot be crossed on the way to being checked. Its value is a power of two
// wide enough for any ledger total and small enough that one sum costs kilobytes.
const maxSumDigits = 1 << 16

// errSumKind is why a sum was not taken: an addend is not a number. It is not an
// evaluation error; an aggregate that meets it derives no tuple for its group.
var errSumKind = errors.New("is not a number, so it cannot be summed")

// add is the exact sum of two numbers: digits aligned to the smaller exponent, added
// as big integers (math/big is stdlib), then canonical. It is associative and
// commutative, so a fold over a set is the same number in every order. A sum wider
// than maxSumDigits, or whose text would write an exponent beyond maxExponent and so
// not read back, is *TooLarge with Bound BoundNumber: fail closed, never rounded.
func (n number) add(o number) (number, error) {
	return sumNumbers([]number{n, o})
}

// sumNumbers is the exact sum of every number in ns, and 0 for none. It aligns all of
// them to the lowest exponent among them and adds them as big integers in one pass, so
// it is the fold of add whenever that fold succeeds, and it also refuses or accepts
// as a function of the set alone. A pairwise fold would not: {1e70000, 1, -1e70000}
// folded left to right passes the width bound only if the two large addends meet
// first, and {9e999999999, 9e999999999, -9e999999999} passes the exponent bound only
// if the last two do. Checking the whole set's width up front, and the exponent on the
// result only, makes the refusal as order-free as the value.
//
// Zero has no digit to align (its exponent 0 is only its form), so zeros are skipped,
// and a set with one non-zero addend sums to that addend unchanged: it is already
// canonical and in range, and x + 0 is x even for an x whose own digits pass the
// width bound.
func sumNumbers(ns []number) (number, error) {
	var low, high, nonZero int
	var only number
	for _, n := range ns {
		if n.digits == "0" {
			continue
		}
		place := len(n.digits) + n.exp
		if nonZero == 0 {
			low, high = n.exp, place
		}
		low, high, only = min(low, n.exp), max(high, place), n
		nonZero++
	}
	switch nonZero {
	case 0:
		return number{digits: "0"}, nil
	case 1:
		return only, nil
	}
	if width := high - low; width > maxSumDigits {
		return number{}, &TooLarge{Bound: BoundNumber, sum: sumOverflow{need: width, limit: maxSumDigits}}
	}
	var total, term big.Int
	for _, n := range ns {
		if n.digits != "0" {
			total.Add(&total, n.aligned(low, &term))
		}
	}
	r := canonical(total.Sign() < 0, total.Abs(&total).String(), low)
	if e := r.sciExponent(); e > maxExponent || e < -maxExponent {
		return number{}, &TooLarge{Bound: BoundNumber, sum: sumOverflow{exponent: true, need: e, limit: maxExponent}}
	}
	return r, nil
}

// aligned sets v to the number's value as an integer count of 10^low, for a low no
// greater than its exponent, and returns v. The digits are decimal by construction,
// so the parse cannot fail.
func (n number) aligned(low int, v *big.Int) *big.Int {
	v.SetString(n.digits+strings.Repeat("0", n.exp-low), 10)
	if n.neg {
		v.Neg(v)
	}
	return v
}

// sumConsts is the exact sum of a group's constants, the value of a sum aggregate: 0
// for an empty group, as Soufflé gives, and the number's canonical text otherwise, so
// a sum reads back through Number like any parsed number (1.5 + 1.5 is 3). The first
// constant that is not a number, in the order given, is errSumKind. A sum past
// maxSumDigits or maxExponent is *TooLarge with Bound BoundNumber; the caller fills in
// the relation being derived.
func sumConsts(cs []Const) (Const, error) {
	ns := make([]number, len(cs))
	for i, c := range cs {
		if c.kind != KindNumber {
			return Const{}, fmt.Errorf("%s %w", c.text(), errSumKind)
		}
		ns[i] = c.num
	}
	n, err := sumNumbers(ns)
	if err != nil {
		return Const{}, err
	}
	return Const{kind: KindNumber, s: n.text(), num: n}, nil
}
