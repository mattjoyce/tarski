package datalog

import (
	"errors"
	"fmt"
)

// ErrTooLarge marks an evaluation refused for its size: it derived more than
// Options.MaxBytes of text, or took more than Options.MaxSteps join steps. A more
// selective rule or a smaller base is the remedy; asking again unchanged will fail
// again.
var ErrTooLarge = errors.New("datalog: derivation too large")

// The values of TooLarge.Bound.
const (
	BoundBytes = "MaxBytes"
	BoundSteps = "MaxSteps"
)

// TooLarge is the refusal itself. Relation is the one being derived when a bound was
// crossed. Tuples and Bytes count every derived relation's tuples and, approximately,
// their JSON text; Steps counts join steps. Bound names the option that was crossed,
// "MaxBytes" or "MaxSteps", so a caller need not infer it from which fields are set.
type TooLarge struct {
	Bound    string
	Relation string
	Tuples   int
	Bytes    int
	MaxBytes int
	Steps    int
	MaxSteps int
}

func (e *TooLarge) Error() string {
	if e.Bound == BoundSteps {
		return fmt.Sprintf("datalog: derivation too large: %d join steps, over the bound of %d (Options.MaxSteps), while deriving %s; "+
			"make a rule more selective or the base smaller", e.Steps, e.MaxSteps, e.Relation)
	}
	return fmt.Sprintf("datalog: derivation too large: %d derived tuples encode to more than %d bytes (Options.MaxBytes), at %s; "+
		"make a rule more selective or the base smaller", e.Tuples, e.MaxBytes, e.Relation)
}

func (e *TooLarge) Is(target error) bool { return target == ErrTooLarge }
