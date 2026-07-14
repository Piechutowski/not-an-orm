package itest

// The generated siblings are checked in so the package always compiles;
// these directives refresh them after editing schema.dbml:
//
//go:generate go run ../cmd/nao gen go --out . --package itest schema.dbml
//go:generate go run ../cmd/nao gen sqlite --out . schema.dbml

import (
	"bytes"
	"os"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	golanggen "github.com/Piechutowski/not-an-orm/gen/golang"
	sqlitegen "github.com/Piechutowski/not-an-orm/gen/sqlite"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
)

// TestGeneratedFilesCurrent proves the checked-in generated files match
// what the generators produce from schema.dbml today — the itest analogue
// of the golden tests. On failure: go generate ./itest
func TestGeneratedFilesCurrent(t *testing.T) {
	src, err := os.ReadFile("schema.dbml")
	if err != nil {
		t.Fatal(err)
	}
	f, diags := parser.ParseFile("schema.dbml", string(src))
	info, semDiags := check.File(f)
	if diags = append(diags, semDiags...); diag.HasErrors(diags) {
		t.Fatalf("schema.dbml must be valid: %v", diags)
	}

	opts := golanggen.Options{Package: "itest", Source: "schema.dbml"}
	models, err := golanggen.Generate(f, info, opts)
	if err != nil {
		t.Fatal(err)
	}
	queries, err := golanggen.GenerateQueries(f, info, opts)
	if err != nil {
		t.Fatal(err)
	}
	schema, err := sqlitegen.Generate(f, info, sqlitegen.Options{Source: "schema.dbml"})
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		file string
		want []byte
	}{
		{"edbml_models.go", models},
		{"edbml_queries.go", queries},
		{"edbml_schema.sql", schema},
	} {
		got, err := os.ReadFile(tc.file)
		if err != nil {
			t.Errorf("%s: %v (run 'go generate ./itest')", tc.file, err)
			continue
		}
		if !bytes.Equal(got, tc.want) {
			t.Errorf("%s is stale; run 'go generate ./itest'", tc.file)
		}
	}
}
