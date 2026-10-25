package tarski

import "maps"

// miss is the farthest failure a walk has seen: where, with what bindings, for a
// negation that held, the tuple that held it, and for an aggregate, its value and why
// it stopped the rule.
type miss struct {
	idx     int
	env     map[string]Const
	lit     Literal
	blocked Tuple
	value   *Const
	reason  string
}

// walker is a depth-first walk over one rule's ordered body.
type walker struct {
	x *explainer
	r *rule
	// body is the rule's ordered body as walked: the rule's own, or a copy with the
	// head variables the goal equates renamed to one, so the walk honours the equality.
	body []Literal
	// alias maps each renamed variable to the one it was renamed to.
	alias map[string]string
	best  miss
	found bool // some literal found no match
	full  bool // the body matched in full
}

// bound is env as the rule's own variables: a variable renamed for the walk is shown
// bound to what its canonical name is bound to.
func (w *walker) bound(env map[string]Const) map[string]Const {
	out := maps.Clone(env)
	for v, canon := range w.alias {
		if c, ok := env[canon]; ok {
			out[v] = c
		}
	}
	return out
}

// walk extends env through the body from literal i on. It returns false to stop the
// whole walk (a full match, or the step limit).
func (w *walker) walk(i int, env map[string]Const) bool {
	if w.x.exhausted || w.full {
		return false
	}
	if i == len(w.body) {
		w.full = true
		return false
	}
	l := w.body[i]
	switch {
	case l.Aggregate != nil:
		return w.aggregate(i, l, env)
	case l.Compare != nil || w.x.r.plan.builtins.has(l.Atom.Name):
		ok, err := w.x.r.plan.builtins.guardHolds(l, env)
		if err != nil {
			w.x.failed, w.x.exhausted = err, true
			return false
		}
		if ok {
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
	if w.x.spendN(scanCost(rel, l.Atom.Args, env)) {
		return false
	}
	matched := false
	for _, idx := range matching(source{val: rel}, l.Atom.Args, env) {
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
	if w.x.spendN(scanCost(rel, args, env)) {
		return nil, false, true
	}
	for _, idx := range matching(source{val: rel}, args, env) {
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
	w.keep(miss{idx: i, env: env, lit: l, blocked: blocked})
}

// keep is record for a miss already made; its env is cloned if it is kept.
func (w *walker) keep(m miss) {
	if w.found && m.idx <= w.best.idx {
		return
	}
	m.env = maps.Clone(m.env)
	w.found, w.best = true, m
}
