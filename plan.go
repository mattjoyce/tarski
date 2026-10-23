package datalog

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// ErrInvalid is returned for a program or a fact that is well formed but cannot be
// used: an unknown relation, a wrong arity, an unsafe variable, a negation that
// cannot be stratified, a rule over a base relation, a built-in where a relation
// belongs, or an atom its relation's check refuses. Each message says what to change.
var ErrInvalid = errors.New("datalog: invalid program")

// Plan is a validated, stratified program over a schema, ready to evaluate.
type Plan struct {
	strata  [][]rule
	idb     map[string]bool
	defined map[string]bool // the derived relations some rule has as its head
	base    map[string]int  // the schema's relations and arities, which a Base must match
	arity   map[string]int  // every relation, base and derived
}

// Schema names the base relations a base may hold and, per relation, an optional
// compile-time check run on every atom (head or body) that names it. A check sees one
// atom, so it can hold a constant to a shape; reachability is not its business.
type Schema struct {
	Arities map[string]int
	Checks  map[string]func(Atom) error // nil map or nil entry: no check
}

// rule is a Rule with its body in evaluation order: positive atoms as written, each
// followed by the negations and comparisons it binds, and with the positions of the
// positive atoms whose relation the rule's own stratum derives.
type rule struct {
	Rule
	ordered   []Literal
	recursive []int
}

// Compile validates programs against a schema and groups their rules into strata.
// The programs are read as one: a later program defines what an earlier one declared,
// which is how a consumer's rules build on a bundled rule set. Programs are copied, and
// a malformed one (see checkShape) is ErrInvalid. A relation declared and
// defined nowhere is empty, not an error: that is how a rule set leaves a hook open.
func Compile(schema Schema, programs ...Program) (Plan, error) {
	// The plan owns what it compiles: it works from copies, so the caller editing a
	// Program afterwards changes nothing the plan derives. Shape is checked on the copy
	// before anything walks it.
	owned := make([]Program, len(programs))
	for i, p := range programs {
		owned[i] = p.Clone()
	}
	programs = owned
	if err := checkShape(programs); err != nil {
		return Plan{}, err
	}
	if err := checkSchema(schema); err != nil {
		return Plan{}, err
	}
	arities, idb, err := relations(schema, programs)
	if err != nil {
		return Plan{}, err
	}
	defined := map[string]bool{}
	for _, p := range programs {
		for _, r := range p.Rules {
			defined[r.Head.Name] = true
		}
	}
	var rules []Rule
	for _, p := range programs {
		for _, r := range p.Rules {
			if err := checkRule(r, arities, schema); err != nil {
				return Plan{}, err
			}
			if p.Name != "" {
				r.Set = p.Name // r is the plan's own copy: the caller's Program is not tagged
			}
			rules = append(rules, r)
		}
	}
	strata, err := stratify(rules, idb)
	if err != nil {
		return Plan{}, err
	}
	base := make(map[string]int, len(schema.Arities))
	for name, n := range schema.Arities {
		base[name] = n
	}
	return Plan{strata: strata, idb: idb, defined: defined, base: base, arity: arities}, nil
}

// Arity is the arity of a relation the plan knows, base or derived. ok is false for a
// name it does not: a consumer uses it to check that a rule set defines the relations
// the consumer will read, with the arity it will read them at.
func (p Plan) Arity(name string) (int, bool) {
	n, ok := p.arity[name]
	return n, ok
}

// Defines reports whether some rule has name as its head. A relation only declared
// has an arity but no rule, and is empty: a consumer that reads a relation checks
// both, so a rule set that declares what it was meant to define is refused, not
// silently empty.
func (p Plan) Defines(name string) bool { return p.defined[name] }

// checkSchema refuses a schema that could not hold a relation: a name the lexer
// would not read as one, a built-in's name, a non-positive arity, or a check for a
// relation the schema does not have.
func checkSchema(schema Schema) error {
	for name, n := range schema.Arities {
		if !isName(name) {
			return fmt.Errorf("%w: %q is not a relation name", ErrInvalid, name)
		}
		if _, ok := builtins[name]; ok {
			return fmt.Errorf("%w: %s is a built-in and cannot be a base relation", ErrInvalid, name)
		}
		if n < 1 {
			return fmt.Errorf("%w: %s/%d: a relation needs at least one argument", ErrInvalid, name, n)
		}
	}
	for name := range schema.Checks {
		if _, ok := schema.Arities[name]; !ok {
			return fmt.Errorf("%w: a check for %s, which the schema does not have", ErrInvalid, name)
		}
	}
	return nil
}

// relations collects every relation's arity: the schema's base relations, the
// declarations, and the heads. One name, one arity.
func relations(schema Schema, programs []Program) (arities map[string]int, idb map[string]bool, err error) {
	arities, idb = map[string]int{}, map[string]bool{}
	for name, n := range schema.Arities {
		arities[name] = n
	}
	note := func(name string, n, line int) error {
		if have, ok := arities[name]; ok && have != n {
			return fmt.Errorf("%w: line %d: %s/%d, but %s is %s/%d", ErrInvalid, line, name, n, name, name, have)
		}
		arities[name] = n
		return nil
	}
	for _, p := range programs {
		for name, n := range p.Declared {
			if _, ok := builtins[name]; ok {
				return nil, nil, fmt.Errorf("%w: %s is a built-in and cannot be declared", ErrInvalid, name)
			}
			if schema.Arities[name] != 0 {
				return nil, nil, fmt.Errorf("%w: %s/%d is a base relation and cannot be declared", ErrInvalid, name, n)
			}
			if err := note(name, n, 0); err != nil {
				return nil, nil, err
			}
			idb[name] = true
		}
		for _, r := range p.Rules {
			if _, ok := builtins[r.Head.Name]; ok {
				return nil, nil, fmt.Errorf("%w: line %d: %s is a built-in and cannot be the head of a rule", ErrInvalid, r.Line, r.Head.Name)
			}
			if schema.Arities[r.Head.Name] != 0 {
				return nil, nil, fmt.Errorf("%w: line %d: %s/%d is a base relation; a rule cannot add to it",
					ErrInvalid, r.Line, r.Head.Name, schema.Arities[r.Head.Name])
			}
			if err := note(r.Head.Name, len(r.Head.Args), r.Line); err != nil {
				return nil, nil, err
			}
			idb[r.Head.Name] = true
		}
	}
	return arities, idb, nil
}

// checkRule holds one rule to the arities, to the schema's checks, and to safety:
// every variable in the head, in a negated atom, in a comparison or in a built-in is
// bound by a positive body atom. A built-in binds nothing, so it never counts as one.
func checkRule(r Rule, arities map[string]int, schema Schema) error {
	bound := map[string]bool{}
	atoms := []*Atom{&r.Head}
	for _, l := range r.Body {
		if l.Atom == nil {
			continue
		}
		atoms = append(atoms, l.Atom)
		if l.Negated || isBuiltin(l.Atom.Name) {
			continue
		}
		for _, a := range l.Atom.Args {
			if a.IsVar() {
				bound[a.Var] = true
			}
		}
	}
	for _, a := range atoms {
		if err := checkAtom(*a, arities, schema); err != nil {
			return fmt.Errorf("%w: line %d: %w", ErrInvalid, a.Line, err)
		}
	}
	for _, a := range r.Head.Args {
		if a.Var == "_" {
			return fmt.Errorf("%w: line %d: _ in the head of %s; name the variable or use a constant", ErrInvalid, r.Line, r.Head.Name)
		}
		if a.IsVar() && !bound[a.Var] {
			return fmt.Errorf("%w: line %d: %s in the head of %s is not bound by a positive body atom", ErrInvalid, r.Line, a.Var, r.Head.Name)
		}
	}
	for _, l := range r.Body {
		if err := checkGuard(l, bound); err != nil {
			return err
		}
	}
	return nil
}

// checkGuard holds a negation, comparison or built-in to safety. A negated atom may
// use _ for a position it does not care about; a built-in or comparison may not, since
// it has nothing to test there.
func checkGuard(l Literal, bound map[string]bool) error {
	switch {
	case l.Compare != nil:
		for _, t := range []Term{l.Compare.Left, l.Compare.Right} {
			if t.Var == "_" {
				return fmt.Errorf("%w: line %d: _ in a comparison compares nothing", ErrInvalid, l.Compare.Line)
			}
			if t.IsVar() && !bound[t.Var] {
				return fmt.Errorf("%w: line %d: %s in a comparison is not bound by a positive body atom", ErrInvalid, l.Compare.Line, t.Var)
			}
		}
	case l.Atom != nil && isBuiltin(l.Atom.Name):
		for _, t := range l.Atom.Args {
			if t.Var == "_" {
				return fmt.Errorf("%w: line %d: _ in %s tests nothing", ErrInvalid, l.Atom.Line, l.Atom.Name)
			}
			if t.IsVar() && !bound[t.Var] {
				return fmt.Errorf("%w: line %d: %s in %s must be bound by a positive body atom; a built-in binds nothing",
					ErrInvalid, l.Atom.Line, t.Var, l.Atom.Name)
			}
		}
	case l.Negated:
		for _, a := range l.Atom.Args {
			if a.IsVar() && a.Var != "_" && !bound[a.Var] {
				return fmt.Errorf("%w: line %d: %s in not %s is not bound by a positive body atom", ErrInvalid, l.Atom.Line, a.Var, l.Atom.Name)
			}
		}
	}
	return nil
}

// checkAtom holds an atom to its relation's arity, then to the schema's check for it.
// A built-in has its own arity and no check.
func checkAtom(a Atom, arities map[string]int, schema Schema) error {
	if b, ok := builtins[a.Name]; ok {
		if b.arity != len(a.Args) {
			return fmt.Errorf("%s has %d arguments, but %s is %s/%d", a.Name, len(a.Args), a.Name, a.Name, b.arity)
		}
		return nil
	}
	n, ok := arities[a.Name]
	if !ok {
		return fmt.Errorf("unknown relation %s/%d; %s", a.Name, len(a.Args), known(arities))
	}
	if n != len(a.Args) {
		return fmt.Errorf("%s has %d arguments, but %s is %s/%d", a.Name, len(a.Args), a.Name, a.Name, n)
	}
	if check := schema.Checks[a.Name]; check != nil {
		return check(Atom{Name: a.Name, Args: slices.Clone(a.Args), Line: a.Line})
	}
	return nil
}

// known says which relations a schema has, as the tail of an error sentence.
func known(arities map[string]int) string {
	if len(arities) == 0 {
		return "the schema has no relations"
	}
	names := make([]string, 0, len(arities))
	for name, n := range arities {
		names = append(names, fmt.Sprintf("%s/%d", name, n))
	}
	sort.Strings(names)
	return "the relations are " + strings.Join(names, ", ")
}

// accepts refuses a base built under a different schema than the plan was compiled
// over: a relation the plan reads that the base lacks, or holds at another arity,
// would otherwise be read as empty or panic mid-join; a relation the plan derives that
// the base also holds would have its facts silently dropped.
func (p Plan) accepts(b Base) error {
	for _, name := range sortedKeys(b.relations) {
		if p.idb[name] {
			return fmt.Errorf("%w: the base holds %s, which the plan derives; a base relation cannot also be derived", ErrInvalid, name)
		}
	}
	for name, n := range p.base {
		r, ok := b.relations[name]
		if !ok || r.arity != n {
			return fmt.Errorf("%w: the base has no %s/%d; build it under the schema the plan was compiled with", ErrInvalid, name, n)
		}
	}
	return nil
}

// sortedKeys is the keys of m in order, so an error that names the first offender names
// the same one every time.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
