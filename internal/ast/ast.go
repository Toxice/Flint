// Package ast defines the Flint abstract syntax tree.
//
// Every node carries the source position of its first token so the checker and
// the interpreter can report errors against the original file.
package ast

import "flint/internal/token"

// Node is any AST node.
type Node interface {
	Pos() token.Pos
}

// TypeExpr is a type as it appears in source: Int, [Shape], [String: Int].
type TypeExpr interface {
	Node
	typeExpr()
}

// Expr is an expression node.
type Expr interface {
	Node
	expr()
}

// Stmt is a statement node.
type Stmt interface {
	Node
	stmt()
}

// Decl is a top-level declaration.
type Decl interface {
	Node
	decl()
}

// ---------- Type expressions ----------

// NamedType is a primitive, class, or interface name: Int, Shape.
type NamedType struct {
	Name string
	P    token.Pos
}

// ListType is a list type: [Shape].
type ListType struct {
	Elem TypeExpr
	P    token.Pos
}

// MapType is a map type: [String: Int].
type MapType struct {
	Key TypeExpr
	Val TypeExpr
	P   token.Pos
}

func (t *NamedType) Pos() token.Pos { return t.P }
func (t *ListType) Pos() token.Pos  { return t.P }
func (t *MapType) Pos() token.Pos   { return t.P }

// String renders the type back as Flint source.
func (t *NamedType) String() string { return t.Name }

// String renders the type back as Flint source.
func (t *ListType) String() string { return "[" + TypeString(t.Elem) + "]" }

// String renders the type back as Flint source.
func (t *MapType) String() string {
	return "[" + TypeString(t.Key) + ": " + TypeString(t.Val) + "]"
}

// TypeString renders a type annotation back as source, or "?" if it is absent.
func TypeString(t TypeExpr) string {
	switch t := t.(type) {
	case *NamedType:
		return t.String()
	case *ListType:
		return t.String()
	case *MapType:
		return t.String()
	}
	return "?"
}

func (*NamedType) typeExpr() {}
func (*ListType) typeExpr()  {}
func (*MapType) typeExpr()   {}

// ---------- Expressions ----------

// IntLit is an Int literal.
type IntLit struct {
	Value int64
	P     token.Pos
}

// FloatLit is a Float literal.
type FloatLit struct {
	Value float64
	P     token.Pos
}

// StringLit is a String literal, already unescaped.
type StringLit struct {
	Value string
	P     token.Pos
}

// CharLit is a Char literal.
type CharLit struct {
	Value rune
	P     token.Pos
}

// BoolLit is true or false.
type BoolLit struct {
	Value bool
	P     token.Pos
}

// Ident is a variable, parameter, function, or type name used as a value.
type Ident struct {
	Name string
	P    token.Pos
}

// ObjectExpr is the object keyword: the current instance.
type ObjectExpr struct {
	P token.Pos
}

// Unary is a prefix operation: -x, !x.
type Unary struct {
	Op token.Kind
	X  Expr
	P  token.Pos
}

// Binary is an infix operation.
type Binary struct {
	Op token.Kind
	X  Expr
	Y  Expr
	P  token.Pos
}

// Call is a function call, a method call, or a constructor call.
type Call struct {
	Fn   Expr
	Args []Expr
	P    token.Pos
}

// Field is a member access: object.name, shape.area.
type Field struct {
	X    Expr
	Name string
	P    token.Pos
}

// Index is a subscript: list[0], map["a"].
type Index struct {
	X     Expr
	Index Expr
	P     token.Pos
}

// ListLit is a list literal: [a, b].
type ListLit struct {
	Elems []Expr
	P     token.Pos
}

// MapLit is a map literal: ["a": 1]. Keys and Vals are parallel slices.
type MapLit struct {
	Keys []Expr
	Vals []Expr
	P    token.Pos
}

func (e *IntLit) Pos() token.Pos     { return e.P }
func (e *FloatLit) Pos() token.Pos   { return e.P }
func (e *StringLit) Pos() token.Pos  { return e.P }
func (e *CharLit) Pos() token.Pos    { return e.P }
func (e *BoolLit) Pos() token.Pos    { return e.P }
func (e *Ident) Pos() token.Pos      { return e.P }
func (e *ObjectExpr) Pos() token.Pos { return e.P }
func (e *Unary) Pos() token.Pos      { return e.P }
func (e *Binary) Pos() token.Pos     { return e.P }
func (e *Call) Pos() token.Pos       { return e.P }
func (e *Field) Pos() token.Pos      { return e.P }
func (e *Index) Pos() token.Pos      { return e.P }
func (e *ListLit) Pos() token.Pos    { return e.P }
func (e *MapLit) Pos() token.Pos     { return e.P }

func (*IntLit) expr()     {}
func (*FloatLit) expr()   {}
func (*StringLit) expr()  {}
func (*CharLit) expr()    {}
func (*BoolLit) expr()    {}
func (*Ident) expr()      {}
func (*ObjectExpr) expr() {}
func (*Unary) expr()      {}
func (*Binary) expr()     {}
func (*Call) expr()       {}
func (*Field) expr()      {}
func (*Index) expr()      {}
func (*ListLit) expr()    {}
func (*MapLit) expr()     {}

// ---------- Statements ----------

// Let declares a local variable. Type is never nil: annotations are mandatory.
type Let struct {
	Name  string
	Type  TypeExpr
	Value Expr
	P     token.Pos
}

// Assign stores into a variable, a field, or an index.
type Assign struct {
	Target Expr
	Value  Expr
	P      token.Pos
}

// ExprStmt is an expression evaluated for its effect, such as a call.
type ExprStmt struct {
	X Expr
	P token.Pos
}

// Block is a brace-delimited statement sequence and a lexical scope.
type Block struct {
	Stmts []Stmt
	P     token.Pos
}

// If is a conditional. Else is nil, another *If, or a *Block.
type If struct {
	Cond Expr
	Then *Block
	Else Stmt
	P    token.Pos
}

// While is a conditional loop.
type While struct {
	Cond Expr
	Body *Block
	P    token.Pos
}

// For is the for-in loop: for (shape: Shape in shapes) { ... }.
type For struct {
	Var  string
	Type TypeExpr
	Iter Expr
	Body *Block
	P    token.Pos
}

// Return exits a function. Value is nil in a Void function.
type Return struct {
	Value Expr
	P     token.Pos
}

// Throw raises an exception.
type Throw struct {
	Value Expr
	P     token.Pos
}

// CatchClause binds a thrown exception to a name for one exception type.
type CatchClause struct {
	Name string
	Type TypeExpr
	Body *Block
	P    token.Pos
}

// Try runs Body, dispatching a thrown exception to the first matching clause.
type Try struct {
	Body    *Block
	Catches []*CatchClause
	P       token.Pos
}

func (s *Let) Pos() token.Pos         { return s.P }
func (s *Assign) Pos() token.Pos      { return s.P }
func (s *ExprStmt) Pos() token.Pos    { return s.P }
func (s *Block) Pos() token.Pos       { return s.P }
func (s *If) Pos() token.Pos          { return s.P }
func (s *While) Pos() token.Pos       { return s.P }
func (s *For) Pos() token.Pos         { return s.P }
func (s *Return) Pos() token.Pos      { return s.P }
func (s *Throw) Pos() token.Pos       { return s.P }
func (s *Try) Pos() token.Pos         { return s.P }
func (s *CatchClause) Pos() token.Pos { return s.P }

func (*Let) stmt()      {}
func (*Assign) stmt()   {}
func (*ExprStmt) stmt() {}
func (*Block) stmt()    {}
func (*If) stmt()       {}
func (*While) stmt()    {}
func (*For) stmt()      {}
func (*Return) stmt()   {}
func (*Throw) stmt()    {}
func (*Try) stmt()      {}

// ---------- Declarations ----------

// Param is one function or constructor parameter.
type Param struct {
	Name string
	Type TypeExpr
	P    token.Pos
}

// Pos reports the parameter's source position.
func (p *Param) Pos() token.Pos { return p.P }

// FuncDecl is a top-level function, a method, or a constructor.
//
// Body is nil for an abstract method and for an interface method. Result is nil
// only for a constructor, which has no return type.
type FuncDecl struct {
	Doc      string // the $ docstring preceding this declaration
	Name     string
	Params   []*Param
	Result   TypeExpr
	Body     *Block
	Abstract bool
	Private  bool
	IsCtor   bool
	P        token.Pos
}

// FieldDecl is an instance field.
type FieldDecl struct {
	Doc     string // the $ docstring preceding this field
	Name    string
	Type    TypeExpr
	Private bool
	P       token.Pos
}

// ClassDecl is a class. Bases holds the names after the single colon, which may
// name one superclass and any number of interfaces, in any order.
type ClassDecl struct {
	Doc      string // the $ docstring preceding this class
	Name     string
	Abstract bool
	Bases    []string
	Fields   []*FieldDecl
	Ctor     *FuncDecl
	Methods  []*FuncDecl
	P        token.Pos
}

// InterfaceDecl is an interface. Its methods always have a nil Body.
type InterfaceDecl struct {
	Doc     string // the $ docstring preceding this interface
	Name    string
	Methods []*FuncDecl
	P       token.Pos
}

// Program is a whole source file.
type Program struct {
	Decls []Decl
	P     token.Pos
}

func (d *FuncDecl) Pos() token.Pos      { return d.P }
func (d *FieldDecl) Pos() token.Pos     { return d.P }
func (d *ClassDecl) Pos() token.Pos     { return d.P }
func (d *InterfaceDecl) Pos() token.Pos { return d.P }
func (d *Program) Pos() token.Pos       { return d.P }

func (*FuncDecl) decl()      {}
func (*ClassDecl) decl()     {}
func (*InterfaceDecl) decl() {}
