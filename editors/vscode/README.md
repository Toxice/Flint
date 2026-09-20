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

`tests/edges.flint` deliberately ends without a trailing newline — it pins
the `$` docstring rule at end of file. Do not "fix" it.
