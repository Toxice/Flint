package interp

import (
	"bytes"
	"strings"
	"testing"

	"flint/internal/checker"
	"flint/internal/lexer"
	"flint/internal/parser"
	"flint/internal/types"
)

// run executes src and returns everything it printed, failing on any lex,
// parse, or check error.
func run(t *testing.T, src string) string {
	t.Helper()
	out, err := runErr(t, src)
	if err != nil {
		t.Fatalf("uncaught exception: %v\noutput so far:\n%s", err, out)
	}
	return out
}

// runErr executes src, returning its output and any exception that escaped main.
func runErr(t *testing.T, src string) (string, error) {
	t.Helper()
	toks, lexErrs := lexer.Scan(src, "test.flint")
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, parseErrs := parser.Parse(toks, "test.flint")
	if len(parseErrs) != 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	info, checkErrs := checker.Check(prog, "test.flint")
	if len(checkErrs) != 0 {
		t.Fatalf("check errors: %v", checkErrs)
	}
	var buf bytes.Buffer
	err := New(prog, info, &buf).Run()
	return buf.String(), err
}

// inMain wraps statements in a main function.
func inMain(stmts string) string {
	return "func main(): Void {\n" + stmts + "\n}\n"
}

// wantOut runs src and compares the printed output line for line.
func wantOut(t *testing.T, src string, want ...string) {
	t.Helper()
	got := strings.Split(strings.TrimRight(run(t, src), "\n"), "\n")
	if len(got) == 1 && got[0] == "" {
		got = nil
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q, want %d lines %q", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: got %q, want %q", i+1, got[i], want[i])
		}
	}
}

// ---------- Task 8: values ----------

func TestStringify(t *testing.T) {
	list := &ListVal{Elems: []Value{Int(1), Int(2)}}
	m := NewMap()
	m.Set(Str("a"), Int(1))
	m.Set(Str("b"), Int(2))

	tests := []struct {
		name string
		v    Value
		want string
	}{
		{"int", Int(42), "42"},
		{"negative int", Int(-7), "-7"},
		// A Float must never render as a bare integer, or it would be
		// indistinguishable from an Int.
		{"integral float", Float(20), "20.0"},
		{"zero float", Float(0), "0.0"},
		{"fractional float", Float(48.27431), "48.27431"},
		{"negative float", Float(-1.5), "-1.5"},
		{"true", Bool(true), "true"},
		{"false", Bool(false), "false"},
		{"string is unquoted", Str("hi"), "hi"},
		{"char", Char('a'), "a"},
		{"list", list, "[1, 2]"},
		{"empty list", &ListVal{}, "[]"},
		{"map", m, "[a: 1, b: 2]"},
		{"empty map", NewMap(), "[:]"},
		{"uninitialized", Uninit{}, "uninitialized"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Stringify(tt.v); got != tt.want {
				t.Fatalf("Stringify() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestZeroValue(t *testing.T) {
	cl := types.NewClass("C")
	tests := []struct {
		name string
		typ  types.Type
		want Value
	}{
		{"Int", types.Int, Int(0)},
		{"Float", types.Float, Float(0)},
		{"Bool", types.Bool, Bool(false)},
		{"String", types.String, Str("")},
		{"Char", types.Char, Char(0)},
		{"class is uninitialized", cl, Uninit{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ZeroValue(tt.typ); got != tt.want {
				t.Fatalf("ZeroValue(%s) = %#v, want %#v", tt.typ, got, tt.want)
			}
		})
	}
	if l, ok := ZeroValue(&types.List{Elem: types.Int}).(*ListVal); !ok || len(l.Elems) != 0 {
		t.Fatal("a list field should start empty, not uninitialized")
	}
	if m, ok := ZeroValue(&types.Map{Key: types.String, Val: types.Int}).(*MapVal); !ok || m.Len() != 0 {
		t.Fatal("a map field should start empty, not uninitialized")
	}
}

func TestMapPreservesInsertionOrder(t *testing.T) {
	m := NewMap()
	m.Set(Str("z"), Int(1))
	m.Set(Str("a"), Int(2))
	m.Set(Str("z"), Int(3)) // overwriting must not reorder

	if got := Stringify(m); got != "[z: 3, a: 2]" {
		t.Fatalf("got %q, want [z: 3, a: 2]", got)
	}
	if m.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", m.Len())
	}
}

// ---------- Task 9: evaluation ----------

func TestArithmeticAndPrecedence(t *testing.T) {
	wantOut(t, inMain(`
    print(1 + 2 * 3)
    print((1 + 2) * 3)
    print(7 / 2)
    print(7 % 2)
    print(10 - 3 - 2)
    print(-5 + 1)`),
		"7", "9", "3", "1", "5", "-4")
}

func TestIntAndFloatMixingProducesFloat(t *testing.T) {
	wantOut(t, inMain(`
    print(1 + 2.0)
    print(7.0 / 2)
    print(4.0 * 5.0)
    let f: Float = 3
    print(f)`),
		"3.0", "3.5", "20.0", "3.0")
}

func TestStringConcatenation(t *testing.T) {
	wantOut(t, inMain(`
    print("a" + "b")
    print("x " + 2.5)
    print("n " + 7)
    print("b " + true)`),
		"ab", "x 2.5", "n 7", "b true")
}

func TestComparisonsAndLogic(t *testing.T) {
	wantOut(t, inMain(`
    print(1 < 2)
    print(2.5 >= 2)
    print("a" < "b")
    print(1 == 1)
    print(1 != 1)
    print(true && false)
    print(true || false)
    print(!true)`),
		"true", "true", "true", "true", "false", "false", "true", "false")
}

func TestShortCircuit(t *testing.T) {
	// If || evaluated its right side, the division by zero would throw.
	wantOut(t, inMain(`
    let n: Int = 0
    if true || 1 / n == 0 {
        print("short circuited")
    }`),
		"short circuited")
}

func TestControlFlow(t *testing.T) {
	wantOut(t, inMain(`
    let i: Int = 0
    while i < 3 {
        print(i)
        i = i + 1
    }
    if i == 3 {
        print("three")
    } else if i == 2 {
        print("two")
    } else {
        print("other")
    }
    for (x: Int in [10, 20]) {
        print(x)
    }`),
		"0", "1", "2", "three", "10", "20")
}

func TestFieldsAndMethodsThroughObject(t *testing.T) {
	src := `
class Counter {
    n: Int

    Counter(start: Int) { object.n = start }

    func bump(): Void { object.n = object.n + 1 }
    func doubled(): Int { return object.n * 2 }
    func describe(): String { return "n=" + object.doubled() }
}
` + inMain(`
    let c: Counter = Counter(5)
    print(c.n)
    c.bump()
    print(c.n)
    print(c.describe())`)
	wantOut(t, src, "5", "6", "n=12")
}

// The single most important behaviour in the language: a call through a
// [Shape] must reach each concrete class's own area().
func TestDynamicDispatch(t *testing.T) {
	src := `
interface Shape {
    func area(): Float
    func describe(): String
}

abstract class BaseShape : Shape {
    name: String

    BaseShape(name: String) { object.name = name }

    func describe(): String {
        return object.name + " has area " + object.area()
    }
}

class Rectangle : BaseShape {
    width: Float
    height: Float

    Rectangle(name: String, width: Float, height: Float) {
        object.name = name
        object.width = width
        object.height = height
    }

    func area(): Float { return object.width * object.height }
}

class Circle : BaseShape {
    radius: Float

    Circle(name: String, radius: Float) {
        object.name = name
        object.radius = radius
    }

    func area(): Float { return 3.14159 * object.radius * object.radius }
}
` + inMain(`
    let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0), Circle("Circ1", 3.0)]
    for (s: Shape in shapes) {
        print(s.area())
        print(s.describe())
    }`)
	wantOut(t, src,
		"20.0", "Rect1 has area 20.0",
		"28.274309999999996", "Circ1 has area 28.274309999999996")
}

// Spec gap 2: a class with no constructor inherits its parent's.
func TestInheritedConstructor(t *testing.T) {
	src := `class NegativeAreaError : Exception { }
` + inMain(`
    let e: NegativeAreaError = NegativeAreaError("boom")
    print(e.message)`)
	wantOut(t, src, "boom")
}

func TestTryCatchExactType(t *testing.T) {
	src := `class E : Exception { }
` + inMain(`
    try {
        throw E("caught me")
    } catch (e: E) {
        print("got: " + e.message)
    }`)
	wantOut(t, src, "got: caught me")
}

func TestCatchMatchesASubclass(t *testing.T) {
	src := `class Base : Exception { }
class Derived : Base { }
` + inMain(`
    try {
        throw Derived("derived")
    } catch (e: Base) {
        print("caught as Base: " + e.message)
    }`)
	wantOut(t, src, "caught as Base: derived")
}

func TestUnmatchedTypePropagatesToAnOuterTry(t *testing.T) {
	src := `class A : Exception { }
class B : Exception { }
` + inMain(`
    try {
        try {
            throw B("inner")
        } catch (e: A) {
            print("wrong handler")
        }
    } catch (e: B) {
        print("outer caught " + e.message)
    }`)
	wantOut(t, src, "outer caught inner")
}

func TestFirstMatchingCatchWins(t *testing.T) {
	src := `class A : Exception { }
class B : Exception { }
` + inMain(`
    try {
        throw A("a")
    } catch (e: B) {
        print("B")
    } catch (e: A) {
        print("A")
    }`)
	wantOut(t, src, "A")
}

func TestThrowUnwindsThroughNestedCalls(t *testing.T) {
	src := `class E : Exception { }

func deep(): Int {
    throw E("from deep")
}

func middle(): Int {
    return deep()
}
` + inMain(`
    try {
        print(middle())
    } catch (e: E) {
        print("caught " + e.message)
    }`)
	wantOut(t, src, "caught from deep")
}

func TestReturnCrossingATryIsNotCaught(t *testing.T) {
	src := `class E : Exception { }

func f(): Int {
    try {
        return 42
    } catch (e: E) {
        return 0
    }
}
` + inMain(`print(f())`)
	wantOut(t, src, "42")
}

// Review Focus 4: runtime failures must be catchable Flint exceptions, never a
// Go panic escaping the interpreter.
func TestArithmeticErrors(t *testing.T) {
	tests := []struct {
		name string
		expr string
		want string
	}{
		{"division by zero", "1 / z", "division by zero"},
		{"modulo by zero", "1 % z", "modulo by zero"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := inMain(`
    let z: Int = 0
    try {
        print(` + tt.expr + `)
    } catch (e: ArithmeticError) {
        print("caught: " + e.message)
    }`)
			wantOut(t, src, "caught: "+tt.want)
		})
	}
}

func TestFloatDivisionByZeroDoesNotThrow(t *testing.T) {
	// IEEE semantics, as in Java: only integer division traps.
	wantOut(t, inMain(`
    let z: Float = 0.0
    print(1.0 / z)`),
		"+Inf")
}

func TestIndexErrors(t *testing.T) {
	tests := []struct {
		name string
		expr string
	}{
		{"past the end", "l[5]"},
		{"negative", "l[-1]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := inMain(`
    let l: [Int] = [1, 2]
    try {
        print(` + tt.expr + `)
    } catch (e: IndexError) {
        print("caught: " + e.message)
    }`)
			got := run(t, src)
			if !strings.Contains(got, "caught: list index") {
				t.Fatalf("got %q, want a caught IndexError", got)
			}
		})
	}
}

func TestMissingMapKeyThrows(t *testing.T) {
	src := inMain(`
    let m: [String: Int] = ["a": 1]
    try {
        print(m["b"])
    } catch (e: IndexError) {
        print("caught: " + e.message)
    }`)
	wantOut(t, src, "caught: no entry for key b")
}

// Spec gap 3: field defaults.
func TestFieldDefaults(t *testing.T) {
	src := `
class Other { }
class Holder {
    n: Int
    f: Float
    b: Bool
    s: String
    l: [Int]
    o: Other

    Holder() { }
}
` + inMain(`
    let h: Holder = Holder()
    print(h.n)
    print(h.f)
    print(h.b)
    print("s=" + h.s)
    print(h.l)`)
	wantOut(t, src, "0", "0.0", "false", "s=", "[]")
}

func TestReadingAnUninitializedObjectFieldThrows(t *testing.T) {
	src := `
class Other { func hi(): Void { print("hi") } }
class Holder {
    o: Other
    Holder() { }
}
` + inMain(`
    let h: Holder = Holder()
    try {
        h.o.hi()
    } catch (e: NullError) {
        print("caught a NullError")
    }`)
	wantOut(t, src, "caught a NullError")
}

func TestLenBuiltin(t *testing.T) {
	wantOut(t, inMain(`
    print(len([1, 2, 3]))
    print(len(["a": 1]))
    print(len("hello"))
    print(len([]))`),
		"3", "1", "5", "0")
}

func TestCollectionsAreMutableAndPassedByReference(t *testing.T) {
	src := `
func fill(l: [Int]): Void {
    l[0] = 99
}
` + inMain(`
    let l: [Int] = [1, 2]
    fill(l)
    print(l)
    let m: [String: Int] = [:]
    m["k"] = 7
    print(m)`)
	wantOut(t, src, "[99, 2]", "[k: 7]")
}

func TestObjectsCompareByIdentity(t *testing.T) {
	src := `
class C { C() { } }
` + inMain(`
    let a: C = C()
    let b: C = C()
    print(a == a)
    print(a == b)`)
	wantOut(t, src, "true", "false")
}

func TestUncaughtExceptionIsReturnedAsAnError(t *testing.T) {
	src := `class Boom : Exception { }
` + inMain(`
    print("before")
    throw Boom("it broke")`)
	out, err := runErr(t, src)
	if err == nil {
		t.Fatal("Run returned nil, want an error for the uncaught exception")
	}
	if !strings.Contains(err.Error(), "Boom") || !strings.Contains(err.Error(), "it broke") {
		t.Fatalf("error = %v, want it to name the class and message", err)
	}
	if !strings.Contains(out, "before") {
		t.Fatalf("output %q lost the work done before the throw", out)
	}
}

func TestRecursion(t *testing.T) {
	src := `
func fact(n: Int): Int {
    if n <= 1 {
        return 1
    }
    return n * fact(n - 1)
}
` + inMain(`print(fact(10))`)
	wantOut(t, src, "3628800")
}

func TestShadowingInNestedScopes(t *testing.T) {
	wantOut(t, inMain(`
    let x: Int = 1
    if true {
        let x: Int = 2
        print(x)
    }
    print(x)`),
		"2", "1")
}
