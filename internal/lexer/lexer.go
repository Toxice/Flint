// Package lexer turns Flint source text into a token stream.
package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"flint/internal/token"
)

type lexer struct {
	src  []rune
	file string
	i    int // index into src
	line int
	col  int
	toks []token.Token
	errs []error
}

// Scan converts src into a token stream terminated by a single EOF token.
//
// Scan always returns a usable token slice, even when it reports errors: a
// malformed literal or an unrecognized character is recorded and scanning
// continues, so one bad character does not hide the rest of the file.
func Scan(src, filename string) ([]token.Token, []error) {
	l := &lexer{src: []rune(src), file: filename, line: 1, col: 1}
	l.run()
	return l.toks, l.errs
}

func (l *lexer) run() {
	for {
		l.skipSpaceAndComments()
		if l.atEnd() {
			break
		}
		l.scanToken()
	}
	l.emitAt(token.EOF, "", l.pos())
}

func (l *lexer) pos() token.Pos { return token.Pos{Line: l.line, Col: l.col} }

func (l *lexer) atEnd() bool { return l.i >= len(l.src) }

func (l *lexer) peek() rune {
	if l.atEnd() {
		return 0
	}
	return l.src[l.i]
}

func (l *lexer) peekAt(n int) rune {
	if l.i+n >= len(l.src) {
		return 0
	}
	return l.src[l.i+n]
}

// next consumes and returns one rune, tracking line and column.
func (l *lexer) next() rune {
	r := l.src[l.i]
	l.i++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return r
}

func (l *lexer) emitAt(k token.Kind, lit string, p token.Pos) {
	l.toks = append(l.toks, token.Token{Kind: k, Lit: lit, Pos: p})
}

func (l *lexer) errorAt(p token.Pos, format string, args ...any) {
	l.errs = append(l.errs, fmt.Errorf("%s:%d:%d: %s", l.file, p.Line, p.Col,
		fmt.Sprintf(format, args...)))
}

func (l *lexer) skipSpaceAndComments() {
	for !l.atEnd() {
		r := l.peek()
		switch {
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			l.next()
		case r == '/' && l.peekAt(1) == '/':
			// Line comment: consume to end of line, or to EOF if the comment
			// is on the last line with no trailing newline.
			for !l.atEnd() && l.peek() != '\n' {
				l.next()
			}
		default:
			return
		}
	}
}

func (l *lexer) scanToken() {
	start := l.pos()
	r := l.peek()

	switch {
	case isIdentStart(r):
		l.scanIdent(start)
		return
	case unicode.IsDigit(r):
		l.scanNumber(start)
		return
	case r == '"':
		l.scanString(start)
		return
	case r == '\'':
		l.scanChar(start)
		return
	case r == '$':
		l.scanDoc(start)
		return
	}

	// Two-rune operators are matched before their one-rune prefixes.
	two := string(r) + string(l.peekAt(1))
	if k, ok := twoRuneOps[two]; ok {
		l.next()
		l.next()
		l.emitAt(k, two, start)
		return
	}
	if k, ok := oneRuneOps[r]; ok {
		l.next()
		l.emitAt(k, string(r), start)
		return
	}

	l.next()
	l.errorAt(start, "unexpected character %q", r)
	l.emitAt(token.ILLEGAL, string(r), start)
}

var twoRuneOps = map[string]token.Kind{
	"==": token.EQ,
	"!=": token.NE,
	"<=": token.LE,
	">=": token.GE,
	"&&": token.ANDAND,
	"||": token.OROR,
}

var oneRuneOps = map[rune]token.Kind{
	'(': token.LPAREN,
	')': token.RPAREN,
	'{': token.LBRACE,
	'}': token.RBRACE,
	'[': token.LBRACK,
	']': token.RBRACK,
	',': token.COMMA,
	':': token.COLON,
	'.': token.DOT,
	'=': token.ASSIGN,
	'+': token.PLUS,
	'-': token.MINUS,
	'*': token.STAR,
	'/': token.SLASH,
	'%': token.PERCENT,
	'<': token.LT,
	'>': token.GT,
	'!': token.BANG,
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (l *lexer) scanIdent(start token.Pos) {
	var b strings.Builder
	for !l.atEnd() && isIdentPart(l.peek()) {
		b.WriteRune(l.next())
	}
	s := b.String()
	l.emitAt(token.Lookup(s), s, start)
}

// scanNumber reads an Int or a Float. A digit sequence followed by '.' must be
// followed by at least one more digit: "4." is an error, not a Float.
func (l *lexer) scanNumber(start token.Pos) {
	var b strings.Builder
	for !l.atEnd() && unicode.IsDigit(l.peek()) {
		b.WriteRune(l.next())
	}
	if l.peek() != '.' {
		l.emitAt(token.INT, b.String(), start)
		return
	}
	if !unicode.IsDigit(l.peekAt(1)) {
		// Consume the dot so scanning makes progress, then report.
		l.next()
		l.errorAt(start, "malformed float literal %q: expected a digit after the decimal point", b.String()+".")
		l.emitAt(token.ILLEGAL, b.String()+".", start)
		return
	}
	b.WriteRune(l.next()) // '.'
	for !l.atEnd() && unicode.IsDigit(l.peek()) {
		b.WriteRune(l.next())
	}
	l.emitAt(token.FLOAT, b.String(), start)
}

func (l *lexer) scanString(start token.Pos) {
	l.next() // opening quote
	var b strings.Builder
	for {
		if l.atEnd() || l.peek() == '\n' {
			l.errorAt(start, "unterminated string literal")
			l.emitAt(token.ILLEGAL, b.String(), start)
			return
		}
		r := l.next()
		if r == '"' {
			l.emitAt(token.STRING, b.String(), start)
			return
		}
		if r != '\\' {
			b.WriteRune(r)
			continue
		}
		if l.atEnd() {
			l.errorAt(start, "unterminated string literal")
			l.emitAt(token.ILLEGAL, b.String(), start)
			return
		}
		esc, ok := decodeEscape(l.next())
		if !ok {
			l.errorAt(l.pos(), "unknown escape sequence")
			continue
		}
		b.WriteRune(esc)
	}
}

func (l *lexer) scanChar(start token.Pos) {
	l.next() // opening quote
	if l.atEnd() || l.peek() == '\'' {
		l.errorAt(start, "empty or unterminated char literal")
		if !l.atEnd() {
			l.next()
		}
		l.emitAt(token.ILLEGAL, "", start)
		return
	}
	r := l.next()
	if r == '\\' {
		if l.atEnd() {
			l.errorAt(start, "unterminated char literal")
			l.emitAt(token.ILLEGAL, "", start)
			return
		}
		esc, ok := decodeEscape(l.next())
		if !ok {
			l.errorAt(start, "unknown escape sequence in char literal")
			l.emitAt(token.ILLEGAL, "", start)
			return
		}
		r = esc
	}
	if l.atEnd() || l.peek() != '\'' {
		l.errorAt(start, "unterminated char literal")
		l.emitAt(token.ILLEGAL, string(r), start)
		return
	}
	l.next() // closing quote
	l.emitAt(token.CHAR, string(r), start)
}

// scanDoc reads a docstring line: everything after $ up to the end of the line.
//
// Unlike a // comment, a docstring is kept as a token, because the parser
// attaches it to the declaration that follows.
func (l *lexer) scanDoc(start token.Pos) {
	l.next() // '$'
	var b strings.Builder
	for !l.atEnd() && l.peek() != '\n' {
		b.WriteRune(l.next())
	}
	l.emitAt(token.DOC, strings.TrimSpace(b.String()), start)
}

func decodeEscape(r rune) (rune, bool) {
	switch r {
	case 'n':
		return '\n', true
	case 't':
		return '\t', true
	case 'r':
		return '\r', true
	case '0':
		return 0, true
	case '"':
		return '"', true
	case '\'':
		return '\'', true
	case '\\':
		return '\\', true
	}
	return 0, false
}
