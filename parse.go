package datalog

import (
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
)

// ErrSyntax is returned for text that is not a program.
var ErrSyntax = errors.New("datalog: syntax")

// Term is one argument of an atom or one side of a comparison: a variable, the
// anonymous variable, or a constant.
type Term struct {
	Var   string // "" for a constant; "_" is the anonymous variable
	Const Const
}

// IsVar reports whether the term is a variable, the anonymous one included.
func (t Term) IsVar() bool { return t.Var != "" }

// Atom is a relation applied to terms.
type Atom struct {
	Name string
	Args []Term
	Line int
}

// Comparison is a guard between two terms: =, !=, <, <=, > or >=.
type Comparison struct {
	Op          string
	Left, Right Term
	Line        int
}

// Literal is one body element: a positive atom, a negated atom, or a comparison.
type Literal struct {
	Atom    *Atom
	Negated bool
	Compare *Comparison
}

// Rule is head :- body. A fact is a rule with no body.
type Rule struct {
	Head Atom
	Body []Literal
	Line int
	// Text is the rule as written, its final period included and its surrounding
	// whitespace trimmed. A comment between its tokens stays in it. A rule built by hand
	// has none.
	Text string
	// Set is the rule set the rule came from, "name/version" as RuleSet.ID leads with,
	// so a proof can say which of several loaded sets a rule belongs to. It is empty for
	// a rule from a bare Program. Parse leaves it empty; Compile fills it from the
	// Program's Name.
	Set string
}

// Program is parsed text: rules, and the relations declared without rules
// (name/arity.), which a later program defines.
type Program struct {
	Rules    []Rule
	Declared map[string]int
	Text     string
	// Name is the rule set the program belongs to, "name/version", which NewRuleSet sets.
	// Parse leaves it empty.
	Name string
}

// Parse reads a program. The syntax is small:
//
//	% a comment runs to the end of the line
//	name/arity.                                  declares a relation another program defines
//	head(X, "c", 1).                             a fact
//	head(X, Y) :- rel(X, Z), not other(Z), Y > 3.  a rule
//
// A relation name is lower-case; a variable starts upper-case; _ is anonymous.
// Constants are double-quoted strings, numbers, true and false.
func Parse(text string) (Program, error) {
	lx := &lexer{src: text, line: 1}
	toks, err := lx.all()
	if err != nil {
		return Program{}, err
	}
	p := &parser{src: text, toks: toks, prog: Program{Declared: map[string]int{}, Text: text}}
	for !p.at(tokEOF) {
		if err := p.statement(); err != nil {
			return Program{}, err
		}
	}
	return p.prog, nil
}

// comparisonOps are the guards a body may hold.
var comparisonOps = map[string]bool{"=": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true}

type parser struct {
	toks []token
	src  string
	i    int
	prog Program
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) at(k tokKind, text ...string) bool {
	t := p.peek()
	return t.kind == k && (len(text) == 0 || t.text == text[0])
}

func (p *parser) take() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) expect(text string) error {
	if !p.at(tokPunct, text) {
		return p.fail("expected %q", text)
	}
	p.take()
	return nil
}

func (p *parser) fail(format string, args ...any) error {
	t := p.peek()
	got := t.text
	if t.kind == tokEOF {
		got = "end of program"
	}
	return fmt.Errorf("%w: line %d: %s, got %q", ErrSyntax, t.line, fmt.Sprintf(format, args...), got)
}

// statement is a declaration (name/arity.), a fact, or a rule.
func (p *parser) statement() error {
	if !p.at(tokName) {
		return p.fail("expected a relation name")
	}
	first := p.peek()
	line := first.line
	if p.toks[p.i+1].kind == tokPunct && p.toks[p.i+1].text == "/" {
		name := p.take().text
		p.take()
		if !p.at(tokNumber) {
			return p.fail("expected an arity after %s/", name)
		}
		arity, err := strconv.Atoi(p.take().text)
		if err != nil || arity < 1 {
			return fmt.Errorf("%w: line %d: %s/%v is not a relation: arity must be a positive integer", ErrSyntax, line, name, arity)
		}
		p.prog.Declared[name] = arity
		return p.expect(".")
	}
	head, err := p.atom()
	if err != nil {
		return err
	}
	r := Rule{Head: head, Line: line}
	if p.at(tokPunct, ":-") {
		p.take()
		for {
			lit, err := p.literal()
			if err != nil {
				return err
			}
			r.Body = append(r.Body, lit)
			if !p.at(tokPunct, ",") {
				break
			}
			p.take()
		}
	}
	dot := p.peek()
	if err := p.expect("."); err != nil {
		return err
	}
	r.Text = strings.TrimSpace(p.src[first.pos : dot.pos+1])
	p.prog.Rules = append(p.prog.Rules, r)
	return nil
}

func (p *parser) literal() (Literal, error) {
	if p.at(tokName, "not") {
		p.take()
		a, err := p.atom()
		if err != nil {
			return Literal{}, err
		}
		return Literal{Atom: &a, Negated: true}, nil
	}
	if p.at(tokName) {
		a, err := p.atom()
		if err != nil {
			return Literal{}, err
		}
		return Literal{Atom: &a}, nil
	}
	line := p.peek().line
	left, err := p.term()
	if err != nil {
		return Literal{}, err
	}
	op := p.peek()
	if op.kind != tokPunct || !comparisonOps[op.text] {
		return Literal{}, p.fail("expected a comparison operator")
	}
	p.take()
	right, err := p.term()
	if err != nil {
		return Literal{}, err
	}
	return Literal{Compare: &Comparison{Op: op.text, Left: left, Right: right, Line: line}}, nil
}

func (p *parser) atom() (Atom, error) {
	if !p.at(tokName) {
		return Atom{}, p.fail("expected a relation name")
	}
	t := p.take()
	a := Atom{Name: t.text, Line: t.line}
	if err := p.expect("("); err != nil {
		return Atom{}, err
	}
	for {
		arg, err := p.term()
		if err != nil {
			return Atom{}, err
		}
		a.Args = append(a.Args, arg)
		if p.at(tokPunct, ")") {
			p.take()
			return a, nil
		}
		if err := p.expect(","); err != nil {
			return Atom{}, err
		}
	}
}

func (p *parser) term() (Term, error) {
	t := p.peek()
	switch t.kind {
	case tokVar:
		p.take()
		return Term{Var: t.text}, nil
	case tokString:
		p.take()
		return Term{Const: String(t.text)}, nil
	case tokNumber:
		p.take()
		c, err := Number(t.text)
		if err != nil {
			return Term{}, fmt.Errorf("line %d: %w", t.line, err)
		}
		return Term{Const: c}, nil
	case tokName:
		if t.text == "true" || t.text == "false" {
			p.take()
			return Term{Const: Bool(t.text == "true")}, nil
		}
	}
	return Term{}, p.fail("expected a variable or a constant")
}

// Clone is a deep copy of the program that shares nothing with p: rules, their atoms
// and comparisons, the declarations. Each value is copied whole and then its reference
// parts replaced, so a field added to Program or Rule later is carried by the copy
// without anyone remembering to add it here.
func (p Program) Clone() Program {
	out := p
	if p.Rules != nil {
		out.Rules = make([]Rule, len(p.Rules))
		for i, r := range p.Rules {
			out.Rules[i] = r.clone()
		}
	}
	if p.Declared != nil {
		out.Declared = maps.Clone(p.Declared)
	}
	return out
}

// clone is a copy of the rule that shares no atom, comparison or slice with r.
func (r Rule) clone() Rule {
	out := r
	out.Head = copyAtom(r.Head)
	if r.Body != nil {
		out.Body = make([]Literal, len(r.Body))
		for i, l := range r.Body {
			out.Body[i] = copyLiteral(l)
		}
	}
	return out
}
