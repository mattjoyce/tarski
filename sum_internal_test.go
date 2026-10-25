package tarski

import (
	"errors"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// The exact sum behind the sum aggregate (docs/design/aggregates.md §4). The value of
// a sum must be a function of the group alone, so it is exact, order-free, canonical,
// and refused rather than rounded when it would not fit.

// mustNumber parses a lexeme the test knows is good.
func mustNumber(t testing.TB, lexeme string) number {
	t.Helper()
	n, err := parseNumber(lexeme)
	if err != nil {
		t.Fatalf("parseNumber(%q): %v", lexeme, err)
	}
	return n
}

func TestNumberAdd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, a, b, want string
	}{
		{"zero plus zero", "0", "0", "0"},
		{"identity right", "81.4", "0", "81.4"},
		{"identity left", "0", "-81.4", "-81.4"},
		{"identity on a tiny number", "0", "1e-999999999", "1e-999999999"},
		{"identity on a huge number", "1e999999999", "-0", "1e+999999999"},
		{"carry", "999", "1", "1000"},
		{"carry through the point", "0.99", "0.01", "1"},
		{"both negative", "-1.5", "-2.25", "-3.75"},
		{"mixed signs, positive result", "5", "-3.5", "1.5"},
		{"mixed signs, negative result", "3.5", "-5", "-1.5"},
		{"borrow", "1000", "-0.001", "999.999"},
		{"cancellation is unsigned zero", "12.5", "-12.5", "0"},
		{"cancellation of huge values", "-1e999999999", "1e999999999", "0"},
		{"mixed scales, exact", "0.1", "0.2", "0.3"},
		{"halves make a whole", "1.5", "1.5", "3"},
		{"past 2^53", "9007199254740992", "1", "9007199254740993"},
		{"past int64", "9223372036854775807", "9223372036854775807", "18446744073709551614"},
		{"exponent forms align", "1e3", "2.5e1", "1025"},
		{"scientific in, scientific out", "1e64", "1e64", "2e+64"},
		{"scientific plus one is long", "1e70", "1", "1." + strings.Repeat("0", 69) + "1e+70"},
		{"tiny plus tiny", "1e-70", "2e-70", "3e-70"},
		{"at the exponent bound", "4.5e999999999", "4.5e999999999", "9e+999999999"},
		{"at the low exponent bound", "2e-999999999", "-1e-999999999", "1e-999999999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			a, b := mustNumber(t, tt.a), mustNumber(t, tt.b)
			got, err := a.add(b)
			if err != nil {
				t.Fatalf("%s + %s: %v", tt.a, tt.b, err)
			}
			if got.text() != tt.want {
				t.Errorf("%s + %s = %s, want %s", tt.a, tt.b, got.text(), tt.want)
			}
			if got != mustNumber(t, got.text()) {
				t.Errorf("%s + %s = %#v, not the canonical form %#v of its text", tt.a, tt.b, got, mustNumber(t, got.text()))
			}
		})
	}
}

// Zero has one sign: a cancelled sum is the very value Number("0") and Number("-0")
// give, so it is one tuple and hashes as one.
func TestNumberAdd_CancellationIsTheOneZero(t *testing.T) {
	t.Parallel()
	got, err := mustNumber(t, "-0.000001").add(mustNumber(t, "1e-6"))
	if err != nil || got != (number{digits: "0"}) || got != mustNumber(t, "-0") {
		t.Errorf("-0.000001 + 1e-6 = %#v, %v; want the unsigned zero", got, err)
	}
}

func TestNumberAdd_RefusedPastTheBounds(t *testing.T) {
	t.Parallel()
	wide := "1e" + strconv.Itoa(maxSumDigits) // with 1, spans maxSumDigits+1 places
	tests := []struct {
		name, a, b string
		exponent   bool
		need       int
		limit      int
		message    string
	}{
		{"digits: just past the width", wide, "1", false, maxSumDigits + 1, maxSumDigits, "a sum needs 65537 digits, over the bound of 65536"},
		{"digits: huge and tiny", "1e999999999", "1e-999999999", false, 1999999999, maxSumDigits, "add smaller numbers or fewer of them"},
		{"exponent: carry past the top", "9e999999999", "9e999999999", true, maxExponent + 1, maxExponent, "a sum has the exponent 1000000000"},
		{"exponent: cancel past the bottom", "1.5e-999999999", "-1e-999999999", true, -maxExponent - 1, maxExponent, "so its text would not read back"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := mustNumber(t, tt.a).add(mustNumber(t, tt.b))
			var tl *TooLarge
			if !errors.As(err, &tl) || !errors.Is(err, ErrTooLarge) {
				t.Fatalf("%s + %s: err = %v, want *TooLarge", tt.a, tt.b, err)
			}
			want := sumOverflow{exponent: tt.exponent, need: tt.need, limit: tt.limit}
			if tl.Bound != BoundNumber || tl.sum != want {
				t.Errorf("%s + %s: Bound %q, %+v; want %q, %+v", tt.a, tt.b, tl.Bound, tl.sum, BoundNumber, want)
			}
			if msg := err.Error(); !strings.HasPrefix(msg, "tarski:") || !strings.Contains(msg, tt.message) {
				t.Errorf("message %q, want it to start tarski: and say %q", msg, tt.message)
			}
		})
	}
	// At the width bound itself the sum is taken: the bound refuses past it, not at it.
	got, err := mustNumber(t, "1e"+strconv.Itoa(maxSumDigits-1)).add(mustNumber(t, "1"))
	if err != nil || len(got.digits) != maxSumDigits {
		t.Errorf("a sum exactly maxSumDigits wide: %d digits, %v", len(got.digits), err)
	}
	// A number already wider than the bound is still itself plus zero: zero has no digit
	// to align. Plus anything else, the sum is refused.
	long := mustNumber(t, strings.Repeat("7", maxSumDigits+10))
	if got, err := long.add(number{digits: "0"}); err != nil || got != long {
		t.Errorf("a %d-digit number plus 0: %v", maxSumDigits+10, err)
	}
	if _, err := long.add(mustNumber(t, "1")); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a %d-digit number plus 1: err = %v, want ErrTooLarge", maxSumDigits+10, err)
	}
}

// The message names the relation once evaluation fills it in, and says what to change.
func TestTooLarge_NumberMessage(t *testing.T) {
	t.Parallel()
	e := &TooLarge{Bound: BoundNumber, Relation: "spent", sum: sumOverflow{need: 70001, limit: maxSumDigits}}
	want := "tarski: derivation too large: a sum needs 70001 digits, over the bound of 65536, while deriving spent; " +
		"add smaller numbers or fewer of them"
	if e.Error() != want {
		t.Errorf("Error() = %q\nwant %q", e.Error(), want)
	}
}

// randomDecimal is an exact decimal lexeme of up to 30 digits, a sign and a point
// anywhere, sometimes with an exponent, so the property tests cross scales.
func randomDecimal(r *rand.Rand) string {
	var b strings.Builder
	if r.IntN(2) == 0 {
		b.WriteByte('-')
	}
	digits := 1 + r.IntN(30)
	b.WriteByte(byte('1' + r.IntN(9)))
	for range digits - 1 {
		b.WriteByte(byte('0' + r.IntN(10)))
	}
	s := b.String()
	if point := r.IntN(digits + 1); point > 0 && point < digits {
		at := len(s) - digits + point
		s = s[:at] + "." + s[at:]
	}
	if r.IntN(3) == 0 {
		s += "e" + strconv.Itoa(r.IntN(81)-40)
	}
	return s
}

// Order-independence is the determinism promise (design I3): a sum over a set is the
// same number however the set is walked. Pairwise commutativity and associativity, and
// the one-pass sum of every permutation tried, all agree.
func TestNumberAdd_OrderFree(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(6, 6))
	for range 2000 {
		a, b, c := mustNumber(t, randomDecimal(r)), mustNumber(t, randomDecimal(r)), mustNumber(t, randomDecimal(r))
		ab, err1 := a.add(b)
		ba, err2 := b.add(a)
		if err1 != nil || err2 != nil || ab != ba {
			t.Fatalf("%s + %s = %s, but the other way %s (%v, %v)", a.text(), b.text(), ab.text(), ba.text(), err1, err2)
		}
		bc, _ := b.add(c)
		left, err1 := ab.add(c)
		right, err2 := a.add(bc)
		if err1 != nil || err2 != nil || left != right {
			t.Fatalf("(%s + %s) + %s = %s, but %s + (%s + %s) = %s", a.text(), b.text(), c.text(), left.text(),
				a.text(), b.text(), c.text(), right.text())
		}
		set := []number{a, b, c, ab, bc}
		want, _ := sumNumbers(set)
		r.Shuffle(len(set), func(i, j int) { set[i], set[j] = set[j], set[i] })
		if got, err := sumNumbers(set); err != nil || got != want {
			t.Fatalf("a permutation summed to %s, %v; want %s", got.text(), err, want.text())
		}
		if s := b.sign(); ab.cmp(a) != s {
			t.Fatalf("%s + %s = %s, which compares %d to %s; want %d", a.text(), b.text(), ab.text(), ab.cmp(a), a.text(), s)
		}
	}
}

// The refusal is as order-free as the value: the whole set's width and the result's
// exponent decide it, never which addends happened to meet first.
func TestSumNumbers_RefusalIsOrderFree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		set     []string
		want    string // "" when refused
		refused bool
	}{
		{"wide in every order", []string{"1e70000", "1", "-1e70000"}, "", true},
		{"a carry the set cancels", []string{"9e999999999", "9e999999999", "-9e999999999"}, "9e+999999999", false},
		{"empty is zero", nil, "0", false},
		{"zeros are zero", []string{"0", "-0", "0e5"}, "0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ns := make([]number, len(tt.set))
			for i, s := range tt.set {
				ns[i] = mustNumber(t, s)
			}
			for _, perm := range permutations(len(ns)) {
				p := make([]number, len(ns))
				for i, j := range perm {
					p[i] = ns[j]
				}
				got, err := sumNumbers(p)
				if refused := errors.Is(err, ErrTooLarge); refused != tt.refused || (!refused && got.text() != tt.want) {
					t.Errorf("order %v: %s, %v; want %q, refused %v", perm, got.text(), err, tt.want, tt.refused)
				}
			}
		})
	}
}

// permutations lists every ordering of 0..n-1.
func permutations(n int) [][]int {
	if n == 0 {
		return [][]int{{}}
	}
	var out [][]int
	for _, p := range permutations(n - 1) {
		for i := 0; i <= len(p); i++ {
			q := append(append(append([]int(nil), p[:i]...), n-1), p[i:]...)
			out = append(out, q)
		}
	}
	return out
}

func TestSumConsts(t *testing.T) {
	t.Parallel()
	c := func(lexeme string) Const {
		k, err := Number(lexeme)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}
	tests := []struct {
		name    string
		in      []Const
		want    string
		kindErr string // the constant named when an addend is not a number
	}{
		{"empty group is 0", nil, "0", ""},
		{"one", []Const{c("12.5")}, "12.5", ""},
		{"halves", []Const{c("1.5"), c("1.5")}, "3", ""},
		{"ledger", []Const{c("19.99"), c("0.01"), c("-5"), Int(100)}, "115", ""},
		{"a string", []Const{c("1"), String("12")}, "", `"12"`},
		{"a bool", []Const{Bool(true), c("1")}, "", "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := sumConsts(tt.in)
			if tt.kindErr != "" {
				if !errors.Is(err, errSumKind) || !strings.HasPrefix(err.Error(), tt.kindErr+" ") {
					t.Errorf("err = %v, want errSumKind naming %s", err, tt.kindErr)
				}
				return
			}
			if err != nil || got.Text() != tt.want || got != c(tt.want) {
				t.Errorf("sum = %q (%#v), %v; want the constant Number(%q)", got.Text(), got, err, tt.want)
			}
		})
	}
	wide, _ := Number("1e70000")
	if _, err := sumConsts([]Const{wide, Int(1)}); !errors.Is(err, ErrTooLarge) {
		t.Errorf("a sum past maxSumDigits: err = %v, want ErrTooLarge", err)
	}
}

// FuzzNumberAdd: for two accepted lexemes, add is commutative, refuses in both orders
// alike, agrees with cmp, adds Number("0") as the identity, and its text reads back
// through Number as the very constant sumConsts makes.
func FuzzNumberAdd(f *testing.F) {
	for _, s := range [][2]string{
		{"0.1", "0.2"}, {"-1.5", "1.5"}, {"1e999999999", "1e-999999999"}, {"9e999999999", "9e999999999"},
		{"9007199254740993", "-1"}, {"1e64", "1e-63"}, {"0", "-0"}, {"123.456e-7", "-9.9e3"},
	} {
		f.Add(s[0], s[1])
	}
	zero, err := Number("0")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, x, y string) {
		cx, errX := Number(x)
		cy, errY := Number(y)
		if errX != nil || errY != nil {
			return
		}
		a, b := cx.num, cy.num
		for _, n := range []number{a, b} {
			if id, err := n.add(zero.num); err != nil || id != n {
				t.Fatalf("%s + 0 = %s, %v", n.text(), id.text(), err)
			}
		}
		ab, err1 := a.add(b)
		ba, err2 := b.add(a)
		if (err1 == nil) != (err2 == nil) || ab != ba {
			t.Fatalf("%s + %s = %s, %v; the other way %s, %v", x, y, ab.text(), err1, ba.text(), err2)
		}
		if err1 != nil {
			if !errors.Is(err1, ErrTooLarge) {
				t.Fatalf("%s + %s refused with %v, want ErrTooLarge", x, y, err1)
			}
			return
		}
		if ab.cmp(a) != b.sign() {
			t.Fatalf("%s + %s = %s compares %d to %s", x, y, ab.text(), ab.cmp(a), x)
		}
		sum, err := sumConsts([]Const{cx, cy})
		back, errBack := Number(ab.text())
		if err != nil || errBack != nil || back != sum || back.num != ab {
			t.Fatalf("%s + %s = %q does not read back: %v, %v", x, y, ab.text(), err, errBack)
		}
	})
}
