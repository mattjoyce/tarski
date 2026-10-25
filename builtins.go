package tarski

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Builtin is a relation answered in Go rather than by facts: a guard like a
// comparison. Every argument is bound by a positive body atom before it runs, it binds
// nothing and derives nothing, and it never appears in a head, a declaration or among
// a schema's relations. So a builtin creates no value and evaluation still terminates.
//
// The engine has three (see EngineBuiltins); a consumer declares its own in
// Schema.Builtins. Holds must be a pure, total, deterministic function of its
// arguments, safe to call from concurrent evaluations: the engine bounds how often it
// is called (each call follows a charged join step), but not what one call costs, and
// an impure Holds voids the promise that one (plan, base) has one answer and one hash.
type Builtin struct {
	// Args names each argument, in order, for catalogues and error messages. The
	// builtin's arity is len(Args), at least one.
	Args []BuiltinArg `json:"args"`
	// Doc is the contract a catalogue prints, kept beside the function.
	Doc string `json:"doc"`
	// Check, if not nil, refuses at Compile a constant argument the builtin could
	// never accept. consts holds one entry per argument: the constant written there,
	// or nil for a variable, which is judged only when it is bound. Its error is
	// wrapped in ErrInvalid with the line.
	Check func(consts []*Const) error `json:"-"`
	// Holds is the guard over the bound arguments. A value of the wrong kind or shape
	// is false, not an error: it came from data, and data is not the program's fault.
	// A Holds that panics fails the evaluation with ErrInvalid; the panic does not
	// escape.
	Holds func(args []Const) bool `json:"-"`
}

// BuiltinArg is one argument of a builtin: its name, as a program would write it in
// a signature such as between(X, Lo, Hi), and what it means.
type BuiltinArg struct {
	Name string `json:"name"`
	Doc  string `json:"doc"`
}

// engineBuiltins is keyed by relation name. Pattern comes first and subject second, so
// prefix("timeline/", R) reads as R starts with "timeline/". It is written once at
// package initialisation and only read after; a plan copies what it needs at Compile.
var engineBuiltins = map[string]Builtin{
	"prefix":   stringBuiltin("true when Subject starts with Pattern", strings.HasPrefix),
	"suffix":   stringBuiltin("true when Subject ends with Pattern", strings.HasSuffix),
	"contains": stringBuiltin("true when Pattern occurs in Subject", strings.Contains),
}

// EngineBuiltins is the engine's own builtins, as a fresh copy a catalogue may list
// beside a consumer's. Changing it changes nothing the engine does.
func EngineBuiltins() map[string]Builtin {
	out := make(map[string]Builtin, len(engineBuiltins))
	for name, b := range engineBuiltins {
		b.Args = slices.Clone(b.Args)
		out[name] = b
	}
	return out
}

// stringBuiltin lifts a (subject, pattern) string test to a builtin. A non-string
// argument is false, never an error: a guard that cannot apply simply does not hold.
// So under not the guard is true, as != holds for mismatched kinds in a comparison:
// a number is not prefixed by anything.
func stringBuiltin(doc string, test func(subject, pattern string) bool) Builtin {
	return Builtin{
		Args: []BuiltinArg{
			{Name: "Pattern", Doc: "a string"},
			{Name: "Subject", Doc: "a string"},
		},
		Doc: doc + "; a non-string argument is false. Both arguments must be bound by a positive body atom",
		Holds: func(args []Const) bool {
			if args[0].Kind() != KindString || args[1].Kind() != KindString {
				return false
			}
			return test(args[1].Text(), args[0].Text())
		},
	}
}

// builtinTable is the builtins one plan knows, engine and consumer, as Compile copied
// them. It is never written after Compile, so concurrent evaluations may share it.
type builtinTable map[string]Builtin

// newBuiltinTable copies the engine's builtins and the consumer's declared ones into
// one table. A consumer's Args are cloned, so changing them later changes nothing
// here. The consumer's map has been checked by checkBuiltins.
func newBuiltinTable(declared map[string]Builtin) builtinTable {
	t := maps.Clone(builtinTable(engineBuiltins))
	for name, b := range declared {
		b.Args = slices.Clone(b.Args)
		t[name] = b
	}
	return t
}

// has reports whether name is a builtin rather than a relation.
func (t builtinTable) has(name string) bool {
	_, ok := t[name]
	return ok
}

// holds evaluates a builtin literal under env: the builtin's own answer, or its
// negation when the literal is negated. A Holds that panics is an error, ErrInvalid,
// naming the builtin and its arguments: the panic is recovered here, in the frame
// that called it, so it never unwinds through the evaluator.
func (t builtinTable) holds(l Literal, env map[string]Const) (ok bool, err error) {
	args := make([]Const, len(l.Atom.Args))
	for i, a := range l.Atom.Args {
		args[i] = value(a, env)
	}
	defer func() {
		if v := recover(); v != nil {
			ok, err = false, builtinPanicked(l.Atom.Name, args, v)
		}
	}()
	return t[l.Atom.Name].Holds(args) != l.Negated, nil
}

// checkBuiltins refuses a consumer builtin the engine could not use: a name the lexer
// would not read as a relation, an engine builtin's name, a base relation's name, no
// arguments, or no Holds. Names are walked in order, so the error names the same one
// every time.
func checkBuiltins(schema Schema) error {
	for _, name := range sortedKeys(schema.Builtins) {
		b := schema.Builtins[name]
		switch _, engine := engineBuiltins[name]; {
		case !isName(name):
			return fmt.Errorf("%w: %q is not a builtin name; start it with a lower-case letter", ErrInvalid, name)
		case engine:
			return fmt.Errorf("%w: %s is an engine builtin; give yours another name", ErrInvalid, name)
		case schema.Arities[name] != 0:
			return fmt.Errorf("%w: %s is both a base relation and a builtin; rename one", ErrInvalid, name)
		case len(b.Args) < 1:
			return fmt.Errorf("%w: builtin %s has no arguments; name at least one in Args", ErrInvalid, name)
		case b.Holds == nil:
			return fmt.Errorf("%w: builtin %s has no Holds", ErrInvalid, name)
		}
	}
	return nil
}

// checkBuiltinAtom holds a builtin atom to its arity, then to the builtin's Check
// over the constants written in it.
func checkBuiltinAtom(a Atom, b Builtin) error {
	if len(b.Args) != len(a.Args) {
		return fmt.Errorf("%s has %d arguments, but %s is %s/%d", a.Name, len(a.Args), a.Name, a.Name, len(b.Args))
	}
	if b.Check == nil {
		return nil
	}
	consts := make([]*Const, len(a.Args))
	for i, t := range a.Args {
		if !t.IsVar() {
			c := t.Const
			consts[i] = &c
		}
	}
	if err := b.Check(consts); err != nil {
		return fmt.Errorf("%s: %w", a.Name, err)
	}
	return nil
}

// builtinPanicked is the error for a builtin whose Holds panicked on args.
func builtinPanicked(name string, args []Const, v any) error {
	terms := make([]Term, len(args))
	for i, c := range args {
		terms[i] = Term{Const: c}
	}
	return fmt.Errorf("%w: builtin %s(%s) panicked: %v; a builtin must be total over its arguments",
		ErrInvalid, name, termsText(terms), v)
}
