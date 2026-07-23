package rt

import (
	"reflect"
	"strings"
	"testing"
)

// The interpreter is pure: options in, SQL text + args out. These tests
// pin the rendered text exactly — the text is a statement-cache key
// (D31), so any change to rendering is a cache-behavior change and must
// be visible in review.

type m struct{} // phantom model for handle construction in tests

var (
	id    = Column[m, int64]{Name: "id"}
	email = Column[m, string]{Name: "email"}
	bio   = NullColumn[m, string]{Column[m, string]{Name: "bio"}}
)

func selectRender(t *testing.T, opts ...Opt[m]) (string, []any) {
	t.Helper()
	query, args, err := SelectRender(`"users"`, `"id", "email", "bio"`, opts)
	if err != nil {
		t.Fatalf("SelectRender: %v", err)
	}
	return query, args
}

func argsCheck(t *testing.T, got []any, want ...any) {
	t.Helper()
	if len(got) == 0 && len(want) == 0 {
		return
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %#v, want %#v", got, want)
	}
}

func TestSelectRenderBare(t *testing.T) {
	query, args := selectRender(t)
	if want := `SELECT "id", "email", "bio" FROM "users"`; query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args)
}

func TestSelectRenderOperators(t *testing.T) {
	cases := []struct {
		name string
		pred Pred[m]
		sql  string
		args []any
	}{
		{"eq", email.Eq("a"), `"email" = ?`, []any{"a"}},
		{"ne", email.Ne("a"), `"email" <> ?`, []any{"a"}},
		{"gt", id.Gt(4), `"id" > ?`, []any{int64(4)}},
		{"ge", id.Ge(4), `"id" >= ?`, []any{int64(4)}},
		{"lt", id.Lt(4), `"id" < ?`, []any{int64(4)}},
		{"le", id.Le(4), `"id" <= ?`, []any{int64(4)}},
		{"like", email.Like("%@x%"), `"email" LIKE ?`, []any{"%@x%"}},
		{"notlike", email.NotLike("%@x%"), `"email" NOT LIKE ?`, []any{"%@x%"}},
		{"in", id.In(1, 2, 3), `"id" IN (?, ?, ?)`, []any{int64(1), int64(2), int64(3)}},
		{"in empty", id.In(), `1 = 0`, nil},
		{"notin", id.NotIn(1), `"id" NOT IN (?)`, []any{int64(1)}},
		{"notin empty", id.NotIn(), `1 = 1`, nil},
		{"isnull", bio.IsNull(), `"bio" IS NULL`, nil},
		{"isnotnull", bio.IsNotNull(), `"bio" IS NOT NULL`, nil},
		{"eqcol", email.EqCol(Column[m, string]{Name: "name"}), `"email" = "name"`, nil},
		{"ltcol", id.LtCol(Column[m, int64]{Name: "parent_id"}), `"id" < "parent_id"`, nil},
		{"raw", Raw[m]("length(email) > ?", 3), `(length(email) > ?)`, []any{3}},
		{"and", And(email.Eq("a"), id.Gt(1)), `("email" = ? AND "id" > ?)`, []any{"a", int64(1)}},
		{"or", Or(email.Eq("a"), bio.IsNull()), `("email" = ? OR "bio" IS NULL)`, []any{"a"}},
		{"not", Not(email.Eq("a")), `NOT ("email" = ?)`, []any{"a"}},
		{
			"nested",
			And(Or(id.Lt(1), id.Gt(9)), Not(And(email.Eq("a"), bio.IsNotNull()))),
			`(("id" < ? OR "id" > ?) AND NOT (("email" = ? AND "bio" IS NOT NULL)))`,
			[]any{int64(1), int64(9), "a"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, args := selectRender(t, tc.pred)
			want := `SELECT "id", "email", "bio" FROM "users" WHERE ` + tc.sql
			if query != want {
				t.Errorf("query = %q, want %q", query, want)
			}
			argsCheck(t, args, tc.args...)
		})
	}
}

func TestSelectRenderEmptyPreds(t *testing.T) {
	var zero Pred[m]
	cases := []struct {
		name string
		opts []Opt[m]
	}{
		{"zero value", []Opt[m]{zero}},
		{"and of nothing", []Opt[m]{And[m]()}},
		{"and of zeros", []Opt[m]{And(zero, zero)}},
		{"not of zero", []Opt[m]{Not(zero)}},
		{"raw empty", []Opt[m]{Raw[m]("")}},
		{"nil opt", []Opt[m]{nil}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			query, args := selectRender(t, tc.opts...)
			if want := `SELECT "id", "email", "bio" FROM "users"`; query != want {
				t.Errorf("query = %q, want %q", query, want)
			}
			argsCheck(t, args)
		})
	}
	// a single surviving operand collapses: no needless parens in the text
	query, _ := selectRender(t, And(zero, email.Eq("a"), zero))
	if want := `SELECT "id", "email", "bio" FROM "users" WHERE "email" = ?`; query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
}

func TestSelectRenderMultiplePredsJoinWithAnd(t *testing.T) {
	query, args := selectRender(t, email.Like("%x%"), bio.IsNull())
	want := `SELECT "id", "email", "bio" FROM "users" WHERE "email" LIKE ? AND "bio" IS NULL`
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args, "%x%")
}

func TestSelectRenderShaping(t *testing.T) {
	query, args := selectRender(t,
		Distinct[m](),
		email.Like("%@x"),
		OrderBy(email.Asc(), id.Desc()),
		Limit[m](10),
		Offset[m](20),
	)
	want := `SELECT DISTINCT "id", "email", "bio" FROM "users" WHERE "email" LIKE ? ORDER BY "email", "id" DESC LIMIT ? OFFSET ?`
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args, "%@x", 10, 20)
}

func TestSelectRenderOffsetWithoutLimit(t *testing.T) {
	query, args := selectRender(t, Offset[m](5))
	if want := `SELECT "id", "email", "bio" FROM "users" LIMIT -1 OFFSET ?`; query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args, 5)
}

func TestSelectRenderAfter(t *testing.T) {
	// single ascending term
	query, args := selectRender(t, OrderBy(id.Asc()), After[m](int64(7)))
	want := `SELECT "id", "email", "bio" FROM "users" WHERE ("id" > ?) ORDER BY "id"`
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args, int64(7))

	// mixed directions, existing WHERE, limit: the lexicographic expansion
	query, args = selectRender(t,
		bio.IsNotNull(),
		OrderBy(email.Desc(), id.Asc()),
		After[m]("bob@x", int64(3)),
		Limit[m](2),
	)
	want = `SELECT "id", "email", "bio" FROM "users" WHERE "bio" IS NOT NULL AND ` +
		`(("email" < ?) OR ("email" = ? AND "id" > ?)) ORDER BY "email" DESC, "id" LIMIT ?`
	if query != want {
		t.Errorf("query = %q, want %q", query, want)
	}
	argsCheck(t, args, "bob@x", "bob@x", int64(3), 2)
}

func TestSelectRenderAfterErrors(t *testing.T) {
	if _, _, err := SelectRender(`"users"`, `*`, []Opt[m]{After[m](1)}); err == nil || !strings.Contains(err.Error(), "OrderBy") {
		t.Errorf("After without OrderBy: err = %v, want OrderBy complaint", err)
	}
	if _, _, err := SelectRender(`"users"`, `*`, []Opt[m]{OrderBy(id.Asc(), email.Asc()), After[m](1)}); err == nil || !strings.Contains(err.Error(), "key values") {
		t.Errorf("After arity mismatch: err = %v, want key-count complaint", err)
	}
}

func TestCountExistsRender(t *testing.T) {
	query, args, err := CountRender(`"users"`, []Pred[m]{email.Eq("a")})
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT count(*) FROM "users" WHERE "email" = ?`; query != want {
		t.Errorf("count = %q, want %q", query, want)
	}
	argsCheck(t, args, "a")

	query, _, err = CountRender[m](`"users"`, nil)
	if err != nil || query != `SELECT count(*) FROM "users"` {
		t.Errorf("bare count = %q, %v", query, err)
	}

	query, args, err = ExistsRender(`"users"`, []Pred[m]{id.Gt(int64(0))})
	if err != nil {
		t.Fatal(err)
	}
	if want := `SELECT EXISTS (SELECT 1 FROM "users" WHERE "id" > ?)`; query != want {
		t.Errorf("exists = %q, want %q", query, want)
	}
	argsCheck(t, args, int64(0))
}

func TestDeleteUpdateRender(t *testing.T) {
	query, args, err := DeleteRender(`"users"`, []Pred[m]{id.Eq(1)})
	if err != nil {
		t.Fatal(err)
	}
	if want := `DELETE FROM "users" WHERE "id" = ?`; query != want {
		t.Errorf("delete = %q, want %q", query, want)
	}
	argsCheck(t, args, int64(1))

	query, args, err = UpdateRender(`"users"`,
		[]Assign[m]{email.Set("new@x"), bio.SetNull()},
		[]Pred[m]{id.Eq(1)},
	)
	if err != nil {
		t.Fatal(err)
	}
	if want := `UPDATE "users" SET "email" = ?, "bio" = ? WHERE "id" = ?`; query != want {
		t.Errorf("update = %q, want %q", query, want)
	}
	argsCheck(t, args, "new@x", nil, int64(1))
}

// TestWholeTableGuards pins the loud refusals: a DELETE or UPDATE whose
// predicates all vanish must not silently become a whole-table statement.
func TestWholeTableGuards(t *testing.T) {
	var zero Pred[m]
	if _, _, err := DeleteRender[m](`"users"`, nil); err == nil {
		t.Error("DeleteRender(nil preds): want error")
	}
	if _, _, err := DeleteRender(`"users"`, []Pred[m]{zero, And(zero)}); err == nil {
		t.Error("DeleteRender(empty preds): want error")
	}
	if _, _, err := UpdateRender(`"users"`, []Assign[m]{email.Set("x")}, nil); err == nil {
		t.Error("UpdateRender(no preds): want error")
	}
	if _, _, err := UpdateRender(`"users"`, nil, []Pred[m]{id.Eq(1)}); err == nil {
		t.Error("UpdateRender(no assignments): want error")
	}
	// the explicit way to mean "every row"
	if _, _, err := DeleteRender(`"users"`, []Pred[m]{Raw[m]("1 = 1")}); err != nil {
		t.Errorf("DeleteRender(Raw true): %v", err)
	}
}

// TestRenderDeterminism pins D31's premise: the same option values render
// the same SQL text, so the text can key the statement cache.
func TestRenderDeterminism(t *testing.T) {
	build := func() []Opt[m] {
		return []Opt[m]{
			Or(email.Like("%a%"), And(id.Gt(1), bio.IsNull())),
			OrderBy(id.Desc()),
			Limit[m](3),
		}
	}
	q1, a1, err1 := SelectRender(`"users"`, `*`, build())
	q2, a2, err2 := SelectRender(`"users"`, `*`, build())
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if q1 != q2 {
		t.Errorf("same tree rendered differently:\n%q\n%q", q1, q2)
	}
	if !reflect.DeepEqual(a1, a2) {
		t.Errorf("same tree bound differently: %#v vs %#v", a1, a2)
	}
	// different values, same shape: same text (limit is a parameter)
	q3, _, _ := SelectRender(`"users"`, `*`, []Opt[m]{
		Or(email.Like("%zzz%"), And(id.Gt(99), bio.IsNull())),
		OrderBy(id.Desc()),
		Limit[m](8),
	})
	if q1 != q3 {
		t.Errorf("same shape, different values changed the SQL text:\n%q\n%q", q1, q3)
	}
}

// TestIdentQuote pins quoting: hostile identifiers stay identifiers.
func TestIdentQuote(t *testing.T) {
	weird := Column[m, string]{Name: `evil"col`}
	query, _ := selectRender(t, weird.Eq("x"))
	if !strings.Contains(query, `"evil""col" = ?`) {
		t.Errorf("quoting failed: %q", query)
	}
}
