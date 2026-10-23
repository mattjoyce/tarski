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

// ruleLine is "ref  text" for a rule, or a marker for a RuleFailure built by hand with
// no rule in it, which the engine never produces.
func ruleLine(r *Rule) string {
	if r == nil {
		return "(no rule)"
	}
	return ruleRef(r) + "  " + oneLine(r.Text)
}

// oneLine is a rule's text on one line: a run of whitespace between tokens becomes one
// space and a comment is dropped, while a quoted string is kept as written, since a
// constant's whitespace is part of the constant.
func oneLine(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	space := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++
				}
				j++
			}
			if j < len(s) {
				j++ // the closing quote; an unterminated string runs to the end
			}
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(s[i:j])
			space, i = false, j-1
		case '%':
			for i+1 < len(s) && s[i+1] != '\n' {
				i++
			}
		case ' ', '\t', '\n', '\r':
			space = true
		default:
			if space && b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteByte(c)
			space = false
		}
	}
	return b.String()
}

// renderDepth bounds how deep a rendering follows Because. An engine-made Absence nests
// at most as deep as WhyNot was asked to, far below this; the bound is so that a value
// a caller built by hand with a cycle in it ends, as "(explanation truncated)".
const renderDepth = 1024

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
		fmt.Fprintf(b, "  %s\n", ruleLine(p.Rule))
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
	if depth > renderDepth {
		fmt.Fprintf(b, "%s(explanation truncated)\n", pad)
		return
	}
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
		fmt.Fprintf(b, "%s  %s\n", pad, ruleLine(rf.Rule))
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
	Shown    bool        `json:"shown,omitempty"`
}

// json writes the proof once per (relation, tuple): a sub-proof already in the
// document is a reference, as in String. The engine's proof is a DAG whose shared
// parts can make the unfolded tree exponential in the derivation; the document stays
// linear in it.
func (p Proof) json(shown map[string]bool) jsonProof {
	out := jsonProof{Relation: p.Relation, Tuple: p.Tuple}
	key := p.Relation + "\x00" + p.Tuple.key()
	if shown[key] {
		out.Shown = true
		return out
	}
	shown[key] = true
	out.Rule = ruleJSON(p.Rule)
	for _, sub := range p.Body {
		out.Body = append(out.Body, sub.json(shown))
	}
	return out
}

// MarshalJSON writes the proof as {relation, tuple, rule: {set, line, text} or null,
// body: [...]}, the body omitted for a base fact. A tuple proved earlier in the same
// document is not written out again: it is {relation, tuple, rule: null, shown: true},
// and its proof is the first one in the document with that relation and tuple, so the
// document is linear in the derivation however much of it is shared.
func (p Proof) MarshalJSON() ([]byte, error) { return json.Marshal(p.json(map[string]bool{})) }

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

func (a Absence) json(depth int) jsonAbsence {
	out := jsonAbsence{Relation: a.Relation, Truncated: a.Truncated, Goal: make([]*Const, len(a.Goal)), Rules: []jsonFailure{}}
	if depth > renderDepth {
		out.Truncated = true
		return out
	}
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
			f.Because = append(f.Because, sub.json(depth+1))
		}
		if rf.Blocked != nil {
			p := rf.Blocked.json(map[string]bool{})
			f.Blocked = &p
		}
		out.Rules = append(out.Rules, f)
	}
	return out
}

// MarshalJSON writes the absence as {relation, goal, truncated, rules: [{rule, bound,
// failed, because, blocked}]}. A goal position the caller left open is null, a failed
// literal is its source text, and because and blocked appear only when there is one.
// A blocked proof is written as Proof.MarshalJSON writes one.
func (a Absence) MarshalJSON() ([]byte, error) { return json.Marshal(a.json(0)) }
