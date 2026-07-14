// The SQL-to-SQLite type mapping (storage affinities) and identifier
// quoting. An unmapped type is a generation error, mirroring gen/golang.
package sqlite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Piechutowski/not-an-orm/edbml/ast"
	"github.com/Piechutowski/not-an-orm/edbml/check"
)

// sqliteType maps the lower-cased DBML type name (arguments stripped) to
// the SQLite storage class it should carry.
var sqliteType = map[string]string{
	// integers (including unsigned quoted variants)
	"tinyint": "INTEGER", "int2": "INTEGER", "smallint": "INTEGER",
	"smallserial": "INTEGER", "int": "INTEGER", "integer": "INTEGER",
	"int4": "INTEGER", "mediumint": "INTEGER", "serial": "INTEGER",
	"bigint": "INTEGER", "int8": "INTEGER", "bigserial": "INTEGER",
	"tinyint unsigned": "INTEGER", "smallint unsigned": "INTEGER",
	"int unsigned": "INTEGER", "integer unsigned": "INTEGER",
	"bigint unsigned": "INTEGER",

	// booleans are integers 0/1 in SQLite
	"bool": "INTEGER", "boolean": "INTEGER",

	// floating point
	"real": "REAL", "float4": "REAL", "float": "REAL", "float8": "REAL",
	"double": "REAL", "double precision": "REAL",

	// exact numerics as TEXT, consistent with the Go backend's string
	"decimal": "TEXT", "numeric": "TEXT", "money": "TEXT",

	// character data
	"varchar": "TEXT", "character varying": "TEXT", "char": "TEXT",
	"character": "TEXT", "text": "TEXT", "tinytext": "TEXT",
	"mediumtext": "TEXT", "longtext": "TEXT", "citext": "TEXT",
	"string": "TEXT", "uuid": "TEXT",

	// date and time: stored as ISO-8601 text (SQLite's own convention),
	// but declared as DATE/TIMESTAMP/DATETIME — the standard SQLite-driver
	// signal (mattn, modernc) to parse the column into a time value on
	// scan. The declared types have NUMERIC affinity, which stores
	// ISO-8601 strings as TEXT anyway. Time-of-day types stay TEXT:
	// drivers have no parse rule for them (gen/golang maps them to string).
	"date": "DATE", "time": "TEXT", "timetz": "TEXT",
	"time with time zone": "TEXT", "time without time zone": "TEXT",
	"timestamp": "TIMESTAMP", "timestamptz": "TIMESTAMP",
	"timestamp with time zone": "TIMESTAMP", "timestamp without time zone": "TIMESTAMP",
	"datetime": "DATETIME",

	// documents and binary
	"json": "TEXT", "jsonb": "TEXT",
	"bytea": "BLOB", "blob": "BLOB", "tinyblob": "BLOB",
	"mediumblob": "BLOB", "longblob": "BLOB",
	"binary": "BLOB", "varbinary": "BLOB",
}

// resolveType maps a column type to its SQLite type; enum-typed columns
// resolve to TEXT plus the enum whose values feed the CHECK constraint.
func (g *generator) resolveType(col *ast.Column) (string, *check.EnumInfo, error) {
	schema, base := col.Type.Name.Schema(), col.Type.Name.Base()
	key := "public." + base
	if schema != "" {
		key = schema + "." + base
	}
	if e, ok := g.enums[key]; ok {
		return "TEXT", e, nil
	}
	if schema != "" {
		return "", nil, fmt.Errorf("type %s.%s does not name a known enum", schema, base)
	}
	if t, ok := sqliteType[strings.ToLower(base)]; ok {
		return t, nil, nil
	}
	return "", nil, fmt.Errorf("no SQLite mapping for type %q (extend the map in gen/sqlite/types.go)", base)
}

/* ===== identifier and string quoting ===== */

var plainIdent = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// sqliteKeywords is the SQLite keyword list (https://sqlite.org/lang_keywords.html).
var sqliteKeywords = map[string]bool{
	"abort": true, "action": true, "add": true, "after": true, "all": true,
	"alter": true, "always": true, "analyze": true, "and": true, "as": true,
	"asc": true, "attach": true, "autoincrement": true, "before": true,
	"begin": true, "between": true, "by": true, "cascade": true, "case": true,
	"cast": true, "check": true, "collate": true, "column": true,
	"commit": true, "conflict": true, "constraint": true, "create": true,
	"cross": true, "current": true, "current_date": true,
	"current_time": true, "current_timestamp": true, "database": true,
	"default": true, "deferrable": true, "deferred": true, "delete": true,
	"desc": true, "detach": true, "distinct": true, "do": true, "drop": true,
	"each": true, "else": true, "end": true, "escape": true, "except": true,
	"exclude": true, "exclusive": true, "exists": true, "explain": true,
	"fail": true, "filter": true, "first": true, "following": true,
	"for": true, "foreign": true, "from": true, "full": true,
	"generated": true, "glob": true, "group": true, "groups": true,
	"having": true, "if": true, "ignore": true, "immediate": true,
	"in": true, "index": true, "indexed": true, "initially": true,
	"inner": true, "insert": true, "instead": true, "intersect": true,
	"into": true, "is": true, "isnull": true, "join": true, "key": true,
	"last": true, "left": true, "like": true, "limit": true, "match": true,
	"materialized": true, "natural": true, "no": true, "not": true,
	"nothing": true, "notnull": true, "null": true, "nulls": true,
	"of": true, "offset": true, "on": true, "or": true, "order": true,
	"others": true, "outer": true, "over": true, "partition": true,
	"plan": true, "pragma": true, "preceding": true, "primary": true,
	"query": true, "raise": true, "range": true, "recursive": true,
	"references": true, "regexp": true, "reindex": true, "release": true,
	"rename": true, "replace": true, "restrict": true, "returning": true,
	"right": true, "rollback": true, "row": true, "rows": true,
	"savepoint": true, "select": true, "set": true, "table": true,
	"temp": true, "temporary": true, "then": true, "ties": true, "to": true,
	"transaction": true, "trigger": true, "unbounded": true, "union": true,
	"unique": true, "update": true, "using": true, "vacuum": true,
	"values": true, "view": true, "virtual": true, "when": true,
	"where": true, "window": true, "with": true, "without": true,
}

// quoteIdent quotes an identifier only when necessary: not a plain
// lower-case identifier, or a SQLite keyword.
func quoteIdent(name string) string {
	if plainIdent.MatchString(name) && !sqliteKeywords[name] {
		return name
	}
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteJoin(names []string) string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = quoteIdent(n)
	}
	return strings.Join(out, ", ")
}

// quoteString renders a SQL string literal with ” doubling.
func quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}
