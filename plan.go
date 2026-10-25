package tarski

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
)

// ErrInvalid is returned for a program or a fact that is well formed but cannot be
// used: an unknown relation, a wrong arity, an unsafe variable, a negation that
// cannot be stratified, a rule over a base relation, a built-in where a relation
// belongs, an atom its relation's check refuses, a builtin declaration the engine
// cannot use, an aggregate that is unsafe or reads a relation that depends on its own
// rule, or a consumer builtin that panicked during evaluation. Each message says what
// to change.
var ErrInvalid = errors.New("tarski: invalid program")

// Plan is a validated, stratified program over a schema, ready to evaluate.
type Plan struct {
	strata   [][]rule
	idb      map[string]bool
	defined  map[string]bool // the derived relations some rule has as its head
	base     map[string]int  // the schema's relations and arities, which a Base must match
	arity    map[string]int  // every relation, base and derived
	builtins builtinTable    // engine and consumer builtins, copied at Compile
}

// Schema names the base relations a base may hold and, per relation, an optional
// compile-time check run on every atom (head or body) that names it. A check sees one
// atom, so it can hold a constant to a shape; reachability is not its business.
// Builtins declares the consumer's own guards beside the engine's (see Builtin).
type Schema struct {
	Arities  map[string]int
	Checks   map[string]func(Atom) error // nil map or nil entry: no check
	Builtins map[string]Builtin          // nil map: the engine's builtins only
}

// rule is a Rule with its body in evaluation order: positive atoms as written, each
// followed by the negations and comparisons it binds, and with the positions of the
// positive atoms whose relation the rule's own stratum derives. aggs is indexed like
// ordered and holds each aggregate literal's evaluation plan; it is nil for a rule with
// no aggregate, which is every rule a v0.1 program has.
type rule struct {
	Rule
	ordered   []Literal
	recursive []int
	aggs      []*aggPlan
}

// Compile validates programs against a schema and groups their rules into strata.
// The programs are read as one: a later program defines what an earlier one declared,
// which is how a consumer's rules build on a bundled rule set. Programs are copied, and
// a malformed one (see checkShape) is ErrInvalid. A relation declared and
// defined nowhere is empty, not an error: that is how a rule set leaves a hook open.
// Schema maps are copied before any check runs; check functions still own their
// closure state and must provide stable results for stable inputs. The builtin table
// is copied too, so changing Schema.Builtins after Compile changes nothing the plan does.
func Compile(schema Schema, programs ...Program) (Plan, error) {
	schema = Schema{Arities: maps.Clone(schema.Arities), Checks: maps.Clone(schema.Checks), Builtins: maps.Clone(schema.Builtins)}
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
	table := newBuiltinTable(schema.Builtins)
	arities, idb, err := relations(schema, table, programs)
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
			if err := checkRule(r, arities, schema, table); err != nil {
				return Plan{}, err
			}
			if p.Name != "" {
				r.Set = p.Name // r is the plan's own copy: the caller's Program is not tagged
			}
			rules = append(rules, r)
		}
	}
	strata, err := stratify(rules, idb, table)
	if err != nil {
		return Plan{}, err
	}
	base := make(map[string]int, len(schema.Arities))
	for name, n := range schema.Arities {
		base[name] = n
	}
	return Plan{strata: strata, idb: idb, defined: defined, base: base, arity: arities, builtins: table}, nil
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
// would not read as one, a built-in's name, a non-positive arity, a check for a
// relation the schema does not have, or a builtin declaration checkBuiltins refuses.
func checkSchema(schema Schema) error {
	for name, n := range schema.Arities {
		if !isName(name) {
			return fmt.Errorf("%w: %q is not a relation name", ErrInvalid, name)
		}
		if _, ok := engineBuiltins[name]; ok {
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
	return checkBuiltins(schema)
}

// relations collects every relation's arity: the schema's base relations, the
// declarations, and the heads. One name, one arity.
func relations(schema Schema, table builtinTable, programs []Program) (arities map[string]int, idb map[string]bool, err error) {
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
			if table.has(name) {
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
			if table.has(r.Head.Name) {
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
// bound by a positive body atom or is an aggregate's result. A built-in binds nothing,
// so it never counts as one. An aggregate's own body is held to the same rules inside
// its braces (checkAggregate), and the atoms there to the arities and checks.
func checkRule(r Rule, arities map[string]int, schema Schema, table builtinTable) error {
	positive := map[string]bool{}
	atoms := []*Atom{&r.Head}
	for _, l := range r.Body {
		if l.Aggregate != nil {
			atoms = appendInnerAtoms(atoms, l.Aggregate)
			continue
		}
		if l.Atom == nil {
			continue
		}
		atoms = append(atoms, l.Atom)
		if !l.Negated && !table.has(l.Atom.Name) {
			bindVars(positive, l.Atom.Args)
		}
	}
	for _, a := range atoms {
		if err := checkAtom(*a, arities, schema, table); err != nil {
			return fmt.Errorf("%w: line %d: %w", ErrInvalid, a.Line, err)
		}
	}
	bound := withResults(positive, r.Body)
	if err := checkHead(r, bound); err != nil {
		return err
	}
	for i := range r.Body {
		if err := checkAggregate(r, i, positive, table); err != nil {
			return err
		}
	}
	for _, l := range r.Body {
		if err := checkGuard(l, bound, table); err != nil {
			return err
		}
	}
	return nil
}

// checkGuard holds a negation, comparison or built-in to safety. A negated atom may
// use _ for a position it does not care about; a built-in or comparison may not, since
// it has nothing to test there.
func checkGuard(l Literal, bound map[string]bool, table builtinTable) error {
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
	case l.Atom != nil && table.has(l.Atom.Name):
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
// A built-in has its own arity and, if it declares one, its own Check.
func checkAtom(a Atom, arities map[string]int, schema Schema, table builtinTable) error {
	if b, ok := table[a.Name]; ok {
		return checkBuiltinAtom(a, b)
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
