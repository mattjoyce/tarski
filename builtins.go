package datalog

import "strings"

// builtin is a relation the engine answers itself. It is a guard like a comparison:
// every argument is bound by a positive body atom before it runs, it binds nothing and
// derives nothing, and it never appears in a head, a declaration or a schema.
type builtin struct {
	arity int
	eval  func(args []Const) bool
}

// builtins is keyed by relation name. Pattern comes first and subject second, so
// prefix("timeline/", R) reads as R starts with "timeline/". The hooks that consult
// it: relations and checkAtom in plan.go refuse a built-in where a relation belongs,
// order and ready in stratify.go place a built-in literal as a guard, and join in
// eval.go evaluates it where it evaluates a comparison. It is written once at package
// initialisation and only read after, so Evaluate calls may share it.
var builtins = map[string]builtin{
	"prefix":   {arity: 2, eval: stringTest(strings.HasPrefix)},
	"suffix":   {arity: 2, eval: stringTest(strings.HasSuffix)},
	"contains": {arity: 2, eval: stringTest(strings.Contains)},
}

// stringTest lifts a (subject, pattern) string test to a built-in. A non-string
// argument is false, never an error: a guard that cannot apply simply does not hold.
// So under not the guard is true, as != holds for mismatched kinds in a comparison:
// a number is not prefixed by anything.
func stringTest(test func(subject, pattern string) bool) func(args []Const) bool {
	return func(args []Const) bool {
		if args[0].Kind() != KindString || args[1].Kind() != KindString {
			return false
		}
		return test(args[1].Text(), args[0].Text())
	}
}

// isBuiltin reports whether name is a built-in rather than a relation.
func isBuiltin(name string) bool {
	_, ok := builtins[name]
	return ok
}

// holdsBuiltin evaluates a built-in literal under env: the built-in's own answer, or
// its negation when the literal is negated.
func holdsBuiltin(l Literal, env map[string]Const) bool {
	args := make([]Const, len(l.Atom.Args))
	for i, a := range l.Atom.Args {
		args[i] = value(a, env)
	}
	return builtins[l.Atom.Name].eval(args) != l.Negated
}
