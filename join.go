package tarski

import "fmt"

// This file is the join: how a rule body is walked against the database, literal by
// literal, and the charges and guards a walk meets on the way. eval.go drives it.

// join extends env through the body literals from i on, calling emit for every
// complete binding.
func (ev *evaluator) join(r *rule, i int, env map[string]Const, delta *relBuilder, deltaPos int, emit func()) error {
	if i == len(r.ordered) {
		emit()
		return nil
	}
	l := r.ordered[i]
	switch {
	case l.Aggregate != nil:
		return ev.aggregate(r, i, env, delta, deltaPos, emit)
	case l.Compare != nil || ev.builtins.has(l.Atom.Name):
		ok, err := ev.builtins.guardHolds(l, env)
		if err != nil || !ok {
			return err
		}
		return ev.join(r, i+1, env, delta, deltaPos, emit)
	case l.Negated:
		found, err := ev.exists(r.Head.Name, ev.db[l.Atom.Name], l.Atom.Args, env)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		return ev.join(r, i+1, env, delta, deltaPos, emit)
	}
	rel := ev.db[l.Atom.Name]
	if i == deltaPos {
		rel = source{bld: delta}
	}
	for _, idx := range matching(rel, l.Atom.Args, env) {
		if ev.over != nil {
			return ev.over
		}
		if err := ev.step(r.Head.Name); err != nil {
			return err
		}
		bound, ok := bind(rel.tuples()[idx], l.Atom.Args, env)
		if !ok {
			continue
		}
		if ev.proofs != nil {
			ev.stack = append(ev.stack, bodyTuple{l.Atom.Name, rel.tuples()[idx]})
		}
		if err := ev.join(r, i+1, env, delta, deltaPos, emit); err != nil {
			return err
		}
		if ev.proofs != nil {
			ev.stack = ev.stack[:len(ev.stack)-1]
		}
		for _, v := range bound {
			delete(env, v)
		}
	}
	return nil
}

// step charges one join step: against MaxSteps, and every stepsPerDeadlineCheck-th
// time against the deadline.
func (ev *evaluator) step(relation string) error {
	ev.steps++
	if ev.opts.MaxSteps > 0 && ev.steps > ev.opts.MaxSteps {
		ev.over = &TooLarge{Bound: BoundSteps, Relation: relation, Tuples: ev.tuples, Bytes: ev.bytes, Steps: ev.steps, MaxSteps: ev.opts.MaxSteps}
		return ev.over
	}
	if ev.steps%stepsPerDeadlineCheck == 0 {
		return ev.deadline()
	}
	return nil
}

// stepN charges n steps at once: work done outside a join that must still count against
// MaxSteps, and that crosses a deadline check whenever step would have.
func (ev *evaluator) stepN(relation string, n int) error {
	if n <= 0 {
		return nil
	}
	before := ev.steps
	ev.steps += n
	if ev.opts.MaxSteps > 0 && ev.steps > ev.opts.MaxSteps {
		ev.over = &TooLarge{Bound: BoundSteps, Relation: relation, Tuples: ev.tuples, Bytes: ev.bytes, Steps: ev.steps, MaxSteps: ev.opts.MaxSteps}
		return ev.over
	}
	if before/stepsPerDeadlineCheck != ev.steps/stepsPerDeadlineCheck {
		return ev.deadline()
	}
	return nil
}

// deadline is the evaluation's error when its context is done. It is asked at entry,
// before each rule is applied, every stepsPerDeadlineCheck-th join step inside a
// rule, and before the result is returned: a rule that visits no tuple (a ground
// fact, a guard over nothing) charges no step, so the per-rule look is what bounds
// how long a cancelled evaluation of many such rules runs on.
func (ev *evaluator) deadline() error {
	if err := ev.ctx.Err(); err != nil {
		return fmt.Errorf("tarski: evaluation stopped: %w", err)
	}
	return nil
}

// stepsPerDeadlineCheck is how many tuples a join visits between looks at the
// deadline. It changes no result and no limit a person meets: only how promptly a
// cancelled evaluation stops, against the cost of asking.
const stepsPerDeadlineCheck = 1024

// exists reports whether some tuple of rel matches args under env. Each candidate it
// tries is a join step like any other, charged against MaxSteps and the deadline: a
// negation can scan as much as a join, and an error here is the bound's, not a miss.
func (ev *evaluator) exists(head string, rel source, args []Term, env map[string]Const) (bool, error) {
	for _, idx := range matching(rel, args, env) {
		if err := ev.step(head); err != nil {
			return false, err
		}
		if bound, ok := bind(rel.tuples()[idx], args, env); ok {
			for _, v := range bound {
				delete(env, v)
			}
			return true, nil
		}
	}
	return false, nil
}

// holds evaluates a comparison under env. Constants of different kinds are
// unequal and unordered: = is false, != is true, and the orderings are false.
func holds(c Comparison, env map[string]Const) bool {
	l, r := value(c.Left, env), value(c.Right, env)
	cmp, ok := compare(l, r)
	switch c.Op {
	case "=":
		return ok && cmp == 0
	case "!=":
		return !ok || cmp != 0
	case "<":
		return ok && cmp < 0
	case "<=":
		return ok && cmp <= 0
	case ">":
		return ok && cmp > 0
	case ">=":
		return ok && cmp >= 0
	default:
		return false // Compile refuses any other operator; failing closed if one gets here
	}
}

// guardHolds evaluates a comparison or a built-in literal under env. The why-not walk
// uses it too. Only a builtin that panics returns an error.
func (t builtinTable) guardHolds(l Literal, env map[string]Const) (bool, error) {
	if l.Compare != nil {
		return holds(*l.Compare, env), nil
	}
	return t.holds(l, env)
}
