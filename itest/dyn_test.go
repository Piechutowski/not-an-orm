// Integration tests for the v2 dynamic query layer (decisions D28-D34):
// predicates, options and typed assignments run against a real SQLite
// through a real driver, sharing the fixture with the CRUD tests (D25).
package itest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Piechutowski/not-an-orm/rt"
)

// errTxAbort is the sentinel a test callback returns to force a rollback.
var errTxAbort = errors.New("abort")

// dynSeed creates three users: two sharing a name (for tiebreak
// pagination), one with a bio (the others stay NULL).
func dynSeed(t *testing.T, q *Queries) (ann, bea, bob User) {
	t.Helper()
	ctx := context.Background()
	create := func(email, name string, bio rt.Null[string]) User {
		u, err := q.UserCreate(ctx, UserCreateParams{Email: email, Name: name, Bio: bio})
		if err != nil {
			t.Fatalf("UserCreate(%s): %v", email, err)
		}
		return u
	}
	ann = create("ann@example.com", "Ann", rt.Some("gardener"))
	bea = create("bea@example.com", "Bo", rt.Null[string]{})
	bob = create("bob@example.com", "Bo", rt.Null[string]{})
	return ann, bea, bob
}

func emails(us []User) []string {
	out := make([]string, len(us))
	for i, u := range us {
		out[i] = u.Email
	}
	return out
}

func TestUserQueryPredicates(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	ann, bea, bob := dynSeed(t, q)

	cases := []struct {
		name string
		opts []rt.Opt[User]
		want []string // expected emails, in id order
	}{
		{"eq", []rt.Opt[User]{UserCols.Email.Eq("ann@example.com")}, []string{"ann@example.com"}},
		{"like", []rt.Opt[User]{UserCols.Email.Like("b%")}, []string{"bea@example.com", "bob@example.com"}},
		{"isnull", []rt.Opt[User]{UserCols.Bio.IsNull()}, []string{"bea@example.com", "bob@example.com"}},
		{"isnotnull", []rt.Opt[User]{UserCols.Bio.IsNotNull()}, []string{"ann@example.com"}},
		{"in", []rt.Opt[User]{UserCols.ID.In(ann.ID, bob.ID)}, []string{"ann@example.com", "bob@example.com"}},
		{"in empty", []rt.Opt[User]{UserCols.ID.In()}, nil},
		{"notin", []rt.Opt[User]{UserCols.ID.NotIn(bea.ID)}, []string{"ann@example.com", "bob@example.com"}},
		{"gt", []rt.Opt[User]{UserCols.ID.Gt(ann.ID)}, []string{"bea@example.com", "bob@example.com"}},
		{
			"two opts join with and",
			[]rt.Opt[User]{UserCols.Name.Eq("Bo"), UserCols.Bio.IsNull()},
			[]string{"bea@example.com", "bob@example.com"},
		},
		{
			"or with not",
			[]rt.Opt[User]{rt.Or(UserCols.Email.Eq("ann@example.com"), rt.Not(UserCols.Name.Eq("Bo")))},
			[]string{"ann@example.com"},
		},
		{"eqcol no match", []rt.Opt[User]{UserCols.Email.EqCol(UserCols.Name)}, nil},
		{"raw", []rt.Opt[User]{rt.Raw[User]("length(\"name\") = ?", 2)}, []string{"bea@example.com", "bob@example.com"}},
		{"empty pred means all", []rt.Opt[User]{rt.Pred[User]{}}, []string{"ann@example.com", "bea@example.com", "bob@example.com"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := q.UserQuery(ctx, tc.opts...)
			if err != nil {
				t.Fatalf("UserQuery: %v", err)
			}
			if gotEmails := emails(got); !equalStrings(gotEmails, tc.want) {
				t.Errorf("UserQuery = %v, want %v", gotEmails, tc.want)
			}
		})
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestUserQueryShaping(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	dynSeed(t, q)

	got, err := q.UserQuery(ctx, UserOrderBy(UserCols.Email.Desc()), UserLimit(2))
	if err != nil {
		t.Fatalf("UserQuery: %v", err)
	}
	if want := []string{"bob@example.com", "bea@example.com"}; !equalStrings(emails(got), want) {
		t.Errorf("desc+limit = %v, want %v", emails(got), want)
	}

	got, err = q.UserQuery(ctx, UserOrderBy(UserCols.Email.Asc()), UserLimit(2), UserOffset(2))
	if err != nil {
		t.Fatalf("UserQuery: %v", err)
	}
	if want := []string{"bob@example.com"}; !equalStrings(emails(got), want) {
		t.Errorf("offset page = %v, want %v", emails(got), want)
	}
}

// TestUserQueryKeyset walks the whole table one row at a time via After
// (D34), over a deliberately non-unique first order term with a mixed-
// direction unique tiebreak — the case OFFSET pagination gets wrong when
// rows shift between pages.
func TestUserQueryKeyset(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	dynSeed(t, q)

	var walked []string
	var opts []rt.Opt[User]
	base := []rt.Opt[User]{UserOrderBy(UserCols.Name.Asc(), UserCols.ID.Desc()), UserLimit(1)}
	opts = base
	for i := 0; i < 10; i++ {
		page, err := q.UserQuery(ctx, opts...)
		if err != nil {
			t.Fatalf("page %d: %v", i, err)
		}
		if len(page) == 0 {
			break
		}
		last := page[len(page)-1]
		walked = append(walked, last.Email)
		opts = append(append([]rt.Opt[User]{}, base...), UserAfter(last.Name, last.ID))
	}
	// name asc gives Ann first; the two "Bo" rows come id-descending
	if want := []string{"ann@example.com", "bob@example.com", "bea@example.com"}; !equalStrings(walked, want) {
		t.Errorf("keyset walk = %v, want %v", walked, want)
	}

	// misuse fails loudly, not wrongly
	if _, err := q.UserQuery(ctx, UserAfter("x")); err == nil || !strings.Contains(err.Error(), "OrderBy") {
		t.Errorf("After without OrderBy: err = %v", err)
	}
}

func TestUserCountExists(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	dynSeed(t, q)

	if n, err := q.UserCount(ctx); err != nil || n != 3 {
		t.Errorf("bare UserCount = %d, %v; want 3", n, err)
	}
	if n, err := q.UserCount(ctx, UserCols.Name.Eq("Bo")); err != nil || n != 2 {
		t.Errorf("filtered UserCount = %d, %v; want 2", n, err)
	}
	if ok, err := q.UserExists(ctx, UserCols.Bio.IsNotNull()); err != nil || !ok {
		t.Errorf("UserExists(bio set) = %v, %v; want true", ok, err)
	}
	if ok, err := q.UserExists(ctx, UserCols.Email.Eq("nobody@example.com")); err != nil || ok {
		t.Errorf("UserExists(missing) = %v, %v; want false", ok, err)
	}
}

func TestUserUpdateWhere(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	ann, _, _ := dynSeed(t, q)

	// partial update: only bio moves, everything else stays
	n, err := q.UserUpdateWhere(ctx, UserSet(UserCols.Bio.Set("beekeeper")), UserCols.ID.Eq(ann.ID))
	if err != nil || n != 1 {
		t.Fatalf("UserUpdateWhere = %d, %v; want 1 row", n, err)
	}
	got, err := q.UserGet(ctx, ann.ID)
	if err != nil || got.Bio.Or("") != "beekeeper" || got.Email != ann.Email {
		t.Errorf("after update: %+v, %v", got, err)
	}

	// SetNull is the explicit way back to NULL
	if n, err := q.UserUpdateWhere(ctx, UserSet(UserCols.Bio.SetNull()), UserCols.ID.Eq(ann.ID)); err != nil || n != 1 {
		t.Fatalf("SetNull = %d, %v", n, err)
	}
	if got, _ := q.UserGet(ctx, ann.ID); got.Bio.Valid {
		t.Errorf("bio still set after SetNull: %+v", got.Bio)
	}

	// predicates that match nothing affect nothing, without error
	if n, err := q.UserUpdateWhere(ctx, UserSet(UserCols.Name.Set("x")), UserCols.ID.Eq(99)); err != nil || n != 0 {
		t.Errorf("miss = %d, %v; want 0, nil", n, err)
	}

	// the whole-table guard (D42)
	if _, err := q.UserUpdateWhere(ctx, UserSet(UserCols.Name.Set("x"))); err == nil {
		t.Error("UserUpdateWhere without predicate: want error")
	}
	// ... and the explicit way to mean it
	if n, err := q.UserUpdateWhere(ctx, UserSet(UserCols.Name.Set("Everyone")), rt.Raw[User]("1 = 1")); err != nil || n != 3 {
		t.Errorf("explicit all-rows update = %d, %v; want 3", n, err)
	}
}

func TestUserDeleteWhere(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	dynSeed(t, q)

	if _, err := q.UserDeleteWhere(ctx); err == nil {
		t.Error("UserDeleteWhere without predicate: want error")
	}
	n, err := q.UserDeleteWhere(ctx, UserCols.Name.Eq("Bo"))
	if err != nil || n != 2 {
		t.Fatalf("UserDeleteWhere = %d, %v; want 2", n, err)
	}
	if left, err := q.UserCount(ctx); err != nil || left != 1 {
		t.Errorf("rows left = %d, %v; want 1", left, err)
	}
}

// TestOrderDynamic exercises the enum-typed handle and a second model's
// surface: a Pred[Order] is a different type from a Pred[User], so
// cross-model mixups never reach the database (D29 — a compile error).
func TestOrderDynamic(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	ann, _, _ := dynSeed(t, q)

	placed, err := q.OrderCreate(ctx, OrderCreateParams{UserID: ann.ID, Total: "9.99", PlacedAt: rt.Some(time.Now().UTC())})
	if err != nil {
		t.Fatalf("OrderCreate: %v", err)
	}
	unplaced, err := q.OrderCreate(ctx, OrderCreateParams{UserID: ann.ID, Total: "15.00", PlacedAt: rt.Null[time.Time]{}})
	if err != nil {
		t.Fatalf("OrderCreate: %v", err)
	}

	// both got the database default status (D16); the enum handle is typed
	if n, err := q.OrderCount(ctx, OrderCols.Status.Eq(OrderStatusPending)); err != nil || n != 2 {
		t.Errorf("pending count = %d, %v; want 2", n, err)
	}
	if n, err := q.OrderUpdateWhere(ctx, OrderSet(OrderCols.Status.Set(OrderStatusShipped)), OrderCols.ID.Eq(placed.ID)); err != nil || n != 1 {
		t.Fatalf("ship = %d, %v", n, err)
	}
	shipped, err := q.OrderQuery(ctx, OrderCols.Status.Eq(OrderStatusShipped))
	if err != nil || len(shipped) != 1 || shipped[0].ID != placed.ID {
		t.Errorf("shipped = %+v, %v; want just order %d", shipped, err, placed.ID)
	}
	if never, err := q.OrderQuery(ctx, OrderCols.PlacedAt.IsNull()); err != nil || len(never) != 1 || never[0].ID != unplaced.ID {
		t.Errorf("unplaced = %+v, %v; want just order %d", never, err, unplaced.ID)
	}
}

// TestDynStmtCache proves the D31 wiring end to end: same option shape,
// different values -> one prepared statement; a new shape -> a second.
func TestDynStmtCache(t *testing.T) {
	ctx := context.Background()
	db, q := newDB(t)
	dynSeed(t, q)

	cache := rt.NewStmtCache(8)
	defer cache.Close()
	qc := New(db).WithCache(cache)

	for _, name := range []string{"Ann", "Bo", "Nobody"} {
		if _, err := qc.UserQuery(ctx, UserCols.Name.Eq(name), UserLimit(5)); err != nil {
			t.Fatalf("cached query (%s): %v", name, err)
		}
	}
	if n := cache.Len(); n != 1 {
		t.Errorf("cache holds %d statements after 3 same-shape queries; want 1 — values must ride as parameters", n)
	}
	if _, err := qc.UserQuery(ctx, UserCols.Name.Eq("Ann"), UserCols.Bio.IsNull(), UserLimit(5)); err != nil {
		t.Fatal(err)
	}
	if n := cache.Len(); n != 2 {
		t.Errorf("cache holds %d statements after a second shape; want 2", n)
	}
}

// TestDynInTx runs dynamic verbs inside Queries.Tx: the transactional
// copy drops the statement cache by construction (D31 — its statements
// belong to the outer handle) and a rollback undoes the writes.
func TestDynInTx(t *testing.T) {
	ctx := context.Background()
	db, _ := newDB(t)
	cache := rt.NewStmtCache(8)
	defer cache.Close()
	q := New(db).WithCache(cache)
	dynSeed(t, q)

	failed := errTxAbort
	err := q.Tx(ctx, func(qt *Queries) error {
		if n, err := qt.UserDeleteWhere(ctx, UserCols.Name.Eq("Bo")); err != nil || n != 2 {
			t.Errorf("in-tx delete = %d, %v; want 2", n, err)
		}
		return failed
	})
	if err != failed {
		t.Fatalf("Tx err = %v, want the abort sentinel", err)
	}
	if n, err := q.UserCount(ctx); err != nil || n != 3 {
		t.Errorf("after rollback count = %d, %v; want 3", n, err)
	}

	if err := q.Tx(ctx, func(qt *Queries) error {
		_, err := qt.UserDeleteWhere(ctx, UserCols.Name.Eq("Bo"))
		return err
	}); err != nil {
		t.Fatalf("committing tx: %v", err)
	}
	if n, err := q.UserCount(ctx); err != nil || n != 1 {
		t.Errorf("after commit count = %d, %v; want 1", n, err)
	}
}
