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
	inner := &rule{Rule: Rule{Head: Atom{Name: head}}, ordered: order(a.Body, table, bound, nil), demand: noDemand}
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
// the first non-number under sum; under min or max, the extreme held so far and the
// first constant whose kind differs from it. It is kept for the why-not of an absent tuple, which reports it.
type aggValue struct {
	value Const
	ok    bool
	size  int
	mixed []Const
}

// maxProofGroup is how many witnesses a proof keeps of one aggregate-derived tuple's
// group; Proof.GroupSize states the whole group's size beside them. A count over 10^5
// entries would otherwise carry 10^5 witnesses in every receipt. It is a constant and
// not an option so that a proof's bytes stay a function of (plan, base), as they are
// for every other tuple (docs/design/aggregates.md §5.2, Q4).
const maxProofGroup = 64

// aggregate is join's step for the aggregate literal at ordered index i: its value for
// the group the bindings so far select binds the result, or tests it when the result is
// already bound, and the join goes on. A group that derives no tuple ends this branch.
func (ev *evaluator) aggregate(r *rule, i int, env map[string]Const, delta *relBuilder, deltaPos int, emit func()) error {
	a := r.aggs[i]
	v, k, err := ev.aggValue(a, env)
	if err != nil || !v.ok {
		return err
	}
	res := a.lit.Result.Var
	c, bound := env[res]
	if bound && !c.same(v.value) {
		return nil
	}
	if ev.proofs != nil {
		ev.aggPath = append(ev.aggPath, usedGroup{lit: a.lit, key: k, size: v.size})
	}
	if bound {
		err = ev.join(r, i+1, env, delta, deltaPos, emit)
	} else {
		env[res] = v.value
		err = ev.join(r, i+1, env, delta, deltaPos, emit)
		delete(env, res)
	}
	if ev.proofs != nil {
		ev.aggPath = ev.aggPath[:len(ev.aggPath)-1]
	}
	return err
}

// usedGroup is one aggregate literal's group on the join's current path, for the proof
// of a tuple the path derives (proofs only).
type usedGroup struct {
	lit  *Aggregate
	key  aggKey
	size int
}

// groupRecord is the group one aggregate-derived tuple was first taken over: the kept
// witnesses and the whole size.
type groupRecord struct {
	witnesses [][]bodyTuple
	size      int
}

// proofKey identifies a tuple of a relation: unambiguous, since a name holds no NUL
// (isName) and a key is self-delimiting.
func proofKey(name string, t Tuple) string { return name + "\x00" + t.key() }

// groupOf is the group a derivation of r on the current path was taken over: every
// aggregate literal's kept witnesses, concatenated in the rule's written order and
// cut at maxProofGroup, and the sum of the groups' sizes.
func (ev *evaluator) groupOf(r *rule) groupRecord {
	var g groupRecord
	for _, l := range r.Body {
		if l.Aggregate == nil {
			continue
		}
		for _, used := range ev.aggPath {
			if used.lit != l.Aggregate {
				continue
			}
			kept := ev.witnesses[used.key]
			g.size += used.size
			room := maxProofGroup - len(g.witnesses)
			g.witnesses = append(g.witnesses, kept[:min(room, len(kept))]...)
		}
	}
	return g
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
func (ev *evaluator) aggValue(a *aggPlan, env map[string]Const) (aggValue, aggKey, error) {
	var key strings.Builder
	inner := make(map[string]Const, len(a.group))
	for _, g := range a.group {
		c := env[g]
		c.writeKey(&key)
		inner[g] = c
	}
	k := aggKey{plan: a, group: key.String()}
	if v, done := ev.aggMemo[k]; done {
		return v, k, nil
	}
	if err := ev.step(a.inner.Head.Name); err != nil {
		return aggValue{}, k, err
	}
	f := folder{op: a.lit.Op, target: a.lit.Target.Var}
	emit := func() { f.add(inner) }
	var log *witnessLog
	if ev.proofs != nil {
		// The inner join pushes its positive tuples onto the proof stack above the outer
		// rule's and pops them again, so the witness is the stack above base.
		log = &witnessLog{base: len(ev.stack)}
		emit = func() {
			f.add(inner)
			log.add(ev.stack)
		}
	}
	if err := ev.join(a.inner, 0, inner, nil, -1, emit); err != nil {
		return aggValue{}, k, err
	}
	if f.op == "sum" && f.mixed == nil {
		// The fold aligns every addend to the group's width: charge it before doing it.
		if err := ev.stepN(a.inner.Head.Name, sumWork(f.values)); err != nil {
			return aggValue{}, k, err
		}
	}
	v, err := f.value()
	if err != nil {
		var tl *TooLarge
		if errors.As(err, &tl) {
			tl.Relation, tl.Tuples, tl.Bytes, tl.Steps = a.inner.Head.Name, ev.tuples, ev.bytes, ev.steps
		}
		return aggValue{}, k, err
	}
	if log != nil {
		if ev.witnesses == nil {
			ev.witnesses = map[aggKey][][]bodyTuple{}
		}
		ev.witnesses[k] = log.witnesses()
	}
	if ev.aggMemo == nil {
		ev.aggMemo = map[aggKey]aggValue{}
	}
	ev.aggMemo[k] = v
	return v, k, nil
}

// witnessLog keeps a group's first maxProofGroup witnesses as the inner join emits
// them, each the proof stack above base, in one backing array.
type witnessLog struct {
	base int
	flat []bodyTuple
	ends []int
}

func (w *witnessLog) add(stack []bodyTuple) {
	if len(w.ends) < maxProofGroup {
		w.flat = append(w.flat, stack[w.base:]...)
		w.ends = append(w.ends, len(w.flat))
	}
}

// witnesses is the kept witnesses, each a slice of the backing array capped at its
// end, so no holder can append into the next.
func (w *witnessLog) witnesses() [][]bodyTuple {
	if len(w.ends) == 0 {
		return nil
	}
	out := make([][]bodyTuple, len(w.ends))
	start := 0
	for i, end := range w.ends {
		out[i] = w.flat[start:end:end]
		start = end
	}
	return out
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
