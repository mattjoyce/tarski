package tarski

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// This file is the API of demand-driven evaluation (docs/design/demand.md §3): a plan
// compiled for one shape of goal, the goal each evaluation of it answers, and the
// hidden relations the rewrite adds, which nothing exported names. The rewrite itself
// is demand_rewrite.go; what a demand Result will and will not answer is
// demand_result.go.

// Goal is the question a demand evaluation answers: a relation and, per position, a
// constant to hold the position to or a variable to leave it open. Any variable name
// leaves a position open, "_" included, but no name other than "_" may appear twice:
// a goal cannot ask for two positions to be equal.
type Goal struct {
	Relation string
	Args     []Term
}

// demandPlan is the rewrite of a plan for one shape of goal: the relation and its
// adornment (a b where the goal binds a position, an f where it leaves it open), the
// hidden relation the goal's constants seed, and the rewritten program's strata and
// derived relations. hidden maps each hidden demand relation to the source relation
// whose demand it holds.
type demandPlan struct {
	relation  string
	adornment string
	seed      string
	strata    [][]rule
	idb       map[string]bool
	hidden    map[string]string
}

// Demand compiles the plan to answer one shape of question: goals over relation whose
// bound positions are the b's of adornment, a string of b and f as long as the
// relation's arity ("bbf": the first two positions bound, the third open). The constants
// come later, per evaluation, in Options.Goal. The plan returned evaluates to exactly
// what the receiver would for the goal's relation, filtered to the goal's constants,
// and derives only what can reach the goal; it is a value, shareable as any plan is.
//
// ErrInvalid for an unknown or base relation, a malformed adornment, or a plan already
// compiled for a goal. Two kinds of plan are not yet supported and are refused with
// ErrInvalid saying so: a plan with aggregates, and one whose rewrite cannot be
// stratified.
func (p Plan) Demand(relation, adornment string) (Plan, error) {
	if p.demand != nil {
		return Plan{}, fmt.Errorf("%w: the plan is already compiled for goals over %s; call Demand on the plan Compile returned",
			ErrInvalid, p.demand.relation)
	}
	arity, ok := p.arity[relation]
	switch {
	case !ok:
		return Plan{}, fmt.Errorf("%w: Demand: unknown relation %s; %s", ErrInvalid, relation, known(p.arity))
	case !p.idb[relation]:
		return Plan{}, fmt.Errorf("%w: Demand: %s is a base relation, which evaluation reads whole; demand a relation the rules derive",
			ErrInvalid, relation)
	case len(adornment) != arity || strings.Trim(adornment, "bf") != "":
		return Plan{}, fmt.Errorf("%w: Demand: adornment %q for %s/%d: write one b (bound) or f (open) per position",
			ErrInvalid, adornment, relation, arity)
	case p.HasAggregates():
		return Plan{}, fmt.Errorf("%w: Demand: demand through aggregates is not yet supported; evaluate the plan without a goal",
			ErrInvalid)
	}
	d, err := p.rewrite(relation, adornment)
	if err != nil {
		return Plan{}, err
	}
	p.demand = d
	return p, nil
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

// goal holds an evaluation's Options.Goal to the plan: required on a plan compiled for a
// goal and refused on any other. It returns the plan's strata and derived relations for
// the evaluation, and a copy of the goal for the Result.
func (p Plan) goal(g *Goal) (strata [][]rule, idb map[string]bool, own *Goal, err error) {
	if p.demand == nil {
		if g != nil {
			return nil, nil, nil, fmt.Errorf("%w: Options.Goal is set, but the plan was not compiled for a goal; "+
				"compile the plan for the goal with Plan.Demand", ErrInvalid)
		}
		return p.strata, p.idb, nil, nil
	}
	if g == nil {
		return nil, nil, nil, fmt.Errorf("%w: the plan was compiled for goals over %s (Plan.Demand %q); set Options.Goal",
			ErrInvalid, p.demand.relation, p.demand.adornment)
	}
	if err := p.demand.check(*g); err != nil {
		return nil, nil, nil, err
	}
	return p.demand.strata, p.demand.idb, &Goal{Relation: g.Relation, Args: slices.Clone(g.Args)}, nil
}

// check holds a goal to the plan's shape: its relation, its arity, a constant at every
// bound position and a variable at every open one, no variable but _ twice (an
// adornment cannot express an equality, and silently ignoring one would be a wrong
// answer), and strings that are valid UTF-8, the engine's one string domain.
func (d *demandPlan) check(g Goal) error {
	shape := fmt.Sprintf("the plan answers goals over %s with adornment %q", d.relation, d.adornment)
	if g.Relation != d.relation || len(g.Args) != len(d.adornment) {
		return fmt.Errorf("%w: Options.Goal is %s/%d, but %s", ErrInvalid, g.Relation, len(g.Args), shape)
	}
	seen := map[string]bool{}
	for i, a := range g.Args {
		switch {
		case d.adornment[i] == 'b' && a.IsVar():
			return fmt.Errorf("%w: Options.Goal leaves position %d open, but %s; give it a constant", ErrInvalid, i+1, shape)
		case d.adornment[i] == 'f' && !a.IsVar():
			return fmt.Errorf("%w: Options.Goal binds position %d, but %s; leave it open with a variable", ErrInvalid, i+1, shape)
		case a.IsVar() && a.Var != "_" && seen[a.Var]:
			return fmt.Errorf("%w: Options.Goal repeats the variable %s; a goal cannot ask for two positions to be equal, "+
				"so write the equality in a rule and ask for that rule's relation", ErrInvalid, a.Var)
		case !a.IsVar() && a.Const.kind == KindString && !utf8.ValidString(a.Const.s):
			return fmt.Errorf("%w: Options.Goal has a string at position %d that is not valid UTF-8", ErrInvalid, i+1)
		}
		seen[a.Var] = true
	}
	return nil
}

// seedTuple is the goal's constants at its bound positions: the one tuple of the seed
// relation, added before the first stratum runs. ok is false for a goal with every
// position open, which has no seed: its relation is derived whole.
func (d *demandPlan) seedTuple(g *Goal) (t Tuple, ok bool) {
	for i, a := range g.Args {
		if d.adornment[i] == 'b' {
			t = append(t, a.Const)
		}
	}
	return t, len(t) > 0
}

// Goal is the goal the result was evaluated for; ok is false for a result of a plan not
// compiled for a goal. The Args are a copy the caller may change.
func (r Result) Goal() (g Goal, ok bool) {
	if r.goal == nil {
		return Goal{}, false
	}
	return Goal{Relation: r.goal.Relation, Args: slices.Clone(r.goal.Args)}, true
}
