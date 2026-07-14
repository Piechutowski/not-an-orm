package check_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Piechutowski/not-an-orm/edbml/check"
	"github.com/Piechutowski/not-an-orm/edbml/diag"
	"github.com/Piechutowski/not-an-orm/edbml/parser"
)

// TestConformanceCorpus runs the spec's snippet corpus through the full
// front end: every valid snippet must produce zero errors, every invalid
// snippet at least one. The corpus verdicts are cross-checked against the
// upstream @dbml/parse compiler (see conformance/refcheck), so this test
// pins the library to the reference implementation's behavior.
func TestConformanceCorpus(t *testing.T) {
	root := filepath.Join("..", "conformance", "snippets")
	for _, group := range []struct {
		dir       string
		wantError bool
	}{
		{"valid", false},
		{"invalid", true},
	} {
		files, err := filepath.Glob(filepath.Join(root, group.dir, "*.dbml"))
		if err != nil || len(files) == 0 {
			t.Fatalf("no snippets in %s", filepath.Join(root, group.dir))
		}
		for _, file := range files {
			file := file
			t.Run(group.dir+"/"+filepath.Base(file), func(t *testing.T) {
				src, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				f, diags := parser.ParseFile(file, string(src))
				_, semDiags := check.File(f)
				diags = append(diags, semDiags...)
				gotError := diag.HasErrors(diags)
				if gotError != group.wantError {
					t.Errorf("want error=%v, got error=%v; diagnostics: %v", group.wantError, gotError, diags)
				}
			})
		}
	}
}
