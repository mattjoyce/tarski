package tarski

import (
	"errors"
	"strings"
)

// This file evaluates aggregates (docs/design/aggregates.md §4). An aggregate literal
// reads only relations in strictly lower strata (stratify gives every atom inside its
// braces a negative edge), so when its rule runs every relation it reads is complete,
// and its value for a group is a fixed function of the group key for the whole
// evaluation. That is what makes the memo sound and the stratum's operator monotone.

// aggPlan is one aggregate literal ready to evaluate: the literal, its group variables
// in sorted order (the order of the memo key), and its body ordered with those
// variables bound, as a rule whose head names the relation being derived so a bound
// crossed inside the braces names it.
type aggPlan struct {
	lit   *Aggregate
	group []string
	inner *rule
}

// newAggPlan orders a's body with its group variables bound.
func newAggPlan(head string, a *Aggregate, group []string, table builtinTable) *aggPlan {
	bound := make(map[string]bool, len(group))
	for _, g := range group {
		bound[g] = true
	}
	inner := &rule{Rule: Rule{Head: Atom{Name: head}}, ordered: order(a.Body, table, bound, nil)}
	return &aggPlan{lit: a, group: group, inner: inner}
}

// aggKey is one group of one aggregate literal: the literal's plan and the keys of its
// group variables' values, in the plan's order.
type aggKey struct {
	plan  *aggPlan
	group string
}

// aggValue is what an aggregate came to for one group. ok is false when the group
// derives no tuple: a min or max over an empty group, or a group whose values the
// operation cannot take (a non-number under sum, two kinds under min or max). size is
// the number of witnesses. mixed holds what made the group unusable, in witness order:
// the first non-number under sum, or the two first constants of different kinds under
// min or max. It is kept for the why-not of an absent tuple, which reports it.
type aggValue struct {
	value Const
	ok    bool
	size  int
	mixed []Const
}

// aggregate is join's step for the aggregate literal at ordered index i: its value for
// the group the bindings so far select binds the result, or tests it when the result is
// already bound, and the join goes on. A group that derives no tuple ends this branch.
func (ev *evaluator) aggregate(r *rule, i int, env map[string]Const, delta *relBuilder, deltaPos int, emit func()) error {
	a := r.aggs[i]
	v, err := ev.aggValue(a, env)
	if err != nil || !v.ok {
		return err
	}
	res := a.lit.Result.Var
	if c, bound := env[res]; bound {
		if !c.same(v.value) {
			return nil
		}
		return ev.join(r, i+1, env, delta, deltaPos, emit)
	}
	env[res] = v.value
	err = ev.join(r, i+1, env, delta, deltaPos, emit)
	delete(env, res)
	return err
}

// aggValue is the aggregate's value for the group env selects, computed once per group
// in an evaluation. The body runs through join over a fresh environment holding only
// the group bindings, so nothing outside the group reaches it, and each candidate it
// visits is a join step charged to the relation being derived. One more step is charged
// per group computed, so an aggregate over nothing is not free.
//
// Each solution join emits is one witness. The relations are sets and the body's
// positive atoms are walked one distinct tuple at a time, so two solutions differ in
// at least one atom's tuple: the solutions are the distinct combinations of tuples, the
// witness set of §1.1, with no second pass to remove duplicates.
func (ev *evaluator) aggValue(a *aggPlan, env map[string]Const) (aggValue, error) {
	var key strings.Builder
	inner := make(map[string]Const, len(a.group))
	for _, g := range a.group {
		c := env[g]
		c.writeKey(&key)
		inner[g] = c
	}
	k := aggKey{plan: a, group: key.String()}
	if v, done := ev.aggMemo[k]; done {
		return v, nil
	}
	if err := ev.step(a.inner.Head.Name); err != nil {
		return aggValue{}, err
	}
	f := folder{op: a.lit.Op, target: a.lit.Target.Var}
	if err := ev.join(a.inner, 0, inner, nil, -1, func() { f.add(inner) }); err != nil {
		return aggValue{}, err
	}
	v, err := f.value()
	if err != nil {
		var tl *TooLarge
		if errors.As(err, &tl) {
			tl.Relation, tl.Tuples, tl.Bytes, tl.Steps = a.inner.Head.Name, ev.tuples, ev.bytes, ev.steps
		}
		return aggValue{}, err
	}
	if ev.aggMemo == nil {
		ev.aggMemo = map[aggKey]aggValue{}
	}
	ev.aggMemo[k] = v
	return v, nil
}

// folder takes one group's witnesses, in witness order, to its value.
type folder struct {
	op, target string
	size       int
	values     []Const // sum: the target of each witness
	best       Const   // min, max: the extreme so far
	mixed      []Const
}

// add takes one witness, whose bindings env holds.
func (f *folder) add(env map[string]Const) {
	f.size++
	if f.op == "count" {
		return
	}
	c := env[f.target]
	switch {
	case f.op == "sum":
		if c.kind != KindNumber && f.mixed == nil {
			f.mixed = []Const{c}
		}
		f.values = append(f.values, c)
	case f.size == 1:
		f.best = c
	case f.mixed != nil:
	default:
		cmp, ok := compare(c, f.best)
		switch {
		case !ok:
			f.mixed = []Const{f.best, c}
		case f.op == "min" && cmp < 0, f.op == "max" && cmp > 0:
			f.best = c
		}
	}
}

// value is the group's value: count and sum give 0 for an empty group, as Soufflé
// does; min and max give none. A sum is exact; past maxSumDigits or maxExponent it is
// *TooLarge with Bound BoundNumber, which the caller completes.
func (f *folder) value() (aggValue, error) {
	v := aggValue{size: f.size, mixed: f.mixed}
	switch {
	case f.mixed != nil:
		return v, nil
	case f.op == "count":
		v.value, v.ok = Int(int64(f.size)), true
	case f.op == "sum":
		total, err := sumConsts(f.values)
		if err != nil {
			return aggValue{}, err
		}
		v.value, v.ok = total, true
	case f.size > 0:
		v.value, v.ok = f.best, true
	}
	return v, nil
}
