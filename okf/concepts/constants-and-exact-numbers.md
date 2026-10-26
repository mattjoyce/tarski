---
type: Concept
title: Constants and exact numbers
description: "Three scalar kinds, a valid-UTF-8 string domain and numbers held as exact decimals with one canonical text, so 9007199254740993 stays itself and 1, 1.0 and 1e0 are one constant."
tags: [constants, kinds, numbers, utf8, decimal, canonical-text]
timestamp: 2026-10-26T00:00:00Z
---

# Kinds

A `Const` is a string, a number or a boolean, the scalar kinds JSON has. No null, list or object. Kinds sort `KindString < KindNumber < KindBool`, and a constant's identity includes its kind, so the string `"1"` and the number `1` are different constants. Comparisons across kinds are false; a wrong-kind builtin argument is false.

| Constructor | Notes |
|---|---|
| `String(s)` | Total. The string domain is valid UTF-8, checked where it enters. |
| `Bool(b)` | |
| `Number(lexeme)` | A JSON number lexeme held exactly; otherwise `ErrSyntax`. `-0` is 0. |
| `Int(n int64)` | Every int64 is exact; the same constant `Number` makes for its text. |
| `Float(f float64)` | `Number` of the shortest round-trip text; NaN or Inf is `ErrInvalid`. |

# Exact decimals, not float64

Numbers are exact decimals, ported from Anam (card 1). The value is what the lexeme says and nothing is rounded: `9007199254740993` is itself, and `1` and `1.0000000000000000001` are two numbers. Identity, equality and order follow the value exactly, so `1`, `1.0` and `1e0` are one constant with one hash.

- **Grammar.** `Number` reads JSON's number grammar only: no plus, leading zero (`01` is refused), hex, underscore or bare point; NaN and Inf are not finite.
- **Exponent bound.** An exponent is bounded at nine digits either way (`maxExponent` = 999 999 999, Anam card 224) and refused past it, before any arithmetic. `1e999` is accepted (Shearline refused it because a float64 overflows).
- **Read-back rule.** A number is accepted only if its canonical text reads back. The commit security review found `10e999999999` was accepted but its text `1e+1000000000` was refused on re-read, so an Answer holding it could not be decoded. `parseNumber` now bounds the scientific exponent after canonicalising, and `FuzzNumberRoundTrip` (in CI) holds that every accepted lexeme's text reads back as the same constant.

# Canonical text

One value has one canonical text. It is the plain decimal: an integer in full, so `1792269840000` and `1e21` print every digit, and `0.00001`, not `1e-05`. Only when the plain form would pass 64 characters (`maxNumberDigits`) does it use the exact scientific form (`1e+999`, `1.23e-70`). The text matches Anam's byte for byte, so Anam's stored receipts keep their hashes.

# The breaking change against Shearline's engine

Against the engine as Shearline v0.1.0 had it, a number at or above 1e6 or below 1e-4 in magnitude now prints in full (`1000000`, `0.00001`) where float64's shortest form wrote an exponent. That changes those answers' hashes. This was the one deliberate break, taken in the first tag; see [decision 2](/decisions/2-exact-numbers-in-the-first-tag.md). Nothing in Shearline's bundled rules or tests carries such a number.

# Sums

Exact decimals are what make `sum` order-free: addition is associative and commutative on them. See [aggregates](/concepts/aggregates.md) and [bounds](/concepts/bounds.md) for `maxSumDigits`.

# Names

A relation name is a lower-case letter then letters, digits and underscores, and a variable an upper-case letter or underscore then the same, wherever it comes from: `Compile` holds a hand-built `Program` and a `Schema` to the grammar as the lexer holds text. A string literal's escapes are JSON's or Go's; a lone surrogate reads as neither.

# Why UTF-8

JSON writes an invalid byte and U+FFFD as the same text, so two distinct constants would share an Answer hash. The lexer, `NewBase`, `Base.With` and `Compile` (for hand-built constants) refuse invalid UTF-8.

# Related

[Frozen bytes and answer hashes](/concepts/frozen-bytes-and-hashes.md), [decision 2](/decisions/2-exact-numbers-in-the-first-tag.md).

# Citations

[1] [Card 1, exact decimal numbers](../../kanban/done/1-exact-numbers.md)
[2] [docs/DESIGN.md, constants](../../docs/DESIGN.md)
[3] [CHANGELOG.md, v0.1.0](../../CHANGELOG.md)
[4] [number.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/number.go)
[5] [golden_test.go](https://github.com/mattjoyce/tarski/blob/v0.2.2/golden_test.go)
