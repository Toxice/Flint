package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flint/internal/lexer"
	"flint/internal/parser"
	"flint/internal/types"
)

// analyse lexes, parses, and checks src. A `func main(): Void { }` is appended
// when src does not declare one, since every program needs an entry point.
func analyse(t *testing.T, src string) (*Info, []error) {
	t.Helper()
	if !strings.Contains(src, "func main(") {
		src += "\nfunc main(): Void { }\n"
	}
	toks, lexErrs := lexer.Scan(src, "test.flint")
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, parseErrs := parser.Parse(toks, "test.flint")
	if len(parseErrs) != 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	return Check(prog, "test.flint")
}

// mustCheck requires src to check cleanly.
func mustCheck(t *testing.T, src string) *Info {
	t.Helper()
	info, errs := analyse(t, src)
	if len(errs) != 0 {
		t.Fatalf("unexpected check errors: %v", errs)
	}
	return info
}

// wantErr requires at least one error containing want.
func wantErr(t *testing.T, src, want string) {
	t.Helper()
	_, errs := analyse(t, src)
	if len(errs) == 0 {
		t.Fatalf("checked cleanly, want an error mentioning %q", want)
	}
	for _, e := range errs {
		if strings.Contains(e.Error(), want) {
			return
		}
	}
	t.Fatalf("errors = %v, want one mentioning %q", errs, want)
}

// ---------- pass 1: declaration collection ----------

func TestInterfaceIsRegistered(t *testing.T) {
	info := mustCheck(t, "interface Shape { func area(): Float }")
	iface, ok := info.Interfaces["Shape"]
	if !ok {
		t.Fatal("interface Shape not registered")
	}
	m, ok := iface.Methods["area"]
	if !ok {
		t.Fatal("method area not registered")
	}
	if m.Sig.Result != types.Float || len(m.Sig.Params) != 0 {
		t.Fatalf("area signature = %s, want (): Float", m.Sig)
	}
}

func TestSuperclassIsLinked(t *testing.T) {
	info := mustCheck(t, "abstract class BaseShape { }\nclass Rectangle : BaseShape { }")
	rect := info.Classes["Rectangle"]
	if rect.Super != info.Classes["BaseShape"] {
		t.Fatalf("Super = %v, want the BaseShape class object", rect.Super)
	}
	if len(rect.Interfaces) != 0 {
		t.Fatalf("Interfaces = %v, want none", rect.Interfaces)
	}
}

// Inheritance and interface conformance share one syntax, so the checker must
// tell a class base from an interface base by looking the name up.
func TestMixedBaseListIsSplitCorrectly(t *testing.T) {
	src := `
interface Comparable { func compare(): Int }
abstract class BaseShape { }
class Circle : BaseShape, Comparable {
    func compare(): Int { return 0 }
}`
	info := mustCheck(t, src)
	circ := info.Classes["Circle"]
	if circ.Super != info.Classes["BaseShape"] {
		t.Fatalf("Super = %v, want BaseShape", circ.Super)
	}
	if len(circ.Interfaces) != 1 || circ.Interfaces[0] != info.Interfaces["Comparable"] {
		t.Fatalf("Interfaces = %v, want [Comparable]", circ.Interfaces)
	}
}

func TestDuplicateDeclarations(t *testing.T) {
	wantErr(t, "class A { }\nclass A { }", "already declared")
	wantErr(t, "class A { }\ninterface A { }", "already declared")
	wantErr(t, "class Int { }", "already declared")
	wantErr(t, "func f(): Void { }\nfunc f(): Void { }", "already declared")
	wantErr(t, "class A {\n n: Int\n n: Int\n}", "field n more than once")
	wantErr(t, "class A {\n func m(): Void { }\n func m(): Void { }\n}", "method m more than once")
}

// Review Focus 3: a cycle must be reported, not recursed into forever.
func TestInheritanceCycleIsReportedAndSevered(t *testing.T) {
	done := make(chan struct{})
	var errs []error
	go func() {
		defer close(done)
		_, errs = analyse(t, "class A : B { }\nclass B : A { }")
	}()
	<-done

	if len(errs) == 0 {
		t.Fatal("no error for an inheritance cycle")
	}
	var found bool
	for _, e := range errs {
		if strings.Contains(e.Error(), "inheritance cycle") {
			found = true
		}
	}
	if !found {
		t.Fatalf("errors = %v, want one mentioning an inheritance cycle", errs)
	}
}

func TestSelfInheritanceIsACycle(t *testing.T) {
	wantErr(t, "class A : A { }", "inheritance cycle")
}

func TestUnknownBase(t *testing.T) {
	wantErr(t, "class A : Nope { }", "unknown type Nope")
}

func TestMultipleSuperclassesRejected(t *testing.T) {
	wantErr(t, "class A { }\nclass B { }\nclass C : A, B { }", "single inheritance")
}

func TestUnknownTypeInABody(t *testing.T) {
	wantErr(t, "func f(): Void { let x: Nope = 1 }", "unknown type Nope")
}

func TestBuiltinExceptionsArePreRegistered(t *testing.T) {
	info := mustCheck(t, "func main(): Void { }")
	exc, ok := info.Classes["Exception"]
	if !ok {
		t.Fatal("Exception is not registered")
	}
	f := exc.LookupField("message")
	if f == nil || f.Type != types.String {
		t.Fatalf("Exception.message = %#v, want a String field", f)
	}
	if exc.LookupCtor() == nil {
		t.Fatal("Exception has no constructor")
	}
	for _, name := range BuiltinExceptions {
		cl, ok := info.Classes[name]
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if cl.Super != exc {
			t.Fatalf("%s.Super = %v, want Exception", name, cl.Super)
		}
	}
}

// ---------- pass 2: bodies ----------

func TestLetTypeMismatch(t *testing.T) {
	wantErr(t, `func f(): Void { let x: Int = "s" }`, "cannot use String as Int")
}

// Int widens to Float; the spec's own sample compares a Float against 0.
func TestIntWidensToFloat(t *testing.T) {
	mustCheck(t, "func f(): Void { let x: Float = 1 }")
	mustCheck(t, "func f(): Void { let a: Float = 1.0\n if a < 0 { } }")
	wantErr(t, "func f(): Void { let x: Int = 1.0 }", "cannot use Float as Int")
}

func TestBinaryOperatorTypes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string // the declared type the expression must fit
	}{
		{"int plus int", "let x: Int = 1 + 2", ""},
		{"int plus float", "let x: Float = 1 + 2.0", ""},
		{"string concat", `let x: String = "a" + "b"`, ""},
		// The spec concatenates a Float onto a String in BaseShape.describe.
		{"string plus float", `let x: String = "a" + 1.5`, ""},
		{"float plus string", `let x: String = 1.5 + "a"`, ""},
		{"string plus int", `let x: String = "a" + 1`, ""},
		{"comparison", "let x: Bool = 1 < 2.0", ""},
		{"equality", "let x: Bool = 1 == 2", ""},
		{"logical", "let x: Bool = true && false", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mustCheck(t, "func f(): Void { "+tt.src+" }")
		})
	}
}

func TestBinaryOperatorErrors(t *testing.T) {
	wantErr(t, "func f(): Void { let x: Int = true + 1 }", "numeric operands")
	wantErr(t, "func f(): Void { let x: Int = 1 - true }", "numeric operands")
	wantErr(t, "func f(): Void { let x: Bool = true && 1 }", "Bool operands")
	wantErr(t, "func f(): Void { let x: Bool = true < false }", "cannot compare")
	wantErr(t, `func f(): Void { let x: Bool = 1 == "a" }`, "cannot compare")
	wantErr(t, "func f(): Void { let x: Int = -true }", "cannot negate")
	wantErr(t, "func f(): Void { let x: Bool = !1 }", "cannot apply !")
}

func TestConditionMustBeBool(t *testing.T) {
	wantErr(t, "func f(): Void { if 1 { } }", "if condition must be Bool")
	wantErr(t, "func f(): Void { while 1 { } }", "while condition must be Bool")
}

func TestCallArity(t *testing.T) {
	src := "class C {\n func area(): Float { return 1.0 }\n}\nfunc f(): Void {\n let c: C = C()\n let a: Float = c.area(1)\n}"
	wantErr(t, src, "takes 0 argument(s), found 1")
}

func TestArgumentTypeMismatch(t *testing.T) {
	src := "func g(a: Float): Void { }\nfunc f(): Void { g(\"s\") }"
	wantErr(t, src, "cannot use String as Float")
}

func TestObjectOnlyInsideAClass(t *testing.T) {
	wantErr(t, "func f(): Void { let x: Int = object.n }", "object is only valid inside a class body")
}

func TestPrivateAccess(t *testing.T) {
	src := `
class C {
    private secret: Int
    private func hidden(): Void { }
    func ok(): Int { return object.secret }
}
func f(): Void {
    let c: C = C()
    let n: Int = c.secret
}`
	wantErr(t, src, "private to class C")

	methodSrc := `
class C {
    private func hidden(): Void { }
}
func f(): Void {
    let c: C = C()
    c.hidden()
}`
	wantErr(t, methodSrc, "private to class C")
}

// Review Focus 5: conformance.
func TestConcreteClassMustImplementItsInterface(t *testing.T) {
	src := `
interface Shape { func area(): Float }
class Square : Shape { }`
	wantErr(t, src, "does not implement area")
}

func TestAbstractClassMayLeaveMethodsOpen(t *testing.T) {
	mustCheck(t, "interface Shape { func area(): Float }\nabstract class Partial : Shape { }")
}

func TestConcreteSubclassMustImplementInheritedAbstractMethods(t *testing.T) {
	src := `
abstract class B { abstract func area(): Float }
class D : B { }`
	wantErr(t, src, "does not implement area")
}

func TestCannotInstantiateAbstractClass(t *testing.T) {
	wantErr(t, "abstract class B { }\nfunc f(): Void { let b: B = B() }", "cannot instantiate abstract class B")
}

func TestCannotInstantiateInterface(t *testing.T) {
	wantErr(t, "interface I { }\nfunc f(): Void { let i: I = I() }", "cannot instantiate interface I")
}

func TestOverrideMustMatchTheSignature(t *testing.T) {
	src := `
interface Shape { func area(): Float }
class Bad : Shape { func area(): Int { return 1 } }`
	wantErr(t, src, "is required")
}

func TestMissingReturn(t *testing.T) {
	wantErr(t, "func f(): Int { }", "must return Int on every path")
	wantErr(t, "func f(): Int { if true { return 1 } }", "must return Int on every path")
	// Both branches returning is enough.
	mustCheck(t, "func f(): Int { if true { return 1 } else { return 2 } }")
	// A throw also terminates the path.
	mustCheck(t, "func f(): Int { throw Exception(\"x\") }")
	// A Void function needs no return.
	mustCheck(t, "func f(): Void { }")
	wantErr(t, "func f(): Void { return 1 }", "cannot return a value from a Void function")
	wantErr(t, "func f(): Int { return }", "must return Int")
}

func TestForInBindsTheElementType(t *testing.T) {
	src := `
interface Shape { func area(): Float }
class Sq : Shape { func area(): Float { return 1.0 } }
func f(shapes: [Shape]): Void {
    for (shape: Shape in shapes) {
        let a: Float = shape.area()
    }
}`
	mustCheck(t, src)
}

func TestForInErrors(t *testing.T) {
	wantErr(t, "func f(): Void { for (x: Int in 5) { } }", "for-in needs a list")
	wantErr(t, `func f(): Void { for (x: Int in ["a"]) { } }`, "loop variable x is declared Int, but the list holds String")
}

func TestCatchTypeMustBeAnException(t *testing.T) {
	wantErr(t, "func f(): Void { try { } catch (e: Int) { } }", "catch type must be Exception")
	mustCheck(t, "class E : Exception { }\nfunc f(): Void { try { } catch (e: E) { } }")
}

func TestThrowMustBeAnException(t *testing.T) {
	wantErr(t, "func f(): Void { throw 5 }", "can only throw an Exception")
	mustCheck(t, `func f(): Void { throw Exception("boom") }`)
}

// Spec gap 2: a class with no constructor inherits its parent's.
func TestInheritedConstructor(t *testing.T) {
	mustCheck(t, `class NegativeAreaError : Exception { }
func f(): Void { throw NegativeAreaError("Area cannot be negative") }`)
	wantErr(t, "class E : Exception { }\nfunc f(): Void { throw E() }", "takes 1 argument(s), found 0")
}

func TestBuiltins(t *testing.T) {
	mustCheck(t, `func f(): Void { print("x") }`)
	mustCheck(t, "func f(): Void { print(1) }")
	mustCheck(t, "func f(): Void { let n: Int = len([1, 2]) }")
	mustCheck(t, `func f(): Void { let n: Int = len("abc") }`)
	mustCheck(t, `func f(): Void { let n: Int = len(["a": 1]) }`)
	wantErr(t, "func f(): Void { let n: Int = len(1) }", "len needs a list")
	wantErr(t, "func f(): Void { print() }", "takes exactly 1 argument")
	wantErr(t, "func f(): Void { print(f()) }", "cannot print a Void value")
}

func TestCollectionLiteralsAreTypedByContext(t *testing.T) {
	// Lists are invariant, so the literal must be checked against the declared
	// element type rather than inferred and then compared.
	src := `
interface Shape { func area(): Float }
class A : Shape { func area(): Float { return 1.0 } }
class B : Shape { func area(): Float { return 2.0 } }
func f(): Void {
    let shapes: [Shape] = [A(), B()]
    let empty: [Shape] = []
    let m: [String: Int] = ["a": 1]
    let me: [String: Int] = [:]
    let widened: [Float] = [1, 2.0]
}`
	mustCheck(t, src)
	wantErr(t, `func f(): Void { let x: [Int] = ["a"] }`, "cannot use String as Int")
	wantErr(t, `func f(): Void { let x: [String: Int] = ["a": "b"] }`, "cannot use String as the value type Int")
}

func TestIndexing(t *testing.T) {
	mustCheck(t, "func f(): Void { let l: [Int] = [1]\n let n: Int = l[0] }")
	mustCheck(t, `func f(): Void { let m: [String: Int] = ["a": 1]
 let n: Int = m["a"] }`)
	wantErr(t, `func f(): Void { let l: [Int] = [1]
 let n: Int = l["a"] }`, "list index must be Int")
	wantErr(t, "func f(): Void { let n: Int = 5[0] }", "cannot index")
}

func TestUndefinedNames(t *testing.T) {
	wantErr(t, "func f(): Void { let x: Int = nope }", "undefined variable nope")
	wantErr(t, "func f(): Void { nope() }", "undefined function nope")
	wantErr(t, "class C { }\nfunc f(): Void { let c: C = C()\n c.nope() }", "class C has no method nope")
	wantErr(t, "class C { }\nfunc f(): Void { let c: C = C()\n let n: Int = c.nope }", "class C has no field nope")
}

func TestFieldAndMethodConfusion(t *testing.T) {
	wantErr(t, "class C {\n n: Int\n}\nfunc f(): Void { let c: C = C()\n c.n() }", "is a field on C, not a method")
	wantErr(t, "class C {\n func m(): Int { return 1 }\n}\nfunc f(): Void { let c: C = C()\n let x: Int = c.m }", "is a method on C")
}

func TestMainIsRequired(t *testing.T) {
	_, errs := analyse(t, "func notMain(): Void { }\nfunc main(): Int { return 1 }")
	if len(errs) == 0 {
		t.Fatal("main with the wrong signature was accepted")
	}
	toks, _ := lexer.Scan("func g(): Void { }", "test.flint")
	prog, _ := parser.Parse(toks, "test.flint")
	if _, errs := Check(prog, "test.flint"); len(errs) == 0 {
		t.Fatal("a program with no main was accepted")
	}
}

// The spec's own sample must check with zero errors.
func TestSampleProgramChecks(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "shapes.flint")
	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading sample: %v", err)
	}
	toks, lexErrs := lexer.Scan(string(src), path)
	if len(lexErrs) != 0 {
		t.Fatalf("lex errors: %v", lexErrs)
	}
	prog, parseErrs := parser.Parse(toks, path)
	if len(parseErrs) != 0 {
		t.Fatalf("parse errors: %v", parseErrs)
	}
	if _, errs := Check(prog, path); len(errs) != 0 {
		t.Fatalf("check errors: %v", errs)
	}
}
