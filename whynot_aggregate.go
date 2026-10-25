package tarski

import (
	"context"
	"errors"
	"fmt"
)

// This file is the why-not walk's step for an aggregate literal (docs/design/
// aggregates.md §5.4). The walk computes the aggregate for the group its bindings
// select, by the same function evaluation uses, over the result's relations. Those
// are the complete relations the evaluation read, so the value is the one the rule
// met. The work is charged to the walk's budget, whyNotSteps: every candidate the
// inner body visits, every relation it scans, one step per group, and a sum's fold
// (sumWork), so no aggregate can make an explanation unbounded.

// aggregate walks the aggregate literal at i: a group that gives no value, or a value
// other than the result already bound, is a failure with its reason; otherwise the
// value binds the result and the walk goes on.
func (w *walker) aggregate(i int, l Literal, env map[string]Const) bool {
	a := w.r.aggs[i]
	if w.x.spend() {
		return false
	}
	v, reason, stop := w.x.aggValue(a, w.groupEnv(a, env))
	switch {
	case stop:
		return false
	case reason == "" && !v.ok:
		reason = unusable(a.lit, v)
	}
	if reason != "" {
		w.keep(miss{idx: i, env: env, lit: l, reason: reason})
		return true
	}
	res := l.Aggregate.Result.Var
	if c, bound := env[res]; bound {
		if !c.same(v.value) {
			value := v.value
			w.keep(miss{idx: i, env: env, lit: l, value: &value,
				reason: fmt.Sprintf("the aggregate is %s, not %s", v.value.text(), c.text())})
			return true
		}
		return w.walk(i+1, env)
	}
	env[res] = v.value
	more := w.walk(i+1, env)
	delete(env, res)
	return more
}

// groupEnv is the group bindings under the aggregate's own variable names. The walk
// may have renamed a head variable to the one the goal equates it with (alias), and a
// group variable can be such a head variable.
func (w *walker) groupEnv(a *aggPlan, env map[string]Const) map[string]Const {
	out := make(map[string]Const, len(a.group))
	for _, g := range a.group {
		name := g
		if canon, ok := w.alias[g]; ok {
			name = canon
		}
		if c, ok := env[name]; ok {
			out[g] = c
		}
	}
	return out
}

// unusable is why a group gave no value: what aggValue.mixed names, or an empty group
// under min or max.
func unusable(a *Aggregate, v aggValue) string {
	switch {
	case a.Op == "sum" && len(v.mixed) > 0:
		return fmt.Sprintf("the group holds %s, which is not a number, so it cannot be summed", v.mixed[0].text())
	case len(v.mixed) > 1:
		return fmt.Sprintf("the group mixes kinds: %s and %s, so it has no %s", v.mixed[0].text(), v.mixed[1].text(), a.Op)
	}
	return fmt.Sprintf("the group is empty, so it has no %s", a.Op)
}

// aggValue computes an aggregate's value for one group over the result's relations,
// charging the work to the walk's budget. stop reports that the walk must end: the
// budget is spent, or a consumer builtin inside the braces panicked. A sum too wide to
// hold is no value, with the refusal as its reason; the evaluation, which met every
// group the walk can reach, would have refused it first.
func (x *explainer) aggValue(a *aggPlan, group map[string]Const) (v aggValue, reason string, stop bool) {
	left := whyNotSteps - x.steps
	if left <= 0 {
		x.exhausted = true
		return aggValue{}, "", true
	}
	if x.agg == nil {
		db := make(map[string]source, len(x.r.relations))
		for name, rel := range x.r.relations {
			db[name] = source{val: rel}
		}
		x.agg = &evaluator{ctx: context.Background(), db: db, builtins: x.r.plan.builtins, chargeScans: true}
	}
	ev := x.agg
	ev.steps, ev.over, ev.opts.MaxSteps = 0, nil, left
	v, _, err := ev.aggValue(a, group)
	x.spendN(ev.steps)
	var tl *TooLarge
	switch {
	case err == nil:
		return v, "", false
	case errors.As(err, &tl) && tl.Bound == BoundSteps:
		x.exhausted = true
		return aggValue{}, "", true
	case errors.As(err, &tl):
		return aggValue{}, tl.Error(), false
	}
	x.failed, x.exhausted = err, true
	return aggValue{}, "", true
}
