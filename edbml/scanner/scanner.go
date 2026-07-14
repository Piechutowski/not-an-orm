// Package scanner tokenizes DBML source (spec §3), playing the role of
// go/scanner in the Go toolchain.
//
// The implementation uses Rob Pike's state-function design ("Lexical
// Scanning in Go", GTUG Sydney 2011): the scanner is a loop
//
//	for state := anyScan; state != nil; state = state(s) {}
//
// where each state is a func(*Scanner) stateFn that consumes input, emits
// tokens, and returns the next state. Control flow lives in the type
// system instead of an explicit state variable.
//
// Two rules govern tokenization (spec §3.1): left-to-right single pass,
// and longest match. Newlines are not emitted as tokens; instead each
// token records whether a line break precedes it (Token.NLBefore) — the
// same trick as Go's automatic semicolon insertion, adapted to DBML's
// newline-terminated statements.
package scanner

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

// Scan tokenizes src. It always returns the tokens produced so far, with a
// terminating EOF token, plus any lexical errors found (spec §3). Scanning
// continues after an error where possible, so a broken file still yields a
// best-effort token stream for the parser.
func Scan(filename, src string) ([]token.Token, []diag.Diagnostic) {
	// §3.2.1: carriage returns are discarded wherever they appear.
	src = strings.ReplaceAll(src, "\r", "")
	s := &Scanner{
		src:  src,
		file: filename,
		pos:  token.Position{Filename: filename, Line: 1, Column: 1},
	}
	s.nlBefore = true // start of file counts as a line break
	for state := anyScan; state != nil; {
		state = state(s)
	}
	s.emit(token.EOF, "")
	return s.toks, s.errs
}

type stateFn func(*Scanner) stateFn

type Scanner struct {
	src  string
	file string

	pos   token.Position // position of next unread rune
	start token.Position // position where the current token began

	nlBefore bool
	spBefore bool

	toks []token.Token
	errs []diag.Diagnostic

	// val accumulates the processed value for tokens whose Val differs
	// from their raw text (strings, quoted identifiers).
	val strings.Builder
}

const eof = rune(-1)

func (s *Scanner) peek() rune {
	if s.pos.Offset >= len(s.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(s.src[s.pos.Offset:])
	return r
}

func (s *Scanner) peekAt(n int) rune {
	off := s.pos.Offset
	for ; n > 0; n-- {
		if off >= len(s.src) {
			return eof
		}
		_, w := utf8.DecodeRuneInString(s.src[off:])
		off += w
	}
	if off >= len(s.src) {
		return eof
	}
	r, _ := utf8.DecodeRuneInString(s.src[off:])
	return r
}

func (s *Scanner) next() rune {
	if s.pos.Offset >= len(s.src) {
		return eof
	}
	r, w := utf8.DecodeRuneInString(s.src[s.pos.Offset:])
	s.pos.Offset += w
	if r == '\n' {
		s.pos.Line++
		s.pos.Column = 1
	} else {
		s.pos.Column++
	}
	return r
}

func (s *Scanner) mark() { s.start = s.pos; s.val.Reset() }

func (s *Scanner) raw() string { return s.src[s.start.Offset:s.pos.Offset] }

func (s *Scanner) emit(kind token.Kind, val string) {
	s.toks = append(s.toks, token.Token{
		Kind: kind, Pos: s.start, Text: s.raw(), Val: val,
		NLBefore: s.nlBefore, SpBefore: s.spBefore || s.nlBefore,
	})
	s.nlBefore, s.spBefore = false, false
}

func (s *Scanner) tokEmit(t token.Token) {
	t.Pos = s.start
	t.Text = s.raw()
	t.NLBefore = s.nlBefore
	t.SpBefore = s.spBefore || s.nlBefore
	s.toks = append(s.toks, t)
	s.nlBefore, s.spBefore = false, false
}

func (s *Scanner) errorf(code, format string, args ...any) {
	s.errs = append(s.errs, diag.Errorf(s.start, code, format, args...))
}

// §3.4: letter = Unicode category L | Unicode category M | "_".
func isLetter(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsMark(r)
}
func isDigit(r rune) bool     { return '0' <= r && r <= '9' }
func isIdentChar(r rune) bool { return isLetter(r) || isDigit(r) }
func isHex(r rune) bool {
	return isDigit(r) || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F')
}

/* ===== state functions ===== */

// anyScan is the top-level state: it dispatches on the next rune.
func anyScan(s *Scanner) stateFn {
	s.mark()
	switch r := s.peek(); {
	case r == eof:
		return nil
	case r == ' ' || r == '\t':
		s.next()
		s.spBefore = true
		return anyScan
	case r == '\n':
		s.next()
		s.nlBefore = true
		return anyScan
	case r == '/' && s.peekAt(1) == '/':
		return lineCommentScan
	case r == '/' && s.peekAt(1) == '*':
		return blockCommentScan
	case r == '\'':
		if s.peekAt(1) == '\'' && s.peekAt(2) == '\'' {
			return multiStringScan
		}
		return stringScan
	case r == '"':
		return quotedIdentScan
	case r == '`':
		return funcExprScan
	case r == '#':
		return colorScan
	case isDigit(r):
		return numberOrIdentScan
	case isLetter(r):
		return identScan
	default:
		return operatorScan
	}
}

func lineCommentScan(s *Scanner) stateFn {
	for r := s.peek(); r != '\n' && r != eof; r = s.peek() {
		s.next()
	}
	s.spBefore = true // comments are trivia (§3.3.4)
	return anyScan
}

func blockCommentScan(s *Scanner) stateFn {
	s.next()
	s.next() // consume /*
	for {
		switch {
		case s.peek() == eof:
			s.errorf("syntax", "unterminated block comment (§3.3)")
			return nil
		case s.peek() == '*' && s.peekAt(1) == '/':
			s.next()
			s.next()
			s.spBefore = true
			return anyScan
		default:
			s.next()
		}
	}
}

// escapeScan handles one escape sequence (§3.8); the backslash is consumed.
func (s *Scanner) escapeScan() {
	switch r := s.next(); r {
	case 't':
		s.val.WriteByte('\t')
	case 'n':
		s.val.WriteByte('\n')
	case 'r':
		s.val.WriteByte('\r')
	case '0':
		s.val.WriteByte(0)
	case 'b':
		s.val.WriteByte('\b')
	case 'v':
		s.val.WriteByte('\v')
	case 'f':
		s.val.WriteByte('\f')
	case '\n': // line continuation
	case 'u':
		hex := make([]rune, 0, 4)
		for i := 0; i < 4; i++ {
			if !isHex(s.peek()) {
				s.errorf("syntax", `invalid unicode escape '\u%s': exactly 4 hex digits required (§3.8)`, string(hex))
				return
			}
			hex = append(hex, s.next())
		}
		n, _ := strconv.ParseUint(string(hex), 16, 32)
		s.val.WriteRune(rune(n))
	case eof:
		s.val.WriteByte('\\')
	default:
		// §3.8: any other escaped character yields the character itself
		s.val.WriteRune(r)
	}
}

func stringScan(s *Scanner) stateFn {
	s.next() // opening '
	for {
		switch r := s.peek(); r {
		case eof:
			s.errorf("syntax", "unterminated string (§3.6)")
			return nil
		case '\n':
			s.errorf("syntax", "newline in single-line string (§3.6)")
			return anyScan
		case '\'':
			s.next()
			s.tokEmit(token.Token{Kind: token.STRING, Val: s.val.String()})
			return anyScan
		case '\\':
			s.next()
			s.escapeScan()
		default:
			s.val.WriteRune(s.next())
		}
	}
}

func multiStringScan(s *Scanner) stateFn {
	s.next()
	s.next()
	s.next() // opening '''
	for {
		switch r := s.peek(); {
		case r == eof:
			s.errorf("syntax", "unterminated multi-line string (§3.7)")
			return nil
		case r == '\'' && s.peekAt(1) == '\'' && s.peekAt(2) == '\'':
			s.next()
			s.next()
			s.next()
			s.tokEmit(token.Token{Kind: token.STRING, Val: indentStrip(s.val.String()), Multi: true})
			return anyScan
		case r == '\\':
			s.next()
			s.escapeScan()
		default:
			s.val.WriteRune(s.next())
		}
	}
}

// indentStrip implements §3.7.4 indentation stripping.
func indentStrip(v string) string {
	lines := strings.Split(v, "\n")
	if len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	min := -1
	for _, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		n := len(ln) - len(strings.TrimLeft(ln, " "))
		if min < 0 || n < min {
			min = n
		}
	}
	if min > 0 {
		for i, ln := range lines {
			if len(ln) >= min {
				lines[i] = ln[min:]
			} else {
				lines[i] = strings.TrimLeft(ln, " ")
			}
		}
	}
	return strings.Join(lines, "\n")
}

func quotedIdentScan(s *Scanner) stateFn {
	s.next() // opening "
	for {
		switch r := s.peek(); r {
		case eof:
			s.errorf("syntax", "unterminated quoted identifier (§3.4)")
			return nil
		case '\n':
			s.errorf("syntax", "newline in quoted identifier (§3.4)")
			return anyScan
		case '"':
			s.next()
			s.tokEmit(token.Token{Kind: token.IDENT, Val: s.val.String(), Quoted: true})
			return anyScan
		case '\\':
			s.next()
			s.escapeScan()
		default:
			s.val.WriteRune(s.next())
		}
	}
}

func funcExprScan(s *Scanner) stateFn {
	s.next() // opening `
	for {
		switch r := s.peek(); r {
		case eof:
			s.errorf("syntax", "unterminated expression literal (§3.12)")
			return nil
		case '`':
			s.next()
			s.tokEmit(token.Token{Kind: token.FUNCEXPR, Val: s.val.String()})
			return anyScan
		default:
			// §3.12: raw content, no escapes
			s.val.WriteRune(s.next())
		}
	}
}

func colorScan(s *Scanner) stateFn {
	s.next() // #
	for isIdentChar(s.peek()) {
		s.next()
	}
	s.tokEmit(token.Token{Kind: token.COLOR, Val: strings.TrimPrefix(s.raw(), "#")})
	return anyScan
}

func identScan(s *Scanner) stateFn {
	for isIdentChar(s.peek()) {
		s.next()
	}
	s.tokEmit(token.Token{Kind: token.IDENT, Val: s.raw()})
	return anyScan
}

// numberOrIdentScan resolves the digit-leading ambiguity of §3.4/§3.9:
// a token of digits (one optional dot, optional exponent) is a NUMBER; a
// digit-leading token containing letters is an IDENT (no dot allowed).
func numberOrIdentScan(s *Scanner) stateFn {
	nDots, hasLetter := 0, false
	for {
		switch r := s.peek(); {
		case isDigit(r):
			s.next()
		case r == '.' && !hasLetter && nDots == 0 && isDigit(s.peekAt(1)):
			nDots++
			s.next()
		case (r == 'e' || r == 'E') && !hasLetter && s.validExponent():
			s.next()
			if s.peek() == '+' || s.peek() == '-' {
				s.next()
			}
			for isDigit(s.peek()) {
				s.next()
			}
			if isIdentChar(s.peek()) {
				hasLetter = true
				continue
			}
			s.tokEmit(token.Token{Kind: token.NUMBER, Val: s.raw()})
			return anyScan
		case isLetter(r):
			hasLetter = true
			s.next()
		default:
			if hasLetter {
				if nDots > 0 {
					s.errorf("syntax", "invalid number %q (§3.9)", s.raw())
					s.tokEmit(token.Token{Kind: token.ILLEGAL, Val: s.raw()})
					return anyScan
				}
				s.tokEmit(token.Token{Kind: token.IDENT, Val: s.raw()})
				return anyScan
			}
			s.tokEmit(token.Token{Kind: token.NUMBER, Val: s.raw()})
			return anyScan
		}
	}
}

func (s *Scanner) validExponent() bool {
	c := s.peekAt(1)
	if isDigit(c) {
		return true
	}
	if c == '+' || c == '-' {
		return isDigit(s.peekAt(2))
	}
	return false
}

var singleOps = map[rune]token.Kind{
	'{': token.LBRACE, '}': token.RBRACE,
	'[': token.LBRACKET, ']': token.RBRACKET,
	'(': token.LPAREN, ')': token.RPAREN,
	',': token.COMMA, ':': token.COLON, '.': token.DOT,
	'~': token.TILDE, '*': token.STAR,
	'-': token.MINUS, '>': token.GT,
}

func operatorScan(s *Scanner) stateFn {
	r := s.next()
	if r == '<' {
		if s.peek() == '>' { // §3.1 longest match: <> is one token
			s.next()
			s.emit(token.LTGT, "<>")
		} else {
			s.emit(token.LT, "<")
		}
		return anyScan
	}
	if k, ok := singleOps[r]; ok {
		s.emit(k, string(r))
		return anyScan
	}
	s.errorf("syntax", "unexpected character %q (§3)", string(r))
	s.tokEmit(token.Token{Kind: token.ILLEGAL, Val: string(r)})
	return anyScan
}
