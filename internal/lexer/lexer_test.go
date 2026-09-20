package lexer

import (
	"strings"
	"testing"

	"flint/internal/token"
)

func kinds(t *testing.T, src string) []token.Kind {
	t.Helper()
	toks, errs := Scan(src, "test.flint")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ks := make([]token.Kind, 0, len(toks))
	for _, tk := range toks {
		ks = append(ks, tk.Kind)
	}
	return ks
}

func TestKeywordsAndPunctuation(t *testing.T) {
	tests := []struct {
		src  string
		want token.Kind
	}{
		{"let", token.LET}, {"func", token.FUNC}, {"class", token.CLASS},
		{"interface", token.INTERFACE}, {"abstract", token.ABSTRACT},
		{"private", token.PRIVATE}, {"public", token.PUBLIC},
		{"return", token.RETURN}, {"if", token.IF}, {"else", token.ELSE},
		{"while", token.WHILE}, {"for", token.FOR}, {"in", token.IN},
		{"try", token.TRY}, {"catch", token.CATCH}, {"throw", token.THROW},
		{"object", token.OBJECT}, {"true", token.TRUE}, {"false", token.FALSE},
		{"shapes", token.IDENT}, {"Shape", token.IDENT}, {"_x1", token.IDENT},
		{"(", token.LPAREN}, {")", token.RPAREN}, {"{", token.LBRACE},
		{"}", token.RBRACE}, {"[", token.LBRACK}, {"]", token.RBRACK},
		{",", token.COMMA}, {":", token.COLON}, {".", token.DOT},
		{"=", token.ASSIGN}, {"+", token.PLUS}, {"-", token.MINUS},
		{"*", token.STAR}, {"/", token.SLASH}, {"%", token.PERCENT},
		{"==", token.EQ}, {"!=", token.NE}, {"<", token.LT}, {">", token.GT},
		{"<=", token.LE}, {">=", token.GE}, {"&&", token.ANDAND},
		{"||", token.OROR}, {"!", token.BANG},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			got := kinds(t, tt.src)
			if len(got) != 2 || got[0] != tt.want || got[1] != token.EOF {
				t.Fatalf("Scan(%q) = %v, want [%v EOF]", tt.src, got, tt.want)
			}
		})
	}
}

func TestMaximalMunch(t *testing.T) {
	// "<=" must lex as one LE, not LT then ASSIGN.
	got := kinds(t, "<= >= == != && || = ! < >")
	want := []token.Kind{
		token.LE, token.GE, token.EQ, token.NE, token.ANDAND, token.OROR,
		token.ASSIGN, token.BANG, token.LT, token.GT, token.EOF,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("index %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestNumbers(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		want    []token.Kind
		wantLit []string
		wantErr bool
	}{
		{name: "int", src: "21", want: []token.Kind{token.INT, token.EOF}, wantLit: []string{"21"}},
		{name: "float", src: "4.0", want: []token.Kind{token.FLOAT, token.EOF}, wantLit: []string{"4.0"}},
		{name: "pi", src: "3.14159", want: []token.Kind{token.FLOAT, token.EOF}, wantLit: []string{"3.14159"}},
		{name: "zero float", src: "0.0", want: []token.Kind{token.FLOAT, token.EOF}, wantLit: []string{"0.0"}},
		{name: "trailing dot is an error", src: "4.", wantErr: true},
		// No leading-dot floats: ".5" is DOT then INT, which the parser rejects.
		{name: "leading dot", src: ".5", want: []token.Kind{token.DOT, token.INT, token.EOF}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks, errs := Scan(tt.src, "test.flint")
			if tt.wantErr {
				if len(errs) == 0 {
					t.Fatalf("Scan(%q) succeeded, want an error", tt.src)
				}
				return
			}
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			for i, w := range tt.want {
				if toks[i].Kind != w {
					t.Fatalf("index %d: got %v, want %v", i, toks[i].Kind, w)
				}
			}
			for i, w := range tt.wantLit {
				if toks[i].Lit != w {
					t.Fatalf("index %d lit: got %q, want %q", i, toks[i].Lit, w)
				}
			}
		})
	}
}

func TestStringLiteral(t *testing.T) {
	toks, errs := Scan(`"active"`, "test.flint")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if toks[0].Kind != token.STRING || toks[0].Lit != "active" {
		t.Fatalf("got %v %q, want STRING active", toks[0].Kind, toks[0].Lit)
	}
}

func TestStringEscapes(t *testing.T) {
	src := `"a\nb\tc\"d\\e"`
	toks, errs := Scan(src, "test.flint")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := "a\nb\tc\"d\\e"
	if toks[0].Lit != want {
		t.Fatalf("got %q, want %q", toks[0].Lit, want)
	}
}

func TestCharLiteral(t *testing.T) {
	tests := []struct {
		src  string
		want rune
	}{
		{`'a'`, 'a'},
		{`'\n'`, '\n'},
		{`'\''`, '\''},
		{`'\\'`, '\\'},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks, errs := Scan(tt.src, "test.flint")
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %v", errs)
			}
			if toks[0].Kind != token.CHAR {
				t.Fatalf("got %v, want CHAR", toks[0].Kind)
			}
			if []rune(toks[0].Lit)[0] != tt.want {
				t.Fatalf("got %q, want %q", toks[0].Lit, string(tt.want))
			}
		})
	}
}

func TestComments(t *testing.T) {
	got := kinds(t, "let // this is ignored\nx")
	want := []token.Kind{token.LET, token.IDENT, token.EOF}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	// A comment on the final line with no trailing newline must terminate cleanly.
	got = kinds(t, "let x // trailing")
	if got[len(got)-1] != token.EOF {
		t.Fatalf("comment at EOF did not terminate: %v", got)
	}
}

// Review Focus 1: malformed input must produce a positioned error, never a hang.
func TestUnterminatedLiterals(t *testing.T) {
	for _, src := range []string{`"abc`, `"abc\`, `'a`, `''`} {
		t.Run(src, func(t *testing.T) {
			done := make(chan struct{})
			var errs []error
			go func() {
				_, errs = Scan(src, "test.flint")
				close(done)
			}()
			<-done
			if len(errs) == 0 {
				t.Fatalf("Scan(%q) reported no error", src)
			}
			if !strings.Contains(errs[0].Error(), "test.flint:1:") {
				t.Fatalf("error lacks a position: %v", errs[0])
			}
		})
	}
}

func TestIllegalCharacter(t *testing.T) {
	toks, errs := Scan("let @ x", "test.flint")
	if len(errs) != 1 {
		t.Fatalf("got %d errors, want 1: %v", len(errs), errs)
	}
	var sawIllegal bool
	for _, tk := range toks {
		if tk.Kind == token.ILLEGAL {
			sawIllegal = true
			if tk.Pos.Line != 1 || tk.Pos.Col != 5 {
				t.Fatalf("ILLEGAL at %v, want 1:5", tk.Pos)
			}
		}
	}
	if !sawIllegal {
		t.Fatal("no ILLEGAL token emitted")
	}
	// Scanning continues past the bad character.
	if toks[len(toks)-2].Kind != token.IDENT {
		t.Fatalf("scanning did not continue past the illegal char: %v", toks)
	}
}

func TestPositions(t *testing.T) {
	toks, errs := Scan("let\n  x:\nInt", "test.flint")
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	want := []token.Pos{
		{Line: 1, Col: 1}, {Line: 2, Col: 3}, {Line: 2, Col: 4}, {Line: 3, Col: 1},
	}
	for i, w := range want {
		if toks[i].Pos != w {
			t.Fatalf("token %d (%v) at %v, want %v", i, toks[i].Kind, toks[i].Pos, w)
		}
	}
}
