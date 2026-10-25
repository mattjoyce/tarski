package tarski

import "slices"

// Proof is how one tuple came to be. A base fact has Rule nil and no Body. A derived
// tuple has the rule that first produced it and, in the rule's written order, the
// positive body atoms' tuples, each with its own proof. Built-ins and comparisons and
// negations contribute no body entry. The tree is finite and acyclic: a body tuple was
// always derived strictly before its head.
//
// An aggregate contributes no body entry either. Its group is in Group: one entry per
// witness, each the positive atoms inside the braces with their proofs, in written
// order. Witnesses are kept in witness order (relation order, a function of the plan
// and the base) up to 64; GroupSize is the whole group's size, so a cut Group still
// states the count. With several aggregates in a rule their witnesses are
// concatenated in the rule's written order and GroupSize is the sum. Both fields are
// zero for every tuple no aggregate derived, and for an aggregate over an empty group.
//
// It is a DAG shared as values: a tuple that appears in several bodies has one Proof
// value, so the size is linear in the derivation records, not in the unfolded tree.
// Rule is a copy the holder may keep or change; the plan is not reachable through it.
// Within one Result.Proof call a rule used by many tuples is one copy.
type Proof struct {
	Relation string
	Tuple    Tuple
	Rule     *Rule
	Body     []Proof
	// Group is the kept witnesses of the aggregate groups the tuple was taken over.
	Group [][]Proof
	// GroupSize is the number of witnesses in those groups, kept or not.
	GroupSize int
}

// Proof is the proof of t in relation name; ok is false when t is not in the result or
// the evaluation ran without Options.Proofs.
func (r Result) Proof(name string, t Tuple) (p Proof, ok bool) {
	if r.proofs == nil {
		return Proof{}, false
	}
	if rel, found := r.relations[name]; !found || !rel.has(t) {
		return Proof{}, false
	}
	b := &builder{r: r, proofs: map[string]Proof{}, rules: map[*Rule]*Rule{}}
	return b.build(name, t), true
}

// builder holds one Proof call's memos: proofs by (relation, tuple), which makes a
// shared sub-proof one value, and rule copies by source, so a rule is copied once.
type builder struct {
	r      Result
	proofs map[string]Proof
	rules  map[*Rule]*Rule
}

// build makes the proof of a tuple known to be in the result. A tuple with no
// derivation record is a base fact, since every derived tuple has one.
func (b *builder) build(name string, t Tuple) Proof {
	k := proofKey(name, t)
	if p, done := b.proofs[k]; done {
		return p
	}
	p := Proof{Relation: name, Tuple: slices.Clone(t)}
	if d := b.r.proofs[name][t.key()]; d != nil {
		p.Rule = b.rule(d.rule)
		if len(d.body) > 0 {
			p.Body = make([]Proof, len(d.body))
			for i, bt := range d.body {
				p.Body[i] = b.build(bt.relation, bt.tuple)
			}
		}
		if g, ok := b.r.groups[k]; ok {
			p.Group, p.GroupSize = b.group(g.witnesses), g.size
		}
	}
	b.proofs[k] = p
	return p
}

// group builds each kept witness's tuples through the same memo as the body, so a
// tuple shared by witnesses, or by a witness and the body, is one value. Witnesses are
// tuples of strictly lower strata, so the DAG stays acyclic.
func (b *builder) group(ws [][]bodyTuple) [][]Proof {
	if len(ws) == 0 {
		return nil
	}
	out := make([][]Proof, len(ws))
	for i, w := range ws {
		out[i] = make([]Proof, len(w))
		for j, bt := range w {
			out[i][j] = b.build(bt.relation, bt.tuple)
		}
	}
	return out
}

// rule is the copy of src made for this call, so nothing reachable from a proof is
// the plan's.
func (b *builder) rule(src *Rule) *Rule {
	if c, ok := b.rules[src]; ok {
		return c
	}
	c := &Rule{Head: copyAtom(src.Head), Line: src.Line, Text: src.Text, Set: src.Set}
	if src.Body != nil {
		c.Body = make([]Literal, len(src.Body))
		for i, l := range src.Body {
			c.Body[i] = copyLiteral(l)
		}
	}
	b.rules[src] = c
	return c
}

// copyLiteral is a literal that shares nothing with l.
func copyLiteral(l Literal) Literal {
	out := Literal{Negated: l.Negated}
	if l.Atom != nil {
		a := copyAtom(*l.Atom)
		out.Atom = &a
	}
	if l.Compare != nil {
		cmp := *l.Compare // Terms hold no pointers or slices
		out.Compare = &cmp
	}
	if l.Aggregate != nil {
		agg := *l.Aggregate // Result and Target are Terms; only the body needs copying
		if l.Aggregate.Body != nil {
			agg.Body = make([]Literal, len(l.Aggregate.Body))
			for i, inner := range l.Aggregate.Body {
				agg.Body[i] = copyLiteral(inner)
			}
		}
		out.Aggregate = &agg
	}
	return out
}

func copyAtom(a Atom) Atom {
	a.Args = slices.Clone(a.Args)
	return a
}
