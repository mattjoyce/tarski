package datalog

// relBuilder is a relation while it is being made: the place tuples are added. The
// evaluator holds one per derived relation for as long as Evaluate runs, and a Base
// uses one to make each base relation. It indexes lazily, for the joins that read it
// mid-evaluation. Nothing else holds a relBuilder, and none is read after seal: Base and
// Result hold relation values only.
type relBuilder struct {
	tuples []Tuple
	set    map[string]bool
	idx    map[int]map[string][]int
	idxLen int
}

func newRelBuilder() *relBuilder {
	return &relBuilder{set: map[string]bool{}}
}

// add puts t in the relation and reports whether it was new.
func (b *relBuilder) add(t Tuple) bool {
	k := t.key()
	if b.set[k] {
		return false
	}
	b.set[k] = true
	b.tuples = append(b.tuples, t)
	return true
}

func (b *relBuilder) has(t Tuple) bool { return b.set[t.key()] }

// candidates is the indexes of the tuples whose argument pos is c. The index is built
// on first use and dropped when the relation grows.
func (b *relBuilder) candidates(pos int, c Const) []int {
	if b.idx == nil || b.idxLen != len(b.tuples) {
		b.idx, b.idxLen = map[int]map[string][]int{}, len(b.tuples)
	}
	by, ok := b.idx[pos]
	if !ok {
		by = indexAt(b.tuples, pos)
		b.idx[pos] = by
	}
	return by[c.key()]
}

// seal is the relation value the builder has made, and the builder must not be used
// again: the value takes its tuples and set as they are. With indexes it has one per
// argument position, which costs a pass over the tuples for each; without, it costs
// nothing, and the value scans.
func (b *relBuilder) seal(arity int, withIndexes bool) *relation {
	r := &relation{tuples: b.tuples, set: b.set, arity: arity}
	if withIndexes {
		r.idx = make(map[int]map[string][]int, arity)
		for pos := 0; pos < arity; pos++ {
			r.idx[pos] = indexAt(r.tuples, pos)
		}
	}
	*b = relBuilder{}
	return r
}

// indexAt groups the tuples' positions by the key of their argument at pos.
func indexAt(tuples []Tuple, pos int) map[string][]int {
	by := map[string][]int{}
	for i, t := range tuples {
		k := t[pos].key()
		by[k] = append(by[k], i)
	}
	return by
}
