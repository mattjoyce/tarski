package tarski

import (
	"context"
	"fmt"
	"sort"
)

// Result is what an evaluation derived: every relation, base and derived, at the
// fixpoint.
type Result struct {
	relations map[string]*relation
	proofs    map[string]map[string]*derivation // nil unless Options.Proofs
	plan      Plan                              // what WhyNot reads the rules and arities from
}

// Options bounds and shapes one evaluation.
type Options struct {
	// MaxBytes is a bound over approximately the JSON text of every derived tuple
	// (lengths unescaped), intermediate relations included, since derived relations can
	// be larger than the input. Zero is no bound; over it, Evaluate returns *TooLarge.
	MaxBytes int
	// MaxSteps bounds the work: join steps (tuples visited) over the whole evaluation.
	// MaxBytes cannot see a rule that visits a great deal and derives little, such as a
	// cross product of duplicates. Zero is no bound; over it, Evaluate returns *TooLarge.
	MaxSteps int
	// Proofs records, for each derived tuple, the first derivation that produced it.
	// It touches the hot loop, so it is off unless asked for.
	Proofs bool
}

// Evaluate runs a plan over a base to its fixpoint, stratum by stratum, semi-naive
// within each: after the first round only the tuples new in the last round are
// joined again. It reads values and writes nothing, and the base is never changed, so
// calls may share one. The deadline on ctx, opts.MaxBytes and opts.MaxSteps bound it.
// A base built under a schema other than the plan's is ErrInvalid. A context already done fails
// at once, before any join, so the deadline is honoured however small the evaluation.
// A consumer builtin that panics fails the evaluation with ErrInvalid, never a partial
// result; the panic does not escape.
func Evaluate(ctx context.Context, plan Plan, base Base, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("tarski: evaluation stopped: %w", err)
	}
	if err := plan.accepts(base); err != nil {
		return Result{}, err
	}
	ev := &evaluator{ctx: ctx, db: map[string]source{}, opts: opts, builtins: plan.builtins}
	if opts.Proofs {
		ev.proofs = map[string]map[string]*derivation{}
	}
	for name, rel := range base.relations {
		ev.db[name] = source{val: rel}
	}
	for name := range plan.idb {
		ev.db[name] = source{bld: newRelBuilder()}
	}
	for _, stratum := range plan.strata {
		if err := ev.stratum(stratum); err != nil {
			return Result{}, err
		}
	}
	// A bound crossed by the last tuple of the last round raises no error inside the
	// join; it is raised here, so MaxBytes is never exceeded with a nil error.
	if ev.over != nil {
		return Result{}, ev.over
	}
	// A cancellation that landed after the last look at the deadline is honoured
	// here, so a cancelled evaluation never returns a result.
	if err := ev.deadline(); err != nil {
		return Result{}, err
	}
	return Result{relations: ev.sealed(plan), proofs: ev.proofs, plan: plan}, nil
}

// sealed is the evaluation's relations as values: the base's own, and each derived
// relation sealed without indexes, which costs nothing. After this no builder is held.
func (ev *evaluator) sealed(plan Plan) map[string]*relation {
	out := make(map[string]*relation, len(ev.db))
	for name, s := range ev.db {
		if s.val != nil {
			out[name] = s.val
		} else {
			out[name] = s.bld.seal(plan.arity[name], false)
		}
	}
	return out
}

type evaluator struct {
	ctx   context.Context
	db    map[string]source
	steps int

	// Proofs only: the first derivation of each derived tuple, by relation then tuple
	// key, and the positive body tuples matched so far in the rule being joined. Both
	// stay nil when Proofs is off, so the join pays one nil check and nothing else.
	proofs map[string]map[string]*derivation
	stack  []bodyTuple

	opts          Options
	builtins      builtinTable // the plan's, read only
	bytes, tuples int
	over          *TooLarge

	// aggMemo is each aggregate's value per group, computed once in the evaluation
	// (aggregate_eval.go). It stays nil for a plan with no aggregate.
	aggMemo map[aggKey]aggValue
}

// stratum evaluates one stratum's rules to a fixpoint.
func (ev *evaluator) stratum(rules []rule) error {
	delta := map[string]*relBuilder{}
	for i := range rules {
		if err := ev.deadline(); err != nil {
			return err
		}
		if err := ev.apply(&rules[i], nil, -1, delta); err != nil {
			return err
		}
		if ev.over != nil {
			return ev.over
		}
	}
	for len(delta) > 0 {
		ev.merge(delta)
		next := map[string]*relBuilder{}
		for i := range rules {
			r := &rules[i] // a pointer into the plan, which a proof's Rule shares
			if err := ev.deadline(); err != nil {
				return err
			}
			for _, pos := range r.recursive {
				d, ok := delta[r.ordered[pos].Atom.Name]
				if !ok {
					continue
				}
				if err := ev.apply(r, d, pos, next); err != nil {
					return err
				}
			}
		}
		delta = next
	}
	return nil
}

// merge adds a round's new tuples to the database, relations in name order. Each
// relation's tuples are already in derivation order; walking the relations sorted
// keeps the evaluation a function of its inputs, with no map order in it, which a
// proof of "the first derivation" relies on.
func (ev *evaluator) merge(delta map[string]*relBuilder) {
	names := make([]string, 0, len(delta))
	for name := range delta {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		rel := delta[name]
		for _, t := range rel.tuples {
			ev.db[name].bld.add(t)
		}
	}
}

// apply derives r's head once over the database, with the positive atom at deltaPos
// (if any) read from delta instead, and collects tuples not yet derived into out.
func (ev *evaluator) apply(r *rule, delta *relBuilder, deltaPos int, out map[string]*relBuilder) error {
	env := map[string]Const{}
	if ev.proofs != nil {
		ev.stack = make([]bodyTuple, 0, len(r.ordered))
	}
	return ev.join(r, 0, env, delta, deltaPos, func() {
		t := make(Tuple, len(r.Head.Args))
		for i, a := range r.Head.Args {
			t[i] = value(a, env)
		}
		if ev.db[r.Head.Name].has(t) {
			return
		}
		rel, ok := out[r.Head.Name]
		if !ok {
			rel = newRelBuilder()
			out[r.Head.Name] = rel
		}
		if rel.add(t) {
			ev.count(r.Head.Name, t)
			if ev.proofs != nil {
				ev.record(r, t)
			}
		}
	})
}

// derivation is one way a tuple came to be: the rule, and the positive body tuples
// in the order written.
type derivation struct {
	rule *Rule
	body []bodyTuple
}

type bodyTuple struct {
	relation string
	tuple    Tuple
}

// record keeps the derivation that just added t. apply reaches it only for a tuple
// neither the database nor this round had, so what is kept is the first derivation. A
// body tuple was in the database or the last round's delta, hence derived strictly
// earlier, which is what keeps a proof acyclic.
func (ev *evaluator) record(r *rule, t Tuple) {
	by, ok := ev.proofs[r.Head.Name]
	if !ok {
		by = map[string]*derivation{}
		ev.proofs[r.Head.Name] = by
	}
	d := &derivation{rule: &r.Rule}
	if len(ev.stack) > 0 {
		d.body = append([]bodyTuple(nil), ev.stack...)
	}
	by[t.key()] = d
}

// count charges one derived tuple against the byte bound. The charge approximates the
// tuple's JSON text: brackets, commas, the lengths of the constants and two quotes per
// string, but not the escapes encoding/json adds, so the real text can be somewhat
// larger.
func (ev *evaluator) count(relation string, t Tuple) {
	n := 1 + len(t) // brackets and commas
	for _, c := range t {
		n += len(c.s)
		if c.kind == KindString {
			n += 2
		}
	}
	ev.tuples++
	ev.bytes += n
	if ev.opts.MaxBytes > 0 && ev.bytes > ev.opts.MaxBytes && ev.over == nil {
		ev.over = &TooLarge{Bound: BoundBytes, Relation: relation, Tuples: ev.tuples, Bytes: ev.bytes, MaxBytes: ev.opts.MaxBytes, Steps: ev.steps}
	}
}
