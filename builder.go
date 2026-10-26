package tarski

// relBuilder is a relation while it is being made: the place tuples are added. The
// evaluator holds one per derived relation for as long as Evaluate runs, and a Base
// uses one to make each base relation. It indexes lazily, for the joins that read it
// mid-evaluation. Nothing else holds a relBuilder, and none is read after seal: Base and
// Result hold relation values only.
type relBuilder struct {
	tuples []Tuple
	set    map[string]bool
	idx    []builderIndex // by argument position, grown to the highest one asked for
}

// builderIndex is a builder's index at one argument position: the tuple indexes by
// the key of their argument there, over the first n tuples.
type builderIndex struct {
	by map[string][]int
	n  int
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

// candidates is the indexes of the tuples whose argument pos is c. The index at pos is
// built on first use and extended, not rebuilt, when the relation has grown: tuples are
// only appended, so each tuple is indexed once per position. Indexing a derived
// relation therefore costs work in proportion to the tuples it derived, each already
// charged to MaxBytes, however many rounds read it.
func (b *relBuilder) candidates(pos int, c Const) []int {
	for len(b.idx) <= pos {
		b.idx = append(b.idx, builderIndex{})
	}
	x := &b.idx[pos]
	if x.n < len(b.tuples) {
		if x.by == nil {
			x.by = map[string][]int{}
		}
		for i := x.n; i < len(b.tuples); i++ {
			k := b.tuples[i][pos].key()
			x.by[k] = append(x.by[k], i)
		}
		x.n = len(b.tuples)
	}
	return x.by[c.key()]
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

// compact gives a builder whose input held duplicates storage sized to what it kept.
// rebuild sizes the tuple slice and the set from the input count, and group copies a
// relation's facts into one backing array, so without this the duplicates' room stays
// reachable: 300,000 identical facts kept one tuple and 38 MB. The kept tuples are
// copied into one exact array and the set into an exact map. Order is unchanged.
func (b *relBuilder) compact(arity int) {
	backing := make([]Const, 0, len(b.tuples)*arity)
	tuples := make([]Tuple, len(b.tuples))
	for i, t := range b.tuples {
		start := len(backing)
		backing = append(backing, t...)
		tuples[i] = Tuple(backing[start:len(backing):len(backing)])
	}
	set := make(map[string]bool, len(b.set))
	for k := range b.set {
		set[k] = true
	}
	b.tuples, b.set = tuples, set
}
