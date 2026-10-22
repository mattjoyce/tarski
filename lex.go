package datalog

import (
	"fmt"
	"strconv"
	"strings"
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

// str reads a double-quoted string with Go/JSON escapes; the quoted text is handed to
// strconv.Unquote rather than a second escape table.
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
			s, err := strconv.Unquote(lx.src[start:lx.pos])
			if err != nil {
				return token{}, fmt.Errorf("%w: line %d: bad string %s", ErrSyntax, line, lx.src[start:lx.pos])
			}
			return token{kind: tokString, text: s, line: line}, nil
		case '\n':
			return token{}, fmt.Errorf("%w: line %d: string runs past the end of the line", ErrSyntax, line)
		}
		lx.pos++
	}
	return token{}, fmt.Errorf("%w: line %d: unterminated string", ErrSyntax, line)
}

// isDigit and isLetter are ASCII only: a name is a name in every locale, and a byte of
// a multi-byte character must never be read as a Latin-1 letter.
func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
