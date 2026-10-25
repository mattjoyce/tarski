package tarski

import (
	"fmt"
	"unicode/utf8"
)

// This file holds the structural check of a Program. The parser produces only
// well-formed programs, but Program, Rule and Literal are exported and a caller can
// build or edit one by hand; Compile checks the shape before it traverses anything, so
// a malformed value is an ErrInvalid with a reason and never a nil dereference.

// checkShape refuses a program set whose values are not a program: a declaration or
// an atom whose relation name is not one the lexer would read, or with an arity
// below one; a variable that is not a variable name; a string constant that is not
// valid UTF-8 (the engine's string domain, see String); a literal that is not exactly
// one of an atom, a comparison and an aggregate; a negated comparison or aggregate; a
// comparison with an operator the language does not have; or an aggregate that is not
// one (see checkAggregateShape). A zero Term is the empty string constant and
// is fine. Every value the parser makes passes; the check is for a program built or
// edited by hand.
func checkShape(programs []Program) error {
	for i, p := range programs {
		who := programName(p, i)
		for _, name := range sortedKeys(p.Declared) {
			if name == "" {
				return fmt.Errorf("%w: %s declares a relation with no name", ErrInvalid, who)
			}
			if !isName(name) {
				return fmt.Errorf("%w: %s declares %q, which is not a relation name", ErrInvalid, who, name)
			}
			if n := p.Declared[name]; n < 1 {
				return fmt.Errorf("%w: %s declares %s/%d; a relation needs at least one argument", ErrInvalid, who, name, n)
			}
		}
		for j, r := range p.Rules {
			if err := checkRuleShape(r); err != nil {
				return fmt.Errorf("%w: %s, rule %d (line %d): %w", ErrInvalid, who, j+1, r.Line, err)
			}
		}
	}
	return nil
}

// programName is how an error names the i-th program passed to Compile: its rule set,
// or its position when it has none.
func programName(p Program, i int) string {
	if p.Name != "" {
		return p.Name
	}
	return fmt.Sprintf("program %d", i+1)
}

func checkRuleShape(r Rule) error {
	if err := checkAtomShape(r.Head); err != nil {
		return fmt.Errorf("the head: %w", err)
	}
	for i, l := range r.Body {
		if err := checkLiteralShape(l); err != nil {
			return fmt.Errorf("body literal %d: %w", i+1, err)
		}
	}
	return nil
}

func checkLiteralShape(l Literal) error {
	switch {
	case l.Atom == nil && l.Compare == nil && l.Aggregate == nil:
		return fmt.Errorf("it is neither an atom nor a comparison nor an aggregate")
	case l.Atom != nil && l.Compare != nil:
		return fmt.Errorf("it is both an atom and a comparison")
	case l.Atom != nil && l.Aggregate != nil:
		return fmt.Errorf("it is both an atom and an aggregate")
	case l.Compare != nil && l.Aggregate != nil:
		return fmt.Errorf("it is both a comparison and an aggregate")
	case l.Aggregate != nil && l.Negated:
		return fmt.Errorf("an aggregate cannot be negated; compare its result instead")
	case l.Aggregate != nil:
		return checkAggregateShape(*l.Aggregate)
	case l.Compare != nil && l.Negated:
		return fmt.Errorf("a comparison cannot be negated; use the opposite operator")
	case l.Atom != nil:
		return checkAtomShape(*l.Atom)
	case !comparisonOps[l.Compare.Op]:
		return fmt.Errorf("comparison operator %q is not one of =, !=, <, <=, >, >=", l.Compare.Op)
	}
	for _, t := range []Term{l.Compare.Left, l.Compare.Right} {
		if err := checkTermShape(t); err != nil {
			return err
		}
	}
	return nil
}

func checkAtomShape(a Atom) error {
	if a.Name == "" {
		return fmt.Errorf("the atom has no relation name")
	}
	if !isName(a.Name) {
		return fmt.Errorf("%q is not a relation name", a.Name)
	}
	if len(a.Args) == 0 {
		return fmt.Errorf("%s has no arguments; a relation needs at least one", a.Name)
	}
	for i, t := range a.Args {
		if err := checkTermShape(t); err != nil {
			return fmt.Errorf("%s, argument %d: %w", a.Name, i+1, err)
		}
	}
	return nil
}

func checkTermShape(t Term) error {
	switch {
	case t.IsVar() && !isVarName(t.Var):
		return fmt.Errorf("%q is not a variable name", t.Var)
	case !t.IsVar() && t.Const.kind == KindString && !utf8.ValidString(t.Const.s):
		return fmt.Errorf("a string constant is not valid UTF-8")
	}
	return nil
}
