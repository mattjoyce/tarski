package tarski

import (
	"fmt"
	"strings"
)

// This file is what a Result of a demand evaluation answers (docs/design/demand.md
// §3.2-3.3, decided §8). Its goal relation is full evaluation's filtered to the goal's
// constants and its base relations are the base's; every other derived relation holds
// only what the goal needed, so reading one is refused rather than answered in part.
// Proofs and why-not are given only inside the goal, where the relations a walk reads are
// complete (design D3).

// admits reports whether t agrees with the goal's constants.
func (g *Goal) admits(t Tuple) bool {
	if len(t) != len(g.Args) {
		return false
	}
	for i, a := range g.Args {
		if !a.IsVar() && !a.Const.same(t[i]) {
			return false
		}
	}
	return true
}

// covers reports whether a question about t in relation name lies inside the goal: the
// goal's relation, and a tuple that agrees with its constants. A Result with no goal
// covers everything.
func (r Result) covers(name string, t Tuple) bool {
	return r.goal == nil || name == r.goal.Relation && r.goal.admits(t)
}

// partial is the refusal of a derived relation other than the goal's on a demand Result,
// or nil when name may be read.
func (r Result) partial(name string) error {
	if r.goal == nil || name == r.goal.Relation || !r.plan.idb[name] {
		return nil
	}
	return fmt.Errorf("%w: the result was evaluated for the goal %s; %s is partial under that goal; "+
		"evaluate without a goal, or compile a plan for %s with Plan.Demand, to read it",
		ErrInvalid, Atom{Name: r.goal.Relation, Args: r.goal.Args}.text(), name, name)
}

// openGoal reports whether WhyEmpty may explain name: always without a goal, and on a
// demand Result only for the goal's relation with every position open, since a goal
// that binds a position is partial by design and has no "empty".
func (r Result) openGoal(name string) bool {
	if r.goal == nil {
		return true
	}
	return name == r.goal.Relation && !strings.Contains(r.plan.demand.adornment, "b")
}
