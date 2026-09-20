// Command flint runs and type-checks Flint programs.
//
// Usage:
//
//	flint run <file.flint>     compile and execute
//	flint check <file.flint>   compile only, report diagnostics
//	flint doc <file.flint>     print declarations with their $ docstrings
//	flint <file.flint>         same as run
package main

import (
	"fmt"
	"os"

	"flint/internal/doc"
	"flint/internal/pipeline"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

const usage = `flint - an interpreter for the Flint language

usage:
  flint run <file.flint>     compile and execute the program
  flint check <file.flint>   compile only and report any diagnostics
  flint doc <file.flint>     print declarations with their $ docstrings
  flint <file.flint>         same as run
`

func run(args []string) int {
	var cmd, path string
	switch len(args) {
	case 1:
		cmd, path = "run", args[0]
	case 2:
		cmd, path = args[0], args[1]
	default:
		fmt.Fprint(os.Stderr, usage)
		return 2
	}

	if cmd != "run" && cmd != "check" && cmd != "doc" {
		fmt.Fprintf(os.Stderr, "flint: unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	src, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "flint: %v\n", err)
		return 1
	}

	if cmd == "doc" {
		// Documentation is rendered from the AST, so a file that fails to
		// type-check can still be documented. Only a parse failure blocks it.
		prog, _, diags := pipeline.Compile(string(src), path)
		if prog == nil {
			report(diags)
			return 1
		}
		doc.Render(prog, os.Stdout)
		return 0
	}

	if cmd == "check" {
		_, _, diags := pipeline.Compile(string(src), path)
		if len(diags) != 0 {
			report(diags)
			return 1
		}
		fmt.Fprintf(os.Stderr, "%s: no errors\n", path)
		return 0
	}

	diags, runErr := pipeline.Run(string(src), path, os.Stdout)
	if len(diags) != 0 {
		report(diags)
		return 1
	}
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "flint: %v\n", runErr)
		return 1
	}
	return 0
}

// report prints diagnostics to stderr, one per line, already positioned as
// file:line:col by the stage that produced them.
func report(diags []error) {
	for _, d := range diags {
		fmt.Fprintln(os.Stderr, d)
	}
	noun := "errors"
	if len(diags) == 1 {
		noun = "error"
	}
	fmt.Fprintf(os.Stderr, "%d %s\n", len(diags), noun)
}
