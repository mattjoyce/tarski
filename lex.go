package datalog

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// This file is the lexer: it reads program text into tokens, each with its line and
// its byte offset, so the parser can report a line and cut a rule's text out of the
// source. Names are ASCII; strings may hold any UTF-8. It knows no grammar beyond the
// shape of a token.

type tokKind uint8

const (
	tokEOF tokKind = iota
	tokName
	tokVar
	tokString
	tokNumber
	tokPunct // ( ) , . :- / and the comparison operators
)

type token struct {
	kind tokKind
	text string
	line int
	pos  int // byte offset of the token's first character in the source
}

type lexer struct {
	src  string
	pos  int
	line int
}

func (lx *lexer) all() ([]token, error) {
	var toks []token
	for {
		lx.skipSpace()
		start := lx.pos
		t, err := lx.next()
		if err != nil {
			return nil, err
		}
		t.pos = start
		toks = append(toks, t)
		if t.kind == tokEOF {
			return toks, nil
		}
	}
}

func (lx *lexer) next() (token, error) {
	lx.skipSpace()
	if lx.pos >= len(lx.src) {
		return token{kind: tokEOF, line: lx.line}, nil
	}
	start, line := lx.pos, lx.line
	// Decode a whole rune: a name is ASCII, and anything else is reported as the
	// character it is, not as one byte of its encoding.
	r, _ := utf8.DecodeRuneInString(lx.src[lx.pos:])
	switch {
	case r == '"':
		return lx.str(line)
	case r >= 'a' && r <= 'z':
		return token{kind: tokName, text: lx.word(), line: line}, nil
	case r >= 'A' && r <= 'Z' || r == '_':
		return token{kind: tokVar, text: lx.word(), line: line}, nil
	case isDigit(r) || r == '-':
		lx.number()
		return token{kind: tokNumber, text: lx.src[start:lx.pos], line: line}, nil
	}
	for _, p := range []string{":-", "!=", "<=", ">=", "=", "<", ">", "(", ")", ",", ".", "/"} {
		if strings.HasPrefix(lx.src[lx.pos:], p) {
			lx.pos += len(p)
			return token{kind: tokPunct, text: p, line: line}, nil
		}
	}
	return token{}, fmt.Errorf("%w: line %d: unexpected %q", ErrSyntax, line, r)
}

func (lx *lexer) skipSpace() {
	for lx.pos < len(lx.src) {
		switch lx.src[lx.pos] {
		case '\n':
			lx.line++
			lx.pos++
		case ' ', '\t', '\r':
			lx.pos++
		case '%':
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.pos++
			}
		default:
			return
		}
	}
}

// number scans a JSON number: an optional sign, digits, a fraction only when a digit
// follows the point (so the period that ends a statement is left alone), and an
// exponent.
func (lx *lexer) number() {
	digits := func() {
		for lx.pos < len(lx.src) && isDigit(rune(lx.src[lx.pos])) {
			lx.pos++
		}
	}
	if lx.src[lx.pos] == '-' {
		lx.pos++
	}
	digits()
	if lx.pos+1 < len(lx.src) && lx.src[lx.pos] == '.' && isDigit(rune(lx.src[lx.pos+1])) {
		lx.pos++
		digits()
	}
	if lx.pos < len(lx.src) && (lx.src[lx.pos] == 'e' || lx.src[lx.pos] == 'E') {
		lx.pos++
		if lx.pos < len(lx.src) && (lx.src[lx.pos] == '+' || lx.src[lx.pos] == '-') {
			lx.pos++
		}
		digits()
	}
}

func (lx *lexer) word() string {
	start := lx.pos
	for lx.pos < len(lx.src) {
		c := rune(lx.src[lx.pos])
		if !isLetter(c) && !isDigit(c) && c != '_' {
			break
		}
		lx.pos++
	}
	return lx.src[start:lx.pos]
}

// str reads a double-quoted string. Its escapes are JSON's or Go's: see unquote.
func (lx *lexer) str(line int) (token, error) {
	start := lx.pos
	lx.pos++
	for lx.pos < len(lx.src) {
		switch lx.src[lx.pos] {
		case '\\':
			lx.pos += 2
			continue
		case '"':
			lx.pos++
			raw := lx.src[start:lx.pos]
			s, err := unquote(raw)
			if err != nil {
				return token{}, fmt.Errorf("%w: line %d: bad string %s", ErrSyntax, line, raw)
			}
			// Both the bytes as written and the string they unquote to: a raw 0xff would be
			// read as U+FFFD, and an escape such as \xff would make a byte that is not text.
			if !utf8.ValidString(raw) || !utf8.ValidString(s) {
				return token{}, fmt.Errorf("%w: line %d: string is not valid UTF-8", ErrSyntax, line)
			}
			return token{kind: tokString, text: s, line: line}, nil
		case '\n':
			return token{}, fmt.Errorf("%w: line %d: string runs past the end of the line", ErrSyntax, line)
		}
		lx.pos++
	}
	return token{}, fmt.Errorf("%w: line %d: unterminated string", ErrSyntax, line)
}

// unquote reads a double-quoted string literal, quotes included, as either a JSON
// string or a Go string: a literal a JSON encoder wrote (\/, a surrogate pair for a
// character outside the BMP) and one a person wrote with Go's escapes (\x41, \u00e9,
// \U0001F600) both read as the text they spell. JSON is tried first, Go second; a
// literal neither accepts is an error. Both refuse a lone surrogate, and the caller
// checks that the result is valid UTF-8.
func unquote(raw string) (string, error) {
	if s, ok := jsonUnquote(raw); ok {
		return s, nil
	}
	return strconv.Unquote(raw)
}

// jsonUnquote reads raw as a JSON string literal: the escapes \" \\ \/ \b \f \n \r \t
// and \uXXXX, a surrogate pair read as one character, and nothing else. ok is false
// for anything that is not one.
func jsonUnquote(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", false
	}
	body := raw[1 : len(raw)-1]
	var b strings.Builder
	b.Grow(len(body))
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"' || c < 0x20:
			return "", false // an unescaped quote or control character is not JSON
		case c != '\\':
			b.WriteByte(c)
			continue
		}
		i++
		if i >= len(body) {
			return "", false
		}
		switch body[i] {
		case '"', '\\', '/':
			b.WriteByte(body[i])
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, n, ok := jsonRune(body[i+1:])
			if !ok {
				return "", false
			}
			b.WriteRune(r)
			i += n
		default:
			return "", false
		}
	}
	return b.String(), true
}

// jsonRune reads the hex digits after \u, and the second escape of a surrogate pair,
// returning the character and how many bytes it consumed. A lone surrogate is not ok.
func jsonRune(s string) (r rune, n int, ok bool) {
	hex4 := func(s string) (rune, bool) {
		if len(s) < 4 {
			return 0, false
		}
		v, err := strconv.ParseUint(s[:4], 16, 32)
		return rune(v), err == nil
	}
	r, ok = hex4(s)
	if !ok {
		return 0, 0, false
	}
	switch {
	case utf16.IsSurrogate(r) && r < 0xDC00: // a high surrogate: the low one must follow
		if len(s) < 10 || s[4] != '\\' || s[5] != 'u' {
			return 0, 0, false
		}
		low, ok := hex4(s[6:])
		if !ok || low < 0xDC00 || low > 0xDFFF {
			return 0, 0, false
		}
		return utf16.DecodeRune(r, low), 10, true
	case utf16.IsSurrogate(r):
		return 0, 0, false
	}
	return r, 4, true
}

// isDigit and isLetter are ASCII only: a name is a name in every locale, and a byte of
// a multi-byte character must never be read as a Latin-1 letter.
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }

// isName reports whether s is a relation name as the lexer reads one: a lower-case
// letter, then letters, digits and underscores. The checks on a schema, a program
// built by hand and an answer read back hold names to this, so a name is a name
// wherever it came from, and the NUL that separates a name from a tuple in a key
// can never be part of one.
func isName(s string) bool {
	if s == "" || s[0] < 'a' || s[0] > 'z' {
		return false
	}
	return isWord(s)
}

// isVarName reports whether s is a variable name as the lexer reads one: an
// upper-case letter or an underscore, then letters, digits and underscores.
func isVarName(s string) bool {
	if s == "" || (s[0] < 'A' || s[0] > 'Z') && s[0] != '_' {
		return false
	}
	return isWord(s)
}

func isWord(s string) bool {
	for i := 0; i < len(s); i++ {
		c := rune(s[i])
		if !isLetter(c) && !isDigit(c) && c != '_' {
			return false
		}
	}
	return true
}
