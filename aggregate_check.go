package tarski

import (
	"fmt"
	"slices"
)

// This file holds what Compile checks of an aggregate and which of its variables group
// (docs/design/aggregates.md §1.1, §3). A variable inside an aggregate's braces is a
// group variable when the rule also uses it outside them: in the head, in another
// literal, or as another aggregate's result. Any other variable there is local to the
// aggregate: each binding of the group variables has one group, the distinct solutions
// of the body over the local variables, with every _ a fresh variable. A variable
// inside a second aggregate's braces is that aggregate's own, so two aggregates in one
// rule are independent unless they share a variable bound outside both.

// bindVars marks every named variable of args as bound.
func bindVars(bound map[string]bool, args []Term) {
	for _, a := range args {
		if a.IsVar() {
			bound[a.Var] = true
		}
	}
}

// withResults is bound plus every aggregate's result in body, as a new map: what the
// head and the outer guards may use.
func withResults(bound map[string]bool, body []Literal) map[string]bool {
	out := make(map[string]bool, len(bound))
	for v := range bound {
		out[v] = true
	}
	for _, l := range body {
		if l.Aggregate != nil {
			out[l.Aggregate.Result.Var] = true
		}
	}
	return out
}

// appendInnerAtoms adds the atoms inside an aggregate's braces, so the arities and the
// schema's checks hold them as they hold any body atom.
func appendInnerAtoms(atoms []*Atom, a *Aggregate) []*Atom {
	for _, l := range a.Body {
		if l.Atom != nil {
			atoms = append(atoms, l.Atom)
		}
	}
	return atoms
}

// checkHead holds the head to safety: no _, and every variable bound. A head variable
// that occurs only inside an aggregate's braces is local to the group, and says so.
func checkHead(r Rule, bound map[string]bool) error {
	for _, a := range r.Head.Args {
		switch {
		case a.Var == "_":
			return fmt.Errorf("%w: line %d: _ in the head of %s; name the variable or use a constant", ErrInvalid, r.Line, r.Head.Name)
		case !a.IsVar() || bound[a.Var]:
			continue
		case insideAggregate(r.Body, a.Var):
			return fmt.Errorf("%w: line %d: %s in the head of %s occurs only inside an aggregate, where it is local to the group; "+
				"bind it by a positive atom in the outer body or aggregate over it", ErrInvalid, r.Line, a.Var, r.Head.Name)
		default:
			return fmt.Errorf("%w: line %d: %s in the head of %s is not bound by a positive body atom", ErrInvalid, r.Line, a.Var, r.Head.Name)
		}
	}
	return nil
}

// insideAggregate reports whether v occurs inside some aggregate's braces in body.
func insideAggregate(body []Literal, v string) bool {
	for _, l := range body {
		if l.Aggregate != nil && innerVars(l.Aggregate)[v] {
			return true
		}
	}
	return false
}

// checkAggregate holds the aggregate at body index i, if it is one, to safety
// (docs/design/aggregates.md §3). positive is the variables the rule's positive atoms
// outside any braces bind. The result must not occur inside; the target must be bound
// by a positive atom inside; every group variable must be bound by a positive atom
// outside; and every guard inside is held to the variables bound inside plus the group
// variables.
func checkAggregate(r Rule, i int, positive map[string]bool, table builtinTable) error {
	a := r.Body[i].Aggregate
	if a == nil {
		return nil
	}
	if innerVars(a)[a.Result.Var] {
		return fmt.Errorf("%w: line %d: %s is the result of %s and cannot appear inside its braces; name the value inside another variable",
			ErrInvalid, a.Line, a.Result.Var, a.Op)
	}
	inside := map[string]bool{}
	for _, l := range a.Body {
		if l.Atom != nil && !l.Negated && !table.has(l.Atom.Name) {
			bindVars(inside, l.Atom.Args)
		}
	}
	if a.Op != "count" && !inside[a.Target.Var] {
		return fmt.Errorf("%w: line %d: %s in %s must be bound by a positive atom inside the braces", ErrInvalid, a.Line, a.Target.Var, a.Op)
	}
	for _, g := range groupVars(r, i) {
		if !positive[g] {
			return fmt.Errorf("%w: line %d: %s is used inside the braces of %s and outside them, so it groups the aggregate; bind it by a "+
				"positive atom outside the braces, or rename it inside to keep it local", ErrInvalid, a.Line, g, a.Op)
		}
		inside[g] = true
	}
	for _, l := range a.Body {
		if err := checkGuard(l, inside, table); err != nil {
			return fmt.Errorf("%w (inside the braces of %s a variable is bound by a positive atom there, or by one outside when it groups)",
				err, a.Op)
		}
	}
	return nil
}

// aggregateGroups is each aggregate literal of r with its group variables, or nil when
// r has none.
func aggregateGroups(r Rule) map[*Aggregate][]string {
	var groups map[*Aggregate][]string
	for i, l := range r.Body {
		if l.Aggregate == nil {
			continue
		}
		if groups == nil {
			groups = map[*Aggregate][]string{}
		}
		groups[l.Aggregate] = groupVars(r, i)
	}
	return groups
}

// groupVars is the group variables of the aggregate at body index i, sorted: the
// variables inside its braces that r also uses outside them.
func groupVars(r Rule, i int) []string {
	outside := map[string]bool{}
	bindVars(outside, r.Head.Args)
	for j, l := range r.Body {
		switch {
		case j == i:
		case l.Aggregate != nil:
			outside[l.Aggregate.Result.Var] = true
		case l.Atom != nil:
			bindVars(outside, l.Atom.Args)
		default:
			bindVars(outside, []Term{l.Compare.Left, l.Compare.Right})
		}
	}
	var out []string
	for v := range innerVars(r.Body[i].Aggregate) {
		if outside[v] {
			out = append(out, v)
		}
	}
	slices.Sort(out)
	return out
}

// innerVars is every named variable inside an aggregate's braces, its target included.
func innerVars(a *Aggregate) map[string]bool {
	vars := map[string]bool{}
	if a.Target.IsVar() {
		vars[a.Target.Var] = true
	}
	for _, l := range a.Body {
		switch {
		case l.Atom != nil:
			bindVars(vars, l.Atom.Args)
		case l.Compare != nil:
			bindVars(vars, []Term{l.Compare.Left, l.Compare.Right})
		}
	}
	delete(vars, "_")
	return vars
}
