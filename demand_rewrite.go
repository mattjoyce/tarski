package tarski

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// This file is the magic-sets rewrite (docs/design/demand.md §2): non-subsumptive, one
// hidden demand relation per (relation, adornment) reached from the goal, every copy of
// a source rule writing into the source relation, so the Result, proofs and why-not see
// only source names.
//
// The invariants it serves (design §0): a copy derives only tuples its source rule
// derives (D2, soundness), and for every binding in a demand relation at the fixpoint
// every tuple of the full fixpoint agreeing with it is derived (D3, completeness per
// demand), which is what makes the goal's relation, filtered, equal full evaluation's
// (D1). Demand rules leave guards and negations out, so a demand set may be a superset
// of the exact one: still sound and complete, only more work downstream.

// adorned is one relation under one binding pattern: b where a position is bound, f
// where it is open.
type adorned struct {
	relation  string
	adornment string
}

// hiddenName is the demand relation of (relation, adornment). It holds "?", which
// isName refuses, so no consumer name collides with it and no consumer can read, declare
// or seed it.
func hiddenName(a adorned) string { return "m?" + a.relation + "?" + a.adornment }

// rewriter walks the adornments reached from the goal, first come first served, and
// makes each one's rules: per source rule a copy led by the demand literal, and per
// derived literal in it a demand rule. The order is a function of the plan and the goal
// shape (D5), since the worklist is walked in order and source rules in plan order.
type rewriter struct {
	src    Plan
	rules  map[string][]*rule // source rules by head, in plan order
	queue  []adorned
	seen   map[adorned]bool
	out    []rule
	hidden map[string]string
}

// rewrite is the demand plan for goals over relation with adornment: the rewritten
// rules, stratified and placed. A rewrite stratify refuses is refused here (lifting
// that is a later slice: drop demand per relation until it stratifies).
func (p Plan) rewrite(relation, adornment string) (*demandPlan, error) {
	w := &rewriter{src: p, rules: map[string][]*rule{}, seen: map[adorned]bool{}, hidden: map[string]string{}}
	for s := range p.strata {
		for i := range p.strata[s] {
			r := &p.strata[s][i]
			w.rules[r.Head.Name] = append(w.rules[r.Head.Name], r)
		}
	}
	goal := adorned{relation, adornment}
	w.push(goal)
	for i := 0; i < len(w.queue); i++ {
		a := w.queue[i]
		for _, sr := range w.rules[a.relation] {
			w.copyRule(sr, a)
		}
	}
	idb := make(map[string]bool, len(p.idb)+len(w.hidden))
	for name := range p.idb {
		idb[name] = true
	}
	for name := range w.hidden {
		idb[name] = true
	}
	plain := make([]Rule, len(w.out))
	for i, r := range w.out {
		plain[i] = r.Rule
	}
	stratum, err := levels(plain, idb)
	if err != nil {
		return nil, fmt.Errorf("%w: Demand(%q, %q): demand for this plan is not yet supported, since its rewrite cannot be stratified (%w); "+
			"evaluate the plan without a goal", ErrInvalid, relation, adornment, err)
	}
	d := &demandPlan{relation: relation, adornment: adornment, strata: place(w.out, stratum, idb), idb: idb, hidden: w.hidden}
	if strings.Contains(adornment, "b") {
		d.seed = hiddenName(goal)
	}
	return d, nil
}

// push puts an adornment on the worklist once, and names its demand relation when it
// binds something. The all-free adornment means the whole relation: it has no demand
// relation, and its copies are the source rules.
func (w *rewriter) push(a adorned) {
	if w.seen[a] {
		return
	}
	w.seen[a] = true
	w.queue = append(w.queue, a)
	if strings.Contains(a.adornment, "b") {
		w.hidden[hiddenName(a)] = a.relation
	}
}

// copyRule makes sr's copy for adornment a and the demand rules of the derived literals
// in its body. The body is walked in the source's evaluation order with the head's bound
// positions bound: a derived atom, positive or negated, is adorned by which of its
// arguments are bound there (a constant is bound, _ is open), and a positive atom binds
// its variables after it. The copy's ordered body is the demand literal and then the
// source's ordered body unchanged, so a guard keeps its place and a proof's body order is
// the source's.
func (w *rewriter) copyRule(sr *rule, a adorned) {
	bound := map[string]bool{}
	var lead *Literal
	if strings.Contains(a.adornment, "b") {
		lead = &Literal{Atom: &Atom{Name: hiddenName(a), Args: pick(sr.Head.Args, a.adornment), Line: sr.Line}}
		for i, t := range sr.Head.Args {
			if a.adornment[i] == 'b' && t.IsVar() && t.Var != "_" {
				bound[t.Var] = true
			}
		}
	}
	w.out = append(w.out, copyOf(sr, lead))
	headBound := maps.Clone(bound)
	for k, l := range sr.ordered {
		if l.Atom == nil || w.src.builtins.has(l.Atom.Name) {
			continue
		}
		if w.src.idb[l.Atom.Name] {
			s := adorned{l.Atom.Name, adornmentOf(l.Atom.Args, bound)}
			w.push(s)
			if dr, ok := w.demandRule(sr, k, s, lead, headBound); ok {
				w.out = append(w.out, dr)
			}
		}
		if !l.Negated {
			bindVars(bound, l.Atom.Args)
		}
	}
}

// copyOf is sr's copy led by lead, or sr itself as a rewritten rule when lead is nil
// (the all-free adornment). The Rule a copy carries has the demand literal in its body
// so stratification sees the edge; its proofs name sr, its source.
func copyOf(sr *rule, lead *Literal) rule {
	c := rule{Rule: sr.Rule, ordered: sr.ordered, source: &sr.Rule, demand: noDemand}
	if lead == nil {
		return c
	}
	c.Body = append([]Literal{*lead}, sr.Body...)
	c.ordered = append([]Literal{*lead}, sr.ordered...)
	c.demand = 0
	return c
}

// demandRule is the rule that passes demand to the derived literal at sr.ordered[k],
// adorned s: its head is s's demand relation over s's bound arguments, its body the
// copy's demand literal (if any) and the smallest set of positive atoms before k that
// binds what the head needs, found by walking back from k and keeping an atom when it
// binds a needed variable, then needing its variables too. ok is false when s binds
// nothing, and when the rule would only restate its own demand literal (a recursion
// through the same binding).
func (w *rewriter) demandRule(sr *rule, k int, s adorned, lead *Literal, headBound map[string]bool) (rule, bool) {
	if !strings.Contains(s.adornment, "b") {
		return rule{}, false
	}
	head := Atom{Name: hiddenName(s), Args: pick(sr.ordered[k].Atom.Args, s.adornment), Line: sr.Line}
	need := map[string]bool{}
	note := func(args []Term) {
		for _, t := range args {
			if t.IsVar() && t.Var != "_" && !headBound[t.Var] {
				need[t.Var] = true
			}
		}
	}
	note(head.Args)
	var prefix []Literal
	for j := k - 1; j >= 0; j-- {
		l := sr.ordered[j]
		if l.Atom == nil || l.Negated || w.src.builtins.has(l.Atom.Name) || !binds(l.Atom.Args, need) {
			continue
		}
		prefix = append(prefix, l)
		note(l.Atom.Args)
	}
	slices.Reverse(prefix)
	var body []Literal
	demand := noDemand
	if lead != nil {
		if len(prefix) == 0 && lead.Atom.Name == head.Name && slices.Equal(lead.Atom.Args, head.Args) {
			return rule{}, false
		}
		body, demand = append(body, *lead), 0
	}
	body = append(body, prefix...)
	r := Rule{Head: head, Body: body, Line: sr.Line}
	return rule{Rule: r, ordered: body, demand: demand}, true
}

// binds reports whether args name a variable in need.
func binds(args []Term, need map[string]bool) bool {
	for _, t := range args {
		if t.IsVar() && need[t.Var] {
			return true
		}
	}
	return false
}

// adornmentOf is the binding pattern of args under bound: b for a constant or a bound
// variable, f for _ or an unbound variable.
func adornmentOf(args []Term, bound map[string]bool) string {
	b := make([]byte, len(args))
	for i, t := range args {
		b[i] = 'f'
		if !t.IsVar() || t.Var != "_" && bound[t.Var] {
			b[i] = 'b'
		}
	}
	return string(b)
}

// pick is the terms at the bound positions of adornment, in order.
func pick(args []Term, adornment string) []Term {
	var out []Term
	for i, t := range args {
		if adornment[i] == 'b' {
			out = append(out, t)
		}
	}
	return out
}
