// Package parser builds a Flint AST from a token stream.
//
// Declarations and statements use recursive descent; expressions use precedence
// climbing. On an error the parser records it and resynchronises at the next
// statement or declaration boundary, so a single mistake does not cascade into
// a page of noise.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"flint/internal/ast"
	"flint/internal/token"
)

type parser struct {
	toks []token.Token
	file string
	i    int
	errs []error
}

// bailout unwinds to the nearest recovery point after a parse error. It never
// escapes Parse.
type bailout struct{}

// Parse converts a token stream into a program, returning every error it found.
// The returned program is always non-nil but is incomplete when errors exist.
func Parse(toks []token.Token, filename string) (*ast.Program, []error) {
	p := &parser{toks: toks, file: filename}
	prog := &ast.Program{P: p.pos()}
	for !p.at(token.EOF) {
		before := p.i
		d := p.parseDeclSafely()
		if d != nil {
			prog.Decls = append(prog.Decls, d)
		}
		if p.i == before {
			// Guarantee forward progress even if recovery consumed nothing.
			p.advance()
		}
	}
	return prog, p.errs
}

// parseDeclSafely parses one declaration, converting a bailout into a skip to
// the next top-level declaration.
func (p *parser) parseDeclSafely() (d ast.Decl) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(bailout); !ok {
			panic(r)
		}
		p.syncTopLevel()
		d = nil
	}()
	return p.parseDecl()
}

// ---------- token helpers ----------

func (p *parser) cur() token.Token { return p.toks[p.i] }

func (p *parser) pos() token.Pos { return p.toks[p.i].Pos }

func (p *parser) at(k token.Kind) bool { return p.toks[p.i].Kind == k }

func (p *parser) peekIs(n int, k token.Kind) bool {
	if p.i+n >= len(p.toks) {
		return false
	}
	return p.toks[p.i+n].Kind == k
}

func (p *parser) advance() token.Token {
	t := p.toks[p.i]
	if t.Kind != token.EOF {
		p.i++
	}
	return t
}

// accept consumes the current token if it matches, reporting whether it did.
func (p *parser) accept(k token.Kind) bool {
	if p.at(k) {
		p.advance()
		return true
	}
	return false
}

// expect consumes a token of kind k or aborts the current declaration.
func (p *parser) expect(k token.Kind, context string) token.Token {
	if p.at(k) {
		return p.advance()
	}
	p.errorf(p.pos(), "expected %s %s, found %s", k, context, p.cur().Kind)
	panic(bailout{})
}

// takeDoc consumes a run of consecutive $ docstring lines and joins them into
// one string. Consecutive lines merge, so a multi-line docstring needs no
// closing marker.
func (p *parser) takeDoc() string {
	var lines []string
	for p.at(token.DOC) {
		lines = append(lines, p.advance().Lit)
	}
	return strings.Join(lines, "\n")
}

func (p *parser) errorf(pos token.Pos, format string, args ...any) {
	p.errs = append(p.errs, fmt.Errorf("%s:%d:%d: %s", p.file, pos.Line, pos.Col,
		fmt.Sprintf(format, args...)))
}

// syncTopLevel skips ahead to the next plausible declaration start, balancing
// braces so a broken function body does not swallow the rest of the file.
func (p *parser) syncTopLevel() {
	depth := 0
	for !p.at(token.EOF) {
		switch p.cur().Kind {
		case token.LBRACE:
			depth++
		case token.RBRACE:
			depth--
			if depth <= 0 {
				p.advance()
				return
			}
		case token.FUNC, token.CLASS, token.INTERFACE, token.ABSTRACT:
			if depth == 0 {
				return
			}
		}
		p.advance()
	}
}

// syncStmt skips to the end of the current statement or block.
func (p *parser) syncStmt() {
	depth := 0
	for !p.at(token.EOF) {
		switch p.cur().Kind {
		case token.LBRACE:
			depth++
		case token.RBRACE:
			if depth == 0 {
				return
			}
			depth--
		case token.LET, token.RETURN, token.IF, token.WHILE, token.FOR,
			token.TRY, token.THROW:
			if depth == 0 {
				return
			}
		}
		p.advance()
	}
}

// ---------- declarations ----------

func (p *parser) parseDecl() ast.Decl {
	doc := p.takeDoc()
	// A file may end with a trailing docstring attached to nothing.
	if p.at(token.EOF) {
		return nil
	}

	switch {
	case p.at(token.FUNC):
		d := p.parseFuncDecl(false, false)
		d.Doc = doc
		return d
	case p.at(token.CLASS):
		d := p.parseClassDecl(false)
		d.Doc = doc
		return d
	case p.at(token.ABSTRACT) && p.peekIs(1, token.CLASS):
		p.advance()
		d := p.parseClassDecl(true)
		d.Doc = doc
		return d
	case p.at(token.INTERFACE):
		d := p.parseInterfaceDecl()
		d.Doc = doc
		return d
	}
	p.errorf(p.pos(), "expected a class, interface, or func declaration, found %s", p.cur().Kind)
	panic(bailout{})
}

// parseFuncDecl parses `func name(params): Result { body }`. An abstract or
// interface method has no body.
func (p *parser) parseFuncDecl(abstract, private bool) *ast.FuncDecl {
	start := p.pos()
	p.expect(token.FUNC, "to begin a function")
	name := p.expect(token.IDENT, "as the function name")

	fn := &ast.FuncDecl{Name: name.Lit, Abstract: abstract, Private: private, P: start}
	fn.Params = p.parseParams()

	if !p.at(token.COLON) {
		p.errorf(p.pos(), "function %s is missing a return type; write `: Void` if it returns nothing", fn.Name)
		panic(bailout{})
	}
	p.advance()
	fn.Result = p.parseTypeExpr()

	if abstract {
		if p.at(token.LBRACE) {
			p.errorf(p.pos(), "abstract func %s must not have a body", fn.Name)
			panic(bailout{})
		}
		return fn
	}
	fn.Body = p.parseBlock()
	return fn
}

func (p *parser) parseParams() []*ast.Param {
	p.expect(token.LPAREN, "to begin the parameter list")
	var params []*ast.Param
	for !p.at(token.RPAREN) {
		start := p.pos()
		name := p.expect(token.IDENT, "as a parameter name")
		if !p.at(token.COLON) {
			p.errorf(p.pos(), "parameter %s is missing a type annotation; write `%s: Type`", name.Lit, name.Lit)
			panic(bailout{})
		}
		p.advance()
		params = append(params, &ast.Param{Name: name.Lit, Type: p.parseTypeExpr(), P: start})
		if !p.accept(token.COMMA) {
			break
		}
	}
	p.expect(token.RPAREN, "to close the parameter list")
	return params
}

func (p *parser) parseClassDecl(abstract bool) *ast.ClassDecl {
	start := p.pos()
	p.expect(token.CLASS, "to begin a class")
	name := p.expect(token.IDENT, "as the class name")
	c := &ast.ClassDecl{Name: name.Lit, Abstract: abstract, P: start}

	// Inheritance and interface conformance share one colon-separated list;
	// which names are classes and which are interfaces is the checker's job.
	if p.accept(token.COLON) {
		for {
			base := p.expect(token.IDENT, "as a base class or interface name")
			c.Bases = append(c.Bases, base.Lit)
			if !p.accept(token.COMMA) {
				break
			}
		}
	}

	p.expect(token.LBRACE, "to begin the class body")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		p.parseMember(c)
	}
	p.expect(token.RBRACE, "to close the class body")
	return c
}

// parseMember parses one field, constructor, or method into c.
func (p *parser) parseMember(c *ast.ClassDecl) {
	doc := p.takeDoc()
	// A class body may end with a trailing docstring attached to nothing.
	if p.at(token.RBRACE) || p.at(token.EOF) {
		return
	}
	start := p.pos()

	private := false
	if p.at(token.PRIVATE) {
		private = true
		p.advance()
	} else if p.at(token.PUBLIC) {
		// Members are public by default; the keyword is accepted but redundant.
		p.advance()
	}

	abstract := false
	if p.at(token.ABSTRACT) {
		abstract = true
		p.advance()
	}

	if p.at(token.FUNC) {
		m := p.parseFuncDecl(abstract, private)
		m.Doc = doc
		c.Methods = append(c.Methods, m)
		return
	}
	if abstract {
		p.errorf(start, "abstract may only be applied to a func or a class")
		panic(bailout{})
	}

	// A constructor is a method named for its class: `Dog(name: String) { ... }`.
	if p.at(token.IDENT) && p.cur().Lit == c.Name && p.peekIs(1, token.LPAREN) {
		ctorStart := p.pos()
		p.advance()
		ctor := &ast.FuncDecl{Doc: doc, Name: c.Name, IsCtor: true, Private: private, P: ctorStart}
		ctor.Params = p.parseParams()
		ctor.Body = p.parseBlock()
		if c.Ctor != nil {
			p.errorf(ctorStart, "class %s declares more than one constructor", c.Name)
		}
		c.Ctor = ctor
		return
	}

	// Otherwise it is a field: `name: String`.
	name := p.expect(token.IDENT, "as a field name")
	if !p.at(token.COLON) {
		p.errorf(p.pos(), "field %s is missing a type annotation; write `%s: Type`", name.Lit, name.Lit)
		panic(bailout{})
	}
	p.advance()
	c.Fields = append(c.Fields, &ast.FieldDecl{
		Doc: doc, Name: name.Lit, Type: p.parseTypeExpr(), Private: private, P: start,
	})
}

func (p *parser) parseInterfaceDecl() *ast.InterfaceDecl {
	start := p.pos()
	p.expect(token.INTERFACE, "to begin an interface")
	name := p.expect(token.IDENT, "as the interface name")
	d := &ast.InterfaceDecl{Name: name.Lit, P: start}

	p.expect(token.LBRACE, "to begin the interface body")
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		mDoc := p.takeDoc()
		if p.at(token.RBRACE) || p.at(token.EOF) {
			break
		}
		mStart := p.pos()
		m := p.parseFuncDecl(true, false) // abstract: interface methods have no body
		if p.at(token.LBRACE) {
			p.errorf(mStart, "interface method %s must not have a body", m.Name)
			panic(bailout{})
		}
		m.Abstract = false // an interface method is not itself marked abstract
		m.Doc = mDoc
		d.Methods = append(d.Methods, m)
	}
	p.expect(token.RBRACE, "to close the interface body")
	return d
}

// ---------- types ----------

// parseTypeExpr parses Int, [Shape], or [String: Int].
func (p *parser) parseTypeExpr() ast.TypeExpr {
	start := p.pos()
	if p.accept(token.LBRACK) {
		elem := p.parseTypeExpr()
		if p.accept(token.COLON) {
			val := p.parseTypeExpr()
			p.expect(token.RBRACK, "to close a map type")
			return &ast.MapType{Key: elem, Val: val, P: start}
		}
		p.expect(token.RBRACK, "to close a list type")
		return &ast.ListType{Elem: elem, P: start}
	}
	name := p.expect(token.IDENT, "as a type name")
	return &ast.NamedType{Name: name.Lit, P: start}
}

// ---------- statements ----------

func (p *parser) parseBlock() *ast.Block {
	start := p.pos()
	p.expect(token.LBRACE, "to begin a block")
	b := &ast.Block{P: start}
	for !p.at(token.RBRACE) && !p.at(token.EOF) {
		// A docstring in statement position documents nothing; skip it rather
		// than failing, so a stray $ inside a body is not a parse error.
		if p.at(token.DOC) {
			p.advance()
			continue
		}
		before := p.i
		s := p.parseStmtSafely()
		if s != nil {
			b.Stmts = append(b.Stmts, s)
		}
		if p.i == before {
			p.advance()
		}
	}
	p.expect(token.RBRACE, "to close a block")
	return b
}

// parseStmtSafely parses one statement, recovering at the next statement
// boundary so the rest of the block still parses.
func (p *parser) parseStmtSafely() (s ast.Stmt) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		if _, ok := r.(bailout); !ok {
			panic(r)
		}
		p.syncStmt()
		s = nil
	}()
	return p.parseStmt()
}

func (p *parser) parseStmt() ast.Stmt {
	switch p.cur().Kind {
	case token.LET:
		return p.parseLet()
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		return p.parseWhile()
	case token.FOR:
		return p.parseFor()
	case token.RETURN:
		return p.parseReturn()
	case token.THROW:
		return p.parseThrow()
	case token.TRY:
		return p.parseTry()
	case token.LBRACE:
		return p.parseBlock()
	}
	return p.parseSimpleStmt()
}

func (p *parser) parseLet() ast.Stmt {
	start := p.pos()
	p.expect(token.LET, "to begin a variable declaration")
	name := p.expect(token.IDENT, "as a variable name")
	if !p.at(token.COLON) {
		p.errorf(p.pos(), "variable %s is missing a type annotation; Flint has no type inference, write `let %s: Type = ...`", name.Lit, name.Lit)
		panic(bailout{})
	}
	p.advance()
	typ := p.parseTypeExpr()
	p.expect(token.ASSIGN, "to initialise the variable")
	return &ast.Let{Name: name.Lit, Type: typ, Value: p.parseExpr(), P: start}
}

// parseSimpleStmt parses an assignment or a bare expression statement.
func (p *parser) parseSimpleStmt() ast.Stmt {
	start := p.pos()
	x := p.parseExpr()
	if !p.at(token.ASSIGN) {
		return &ast.ExprStmt{X: x, P: start}
	}
	eq := p.pos()
	p.advance()
	if !isAssignable(x) {
		p.errorf(eq, "cannot assign to this expression; the target must be a variable, a field, or an index")
		panic(bailout{})
	}
	return &ast.Assign{Target: x, Value: p.parseExpr(), P: start}
}

// isAssignable reports whether e may appear on the left of `=`.
func isAssignable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.Field, *ast.Index:
		return true
	}
	return false
}

func (p *parser) parseIf() ast.Stmt {
	start := p.pos()
	p.expect(token.IF, "to begin a conditional")
	// The condition has no required parentheses; a parenthesised condition is
	// simply a grouped expression.
	cond := p.parseExpr()
	stmt := &ast.If{Cond: cond, Then: p.parseBlock(), P: start}
	if p.accept(token.ELSE) {
		if p.at(token.IF) {
			stmt.Else = p.parseIf()
		} else {
			stmt.Else = p.parseBlock()
		}
	}
	return stmt
}

func (p *parser) parseWhile() ast.Stmt {
	start := p.pos()
	p.expect(token.WHILE, "to begin a loop")
	cond := p.parseExpr()
	return &ast.While{Cond: cond, Body: p.parseBlock(), P: start}
}

// parseFor parses `for (shape: Shape in shapes) { ... }`. Unlike if and while,
// the for-in header keeps its parentheses, per the spec's own sample.
func (p *parser) parseFor() ast.Stmt {
	start := p.pos()
	p.expect(token.FOR, "to begin a loop")
	p.expect(token.LPAREN, "to begin the for-in header")
	name := p.expect(token.IDENT, "as the loop variable")
	if !p.at(token.COLON) {
		p.errorf(p.pos(), "loop variable %s is missing a type annotation; write `for (%s: Type in ...)`", name.Lit, name.Lit)
		panic(bailout{})
	}
	p.advance()
	typ := p.parseTypeExpr()
	p.expect(token.IN, "between the loop variable and the collection")
	iter := p.parseExpr()
	p.expect(token.RPAREN, "to close the for-in header")
	return &ast.For{Var: name.Lit, Type: typ, Iter: iter, Body: p.parseBlock(), P: start}
}

func (p *parser) parseReturn() ast.Stmt {
	start := p.pos()
	p.expect(token.RETURN, "to return from a function")
	// A bare `return` in a Void function is followed by `}` or the next statement.
	if p.at(token.RBRACE) || startsStatement(p.cur().Kind) {
		return &ast.Return{P: start}
	}
	return &ast.Return{Value: p.parseExpr(), P: start}
}

func startsStatement(k token.Kind) bool {
	switch k {
	case token.LET, token.IF, token.WHILE, token.FOR, token.RETURN,
		token.THROW, token.TRY, token.EOF:
		return true
	}
	return false
}

func (p *parser) parseThrow() ast.Stmt {
	start := p.pos()
	p.expect(token.THROW, "to raise an exception")
	return &ast.Throw{Value: p.parseExpr(), P: start}
}

func (p *parser) parseTry() ast.Stmt {
	start := p.pos()
	p.expect(token.TRY, "to begin a try block")
	tr := &ast.Try{Body: p.parseBlock(), P: start}
	for p.at(token.CATCH) {
		cStart := p.pos()
		p.advance()
		p.expect(token.LPAREN, "to begin the catch clause")
		name := p.expect(token.IDENT, "as the caught exception's name")
		p.expect(token.COLON, "before the exception type")
		typ := p.parseTypeExpr()
		p.expect(token.RPAREN, "to close the catch clause")
		tr.Catches = append(tr.Catches, &ast.CatchClause{
			Name: name.Lit, Type: typ, Body: p.parseBlock(), P: cStart,
		})
	}
	if len(tr.Catches) == 0 {
		p.errorf(start, "try must be followed by at least one catch clause")
		panic(bailout{})
	}
	return tr
}

// ---------- expressions ----------

// binaryPrec gives each infix operator's binding power; higher binds tighter.
// Zero means the token is not an infix operator.
func binaryPrec(k token.Kind) int {
	switch k {
	case token.OROR:
		return 1
	case token.ANDAND:
		return 2
	case token.EQ, token.NE:
		return 3
	case token.LT, token.GT, token.LE, token.GE:
		return 4
	case token.PLUS, token.MINUS:
		return 5
	case token.STAR, token.SLASH, token.PERCENT:
		return 6
	}
	return 0
}

func (p *parser) parseExpr() ast.Expr { return p.parseBinary(1) }

// parseBinary is precedence climbing: it parses a unary expression, then while
// the next operator binds at least as tightly as minPrec, folds it in. All
// Flint infix operators are left-associative.
func (p *parser) parseBinary(minPrec int) ast.Expr {
	left := p.parseUnary()
	for {
		prec := binaryPrec(p.cur().Kind)
		if prec < minPrec {
			return left
		}
		op := p.advance()
		right := p.parseBinary(prec + 1)
		left = &ast.Binary{Op: op.Kind, X: left, Y: right, P: op.Pos}
	}
}

func (p *parser) parseUnary() ast.Expr {
	if p.at(token.MINUS) || p.at(token.BANG) {
		op := p.advance()
		return &ast.Unary{Op: op.Kind, X: p.parseUnary(), P: op.Pos}
	}
	return p.parsePostfix()
}

// parsePostfix applies call, field, and index suffixes left to right.
func (p *parser) parsePostfix() ast.Expr {
	x := p.parsePrimary()
	for {
		switch {
		case p.at(token.LPAREN):
			start := p.pos()
			p.advance()
			var args []ast.Expr
			for !p.at(token.RPAREN) {
				args = append(args, p.parseExpr())
				if !p.accept(token.COMMA) {
					break
				}
			}
			p.expect(token.RPAREN, "to close the argument list")
			x = &ast.Call{Fn: x, Args: args, P: start}
		case p.at(token.DOT):
			start := p.pos()
			p.advance()
			name := p.expect(token.IDENT, "as a member name")
			x = &ast.Field{X: x, Name: name.Lit, P: start}
		case p.at(token.LBRACK):
			start := p.pos()
			p.advance()
			idx := p.parseExpr()
			p.expect(token.RBRACK, "to close an index")
			x = &ast.Index{X: x, Index: idx, P: start}
		default:
			return x
		}
	}
}

func (p *parser) parsePrimary() ast.Expr {
	t := p.cur()
	switch t.Kind {
	case token.INT:
		p.advance()
		v, err := strconv.ParseInt(t.Lit, 10, 64)
		if err != nil {
			p.errorf(t.Pos, "integer literal %s is out of range for Int", t.Lit)
			panic(bailout{})
		}
		return &ast.IntLit{Value: v, P: t.Pos}

	case token.FLOAT:
		p.advance()
		v, err := strconv.ParseFloat(t.Lit, 64)
		if err != nil {
			p.errorf(t.Pos, "malformed float literal %s", t.Lit)
			panic(bailout{})
		}
		return &ast.FloatLit{Value: v, P: t.Pos}

	case token.STRING:
		p.advance()
		return &ast.StringLit{Value: t.Lit, P: t.Pos}

	case token.CHAR:
		p.advance()
		return &ast.CharLit{Value: []rune(t.Lit)[0], P: t.Pos}

	case token.TRUE:
		p.advance()
		return &ast.BoolLit{Value: true, P: t.Pos}

	case token.FALSE:
		p.advance()
		return &ast.BoolLit{Value: false, P: t.Pos}

	case token.OBJECT:
		p.advance()
		return &ast.ObjectExpr{P: t.Pos}

	case token.IDENT:
		p.advance()
		return &ast.Ident{Name: t.Lit, P: t.Pos}

	case token.LPAREN:
		p.advance()
		x := p.parseExpr()
		p.expect(token.RPAREN, "to close a parenthesised expression")
		return x

	case token.LBRACK:
		return p.parseBracketLit()
	}

	p.errorf(t.Pos, "expected an expression, found %s", t.Kind)
	panic(bailout{})
}

// parseBracketLit parses a list or a map literal. The two share a bracket, so
// they are told apart by what follows: `[]` is an empty list, `[:]` an empty
// map, and a `:` after the first element marks a map.
func (p *parser) parseBracketLit() ast.Expr {
	start := p.pos()
	p.expect(token.LBRACK, "to begin a collection literal")

	if p.accept(token.RBRACK) {
		return &ast.ListLit{P: start}
	}
	if p.at(token.COLON) && p.peekIs(1, token.RBRACK) {
		p.advance()
		p.advance()
		return &ast.MapLit{P: start}
	}

	first := p.parseExpr()

	if p.accept(token.COLON) {
		m := &ast.MapLit{Keys: []ast.Expr{first}, Vals: []ast.Expr{p.parseExpr()}, P: start}
		for p.accept(token.COMMA) {
			if p.at(token.RBRACK) {
				break // tolerate a trailing comma
			}
			m.Keys = append(m.Keys, p.parseExpr())
			p.expect(token.COLON, "between a map key and its value")
			m.Vals = append(m.Vals, p.parseExpr())
		}
		p.expect(token.RBRACK, "to close a map literal")
		return m
	}

	l := &ast.ListLit{Elems: []ast.Expr{first}, P: start}
	for p.accept(token.COMMA) {
		if p.at(token.RBRACK) {
			break // tolerate a trailing comma
		}
		l.Elems = append(l.Elems, p.parseExpr())
	}
	p.expect(token.RBRACK, "to close a list literal")
	return l
}
