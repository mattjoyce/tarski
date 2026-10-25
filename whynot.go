package tarski

import (
	"slices"
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
	// Rules has one entry per rule whose head unifies with the goal. It is nil for a
	// base relation, which has no rules ("no such fact"), and empty but not nil for a
	// derived relation none of whose rules can derive the goal ("no rule's head
	// matches"): the two render apart, since a missing fact and a rule set with no
	// rule for the case are different things to fix.
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
	// it is the one that held, since that is what stopped the rule, and for an
	// aggregate it is one whose group gave no value or a value other than the one
	// bound. A zero Literal (no Atom, no Compare, no Aggregate) means the walk found no
	// failing literal: the body matched in full, which cannot happen against the
	// result the rule ran over, or the step limit ended the walk first
	// (Absence.Truncated is set).
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
	// Value is the aggregate's value for the group, when Failed is an aggregate whose
	// group had one that differed from the result already bound; nil otherwise.
	Value *Const
	// Reason says why Failed, an aggregate, stopped the rule: its value against the
	// one bound, an empty group under min or max, a group mixing kinds or summing a
	// non-number (naming the constant). Empty unless Failed is an aggregate.
	Reason string
}

// WhyNot explains a tuple's absence. ok is false when t is in the result (ask Proof),
// when the relation or the tuple's arity is not the plan's, or when the evaluation ran
// without Options.Proofs, or when a consumer builtin panicked while the rules were
// walked (the panic does not escape). maxDepth bounds the recursion into Because; 0
// means the engine's default.
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
	a = x.absence(name, goal, maxDepth)
	if x.failed != nil {
		return Absence{}, false
	}
	return a, true
}

// explainer is one WhyNot call's state: the rule copies shared with Proof's builder,
// the goals already explained, and the steps spent.
type explainer struct {
	r         Result
	b         *builder
	seen      map[string]bool
	steps     int
	exhausted bool
	failed    error      // a builtin panicked; the walk stops and WhyNot explains nothing
	agg       *evaluator // computes aggregates over the result; made when first needed
}

// spend charges one candidate against the step limit and reports whether it is gone.
func (x *explainer) spend() bool { return x.spendN(1) }

// spendN charges n against the step limit.
func (x *explainer) spendN(n int) bool {
	if x.steps += n; x.steps > whyNotSteps {
		x.exhausted = true
	}
	return x.exhausted
}

// scanCost is what finding the candidates for args in rel costs before any is tried:
// nothing when an index answers, every tuple when the relation has no index (a derived
// relation) or no argument is bound, since then the relation is walked whole. It is
// charged so the budget bounds the work and not only the matches.
func scanCost(rel *relation, args []Term, env map[string]Const) int {
	if rel.idx != nil {
		for _, a := range args {
			if _, ok := constant(a, env); ok {
				return 0
			}
		}
	}
	return len(rel.tuples)
}

// absence explains why goal, over relation name, is not in the result. depth is how
// many more levels of Because it may open.
func (x *explainer) absence(name string, goal []Term, depth int) Absence {
	a := Absence{Relation: name, Goal: slices.Clone(goal)}
	if x.r.plan.idb[name] {
		a.Rules = []RuleFailure{}
	}
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
	env, alias, ok := unifyHead(ru.Head.Args, goal)
	if !ok {
		return RuleFailure{}, false, false
	}
	w := &walker{x: x, r: ru, alias: alias, body: ru.ordered}
	if len(alias) > 0 {
		w.body = renamed(ru.ordered, alias)
	}
	w.walk(0, env)
	rf = RuleFailure{Rule: x.b.rule(&ru.Rule), Bound: w.bound(env)}
	if !w.found {
		return rf, false, true
	}
	rf.Bound, rf.Failed = w.bound(w.best.env), copyLiteral(ru.ordered[w.best.idx])
	rf.Value, rf.Reason = w.best.value, w.best.reason
	switch {
	case w.best.blocked != nil:
		p := x.b.build(w.best.lit.Atom.Name, w.best.blocked)
		rf.Blocked = &p
	case w.best.lit.Atom != nil && !w.best.lit.Negated && !x.r.plan.builtins.has(w.best.lit.Atom.Name):
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
