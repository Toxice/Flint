# Flint

<img src="assets/flint-icon.png" alt="Flint logo" width="150" align="right">

A working interpreter for **Flint**, the language described in
[`flint-language-spec.md`](flint-language-spec.md): Java's structural
discipline — classes, interfaces, mandatory type annotations — without Java's
ceremony.

The spec calls itself "a design exercise: a full specification and worked
pseudo-code, not a working compiler or interpreter." This is that interpreter,
built anyway.

## The language at a glance

```
$ Anything that has a measurable area.
interface Shape {
    func area(): Float
}

$ Shared behaviour for every named shape.
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

func main(): Void {
    let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0)]
    for (shape: Shape in shapes) {
        print(shape.describe())
    }
}
```

Every declaration states its type — there is no inference. `object` replaces
`this`/`self`. One `:` covers both inheritance and interface conformance.
Collections are `[T]` and `[K: V]`, never `<>`. Members are public unless marked
`private`, exceptions are unchecked, and `if`/`while` drop the parentheses.

## Running it

```
go build -o flint ./cmd/flint

./flint run examples/shapes.flint     # compile and execute
./flint check examples/shapes.flint   # compile only, report diagnostics
./flint doc examples/shapes.flint     # print declarations with their docstrings
./flint examples/shapes.flint         # same as run
```

The spec's sample program runs as written:

```
$ ./flint run examples/shapes.flint
48.27431
```

Diagnostics are `file:line:col: message` on stderr, and a failed compile or an
uncaught exception exits 1.

## Architecture

Four stages, one package each, no back-edges:

| Stage | Package | Does |
|---|---|---|
| Lexer | `internal/lexer` | source text to tokens |
| Parser | `internal/parser` | tokens to AST — recursive descent, precedence climbing for expressions |
| Checker | `internal/checker` | two passes: collect declarations, then check bodies and conformance |
| Evaluator | `internal/interp` | tree-walks the AST |

`internal/types` holds the type model both the checker and the evaluator read,
which is what keeps the evaluator independent of the checker.
`internal/pipeline` wires the four together so the CLI and the tests drive
exactly the same path.

Every stage collects *all* its errors rather than stopping at the first, and the
parser resynchronises at statement and declaration boundaries so one mistake
does not cascade.

`return` and `throw` unwind with `panic`/`recover` on unexported signal types,
caught at the function-call, `try`, and top-level boundaries. That is the usual
Go idiom for an evaluator; the signals never leave the package, and any panic
that is not one of them is re-panicked so real bugs still surface.

## What the spec left open

Four things the spec does not settle. Each is resolved here in the way that
keeps the spec's own sample program working. All four are decided and
implemented — this section is a record of the choices, not open work.

**1. The spec's sample drops `name`, and there is no `super`.**
The spec's `Rectangle` and `Circle` constructors accept a `name` parameter and
then never store it, so `describe()` produced `" has area 20.0"` with a blank
name. The spec defines no `super(...)` call anywhere — no keyword, no syntax,
not a single mention — so this is a typo in the spec, not a missing language
feature.

Resolution: **no `super` keyword was invented.** Fields are public and
inherited, so the constructors in `examples/shapes.flint` now assign
`object.name = name` directly, and `describe()` correctly yields
`"Rect1 has area 20.0"`. The program's printed output is unchanged either way,
because `main` only prints the total area.

**2. A class with no constructor inherits its parent's.**
The sample constructs `NegativeAreaError("Area cannot be negative")` though that
class declares no constructor. Constructor lookup therefore walks up the
inheritance chain.

**3. Field defaults.** `Int` → `0`, `Float` → `0.0`, `Bool` → `false`,
`String` → `""`, `Char` → NUL, `[T]` → empty list, `[K:V]` → empty map. A
class- or interface-typed field starts *uninitialised*, and using one throws a
catchable `NullError` rather than crashing the host or silently reading a zero.

**4. Comments and documentation.** `//` to end of line for an ordinary
comment, which the lexer discards. `$` to end of line for a **docstring**,
which is kept and attached to the declaration that follows. Consecutive `$`
lines merge into one docstring, so there is no closing marker to forget:

```
$ Sums the areas of every shape in the list.
$ Throws NegativeAreaError if any shape reports a negative area.
func totalArea(shapes: [Shape]): Float { ... }
```

Docstrings attach to interfaces, classes, fields, constructors, methods, and
top-level functions. `flint doc <file>` prints every declaration with its
documentation and marks the rest `(undocumented)`. A `$` line somewhere it
documents nothing — inside a function body, or trailing at the end of a file
— is skipped rather than being an error. Block comments were not added.

## Additions beyond the spec

Kept to the minimum that makes the language runnable.

- **`print(x)`** — the spec uses it throughout but never declares it. Takes one
  argument of any type, returns `Void`.
- **`len(x): Int`** — for `[T]`, `[K:V]`, and `String`.
- **`Exception`** — a built-in root class with one field `message: String` and
  constructor `Exception(message: String)`, because the spec's `e.message`
  needs it to exist.
- **`ArithmeticError`, `IndexError`, `NullError`** — built-in subclasses of
  `Exception`, so integer division by zero, a bad index, and use of an
  uninitialised reference are catchable Flint values instead of host crashes.

## Decisions worth knowing

- **`Int` widens to `Float`** implicitly, never the reverse. The spec's own
  sample compares a `Float` against the literal `0`, so this is forced.
- **`+` with a `String` on either side is concatenation** and stringifies the
  other operand. Forced by `object.name + " has area " + object.area()`, which
  concatenates a `Float`.
- **Lists and maps are invariant.** `[Rectangle]` is not a `[Shape]` — writing a
  `Circle` through the wider view would break the narrower one. Collection
  *literals* are instead typed against their expected type, which is why
  `let shapes: [Shape] = [Rectangle(...), Circle(...)]` is fine and why `[]`
  and `[:]` work.
- **Floats always print with a decimal point.** Go's shortest representation of
  `20.0` is `20`, indistinguishable from an `Int`, so `.0` is restored.
  Arithmetic is plain IEEE 754: `3.14159 * 3.0 * 3.0` is
  `28.274309999999996`, and `1.0 / 0.0` is `+Inf`. Only *integer* division and
  modulo by zero throw, as in Java.
- **Maps keep insertion order** when printed, which Go's own map iteration does
  not.
- **`for`-in keeps its parentheses**, `if` and `while` drop them — that is what
  the spec's sample actually shows.
- **`main` is required** and must be `func main(): Void`.

## Deliberately out of scope

Excluded because the spec excludes them: user-defined generics (only the
built-in `[T]` and `[K:V]` exist), concurrency, pattern matching, modules and
imports, checked exceptions and `throws` clauses, and a `super` keyword. String
indexing is also absent — `len` is there, but the spec never asks for a way to
get a `Char` out of a `String`.

## Tests

```
go test ./...
```

- Unit tests per stage: lexer, parser, type model, checker, evaluator.
- `internal/e2e` runs whole programs from `internal/e2e/testdata` and compares
  against golden files — `NAME.out` for the exact expected output, `NAME.err`
  for programs that must fail, holding a substring of the expected diagnostic.
  Each spec gap and each runtime-error class is pinned there end to end, not
  just at unit level.

`go test -race` is not used here: the race detector needs cgo and a C compiler,
and the interpreter is single-threaded, so it would report nothing.

## Layout

```
cmd/flint/            CLI
internal/token/       token kinds and positions
internal/lexer/       scanner
internal/ast/         syntax tree
internal/parser/      recursive-descent parser
internal/types/       type model, assignability, lookup
internal/checker/     two-pass static analysis
internal/interp/      values, built-ins, evaluator
internal/doc/         renders $ docstrings for `flint doc`
internal/pipeline/    the four stages wired together
internal/e2e/         whole-program golden tests
examples/             the spec's sample program, verbatim
docs/superpowers/plans/   the implementation plan this was built from
```

## Contributing

Forks and pull requests are welcome. Requirements for a change to be merged:

- `gofmt -l .` produces no output, and `go vet ./...` is clean.
- `go test ./...` passes.
- A language change comes with tests at the stage it touches, plus a golden
  program in `internal/e2e/testdata/` — `NAME.flint` paired with `NAME.out` for
  expected output, or `NAME.err` holding a substring of the expected diagnostic.
- Anything that departs from `flint-language-spec.md` is written down in the
  README, under "What the spec left open" or "Additions beyond the spec". The
  spec is the reference; silent divergence from it is the one thing to avoid.

No external dependencies. The interpreter is standard library only, and the
intent is to keep it that way.

## License

MIT — see [LICENSE](LICENSE). Fork it, build on it, ship it.

The language specification in `flint-language-spec.md` is a design document;
this repository is an independent implementation of it.
