package tarski

import (
	"fmt"
	"slices"
)

// This file is the rewrite's driver and its fallback (docs/design/demand.md §2.4, decided
// §8 Q5 and Q6). Magic rewriting can break stratification where the source program
// stratifies, and a relation can be reached under more binding patterns than are worth
// copying its rules for. In both cases demand is dropped for one relation, which is then
// derived whole wherever it is reached, and the rewrite runs again.
//
// Why it never changes an answer: a relation derived whole is complete for every binding,
// so D3 holds for it trivially, and the relations below it keep their own demand, still
// complete per demand. Why it ends: each round drops a relation not dropped before, and
// with every relation dropped the rewrite is the source program, which stratifies. Why it
// is deterministic (D5): the worklist is walked in plan order, components in the order
// components gives them, and the relation dropped from a component is chosen by sorted
// name.

// maxAdornments is how many binding patterns one relation may be reached under before its
// demand is dropped. Each pattern copies the relation's rules once and adds demand rules,
// so the cap bounds the rewritten program at maxAdornments copies per source rule. It is
// a constant, not an option, because dropping demand never changes an answer: it only
// trades the rewrite's size for derivation work, which MaxSteps already bounds. 64 is
// every pattern of a relation of arity six, far above what a consumer's rules reach (a
// handful per relation, design §1.2).
const maxAdornments = 64

// rewrite is the demand plan for goals over relation with adornment: the rewritten rules,
// stratified and placed, with demand dropped for as few relations as the fallback needed,
// in the order it dropped them.
func (p Plan) rewrite(relation, adornment string) (*demandPlan, error) {
	goal := adorned{relation, adornment}
	dropped := map[string]bool{}
	var order []string
	for {
		w := p.newRewriter(dropped)
		w.walk(goal)
		drop := w.over
		if drop == "" {
			plain, idb := w.program()
			stratum, err := levels(plain, idb)
			if err == nil {
				return w.plan(goal, stratum, idb, order), nil
			}
			if drop = w.breaker(plain, idb); drop == "" {
				// Unreachable: a cycle of source relations alone is a cycle of the source
				// program, which Compile stratified. Kept so a broken invariant is an error.
				return nil, fmt.Errorf("%w: Demand(%q, %q): the rewrite cannot be stratified (%w)", ErrInvalid, relation, adornment, err)
			}
		}
		dropped[drop] = true
		order = append(order, drop)
	}
}

// plan is the demand plan the walk made, its rules placed in their strata.
func (w *rewriter) plan(goal adorned, stratum map[string]int, idb map[string]bool, dropped []string) *demandPlan {
	d := &demandPlan{relation: goal.relation, adornment: goal.adornment, strata: place(w.out, stratum, idb), idb: idb,
		hidden: w.hidden, dropped: dropped}
	if seed := hiddenName(goal); w.hidden[seed] != "" {
		d.seed = seed
	}
	return d
}

// breaker is the relation whose demand to drop when the rewritten rules cannot be
// stratified: in the first component, in components' order, that holds an edge through a
// negation or an aggregate between two of its members, the source relation of the first
// demand relation by sorted name. Every such component holds a demand relation, since
// the source relations alone stratify; "" says one did not.
func (w *rewriter) breaker(plain []Rule, idb map[string]bool) string {
	deps := dependencies(plain, idb)
	for _, comp := range components(deps) {
		if !negativeWithin(comp, deps) {
			continue
		}
		for _, n := range comp { // sorted by components
			if src, ok := w.hidden[n]; ok {
				return src
			}
		}
		return ""
	}
	return ""
}

// negativeWithin reports whether some member of comp reads another, or itself, through a
// negation or an aggregate.
func negativeWithin(comp []string, deps map[string][]edge) bool {
	for _, n := range comp {
		for _, d := range deps[n] {
			if d.negative && slices.Contains(comp, d.to) {
				return true
			}
		}
	}
	return false
}
