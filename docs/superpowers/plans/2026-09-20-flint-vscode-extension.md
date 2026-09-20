# Flint VS Code Extension Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a declarative VS Code extension that makes VS Code recognize
`.flint` files as a first-class language — name, file icon, syntax
highlighting, bracket matching, and comment toggling.

**Architecture:** A pure *declarative* language extension — no TypeScript, no
activation events, no language server. VS Code language support has three
layers: `contributes.languages` (identity: id, file extensions, icon,
`language-configuration.json`), `contributes.grammars` (a TextMate grammar
mapping regexes to scope names, which the color theme paints), and optionally
a language server for semantics. Layers one and two are plain JSON and cover
everything asked for here. The extension lives in-repo at `editors/vscode/`
so the grammar stays next to `internal/token/token.go`, its source of truth.

**Tech Stack:** JSON only (`package.json`, `language-configuration.json`,
`flint.tmLanguage.json`), Node 25 / npm 11 for tooling,
`vscode-tmgrammar-test` (provides `vscode-tmgrammar-snap`) for grammar
snapshot tests, `@vscode/vsce` for packaging.

**Spec:** [`flint-language-spec.md`](../../../flint-language-spec.md), with
`internal/token/token.go` as the authoritative keyword and operator list.

## Global Constraints

- Language id is exactly `flint`; scope name is exactly `source.flint`.
- File extension is `.flint` only.
- Keywords are exactly the 19 in `token.go`'s `keywords` map: `let func class
  interface abstract private public return if else while for in try catch
  throw object true false`. No others. No `extends`, `implements`, `this`,
  `self`, `new`, `null`, `var`, `def`.
- Primitive types are exactly: `Int Float Bool String Char Void`.
- Builtin functions are exactly: `print len` (see `internal/interp/builtin.go`).
- Comments: `//` to end of line. Docstrings: `$` to end of line — these are
  *tokens* the parser keeps, not comments, but they read as documentation and
  are scoped `comment.line.documentation.flint`.
- There is no block comment syntax. Do not add a `blockComment` pair the lexer
  does not implement.
- `engines.vscode` floor is `^1.75.0` (the `contributes.languages[].icon`
  field needs 1.66+; 1.75 is a safe, widely-installed floor).
- No runtime dependencies. `dependencies` in `package.json` stays empty.
- Extension version starts at `0.1.0`.
- All new files live under `editors/vscode/`. Do not touch `internal/` or `cmd/`.

## Review Focus

Five input classes the grammar will meet that no obvious test covers, most
likely to bite first:

1. **A `$` docstring on the last line with no trailing newline** — the `$`
   pattern must still terminate. Covered by a fixture in Task 2.
2. **An escaped quote inside a string** (`"he said \"hi\""`) — a naive
   `begin`/`end` string rule ends the token at the escaped quote and paints
   the rest of the line as code. Covered by a fixture in Task 2.
3. **A char literal holding a brace or quote** (`'{'`, `'\''`) — TextMate
   brace matching is regex-level and will treat `'{'` as an open brace, so
   `language-configuration.json` bracket pairs can mis-nest. This is a known
   ceiling of declarative grammars, not a fixable bug; the fixture in Task 2
   pins the *scope* (it must be `string.quoted.single.flint`) so at least the
   color is right.
4. **`in` inside an identifier** (`index`, `inner`, `printer`, `classy`) — the keyword
   regex must be `\b`-anchored or it paints half a variable name. Covered by
   a fixture in Task 2.
5. **Colon overload** — `:` means three different things (`x: Int`
   annotation, `class C : Base` inheritance, `[String: Int]` map type). The
   grammar must not assume one reading; all three must still highlight their
   type names. Covered by a fixture in Task 2.

---

### Task 1: Extension skeleton, language identity, and icon

VS Code should recognize `.flint`, name it "Flint" in the status bar, show
the Flint icon on the file, match brackets, and toggle `//` comments with
Ctrl+/. No colors yet — that is Task 2.

**Files:**
- Create: `editors/vscode/package.json`
- Create: `editors/vscode/language-configuration.json`
- Create: `editors/vscode/icons/flint.png` (generated, 128x128)
- Create: `editors/vscode/.vscodeignore`
- Modify: `.gitignore` (append extension build artifacts)

**Interfaces:**
- Consumes: `assets/flint-icon.png` (500x500 RGBA), committed in `b07c847`.
- Produces: language id `flint` and scope name `source.flint` (Task 2 declares
  a grammar against both), and `editors/vscode/` as the extension root that
  Tasks 2 and 3 run npm commands from.

- [ ] **Step 1: Create the extension directory and generate the 128x128 icon**

Run from the repo root:

```bash
mkdir -p editors/vscode/icons editors/vscode/syntaxes editors/vscode/tests
python -c "from PIL import Image; im=Image.open('assets/flint-icon.png').convert('RGBA'); im.resize((128,128), Image.LANCZOS).save('editors/vscode/icons/flint.png')"
file editors/vscode/icons/flint.png
```

Expected: `PNG image data, 128 x 128, 8-bit/color RGBA`. Resize from the
500px **PNG**, not the 2048px JPEG — the PNG has the transparent background
the icon needs on both light and dark file trees.

- [ ] **Step 2: Write `editors/vscode/package.json`**

```json
{
  "name": "flint-lang",
  "displayName": "Flint",
  "description": "Syntax highlighting and language support for the Flint programming language.",
  "version": "0.1.0",
  "publisher": "toxice",
  "license": "MIT",
  "icon": "icons/flint.png",
  "repository": {
    "type": "git",
    "url": "https://github.com/Toxice/Flint.git"
  },
  "engines": {
    "vscode": "^1.75.0"
  },
  "categories": [
    "Programming Languages"
  ],
  "keywords": [
    "flint",
    "flint-lang"
  ],
  "contributes": {
    "languages": [
      {
        "id": "flint",
        "aliases": [
          "Flint",
          "flint"
        ],
        "extensions": [
          ".flint"
        ],
        "configuration": "./language-configuration.json",
        "icon": {
          "light": "./icons/flint.png",
          "dark": "./icons/flint.png"
        }
      }
    ]
  },
  "scripts": {
    "test": "vscode-tmgrammar-snap \"tests/**/*.flint\"",
    "test:update": "vscode-tmgrammar-snap --updateSnapshot \"tests/**/*.flint\"",
    "package": "vsce package"
  },
  "devDependencies": {
    "@vscode/vsce": "^3.2.0",
    "vscode-tmgrammar-test": "^0.1.3"
  }
}
```

Note there is no `main` and no `activationEvents`. A declarative language
extension has no code to run; VS Code reads the contributions from the
manifest at startup. Adding a `main` would be dead weight.

The `test` scripts reference `tests/**/*.flint`, which Task 2 creates. They
fail until then — that is expected.

- [ ] **Step 3: Write `editors/vscode/language-configuration.json`**

```json
{
  "comments": {
    "lineComment": "//"
  },
  "brackets": [
    ["{", "}"],
    ["[", "]"],
    ["(", ")"]
  ],
  "autoClosingPairs": [
    { "open": "{", "close": "}" },
    { "open": "[", "close": "]" },
    { "open": "(", "close": ")" },
    { "open": "\"", "close": "\"", "notIn": ["string", "comment"] },
    { "open": "'", "close": "'", "notIn": ["string", "comment"] }
  ],
  "surroundingPairs": [
    ["{", "}"],
    ["[", "]"],
    ["(", ")"],
    ["\"", "\""],
    ["'", "'"]
  ],
  "wordPattern": "[A-Za-z_][A-Za-z0-9_]*",
  "folding": {
    "markers": {
      "start": "^\\s*//\\s*#?region\\b",
      "end": "^\\s*//\\s*#?endregion\\b"
    }
  }
}
```

No `blockComment` key: the Flint lexer has no block comment, and declaring
one would make Shift+Alt+A emit syntax that fails to parse.

- [ ] **Step 4: Write `editors/vscode/.vscodeignore`**

```
tests/**
node_modules/**
.vscode-test/**
**/*.snap
**/*.vsix
```

- [ ] **Step 5: Append build artifacts to the root `.gitignore`**

Append these lines to the existing `.gitignore`, keeping its current contents:

```
# VS Code extension
editors/vscode/node_modules/
editors/vscode/*.vsix
```

- [ ] **Step 6: Verify VS Code recognizes the language**

```bash
code --extensionDevelopmentPath="$PWD/editors/vscode" examples/hello.flint
```

Expected, in the Extension Development Host window that opens:
1. The status bar bottom-right reads **Flint**, not "Plain Text".
2. The file tab and Explorer entry show the Flint rock icon.
3. Put the cursor on `{` in `func main(): Void {` — its matching `}` is
   outlined.
4. Select the `print(...)` line and press Ctrl+/ — it becomes
   `// print("Hello, world!")`. Press again — the `//` is removed.

There are no colors yet; `hello.flint` is monochrome. That is correct here.

If the status bar still says "Plain Text", the manifest failed to load —
check the Extension Development Host's Help > Toggle Developer Tools console
for a manifest parse error.

- [ ] **Step 7: Verify the extension packages cleanly**

```bash
cd editors/vscode && npm install && npx vsce package --out /tmp/flint-check.vsix
```

Expected: `DONE  Packaged: /tmp/flint-check.vsix`. `vsce` will warn about a
missing `README.md` and `LICENSE` — Task 3 adds those. Any *error* (bad
manifest, missing icon, bad `engines` range) is a real failure.

```bash
rm -f /tmp/flint-check.vsix
```

- [ ] **Step 8: Commit**

```bash
cd "$(git rev-parse --show-toplevel)"
git add editors/vscode/package.json editors/vscode/language-configuration.json editors/vscode/.vscodeignore editors/vscode/icons/flint.png .gitignore
git commit -m "feat(vscode): recognize .flint as a language with icon and bracket config"
```

---

### Task 2: TextMate grammar and snapshot tests

Give `.flint` files real syntax colors, and pin every token's scope with
snapshot tests so a later regex tweak cannot silently repaint the language.

**Files:**
- Create: `editors/vscode/syntaxes/flint.tmLanguage.json`
- Create: `editors/vscode/tests/basics.flint`
- Create: `editors/vscode/tests/edges.flint`
- Modify: `editors/vscode/package.json` (add the `contributes.grammars` block)

**Interfaces:**
- Consumes: language id `flint` from Task 1.
- Produces: scope name `source.flint` and these token scopes, which Task 3's
  README documents and any future theme keys off:
  `keyword.control.flint`, `keyword.declaration.flint`,
  `storage.modifier.flint`, `variable.language.flint`,
  `constant.language.boolean.flint`, `support.function.builtin.flint`,
  `support.type.primitive.flint`, `entity.name.type.flint`,
  `entity.name.function.flint`, `constant.numeric.integer.flint`,
  `constant.numeric.float.flint`, `string.quoted.double.flint`,
  `string.quoted.single.flint`, `constant.character.escape.flint`,
  `comment.line.double-slash.flint`, `comment.line.documentation.flint`,
  `keyword.operator.flint`.

- [ ] **Step 1: Write the grammar**

Create `editors/vscode/syntaxes/flint.tmLanguage.json`:

```json
{
  "$schema": "https://raw.githubusercontent.com/martinring/tmlanguage/master/tmlanguage.json",
  "name": "Flint",
  "scopeName": "source.flint",
  "patterns": [
    { "include": "#comment" },
    { "include": "#docstring" },
    { "include": "#string" },
    { "include": "#char" },
    { "include": "#number" },
    { "include": "#declaration" },
    { "include": "#keyword" },
    { "include": "#type" },
    { "include": "#operator" }
  ],
  "repository": {
    "comment": {
      "name": "comment.line.double-slash.flint",
      "match": "//.*$"
    },
    "docstring": {
      "name": "comment.line.documentation.flint",
      "match": "\\$.*$"
    },
    "string": {
      "name": "string.quoted.double.flint",
      "begin": "\"",
      "end": "\"",
      "patterns": [
        {
          "name": "constant.character.escape.flint",
          "match": "\\\\."
        }
      ]
    },
    "char": {
      "name": "string.quoted.single.flint",
      "match": "'(\\\\.|[^'\\\\])'"
    },
    "number": {
      "patterns": [
        {
          "name": "constant.numeric.float.flint",
          "match": "\\b[0-9]+\\.[0-9]+\\b"
        },
        {
          "name": "constant.numeric.integer.flint",
          "match": "\\b[0-9]+\\b"
        }
      ]
    },
    "declaration": {
      "patterns": [
        {
          "match": "\\b(func)\\s+([A-Za-z_][A-Za-z0-9_]*)",
          "captures": {
            "1": { "name": "keyword.declaration.flint" },
            "2": { "name": "entity.name.function.flint" }
          }
        },
        {
          "match": "\\b(class|interface)\\s+([A-Za-z_][A-Za-z0-9_]*)",
          "captures": {
            "1": { "name": "keyword.declaration.flint" },
            "2": { "name": "entity.name.type.flint" }
          }
        }
      ]
    },
    "keyword": {
      "patterns": [
        {
          "name": "keyword.control.flint",
          "match": "\\b(if|else|while|for|in|return|try|catch|throw)\\b"
        },
        {
          "name": "keyword.declaration.flint",
          "match": "\\b(let|func|class|interface|abstract)\\b"
        },
        {
          "name": "storage.modifier.flint",
          "match": "\\b(private|public)\\b"
        },
        {
          "name": "variable.language.flint",
          "match": "\\bobject\\b"
        },
        {
          "name": "constant.language.boolean.flint",
          "match": "\\b(true|false)\\b"
        },
        {
          "name": "support.function.builtin.flint",
          "match": "\\b(print|len)\\b(?=\\s*\\()"
        }
      ]
    },
    "type": {
      "patterns": [
        {
          "name": "support.type.primitive.flint",
          "match": "\\b(Int|Float|Bool|String|Char|Void)\\b"
        },
        {
          "name": "entity.name.type.flint",
          "match": "\\b[A-Z][A-Za-z0-9_]*\\b"
        }
      ]
    },
    "operator": {
      "name": "keyword.operator.flint",
      "match": "(==|!=|<=|>=|&&|\\|\\||[-+*/%<>=!])"
    }
  }
}
```

Three ordering facts that matter and are easy to break:

- `#declaration` is listed **before** `#keyword` so `func area` paints `area`
  as a function name. Move it after and the bare `func` keyword rule wins,
  consuming `func` and leaving `area` unstyled.
- Inside `#number`, the float rule is **before** the integer rule. Reverse
  them and `4.0` paints as two integers with a stray operator between.
- Inside `#type`, the primitive rule is **before** the general capitalized
  rule, so `Int` gets `support.type.primitive.flint` rather than the generic
  `entity.name.type.flint`.

- [ ] **Step 2: Register the grammar in `package.json`**

Add a `grammars` array inside the existing `contributes` object, as a sibling
of `languages`:

```json
    "grammars": [
      {
        "language": "flint",
        "scopeName": "source.flint",
        "path": "./syntaxes/flint.tmLanguage.json"
      }
    ]
```

- [ ] **Step 3: Write the basics fixture**

Create `editors/vscode/tests/basics.flint`:

```
$ Anything that has a measurable area.
interface Shape {
    func area(): Float
}

abstract class BaseShape : Shape {
    private name: String

    BaseShape(name: String) { object.name = name }

    func describe(): String {
        return object.name + " has area " + object.area()
    }
}

class Rectangle : BaseShape {
    width: Float
    height: Float

    func area(): Float { return object.width * object.height }
}

// A plain line comment.
func main(): Void {
    let shapes: [Shape] = [Rectangle("Rect1", 4.0, 5.0)]
    let counts: [String: Int] = ["a": 1, "b": 2]
    let ok: Bool = true
    let initial: Char = 'R'
    if len(shapes) > 0 && ok != false {
        print(shapes)
    }
}
```

- [ ] **Step 4: Write the edge-case fixture**

Create `editors/vscode/tests/edges.flint`. This file pins every item in the
plan's Review Focus section:

```
func escapes(): Void {
    let quoted: String = "he said \"hi\" and left"
    let backslash: String = "a\\b"
    let brace: Char = '{'
    let tick: Char = '\''
}

func identifiers(): Void {
    let index: Int = 0
    let inner: Int = 1
    let printer: Int = 2
    let classy: Int = 3
    for (item: Int in [index, inner]) {
        print(item)
    }
}

func colons(): Void {
    let annotated: Int = 1
    let mapped: [String: Int] = ["k": 1]
}

class Alone : Exception { }
$ A trailing docstring with no newline after it.
```

**The last line must not end with a newline.** After writing the file, run:

```bash
printf '%s' "$(cat editors/vscode/tests/edges.flint)" > editors/vscode/tests/edges.tmp
mv editors/vscode/tests/edges.tmp editors/vscode/tests/edges.flint
tail -c 1 editors/vscode/tests/edges.flint | xxd | head -1
```

Expected: the last byte is `.` (0x2e), not a newline (0x0a). This is the
whole point of Review Focus item 1 — a `$` rule that relies on a trailing
newline to terminate breaks here.

- [ ] **Step 5: Install tooling and generate the snapshots**

```bash
cd editors/vscode && npm install && npm run test:update
```

Expected: `vscode-tmgrammar-snap` writes `tests/basics.flint.snap` and
`tests/edges.flint.snap`, one line of scopes per source token.

- [ ] **Step 6: Read the snapshots and confirm each Review Focus case**

```bash
cd editors/vscode && grep -n 'string.quoted\|comment.line.documentation\|keyword.control' tests/edges.flint.snap
```

Check by eye, in `tests/edges.flint.snap`:

1. The final `$ A trailing docstring...` line is scoped
   `comment.line.documentation.flint` all the way to the last character.
2. In `"he said \"hi\" and left"`, the `\"` sequences are
   `constant.character.escape.flint` and everything through the final `"` is
   still `string.quoted.double.flint`. The word `and` must **not** appear
   scoped as source.
3. `'{'` is `string.quoted.single.flint` across all three characters.
4. `index`, `inner`, `printer`, `classy` carry **no** `keyword.*` scope on
   any of their characters. The standalone `in` inside `for (...)` does carry
   `keyword.control.flint`.
5. In `[String: Int]`, both `String` and `Int` are
   `support.type.primitive.flint`; in `class Alone : Exception`, `Alone` is
   `entity.name.type.flint` and so is `Exception`.

If any of these is wrong, fix the grammar regex, re-run `npm run test:update`,
and check again. Do not accept a snapshot that contradicts this list.

- [ ] **Step 7: Verify the snapshots pass as a test**

```bash
cd editors/vscode && npm test
```

Expected: exit code 0, every fixture reported as matching its snapshot. This
is the runnable check the grammar leaves behind — from here on, any regex
change that repaints a token fails this command.

- [ ] **Step 8: Verify the colors in a real editor**

```bash
cd "$(git rev-parse --show-toplevel)"
code --extensionDevelopmentPath="$PWD/editors/vscode" examples/shapes.flint
```

Expected in the Extension Development Host: keywords, type names, strings,
numbers, and `$` docstrings each render in a distinct theme color, and
`object` is styled the way `this`/`self` is in other languages.

- [ ] **Step 9: Commit**

```bash
cd "$(git rev-parse --show-toplevel)"
git add editors/vscode/syntaxes editors/vscode/tests editors/vscode/package.json editors/vscode/package-lock.json
git commit -m "feat(vscode): add Flint TextMate grammar with snapshot tests"
```

---

### Task 3: Documentation and installable package

Make the extension something a person can actually install, and tell them how.

**Files:**
- Create: `editors/vscode/README.md`
- Create: `editors/vscode/CHANGELOG.md`
- Create: `editors/vscode/LICENSE` (copy of the root `LICENSE`)
- Modify: `README.md` (add an "Editor support" section)

**Interfaces:**
- Consumes: the working extension from Tasks 1 and 2, and the scope list from
  Task 2's Produces block.
- Produces: `flint-lang-0.1.0.vsix`, an installable artifact.

- [ ] **Step 1: Copy the license into the extension**

```bash
cp LICENSE editors/vscode/LICENSE
```

`vsce` warns on a missing LICENSE and the Marketplace shows one per
extension, so it needs its own copy rather than a reference upward.

- [ ] **Step 2: Write `editors/vscode/README.md`**

````markdown
# Flint for VS Code

Language support for [Flint](https://github.com/Toxice/Flint) — syntax
highlighting, bracket matching, comment toggling, and a file icon for
`.flint` files.

<img src="icons/flint.png" alt="Flint logo" width="96">

## Features

- Syntax highlighting for every Flint keyword, primitive type, literal,
  operator, `//` comment, and `$` docstring
- `.flint` files identified as **Flint**, with the Flint file icon
- Bracket matching and auto-closing for `{}`, `[]`, `()`, `""`, `''`
- Ctrl+/ toggles `//` line comments

This extension is declarative only — there is no language server, so there is
no autocomplete, go-to-definition, or type checking in the editor. Run the
`flint` binary for type errors.

## Install

From a packaged build:

```bash
code --install-extension flint-lang-0.1.0.vsix
```

Or build it from this repository:

```bash
cd editors/vscode
npm install
npm run package
code --install-extension flint-lang-0.1.0.vsix
```

## Development

```bash
npm test              # verify grammar scopes against snapshots
npm run test:update   # re-record snapshots after an intentional change
```

Open the repository root in VS Code and press F5 to launch an Extension
Development Host with the extension loaded.

The grammar's source of truth is `internal/token/token.go`. If a keyword is
added there, add it to `syntaxes/flint.tmLanguage.json` and re-record the
snapshots.
````

- [ ] **Step 3: Write `editors/vscode/CHANGELOG.md`**

```markdown
# Changelog

## 0.1.0

- Initial release: `.flint` language identity, file icon, TextMate grammar,
  bracket matching, and `//` comment toggling.
```

- [ ] **Step 4: Add an "Editor support" section to the root README**

Insert this section into `README.md`, immediately before its license section
(or at the end if there is none):

````markdown
## Editor support

A VS Code extension lives in [`editors/vscode/`](editors/vscode/). It gives
`.flint` files syntax highlighting, the Flint file icon, bracket matching,
and comment toggling.

```bash
cd editors/vscode && npm install && npm run package
code --install-extension flint-lang-0.1.0.vsix
```
````

- [ ] **Step 5: Package and install for real**

```bash
cd editors/vscode && npm run package
```

Expected: `flint-lang-0.1.0.vsix` is created with no errors and no warnings
about a missing README or LICENSE.

```bash
code --install-extension editors/vscode/flint-lang-0.1.0.vsix
```

Then open `examples/shapes.flint` in a **normal** VS Code window, not the
Extension Development Host. Expected: colors, the Flint icon, and "Flint" in
the status bar — proving the packaged artifact works, not just the dev load.

- [ ] **Step 6: Commit**

```bash
cd "$(git rev-parse --show-toplevel)"
git add editors/vscode/README.md editors/vscode/CHANGELOG.md editors/vscode/LICENSE README.md
git commit -m "docs(vscode): document and package the Flint extension"
```

---

## Out of scope

Deliberately excluded, each with the trigger that would justify adding it:

- **Language server / IntelliSense.** `internal/checker` already computes
  types and errors, so an LSP wrapper over it is genuinely possible — but it
  is a Go server, a JSON-RPC protocol, and a TypeScript client, i.e. a larger
  project than this whole extension. Add when editing Flint without inline
  type errors becomes the thing that slows you down.
- **A "Run Flint File" command / task provider.** Needs an activation event
  and a `main`, turning this from a zero-code extension into a real one. Add
  when you are running the `flint` binary by hand often enough to notice.
- **Marketplace publishing.** Needs a `toxice` publisher account and a
  Personal Access Token. Add when someone other than you wants to install it.
- **A full file icon theme.** `contributes.languages[].icon` covers the
  common case; VS Code falls back to it when the active file icon theme has
  no entry for `.flint`. A theme that *does* claim `.flint` would win. Add
  only if a popular icon theme starts overriding it.
