// Integration tests (decision D25): the generated CRUD runs against a
// real SQLite through a real database/sql driver — no mocks, no canned
// rows. The driver is a test-only dependency; generated code and the rt
// runtime stay driver-free (D03).
package itest

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Piechutowski/not-an-orm/rt"
	_ "github.com/mattn/go-sqlite3"
)

func newDB(t *testing.T) (*sql.DB, *Queries) {
	t.Helper()
	db, err := rt.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	ddl, err := os.ReadFile("nao_schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(string(ddl)); err != nil {
		t.Fatalf("generated DDL rejected by SQLite: %v", err)
	}
	return db, New(db)
}

func seedUser(t *testing.T, q *Queries, email string) User {
	t.Helper()
	u, err := q.UserCreate(context.Background(), UserCreateParams{Email: email, Name: "Ann"})
	if err != nil {
		t.Fatalf("UserCreate: %v", err)
	}
	return u
}

func TestUserCRUD(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)

	// Create: the auto-increment key and the defaulted created_at are the
	// database's job; RETURNING brings both back (D16).
	u, err := q.UserCreate(ctx, UserCreateParams{Email: "ann@example.com", Name: "Ann"})
	if err != nil {
		t.Fatalf("UserCreate: %v", err)
	}
	if u.ID != 1 || u.CreatedAt.IsZero() {
		t.Errorf("UserCreate returned %+v; want ID=1 and a database-filled created_at", u)
	}
	if u.Bio.Valid {
		t.Errorf("bio should be NULL, got %+v", u.Bio)
	}

	got, err := q.UserGet(ctx, u.ID)
	if err != nil || got.Email != "ann@example.com" {
		t.Errorf("UserGet = %+v, %v", got, err)
	}
	byEmail, err := q.UserGetByEmail(ctx, "ann@example.com")
	if err != nil || byEmail.ID != u.ID {
		t.Errorf("UserGetByEmail = %+v, %v", byEmail, err)
	}

	u2 := seedUser(t, q, "bob@example.com")
	many, err := q.UserGetMany(ctx, []int32{u.ID, u2.ID, 999})
	if err != nil || len(many) != 2 {
		t.Errorf("UserGetMany = %d rows, %v; want 2 (missing keys are not errors)", len(many), err)
	}
	if none, err := q.UserGetMany(ctx, nil); err != nil || none != nil {
		t.Errorf("UserGetMany(nil) = %v, %v; want nil, nil", none, err)
	}

	all, err := q.UserList(ctx)
	if err != nil || len(all) != 2 {
		t.Errorf("UserList = %d rows, %v", len(all), err)
	}
	if n, err := q.UserCount(ctx); err != nil || n != 2 {
		t.Errorf("UserCount = %d, %v", n, err)
	}

	// Update rewrites every non-key column and returns the row (D15/D16).
	upd, err := q.UserUpdate(ctx, u.ID, UserUpdateParams{
		Email:     u.Email,
		Name:      "Ann B.",
		Bio:       rt.Some("hi there"),
		CreatedAt: u.CreatedAt,
	})
	if err != nil {
		t.Fatalf("UserUpdate: %v", err)
	}
	if upd.Name != "Ann B." || !upd.Bio.Valid || upd.Bio.V != "hi there" {
		t.Errorf("UserUpdate returned %+v", upd)
	}

	// Identity misses are rt.ErrNotFound, which is also sql.ErrNoRows.
	if _, err := q.UserUpdate(ctx, 999, UserUpdateParams{Email: "x@x", Name: "x", CreatedAt: u.CreatedAt}); !errors.Is(err, rt.ErrNotFound) {
		t.Errorf("UserUpdate(999) = %v; want rt.ErrNotFound", err)
	}
	if _, err := q.UserGet(ctx, 999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("UserGet(999) = %v; want sql.ErrNoRows compatibility", err)
	}

	if err := q.UserDelete(ctx, u2.ID); err != nil {
		t.Errorf("UserDelete: %v", err)
	}
	if err := q.UserDelete(ctx, u2.ID); !errors.Is(err, rt.ErrNotFound) {
		t.Errorf("second UserDelete = %v; want rt.ErrNotFound", err)
	}

	// unique column: duplicate email must be rejected by the DDL
	if _, err := q.UserCreate(ctx, UserCreateParams{Email: "ann@example.com", Name: "Imposter"}); err == nil {
		t.Error("duplicate email accepted; UNIQUE constraint missing")
	}
}

func TestOrderDefaultsEnumsAndFKs(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	u := seedUser(t, q, "ann@example.com")

	o, err := q.OrderCreate(ctx, OrderCreateParams{UserID: u.ID, Total: "19.99"})
	if err != nil {
		t.Fatalf("OrderCreate: %v", err)
	}
	// the defaulted enum column comes back typed (D11/D16)
	if o.Status != EOrderStatusPending {
		t.Errorf("status = %q; want the database default %q", o.Status, EOrderStatusPending)
	}
	if o.PlacedAt.Valid {
		t.Errorf("placed_at should be NULL, got %+v", o.PlacedAt)
	}
	if o.Total != "19.99" {
		t.Errorf("total = %q; decimals ride strings, never floats", o.Total)
	}

	placed := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)
	upd, err := q.OrderUpdate(ctx, o.ID, OrderUpdateParams{
		UserID:   o.UserID,
		Status:   EOrderStatusShipped,
		Total:    o.Total,
		PlacedAt: rt.Some(placed),
	})
	if err != nil {
		t.Fatalf("OrderUpdate: %v", err)
	}
	if upd.Status != EOrderStatusShipped || !upd.PlacedAt.Valid || !upd.PlacedAt.V.Equal(placed) {
		t.Errorf("OrderUpdate returned %+v", upd)
	}

	// enum CHECK and FK are real constraints in the generated DDL
	if _, err := q.OrderCreate(ctx, OrderCreateParams{UserID: 999, Total: "1.00"}); err == nil || !strings.Contains(err.Error(), "FOREIGN KEY") {
		t.Errorf("dangling user_id = %v; want a FOREIGN KEY violation", err)
	}
	if _, err := q.OrderUpdate(ctx, o.ID, OrderUpdateParams{UserID: o.UserID, Status: "teleported", Total: o.Total}); err == nil {
		t.Error("invalid enum value accepted; CHECK constraint missing")
	}
}

func TestUserTagCompositePK(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	u := seedUser(t, q, "ann@example.com")

	tag, err := q.UserTagCreate(ctx, UserTagCreateParams{UserID: u.ID, Tag: "vip"})
	if err != nil {
		t.Fatalf("UserTagCreate: %v", err)
	}
	// composite identity: one positional argument per key column (D15)
	got, err := q.UserTagGet(ctx, tag.UserID, tag.Tag)
	if err != nil || got != tag {
		t.Errorf("UserTagGet = %+v, %v", got, err)
	}
	if _, err := q.UserTagCreate(ctx, UserTagCreateParams{UserID: u.ID, Tag: "vip"}); err == nil {
		t.Error("duplicate composite key accepted")
	}
	if err := q.UserTagDelete(ctx, tag.UserID, tag.Tag); err != nil {
		t.Errorf("UserTagDelete: %v", err)
	}
	if _, err := q.UserTagGet(ctx, tag.UserID, tag.Tag); !errors.Is(err, rt.ErrNotFound) {
		t.Errorf("UserTagGet after delete = %v; want rt.ErrNotFound", err)
	}
}

func TestTx(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)

	// commit: work inside fn is visible after
	err := q.Tx(ctx, func(q *Queries) error {
		_, err := q.UserCreate(ctx, UserCreateParams{Email: "tx@example.com", Name: "Tx"})
		return err
	})
	if err != nil {
		t.Fatalf("Tx: %v", err)
	}
	if n, _ := q.UserCount(ctx); n != 1 {
		t.Errorf("after committed Tx, count = %d; want 1", n)
	}

	// rollback: a returned error undoes everything
	sentinel := errors.New("boom")
	err = q.Tx(ctx, func(q *Queries) error {
		if _, err := q.UserCreate(ctx, UserCreateParams{Email: "gone@example.com", Name: "Gone"}); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("Tx = %v; want the fn error back", err)
	}
	if n, _ := q.UserCount(ctx); n != 1 {
		t.Errorf("after rolled-back Tx, count = %d; want 1", n)
	}

	// SQLite has no nested transactions; a tx-backed Queries says so
	err = q.Tx(ctx, func(inner *Queries) error {
		return inner.Tx(ctx, func(*Queries) error { return nil })
	})
	if err == nil {
		t.Error("nested Tx unexpectedly succeeded")
	}
}

// TestJSONNull pins the D13 wire format: nullable columns marshal as the
// value or null — the JSON never shows a wrapper and never omits the key.
func TestJSONNull(t *testing.T) {
	ctx := context.Background()
	_, q := newDB(t)
	u := seedUser(t, q, "ann@example.com")

	doc, err := json.Marshal(u)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"bio":null`) {
		t.Errorf("NULL bio marshals as %s; want \"bio\":null", doc)
	}

	upd, err := q.UserUpdate(ctx, u.ID, UserUpdateParams{Email: u.Email, Name: u.Name, Bio: rt.Some("hi"), CreatedAt: u.CreatedAt})
	if err != nil {
		t.Fatal(err)
	}
	doc, err = json.Marshal(upd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), `"bio":"hi"`) {
		t.Errorf("bio marshals as %s; want \"bio\":\"hi\"", doc)
	}

	var back User
	if err := json.Unmarshal(doc, &back); err != nil {
		t.Fatal(err)
	}
	if !back.Bio.Valid || back.Bio.V != "hi" {
		t.Errorf("round-tripped bio = %+v", back.Bio)
	}
}

func TestStmtCache(t *testing.T) {
	ctx := context.Background()
	db, q := newDB(t)
	seedUser(t, q, "ann@example.com")

	cache := rt.NewStmtCache(2)
	defer cache.Close()

	s1, err := cache.Prepare(ctx, db, userGetSQL)
	if err != nil {
		t.Fatal(err)
	}
	var u User
	if err := s1.QueryRowContext(ctx, sql.Named("id", 1)).Scan(&u.ID, &u.Email, &u.Name, &u.Bio, &u.CreatedAt); err != nil {
		t.Fatalf("cached statement: %v", err)
	}
	again, err := cache.Prepare(ctx, db, userGetSQL)
	if err != nil || again != s1 {
		t.Errorf("second Prepare returned a different statement (%p vs %p, %v); the SQL text is the cache key (D31)", again, s1, err)
	}

	// LRU eviction at capacity 2
	if _, err := cache.Prepare(ctx, db, userListSQL); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.Prepare(ctx, db, userDeleteSQL); err != nil {
		t.Fatal(err)
	}
	if n := cache.Len(); n != 2 {
		t.Errorf("cache holds %d statements; want 2 after eviction", n)
	}

	if err := cache.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if _, err := cache.Prepare(ctx, db, userGetSQL); err == nil {
		t.Error("Prepare on a closed cache unexpectedly succeeded")
	}
}

func TestOpenPragmas(t *testing.T) {
	db, err := rt.Open("sqlite3", filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var fk int
	if err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, %v; want 1", fk, err)
	}
	var mode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil || !strings.EqualFold(mode, "wal") {
		t.Errorf("journal_mode = %q, %v; want wal", mode, err)
	}
	var timeout int
	if err := db.QueryRow("PRAGMA busy_timeout").Scan(&timeout); err != nil || timeout != 5000 {
		t.Errorf("busy_timeout = %d, %v; want 5000", timeout, err)
	}
}
