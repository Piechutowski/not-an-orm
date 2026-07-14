// Settings validation (§4.2): per-construct whitelists, value shapes, and
// the at-most-once rule. Table-driven so adding a setting is one map entry.
package check

import (
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
	"github.com/Piechutowski/not-an-orm/edbml/token"
)

// valueKind describes what value shape a setting accepts.
type valueKind int

const (
	flagOnly    valueKind = iota // no value allowed
	strValue                     // string literal
	colorValue                   // color literal (3 or 6 hex digits)
	colorOrNone                  // color literal or the identifier none
	exprValue                    // backtick expression
	identValue                   // an identifier (e.g. index type)
	actionValue                  // referential action; validated separately
	defaultVal                   // §6.4 default value
	refVal                       // §6.7 inline relationship
)

// settingSpec describes the settings a construct accepts.
type settingSpec struct {
	kinds      map[string]valueKind
	repeatable map[string]bool
	synonyms   map[string]string // alt spelling -> canonical
	conflicts  [][2]string       // mutually exclusive pairs
}

var (
	tableSettings = settingSpec{
		// model is our extension (D10): the singular Go model name when the
		// inflector's guess is wrong or unwanted.
		kinds: map[string]valueKind{"headercolor": colorValue, "note": strValue, "model": strValue},
	}
	columnSettings = settingSpec{
		kinds: map[string]valueKind{
			"pk": flagOnly, "primary key": flagOnly,
			"null": flagOnly, "not null": flagOnly,
			"unique": flagOnly, "increment": flagOnly,
			"default": defaultVal, "check": exprValue,
			"note": strValue, "ref": refVal,
		},
		repeatable: map[string]bool{"check": true, "ref": true},
		synonyms:   map[string]string{"primary key": "pk"},
		conflicts:  [][2]string{{"null", "not null"}},
	}
	indexSettings = settingSpec{
		kinds: map[string]valueKind{
			"type": identValue, "name": strValue,
			"unique": flagOnly, "pk": flagOnly, "note": strValue,
		},
	}
	checkSettings = settingSpec{
		kinds: map[string]valueKind{"name": strValue},
	}
	refSettings = settingSpec{
		kinds: map[string]valueKind{
			"delete": actionValue, "update": actionValue,
			"color": colorValue, "inactive": flagOnly,
		},
	}
	enumValueSettings = settingSpec{
		kinds: map[string]valueKind{"note": strValue},
	}
	groupSettings = settingSpec{
		kinds: map[string]valueKind{"note": strValue, "color": colorValue},
	}
	stickySettings = settingSpec{
		kinds: map[string]valueKind{"color": colorOrNone},
	}
)

func (c *checker) checkSettings(sl *ast.SettingList, section string, spec settingSpec) {
	if sl == nil {
		return
	}
	seen := map[string]bool{}
	for _, s := range sl.Settings {
		canon := s.Name
		if alt, ok := spec.synonyms[canon]; ok {
			canon = alt
		}
		kind, known := spec.kinds[s.Name]
		if !known {
			c.errorf(s.Pos(), section, "unknown setting %q", s.Name)
			continue
		}
		if seen[canon] && !spec.repeatable[canon] {
			c.errorf(s.Pos(), "4.2", "setting %q may appear at most once", s.Name)
		}
		seen[canon] = true
		c.checkSettingValue(s, section, kind)
	}
	for _, pair := range spec.conflicts {
		if seen[pair[0]] && seen[pair[1]] {
			c.errorf(sl.Pos(), section, "%q and %q cannot both be set", pair[0], pair[1])
		}
	}
}

func (c *checker) checkSettingValue(s *ast.Setting, section string, kind valueKind) {
	switch kind {
	case flagOnly:
		if s.Value != nil {
			c.errorf(s.Pos(), section, "%q must not have a value", s.Name)
		}
		return
	default:
		if s.Value == nil {
			c.errorf(s.Pos(), section, "%q requires a value", s.Name)
			return
		}
	}
	switch kind {
	case strValue:
		if !isString(s.Value) {
			c.errorf(s.Value.Pos(), section, "%q must be a string", s.Name)
		}
	case colorValue, colorOrNone:
		if kind == colorOrNone {
			if id, ok := s.Value.(*ast.Ident); ok && strings.EqualFold(id.Name(), "none") && !id.Quoted() {
				return
			}
		}
		lit, ok := s.Value.(*ast.BasicLit)
		if !ok || lit.Tok.Kind != token.COLOR {
			c.errorf(s.Value.Pos(), section, "%q must be a color literal", s.Name)
			return
		}
		if !validColor(lit.Tok.Val) {
			c.errorf(s.Value.Pos(), "3.11", "color literal '#%s' must be 3 or 6 hex digits", lit.Tok.Val)
		}
	case exprValue:
		if _, ok := s.Value.(*ast.FuncExpr); !ok {
			c.errorf(s.Value.Pos(), section, "%q must be a backtick expression", s.Name)
		}
	case identValue:
		if _, ok := s.Value.(*ast.Ident); !ok {
			c.errorf(s.Value.Pos(), section, "%q must be an identifier", s.Name)
		}
	case actionValue:
		switch s.Value.(type) {
		case *ast.Ident, parser.MultiWord:
			// the action vocabulary is validated in checkRefSettings
		default:
			c.errorf(s.Value.Pos(), section, "%q must be a referential action", s.Name)
		}
	case defaultVal:
		c.checkDefaultValue(s)
	case refVal:
		if _, ok := s.Value.(*ast.RefValue); !ok {
			c.errorf(s.Value.Pos(), section, "'ref' must be a relationship operator followed by an endpoint")
		}
	}
}

// checkDefaultValue enforces §6.4: number, string, boolean, null,
// expression, or a dotted enum constant — never a bare identifier.
func (c *checker) checkDefaultValue(s *ast.Setting) {
	switch v := s.Value.(type) {
	case *ast.BasicLit:
		if v.Tok.Kind == token.COLOR {
			c.errorf(v.Pos(), "6.4", "default value must be a number, string, boolean, null, expression or enum constant")
		}
	case *ast.FuncExpr, *ast.NegNumber, *ast.EnumConst:
	case *ast.Ident:
		low := strings.ToLower(v.Name())
		if v.Quoted() || (low != "true" && low != "false" && low != "null") {
			c.errorf(v.Pos(), "6.4", "default value must be a number, string, boolean, null, expression or enum constant; found bare identifier %q", v.Name())
		}
	default:
		c.errorf(s.Value.Pos(), "6.4", "default value must be a number, string, boolean, null, expression or enum constant")
	}
}

func isString(n ast.Node) bool {
	lit, ok := n.(*ast.BasicLit)
	return ok && lit.Tok.Kind == token.STRING
}

func validColor(v string) bool {
	if len(v) != 3 && len(v) != 6 {
		return false
	}
	for _, r := range v {
		if !(('0' <= r && r <= '9') || ('a' <= r && r <= 'f') || ('A' <= r && r <= 'F')) {
			return false
		}
	}
	return true
}
