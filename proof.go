package datalog

import "slices"

// Proof is how one tuple came to be. A base fact has Rule nil and no Body. A derived
// tuple has the rule that first produced it and, in the rule's written order, the
// positive body atoms' tuples, each with its own proof. Built-ins and comparisons and
// negations contribute no body entry. The tree is finite and acyclic: a body tuple was
// always derived strictly before its head.
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
	k := name + "\x00" + t.key()
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
	}
	b.proofs[k] = p
	return p
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
			c.Body[i] = Literal{Negated: l.Negated}
			if l.Atom != nil {
				a := copyAtom(*l.Atom)
				c.Body[i].Atom = &a
			}
			if l.Compare != nil {
				cmp := *l.Compare // Terms hold no pointers or slices
				c.Body[i].Compare = &cmp
			}
		}
	}
	b.rules[src] = c
	return c
}

func copyAtom(a Atom) Atom {
	a.Args = slices.Clone(a.Args)
	return a
}
