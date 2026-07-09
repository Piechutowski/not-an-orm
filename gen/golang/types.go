// The SQL-to-Go type mapping. Generated code depends only on the standard
// library. The table is the single place to extend when a schema uses a
// type not listed; an unmapped type is a generation error, never a silent
// guess (see SPEC.md).
package golang

import (
	"fmt"
	"strings"
)

// goType is a resolved field type plus the import it needs, if any.
type goType struct {
	name    string // e.g. "int64", "time.Time"
	imp     string // e.g. "time"; "" if none
	nilable bool   // slice-like: nil already expresses NULL, no pointer needed
}

// typeMap maps the lower-cased DBML type name (parenthesized arguments
// stripped: varchar(255) -> varchar) to its Go representation.
var typeMap = map[string]goType{
	// integers
	"tinyint":     {name: "int16"},
	"int2":        {name: "int16"},
	"smallint":    {name: "int16"},
	"smallserial": {name: "int16"},
	"int":         {name: "int32"},
	"integer":     {name: "int32"},
	"int4":        {name: "int32"},
	"mediumint":   {name: "int32"},
	"serial":      {name: "int32"},
	"bigint":      {name: "int64"},
	"int8":        {name: "int64"},
	"bigserial":   {name: "int64"},

	// unsigned variants (written as quoted types in DBML, spec §6.3.1)
	"tinyint unsigned":  {name: "uint8"},
	"smallint unsigned": {name: "uint16"},
	"int unsigned":      {name: "uint32"},
	"integer unsigned":  {name: "uint32"},
	"bigint unsigned":   {name: "uint64"},

	// floating point
	"real":             {name: "float32"},
	"float4":           {name: "float32"},
	"float":            {name: "float64"},
	"float8":           {name: "float64"},
	"double":           {name: "float64"},
	"double precision": {name: "float64"},

	// exact numerics: strings, so money never rides a float
	"decimal": {name: "string"},
	"numeric": {name: "string"},
	"money":   {name: "string"},

	// booleans
	"bool":    {name: "bool"},
	"boolean": {name: "bool"},

	// character data
	"varchar":           {name: "string"},
	"character varying": {name: "string"},
	"char":              {name: "string"},
	"character":         {name: "string"},
	"text":              {name: "string"},
	"tinytext":          {name: "string"},
	"mediumtext":        {name: "string"},
	"longtext":          {name: "string"},
	"citext":            {name: "string"},
	"string":            {name: "string"},
	"uuid":              {name: "string"},

	// date and time. Time-of-day types are strings: SQLite has no time
	// type, drivers have no scan rule for one, and gen/sqlite stores them
	// as TEXT ("15:04:05").
	"date":                        {name: "time.Time", imp: "time"},
	"time":                        {name: "string"},
	"timetz":                      {name: "string"},
	"time with time zone":         {name: "string"},
	"time without time zone":      {name: "string"},
	"timestamp":                   {name: "time.Time", imp: "time"},
	"timestamptz":                 {name: "time.Time", imp: "time"},
	"timestamp with time zone":    {name: "time.Time", imp: "time"},
	"timestamp without time zone": {name: "time.Time", imp: "time"},
	"datetime":                    {name: "time.Time", imp: "time"},

	// documents and binary: nil expresses NULL, so never pointered
	"json":       {name: "json.RawMessage", imp: "encoding/json", nilable: true},
	"jsonb":      {name: "json.RawMessage", imp: "encoding/json", nilable: true},
	"bytea":      {name: "[]byte", nilable: true},
	"blob":       {name: "[]byte", nilable: true},
	"tinyblob":   {name: "[]byte", nilable: true},
	"mediumblob": {name: "[]byte", nilable: true},
	"longblob":   {name: "[]byte", nilable: true},
	"binary":     {name: "[]byte", nilable: true},
	"varbinary":  {name: "[]byte", nilable: true},
}

// resolveType maps a column's DBML type to Go. enums maps canonical enum
// keys ("public.job_status") to their generated Go type names.
func resolveType(schema, base string, enums map[string]string) (goType, error) {
	// enum reference? the type may be schema-qualified or inherit the
	// default schema (spec §6.8.4)
	key := base
	if schema != "" {
		key = schema + "." + base
	} else {
		key = "public." + base
	}
	if enumType, ok := enums[key]; ok {
		return goType{name: enumType}, nil
	}

	if schema != "" {
		return goType{}, fmt.Errorf("type %s.%s does not name a known enum", schema, base)
	}
	if t, ok := typeMap[strings.ToLower(base)]; ok {
		return t, nil
	}
	return goType{}, fmt.Errorf("no Go mapping for type %q (extend the map in gen/golang/types.go or adjust the schema)", base)
}
