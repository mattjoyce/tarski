package tarski

import (
	"strconv"
	"strings"
)

// unifyHead unifies the rule's head with the goal. env binds the head variables the
// goal fixes; alias maps a head variable to another the goal equates it with (a goal
// p(X, X) against a head p(A, B) makes B an alias of A), so the walk can hold them to
// one value. ok is false when the head and goal cannot be one atom: two different
// constants, a variable the goal would bind two ways, or two goal positions the goal
// equates that the head holds to different constants.
func unifyHead(head, goal []Term) (env map[string]Const, alias map[string]string, ok bool) {
	u := unifier{parent: map[string]string{}, val: map[string]Const{}}
	for i, h := range head {
		g := goal[i]
		hv, gv := h.IsVar() && h.Var != "_", g.IsVar() && g.Var != "_"
		switch {
		case !h.IsVar() && !g.IsVar():
			if !g.Const.same(h.Const) {
				return nil, nil, false
			}
		case !h.IsVar() && gv:
			if !u.set("g"+g.Var, h.Const) {
				return nil, nil, false
			}
		case hv && !g.IsVar():
			if !u.set("h"+h.Var, g.Const) {
				return nil, nil, false
			}
		case hv && gv:
			if !u.union("h"+h.Var, "g"+g.Var) {
				return nil, nil, false
			}
		}
		// a head "_" binds nothing; a goal "_" is a fresh open position
	}
	env, alias = map[string]Const{}, map[string]string{}
	canon := map[string]string{} // class root -> first head variable in it
	for _, h := range head {
		if !h.IsVar() || h.Var == "_" {
			continue
		}
		root := u.find("h" + h.Var)
		if c, bound := u.val[root]; bound {
			env[h.Var] = c
			continue
		}
		if first, seen := canon[root]; seen {
			if first != h.Var {
				alias[h.Var] = first
			}
			continue
		}
		canon[root] = h.Var
	}
	return env, alias, true
}

// unifier is a union-find over variable names, each class holding at most one constant.
type unifier struct {
	parent map[string]string
	val    map[string]Const
}

func (u unifier) find(v string) string {
	for {
		p, ok := u.parent[v]
		if !ok || p == v {
			return v
		}
		v = p
	}
}

// set binds v's class to c; false when it already holds a different constant.
func (u unifier) set(v string, c Const) bool {
	root := u.find(v)
	if have, ok := u.val[root]; ok {
		return have.same(c)
	}
	u.val[root] = c
	return true
}

// union merges the classes of a and b; false when they hold different constants.
func (u unifier) union(a, b string) bool {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return true
	}
	ca, oka := u.val[ra]
	cb, okb := u.val[rb]
	if oka && okb && !ca.same(cb) {
		return false
	}
	u.parent[rb] = ra
	if okb && !oka {
		u.val[ra] = cb
	}
	return true
}

// renamed is body with every variable alias maps replaced by its canonical name.
func renamed(body []Literal, alias map[string]string) []Literal {
	out := make([]Literal, len(body))
	for i, l := range body {
		out[i] = copyLiteral(l)
		if out[i].Atom != nil {
			renameTerms(out[i].Atom.Args, alias)
		}
		if out[i].Compare != nil {
			pair := []Term{out[i].Compare.Left, out[i].Compare.Right}
			renameTerms(pair, alias)
			out[i].Compare.Left, out[i].Compare.Right = pair[0], pair[1]
		}
	}
	return out
}

func renameTerms(ts []Term, alias map[string]string) {
	for i, t := range ts {
		if canon, ok := alias[t.Var]; ok {
			ts[i].Var = canon
		}
	}
}

// substitute is args with the variables env binds replaced by their constants. A
// variable env does not bind stays a variable: the goal leaves that position open.
func substitute(args []Term, env map[string]Const) []Term {
	out := make([]Term, len(args))
	for i, a := range args {
		out[i] = a
		if a.IsVar() && a.Var != "_" {
			if c, ok := env[a.Var]; ok {
				out[i] = Term{Const: c}
			}
		}
	}
	return out
}

// goalKey identifies a goal by relation, constants, and which open positions are the
// same variable: p(X, X) and p(X, Y) are different goals, p(X, Y) and p(A, B) the same.
// A variable is named by the position it first appears at; "_" is fresh each time.
func goalKey(name string, goal []Term) string {
	var b strings.Builder
	b.WriteString(name)
	first := map[string]int{}
	for i, t := range goal {
		b.WriteByte(0)
		if !t.IsVar() {
			t.Const.writeKey(&b)
			continue
		}
		at := i
		if t.Var != "_" {
			if j, seen := first[t.Var]; seen {
				at = j
			} else {
				first[t.Var] = i
			}
		}
		b.WriteByte('?')
		b.WriteString(strconv.Itoa(at))
	}
	return b.String()
}
