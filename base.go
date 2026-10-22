package datalog

import "fmt"

// Fact is one tuple of a base relation.
type Fact struct {
	Relation string
	Tuple    Tuple
}

// Base is the extensional database: the base relations of a schema, holding facts. It
// is immutable once built and safe for concurrent Evaluate calls. Every relation is
// frozen with all its indexes at construction, so evaluation only reads it.
type Base struct {
	arities   map[string]int
	checks    map[string]func(Atom) error
	relations map[string]*relation
}

// NewBase holds facts under schema. A fact of a relation the schema lacks, of the
// wrong arity, or that its relation's check refuses (the check reads the fact as an
// atom of constants) is ErrInvalid, as is a schema that names a built-in or an arity
// under one. Every relation in schema.Arities exists, empty or not. Duplicate facts count
// once. The tuples are copied, so the caller may reuse its slices.
func NewBase(schema Schema, facts ...Fact) (Base, error) {
	if err := checkSchema(schema); err != nil {
		return Base{}, err
	}
	arities := make(map[string]int, len(schema.Arities))
	for name, n := range schema.Arities {
		arities[name] = n
	}
	checks := make(map[string]func(Atom) error, len(schema.Checks))
	for name, check := range schema.Checks {
		checks[name] = check
	}
	return Base{arities: arities, checks: checks, relations: map[string]*relation{}}.With(facts...)
}

// With is the standing base plus more facts, as a new value; b is unchanged. Relations
// the new facts do not touch are shared, not copied. A relation they do touch is
// rebuilt from the standing tuples plus the new ones, and frozen again. Validation is
// NewBase's.
func (b Base) With(facts ...Fact) (Base, error) {
	added, err := b.group(facts)
	if err != nil {
		return Base{}, err
	}
	out := Base{arities: b.arities, checks: b.checks, relations: make(map[string]*relation, len(b.arities))}
	for name, n := range b.arities {
		prior := b.relations[name]
		if prior != nil && len(added[name]) == 0 {
			out.relations[name] = prior
			continue
		}
		out.relations[name] = rebuild(prior, added[name], n)
	}
	return out, nil
}

// group sorts facts by relation, refusing any the schema does not allow.
func (b Base) group(facts []Fact) (map[string][]Tuple, error) {
	added := map[string][]Tuple{}
	for _, f := range facts {
		n, ok := b.arities[f.Relation]
		if !ok {
			return nil, fmt.Errorf("%w: a fact of %s, which the schema does not have; the relations are %s",
				ErrInvalid, f.Relation, known(b.arities))
		}
		if len(f.Tuple) != n {
			return nil, fmt.Errorf("%w: a fact of %s has %d arguments, but %s is %s/%d",
				ErrInvalid, f.Relation, len(f.Tuple), f.Relation, f.Relation, n)
		}
		if err := b.check(f); err != nil {
			return nil, err
		}
		added[f.Relation] = append(added[f.Relation], append(Tuple(nil), f.Tuple...))
	}
	return added, nil
}

// rebuild is a frozen relation of prior's tuples (if any) then added, in that order.
func rebuild(prior *relation, added []Tuple, arity int) *relation {
	r := newRelation()
	if prior != nil {
		for _, t := range prior.tuples {
			r.add(t)
		}
	}
	for _, t := range added {
		r.add(t)
	}
	r.freeze(arity)
	return r
}

// Len is how many tuples the base holds for relation, zero for a name it does not
// have.
func (b Base) Len(relation string) int {
	if r, ok := b.relations[relation]; ok {
		return len(r.tuples)
	}
	return 0
}

// check runs the schema's check for the fact's relation, if it has one, on the fact
// read as an atom of constants. The schema's word then holds on every tuple that
// enters, as it does on every atom that names the relation at Compile.
func (b Base) check(f Fact) error {
	check := b.checks[f.Relation]
	if check == nil {
		return nil
	}
	args := make([]Term, len(f.Tuple))
	for i, c := range f.Tuple {
		args[i] = Term{Const: c}
	}
	if err := check(Atom{Name: f.Relation, Args: args}); err != nil {
		return fmt.Errorf("%w: a fact of %s: %v", ErrInvalid, f.Relation, err)
	}
	return nil
}
