// Package doc renders the $ docstrings in a Flint program as readable
// documentation.
//
// It works from the AST alone, so `flint doc` still produces output for a file
// that does not type-check.
package doc

import (
	"fmt"
	"io"
	"strings"

	"flint/internal/ast"
)

// Render writes every documented declaration in prog to w, in source order.
func Render(prog *ast.Program, w io.Writer) {
	for n, d := range prog.Decls {
		if n > 0 {
			fmt.Fprintln(w)
		}
		switch d := d.(type) {
		case *ast.InterfaceDecl:
			renderInterface(d, w)
		case *ast.ClassDecl:
			renderClass(d, w)
		case *ast.FuncDecl:
			fmt.Fprintln(w, signature(d))
			writeDoc(w, d.Doc, "    ")
		}
	}
}

func renderInterface(d *ast.InterfaceDecl, w io.Writer) {
	fmt.Fprintf(w, "interface %s\n", d.Name)
	writeDoc(w, d.Doc, "    ")
	for _, m := range d.Methods {
		fmt.Fprintf(w, "    %s\n", signature(m))
		writeDoc(w, m.Doc, "        ")
	}
}

func renderClass(d *ast.ClassDecl, w io.Writer) {
	head := "class " + d.Name
	if d.Abstract {
		head = "abstract " + head
	}
	if len(d.Bases) > 0 {
		head += " : " + strings.Join(d.Bases, ", ")
	}
	fmt.Fprintln(w, head)
	writeDoc(w, d.Doc, "    ")

	for _, f := range d.Fields {
		vis := ""
		if f.Private {
			vis = "private "
		}
		fmt.Fprintf(w, "    %s%s: %s\n", vis, f.Name, ast.TypeString(f.Type))
		writeDoc(w, f.Doc, "        ")
	}
	if d.Ctor != nil {
		fmt.Fprintf(w, "    %s(%s)\n", d.Ctor.Name, params(d.Ctor))
		writeDoc(w, d.Ctor.Doc, "        ")
	}
	for _, m := range d.Methods {
		fmt.Fprintf(w, "    %s\n", signature(m))
		writeDoc(w, m.Doc, "        ")
	}
}

// signature renders a function or method header as it appears in source.
func signature(d *ast.FuncDecl) string {
	var b strings.Builder
	if d.Private {
		b.WriteString("private ")
	}
	if d.Abstract {
		b.WriteString("abstract ")
	}
	fmt.Fprintf(&b, "func %s(%s): %s", d.Name, params(d), ast.TypeString(d.Result))
	return b.String()
}

func params(d *ast.FuncDecl) string {
	parts := make([]string, 0, len(d.Params))
	for _, p := range d.Params {
		parts = append(parts, p.Name+": "+ast.TypeString(p.Type))
	}
	return strings.Join(parts, ", ")
}

// writeDoc prints a docstring indented, one line per source line, or the
// placeholder when a declaration carries no documentation.
func writeDoc(w io.Writer, doc, indent string) {
	if doc == "" {
		fmt.Fprintf(w, "%s(undocumented)\n", indent)
		return
	}
	for _, line := range strings.Split(doc, "\n") {
		fmt.Fprintf(w, "%s%s\n", indent, line)
	}
}
