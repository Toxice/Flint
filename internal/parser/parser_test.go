package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flint/internal/ast"
	"flint/internal/lexer"
	"flint/internal/token"
)

// parseOK parses src, failing the test on any lex or parse error.
func parseOK(t *testing.T, src string) *ast.Program {
	t.Helper()
	toks, lexErrs := lexer.Scan(src, "test.flint")
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, errs := Parse(toks, "test.flint")
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	return prog
}

// parseErr parses src and requires at least one error mentioning want.
func parseErr(t *testing.T, src, want string) {
	t.Helper()
	toks, _ := lexer.Scan(src, "test.flint")
	_, errs := Parse(toks, "test.flint")
	if len(errs) == 0 {
		t.Fatalf("Parse(%q) succeeded, want an error mentioning %q", src, want)
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), want) {
			return
		}
	}
	t.Fatalf("Parse(%q) errors = %v, want one mentioning %q", src, errs, want)
}

// body returns the statements of the first top-level func in src.
func body(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	prog := parseOK(t, "func f(): Void {\n"+src+"\n}")
	fn, ok := prog.Decls[0].(*ast.FuncDecl)
	if !ok {
		t.Fatalf("first decl is %T, want *ast.FuncDecl", prog.Decls[0])
	}
	return fn.Body.Stmts
}

// firstExpr returns the expression of a single `let x: T = <expr>` statement.
func firstExpr(t *testing.T, decl string) ast.Expr {
	t.Helper()
	stmts := body(t, decl)
	let, ok := stmts[0].(*ast.Let)
	if !ok {
		t.Fatalf("statement is %T, want *ast.Let", stmts[0])
	}
	return let.Value
}

func TestLetWithAnnotation(t *testing.T) {
	stmts := body(t, "let age: Int = 21")
	let, ok := stmts[0].(*ast.Let)
	if !ok {
		t.Fatalf("got %T, want *ast.Let", stmts[0])
	}
	if let.Name != "age" {
		t.Fatalf("name = %q, want age", let.Name)
	}
	nt, ok := let.Type.(*ast.NamedType)
	if !ok || nt.Name != "Int" {
		t.Fatalf("type = %#v, want NamedType Int", let.Type)
	}
	lit, ok := let.Value.(*ast.IntLit)
	if !ok || lit.Value != 21 {
		t.Fatalf("value = %#v, want IntLit 21", let.Value)
	}
}

// Global Constraint: annotations are mandatory, so this is a parse error.
func TestLetWithoutAnnotationIsAParseError(t *testing.T) {
	parseErr(t, "func f(): Void { let x = 1 }", "type annotation")
}

func TestMandatoryAnnotationsEverywhere(t *testing.T) {
	parseErr(t, "func f(a): Void { }", "type annotation")        // parameter
	parseErr(t, "func f(a: Int) { }", "return type")             // return type
	parseErr(t, "class C { name }", "type annotation")           // field
	parseErr(t, "func f(): Void { for (x in [1]) { } }", "type") // loop variable
}

func TestListTypeAndListLiteral(t *testing.T) {
	stmts := body(t, `let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0), Circle("Circ1", 3.0)]`)
	let := stmts[0].(*ast.Let)
	lt, ok := let.Type.(*ast.ListType)
	if !ok {
		t.Fatalf("type = %#v, want ListType", let.Type)
	}
	if lt.Elem.(*ast.NamedType).Name != "Shape" {
		t.Fatalf("elem = %#v, want Shape", lt.Elem)
	}
	ll, ok := let.Value.(*ast.ListLit)
	if !ok || len(ll.Elems) != 2 {
		t.Fatalf("value = %#v, want ListLit with 2 elements", let.Value)
	}
	call, ok := ll.Elems[0].(*ast.Call)
	if !ok {
		t.Fatalf("elem 0 = %T, want *ast.Call", ll.Elems[0])
	}
	if call.Fn.(*ast.Ident).Name != "Rectangle" || len(call.Args) != 3 {
		t.Fatalf("elem 0 = %#v, want Rectangle with 3 args", call)
	}
}

func TestMapTypeAndMapLiteral(t *testing.T) {
	stmts := body(t, `let counts: [String: Int] = ["a": 1, "b": 2]`)
	let := stmts[0].(*ast.Let)
	mt, ok := let.Type.(*ast.MapType)
	if !ok {
		t.Fatalf("type = %#v, want MapType", let.Type)
	}
	if mt.Key.(*ast.NamedType).Name != "String" || mt.Val.(*ast.NamedType).Name != "Int" {
		t.Fatalf("map type = %#v, want [String: Int]", mt)
	}
	ml, ok := let.Value.(*ast.MapLit)
	if !ok || len(ml.Keys) != 2 || len(ml.Vals) != 2 {
		t.Fatalf("value = %#v, want MapLit with 2 pairs", let.Value)
	}
}

// Review Focus 2: bracket literals are ambiguous between list and map.
func TestBracketLiteralDisambiguation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // "list" or "map"
		size int
	}{
		{"empty list", "let x: [Int] = []", "list", 0},
		{"empty map", "let x: [Int: Int] = [:]", "map", 0},
		{"one element list", "let x: [Int] = [1]", "list", 1},
		{"one pair map", "let x: [Int: Int] = [1: 2]", "map", 1},
		{"two element list", "let x: [Int] = [1, 2]", "list", 2},
		{"two pair map", "let x: [Int: Int] = [1: 2, 3: 4]", "map", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := firstExpr(t, tt.src)
			switch tt.want {
			case "list":
				ll, ok := e.(*ast.ListLit)
				if !ok {
					t.Fatalf("got %T, want *ast.ListLit", e)
				}
				if len(ll.Elems) != tt.size {
					t.Fatalf("got %d elements, want %d", len(ll.Elems), tt.size)
				}
			case "map":
				ml, ok := e.(*ast.MapLit)
				if !ok {
					t.Fatalf("got %T, want *ast.MapLit", e)
				}
				if len(ml.Keys) != tt.size {
					t.Fatalf("got %d pairs, want %d", len(ml.Keys), tt.size)
				}
			}
		})
	}
}

func TestPrecedence(t *testing.T) {
	tests := []struct {
		src     string
		topOp   token.Kind
		leftOp  token.Kind // ILLEGAL means "not a Binary"
		rightOp token.Kind
	}{
		{"let x: Int = a + b * c", token.PLUS, token.ILLEGAL, token.STAR},
		{"let x: Int = a * b + c", token.PLUS, token.STAR, token.ILLEGAL},
		{"let x: Int = a - b - c", token.MINUS, token.MINUS, token.ILLEGAL},
		{"let x: Bool = a || b && c", token.OROR, token.ILLEGAL, token.ANDAND},
		{"let x: Bool = a == b && c", token.ANDAND, token.EQ, token.ILLEGAL},
		{"let x: Bool = a < b == c", token.EQ, token.LT, token.ILLEGAL},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			bin, ok := firstExpr(t, tt.src).(*ast.Binary)
			if !ok {
				t.Fatalf("top node is not Binary")
			}
			if bin.Op != tt.topOp {
				t.Fatalf("top op = %v, want %v", bin.Op, tt.topOp)
			}
			checkSide := func(side ast.Expr, want token.Kind, label string) {
				sub, isBin := side.(*ast.Binary)
				if want == token.ILLEGAL {
					if isBin {
						t.Fatalf("%s is Binary(%v), want a leaf", label, sub.Op)
					}
					return
				}
				if !isBin || sub.Op != want {
					t.Fatalf("%s = %#v, want Binary(%v)", label, side, want)
				}
			}
			checkSide(bin.X, tt.leftOp, "left")
			checkSide(bin.Y, tt.rightOp, "right")
		})
	}
}

func TestUnaryBindsTighterThanBinary(t *testing.T) {
	bin, ok := firstExpr(t, "let x: Bool = !a && b").(*ast.Binary)
	if !ok || bin.Op != token.ANDAND {
		t.Fatalf("got %#v, want Binary(&&)", bin)
	}
	if _, ok := bin.X.(*ast.Unary); !ok {
		t.Fatalf("left = %T, want *ast.Unary", bin.X)
	}
}

func TestObjectFieldAccess(t *testing.T) {
	bin, ok := firstExpr(t, "let x: Float = object.width * object.height").(*ast.Binary)
	if !ok || bin.Op != token.STAR {
		t.Fatalf("got %#v, want Binary(*)", bin)
	}
	for _, side := range []ast.Expr{bin.X, bin.Y} {
		f, ok := side.(*ast.Field)
		if !ok {
			t.Fatalf("operand = %T, want *ast.Field", side)
		}
		if _, ok := f.X.(*ast.ObjectExpr); !ok {
			t.Fatalf("field base = %T, want *ast.ObjectExpr", f.X)
		}
	}
}

func TestPostfixChaining(t *testing.T) {
	// a.b(c)[0].d parses left to right.
	e := firstExpr(t, "let x: Int = a.b(c)[0].d")
	f, ok := e.(*ast.Field)
	if !ok || f.Name != "d" {
		t.Fatalf("outermost = %#v, want Field d", e)
	}
	idx, ok := f.X.(*ast.Index)
	if !ok {
		t.Fatalf("next = %T, want *ast.Index", f.X)
	}
	call, ok := idx.X.(*ast.Call)
	if !ok {
		t.Fatalf("next = %T, want *ast.Call", idx.X)
	}
	if _, ok := call.Fn.(*ast.Field); !ok {
		t.Fatalf("callee = %T, want *ast.Field", call.Fn)
	}
}

func TestControlFlowDropsParens(t *testing.T) {
	// The spec drops the parens; a parenthesized condition is still a valid
	// grouped expression, so both forms parse.
	for _, src := range []string{"if x > 5 { }", "if (x > 5) { }", "while x > 5 { }", "while (x) { }"} {
		t.Run(src, func(t *testing.T) { body(t, src) })
	}
}

func TestBracesAreMandatory(t *testing.T) {
	parseErr(t, "func f(): Void { if x > 5 print(x) }", "{")
	parseErr(t, "func f(): Void { while x print(x) }", "{")
}

func TestIfElseChain(t *testing.T) {
	stmts := body(t, "if a { } else if b { } else { }")
	first, ok := stmts[0].(*ast.If)
	if !ok {
		t.Fatalf("got %T, want *ast.If", stmts[0])
	}
	second, ok := first.Else.(*ast.If)
	if !ok {
		t.Fatalf("else = %T, want *ast.If", first.Else)
	}
	if _, ok := second.Else.(*ast.Block); !ok {
		t.Fatalf("final else = %T, want *ast.Block", second.Else)
	}
}

func TestForIn(t *testing.T) {
	stmts := body(t, "for (shape: Shape in shapes) { }")
	f, ok := stmts[0].(*ast.For)
	if !ok {
		t.Fatalf("got %T, want *ast.For", stmts[0])
	}
	if f.Var != "shape" {
		t.Fatalf("var = %q, want shape", f.Var)
	}
	if f.Type.(*ast.NamedType).Name != "Shape" {
		t.Fatalf("type = %#v, want Shape", f.Type)
	}
	if f.Iter.(*ast.Ident).Name != "shapes" {
		t.Fatalf("iter = %#v, want Ident shapes", f.Iter)
	}
}

func TestClassInheritanceSingleColon(t *testing.T) {
	prog := parseOK(t, "class Rectangle : BaseShape { }")
	c := prog.Decls[0].(*ast.ClassDecl)
	if len(c.Bases) != 1 || c.Bases[0] != "BaseShape" {
		t.Fatalf("bases = %v, want [BaseShape]", c.Bases)
	}
	prog = parseOK(t, "class Circle : BaseShape, Comparable { }")
	c = prog.Decls[0].(*ast.ClassDecl)
	if len(c.Bases) != 2 || c.Bases[0] != "BaseShape" || c.Bases[1] != "Comparable" {
		t.Fatalf("bases = %v, want [BaseShape Comparable]", c.Bases)
	}
	prog = parseOK(t, "class Plain { }")
	if len(prog.Decls[0].(*ast.ClassDecl).Bases) != 0 {
		t.Fatal("a class with no colon must have no bases")
	}
}

func TestConstructorIsNamedForItsClass(t *testing.T) {
	prog := parseOK(t, "class Dog {\n name: String\n\n Dog(name: String) { object.name = name }\n}")
	c := prog.Decls[0].(*ast.ClassDecl)
	if c.Ctor == nil {
		t.Fatal("ctor not recognised")
	}
	if !c.Ctor.IsCtor || c.Ctor.Name != "Dog" || len(c.Ctor.Params) != 1 {
		t.Fatalf("ctor = %#v", c.Ctor)
	}
	if len(c.Methods) != 0 {
		t.Fatalf("ctor was also recorded as a method: %#v", c.Methods)
	}
	if len(c.Fields) != 1 || c.Fields[0].Name != "name" {
		t.Fatalf("fields = %#v", c.Fields)
	}
}

func TestAbstractAndPrivate(t *testing.T) {
	prog := parseOK(t, "abstract class B {\n private secret: Int\n abstract func area(): Float\n func ok(): Void { }\n}")
	c := prog.Decls[0].(*ast.ClassDecl)
	if !c.Abstract {
		t.Fatal("class not marked abstract")
	}
	if !c.Fields[0].Private {
		t.Fatal("field not marked private")
	}
	var abstractM, concreteM *ast.FuncDecl
	for _, m := range c.Methods {
		if m.Name == "area" {
			abstractM = m
		}
		if m.Name == "ok" {
			concreteM = m
		}
	}
	if abstractM == nil || !abstractM.Abstract || abstractM.Body != nil {
		t.Fatalf("abstract method = %#v", abstractM)
	}
	if concreteM == nil || concreteM.Abstract || concreteM.Body == nil {
		t.Fatalf("concrete method = %#v", concreteM)
	}
}

func TestPublicIsAcceptedButOptional(t *testing.T) {
	prog := parseOK(t, "class C {\n public n: Int\n public func f(): Void { }\n}")
	c := prog.Decls[0].(*ast.ClassDecl)
	if c.Fields[0].Private {
		t.Fatal("explicitly public field marked private")
	}
}

func TestAbstractMethodWithABodyIsAnError(t *testing.T) {
	parseErr(t, "class C { abstract func f(): Void { } }", "abstract")
}

func TestInterface(t *testing.T) {
	prog := parseOK(t, "interface Shape {\n func area(): Float\n func describe(): String\n}")
	i, ok := prog.Decls[0].(*ast.InterfaceDecl)
	if !ok {
		t.Fatalf("got %T, want *ast.InterfaceDecl", prog.Decls[0])
	}
	if len(i.Methods) != 2 {
		t.Fatalf("got %d methods, want 2", len(i.Methods))
	}
	for _, m := range i.Methods {
		if m.Body != nil {
			t.Fatalf("interface method %s has a body", m.Name)
		}
	}
}

func TestInterfaceMethodWithABodyIsAnError(t *testing.T) {
	parseErr(t, "interface I { func f(): Void { } }", "body")
}

func TestTryCatch(t *testing.T) {
	stmts := body(t, "try { } catch (e: NegativeAreaError) { }")
	tr, ok := stmts[0].(*ast.Try)
	if !ok {
		t.Fatalf("got %T, want *ast.Try", stmts[0])
	}
	if len(tr.Catches) != 1 {
		t.Fatalf("got %d catch clauses, want 1", len(tr.Catches))
	}
	if tr.Catches[0].Name != "e" || tr.Catches[0].Type.(*ast.NamedType).Name != "NegativeAreaError" {
		t.Fatalf("catch = %#v", tr.Catches[0])
	}

	stmts = body(t, "try { } catch (e: A) { } catch (e: B) { }")
	if len(stmts[0].(*ast.Try).Catches) != 2 {
		t.Fatal("two catch clauses did not parse")
	}
}

func TestTryWithoutCatchIsAnError(t *testing.T) {
	parseErr(t, "func f(): Void { try { } }", "catch")
}

func TestThrow(t *testing.T) {
	stmts := body(t, `throw NegativeAreaError("Area cannot be negative")`)
	th, ok := stmts[0].(*ast.Throw)
	if !ok {
		t.Fatalf("got %T, want *ast.Throw", stmts[0])
	}
	call, ok := th.Value.(*ast.Call)
	if !ok || call.Fn.(*ast.Ident).Name != "NegativeAreaError" {
		t.Fatalf("throw value = %#v", th.Value)
	}
}

func TestAssignmentTargets(t *testing.T) {
	for _, src := range []string{"x = 1", "object.n = 1", "a[0] = 1", "object.m[0] = 1"} {
		t.Run(src, func(t *testing.T) {
			if _, ok := body(t, src)[0].(*ast.Assign); !ok {
				t.Fatalf("%q did not parse as an assignment", src)
			}
		})
	}
	parseErr(t, "func f(): Void { 1 + 2 = 3 }", "cannot assign")
}

func TestReturn(t *testing.T) {
	stmts := body(t, "return")
	if r, ok := stmts[0].(*ast.Return); !ok || r.Value != nil {
		t.Fatalf("bare return = %#v", stmts[0])
	}
	stmts = body(t, "return 1 + 2")
	if r, ok := stmts[0].(*ast.Return); !ok || r.Value == nil {
		t.Fatalf("return with value = %#v", stmts[0])
	}
}

func TestErrorRecoveryReportsMoreThanOneError(t *testing.T) {
	src := "func f(): Void { let a = 1 }\nfunc g(): Void { let b = 2 }"
	toks, _ := lexer.Scan(src, "test.flint")
	_, errs := Parse(toks, "test.flint")
	if len(errs) < 2 {
		t.Fatalf("got %d errors, want at least 2 (recovery failed): %v", len(errs), errs)
	}
}

func TestSampleProgramParses(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "shapes.flint")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading sample: %v", err)
	}
	toks, lexErrs := lexer.Scan(string(src), path)
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, errs := Parse(toks, path)
	if len(errs) != 0 {
		t.Fatalf("parse errors: %v", errs)
	}
	// interface Shape, BaseShape, Rectangle, Circle, NegativeAreaError,
	// totalArea, main.
	if len(prog.Decls) != 7 {
		t.Fatalf("got %d top-level decls, want 7", len(prog.Decls))
	}
}
