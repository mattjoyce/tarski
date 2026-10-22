package datalog

// Proof is how one tuple came to be. A base fact has Rule nil and no Body. A derived
// tuple has the rule that first produced it and, in the rule's written order, the
// positive body atoms' tuples, each with its own proof. Built-ins and comparisons and
// negations contribute no body entry. The tree is finite and acyclic: a body tuple was
// always derived strictly before its head.
type Proof struct {
	Relation string
	Tuple    Tuple
	Rule     *Rule
	Body     []Proof
}

// Proof is the proof of t in relation name; ok is false when t is not in the result or
// the evaluation ran without Options.Proofs.
//
// Pending: Evaluate accepts Options.Proofs but does not record derivations yet, so this
// returns false for every tuple. The recording belongs in the evaluator's apply, where
// a head tuple is first added.
func (r Result) Proof(name string, t Tuple) (p Proof, ok bool) {
	return Proof{}, false
}
