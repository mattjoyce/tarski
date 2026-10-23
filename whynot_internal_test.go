package datalog

import "testing"

// goalKey tells goals apart by which open positions share a variable, not by what the
// variable is called.
func TestGoalKey_VariableIdentity(t *testing.T) {
	v := func(names ...string) []Term {
		out := make([]Term, len(names))
		for i, n := range names {
			out[i] = Term{Var: n}
		}
		return out
	}
	if goalKey("p", v("X", "X")) == goalKey("p", v("X", "Y")) {
		t.Fatal("p(X, X) and p(X, Y) are different goals")
	}
	if goalKey("p", v("X", "Y")) != goalKey("p", v("A", "B")) {
		t.Fatal("p(X, Y) and p(A, B) are the same goal")
	}
	if goalKey("p", v("X", "X")) != goalKey("p", v("B", "B")) {
		t.Fatal("p(X, X) and p(B, B) are the same goal")
	}
	if goalKey("p", v("_", "_")) != goalKey("p", v("X", "Y")) {
		t.Fatal("each _ is a fresh variable")
	}
}

func TestUnifyHead(t *testing.T) {
	c := func(s string) Term { return Term{Const: String(s)} }
	v := func(n string) Term { return Term{Var: n} }
	cases := []struct {
		name       string
		head, goal []Term
		ok         bool
		env        map[string]string
		alias      map[string]string
	}{
		{"constants agree", []Term{c("a")}, []Term{c("a")}, true, nil, nil},
		{"constants differ", []Term{c("a")}, []Term{c("b")}, false, nil, nil},
		{"goal binds head var", []Term{v("A")}, []Term{c("a")}, true, map[string]string{"A": "a"}, nil},
		{"repeated head var bound twice", []Term{v("A"), v("A")}, []Term{c("a"), c("b")}, false, nil, nil},
		{"goal equality over two constants", []Term{c("a"), c("b")}, []Term{v("X"), v("X")}, false, nil, nil},
		{"goal equality binds through a constant", []Term{v("Y"), c("a")}, []Term{v("X"), v("X")}, true, map[string]string{"Y": "a"}, nil},
		{"goal equality aliases head vars", []Term{v("A"), v("B")}, []Term{v("X"), v("X")}, true, nil, map[string]string{"B": "A"}},
		{"anonymous binds nothing", []Term{v("_"), v("_")}, []Term{v("X"), v("X")}, true, nil, nil},
		{"open goal leaves head free", []Term{v("A"), v("B")}, []Term{v("X"), v("Y")}, true, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env, alias, ok := unifyHead(tc.head, tc.goal)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if len(env) != len(tc.env) {
				t.Fatalf("env = %v, want %v", env, tc.env)
			}
			for k, want := range tc.env {
				if got, ok := env[k]; !ok || got.Text() != want {
					t.Fatalf("env[%s] = %v, want %s", k, got, want)
				}
			}
			if len(alias) != len(tc.alias) {
				t.Fatalf("alias = %v, want %v", alias, tc.alias)
			}
			for k, want := range tc.alias {
				if alias[k] != want {
					t.Fatalf("alias[%s] = %q, want %q", k, alias[k], want)
				}
			}
		})
	}
}
