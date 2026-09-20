package doc

import (
	"bytes"
	"strings"
	"testing"

	"flint/internal/lexer"
	"flint/internal/parser"
)

func render(t *testing.T, src string) string {
	t.Helper()
	toks, lexErrs := lexer.Scan(src, "test.flint")
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, parseErrs := parser.Parse(toks, "test.flint")
	if len(parseErrs) != 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	var buf bytes.Buffer
	Render(prog, &buf)
	return buf.String()
}

func TestRender(t *testing.T) {
	src := `
$ Anything with a measurable area.
interface Shape {
    $ The area in square units.
    func area(): Float
}

$ A rectangle defined by width and height.
class Rectangle : BaseShape, Shape {
    $ The horizontal extent.
    width: Float

    private height: Float

    $ Builds a rectangle.
    Rectangle(width: Float, height: Float) {
        object.width = width
        object.height = height
    }

    $ Returns width times height.
    func area(): Float { return object.width * object.height }
}

$ Sums the areas of every shape.
$ Returns zero for an empty list.
func totalArea(shapes: [Shape]): Float { return 0.0 }

func main(): Void { }
`
	got := render(t, src)

	want := []string{
		"interface Shape",
		"    Anything with a measurable area.",
		"    func area(): Float",
		"        The area in square units.",
		"class Rectangle : BaseShape, Shape",
		"    A rectangle defined by width and height.",
		"    width: Float",
		"        The horizontal extent.",
		"    private height: Float",
		"        (undocumented)",
		"    Rectangle(width: Float, height: Float)",
		"        Builds a rectangle.",
		"    func area(): Float",
		"        Returns width times height.",
		"func totalArea(shapes: [Shape]): Float",
		"    Sums the areas of every shape.",
		"    Returns zero for an empty list.",
		"func main(): Void",
		"    (undocumented)",
	}
	for _, line := range want {
		if !strings.Contains(got, line+"\n") {
			t.Fatalf("output is missing %q\n--- got ---\n%s", line, got)
		}
	}
}

func TestRenderPreservesSourceOrder(t *testing.T) {
	got := render(t, "$ b\nfunc b(): Void { }\n$ a\nfunc a(): Void { }")
	if strings.Index(got, "func b()") > strings.Index(got, "func a()") {
		t.Fatalf("declarations were reordered:\n%s", got)
	}
}

func TestRenderAbstractAndCollectionTypes(t *testing.T) {
	src := `
$ A partial shape.
abstract class BaseShape {
    $ Counts by name.
    tally: [String: Int]

    $ Subclasses supply this.
    abstract func area(): Float

    private func secret(a: [[Int]]): Void { }
}`
	got := render(t, src)
	for _, line := range []string{
		"abstract class BaseShape",
		"    tally: [String: Int]",
		"    abstract func area(): Float",
		"    private func secret(a: [[Int]]): Void",
	} {
		if !strings.Contains(got, line+"\n") {
			t.Fatalf("output is missing %q\n--- got ---\n%s", line, got)
		}
	}
}

// Documentation comes from the AST, so a file that does not type-check can
// still be documented.
func TestRenderWorksOnAProgramThatDoesNotTypeCheck(t *testing.T) {
	got := render(t, "$ Broken but documented.\nfunc f(): Int { let x: Nope = 1 }")
	if !strings.Contains(got, "Broken but documented.") {
		t.Fatalf("got %q", got)
	}
}
