package tarski

import (
	"context"
	"fmt"
	"sort"
)

// Result is what an evaluation derived: every relation, base and derived, at the
// fixpoint.
//
// A Result evaluated for a goal (Options.Goal, on a plan from Plan.Demand) answers a
// narrower contract: among the derived relations only the goal's relation is complete,
// and Relation, Proof, WhyNot and WhyEmpty refuse or decline the rest. Check Goal before
// passing such a Result to code that reads arbitrary relations.
type Result struct {
	relations map[string]*relation
	proofs    map[string]map[string]*derivation // nil unless Options.Proofs
	groups    map[string]groupRecord            // nil unless Options.Proofs and an aggregate derived a tuple
	plan      Plan                              // what WhyNot reads the rules and arities from
	goal      *Goal                             // nil unless the plan was compiled for a goal (demand.go)
	steps     int                               // steps charged to MaxSteps (see Cost)
	bytes     int                               // derived bytes charged to MaxBytes (see Cost)
}

// Cost is the work the evaluation charged: steps, the count Options.MaxSteps bounds
// (Options.MaxSteps lists what a step is), and derived bytes, the count Options.MaxBytes
// bounds. It is available on any Result, with or without a goal. A demand evaluation's
// cost includes its helper tuples.
//
// Cost is the calibration for the bounds: evaluate a representative input, read Cost,
// and set the bounds with headroom above it. Within one version of the engine the count
// is exact and deterministic for one (plan, base, options), and a bound equal to the
// cost passes where one below it is refused. Across releases it is not stable, patch
// releases included, because the engine's accounting changes as it improves, so
// re-measure after an upgrade. Cost does not depend on Options.Proofs. It is no part of
// an Answer or its hash.
func (r Result) Cost() (steps, bytes int) { return r.steps, r.bytes }

// Options bounds and shapes one evaluation.
type Options struct {
	// MaxBytes is a bound over approximately the JSON text of every derived tuple
	// (lengths unescaped), intermediate relations included, since derived relations can
	// be larger than the input. Zero is no bound and a negative bound is ErrInvalid; over
	// it, Evaluate returns *TooLarge.
	MaxBytes int
	// MaxSteps bounds the work over the whole evaluation. MaxBytes cannot see a rule
	// that visits a great deal and derives little, such as a cross product of
	// duplicates. Zero is no bound and a negative bound is ErrInvalid; over it, Evaluate
	// returns *TooLarge.
	//
	// A step is one of:
	//   - a candidate tuple visited by a positive join, including the candidates inside
	//     an aggregate's braces and the hidden helper relations of a demand plan;
	//   - a candidate tuple tried by a negation probe;
	//   - one application of a rule, counted again for each semi-naive re-application
	//     in a later round, and so also for a rule with no positive atom;
	//   - one call of a builtin (a comparison is free);
	//   - one aggregate group computed, once per group however many rules read it;
	//   - the priced work of a sum's fold, charged before the fold runs: its digits
	//     parsed, added and aligned, in steps that grow with the width of the numbers.
	//
	// This list is the one definition of a step: Result.Cost, TooLarge.Steps, ErrTooLarge
	// and Builtin all refer to it. Within one version the count is exact and
	// deterministic for one (plan, base, options), and Result.Cost is the calibration:
	// read it on a representative input and set the bound with headroom. Across releases
	// it is not stable, patch releases included, so re-measure after an upgrade; a bound
	// set tight to the old count can refuse after it. TooLarge.Steps and TooLarge.Bytes
	// are on the same scale as Cost.
	MaxSteps int
	// Proofs records, for each derived tuple, the first derivation that produced it.
	// It touches the hot loop, so it is off unless asked for.
	Proofs bool
	// Goal is the question a plan compiled by Plan.Demand answers: its relation, a
	// constant at every position the plan's adornment binds, and a variable at every
	// position it leaves open. It is required on such a plan and refused (ErrInvalid) on
	// any other. The Result then answers only the goal: see Result.Relation.
	Goal *Goal
}

// bounded refuses a negative bound. Every check against a bound reads it only when it
// is positive, so a negative one would silently mean "no bound": it is ErrInvalid
// instead, and zero stays the one way to ask for none.
func (o Options) bounded() error {
	if o.MaxBytes < 0 {
		return fmt.Errorf("%w: Options.MaxBytes is %d; a bound is positive, or zero for none", ErrInvalid, o.MaxBytes)
	}
	if o.MaxSteps < 0 {
		return fmt.Errorf("%w: Options.MaxSteps is %d; a bound is positive, or zero for none", ErrInvalid, o.MaxSteps)
	}
	return nil
}

// Evaluate runs a plan over a base to its fixpoint, stratum by stratum, semi-naive
// within each: after the first round only the tuples new in the last round are
// joined again. It reads values and writes nothing, and the base is never changed, so
// calls may share one. The deadline on ctx, opts.MaxBytes and opts.MaxSteps bound it.
// A base is accepted when it is compatible with the plan by relation name and arity:
// it must hold every base relation the plan reads, at the plan's arity, and none the plan
// derives; otherwise ErrInvalid. Relations the plan never reads are ignored. Evaluate
// does not run Schema.Checks again, so facts are held to a schema's checks where
// NewBase and Base.With admit them, not here. A context already done fails at once,
// before any join, so the deadline is honoured however small the evaluation.
// A consumer builtin that panics fails the evaluation with ErrInvalid, never a partial
// result; the panic does not escape.
func Evaluate(ctx context.Context, plan Plan, base Base, opts Options) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("tarski: evaluation stopped: %w", err)
	}
	if err := opts.bounded(); err != nil {
		return Result{}, err
	}
	if err := plan.accepts(base); err != nil {
		return Result{}, err
	}
	strata, idb, goal, err := plan.goal(opts.Goal)
	if err != nil {
		return Result{}, err
	}
	ev := &evaluator{ctx: ctx, db: map[string]source{}, opts: opts, builtins: plan.builtins}
	if opts.Proofs {
		ev.proofs = map[string]map[string]*derivation{}
	}
	for name, rel := range base.relations {
		ev.db[name] = source{val: rel}
	}
	for name := range idb {
		ev.db[name] = source{bld: newRelBuilder()}
	}
	if err := ev.seed(plan, goal); err != nil {
		return Result{}, err
	}
	for _, stratum := range strata {
		if err := ev.stratum(stratum); err != nil {
			return Result{}, plan.visible(err)
		}
	}
	// A bound crossed by the last tuple of the last round raises no error inside the
	// join; it is raised here, so MaxBytes is never exceeded with a nil error.
	if ev.over != nil {
		return Result{}, plan.visible(ev.over)
	}
	// A cancellation that landed after the last look at the deadline is honoured
	// here, so a cancelled evaluation never returns a result.
	if err := ev.deadline(); err != nil {
		return Result{}, err
	}
	return Result{relations: ev.sealed(plan), proofs: ev.proofs, groups: ev.groups, plan: plan, goal: goal, steps: ev.steps, bytes: ev.bytes}, nil
}

// seed adds the goal's constants to the seed relation of a plan compiled for a goal,
// charged like any derived tuple, before the first stratum runs. A goal with every
// position open, and a plan compiled for none, have no seed.
func (ev *evaluator) seed(plan Plan, goal *Goal) error {
	if goal == nil {
		return nil
	}
	t, ok := plan.demand.seedTuple(goal)
	if !ok {
		return nil
	}
	ev.db[plan.demand.seed].bld.add(t)
	ev.count(plan.demand.seed, t)
	if ev.over != nil {
		return plan.visible(ev.over)
	}
	return nil
}

// sealed is the evaluation's relations as values: the base's own, and each derived
// relation sealed without indexes, which costs nothing. After this no builder is held.
// A hidden demand relation is left out, so a Result knows no hidden name.
func (ev *evaluator) sealed(plan Plan) map[string]*relation {
	out := make(map[string]*relation, len(ev.db))
	for name, s := range ev.db {
		if plan.demand != nil && plan.demand.hidden[name] != "" {
			continue
		}
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
	// aggPath is the aggregate groups on the join's current path, and groups the group
	// each aggregate-derived tuple was first taken over, by proofKey (aggregate_eval.go).
	// Both stay nil for a plan with no aggregate, so a derivation record is no larger.
	aggPath []usedGroup
	groups  map[string]groupRecord

	opts          Options
	builtins      builtinTable // the plan's, read only
	bytes, tuples int
	over          *TooLarge

	// aggMemo is each aggregate's value per group, computed once in the evaluation
	// (aggregate_eval.go). It stays nil for a plan with no aggregate. witnesses is,
	// beside it and only when proofs are recorded, each group's first maxProofGroup
	// witnesses in witness order, each the inner body's positive tuples in written
	// order. Witness order is the inner join's, relation order over complete
	// relations, so it is a function of (plan, base).
	aggMemo   map[aggKey]aggValue
	witnesses map[aggKey][][]bodyTuple

	// chargeScans is set only for the evaluator a why-not walk uses to compute an
	// aggregate (whynot_aggregate.go): there a relation scanned to find candidates is
	// charged in full, as the walk charges it (scanCost), so the walk's budget bounds
	// the work and not only the matches. Evaluate never sets it.
	chargeScans bool
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
	// Every application costs a step, so a rule with no positive atom (only guards, whose
	// calls no join step pays for) is bounded by MaxSteps like any other.
	if err := ev.step(r.Head.Name); err != nil {
		return err
	}
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
// in the order written. The group an aggregate took it over is kept apart, in groups.
type derivation struct {
	rule *Rule
	body []bodyTuple
}

type bodyTuple struct {
	relation string
	tuple    Tuple
}

// record keeps the derivation that just added t, naming the rule as written: a rule
// rewritten for a goal names its source. apply reaches it only for a tuple
// neither the database nor this round had, so what is kept is the first derivation. A
// body tuple was in the database or the last round's delta, hence derived strictly
// earlier, which is what keeps a proof acyclic.
func (ev *evaluator) record(r *rule, t Tuple) {
	by, ok := ev.proofs[r.Head.Name]
	if !ok {
		by = map[string]*derivation{}
		ev.proofs[r.Head.Name] = by
	}
	src := &r.Rule
	if r.source != nil {
		src = r.source
	}
	d := &derivation{rule: src}
	if len(ev.stack) > 0 {
		d.body = append([]bodyTuple(nil), ev.stack...)
	}
	by[t.key()] = d
	if len(ev.aggPath) > 0 {
		if ev.groups == nil {
			ev.groups = map[string]groupRecord{}
		}
		ev.groups[proofKey(r.Head.Name, t)] = ev.groupOf(r)
	}
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
