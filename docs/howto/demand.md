# How to answer one question with demand-driven evaluation

Full evaluation derives every tuple of every relation, then you read the one you wanted. When the
question binds something ("reachable from `a`", not "everything reachable from anything"), demand
evaluation derives only what can reach the question. The answer is the same, byte for byte and
hash for hash. Only the work differs. The contract is in
[`DESIGN.md`](../DESIGN.md) § Demand-driven evaluation; the runnable version of this page is
`ExamplePlan_Demand`.

The examples use `edge` and `path`:

```
path(X, Y) :- edge(X, Y).
path(X, Z) :- path(X, Y), edge(Y, Z).
```

## 1. Choose an adornment

An adornment is a string of `b` and `f`, one letter per position of the relation. `b` means the
goal will hold that position to a constant. `f` means the goal leaves it open. For `path/2`:

| Adornment | Asks | Bound positions |
|---|---|---|
| `bf` | everything reachable from one node | 1 |
| `bb` | is there a path from this node to that one | 2 |
| `fb` | everything that reaches one node | 1 |
| `ff` | all paths | none (see § 5) |

Pick the adornment from the question your code asks, not from the relation. One relation can need
several: compile one plan per shape you ask.

## 2. Compile the plan and build the goal

```go
plan, err := tarski.Compile(schema, prog)
if err != nil { /* ... */ }

reachableFrom, err := plan.Demand("path", "bf")   // compile once, share freely
if err != nil { /* ... */ }

goal := &tarski.Goal{Relation: "path", Args: []tarski.Term{
    {Const: tarski.String("a")},   // a b position: a constant
    {Var: "_"},                    // an f position: any variable name leaves it open
}}
res, err := tarski.Evaluate(ctx, reachableFrom, base, tarski.Options{Goal: goal})
```

The goal must name the plan's relation, have its arity, hold a constant at every `b` and a
variable at every `f`. `Demand` returns a plan that is a value, like any other, so any number of
goroutines can evaluate it. A variable other than `_` written twice is refused, because an
adornment cannot say that two positions are equal. If you need that, write a rule that states the
equality and ask for that rule's relation.

`Options.Goal` is required on a plan from `Demand` and refused (`ErrInvalid`) on any other. Calling
`Demand` on a plan that is already a demand plan is also `ErrInvalid`: call it on the plan
`Compile` returned.

## 3. Read the result

`res.Relation("path")` is the full plan's `path` filtered to the goal's constants. For the goal
above that is every path from `a`. A base relation reads as the base's.

Any other derived relation is not complete under the goal, so reading it is `ErrInvalid` with a
message saying how to read it:

```go
_, err := res.Relation("other_derived")   // errors.Is(err, tarski.ErrInvalid)
```

If code takes a `Result` from somewhere and reads arbitrary relations, check `res.Goal()` first:
it reports ok false for a full evaluation.

Proofs and why-not work for the goal's relation, and only for tuples that agree with the goal's
constants. `WhyEmpty` works only when the goal leaves every position open. The proof you get is
a valid derivation, deterministic for one (plan, base, options), and it need not be the one the
full plan records first.

Bind as many positions as the question really fixes. With a relation of three positions, `bbf` binds the first two and reads the third off `Relation`, which is already filtered to the first two. `bbb` asks whether one tuple holds, and `Proof` and `WhyNot` on the goal relation then explain it.

If some facts arrive with the question, add them to the standing base with `Base.With` and evaluate the demand plan over the result. Relations the new facts do not touch are shared, not copied.

## 4. Compare with the full plan using `Cost`

Demand is not free: it adds helper rules, and the helper tuples are charged to the bounds. Before
you switch, evaluate the same question both ways and read `Result.Cost`:

```go
full, _ := tarski.Evaluate(ctx, plan, base, tarski.Options{})
fullSteps, fullBytes := full.Cost()

dem, _ := tarski.Evaluate(ctx, reachableFrom, base, tarski.Options{Goal: goal})
demSteps, demBytes := dem.Cost()
```

Compare the steps and the bytes. If demand costs more, use the full plan for that question. Then
set `MaxSteps` and `MaxBytes` from the measurement, with headroom (README § Setting the bounds).
Under the same bounds, a demand plan can be refused where the full plan answers, so tune the bound
for the plan you will run. `Cost` is an instrument: it is not the same across releases.

## 5. The goal that binds nothing

Selectivity comes from bound positions. A goal with every position open (`ff`) has nothing to
propagate, so the helper relations can end up as large as the relations they stand in for, and
demand then pays for helpers without gaining selectivity.

The evidence is in [`design/demand.md`](../design/demand.md) § 9.2. On a shape with an open goal
and a hand-bounded relation underneath, demand cost about 8 times the steps of full evaluation
over the hand-bounded relation (8.2 times at small scale, 8.4 times at medium), and 3 times the
bytes. Against full evaluation over the same relation left wide, demand cost 59 times fewer steps and 28 times less time at small scale. Demand over the wide relation cost the same as demand over the hand-bounded one. So an open goal can rescue a rule set written wide, and still lose to a relation bounded by hand.

If you can bound a relation by hand, measure both before you drop the bound. The measurement decides, not the shape.

## 6. One plan per goal shape

Compile each demand plan once, when the rule set is loaded, and keep them:

```go
type plans struct {
    full           tarski.Plan
    reachableFrom  tarski.Plan   // path, "bf"
    reachable      tarski.Plan   // path, "bb"
}
```

Each evaluation then supplies only the constants. Compiling a shape per request puts a
stratification pass in front of every request and gains nothing, since the shape does not change
between requests. A relation with several shapes in use has one plan per shape, and each plan
answers only its own relation.

## Where demand gives up

Where the rewrite would break stratification, or a relation would be reached under more than 64
binding patterns, demand is dropped for that relation and it is derived whole. The answer is
unchanged and only the work differs, so a measurement with `Cost` shows it. Aggregates are not
trimmed: demand enters their braces only through the group variables, so every group is computed
whole and its value is the full plan's.
