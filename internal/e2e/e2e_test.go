// Package e2e drives whole Flint programs through the real pipeline and
// compares their output against golden files.
//
// Each testdata/NAME.flint is paired with either:
//
//	NAME.out  the exact text the program must print, or
//	NAME.err  a substring that must appear in the diagnostics or in the
//	          uncaught-exception message, for programs expected to fail.
//
// The unit tests pin each stage; these pin the stages working together.
package e2e

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flint/internal/pipeline"
)

func TestPrograms(t *testing.T) {
	sources, err := filepath.Glob(filepath.Join("testdata", "*.flint"))
	if err != nil {
		t.Fatalf("globbing testdata: %v", err)
	}
	if len(sources) == 0 {
		t.Fatal("no testdata programs found")
	}

	for _, src := range sources {
		t.Run(strings.TrimSuffix(filepath.Base(src), ".flint"), func(t *testing.T) {
			code, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("reading %s: %v", src, err)
			}

			var out bytes.Buffer
			diags, runErr := pipeline.Run(string(code), src, &out)

			base := strings.TrimSuffix(src, ".flint")
			if want, err := os.ReadFile(base + ".err"); err == nil {
				checkExpectedFailure(t, string(want), diags, runErr, out.String())
				return
			}

			want, err := os.ReadFile(base + ".out")
			if err != nil {
				t.Fatalf("%s has neither a .out nor a .err golden file", src)
			}
			if len(diags) != 0 {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if runErr != nil {
				t.Fatalf("unexpected uncaught exception: %v\noutput so far:\n%s", runErr, out.String())
			}
			if got, wantStr := normalize(out.String()), normalize(string(want)); got != wantStr {
				t.Fatalf("output mismatch\n--- got ---\n%s\n--- want ---\n%s", got, wantStr)
			}
		})
	}
}

// checkExpectedFailure requires that the program failed, and that the expected
// substring shows up in a diagnostic or in the uncaught exception.
func checkExpectedFailure(t *testing.T, want string, diags []error, runErr error, out string) {
	t.Helper()
	want = strings.TrimSpace(want)

	var all []string
	for _, d := range diags {
		all = append(all, d.Error())
	}
	if runErr != nil {
		all = append(all, runErr.Error())
	}
	if len(all) == 0 {
		t.Fatalf("program succeeded, want a failure mentioning %q\noutput:\n%s", want, out)
	}
	for _, got := range all {
		if strings.Contains(got, want) {
			return
		}
	}
	t.Fatalf("failures %q, want one mentioning %q", all, want)
}

// normalize makes comparison insensitive to trailing whitespace and to the
// line endings git may have rewritten on Windows.
func normalize(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}
