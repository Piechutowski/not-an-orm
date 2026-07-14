// Package token defines the lexical tokens of DBML and source positions,
// mirroring the role of go/token in the Go toolchain.
package token

import "fmt"

// Position is a source location. Unlike go/token, DBML compiles one file at
// a time, so a plain struct replaces the FileSet/Pos machinery.
type Position struct {
	Filename string
	Offset   int // byte offset, 0-based
	Line     int // 1-based
	Column   int // 1-based, in runes
}

func (p Position) String() string {
	name := p.Filename
	if name == "" {
		name = "<input>"
	}
	return fmt.Sprintf("%s:%d:%d", name, p.Line, p.Column)
}

func (p Position) IsValid() bool { return p.Line > 0 }

// Kind is the set of lexical token kinds (spec §3).
type Kind int

const (
	ILLEGAL Kind = iota
	EOF

	IDENT    // users, "quoted name" (spec §3.4)
	STRING   // 'text', '''multi line''' (spec §3.6, §3.7)
	NUMBER   // 42, 3.14, 1.5e10 (spec §3.9)
	COLOR    // #3498DB (spec §3.11)
	FUNCEXPR // `now()` (spec §3.12)

	LBRACE   // {
	RBRACE   // }
	LBRACKET // [
	RBRACKET // ]
	LPAREN   // (
	RPAREN   // )
	COMMA    // ,
	COLON    // :
	DOT      // .
	TILDE    // ~
	STAR     // *

	LT    // <
	GT    // >
	LTGT  // <>
	MINUS // -
)

var kindNames = [...]string{
	ILLEGAL:  "illegal token",
	EOF:      "end of file",
	IDENT:    "identifier",
	STRING:   "string",
	NUMBER:   "number",
	COLOR:    "color",
	FUNCEXPR: "expression literal",
	LBRACE:   "'{'",
	RBRACE:   "'}'",
	LBRACKET: "'['",
	RBRACKET: "']'",
	LPAREN:   "'('",
	RPAREN:   "')'",
	COMMA:    "','",
	COLON:    "':'",
	DOT:      "'.'",
	TILDE:    "'~'",
	STAR:     "'*'",
	LT:       "'<'",
	GT:       "'>'",
	LTGT:     "'<>'",
	MINUS:    "'-'",
}

func (k Kind) String() string {
	if int(k) < len(kindNames) {
		return kindNames[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// Token is one lexical token. Val holds the processed value (escape
// sequences applied, quotes stripped); Text holds the raw source text.
type Token struct {
	Kind Kind
	Pos  Position
	Text string
	Val  string

	Quoted bool // IDENT written as "quoted identifier"
	Multi  bool // STRING written as '''multi-line'''

	// NLBefore reports whether a line break (or start of file) precedes
	// this token. DBML statements are newline-terminated; this flag plays
	// the same role as Go's automatic semicolon insertion.
	NLBefore bool
	// SpBefore reports whether any whitespace precedes this token.
	SpBefore bool
}

func (t Token) String() string {
	if t.Text != "" {
		return fmt.Sprintf("%s %q", t.Kind, t.Text)
	}
	return t.Kind.String()
}

// End returns the position immediately after the token.
func (t Token) End() Position {
	end := t.Pos
	for _, r := range t.Text {
		end.Offset++
		if r == '\n' {
			end.Line++
			end.Column = 1
		} else {
			end.Column++
		}
	}
	return end
}
