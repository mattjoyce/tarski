package tarski

import (
	"errors"
	"fmt"
	"math/big"
	"sort"
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
	total := fold(ns)
	r := canonical(total.Sign() < 0, total.Abs(total).String(), low)
	if e := r.sciExponent(); e > maxExponent || e < -maxExponent {
		return number{}, &TooLarge{Bound: BoundNumber, sum: sumOverflow{exponent: true, need: e, limit: maxExponent}}
	}
	return r, nil
}

// fold is the sum of the non-zero numbers in ns as an integer count of 10^low, where
// low is the least exponent among them (at least two, as sumNumbers ensures). It works
// highest exponent first, scaling the running total by a power of ten only when the
// exponent drops, so each addend's digits are parsed once and unpadded, and the gaps it
// scales by add up to the width, never the width once per addend. Parsing a padded
// decimal string per addend did: Go's decimal parse is quadratic in the string's length,
// and a group of wide addends paid it for every witness (commit security review). The
// digits are decimal by construction, so no parse can fail. Addition is commutative,
// so the order of equal exponents does not move the value.
func fold(ns []number) *big.Int {
	nz := make([]number, 0, len(ns))
	for _, n := range ns {
		if n.digits != "0" {
			nz = append(nz, n)
		}
	}
	sort.Slice(nz, func(i, j int) bool { return nz[i].exp > nz[j].exp })
	var total, term, scale big.Int
	ten := big.NewInt(10)
	cur := nz[0].exp
	for _, n := range nz {
		if gap := cur - n.exp; gap > 0 {
			total.Mul(&total, scale.Exp(ten, big.NewInt(int64(gap)), nil))
			cur = n.exp
		}
		term.SetString(n.digits, 10)
		if n.neg {
			term.Neg(&term)
		}
		total.Add(&total, &term)
	}
	return &total
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

// sumDigitsPerStep is how many digits of alignment one step pays for when a group's
// sum is folded. A join step is a map lookup and a slice append; aligning and adding
// 64 digits in a big.Int is the same order of work. An invariant, not a knob: it is
// what keeps MaxSteps a bound on the work a sum does as well as on the join.
const sumDigitsPerStep = 64

// sumQuadDigits is the length past which a big-number operation is charged as
// quadratic: Go parses a decimal string, and multiplies a long total by a long power
// of ten, in time that grows with the square of the length once it spans many words.
// Below it the linear charge dominates. An invariant, like sumDigitsPerStep.
const sumQuadDigits = 1024

// sumWork is the steps folding cs costs, charged before fold runs so MaxSteps bounds
// the sum's work as well as the join's. It prices fold's three costs:
//   - each addend's digits parsed once: linear in their length, quadratic past
//     sumQuadDigits;
//   - one add per addend into a total up to the group's width wide;
//   - the scalings by powers of ten, whose gaps add up to the width: at most one
//     width-by-width multiplication.
//
// A group of fewer than two non-zero numbers, or one wider than maxSumDigits, costs
// nothing here: sumNumbers returns the one number, or refuses before folding.
func sumWork(cs []Const) int {
	var low, high, nonZero, parse int
	for _, c := range cs {
		n := c.num
		if c.kind != KindNumber || n.digits == "0" {
			continue
		}
		place := len(n.digits) + n.exp
		if nonZero == 0 {
			low, high = n.exp, place
		}
		low, high = min(low, n.exp), max(high, place)
		parse += quadCost(len(n.digits))
		nonZero++
	}
	width := high - low
	if nonZero < 2 || width > maxSumDigits {
		return 0
	}
	return parse + nonZero*(width/sumDigitsPerStep+1) + quadCost(width)
}

// quadCost is the steps for one big-number operation over digits decimal places:
// linear in steps of sumDigitsPerStep, times a factor that grows past sumQuadDigits.
func quadCost(digits int) int {
	return (digits/sumDigitsPerStep + 1) * (digits/sumQuadDigits + 1)
}
