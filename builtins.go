package datalog

// builtin is a relation the engine answers itself. It is a guard like a comparison:
// every argument is bound by a positive body atom before it runs, it binds nothing and
// derives nothing, and it never appears in a head, a declaration or a schema.
type builtin struct {
	arity int
	eval  func(args []Const) bool
}

// builtins is keyed by relation name. It is empty until the string built-ins are added
// beside it; the hooks that consult it are already in place: relations and checkAtom
// in plan.go refuse a built-in where a relation belongs, order and ready in stratify.go
// place a built-in literal as a guard, and join in eval.go evaluates it where it
// evaluates a comparison. It is written once at package initialisation and only read
// after, so Evaluate calls may share it.
var builtins = map[string]builtin{}

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
