package datalog

// This file holds the evaluator's values and the lookups a join makes over them:
// relation, a set of tuples that never changes, source, what a join reads, and the
// small functions that match a tuple against a literal: matching, bind, constant and
// value. The place where tuples are added is builder.go. None of it knows about rules or
// strata; eval.go drives it.

// relation is a set of tuples with their insertion order: a value. It has read methods
// and no add, so a relation cannot change under a reader and evaluations may share one.
// A base relation is sealed with an index for every argument position; a derived
// relation is sealed without any, and answers candidates by scanning. Only a
// relBuilder makes one, and it does so by giving up its own tuples.
type relation struct {
	tuples []Tuple
	set    map[string]bool
	idx    map[int]map[string][]int // nil for a relation sealed without indexes
	arity  int
}

func (r *relation) has(t Tuple) bool { return r.set[t.key()] }

func (r *relation) len() int { return len(r.tuples) }

// candidates is the indexes of the tuples whose argument pos is c: a lookup in the
// index when the relation has them, a scan when it does not.
func (r *relation) candidates(pos int, c Const) []int {
	if r.idx != nil {
		return r.idx[pos][c.key()]
	}
	var out []int
	for i, t := range r.tuples {
		if t[pos].same(c) {
			out = append(out, i)
		}
	}
	return out
}

// source is what a join reads: a sealed relation (a base relation) or a relBuilder
// (a derived relation, still growing). Exactly one is set. It is a small value, two
// pointers, so the join needs no interface and no second copy of its loop.
type source struct {
	val *relation
	bld *relBuilder
}

func (s source) tuples() []Tuple {
	if s.val != nil {
		return s.val.tuples
	}
	return s.bld.tuples
}

func (s source) has(t Tuple) bool {
	if s.val != nil {
		return s.val.has(t)
	}
	return s.bld.has(t)
}

func (s source) candidates(pos int, c Const) []int {
	if s.val != nil {
		return s.val.candidates(pos, c)
	}
	return s.bld.candidates(pos, c)
}

// matching is the tuples of rel worth trying against args under env: the ones an
// index on the first bound argument selects, or all of them when none is bound.
func matching(rel source, args []Term, env map[string]Const) []int {
	for pos, a := range args {
		if c, ok := constant(a, env); ok {
			return rel.candidates(pos, c)
		}
	}
	all := make([]int, len(rel.tuples()))
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
			if !c.same(t[i]) {
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
