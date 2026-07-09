package vet_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/Piechutowski/not-an-orm/vet"
)

// TestRulesDocumentation pins RULES.md, the analyzer registry and the
// testdata corpus to each other. If any of the three drifts, the build
// fails:
//
//  1. every registered analyzer has a "### <name>" section in RULES.md;
//  2. every "### <name>" section documents a registered analyzer (no
//     stale docs for removed rules);
//  3. every section links at least one existing testdata/*.dbml file;
//  4. among a rule's linked files, at least one runs the rule (its
//     "// analyzers:" header names it) and contains at least one bad
//     example ("//WANT <name>") — TestAnalyzers separately proves the
//     bad examples warn and the good lines stay silent;
//  5. every testdata file is linked from RULES.md (no orphaned examples).
func TestRulesDocumentation(t *testing.T) {
	docBytes, err := os.ReadFile("RULES.md")
	if err != nil {
		t.Fatalf("RULES.md: %v", err)
	}
	doc := string(docBytes)

	headingRE := regexp.MustCompile(`(?m)^### ([a-z]+)$`)
	linkRE := regexp.MustCompile(`\((testdata/[a-z_]+\.dbml)\)`)

	// Slice the document into per-rule sections.
	headings := headingRE.FindAllStringSubmatchIndex(doc, -1)
	sections := map[string]string{}
	for i, h := range headings {
		name := doc[h[2]:h[3]]
		end := len(doc)
		if i+1 < len(headings) {
			end = headings[i+1][0]
		}
		if _, dup := sections[name]; dup {
			t.Errorf("RULES.md: duplicate section for %q", name)
		}
		sections[name] = doc[h[1]:end]
	}

	registered := map[string]bool{}
	for _, a := range vet.All() {
		registered[a.Name] = true
	}

	// (2) no stale sections
	for name := range sections {
		if !registered[name] {
			t.Errorf("RULES.md documents %q, which is not a registered analyzer", name)
		}
	}

	linkedAnywhere := map[string]bool{}
	for _, a := range vet.All() {
		sec, ok := sections[a.Name]
		if !ok {
			// (1) every analyzer documented
			t.Errorf("analyzer %q has no '### %s' section in RULES.md", a.Name, a.Name)
			continue
		}
		links := linkRE.FindAllStringSubmatch(sec, -1)
		if len(links) == 0 {
			// (3) every section links examples
			t.Errorf("RULES.md section %q links no testdata file", a.Name)
			continue
		}
		ranByLinked, wantInLinked := false, false
		for _, m := range links {
			rel := m[1]
			linkedAnywhere[filepath.Base(rel)] = true
			src, err := os.ReadFile(rel)
			if err != nil {
				t.Errorf("RULES.md section %q links %s: %v", a.Name, rel, err)
				continue
			}
			lines := strings.Split(string(src), "\n")
			header := strings.TrimPrefix(strings.TrimSpace(lines[0]), "// analyzers:")
			for _, n := range strings.Split(header, ",") {
				if strings.TrimSpace(n) == a.Name {
					ranByLinked = true
				}
			}
			if strings.Contains(string(src), "//WANT") && containsWant(string(src), a.Name) {
				wantInLinked = true
			}
		}
		// (4) linked examples actually exercise the rule
		if !ranByLinked {
			t.Errorf("no file linked from RULES.md section %q runs the analyzer (missing from '// analyzers:' header)", a.Name)
		}
		if !wantInLinked {
			t.Errorf("no file linked from RULES.md section %q contains a bad example ('//WANT %s')", a.Name, a.Name)
		}
	}

	// (5) no orphaned testdata
	files, err := filepath.Glob(filepath.Join("testdata", "*.dbml"))
	if err != nil || len(files) == 0 {
		t.Fatal("no testdata files")
	}
	for _, f := range files {
		if !linkedAnywhere[filepath.Base(f)] {
			t.Errorf("testdata file %s is not linked from RULES.md", f)
		}
	}
}

// containsWant reports whether src has a //WANT marker naming the analyzer.
func containsWant(src, name string) bool {
	for _, m := range regexp.MustCompile(`//WANT ([a-z,]+)`).FindAllStringSubmatch(src, -1) {
		for _, n := range strings.Split(m[1], ",") {
			if n == name {
				return true
			}
		}
	}
	return false
}
