package tarski

import (
	"fmt"
	"strings"
)

// This file is the aggregate's surface: its AST, how the parser reads one, how it is
// written back, and its structural check. Compile's checks are in aggregate_check.go
// and evaluation in aggregate_eval.go. See docs/design/aggregates.md.

// Aggregate is the body literal Result = Op Target : { Body }: count, sum, min or max
// taken over the solutions of Body for each binding of the variables Body shares with
// the rest of the rule.
type Aggregate struct {
	Op     string // "count", "sum", "min" or "max"
	Target Term   // the variable summed or compared; the zero Term for count
	Result Term   // the variable the aggregate's value binds
	Body   []Literal
	Line   int
}

// aggregateOps are the operations an aggregate may name.
var aggregateOps = map[string]bool{"count": true, "sum": true, "min": true, "max": true}

// avgRefusal is why avg and mean are not aggregates: exact division does not terminate
// (1/3), so an average needs a scale and a rounding rule, which would become frozen text
// in every receipt. count and sum are both derivable, so a caller divides at the scale
// it wants. The words are recognised where an operation stands only to say so.
const avgRefusal = "is not an aggregate here; derive count and sum and divide in the caller"

// isAggregateWord reports whether a name is read as an operation where an aggregate
// may stand: one of the four, or avg and mean, which are refused there.
func isAggregateWord(s string) bool { return aggregateOps[s] || s == "avg" || s == "mean" }

// atAggregate reports whether the tokens at the cursor look like an aggregate in a
// place none belongs: an operation word followed by ":" or "{", or by a variable and
// ":". A relation named count is followed by "(", so it never looks like one.
func (p *parser) atAggregate() bool {
	t := p.peek()
	if t.kind != tokName || !isAggregateWord(t.text) {
		return false
	}
	// A name is never the last token: EOF follows it, so i+1 is in range.
	next := p.toks[p.i+1]
	if next.kind == tokPunct && (next.text == ":" || next.text == "{") {
		return true
	}
	return next.kind == tokVar && p.i+2 < len(p.toks) && p.toks[p.i+2].kind == tokPunct && p.toks[p.i+2].text == ":"
}

// aggregate reads op [X] : { literal, ... } after "result =", the operation word at
// the cursor.
func (p *parser) aggregate(result Term, line int) (Literal, error) {
	opTok := p.peek()
	op := opTok.text
	switch {
	case !aggregateOps[op]:
		return Literal{}, fmt.Errorf("%w: line %d: %s %s", ErrSyntax, opTok.line, op, avgRefusal)
	case p.inAggregate:
		return Literal{}, fmt.Errorf("%w: line %d: an aggregate cannot nest; name the inner aggregate's relation and aggregate over it",
			ErrSyntax, opTok.line)
	case !result.IsVar() || result.Var == "_":
		return Literal{}, fmt.Errorf("%w: line %d: the result of %s must be a named variable, as in N = %s ... : { ... }",
			ErrSyntax, line, op, op)
	}
	p.take()
	agg := &Aggregate{Op: op, Result: result, Line: line}
	if op == "count" {
		if p.at(tokVar) {
			return Literal{}, p.fail("count takes no variable; write %s = count : { ... }", result.Var)
		}
	} else {
		if !p.at(tokVar) || p.peek().text == "_" {
			return Literal{}, p.fail("expected the variable %s is taken over, as in %s = %s X : { ... }", op, result.Var, op)
		}
		agg.Target = Term{Var: p.take().text}
	}
	if !p.at(tokPunct, ":") {
		return Literal{}, p.fail("expected \":\" after %s", strings.TrimSpace(op+" "+agg.Target.Var))
	}
	p.take()
	body, err := p.aggregateBody(op)
	if err != nil {
		return Literal{}, err
	}
	agg.Body = body
	return Literal{Aggregate: agg}, nil
}

// aggregateBody reads { literal, ... }, refusing an empty body and an aggregate inside.
func (p *parser) aggregateBody(op string) ([]Literal, error) {
	if !p.at(tokPunct, "{") {
		return nil, p.fail("expected \"{\" to open the body of %s", op)
	}
	p.take()
	if p.at(tokPunct, "}") {
		return nil, p.fail("the body of %s is empty; put at least one atom inside the braces", op)
	}
	p.inAggregate = true
	defer func() { p.inAggregate = false }()
	var body []Literal
	for {
		lit, err := p.literal()
		if err != nil {
			return nil, err
		}
		body = append(body, lit)
		if !p.at(tokPunct, ",") {
			break
		}
		p.take()
	}
	if !p.at(tokPunct, "}") {
		return nil, p.fail("expected \",\" or \"}\" in the body of %s", op)
	}
	p.take()
	return body, nil
}

// text is the aggregate as it is written: N = count : { a(X), b(X) }.
func (a Aggregate) text() string {
	parts := make([]string, len(a.Body))
	for i, l := range a.Body {
		parts[i] = l.text()
	}
	head := a.Result.text() + " = " + a.Op
	if a.Target != (Term{}) {
		head += " " + a.Target.text()
	}
	return head + " : { " + strings.Join(parts, ", ") + " }"
}

// checkAggregateShape refuses an aggregate built by hand that the parser would not
// make: an operation other than the four, a result or target that is not a named
// variable, a target on count, an empty body, or an aggregate inside the body.
func checkAggregateShape(a Aggregate) error {
	switch {
	case a.Op == "avg" || a.Op == "mean":
		return fmt.Errorf("%s %s", a.Op, avgRefusal)
	case !aggregateOps[a.Op]:
		return fmt.Errorf("aggregate operation %q is not one of count, sum, min, max", a.Op)
	case !a.Result.IsVar() || a.Result.Var == "_":
		return fmt.Errorf("the result of %s must be a named variable", a.Op)
	case a.Op == "count" && a.Target != (Term{}):
		return fmt.Errorf("count takes no target; leave Target zero")
	case a.Op != "count" && (!a.Target.IsVar() || a.Target.Var == "_"):
		return fmt.Errorf("the target of %s must be a named variable", a.Op)
	case len(a.Body) == 0:
		return fmt.Errorf("the body of %s is empty", a.Op)
	}
	for _, t := range []Term{a.Result, a.Target} {
		if err := checkTermShape(t); err != nil {
			return err
		}
	}
	for i, l := range a.Body {
		if l.Aggregate != nil {
			return fmt.Errorf("%s, body literal %d: an aggregate cannot nest", a.Op, i+1)
		}
		if err := checkLiteralShape(l); err != nil {
			return fmt.Errorf("%s, body literal %d: %w", a.Op, i+1, err)
		}
	}
	return nil
}

// HasAggregates reports whether any rule of the plan holds an aggregate. A consumer
// whose rules must not aggregate refuses a plan for which it is true.
func (p Plan) HasAggregates() bool {
	for _, stratum := range p.strata {
		for _, r := range stratum {
			for _, l := range r.Body {
				if l.Aggregate != nil {
					return true
				}
			}
		}
	}
	return false
}
