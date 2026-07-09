package rt

import (
	"container/list"
	"context"
	"database/sql"
	"errors"
	"sync"
)

// StmtCache is a prepared-statement cache keyed by rendered SQL text
// (decision D31). The dynamic-query interpreter renders identical SQL for
// identical option trees, so the text is a stable cache key; static
// generated SQL benefits the same way. Eviction is LRU; evicted and
// superseded statements are closed.
//
// A cache is bound to the handle its statements were prepared on: use one
// StmtCache per *sql.DB and do not feed it transaction handles (prepare
// on the DB and let database/sql re-prepare inside transactions).
type StmtCache struct {
	mu      sync.Mutex
	max     int
	order   *list.List               // front = most recently used; values are *cacheEntry
	entries map[string]*list.Element // sql text -> element
	closed  bool
}

type cacheEntry struct {
	sql  string
	stmt *sql.Stmt
}

// NewStmtCache returns a cache holding at most max prepared statements.
// max <= 0 means an unbounded cache.
func NewStmtCache(max int) *StmtCache {
	return &StmtCache{
		max:     max,
		order:   list.New(),
		entries: map[string]*list.Element{},
	}
}

// Prepare returns the cached statement for query, preparing and caching
// it on first use. The returned statement is shared: callers must not
// Close it (Close the cache instead).
func (c *StmtCache) Prepare(ctx context.Context, db DBTX, query string) (*sql.Stmt, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("rt.StmtCache: cache is closed")
	}
	if el, ok := c.entries[query]; ok {
		c.order.MoveToFront(el)
		stmt := el.Value.(*cacheEntry).stmt
		c.mu.Unlock()
		return stmt, nil
	}
	c.mu.Unlock()

	// Prepare outside the lock: preparing may hit the database.
	stmt, err := db.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		_ = stmt.Close()
		return nil, errors.New("rt.StmtCache: cache is closed")
	}
	if el, ok := c.entries[query]; ok {
		// lost a race with another goroutine; keep the incumbent
		_ = stmt.Close()
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).stmt, nil
	}
	c.entries[query] = c.order.PushFront(&cacheEntry{sql: query, stmt: stmt})
	if c.max > 0 && c.order.Len() > c.max {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		e := oldest.Value.(*cacheEntry)
		delete(c.entries, e.sql)
		_ = e.stmt.Close()
	}
	return stmt, nil
}

// Len reports the number of cached statements.
func (c *StmtCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// Close closes every cached statement and rejects further use.
func (c *StmtCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	var errs []error
	for el := c.order.Front(); el != nil; el = el.Next() {
		if err := el.Value.(*cacheEntry).stmt.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	c.order.Init()
	c.entries = map[string]*list.Element{}
	return errors.Join(errs...)
}
