package lexer

import (
	"testing"

	"flint/internal/token"
)

func TestDocstringToken(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // the Lit of each DOC token, in order
	}{
		{
			name: "one line, surrounding space trimmed",
			src:  "$   Computes the area.   \nfunc f(): Void { }",
			want: []string{"Computes the area."},
		},
		{
			name: "consecutive lines are separate tokens",
			src:  "$ first\n$ second\nfunc f(): Void { }",
			want: []string{"first", "second"},
		},
		{
			name: "empty docstring line",
			src:  "$\nfunc f(): Void { }",
			want: []string{""},
		},
		{
			name: "docstring at end of file with no newline",
			src:  "$ trailing",
			want: []string{"trailing"},
		},
		{
			name: "a docstring is not a comment and survives",
			src:  "// dropped\n$ kept\nfunc f(): Void { }",
			want: []string{"kept"},
		},
		{
			name: "a // inside a docstring is part of the text",
			src:  "$ see // for comments\nfunc f(): Void { }",
			want: []string{"see // for comments"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, errs := Scan(tt.src, "test.flint")
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			var got []string
			for _, tk := range toks {
				if tk.Kind == token.DOC {
					got = append(got, tk.Lit)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d docstrings %q, want %d %q", len(got), got, len(tt.want), tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("docstring %d = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDocstringPosition(t *testing.T) {
	toks, errs := Scan("func f(): Void { }\n  $ doc", "test.flint")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	for _, tk := range toks {
		if tk.Kind == token.DOC {
			if tk.Pos.Line != 2 || tk.Pos.Col != 3 {
				t.Fatalf("docstring at %v, want 2:3", tk.Pos)
			}
			return
		}
	}
	t.Fatal("no DOC token emitted")
}
