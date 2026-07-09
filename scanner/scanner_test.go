package scanner

import (
	"testing"

	"github.com/Piechutowski/not-an-orm/token"
)

// kinds scans src and returns the token kinds without the trailing EOF.
func kinds(t *testing.T, src string) []token.Kind {
	t.Helper()
	toks, errs := Scan("test.dbml", src)
	if len(errs) > 0 {
		t.Fatalf("Scan(%q) errors: %v", src, errs)
	}
	out := make([]token.Kind, 0, len(toks)-1)
	for _, tk := range toks[:len(toks)-1] {
		out = append(out, tk.Kind)
	}
	return out
}

func one(t *testing.T, src string) token.Token {
	t.Helper()
	toks, errs := Scan("test.dbml", src)
	if len(errs) > 0 {
		t.Fatalf("Scan(%q) errors: %v", src, errs)
	}
	if len(toks) != 2 {
		t.Fatalf("Scan(%q) = %d tokens, want 1 + EOF", src, len(toks)-1)
	}
	return toks[0]
}

func TestMaximalMunch(t *testing.T) {
	// §3.1: longest match wins.
	tests := []struct {
		src  string
		want []token.Kind
	}{
		{"<>", []token.Kind{token.LTGT}},
		{"< >", []token.Kind{token.LT, token.GT}},
		{">1", []token.Kind{token.GT, token.NUMBER}},
		{"user_id2", []token.Kind{token.IDENT}},
		{"''''''", []token.Kind{token.STRING}}, // empty multi-line string, not three empties
	}
	for _, tt := range tests {
		got := kinds(t, tt.src)
		if len(got) != len(tt.want) {
			t.Errorf("kinds(%q) = %v, want %v", tt.src, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("kinds(%q)[%d] = %v, want %v", tt.src, i, got[i], tt.want[i])
			}
		}
	}
}

func TestNumbersAndIdents(t *testing.T) {
	// §3.4 constraint 2 and §3.9.
	tests := []struct {
		src  string
		kind token.Kind
	}{
		{"42", token.NUMBER},
		{"3.14", token.NUMBER},
		{"1e2", token.NUMBER},
		{"1.5e10", token.NUMBER},
		{"3.14e-5", token.NUMBER},
		{"2E+3", token.NUMBER},
		{"2fa_codes", token.IDENT},
		{"1e2abc", token.IDENT},
		{"żółć", token.IDENT},
		{"_x", token.IDENT},
	}
	for _, tt := range tests {
		if got := one(t, tt.src); got.Kind != tt.kind {
			t.Errorf("Scan(%q) kind = %v, want %v", tt.src, got.Kind, tt.kind)
		}
	}
	// 12.3abc is a lexical error (§3.9.3)
	if _, errs := Scan("t", "12.3abc"); len(errs) == 0 {
		t.Error("Scan(12.3abc): want error, got none")
	}
}

func TestEscapes(t *testing.T) {
	// §3.8.
	tests := []struct{ src, val string }{
		{`'a\tb'`, "a\tb"},
		{`'a\nb'`, "a\nb"},
		{`'it\'s'`, "it's"},
		{`'back\\slash'`, `back\slash`},
		{`'A'`, "A"},
		{`'\q'`, "q"},      // unknown escape yields the char
		{"'a\\\nb'", "ab"}, // line continuation
	}
	for _, tt := range tests {
		if got := one(t, tt.src); got.Val != tt.val {
			t.Errorf("Scan(%s) val = %q, want %q", tt.src, got.Val, tt.val)
		}
	}
	if _, errs := Scan("t", `'\u12'`); len(errs) == 0 {
		t.Error(`Scan('\u12'): want error for short unicode escape`)
	}
}

func TestMultilineIndentStripping(t *testing.T) {
	// §3.7.4.
	src := "'''\n  line one\n  line two\n'''"
	got := one(t, src)
	want := "line one\nline two"
	if got.Val != want {
		t.Errorf("multiline val = %q, want %q", got.Val, want)
	}
	if !got.Multi {
		t.Error("Multi flag not set")
	}
}

func TestFuncExprIsRaw(t *testing.T) {
	// §3.12: backslash is not an escape inside backticks.
	got := one(t, "`a\\nb`")
	if got.Val != `a\nb` {
		t.Errorf("funcexpr val = %q, want %q", got.Val, `a\nb`)
	}
}

func TestNewlineFlag(t *testing.T) {
	// §3.2: the NLBefore flag drives statement termination.
	toks, _ := Scan("t", "a b\nc")
	if toks[1].NLBefore {
		t.Error("b should not have NLBefore")
	}
	if !toks[2].NLBefore {
		t.Error("c should have NLBefore")
	}
	// a line comment does not swallow the newline's significance (§3.3.4)
	toks, _ = Scan("t", "a // comment\nc")
	if !toks[1].NLBefore {
		t.Error("token after line comment + newline should have NLBefore")
	}
}

func TestCRLF(t *testing.T) {
	// §3.2.1: CR is discarded.
	toks, errs := Scan("t", "a\r\nb")
	if len(errs) > 0 {
		t.Fatalf("errors: %v", errs)
	}
	if !toks[1].NLBefore {
		t.Error("CRLF should still mark NLBefore")
	}
}

func TestLexicalErrors(t *testing.T) {
	for _, src := range []string{
		"'unterminated",
		"'line\nbreak'",
		"/* unterminated",
		"`unterminated",
		`"unterminated`,
		"@",
	} {
		if _, errs := Scan("t", src); len(errs) == 0 {
			t.Errorf("Scan(%q): want error, got none", src)
		}
	}
}

func TestQuotedIdent(t *testing.T) {
	got := one(t, `"double precision"`)
	if got.Kind != token.IDENT || !got.Quoted || got.Val != "double precision" {
		t.Errorf("quoted ident = %+v", got)
	}
}
