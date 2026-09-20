// Package pipeline wires the four compiler stages together.
//
// It exists so the CLI and the end-to-end tests drive Flint through exactly the
// same path, rather than each assembling the stages their own way.
package pipeline

import (
	"io"

	"flint/internal/ast"
	"flint/internal/checker"
	"flint/internal/interp"
	"flint/internal/lexer"
	"flint/internal/parser"
)

// Compile lexes, parses, and checks src.
//
// Each stage runs only if the one before it produced no errors, so the caller
// never sees parse errors that are really just fallout from a bad token.
func Compile(src, filename string) (*ast.Program, *checker.Info, []error) {
	toks, errs := lexer.Scan(src, filename)
	if len(errs) != 0 {
		return nil, nil, errs
	}
	prog, errs := parser.Parse(toks, filename)
	if len(errs) != 0 {
		return nil, nil, errs
	}
	info, errs := checker.Check(prog, filename)
	if len(errs) != 0 {
		return prog, info, errs
	}
	return prog, info, nil
}

// Run compiles src and executes main, writing print output to out.
//
// The returned slice holds compile-time diagnostics; the error is a Flint
// exception that escaped main. Both being empty means the program ran to
// completion.
func Run(src, filename string, out io.Writer) ([]error, error) {
	prog, info, errs := Compile(src, filename)
	if len(errs) != 0 {
		return errs, nil
	}
	return nil, interp.New(prog, info, out).Run()
}
