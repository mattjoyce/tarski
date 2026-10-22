package datalog

// This file holds the evaluator's store and its lookups: relation, the set of tuples
// with its indexes, frozen for a base relation and lazily indexed for a derived one,
// and the small functions a join uses to read it: matching, bind, constant and value.
// None of it knows about rules or strata; eval.go drives it.

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
