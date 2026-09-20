# Flint Interpreter Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A working tree-walking interpreter for Flint that runs the spec's sample program end to end, with mandatory type annotations enforced statically before execution.

**Architecture:** Classic four-stage pipeline, each stage a separate `internal/` package with no back-edges: `lexer` -> `parser` (hand-written recursive descent, precedence climbing for expressions) -> `checker` (two-pass: collect all type declarations, then check bodies; collects *all* errors, not just the first) -> `interp` (tree-walking evaluator over the AST). Exception unwinding and `return` use `panic`/`recover` with package-private sentinel types, recovered at `try`/`catch`, function-call, and top-level boundaries -- the standard Go idiom for non-local control flow in an evaluator (`encoding/json`, `text/template`), kept entirely inside the package.

**Tech Stack:** Go 1.21+, standard library only. No external dependencies, no code generation, no parser generator.

**Spec:** `flint-language-spec.md` (repo root)

---

## Global Constraints

Copied verbatim from the spec; every task's requirements implicitly include these.

- **Primitives:** `Int`, `Float`, `Bool`, `String`, `Char`, `Void` (no return value).
- **"Annotations are mandatory on every variable, parameter, return type, and field -- there is no type inference."** A missing annotation is a *parse* error, not a checker error.
- **Syntax is Python-style, `name: Type`, rather than Java's `Type name`.**
- **Collections use bracket types, not `<>`:** `[Type]` for a list, `[Key: Value]` for a map. Collection literals use the same brackets.
- **"Only these built-in parameterized types exist -- there is no user-defined generics system."** No type parameters anywhere in the grammar.
- **The constructor is a method with the same name as the class.**
- **`object` refers to the current instance** -- Flint's replacement for `this`/`self`. `object` is a reserved word.
- **Inheritance and interface conformance share one syntax** -- a single `:` list. No `extends`/`implements` keywords. `class Circle : BaseShape, Comparable { ... }`.
- **Members are `public` by default.** `private` hides a member. `public` is accepted but never required.
- **`abstract class` and `abstract func`** declare a method with no body, filled in by a subclass.
- **Exceptions are unchecked** -- no `throws` clause exists in the grammar. Custom exceptions are ordinary classes inheriting from `Exception`.
- **`if`, `while`, `for` drop the parentheses around their condition but keep braces around the body.** Braces are mandatory -- there is no single-statement form.
- **`for (shape: Shape in shapes) { ... }`** -- the for-in header *does* keep its parentheses, per the spec's own sample. Only `if`/`while` drop them.
- **Out of scope, do not build:** generics beyond `[T]`/`[K:V]`, concurrency, pattern matching, modules/imports, a `super` keyword (the spec defines none), `throws` declarations.

## Spec Gaps and Resolutions

The spec is a design document; these four points it does not settle, resolved here and documented in the README.

1. **No `super` call exists.** The sample's `Rectangle` ctor never sets `object.name`. Resolution: fields are public and inherited, so a subclass ctor assigns `object.name = name` directly. No `super` keyword is added. The sample runs as written; `name` simply stays at its default empty string, which is unobservable because `main` never calls `describe()`.
2. **`NegativeAreaError("...")` is constructed but declares no ctor.** Resolution: a class that declares no constructor **inherits its nearest ancestor's**. Required for the sample to run.
3. **Field default values.** Resolution: `Int` -> `0`, `Float` -> `0.0`, `Bool` -> `false`, `String` -> empty, `Char` -> NUL, `[T]` -> empty list, `[K:V]` -> empty map, class/interface types -> *uninitialized*, which throws `NullError` on use rather than crashing the host.
4. **No comment syntax is specified.** Resolution: `//` to end of line. A language with no comments is unusable; block comments are not added.

## Extensions Beyond the Spec

Kept to the minimum that makes the language runnable. Each is documented in the README as an extension.

- `print(x)` -- the spec uses it throughout but never declares it. Accepts one argument of any type, returns `Void`.
- `len(x): Int` -- for `[T]`, `[K:V]`, and `String`.
- `Exception` -- built-in root class with one public field `message: String` and ctor `Exception(message: String)`, since the spec's `e.message` requires it.
- `ArithmeticError`, `IndexError`, `NullError` -- built-in subclasses of `Exception` so runtime failures are catchable Flint values rather than host crashes.

## Review Focus

Input classes the spec implies but no feature task naturally exercises. Each line names the test that pins it and the task that owns it.

1. **Unterminated string/char literal and a stray `@`** -- must be a positioned lexer error, not a hang or a host panic. -> Task 2 lexer error tests.
2. **`[]` vs `[:]` vs `[1: 2]`** -- an empty bracket literal is ambiguous between list and map. `[]` is an empty list, `[:]` an empty map, and a map literal is detected by a `:` after the first element. -> Task 4 parser tests.
3. **Inheritance cycle (`class A : B`, `class B : A`) and a class listing an unknown base** -- must be a reported checker error, not infinite recursion in the checker or the interpreter. -> Task 6 checker tests.
4. **Integer division and modulo by zero, and list index out of range** -- must throw a catchable Flint `ArithmeticError`/`IndexError` with a useful message, not a Go panic that escapes the interpreter. -> Task 9 interpreter tests.
5. **A concrete class that leaves an interface or abstract method unimplemented, and instantiating an abstract class** -- must be a checker error naming the missing method. -> Task 7 checker tests.

---

## File Structure

```
Flint/
|- go.mod                          module flint, go 1.21
|- README.md                       what it is, how to run, spec gaps, extensions
|- flint-language-spec.md          (existing, untouched)
|- cmd/flint/main.go               CLI: flint run <file>, flint check <file>
|- internal/token/token.go         Kind enum, Token, keyword table, Pos
|- internal/lexer/lexer.go         source -> []Token
|   \- lexer_test.go
|- internal/ast/ast.go             node structs, Expr/Stmt/Decl/TypeExpr interfaces
|- internal/parser/parser.go       []Token -> *ast.Program
|   \- parser_test.go
|- internal/types/types.go         Type model, Class/Interface, Assignable, Conforms
|   \- types_test.go
|- internal/checker/checker.go     *ast.Program -> []Error, populates types
|   \- checker_test.go
|- internal/interp/value.go        runtime Value model, stringify, defaults
|- internal/interp/builtin.go      print, len, Exception hierarchy bootstrap
|- internal/interp/interp.go       evaluator
|   \- interp_test.go
|- examples/shapes.flint           the spec's sample program, verbatim
\- testdata/                       golden end-to-end programs: prog.flint + prog.out
    \- e2e_test.go                 runs every testdata pair
```

Each stage is one package so a stage can be tested without the ones after it. `types` is separate from `checker` because `interp` needs the type model (for `catch` matching and method dispatch) but must not import the checker.

---

## Task 1: Module scaffold and token definitions

**Files:**
- Create: `go.mod`, `internal/token/token.go`

**Interfaces -- Produces:**
- `token.Kind` (int enum), `token.Token{Kind Kind, Lit string, Line, Col int}`
- `token.Lookup(ident string) Kind` -- keyword table, returns `IDENT` if not a keyword
- `func (k Kind) String() string` -- for error messages

- [x] **Step 1:** `go mod init flint` with `go 1.21`.
- [x] **Step 2:** Define every `Kind`: `EOF, ILLEGAL, IDENT, INT, FLOAT, STRING, CHAR`; keywords `LET, FUNC, CLASS, INTERFACE, ABSTRACT, PRIVATE, PUBLIC, RETURN, IF, ELSE, WHILE, FOR, IN, TRY, CATCH, THROW, OBJECT, TRUE, FALSE`; punctuation `LPAREN RPAREN LBRACE RBRACE LBRACK RBRACK COMMA COLON DOT ASSIGN`; operators `PLUS MINUS STAR SLASH PERCENT EQ NE LT GT LE GE ANDAND OROR BANG`.
- [x] **Step 3:** `Kind.String()` returns the source spelling so parser errors read naturally.
- [x] **Step 4:** `go vet ./...` and commit.

## Task 2: Lexer

**Files:**
- Create: `internal/lexer/lexer.go`, `internal/lexer/lexer_test.go`

**Interfaces -- Consumes:** `token`. **Produces:** `lexer.Scan(src, filename string) ([]token.Token, []error)`.

Scan returns every token plus every error found, so one bad character does not hide the rest of the file. Line/col are 1-based.

- [x] **Step 1: Write the failing tests.** Table-driven, one subtest per case:
  - every keyword and punctuation mark round-trips to its `Kind`
  - `21` -> `INT`; `4.0` -> `FLOAT`; `3.14159` -> `FLOAT`; `4.` -> error; `.5` -> `DOT` then `INT` (no leading-dot floats)
  - a double-quoted string yields `STRING` with the quotes stripped; escapes `\n`, `\t`, `\"`, `\\` decode
  - a single-quoted `a` yields `CHAR`; an escaped newline char literal yields `CHAR`
  - `//` comment skipped to end of line; a comment on the last line with no trailing newline terminates cleanly
  - `<=` lexes as one `LE`, not `LT` then `ASSIGN`; likewise `>=`, `==`, `!=`, `&&`, `||`
  - `=` alone is `ASSIGN`, `!` alone is `BANG`
  - **Review Focus 1:** an unterminated string literal produces one positioned error and Scan still returns (no hang); a stray `@` produces one `ILLEGAL` token with position and an error
  - line/col tracking across a multi-line input
- [x] **Step 2:** Run `go test ./internal/lexer/ -v`. Expected: FAIL, package does not compile.
- [x] **Step 3:** Implement. Single pass over `[]rune`, `switch` on the current rune, two-rune operators peeked before one-rune fallbacks.
- [x] **Step 4:** `go test ./internal/lexer/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 3: AST

**Files:**
- Create: `internal/ast/ast.go`

**Interfaces -- Produces:** marker interfaces `Node` (`Pos() token.Pos`), `Expr`, `Stmt`, `Decl`, `TypeExpr`, and the node set below. Later tasks use these names verbatim.

- **TypeExpr:** `NamedType{Name}`, `ListType{Elem}`, `MapType{Key, Val}`
- **Expr:** `IntLit{Value int64}`, `FloatLit{Value float64}`, `StringLit{Value string}`, `CharLit{Value rune}`, `BoolLit{Value bool}`, `Ident{Name}`, `ObjectExpr{}`, `Unary{Op, X}`, `Binary{Op, X, Y}`, `Call{Fn, Args}`, `Field{X, Name}`, `Index{X, Index}`, `ListLit{Elems}`, `MapLit{Keys, Vals}`
- **Stmt:** `Let{Name, Type, Value}`, `Assign{Target, Value}`, `ExprStmt{X}`, `Block{Stmts}`, `If{Cond, Then, Else}`, `While{Cond, Body}`, `For{Var, Type, Iter, Body}`, `Return{Value}`, `Throw{Value}`, `Try{Body, Catches}`, `CatchClause{Name, Type, Body}`
- **Decl:** `FuncDecl{Name, Params, Result, Body, Abstract, Private}`, `Param{Name, Type}`, `FieldDecl{Name, Type, Private}`, `ClassDecl{Name, Abstract, Bases, Fields, Ctor, Methods}`, `InterfaceDecl{Name, Methods}`, `Program{Decls}`

No test task -- the AST is data with no behavior. It is exercised by Task 4.

- [x] **Step 1:** Write the file; every node carries `P token.Pos` and implements `Pos()`.
- [x] **Step 2:** `go build ./...`, commit.

## Task 4: Parser

**Files:**
- Create: `internal/parser/parser.go`, `internal/parser/parser_test.go`

**Interfaces -- Consumes:** `token`, `ast`. **Produces:** `parser.Parse(toks []token.Token) (*ast.Program, []error)`.

Recursive descent for declarations and statements; precedence climbing for expressions, lowest to highest: `||`, `&&`, `==`/`!=`, `<`/`>`/`<=`/`>=`, `+`/`-`, `*`/`/`/`%`, unary `-`/`!`, postfix call/field/index, primary. On a parse error, record it and skip to the next `}` or top-level keyword so one error does not cascade.

- [x] **Step 1: Write the failing tests.**
  - `let age: Int = 21` -> `Let` with `NamedType{"Int"}`
  - `let x = 1` (no annotation) -> error mentioning the missing type. **Global Constraint: annotations are mandatory, this is a parse error**
  - `let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0), Circle("Circ1", 3.0)]` -> `ListType` plus `ListLit` of two `Call`s
  - `let counts: [String: Int] = ["a": 1, "b": 2]` -> `MapType` plus `MapLit`
  - **Review Focus 2:** `[]` -> empty `ListLit`; `[:]` -> empty `MapLit`; `[1: 2]` -> `MapLit`; `[1, 2]` -> `ListLit`
  - `a + b * c` groups as `a + (b * c)`; `a - b - c` groups as `(a - b) - c`; `!a && b` as `(!a) && b`
  - `object.width * object.height` -> `Binary` of two `Field` whose `X` is `ObjectExpr`
  - `if x > 5 { }` parses; `if (x > 5) { }` also parses (parens are just a grouped expr); `if x > 5 print(x)` -> error, braces required
  - `for (shape: Shape in shapes) { }` -> `For{Var:"shape", Type:NamedType{"Shape"}, Iter:Ident{"shapes"}}`
  - `class Rectangle : BaseShape { ... }` -> `Bases == ["BaseShape"]`; `class Circle : BaseShape, Comparable {}` -> two bases
  - `Dog(name: String) { object.name = name }` inside `class Dog` -> parsed as `Ctor`, not a method
  - `abstract class`; `abstract func area(): Float` with no body; `private name: String`
  - `interface Shape { func area(): Float }` -> method with `Body == nil`
  - `try { } catch (e: NegativeAreaError) { }` -> one `CatchClause`; two catch clauses parse
  - `throw NegativeAreaError("Area cannot be negative")` -> `Throw` of a `Call`
  - full `examples/shapes.flint` parses with zero errors
- [x] **Step 2:** `go test ./internal/parser/ -v`. Expected: FAIL.
- [x] **Step 3:** Implement.
- [x] **Step 4:** `go test ./internal/parser/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 5: Type model

**Files:**
- Create: `internal/types/types.go`, `internal/types/types_test.go`

**Interfaces -- Produces:**
- `types.Type` interface (`String() string`)
- `types.Primitive{Name}` with package vars `Int, Float, Bool, String, Char, Void`
- `types.List{Elem Type}`, `types.Map{Key, Val Type}`
- `types.Class{Name, Super *Class, Interfaces []*Interface, Fields map[string]*Field, Methods map[string]*Signature, Ctor *Signature, Abstract bool}`
- `types.Interface{Name, Methods map[string]*Signature}`
- `types.Signature{Params []Type, ParamNames []string, Result Type}`
- `types.Assignable(dst, src Type) bool`, `types.Conforms(c *Class, i *Interface) bool`, `types.IsSubclass(sub, super *Class) bool`

Assignability rules, in order: identical types; `Int` -> `Float` (widening, required by the spec's `if a < 0` where `a: Float`); subclass -> superclass; class -> any interface it conforms to (directly or via a superclass); `[A]` -> `[B]` only when `A` and `B` are identical (invariant -- covariance would be unsound with mutable lists).

- [x] **Step 1: Write the failing tests.** Table-driven over `Assignable`: identical primitives; `Int`->`Float` yes, `Float`->`Int` no; `Rectangle`->`BaseShape` yes, reverse no; `Rectangle`->`Shape` (interface, inherited from `BaseShape`) yes; `[Rectangle]`->`[Shape]` **no**; `[Int]`->`[Int]` yes; unrelated classes no. `IsSubclass` on a 3-deep chain. `String()` renders `[String: Int]` and `[Shape]`.
- [x] **Step 2:** `go test ./internal/types/ -v`. Expected: FAIL.
- [x] **Step 3:** Implement.
- [x] **Step 4:** `go test ./internal/types/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 6: Checker pass 1 -- declaration collection

**Files:**
- Create: `internal/checker/checker.go`, `internal/checker/checker_test.go`

**Interfaces -- Consumes:** `ast`, `types`. **Produces:**
- `checker.Check(prog *ast.Program) (*checker.Info, []error)`
- `checker.Info{Classes map[string]*types.Class, Interfaces map[string]*types.Interface, Funcs map[string]*types.Signature, ExprType map[ast.Expr]types.Type}` -- `ExprType` is how `interp` learns the static type of each expression without re-deriving it.

Pass 1 registers every class/interface/func name, resolves base-name lists into `Super`/`Interfaces`, resolves every `TypeExpr` to a `types.Type`, and builds signatures. Bodies are untouched.

- [x] **Step 1: Write the failing tests.**
  - `interface Shape { func area(): Float }` registers `Shape` with one method
  - `class Rectangle : BaseShape` sets `Super` to the `BaseShape` class object
  - `class Circle : BaseShape, Comparable` splits a class base from an interface base correctly -- the checker decides which is which by looking the name up, since the syntax is identical
  - duplicate class name -> error; duplicate field name -> error; duplicate method name -> error
  - **Review Focus 3:** `class A : B {}` plus `class B : A {}` -> reported "inheritance cycle" error and the test completes (no stack overflow); `class A : Nope {}` -> "unknown type Nope"
  - a class listing two class bases -> error, single inheritance only
  - `let x: Nope = ...` -> "unknown type Nope"
  - built-ins are pre-registered: `Exception` exists with field `message: String`; `ArithmeticError`, `IndexError`, `NullError` have `Super == Exception`
- [x] **Step 2:** `go test ./internal/checker/ -v`. Expected: FAIL.
- [x] **Step 3:** Implement pass 1. Cycle detection: walk `Super` with a visited set, bounded by the class count.
- [x] **Step 4:** `go test ./internal/checker/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 7: Checker pass 2 -- body and conformance checking

**Files:**
- Modify: `internal/checker/checker.go`, `internal/checker/checker_test.go`

**Interfaces:** unchanged from Task 6; `Info.ExprType` is now fully populated.

Walks every function, method, and ctor body with a lexical scope stack, recording each expression's type into `ExprType`. Then checks conformance: a **concrete** class must implement every method of every interface it claims and every `abstract func` it inherits; an **abstract** class may leave them open.

Expression typing rules the spec forces:
- `+` where **either** operand is `String` -> `String` (the spec's `object.name + " has area " + object.area()` concatenates a `Float` onto a `String`). Otherwise both must be numeric; `Float` if either is `Float`, else `Int`.
- `-`, `*`, `/`, `%` numeric only. `<`, `>`, `<=`, `>=` on `Int`/`Float`/`String`/`Char`, result `Bool`. `==`/`!=` on any two assignable-compatible types, result `Bool`. `&&`, `||`, `!` `Bool` only.
- `object` is only legal inside a class body, typed as that class.
- Method/field lookup walks `Super` then interfaces; a `private` member is visible only inside its declaring class.
- A non-`Void` function must return on every path; a `Void` function may `return` with no value.

- [x] **Step 1: Write the failing tests.**
  - `let x: Int = "s"` -> type error; `let f: Float = 1` -> **no** error (Int widens)
  - `"a" + 1.5` -> `String`; `1 + 2` -> `Int`; `1 + 2.0` -> `Float`; `true + 1` -> error
  - `if 1 { }` -> error, condition must be `Bool`
  - calling `area()` with an argument -> arity error; passing `String` where `Float` expected -> error
  - `object` at top level -> error
  - reading a `private` field from outside its class -> error
  - **Review Focus 5:** `class Square : Shape { }` (concrete, no `area`) -> error naming `area`; `abstract class Partial : Shape { }` -> no error; constructing an abstract class -> "cannot instantiate abstract class"
  - a non-`Void` func whose body falls off the end -> "missing return"
  - `for (s: Shape in shapes)` where `shapes: [Shape]` binds `s: Shape`; iterating a non-collection -> error
  - `catch (e: Int)` -> error, catch type must be `Exception` or a subclass
  - `throw 5` -> error, can only throw an `Exception`
  - the full `examples/shapes.flint` checks with **zero** errors
- [x] **Step 2:** `go test ./internal/checker/ -v`. Expected: new tests FAIL.
- [x] **Step 3:** Implement pass 2.
- [x] **Step 4:** `go test ./internal/checker/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 8: Runtime values and built-ins

**Files:**
- Create: `internal/interp/value.go`, `internal/interp/builtin.go`, `internal/interp/interp_test.go`

**Interfaces -- Produces:**
- `interp.Value` interface; concrete `Int int64`, `Float float64`, `Bool bool`, `Str string`, `Char rune`, `Unit struct{}`
- `interp.ListVal{Elems []Value}`, `interp.MapVal` -- insertion-ordered so printing is deterministic
- `interp.ObjectVal{Class *types.Class, Fields map[string]Value}`
- `interp.Uninit struct{}` -- the value of an unassigned object-typed field; any use throws `NullError`
- `interp.Stringify(v Value) string`, `interp.ZeroValue(t types.Type) Value`

`Stringify` rules: `Int` -> decimal; `Float` -> `strconv.FormatFloat(f,'g',-1,64)`, with `.0` appended when the result contains no `.`, `e`, `Inf` or `NaN`, so `20.0` prints as `20.0` and `48.27431` prints as `48.27431`; `Bool` -> `true`/`false`; `Str` -> itself, unquoted; `Char` -> the character; `ListVal` -> `[a, b]`; `MapVal` -> `[k: v, k: v]`; `ObjectVal` -> `ClassName@n` unless it is an `Exception`, which prints its `message`.

- [x] **Step 1: Write the failing tests:** `Stringify` over every case above, especially `Float(20)` -> `20.0` and `Float(48.27431)` -> `48.27431`; `ZeroValue` per type; `MapVal` preserves insertion order across re-assignment of an existing key.
- [x] **Step 2:** `go test ./internal/interp/ -v`. Expected: FAIL.
- [x] **Step 3:** Implement, plus the built-in bootstrap: `print`, `len`, and the `Exception`/`ArithmeticError`/`IndexError`/`NullError` class objects shared with the checker so both stages agree on one identity per class.
- [x] **Step 4:** `go test ./internal/interp/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 9: Evaluator

**Files:**
- Create: `internal/interp/interp.go`; Modify: `internal/interp/interp_test.go`

**Interfaces -- Consumes:** `ast`, `types`, `checker.Info`. **Produces:**
- `interp.New(prog *ast.Program, info *checker.Info, out io.Writer) *Interp`
- `func (i *Interp) Run() error` -- calls `main()`, returns a non-nil error if a Flint exception escapes it

`out io.Writer` rather than `os.Stdout` is what makes the golden tests in Task 10 possible.

Control flow: `evalExpr` and `execStmt` panic with package-private `returnSignal{Value}` or `throwSignal{*ObjectVal}`. `callFunction` recovers `returnSignal`; `execTry` recovers `throwSignal` and matches each `CatchClause` type with `types.IsSubclass`, re-panicking if none match; `Run` recovers at the top. Any other panic is re-panicked unchanged so real bugs still surface.

- [x] **Step 1: Write the failing tests.** Each runs a small program and asserts the captured output:
  - arithmetic and precedence; `Int`/`Float` mixing produces `Float`
  - string concatenation with a `Float` operand
  - `if`/`else if`/`else`, `while`, `for (x: Int in [1,2,3])`
  - field read/write through `object`; a method calling another method on `object`
  - **dynamic dispatch:** a `[Shape]` holding a `Rectangle` and a `Circle`, calling `area()` on each, gets each class's own implementation -- the single most important behavior in the language
  - inherited method: `Rectangle` calls `BaseShape.describe()`
  - **Spec Gap 2:** `class E : Exception { }` constructed as `E("boom")` inherits `Exception`'s ctor and `e.message` reads `boom`
  - `try`/`catch` catches an exact type; catches a **subclass**; an unmatched type propagates past an inner `try` to an outer one
  - `throw` from inside a nested call unwinds through both frames
  - **Review Focus 4:** `5 / 0` and `5 % 0` throw a catchable `ArithmeticError`; an out-of-range and a negative list index throw a catchable `IndexError`; each test catches it and asserts the message, proving no Go panic escapes
  - **Spec Gap 3:** reading an unassigned object-typed field throws `NullError`; an unassigned `Int` field reads as `0`
  - `len` on a list, a map, and a string
  - an uncaught exception makes `Run` return an error naming the class and message
- [x] **Step 2:** `go test ./internal/interp/ -v`. Expected: FAIL.
- [x] **Step 3:** Implement.
- [x] **Step 4:** `go test ./internal/interp/ -race -v`. Expected: PASS.
- [x] **Step 5:** Commit.

## Task 10: CLI, sample program, and end-to-end golden tests

**Files:**
- Create: `cmd/flint/main.go`, `examples/shapes.flint`, `testdata/*.flint` plus `testdata/*.out`, `testdata/e2e_test.go`, `README.md`

**Interfaces -- Consumes:** every package above.

`flint run <file>` lexes, parses, checks, and executes, printing all diagnostics to stderr and exiting 1 if any stage reports an error. `flint check <file>` stops after checking. Errors print as `file:line:col: message`.

- [x] **Step 1:** Write `examples/shapes.flint` as the spec's sample program, **verbatim**, and `testdata/shapes.flint` plus `testdata/shapes.out` containing `48.27431`.
- [x] **Step 2:** Write `e2e_test.go`: glob `testdata/*.flint`, run each through the full pipeline with a `bytes.Buffer` for output, compare against the matching `.out`. A `.err` file instead of `.out` means the program is expected to fail, and the file holds the expected diagnostic substring.
- [x] **Step 3:** `go test ./testdata/ -v`. Expected: FAIL.
- [x] **Step 4:** Implement `main.go`; add golden pairs for each Spec Gap and Review Focus item so they are pinned end to end, not just at unit level.
- [x] **Step 5:** `go build ./... && go vet ./... && go test ./... -race`. Expected: all PASS.
- [x] **Step 6:** Write `README.md` -- what Flint is, `flint run examples/shapes.flint`, the four Spec Gaps and their resolutions, the four Extensions, and what is deliberately out of scope.
- [x] **Step 7:** Commit.

---

## Execution notes (2026-09-20)

Executed natively in one session. All tasks complete; `gofmt`, `go vet`,
`go build`, and `go test ./...` are clean. Three deviations from the plan as
written:

1. **e2e tests live in `internal/e2e/`, not `testdata/`.** The Go tool ignores
   any directory named `testdata`, so a test file there would never run. The
   fixtures are at `internal/e2e/testdata/`.
2. **`internal/pipeline/` was added.** The CLI and the e2e harness both need the
   four stages wired together; duplicating that in two places invited them to
   drift apart.
3. **`-race` is not used.** It requires cgo and a C compiler, neither installed,
   and the interpreter is single-threaded so it would report nothing.

Per-task commits were skipped: this directory is not a git repository and no
commit was requested.
