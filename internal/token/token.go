// Package token defines the lexical tokens of the Flint language.
package token

import "fmt"

// Kind classifies a token.
type Kind int

// The complete set of Flint token kinds.
const (
	EOF Kind = iota
	ILLEGAL

	IDENT  // area, Shape, shapes
	INT    // 21
	FLOAT  // 4.0
	STRING // "active"
	CHAR   // 'a'
	DOC    // $ a docstring line

	// Keywords.
	LET
	FUNC
	CLASS
	INTERFACE
	ABSTRACT
	PRIVATE
	PUBLIC
	RETURN
	IF
	ELSE
	WHILE
	FOR
	IN
	TRY
	CATCH
	THROW
	OBJECT
	TRUE
	FALSE

	// Punctuation.
	LPAREN
	RPAREN
	LBRACE
	RBRACE
	LBRACK
	RBRACK
	COMMA
	COLON
	DOT
	ASSIGN

	// Operators.
	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	EQ
	NE
	LT
	GT
	LE
	GE
	ANDAND
	OROR
	BANG
)

var kindNames = map[Kind]string{
	EOF:     "end of file",
	ILLEGAL: "illegal character",
	IDENT:   "identifier",
	INT:     "int literal",
	FLOAT:   "float literal",
	STRING:  "string literal",
	CHAR:    "char literal",
	DOC:     "docstring",

	LET:       "let",
	FUNC:      "func",
	CLASS:     "class",
	INTERFACE: "interface",
	ABSTRACT:  "abstract",
	PRIVATE:   "private",
	PUBLIC:    "public",
	RETURN:    "return",
	IF:        "if",
	ELSE:      "else",
	WHILE:     "while",
	FOR:       "for",
	IN:        "in",
	TRY:       "try",
	CATCH:     "catch",
	THROW:     "throw",
	OBJECT:    "object",
	TRUE:      "true",
	FALSE:     "false",

	LPAREN: "(",
	RPAREN: ")",
	LBRACE: "{",
	RBRACE: "}",
	LBRACK: "[",
	RBRACK: "]",
	COMMA:  ",",
	COLON:  ":",
	DOT:    ".",
	ASSIGN: "=",

	PLUS:    "+",
	MINUS:   "-",
	STAR:    "*",
	SLASH:   "/",
	PERCENT: "%",
	EQ:      "==",
	NE:      "!=",
	LT:      "<",
	GT:      ">",
	LE:      "<=",
	GE:      ">=",
	ANDAND:  "&&",
	OROR:    "||",
	BANG:    "!",
}

// String returns the source spelling of k, so parser errors read naturally.
func (k Kind) String() string {
	if s, ok := kindNames[k]; ok {
		return s
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

var keywords = map[string]Kind{
	"let":       LET,
	"func":      FUNC,
	"class":     CLASS,
	"interface": INTERFACE,
	"abstract":  ABSTRACT,
	"private":   PRIVATE,
	"public":    PUBLIC,
	"return":    RETURN,
	"if":        IF,
	"else":      ELSE,
	"while":     WHILE,
	"for":       FOR,
	"in":        IN,
	"try":       TRY,
	"catch":     CATCH,
	"throw":     THROW,
	"object":    OBJECT,
	"true":      TRUE,
	"false":     FALSE,
}

// Lookup maps an identifier to its keyword Kind, or IDENT if it is not a keyword.
func Lookup(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return IDENT
}

// Pos is a 1-based source position.
type Pos struct {
	Line int
	Col  int
}

// String renders a position as "line:col".
func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

// Token is a single lexical token with its source position.
type Token struct {
	Kind Kind
	Lit  string // decoded literal text for IDENT/INT/FLOAT/STRING/CHAR
	Pos  Pos
}
