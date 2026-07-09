// Package inflect derives the singular model name from a plural table
// name (decision D10). The inflector is deliberately small and
// deterministic: a fixed irregular list, a handful of suffix rules, and a
// confidence bit. When the result is a guess (Singular returns ok=false),
// vet's modelname rule tells the author to pin the name with [model:].
//
// English pluralization is not decidable by rules ("menus" wants menu,
// "status" wants status); the design accepts that and makes the ambiguous
// cases loud instead of clever.
package inflect

import "strings"

// irregular maps whole (lower-cased) plural words to their singular.
// Entries mapping a word to itself are uninflectable nouns.
var irregular = map[string]string{
	"people":   "person",
	"children": "child",
	"men":      "man",
	"women":    "woman",
	"mice":     "mouse",
	"geese":    "goose",
	"feet":     "foot",
	"teeth":    "tooth",
	"indices":  "index",
	"vertices": "vertex",
	"matrices": "matrix",

	// uninflectable: same word both ways
	"news":    "news",
	"series":  "series",
	"species": "species",
	"data":    "data",
	"media":   "media",

	// words the suffix rules below would mangle
	"statuses":  "status",
	"aliases":   "alias",
	"buses":     "bus",
	"heroes":    "hero",
	"potatoes":  "potato",
	"tomatoes":  "tomato",
	"echoes":    "echo",
	"quizzes":   "quiz",
	"analyses":  "analysis",
	"crises":    "crisis",
	"caches":    "cache",
	"movies":    "movie",
	"cookies":   "cookie",
	"addresses": "address",
	"accesses":  "access",
	"processes": "process",
}

// Singular converts one plural noun to singular. ok reports whether the
// answer is confident; when false the caller should treat the result as a
// guess (the vet modelname rule surfaces exactly these).
//
// Compound snake_case names are handled by SingularLast, which inflects
// only the final segment (order_items -> order_item).
func Singular(word string) (string, bool) {
	if word == "" {
		return word, false
	}
	lower := strings.ToLower(word)
	if s, ok := irregular[lower]; ok {
		return matchCase(word, s), true
	}

	switch {
	// categories -> category; "ties"-length words fall through to plain -s
	case strings.HasSuffix(lower, "ies") && len(lower) > 4:
		return word[:len(word)-3] + matchCase(word[len(word)-3:], "y"), true

	// boxes, churches, dishes, buzzes, classes -> drop "es"
	case strings.HasSuffix(lower, "xes"), strings.HasSuffix(lower, "ches"),
		strings.HasSuffix(lower, "shes"), strings.HasSuffix(lower, "zes"),
		strings.HasSuffix(lower, "sses"):
		return word[:len(word)-2], true

	// houses, cases, databases, releases -> drop "s"
	// (statuses/aliases/buses are irregular entries above)
	case strings.HasSuffix(lower, "ses"):
		return word[:len(word)-1], true

	// endings where a trailing "s" is often not a plural marker at all:
	// status, bonus, analysis, axis. Left unchanged, flagged as a guess.
	case strings.HasSuffix(lower, "ss"), strings.HasSuffix(lower, "us"),
		strings.HasSuffix(lower, "is"):
		return word, false

	// the regular case: users -> user
	case strings.HasSuffix(lower, "s"):
		return word[:len(word)-1], true

	// already singular (user, person, staff): identity, and not a guess
	default:
		return word, true
	}
}

// SingularLast inflects only the final underscore-separated segment of a
// compound name: order_items -> order_item, user_roles -> user_role.
func SingularLast(name string) (string, bool) {
	i := strings.LastIndexByte(name, '_')
	if i < 0 || i == len(name)-1 {
		return Singular(name)
	}
	last, ok := Singular(name[i+1:])
	return name[:i+1] + last, ok
}

// matchCase copies the casing shape of src onto s: all-upper src yields
// all-upper s, leading-upper src yields leading-upper s.
func matchCase(src, s string) string {
	if src == strings.ToUpper(src) && src != strings.ToLower(src) {
		return strings.ToUpper(s)
	}
	if len(src) > 0 && src[:1] == strings.ToUpper(src[:1]) && src[:1] != strings.ToLower(src[:1]) {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}
