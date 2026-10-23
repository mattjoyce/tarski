package datalog

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
)

// Answer is one relation of a result as a value: its tuples in one order, with one
// serialisation and a hash over it, so two evaluations of one program over one value
// answer identically and a receipt can pin what was answered. Nothing in it can be
// changed: Tuples returns a copy, Bytes returns a copy, and the JSON it marshals to
// is Bytes, so what a holder sends, stores and hashes is one thing. A zero Answer
// has no relation, no tuples, no bytes and no hash.
type Answer struct {
	relation string
	tuples   []Tuple
	bytes    []byte
	hash     string
}

// Relation returns the named relation of the result as an Answer. A relation the
// program never named is an error, not an empty answer.
func (r Result) Relation(name string) (Answer, error) {
	rel, ok := r.relations[name]
	if !ok {
		return Answer{}, fmt.Errorf("%w: no relation %s in the result", ErrInvalid, name)
	}
	// Each tuple is cloned: a base relation's tuples are shared by every evaluation, and
	// an answer owns its own.
	tuples := make([]Tuple, len(rel.tuples))
	for i, t := range rel.tuples {
		tuples[i] = slices.Clone(t)
	}
	sort.Slice(tuples, func(i, j int) bool { return tupleLess(tuples[i], tuples[j]) })
	return sealed(name, tuples)
}

// sealed is the answer for relation name over tuples, already sorted, with its one
// serialisation and the hash over it.
func sealed(name string, tuples []Tuple) (Answer, error) {
	b, err := json.Marshal(struct {
		Relation string  `json:"relation"`
		Tuples   []Tuple `json:"tuples"`
	}{name, tuples})
	if err != nil {
		return Answer{}, fmt.Errorf("datalog: serialise answer: %w", err)
	}
	sum := sha256.Sum256(b)
	return Answer{relation: name, tuples: tuples, bytes: b, hash: "sha256:" + hex.EncodeToString(sum[:])}, nil
}

// Relation is the relation the answer is of.
func (a Answer) Relation() string { return a.relation }

// Tuples is the answer's tuples, sorted, as a copy the caller may change.
func (a Answer) Tuples() []Tuple {
	out := make([]Tuple, len(a.tuples))
	for i, t := range a.tuples {
		out[i] = slices.Clone(t)
	}
	return out
}

// Len is how many tuples the answer holds.
func (a Answer) Len() int { return len(a.tuples) }

// Bytes is the answer's one serialisation, {"relation": ..., "tuples": [...]}, as a
// copy the caller may change.
func (a Answer) Bytes() []byte { return slices.Clone(a.bytes) }

// Hash is "sha256:<hex>" over Bytes.
func (a Answer) Hash() string { return a.hash }

// MarshalJSON writes Bytes. A zero Answer is not an answer and is an error.
func (a Answer) MarshalJSON() ([]byte, error) {
	if a.bytes == nil {
		return nil, fmt.Errorf("%w: a zero Answer has nothing to marshal", ErrInvalid)
	}
	return a.Bytes(), nil
}

// UnmarshalJSON reads an answer as it travels and seals it again, so an answer
// decoded on the client, or read back from storage, has the bytes and the hash the
// server's had. What it reads must be an answer as MarshalJSON writes one: a relation
// name, and tuples all of one width, at least one wide, sorted with no repeats. Any
// other shape is ErrSyntax, since a hash over it would pin something no evaluation
// answers.
func (a *Answer) UnmarshalJSON(b []byte) error {
	var p struct {
		Relation string  `json:"relation"`
		Tuples   []Tuple `json:"tuples"`
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return err
	}
	if !isName(p.Relation) {
		return fmt.Errorf("%w: not an answer: relation %q is not a relation name", ErrSyntax, p.Relation)
	}
	if p.Tuples == nil {
		return fmt.Errorf("%w: not an answer: no tuples", ErrSyntax)
	}
	for i, t := range p.Tuples {
		switch {
		case len(t) == 0:
			return fmt.Errorf("%w: not an answer: tuple %d is empty", ErrSyntax, i)
		case len(t) != len(p.Tuples[0]):
			return fmt.Errorf("%w: not an answer: tuple %d has %d constants, tuple 0 has %d", ErrSyntax, i, len(t), len(p.Tuples[0]))
		case i > 0 && !tupleLess(p.Tuples[i-1], t):
			return fmt.Errorf("%w: not an answer: tuple %d is not after tuple %d; tuples are sorted with no repeats", ErrSyntax, i, i-1)
		}
	}
	s, err := sealed(p.Relation, p.Tuples)
	if err != nil {
		return err
	}
	*a = s
	return nil
}
