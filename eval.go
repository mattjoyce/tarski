package datalog

import (
	"context"
	"errors"
	"fmt"
)

// ErrTooLarge marks an evaluation refused for its size: the tuples it derived encode
// to more than the bound. A more selective rule or a smaller base is the remedy;
// asking again unchanged will fail again.
var ErrTooLarge = errors.New("datalog: derivation too large")

// TooLarge is the refusal itself: the relation that crossed the bound, and the
// measure, counted over every derived relation as the tuples' text.
type TooLarge struct {
	Relation string
	Tuples   int
	Bytes    int
	MaxBytes int
}

func (e *TooLarge) Error() string {
	return fmt.Sprintf("derivation too large: %d derived tuples encode to more than %d bytes (Options.MaxBytes), at %s",
		e.Tuples, e.MaxBytes, e.Relation)
}

func (e *TooLarge) Is(target error) bool { return target == ErrTooLarge }

// relation is a set of tuples with insertion order and indexes, one per argument
// position. A derived relation builds an index lazily, the first time a join asks for
// one. A base relation is frozen when its Base is built: every index exists then and
// nothing writes to it afterwards, so evaluations running at once may share it.
type relation struct {
	tuples []Tuple
	set    map[string]bool
	idx    map[int]map[string][]int
	idxLen int
	frozen bool
	arity  int // set when frozen
}

func newRelation() *relation {
	return &relation{set: map[string]bool{}}
}

// freeze builds the index for every argument position and marks the relation
// read-only. Adding to a frozen relation is a programming error, so add panics on it.
func (r *relation) freeze(arity int) {
	r.arity = arity
	r.idx, r.idxLen = map[int]map[string][]int{}, len(r.tuples)
	for pos := 0; pos < arity; pos++ {
		r.idx[pos] = r.index(pos)
	}
	r.frozen = true
}

// index groups the tuples' positions by the key of their argument at pos.
func (r *relation) index(pos int) map[string][]int {
	by := map[string][]int{}
	for i, t := range r.tuples {
		k := t[pos].key()
		by[k] = append(by[k], i)
	}
	return by
}

// add puts t in the relation and reports whether it was new.
func (r *relation) add(t Tuple) bool {
	if r.frozen {
		panic("datalog: add to a frozen relation")
	}
	k := t.key()
	if r.set[k] {
		return false
	}
	r.set[k] = true
	r.tuples = append(r.tuples, t)
	return true
}

func (r *relation) has(t Tuple) bool { return r.set[t.key()] }

// candidates is the indexes of the tuples whose argument pos is c. A derived
// relation's index is built on first use and dropped when the relation grows. A
// frozen relation's indexes all exist already and are only read here.
func (r *relation) candidates(pos int, c Const) []int {
	if r.frozen {
		return r.idx[pos][c.key()]
	}
	if r.idx == nil || r.idxLen != len(r.tuples) {
		r.idx, r.idxLen = map[int]map[string][]int{}, len(r.tuples)
	}
	by, ok := r.idx[pos]
	if !ok {
		by = r.index(pos)
		r.idx[pos] = by
	}
	return by[c.key()]
}

// Result is what an evaluation derived: every relation, base and derived, at the
// fixpoint.
type Result struct {
	relations map[string]*relation
}

// Options bounds and shapes one evaluation.
type Options struct {
	// MaxBytes is a bound over every derived tuple's JSON text, intermediate relations
	// included, since derived relations can be larger than the input. Zero is no
	// bound; over it, Evaluate returns *TooLarge.
	MaxBytes int
	// Proofs records, for each derived tuple, the first derivation that produced it.
	// It touches the hot loop, so it is off unless asked for.
	Proofs bool
}

// Evaluate runs a plan over a base to its fixpoint, stratum by stratum, semi-naive
// within each: after the first round only the tuples new in the last round are
// joined again. It reads values and writes nothing, and the base is never changed, so
// calls may share one. The deadline on ctx and opts.MaxBytes bound it. A base built
// under a schema other than the plan's is ErrInvalid.
func Evaluate(ctx context.Context, plan Plan, base Base, opts Options) (Result, error) {
	if err := plan.accepts(base); err != nil {
		return Result{}, err
	}
	ev := &evaluator{ctx: ctx, db: map[string]*relation{}, opts: opts}
	for name, rel := range base.relations {
		ev.db[name] = rel
	}
	for name := range plan.idb {
		ev.db[name] = newRelation()
	}
	for _, stratum := range plan.strata {
		if err := ev.stratum(stratum); err != nil {
			return Result{}, err
		}
	}
	return Result{relations: ev.db}, nil
}

type evaluator struct {
	ctx   context.Context
	db    map[string]*relation
	steps int

	opts          Options
	bytes, tuples int
	over          *TooLarge
}

// stratum evaluates one stratum's rules to a fixpoint.
func (ev *evaluator) stratum(rules []rule) error {
	delta := map[string]*relation{}
	for _, r := range rules {
		if err := ev.apply(r, nil, -1, delta); err != nil {
			return err
		}
		if ev.over != nil {
			return ev.over
		}
	}
	for len(delta) > 0 {
		ev.merge(delta)
		next := map[string]*relation{}
		for _, r := range rules {
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

func (ev *evaluator) merge(delta map[string]*relation) {
	for name, rel := range delta {
		for _, t := range rel.tuples {
			ev.db[name].add(t)
		}
	}
}

// apply derives r's head once over the database, with the positive atom at deltaPos
// (if any) read from delta instead, and collects tuples not yet derived into out.
func (ev *evaluator) apply(r rule, delta *relation, deltaPos int, out map[string]*relation) error {
	env := map[string]Const{}
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
			rel = newRelation()
			out[r.Head.Name] = rel
		}
		if rel.add(t) {
			ev.count(r.Head.Name, t)
		}
	})
}

// count charges one derived tuple against the bound, as the bytes its JSON takes.
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
		ev.over = &TooLarge{Relation: relation, Tuples: ev.tuples, Bytes: ev.bytes, MaxBytes: ev.opts.MaxBytes}
	}
}

// join extends env through the body literals from i on, calling emit for every
// complete binding.
func (ev *evaluator) join(r rule, i int, env map[string]Const, delta *relation, deltaPos int, emit func()) error {
	if i == len(r.ordered) {
		emit()
		return nil
	}
	l := r.ordered[i]
	switch {
	case l.Compare != nil || isBuiltin(l.Atom.Name):
		if ev.guardHolds(l, env) {
			return ev.join(r, i+1, env, delta, deltaPos, emit)
		}
		return nil
	case l.Negated:
		if ev.exists(ev.db[l.Atom.Name], l.Atom.Args, env) {
			return nil
		}
		return ev.join(r, i+1, env, delta, deltaPos, emit)
	}
	rel := ev.db[l.Atom.Name]
	if i == deltaPos {
		rel = delta
	}
	for _, idx := range matching(rel, l.Atom.Args, env) {
		if ev.over != nil {
			return ev.over
		}
		if ev.steps++; ev.steps%stepsPerDeadlineCheck == 0 {
			if err := ev.ctx.Err(); err != nil {
				return fmt.Errorf("datalog: evaluation stopped: %w", err)
			}
		}
		bound, ok := bind(rel.tuples[idx], l.Atom.Args, env)
		if !ok {
			continue
		}
		if err := ev.join(r, i+1, env, delta, deltaPos, emit); err != nil {
			return err
		}
		for _, v := range bound {
			delete(env, v)
		}
	}
	return nil
}

// stepsPerDeadlineCheck is how many tuples a join visits between looks at the
// deadline. It changes no result and no limit a person meets: only how promptly a
// cancelled evaluation stops, against the cost of asking.
const stepsPerDeadlineCheck = 1024

// exists reports whether some tuple of rel matches args under env.
func (ev *evaluator) exists(rel *relation, args []Term, env map[string]Const) bool {
	for _, idx := range matching(rel, args, env) {
		if bound, ok := bind(rel.tuples[idx], args, env); ok {
			for _, v := range bound {
				delete(env, v)
			}
			return true
		}
	}
	return false
}

// matching is the tuples of rel worth trying against args under env: the ones an
// index on the first bound argument selects, or all of them when none is bound.
func matching(rel *relation, args []Term, env map[string]Const) []int {
	for pos, a := range args {
		if c, ok := constant(a, env); ok {
			return rel.candidates(pos, c)
		}
	}
	all := make([]int, len(rel.tuples))
	for i := range all {
		all[i] = i
	}
	return all
}

// bind unifies t with args, extending env with the variables it binds and
// returning them so the caller can undo it. ok is false on a mismatch; env is
// then unchanged.
func bind(t Tuple, args []Term, env map[string]Const) (bound []string, ok bool) {
	for i, a := range args {
		if a.Var == "_" {
			continue
		}
		if c, isBound := constant(a, env); isBound {
			if c.key() != t[i].key() {
				for _, v := range bound {
					delete(env, v)
				}
				return nil, false
			}
			continue
		}
		env[a.Var] = t[i]
		bound = append(bound, a.Var)
	}
	return bound, true
}

// constant is the term's value under env: its constant, or the binding of its
// variable. ok is false for an unbound variable.
func constant(a Term, env map[string]Const) (Const, bool) {
	if !a.IsVar() {
		return a.Const, true
	}
	c, ok := env[a.Var]
	return c, ok
}

func value(a Term, env map[string]Const) Const {
	c, _ := constant(a, env)
	return c
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
	default:
		return ok && cmp >= 0
	}
}

// guardHolds evaluates a comparison or a built-in literal under env.
func (ev *evaluator) guardHolds(l Literal, env map[string]Const) bool {
	if l.Compare != nil {
		return holds(*l.Compare, env)
	}
	return holdsBuiltin(l, env)
}
