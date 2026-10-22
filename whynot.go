package datalog

import (
	"maps"
	"slices"
	"strings"
)

// The limits on a why-not walk. They bound the explanation, never the evaluation.
const (
	// whyNotDepth is how many levels of Because an Absence nests when the caller asks
	// for the default: enough for any policy rule set, small enough that a pathological
	// recursion stays cheap.
	whyNotDepth = 8
	// whyNotSteps is how many body-literal candidates a WhyNot call may try in all.
	// Each matched literal branches, so a wide base would otherwise make an
	// explanation cost more than the evaluation it explains; 1<<16 is a few
	// milliseconds of work and far more than a request-sized policy needs.
	whyNotSteps = 1 << 16
)

// Absence is why a tuple was not derived. For a goal not in the result, every rule
// whose head unifies with it is walked, body in evaluation order, against the result's
// relations; each rule reports the farthest literal that found no match and the
// bindings it had there. A positive literal that found nothing
// is explained in turn, to a bounded depth. A negation that held reports the proof of
// the tuple that blocked it. A goal over a base relation has no rules: it is simply
// "no such fact". Options.Proofs must have been on. The walk is deterministic for one
// (plan, base, goal): rules in plan order, matches in relation order.
type Absence struct {
	Relation string
	// Goal has a constant where the goal fixed a position and a variable where the
	// caller left it open.
	Goal []Term
	// Rules has one entry per rule whose head unifies with the goal; empty for a base
	// relation.
	Rules []RuleFailure
	// Truncated is set when the walk hit the depth or the step limit. What is here is
	// still true; there is more it did not look at.
	Truncated bool
}

// RuleFailure is why one rule did not derive the goal.
type RuleFailure struct {
	// Rule is a copy, as in Proof.
	Rule *Rule
	// Bound holds the variables bound when the walk stopped, on the first path that
	// reached the failing literal.
	Bound map[string]Const
	// Failed is the farthest literal in evaluation order with no match; for a negation
	// it is the one that held, since that is what stopped the rule. A zero Literal (no
	// Atom, no Compare) means the walk found no failing literal: the body matched in
	// full, which cannot happen against the result the rule ran over, or the step limit
	// ended the walk first (Absence.Truncated tells which).
	Failed Literal
	// Because explains why Failed found nothing, when it is a positive atom: over a
	// derived relation, the rules that did not derive it; over a base relation, an
	// Absence with no rules, "no such fact". A goal already explained on the way down, or elsewhere in the
	// same Absence, is not explained again: the first explanation stands, so a
	// recursion through the same relation ends without being called truncation.
	Because []Absence
	// Blocked is the proof of the tuple that blocked Failed, when Failed is a negation
	// that held.
	Blocked *Proof
}

// WhyNot explains a tuple's absence. ok is false when t is in the result (ask Proof),
// when the relation or the tuple's arity is not the plan's, or when the evaluation ran
// without Options.Proofs. maxDepth bounds the recursion into Because; 0 means the
// engine's default.
func (r Result) WhyNot(name string, t Tuple, maxDepth int) (a Absence, ok bool) {
	rel, found := r.relations[name]
	arity, known := r.plan.Arity(name)
	if r.proofs == nil || !found || !known || len(t) != arity || rel.has(t) {
		return Absence{}, false
	}
	if maxDepth <= 0 {
		maxDepth = whyNotDepth
	}
	goal := make([]Term, len(t))
	for i, c := range t {
		goal[i] = Term{Const: c}
	}
	x := &explainer{r: r, b: &builder{r: r, proofs: map[string]Proof{}, rules: map[*Rule]*Rule{}}, seen: map[string]bool{}}
	x.seen[goalKey(name, goal)] = true
	return x.absence(name, goal, maxDepth), true
}

// explainer is one WhyNot call's state: the rule copies shared with Proof's builder,
// the goals already explained, and the steps spent.
type explainer struct {
	r         Result
	b         *builder
	seen      map[string]bool
	steps     int
	exhausted bool
}

// spend charges one candidate against the step limit and reports whether it is gone.
func (x *explainer) spend() bool {
	if x.steps++; x.steps > whyNotSteps {
		x.exhausted = true
	}
	return x.exhausted
}

// absence explains why goal, over relation name, is not in the result. depth is how
// many more levels of Because it may open.
func (x *explainer) absence(name string, goal []Term, depth int) Absence {
	a := Absence{Relation: name, Goal: slices.Clone(goal), Rules: []RuleFailure{}}
	for _, ru := range x.rulesFor(name) {
		rf, cut, ok := x.failure(ru, goal, depth)
		if !ok {
			continue
		}
		a.Rules = append(a.Rules, rf)
		a.Truncated = a.Truncated || cut
		if x.exhausted {
			break
		}
	}
	a.Truncated = a.Truncated || x.exhausted
	return a
}

// rulesFor is the plan's rules whose head is name, in plan order.
func (x *explainer) rulesFor(name string) []*rule {
	var out []*rule
	for _, stratum := range x.r.plan.strata {
		for i := range stratum {
			if stratum[i].Head.Name == name {
				out = append(out, &stratum[i])
			}
		}
	}
	return out
}

// failure walks one rule against the goal. ok is false when the rule's head cannot
// unify with the goal, so the rule is no candidate. cut reports that the explanation
// was cut short for want of depth or steps.
func (x *explainer) failure(ru *rule, goal []Term, depth int) (rf RuleFailure, cut, ok bool) {
	env, ok := unifyHead(ru.Head.Args, goal)
	if !ok {
		return RuleFailure{}, false, false
	}
	w := &walker{x: x, r: ru}
	w.walk(0, env)
	rf = RuleFailure{Rule: x.b.rule(&ru.Rule), Bound: maps.Clone(env)}
	if !w.found {
		return rf, false, true
	}
	rf.Bound, rf.Failed = w.best.env, copyLiteral(w.best.lit)
	switch {
	case w.best.blocked != nil:
		p := x.b.build(w.best.lit.Atom.Name, w.best.blocked)
		rf.Blocked = &p
	case w.best.lit.Atom != nil && !w.best.lit.Negated && !isBuiltin(w.best.lit.Atom.Name):
		cut = x.because(&rf, w.best.lit.Atom, w.best.env, depth)
	}
	return rf, cut, true
}

// because explains a failed positive atom, derived or base, with the bindings
// the walk had. It reports whether depth stopped it.
func (x *explainer) because(rf *RuleFailure, atom *Atom, env map[string]Const, depth int) (cut bool) {
	sub := substitute(atom.Args, env)
	key := goalKey(atom.Name, sub)
	if x.seen[key] {
		return false
	}
	if depth <= 0 {
		return true
	}
	x.seen[key] = true
	subAbsence := x.absence(atom.Name, sub, depth-1)
	rf.Because = []Absence{subAbsence}
	return subAbsence.Truncated
}

// miss is the farthest failure a walk has seen: where, with what bindings, and for a
// negation that held, the tuple that held it.
type miss struct {
	idx     int
	env     map[string]Const
	lit     Literal
	blocked Tuple
}

// walker is a depth-first walk over one rule's ordered body.
type walker struct {
	x     *explainer
	r     *rule
	best  miss
	found bool // some literal found no match
	full  bool // the body matched in full
}

// walk extends env through the body from literal i on. It returns false to stop the
// whole walk (a full match, or the step limit).
func (w *walker) walk(i int, env map[string]Const) bool {
	if w.x.exhausted || w.full {
		return false
	}
	if i == len(w.r.ordered) {
		w.full = true
		return false
	}
	l := w.r.ordered[i]
	switch {
	case l.Compare != nil || isBuiltin(l.Atom.Name):
		if guardHolds(l, env) {
			return w.walk(i+1, env)
		}
		w.record(i, env, l, nil)
		return true
	case l.Negated:
		t, held, stop := w.probe(w.x.r.relations[l.Atom.Name], l.Atom.Args, env)
		if stop {
			return false
		}
		if held {
			w.record(i, env, l, t)
			return true
		}
		return w.walk(i+1, env)
	}
	return w.positive(i, l, env)
}

// positive tries each tuple of a positive atom's relation, in relation order. A literal
// that matches nothing under env is a failure; one that matches but fails further on is
// the later literal's failure.
func (w *walker) positive(i int, l Literal, env map[string]Const) bool {
	rel := w.x.r.relations[l.Atom.Name]
	matched := false
	for _, idx := range candidates(rel, l.Atom.Args, env) {
		if w.x.spend() {
			return false
		}
		bound, ok := bind(rel.tuples[idx], l.Atom.Args, env)
		if !ok {
			continue
		}
		matched = true
		more := w.walk(i+1, env)
		for _, v := range bound {
			delete(env, v)
		}
		if !more {
			return false
		}
	}
	if !matched {
		w.record(i, env, l, nil)
	}
	return true
}

// probe finds the first tuple of rel that matches args under env, for a negation.
func (w *walker) probe(rel *relation, args []Term, env map[string]Const) (t Tuple, held, stop bool) {
	for _, idx := range candidates(rel, args, env) {
		if w.x.spend() {
			return nil, false, true
		}
		if bound, ok := bind(rel.tuples[idx], args, env); ok {
			for _, v := range bound {
				delete(env, v)
			}
			return rel.tuples[idx], true, false
		}
	}
	return nil, false, false
}

// record keeps a failure if it is farther than any before it. The first path to reach
// an index keeps it.
func (w *walker) record(i int, env map[string]Const, l Literal, blocked Tuple) {
	if w.found && i <= w.best.idx {
		return
	}
	w.found = true
	w.best = miss{idx: i, env: maps.Clone(env), lit: l, blocked: blocked}
}

// candidates are the tuples of rel worth trying. A frozen base relation has every
// index built and is only read; a derived relation's index is built lazily by writing
// to it, which a shared Result must not allow, so it is scanned whole.
func candidates(rel *relation, args []Term, env map[string]Const) []int {
	if rel.frozen {
		return matching(rel, args, env)
	}
	all := make([]int, len(rel.tuples))
	for i := range all {
		all[i] = i
	}
	return all
}

// unifyHead binds the rule's head variables to the goal's constants. ok is false when
// the head and goal cannot be one atom: two different constants, or a variable the
// goal would bind two ways.
func unifyHead(head, goal []Term) (map[string]Const, bool) {
	env := map[string]Const{}
	for i, h := range head {
		g := goal[i]
		switch {
		case !h.IsVar():
			if !g.IsVar() && !g.Const.same(h.Const) {
				return nil, false
			}
		case g.IsVar() || h.Var == "_":
			// an open position binds nothing
		default:
			if c, bound := env[h.Var]; bound && !c.same(g.Const) {
				return nil, false
			}
			env[h.Var] = g.Const
		}
	}
	return env, true
}

// substitute is args with the variables env binds replaced by their constants. A
// variable env does not bind stays a variable: the goal leaves that position open.
func substitute(args []Term, env map[string]Const) []Term {
	out := make([]Term, len(args))
	for i, a := range args {
		out[i] = a
		if a.IsVar() && a.Var != "_" {
			if c, ok := env[a.Var]; ok {
				out[i] = Term{Const: c}
			}
		}
	}
	return out
}

// goalKey identifies a goal by relation and constants. Open positions are all alike,
// whatever the variable is called.
func goalKey(name string, goal []Term) string {
	var b strings.Builder
	b.WriteString(name)
	for _, t := range goal {
		b.WriteByte(0)
		if t.IsVar() {
			b.WriteByte('?')
		} else {
			t.Const.writeKey(&b)
		}
	}
	return b.String()
}
