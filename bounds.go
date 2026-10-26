package tarski

import (
	"errors"
	"fmt"
)

// ErrTooLarge marks an evaluation refused for its size: it derived more than
// Options.MaxBytes of text, took more than Options.MaxSteps steps (Options.MaxSteps
// says what a step is), or summed to a number too wide to hold exactly. A more selective rule or a smaller base is
// the remedy; asking again unchanged will fail again.
var ErrTooLarge = errors.New("tarski: derivation too large")

// The values of TooLarge.Bound. BoundBytes and BoundSteps name the option that was
// crossed. BoundNumber names no option: it is the engine's own bound on an exact
// sum, which no setting moves: a sum is refused when its aligned width passes 65536
// decimal places, or when its scientific exponent passes 999999999 either way.
const (
	BoundBytes  = "MaxBytes"
	BoundSteps  = "MaxSteps"
	BoundNumber = "Number"
)

// TooLarge is the refusal itself. Relation is the one being derived when a bound was
// crossed. Tuples and Bytes count every derived relation's tuples and, approximately,
// their JSON text; Steps counts steps as Options.MaxSteps defines them, on the scale of
// Result.Cost. Bound names what was crossed,
// BoundBytes, BoundSteps or BoundNumber, so a caller need not infer it from which
// fields are set.
type TooLarge struct {
	Bound    string
	Relation string
	Tuples   int
	Bytes    int
	MaxBytes int
	Steps    int
	MaxSteps int

	// sum says why a BoundNumber refusal was made, for its message. It is unexported
	// so that the bound adds no field to the public API.
	sum sumOverflow
}

// sumOverflow is the detail of a BoundNumber refusal: the aligned width a sum
// needed against maxSumDigits, or, when exponent is set, the scientific exponent
// its result would write against maxExponent.
type sumOverflow struct {
	exponent bool
	need     int
	limit    int
}

func (e *TooLarge) Error() string {
	switch e.Bound {
	case BoundSteps:
		return fmt.Sprintf("tarski: derivation too large: %d steps, over the bound of %d (Options.MaxSteps), while deriving %s; "+
			"make a rule more selective or the base smaller", e.Steps, e.MaxSteps, e.Relation)
	case BoundNumber:
		return e.numberError()
	}
	return fmt.Sprintf("tarski: derivation too large: %d derived tuples encode to more than %d bytes (Options.MaxBytes), at %s; "+
		"make a rule more selective or the base smaller", e.Tuples, e.MaxBytes, e.Relation)
}

// numberError is the message of a BoundNumber refusal. The bound is not an option,
// so the remedy it names is in the data: the numbers summed.
func (e *TooLarge) numberError() string {
	at := ""
	if e.Relation != "" {
		at = ", while deriving " + e.Relation
	}
	if e.sum.exponent {
		return fmt.Sprintf("tarski: derivation too large: a sum has the exponent %d, beyond the bound of %d either way, so its text "+
			"would not read back%s; sum numbers nearer in size, or fewer of them", e.sum.need, e.sum.limit, at)
	}
	return fmt.Sprintf("tarski: derivation too large: a sum needs %d digits, over the bound of %d%s; "+
		"add smaller numbers or fewer of them", e.sum.need, e.sum.limit, at)
}

// Is reports whether target is ErrTooLarge, so errors.Is(err, ErrTooLarge) holds for a *TooLarge.
func (e *TooLarge) Is(target error) bool { return target == ErrTooLarge }
