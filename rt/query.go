// The dynamic query layer (decisions D28-D34): predicates, orderings,
// limits and assignments as inert data — plain values forming a small
// expression tree, never closures and never a mutable builder. Generated
// terminal methods (UserQuery, UserCount, ...) hand the values to the
// interpreter here, which walks the tree once and renders SQL text and
// bound arguments in lockstep. The interpreter is deterministic:
// identical trees render identical SQL, which is what makes the rendered
// text a statement-cache key (D31).
//
// The layer only filters, orders and limits an existing row shape (the
// shape rule, D32). Anything that changes what a row is — joins,
// aggregates, projections — happens at generation time, where result
// structs can be minted.
package rt

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

/* ===== typed column handles (D29) ===== */

// Column is the typed handle of one column of model M: its methods build
// the Pred, Order and Assign values of the dynamic query layer. M is a
// phantom type — no value of it is ever held; it exists so a predicate
// over one model cannot enter another model's query (a compile error,
// D29). T is the column's Go value type; operator arguments are T, so a
// value of the wrong type is a compile error too.
//
// Handles are emitted by the generator into per-model sets
// (UserCols.Email); the operators live here, once (D29).
type Column[M, T any] struct {
	// Name is the SQL column name, unquoted.
	Name string
}

// NullColumn is the handle of a nullable column. Comparisons take plain
// present values (T, not Null[T]): SQL comparison with NULL matches
// nothing, so NULL is handled by its own explicit operators —
// IsNull/IsNotNull to test, SetNull to assign.
type NullColumn[M, T any] struct {
	Column[M, T]
}

func (c Column[M, T]) cmp(op string, v T) Pred[M] {
	return Pred[M]{kind: predCmp, col: c.Name, op: op, vals: []any{v}}
}

// Eq is "column = v". NULL never matches (SQL three-valued logic); test
// NULL with IsNull.
func (c Column[M, T]) Eq(v T) Pred[M] { return c.cmp("=", v) }

// Ne is "column <> v". Rows where the column is NULL never match.
func (c Column[M, T]) Ne(v T) Pred[M] { return c.cmp("<>", v) }

// Gt is "column > v".
func (c Column[M, T]) Gt(v T) Pred[M] { return c.cmp(">", v) }

// Ge is "column >= v".
func (c Column[M, T]) Ge(v T) Pred[M] { return c.cmp(">=", v) }

// Lt is "column < v".
func (c Column[M, T]) Lt(v T) Pred[M] { return c.cmp("<", v) }

// Le is "column <= v".
func (c Column[M, T]) Le(v T) Pred[M] { return c.cmp("<=", v) }

// In is "column IN (vs...)". With no values it renders constant false:
// membership in the empty set holds for no row.
func (c Column[M, T]) In(vs ...T) Pred[M] {
	return Pred[M]{kind: predIn, col: c.Name, vals: valsAny(vs)}
}

// NotIn is "column NOT IN (vs...)". With no values it renders constant
// true. Rows where the column is NULL never match a non-empty NOT IN.
func (c Column[M, T]) NotIn(vs ...T) Pred[M] {
	return Pred[M]{kind: predIn, col: c.Name, vals: valsAny(vs), not: true}
}

// Like is "column LIKE pattern" (SQLite LIKE: case-insensitive ASCII,
// '%' and '_' wildcards).
func (c Column[M, T]) Like(pattern string) Pred[M] {
	return Pred[M]{kind: predCmp, col: c.Name, op: "LIKE", vals: []any{pattern}}
}

// NotLike is "column NOT LIKE pattern".
func (c Column[M, T]) NotLike(pattern string) Pred[M] {
	return Pred[M]{kind: predCmp, col: c.Name, op: "NOT LIKE", vals: []any{pattern}}
}

// IsNull is "column IS NULL".
func (c Column[M, T]) IsNull() Pred[M] {
	return Pred[M]{kind: predNull, col: c.Name}
}

// IsNotNull is "column IS NOT NULL".
func (c Column[M, T]) IsNotNull() Pred[M] {
	return Pred[M]{kind: predNull, col: c.Name, not: true}
}

func (c Column[M, T]) cmpCol(op string, o Column[M, T]) Pred[M] {
	return Pred[M]{kind: predColCmp, col: c.Name, op: op, col2: o.Name}
}

// EqCol compares two columns of the same model: "a = b" (same-shape
// column comparison is runtime-builder material, D32).
func (c Column[M, T]) EqCol(o Column[M, T]) Pred[M] { return c.cmpCol("=", o) }

// NeCol is "a <> b".
func (c Column[M, T]) NeCol(o Column[M, T]) Pred[M] { return c.cmpCol("<>", o) }

// GtCol is "a > b".
func (c Column[M, T]) GtCol(o Column[M, T]) Pred[M] { return c.cmpCol(">", o) }

// GeCol is "a >= b".
func (c Column[M, T]) GeCol(o Column[M, T]) Pred[M] { return c.cmpCol(">=", o) }

// LtCol is "a < b".
func (c Column[M, T]) LtCol(o Column[M, T]) Pred[M] { return c.cmpCol("<", o) }

// LeCol is "a <= b".
func (c Column[M, T]) LeCol(o Column[M, T]) Pred[M] { return c.cmpCol("<=", o) }

// Asc orders by this column, ascending.
func (c Column[M, T]) Asc() Order[M] { return Order[M]{col: c.Name} }

// Desc orders by this column, descending.
func (c Column[M, T]) Desc() Order[M] { return Order[M]{col: c.Name, desc: true} }

// Set assigns v to this column in an UpdateWhere.
func (c Column[M, T]) Set(v T) Assign[M] { return Assign[M]{col: c.Name, val: v} }

// SetNull assigns NULL to this column in an UpdateWhere.
func (c NullColumn[M, T]) SetNull() Assign[M] { return Assign[M]{col: c.Name} }

func valsAny[T any](vs []T) []any {
	if len(vs) == 0 {
		return nil
	}
	out := make([]any, len(vs))
	for i, v := range vs {
		out[i] = v
	}
	return out
}

/* ===== predicates as inert data (D28) ===== */

// Pred is one predicate over model M: a small immutable expression tree.
// Predicates compose (And/Or/Not), store in variables, append
// conditionally, and are shared across verbs — the same value drives
// Query, Count, Exists, DeleteWhere and UpdateWhere (D32).
//
// The zero value is the empty predicate: it filters nothing and vanishes
// from And, Or, Not and WHERE clauses, so conditional query building
// needs no special cases:
//
//	var p rt.Pred[User]
//	if search != "" {
//		p = UserCols.Name.Like("%" + search + "%")
//	}
//	users, err := q.UserQuery(ctx, p)
type Pred[M any] struct {
	kind predKind
	col  string    // leaf predicates: the left-hand column
	op   string    // predCmp / predColCmp: the SQL comparator
	col2 string    // predColCmp: the right-hand column
	vals []any     // bound values, in placeholder order
	not  bool      // predIn: NOT IN; predNull: IS NOT NULL
	raw  string    // predRaw: verbatim SQL
	kids []Pred[M] // predAnd, predOr, predNot
}

type predKind uint8

const (
	predEmpty predKind = iota
	predCmp
	predColCmp
	predIn
	predNull
	predRaw
	predAnd
	predOr
	predNot
)

// And is the conjunction of the given predicates. Empty predicates are
// dropped; no (surviving) operands is the empty predicate, one is that
// operand unchanged.
func And[M any](ps ...Pred[M]) Pred[M] { return predJoin(predAnd, ps) }

// Or is the disjunction of the given predicates, with the same empty-
// predicate normalization as And.
func Or[M any](ps ...Pred[M]) Pred[M] { return predJoin(predOr, ps) }

// Not negates a predicate. Not of the empty predicate is empty.
func Not[M any](p Pred[M]) Pred[M] {
	if p.kind == predEmpty {
		return p
	}
	return Pred[M]{kind: predNot, kids: []Pred[M]{p}}
}

// Raw is the last-resort escape hatch: verbatim SQL as a predicate, with
// '?' placeholders bound to args. It is outside the safety net (D18) —
// nothing validates the fragment until SQLite prepares the statement —
// and it is the one deliberate hole in the typed layer. Column and
// parameter hygiene are the caller's problem here; prefer promoting the
// condition to a Select block in the schema.
func Raw[M any](sql string, args ...any) Pred[M] {
	if sql == "" {
		return Pred[M]{}
	}
	return Pred[M]{kind: predRaw, raw: sql, vals: args}
}

func predJoin[M any](kind predKind, ps []Pred[M]) Pred[M] {
	var kids []Pred[M]
	for _, p := range ps {
		if p.kind != predEmpty {
			kids = append(kids, p)
		}
	}
	switch len(kids) {
	case 0:
		return Pred[M]{}
	case 1:
		return kids[0]
	}
	return Pred[M]{kind: kind, kids: kids}
}

// render appends this predicate's SQL to b and its bound values to args,
// in lockstep. Composite operands are parenthesized, so operator
// precedence can never reassociate a tree.
func (p Pred[M]) render(b *strings.Builder, args *[]any) error {
	switch p.kind {
	case predCmp:
		b.WriteString(identQuote(p.col) + " " + p.op + " ?")
		*args = append(*args, p.vals[0])
	case predColCmp:
		b.WriteString(identQuote(p.col) + " " + p.op + " " + identQuote(p.col2))
	case predIn:
		if len(p.vals) == 0 {
			// membership in the empty set: constant false (true for NOT IN)
			if p.not {
				b.WriteString("1 = 1")
			} else {
				b.WriteString("1 = 0")
			}
			return nil
		}
		b.WriteString(identQuote(p.col))
		if p.not {
			b.WriteString(" NOT")
		}
		b.WriteString(" IN (?" + strings.Repeat(", ?", len(p.vals)-1) + ")")
		*args = append(*args, p.vals...)
	case predNull:
		b.WriteString(identQuote(p.col) + " IS")
		if p.not {
			b.WriteString(" NOT")
		}
		b.WriteString(" NULL")
	case predRaw:
		b.WriteString("(" + p.raw + ")")
		*args = append(*args, p.vals...)
	case predAnd, predOr:
		sep := " AND "
		if p.kind == predOr {
			sep = " OR "
		}
		b.WriteString("(")
		for i, k := range p.kids {
			if i > 0 {
				b.WriteString(sep)
			}
			if err := k.render(b, args); err != nil {
				return err
			}
		}
		b.WriteString(")")
	case predNot:
		b.WriteString("NOT (")
		if err := p.kids[0].render(b, args); err != nil {
			return err
		}
		b.WriteString(")")
	default:
		// unreachable through the public constructors: And/Or/Not normalize
		// empty operands away and the renderers skip empty top-level preds
		return errors.New("rt: cannot render an empty predicate")
	}
	return nil
}

/* ===== orderings and assignments ===== */

// Order is one ORDER BY term, built by Column.Asc/Desc.
type Order[M any] struct {
	col  string
	desc bool
}

// Assign is one "column = value" of an UpdateWhere, built by Column.Set
// and NullColumn.SetNull.
type Assign[M any] struct {
	col string
	val any
}

/* ===== options (D28, D30) ===== */

// Opt is one option of a dynamic query: a predicate (Pred is an Opt), an
// ordering, a limit, an offset, a keyset position or a distinct flag.
// Options are inert values; the generated terminal (UserQuery) hands
// them to the interpreter in one deterministic walk. Value-less options
// cannot infer M, so the generator emits per-model wrappers — UserLimit,
// UserOrderBy — over the constructors here (D30).
type Opt[M any] interface{ optApply(*querySpec[M]) }

type querySpec[M any] struct {
	preds     []Pred[M]
	orders    []Order[M]
	limit     int
	limitSet  bool
	offset    int
	offsetSet bool
	after     []any
	afterSet  bool
	distinct  bool
}

func (p Pred[M]) optApply(s *querySpec[M]) { s.preds = append(s.preds, p) }

type orderOpt[M any] struct{ terms []Order[M] }

func (o orderOpt[M]) optApply(s *querySpec[M]) { s.orders = append(s.orders, o.terms...) }

// OrderBy sorts the result by the given terms, in order. Repeated
// OrderBy options append.
func OrderBy[M any](terms ...Order[M]) Opt[M] { return orderOpt[M]{terms: terms} }

type limitOpt[M any] struct{ n int }

func (o limitOpt[M]) optApply(s *querySpec[M]) { s.limit, s.limitSet = o.n, true }

// Limit caps the number of rows returned; the last Limit wins. The value
// is bound as a parameter, so page size does not change the SQL text
// (one cached statement, D31).
func Limit[M any](n int) Opt[M] { return limitOpt[M]{n: n} }

type offsetOpt[M any] struct{ n int }

func (o offsetOpt[M]) optApply(s *querySpec[M]) { s.offset, s.offsetSet = o.n, true }

// Offset skips n rows; the last Offset wins. OFFSET degrades linearly
// with depth — keyset pagination (After) is the scalable page mechanism
// (D34).
func Offset[M any](n int) Opt[M] { return offsetOpt[M]{n: n} }

type afterOpt[M any] struct{ key []any }

func (o afterOpt[M]) optApply(s *querySpec[M]) { s.after, s.afterSet = o.key, true }

// After positions the query strictly after the row with the given key —
// keyset pagination (D34): pass the previous page's last row's values
// for the ORDER BY columns, one per term, in the same order. Rendering
// requires an OrderBy and one key value per term; for a total,
// gap-free order include a unique column as the final tiebreak term.
// NULLs in keyset columns are not paginated over (SQL comparisons skip
// them); keep keyset columns NOT NULL.
func After[M any](key ...any) Opt[M] { return afterOpt[M]{key: key} }

type distinctOpt[M any] struct{}

func (distinctOpt[M]) optApply(s *querySpec[M]) { s.distinct = true }

// Distinct deduplicates the result rows (SELECT DISTINCT).
func Distinct[M any]() Opt[M] { return distinctOpt[M]{} }

/* ===== the interpreter (D28, D31): options -> SQL + args ===== */

// SelectRender renders a SELECT over the model's fixed shape: table and
// columns come pre-rendered from generated code, everything else from
// the options. Deterministic: identical option values render identical
// SQL (D31). Argument order matches placeholder order by construction.
func SelectRender[M any](table, columns string, opts []Opt[M]) (string, []any, error) {
	var s querySpec[M]
	for _, o := range opts {
		if o != nil {
			o.optApply(&s)
		}
	}

	var b strings.Builder
	var args []any
	b.WriteString("SELECT ")
	if s.distinct {
		b.WriteString("DISTINCT ")
	}
	b.WriteString(columns + " FROM " + table)

	where, wargs, err := whereRender(s.preds)
	if err != nil {
		return "", nil, err
	}
	if s.afterSet {
		var w strings.Builder
		w.WriteString(where)
		if err := afterRender(&w, &wargs, s.orders, s.after); err != nil {
			return "", nil, err
		}
		where = w.String()
	}
	if where != "" {
		b.WriteString(" WHERE " + where)
		args = append(args, wargs...)
	}

	if len(s.orders) > 0 {
		b.WriteString(" ORDER BY ")
		for i, o := range s.orders {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(identQuote(o.col))
			if o.desc {
				b.WriteString(" DESC")
			}
		}
	}

	switch {
	case s.limitSet:
		b.WriteString(" LIMIT ?")
		args = append(args, s.limit)
		if s.offsetSet {
			b.WriteString(" OFFSET ?")
			args = append(args, s.offset)
		}
	case s.offsetSet:
		// SQLite has no bare OFFSET; LIMIT -1 means unlimited
		b.WriteString(" LIMIT -1 OFFSET ?")
		args = append(args, s.offset)
	}
	return b.String(), args, nil
}

// CountRender renders "SELECT count(*)" filtered by the predicates; none
// counts the whole table.
func CountRender[M any](table string, preds []Pred[M]) (string, []any, error) {
	where, args, err := whereRender(preds)
	if err != nil {
		return "", nil, err
	}
	q := "SELECT count(*) FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	return q, args, nil
}

// ExistsRender renders "SELECT EXISTS (...)" over the predicates.
func ExistsRender[M any](table string, preds []Pred[M]) (string, []any, error) {
	where, args, err := whereRender(preds)
	if err != nil {
		return "", nil, err
	}
	q := "SELECT EXISTS (SELECT 1 FROM " + table
	if where != "" {
		q += " WHERE " + where
	}
	return q + ")", args, nil
}

// DeleteRender renders a predicate-guarded DELETE. No effective
// predicate is an error, not a full-table delete: affecting every row
// must be written out loud (a trivially true Raw predicate).
func DeleteRender[M any](table string, preds []Pred[M]) (string, []any, error) {
	where, args, err := whereRender(preds)
	if err != nil {
		return "", nil, err
	}
	if where == "" {
		return "", nil, errors.New(`rt: DeleteWhere with no predicate would delete every row; say it out loud (e.g. rt.Raw("1 = 1")) or use the CRUD surface`)
	}
	return "DELETE FROM " + table + " WHERE " + where, args, nil
}

// UpdateRender renders a predicate-guarded partial UPDATE from typed
// assignments. No assignments, or no effective predicate, is an error —
// rewriting every row must be written out loud, like DeleteRender.
func UpdateRender[M any](table string, set []Assign[M], preds []Pred[M]) (string, []any, error) {
	if len(set) == 0 {
		return "", nil, errors.New("rt: UpdateWhere with no assignments has nothing to set")
	}
	var b strings.Builder
	var args []any
	b.WriteString("UPDATE " + table + " SET ")
	for i, a := range set {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(identQuote(a.col) + " = ?")
		args = append(args, a.val)
	}
	where, wargs, err := whereRender(preds)
	if err != nil {
		return "", nil, err
	}
	if where == "" {
		return "", nil, errors.New(`rt: UpdateWhere with no predicate would rewrite every row; say it out loud (e.g. rt.Raw("1 = 1"))`)
	}
	b.WriteString(" WHERE " + where)
	return b.String(), append(args, wargs...), nil
}

// whereRender renders the non-empty predicates joined by AND, without
// the leading WHERE keyword. An empty string means no predicate survived.
func whereRender[M any](preds []Pred[M]) (string, []any, error) {
	var w strings.Builder
	var args []any
	for _, p := range preds {
		if p.kind == predEmpty {
			continue
		}
		if w.Len() > 0 {
			w.WriteString(" AND ")
		}
		if err := p.render(&w, &args); err != nil {
			return "", nil, err
		}
	}
	return w.String(), args, nil
}

// afterRender appends the keyset predicate (D34): rows strictly after
// the key in the query's order, as the lexicographic expansion
//
//	(o1 > k1) OR (o1 = k1 AND o2 > k2) OR ...
//
// with the comparator flipped to < for DESC terms.
func afterRender[M any](w *strings.Builder, args *[]any, orders []Order[M], key []any) error {
	if len(orders) == 0 {
		return errors.New("rt: After needs an OrderBy: a keyset is a position in some order")
	}
	if len(key) != len(orders) {
		return fmt.Errorf("rt: After got %d key values for %d ORDER BY terms; pass the previous row's value for every term", len(key), len(orders))
	}
	if w.Len() > 0 {
		w.WriteString(" AND ")
	}
	if len(orders) > 1 {
		w.WriteString("(")
	}
	for i, o := range orders {
		if i > 0 {
			w.WriteString(" OR ")
		}
		w.WriteString("(")
		for j := 0; j < i; j++ {
			w.WriteString(identQuote(orders[j].col) + " = ? AND ")
			*args = append(*args, key[j])
		}
		cmp := " > ?"
		if o.desc {
			cmp = " < ?"
		}
		w.WriteString(identQuote(o.col) + cmp + ")")
		*args = append(*args, key[i])
	}
	if len(orders) > 1 {
		w.WriteString(")")
	}
	return nil
}

// identQuote double-quotes an identifier for SQLite, matching the
// generators' quoting so dynamic and static SQL cannot disagree.
func identQuote(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

/* ===== statement execution, cache-aware (D31) ===== */

// StmtQuery runs a row-returning statement on db, through cache when one
// is given (nil runs directly).
func StmtQuery(ctx context.Context, db DBTX, cache *StmtCache, query string, args ...any) (*sql.Rows, error) {
	if cache != nil {
		stmt, err := cache.Prepare(ctx, db, query)
		if err != nil {
			return nil, err
		}
		return stmt.QueryContext(ctx, args...)
	}
	return db.QueryContext(ctx, query, args...)
}

// StmtExec runs a non-row statement on db, through cache when one is
// given (nil runs directly).
func StmtExec(ctx context.Context, db DBTX, cache *StmtCache, query string, args ...any) (sql.Result, error) {
	if cache != nil {
		stmt, err := cache.Prepare(ctx, db, query)
		if err != nil {
			return nil, err
		}
		return stmt.ExecContext(ctx, args...)
	}
	return db.ExecContext(ctx, query, args...)
}

// RowScan runs a single-row statement and scans its columns into dest,
// through cache when one is given (nil runs directly).
func RowScan(ctx context.Context, db DBTX, cache *StmtCache, query string, args []any, dest ...any) error {
	if cache != nil {
		stmt, err := cache.Prepare(ctx, db, query)
		if err != nil {
			return err
		}
		return stmt.QueryRowContext(ctx, args...).Scan(dest...)
	}
	return db.QueryRowContext(ctx, query, args...).Scan(dest...)
}
