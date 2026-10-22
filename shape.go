package datalog

import "fmt"

// This file holds the structural check of a Program. The parser produces only
// well-formed programs, but Program, Rule and Literal are exported and a caller can
// build or edit one by hand; Compile checks the shape before it traverses anything, so
// a malformed value is an ErrInvalid with a reason and never a nil dereference.

// checkShape refuses a program set whose values are not a program: a declaration with
// no name or an arity below one, a rule whose head or body atom has no relation name,
// a literal that is neither an atom nor a comparison or is both, a negated comparison,
// or a comparison with an operator the language does not have. A zero Term is the empty
// string constant and is fine.
func checkShape(programs []Program) error {
	for i, p := range programs {
		who := p.Name
		if who == "" {
			who = fmt.Sprintf("program %d", i+1)
		}
		for _, name := range sortedKeys(p.Declared) {
			if name == "" {
				return fmt.Errorf("%w: %s declares a relation with no name", ErrInvalid, who)
			}
			if n := p.Declared[name]; n < 1 {
				return fmt.Errorf("%w: %s declares %s/%d; a relation needs at least one argument", ErrInvalid, who, name, n)
			}
		}
		for j, r := range p.Rules {
			if err := checkRuleShape(r); err != nil {
				return fmt.Errorf("%w: %s, rule %d (line %d): %v", ErrInvalid, who, j+1, r.Line, err)
			}
		}
	}
	return nil
}

func checkRuleShape(r Rule) error {
	if r.Head.Name == "" {
		return fmt.Errorf("the head has no relation name")
	}
	for i, l := range r.Body {
		if err := checkLiteralShape(l); err != nil {
			return fmt.Errorf("body literal %d: %v", i+1, err)
		}
	}
	return nil
}

func checkLiteralShape(l Literal) error {
	switch {
	case l.Atom == nil && l.Compare == nil:
		return fmt.Errorf("it is neither an atom nor a comparison")
	case l.Atom != nil && l.Compare != nil:
		return fmt.Errorf("it is both an atom and a comparison")
	case l.Compare != nil && l.Negated:
		return fmt.Errorf("a comparison cannot be negated; use the opposite operator")
	case l.Atom != nil && l.Atom.Name == "":
		return fmt.Errorf("the atom has no relation name")
	case l.Compare != nil && !comparisonOps[l.Compare.Op]:
		return fmt.Errorf("comparison operator %q is not one of =, !=, <, <=, >, >=", l.Compare.Op)
	}
	return nil
}
