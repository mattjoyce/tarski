package tarski

import (
	"fmt"
	"slices"
	"sort"
)

// stratify groups rules into strata so that every negated relation is complete
// before a rule reads it: a relation's stratum is one above the highest it negates,
// and no less than any it depends on positively. A cycle through a negation has no
// such order and is refused.
func stratify(rules []Rule, idb map[string]bool, table builtinTable) ([][]rule, error) {
	deps := dependencies(rules, idb)
	comps := components(deps)
	stratum := map[string]int{}
	for _, comp := range comps { // in reverse topological order: dependencies first
		members := map[string]bool{}
		for _, n := range comp {
			members[n] = true
		}
		level := 0
		for _, n := range comp {
			for _, d := range deps[n] {
				if members[d.to] {
					if d.negative {
						return nil, cycleError(n, d)
					}
					continue
				}
				need := stratum[d.to]
				if d.negative {
					need++
				}
				if need > level {
					level = need
				}
			}
		}
		for _, n := range comp {
			stratum[n] = level
		}
	}
	return group(rules, stratum, idb, table), nil
}

// edge is one dependency of a relation's rules on a derived relation. negative marks a
// relation that must be complete before the rule reads it, which places it in a strictly
// lower stratum: a negated atom, or any atom inside an aggregate's braces, since a count
// over a relation still growing would count a part of it. aggregate tells the two apart
// for the message when a cycle runs through one.
type edge struct {
	to        string
	negative  bool
	aggregate bool
}

// dependencies is the graph from each derived relation to the derived relations its
// rules read, marking an edge through a negation or an aggregate as negative.
func dependencies(rules []Rule, idb map[string]bool) map[string][]edge {
	deps := map[string][]edge{}
	for _, r := range rules {
		if _, ok := deps[r.Head.Name]; !ok {
			deps[r.Head.Name] = nil
		}
		for _, l := range r.Body {
			if l.Atom != nil && idb[l.Atom.Name] {
				deps[r.Head.Name] = append(deps[r.Head.Name], edge{to: l.Atom.Name, negative: l.Negated})
			}
			if l.Aggregate == nil {
				continue
			}
			for _, inner := range l.Aggregate.Body {
				if inner.Atom != nil && idb[inner.Atom.Name] {
					deps[r.Head.Name] = append(deps[r.Head.Name], edge{to: inner.Atom.Name, negative: true, aggregate: true})
				}
			}
		}
	}
	return deps
}

// cycleError refuses a negative edge from n that stays inside n's component: n reads
// d.to through a negation or inside an aggregate, and d.to depends on n.
func cycleError(n string, d edge) error {
	switch {
	case !d.aggregate:
		return fmt.Errorf("%w: %s and %s depend on each other through a negation, so the program cannot be stratified; break the cycle",
			ErrInvalid, n, d.to)
	case n == d.to:
		return fmt.Errorf("%w: %s reads itself inside an aggregate, so the program cannot be stratified; the aggregated relation must be "+
			"complete before it is counted. Aggregate over a relation that %s's rules do not define", ErrInvalid, n, n)
	}
	return fmt.Errorf("%w: %s reads %s inside an aggregate and %s depends on %s, so the program cannot be stratified; the aggregated "+
		"relation must be complete before it is counted. Move the rules that read %s out of %s's definition",
		ErrInvalid, n, d.to, d.to, n, n, d.to)
}

// components finds the strongly connected components of deps (Tarjan), returned
// with every component after the components it depends on. Nodes are visited in
// name order so the result is a function of the program, not of map order.
func components(deps map[string][]edge) [][]string {
	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)

	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var out [][]string
	var visit func(string)
	visit = func(n string) {
		index[n], low[n] = len(index), len(index)
		stack = append(stack, n)
		onStack[n] = true
		for _, d := range deps[n] {
			if _, seen := index[d.to]; !seen {
				visit(d.to)
				low[n] = min(low[n], low[d.to])
			} else if onStack[d.to] {
				low[n] = min(low[n], index[d.to])
			}
		}
		if low[n] != index[n] {
			return
		}
		var comp []string
		for {
			m := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[m] = false
			comp = append(comp, m)
			if m == n {
				break
			}
		}
		sort.Strings(comp)
		out = append(out, comp)
	}
	for _, n := range names {
		if _, seen := index[n]; !seen {
			visit(n)
		}
	}
	return out
}

// group orders each rule's body for evaluation and places it in its head's stratum. An
// aggregate's body is ordered too, on its own with its group variables already bound,
// and kept beside the literal in rule.aggs. An aggregate literal is never a recursive
// position: every relation it reads is in a strictly lower stratum, so it is complete
// before the stratum runs and the aggregate's value cannot change inside it.
func group(rules []Rule, stratum map[string]int, idb map[string]bool, table builtinTable) [][]rule {
	top := 0
	for _, s := range stratum {
		top = max(top, s)
	}
	strata := make([][]rule, top+1)
	for _, r := range rules {
		s := stratum[r.Head.Name]
		groups := aggregateGroups(r)
		pr := rule{Rule: r, ordered: order(r.Body, table, map[string]bool{}, groups)}
		for i, l := range pr.ordered {
			if l.Atom != nil && !l.Negated && idb[l.Atom.Name] && stratum[l.Atom.Name] == s {
				pr.recursive = append(pr.recursive, i)
			}
			if l.Aggregate != nil {
				if pr.aggs == nil {
					pr.aggs = make([]*aggPlan, len(pr.ordered))
				}
				pr.aggs[i] = newAggPlan(r.Head.Name, l.Aggregate, groups[l.Aggregate], table)
			}
		}
		strata[s] = append(strata[s], pr)
	}
	return strata
}

// order places each negation, comparison and built-in right after the positive atom
// that binds its last variable, so a guard runs as soon as it can and never before it
// may. Positive atoms keep the order written. An aggregate is placed like a guard, as
// soon as its group variables (groups) are bound, and once placed it binds its result,
// so a guard waiting on the result follows it. bound holds the variables bound before
// the body starts: none for a rule, the group variables for an aggregate's body.
func order(body []Literal, table builtinTable, bound map[string]bool, groups map[*Aggregate][]string) []Literal {
	var out []Literal
	var pending []Literal
	for _, l := range body {
		if isGuard(l, table) {
			pending = append(pending, l)
		}
	}
	flush := func() {
		for i := 0; i < len(pending); {
			l := pending[i]
			if !ready(l, bound, groups) {
				i++
				continue
			}
			out = append(out, l)
			pending = slices.Delete(pending, i, i+1)
			if l.Aggregate != nil {
				bound[l.Aggregate.Result.Var] = true
				i = 0 // a guard passed over may wait on the result: sweep again, in written order
			}
		}
	}
	flush()
	for _, l := range body {
		if isGuard(l, table) {
			continue
		}
		out = append(out, l)
		for _, a := range l.Atom.Args {
			if a.IsVar() {
				bound[a.Var] = true
			}
		}
		flush()
	}
	return append(out, pending...) // unreachable after checkRule; kept so nothing is dropped
}

// isGuard reports whether order places a literal once what it needs is bound rather
// than where it was written: a negation, a comparison, a built-in or an aggregate.
func isGuard(l Literal, table builtinTable) bool {
	return l.Atom == nil || l.Negated || table.has(l.Atom.Name)
}

// ready reports whether every variable l needs is bound: for an aggregate its group
// variables, for any other literal each named variable it holds.
func ready(l Literal, bound map[string]bool, groups map[*Aggregate][]string) bool {
	var terms []Term
	switch {
	case l.Aggregate != nil:
		for _, v := range groups[l.Aggregate] {
			if !bound[v] {
				return false
			}
		}
		return true
	case l.Atom != nil:
		terms = l.Atom.Args
	default:
		terms = []Term{l.Compare.Left, l.Compare.Right}
	}
	for _, t := range terms {
		if t.IsVar() && t.Var != "_" && !bound[t.Var] {
			return false
		}
	}
	return true
}
