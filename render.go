package datalog

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// This file renders a Proof and an Absence for people and for receipts. String is an
// indented derivation for a log or an operator; MarshalJSON is the structure a
// consumer stores. Neither shows the parser's AST: a rule is its set, its line and its
// text, a literal is its source form, and a tuple is its constants.

// text is a constant as it is written in a rule: a string quoted, the rest bare.
func (c Const) text() string {
	if c.kind == KindString {
		return strconv.Quote(c.s)
	}
	return c.s
}

func (t Term) text() string {
	if t.IsVar() {
		return t.Var
	}
	return t.Const.text()
}

func termsText(ts []Term) string {
	parts := make([]string, len(ts))
	for i, t := range ts {
		parts[i] = t.text()
	}
	return strings.Join(parts, ", ")
}

func (a Atom) text() string { return a.Name + "(" + termsText(a.Args) + ")" }

// text is the literal as it is written. A zero literal, which a RuleFailure uses for
// "no failing literal found", reads as such.
func (l Literal) text() string {
	switch {
	case l.Compare != nil:
		return l.Compare.Left.text() + " " + l.Compare.Op + " " + l.Compare.Right.text()
	case l.Atom == nil:
		return "(no failing literal found)"
	case l.Negated:
		return "not " + l.Atom.text()
	}
	return l.Atom.text()
}

func tupleText(name string, t Tuple) string {
	terms := make([]Term, len(t))
	for i, c := range t {
		terms[i] = Term{Const: c}
	}
	return name + "(" + termsText(terms) + ")"
}

// ruleRef is where a rule came from: "set:line", or "line N" for a bare program.
func ruleRef(r *Rule) string {
	if r.Set == "" {
		return "line " + strconv.Itoa(r.Line)
	}
	return r.Set + ":" + strconv.Itoa(r.Line)
}

// oneLine is a rule's text on one line, whatever whitespace and comments it was
// written with.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// String is the derivation as an indented tree, one tuple per line, with the rule
// that produced it on the right as "set:line  text", or "fact" for a base fact. A
// tuple shown once is not expanded again: the second time says so. The tree is a
// DAG, and a deep one would otherwise print its shared parts over and over.
func (p Proof) String() string {
	var b strings.Builder
	p.write(&b, 0, map[string]bool{})
	return b.String()
}

func (p Proof) write(b *strings.Builder, depth int, shown map[string]bool) {
	b.WriteString(strings.Repeat("  ", depth))
	b.WriteString(tupleText(p.Relation, p.Tuple))
	key := p.Relation + "\x00" + p.Tuple.key()
	switch {
	case shown[key]:
		b.WriteString("  (shown above)\n")
		return
	case p.Rule == nil:
		b.WriteString("  fact\n")
	default:
		fmt.Fprintf(b, "  %s  %s\n", ruleRef(p.Rule), oneLine(p.Rule.Text))
	}
	shown[key] = true
	for _, sub := range p.Body {
		sub.write(b, depth+1, shown)
	}
}

// String is the absence as an indented explanation: the goal, then for each rule that
// could have derived it the literal it stopped at and the bindings it had, then why
// that literal found nothing, or the proof of what blocked it.
func (a Absence) String() string {
	var b strings.Builder
	a.write(&b, 0)
	return b.String()
}

func (a Absence) write(b *strings.Builder, depth int) {
	pad := strings.Repeat("  ", depth)
	goal := make([]Term, len(a.Goal))
	for i, t := range a.Goal {
		goal[i] = t
		if t.IsVar() {
			goal[i] = Term{Var: "_"}
		}
	}
	fmt.Fprintf(b, "%s%s(%s) is not derived", pad, a.Relation, termsText(goal))
	switch {
	case a.Truncated:
		b.WriteString(" (explanation truncated)\n")
	case len(a.Rules) == 0:
		b.WriteString(": no such fact\n")
	default:
		b.WriteString("\n")
	}
	for _, rf := range a.Rules {
		fmt.Fprintf(b, "%s  %s  %s\n", pad, ruleRef(rf.Rule), oneLine(rf.Rule.Text))
		fmt.Fprintf(b, "%s    stopped at %s%s\n", pad, rf.Failed.text(), boundText(rf.Bound))
		if rf.Blocked != nil {
			fmt.Fprintf(b, "%s    blocked by\n", pad)
			for _, line := range strings.Split(strings.TrimRight(rf.Blocked.String(), "\n"), "\n") {
				fmt.Fprintf(b, "%s      %s\n", pad, line)
			}
		}
		for _, sub := range rf.Because {
			fmt.Fprintf(b, "%s    because\n", pad)
			sub.write(b, depth+3)
		}
	}
}

// boundText is " with A=1, B="x"", variables in name order, or nothing.
func boundText(bound map[string]Const) string {
	if len(bound) == 0 {
		return ""
	}
	names := make([]string, 0, len(bound))
	for n := range bound {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, n := range names {
		parts[i] = n + "=" + bound[n].text()
	}
	return "  with " + strings.Join(parts, ", ")
}

type jsonRule struct {
	Set  string `json:"set"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

func ruleJSON(r *Rule) *jsonRule {
	if r == nil {
		return nil
	}
	return &jsonRule{Set: r.Set, Line: r.Line, Text: r.Text}
}

type jsonProof struct {
	Relation string      `json:"relation"`
	Tuple    []Const     `json:"tuple"`
	Rule     *jsonRule   `json:"rule"`
	Body     []jsonProof `json:"body,omitempty"`
}

func (p Proof) json() jsonProof {
	out := jsonProof{Relation: p.Relation, Tuple: p.Tuple, Rule: ruleJSON(p.Rule)}
	for _, sub := range p.Body {
		out.Body = append(out.Body, sub.json())
	}
	return out
}

// MarshalJSON writes the proof as {relation, tuple, rule: {set, line, text} or null,
// body: [...]}, the body omitted for a base fact. The tree is written out in full.
func (p Proof) MarshalJSON() ([]byte, error) { return json.Marshal(p.json()) }

type jsonFailure struct {
	Rule    *jsonRule        `json:"rule"`
	Bound   map[string]Const `json:"bound"`
	Failed  string           `json:"failed"`
	Because []jsonAbsence    `json:"because,omitempty"`
	Blocked *jsonProof       `json:"blocked,omitempty"`
}

type jsonAbsence struct {
	Relation  string        `json:"relation"`
	Goal      []*Const      `json:"goal"`
	Truncated bool          `json:"truncated"`
	Rules     []jsonFailure `json:"rules"`
}

func (a Absence) json() jsonAbsence {
	out := jsonAbsence{Relation: a.Relation, Truncated: a.Truncated, Goal: make([]*Const, len(a.Goal)), Rules: []jsonFailure{}}
	for i, t := range a.Goal {
		if !t.IsVar() {
			c := t.Const
			out.Goal[i] = &c // an open position stays null
		}
	}
	for _, rf := range a.Rules {
		f := jsonFailure{Rule: ruleJSON(rf.Rule), Bound: rf.Bound, Failed: rf.Failed.text()}
		if f.Bound == nil {
			f.Bound = map[string]Const{}
		}
		for _, sub := range rf.Because {
			f.Because = append(f.Because, sub.json())
		}
		if rf.Blocked != nil {
			p := rf.Blocked.json()
			f.Blocked = &p
		}
		out.Rules = append(out.Rules, f)
	}
	return out
}

// MarshalJSON writes the absence as {relation, goal, truncated, rules: [{rule, bound,
// failed, because, blocked}]}. A goal position the caller left open is null, a failed
// literal is its source text, and because and blocked appear only when there is one.
func (a Absence) MarshalJSON() ([]byte, error) { return json.Marshal(a.json()) }
