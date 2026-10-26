package tarski

import "errors"

// This file holds what a plan compiled for a goal carries (docs/design/demand.md §2):
// the rewritten program a demand evaluation runs, and the hidden relations it adds.

// demandPlan is the rewrite of a plan for one shape of goal. hidden maps each hidden
// demand relation to the source relation whose demand it holds, so nothing exported
// names a hidden relation.
type demandPlan struct {
	hidden map[string]string
}

// visible is err with any relation name the rewrite added replaced by the source
// relation it stands for, so a *TooLarge names a relation the caller wrote (design D7).
// For a plan not compiled for a goal it is err unchanged.
func (p Plan) visible(err error) error {
	var tl *TooLarge
	if p.demand == nil || !errors.As(err, &tl) {
		return err
	}
	if src, ok := p.demand.hidden[tl.Relation]; ok {
		tl.Relation = src
	}
	return err
}
