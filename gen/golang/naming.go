// Identifier conversion: DBML names (snake_case, quoted, Unicode) to
// exported Go names, following the Go community conventions for
// initialisms (user_id -> UserID, not UserId).
package golang

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/Piechutowski/not-an-orm/ast"
	"github.com/Piechutowski/not-an-orm/inflect"
)

// commonInitialisms is the golint list: word parts that appear in all-caps
// in idiomatic Go names.
var commonInitialisms = map[string]bool{
	"acl": true, "api": true, "ascii": true, "cpu": true, "css": true,
	"db": true, "dns": true, "eof": true, "guid": true, "html": true,
	"http": true, "https": true, "id": true, "ip": true, "json": true,
	"lhs": true, "qps": true, "ram": true, "rhs": true, "rpc": true,
	"sla": true, "smtp": true, "sql": true, "ssh": true, "tcp": true,
	"tls": true, "ttl": true, "udp": true, "ui": true, "uid": true,
	"uuid": true, "uri": true, "url": true, "utf8": true, "vm": true,
	"xml": true, "xmpp": true, "xsrf": true, "xss": true,
}

// goName converts a DBML name to an exported Go identifier.
//
// Words are split on characters that cannot appear in a Go identifier
// (underscores, spaces, punctuation from quoted names) and on combining
// marks, which are valid in DBML identifiers (spec §3.4) but not in Go.
// Each word is title-cased, or upper-cased if it is a common initialism.
// A leading digit is prefixed with "X" ("2fa_codes" -> "X2faCodes").
// An error is returned when nothing usable remains.
func goName(dbmlName string) (string, error) {
	var words []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range dbmlName {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			cur.WriteRune(r)
		default:
			// separators and characters Go identifiers cannot contain,
			// including combining marks
			flush()
		}
	}
	flush()
	if len(words) == 0 {
		return "", fmt.Errorf("name %q contains no characters usable in a Go identifier", dbmlName)
	}

	var b strings.Builder
	for _, w := range words {
		if commonInitialisms[strings.ToLower(w)] {
			b.WriteString(strings.ToUpper(w))
			continue
		}
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		b.WriteString(string(runes))
	}
	name := b.String()
	if r := []rune(name)[0]; unicode.IsDigit(r) {
		name = "X" + name
	}
	return name, nil
}

// modelName derives the Go model type for a table (decision D10): the
// singular of the table name (users -> User, order_items -> OrderItem) via
// the deterministic inflector, or the [model: '...'] override verbatim
// (normalized by goName). Non-public schemas are prefixed like structName.
// When the inflector is not confident and no [model:] pins the name, the
// vet modelname rule warns; generation itself proceeds with the guess.
func modelName(t *ast.Table) (string, error) {
	if s := t.Settings.Get("model"); s != nil {
		if lit, ok := s.Value.(*ast.BasicLit); ok {
			return goName(lit.Tok.Val)
		}
	}
	singular, _ := inflect.SingularLast(t.Name.Base())
	return structName(t.Name.Schema(), singular)
}

// structName derives the Go type name for a table or enum: tables in the
// default public schema use the bare name; any other schema is prefixed so
// core.users and public.users cannot collide (CoreUsers vs Users).
func structName(schema, base string) (string, error) {
	n, err := goName(base)
	if err != nil {
		return "", err
	}
	if schema == "" || schema == "public" {
		return n, nil
	}
	p, err := goName(schema)
	if err != nil {
		return "", err
	}
	return p + n, nil
}
