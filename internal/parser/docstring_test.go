package parser

import (
	"testing"

	"flint/internal/ast"
)

func TestDocstringOnTopLevelFunc(t *testing.T) {
	prog := parseOK(t, "$ Adds two numbers.\nfunc add(a: Int, b: Int): Int { return a + b }")
	fn := prog.Decls[0].(*ast.FuncDecl)
	if fn.Doc != "Adds two numbers." {
		t.Fatalf("Doc = %q", fn.Doc)
	}
}

// Consecutive $ lines merge into one docstring, separated by newlines.
func TestConsecutiveDocstringLinesMerge(t *testing.T) {
	src := "$ Computes the total area of every shape.\n$ Throws NegativeAreaError if any area is negative.\nfunc totalArea(): Float { return 0.0 }"
	fn := parseOK(t, src).Decls[0].(*ast.FuncDecl)
	want := "Computes the total area of every shape.\nThrows NegativeAreaError if any area is negative."
	if fn.Doc != want {
		t.Fatalf("Doc = %q, want %q", fn.Doc, want)
	}
}

func TestDocstringOnClassFieldsCtorAndMethods(t *testing.T) {
	src := `
$ A rectangle defined by width and height.
class Rectangle {
    $ The horizontal extent.
    width: Float

    $ Undocumented on purpose below.
    private height: Float

    $ Builds a rectangle.
    Rectangle(width: Float, height: Float) {
        object.width = width
        object.height = height
    }

    $ Returns width times height.
    func area(): Float { return object.width * object.height }

    func undocumented(): Void { }
}`
	c := parseOK(t, src).Decls[0].(*ast.ClassDecl)

	if c.Doc != "A rectangle defined by width and height." {
		t.Fatalf("class Doc = %q", c.Doc)
	}
	if c.Fields[0].Doc != "The horizontal extent." {
		t.Fatalf("field 0 Doc = %q", c.Fields[0].Doc)
	}
	if c.Fields[1].Doc != "Undocumented on purpose below." {
		t.Fatalf("field 1 Doc = %q", c.Fields[1].Doc)
	}
	if c.Ctor.Doc != "Builds a rectangle." {
		t.Fatalf("ctor Doc = %q", c.Ctor.Doc)
	}

	byName := map[string]*ast.FuncDecl{}
	for _, m := range c.Methods {
		byName[m.Name] = m
	}
	if byName["area"].Doc != "Returns width times height." {
		t.Fatalf("area Doc = %q", byName["area"].Doc)
	}
	if byName["undocumented"].Doc != "" {
		t.Fatalf("undocumented method picked up a doc: %q", byName["undocumented"].Doc)
	}
}

func TestDocstringOnInterfaceAndItsMethods(t *testing.T) {
	src := `
$ Anything with a measurable area.
interface Shape {
    $ The area in square units.
    func area(): Float

    func describe(): String
}`
	d := parseOK(t, src).Decls[0].(*ast.InterfaceDecl)
	if d.Doc != "Anything with a measurable area." {
		t.Fatalf("interface Doc = %q", d.Doc)
	}
	if d.Methods[0].Doc != "The area in square units." {
		t.Fatalf("method 0 Doc = %q", d.Methods[0].Doc)
	}
	if d.Methods[1].Doc != "" {
		t.Fatalf("method 1 picked up a doc: %q", d.Methods[1].Doc)
	}
}

func TestDocstringOnAbstractClassAndMethod(t *testing.T) {
	src := `
$ A partial shape.
abstract class BaseShape {
    $ Subclasses supply this.
    abstract func area(): Float
}`
	c := parseOK(t, src).Decls[0].(*ast.ClassDecl)
	if !c.Abstract {
		t.Fatal("abstract flag lost when a docstring precedes the class")
	}
	if c.Doc != "A partial shape." {
		t.Fatalf("class Doc = %q", c.Doc)
	}
	if c.Methods[0].Doc != "Subclasses supply this." {
		t.Fatalf("method Doc = %q", c.Methods[0].Doc)
	}
}

// A docstring documents nothing in these positions. It must be skipped rather
// than becoming a parse error.
func TestStrayDocstringsAreNotErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"inside a function body", "func f(): Void {\n $ stray\n print(1)\n}"},
		{"at end of a function body", "func f(): Void {\n print(1)\n $ trailing\n}"},
		{"at end of a class body", "class C {\n n: Int\n $ trailing\n}"},
		{"at end of an interface body", "interface I {\n func m(): Void\n $ trailing\n}"},
		{"at end of file", "func f(): Void { }\n$ trailing"},
		{"whole file is a docstring", "$ nothing here"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			parseOK(t, tt.src)
		})
	}
}

func TestDocstringDoesNotLeakToTheNextDeclaration(t *testing.T) {
	prog := parseOK(t, "$ For f.\nfunc f(): Void { }\nfunc g(): Void { }")
	if prog.Decls[0].(*ast.FuncDecl).Doc != "For f." {
		t.Fatal("f lost its docstring")
	}
	if got := prog.Decls[1].(*ast.FuncDecl).Doc; got != "" {
		t.Fatalf("g picked up %q", got)
	}
}

func TestCommentBetweenDocstringAndDeclarationIsIgnored(t *testing.T) {
	prog := parseOK(t, "$ Documented.\n// just a comment\nfunc f(): Void { }")
	if got := prog.Decls[0].(*ast.FuncDecl).Doc; got != "Documented." {
		t.Fatalf("Doc = %q, want it to survive an intervening comment", got)
	}
}
