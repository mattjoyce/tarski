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
// Rule points into the plan, shared not copied, and is never written through.
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
	return r.build(name, t, map[string]Proof{}), true
}

// build makes the proof of a tuple known to be in the result, once per (relation,
// tuple): memo is what makes a shared sub-proof one value. A tuple with no
// derivation record is a base fact, since every derived tuple has one.
func (r Result) build(name string, t Tuple, memo map[string]Proof) Proof {
	k := t.key()
	if p, done := memo[name+"\x00"+k]; done {
		return p
	}
	p := Proof{Relation: name, Tuple: slices.Clone(t)}
	if d := r.proofs[name][k]; d != nil {
		p.Rule = d.rule
		if len(d.body) > 0 {
			p.Body = make([]Proof, len(d.body))
			for i, b := range d.body {
				p.Body[i] = r.build(b.relation, b.tuple, memo)
			}
		}
	}
	memo[name+"\x00"+k] = p
	return p
}
