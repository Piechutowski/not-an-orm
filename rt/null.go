package rt

import (
	"bytes"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
)

// Null is the representation of a nullable column (decision D13): a value
// plus a validity bit, never a pointer. The zero value is NULL. JSON shows
// the value or null — the wire format never sees the wrapper.
//
//	var n rt.Null[string]        // NULL
//	n = rt.Some("x")             // 'x'
//	if n.Valid { use(n.V) }
type Null[T any] struct {
	V     T
	Valid bool
}

// Some wraps a value in a valid Null.
func Some[T any](v T) Null[T] {
	return Null[T]{V: v, Valid: true}
}

// Get returns the value and whether it is present.
func (n Null[T]) Get() (T, bool) { return n.V, n.Valid }

// Or returns the value, or def when NULL.
func (n Null[T]) Or(def T) T {
	if n.Valid {
		return n.V
	}
	return def
}

// Scan implements sql.Scanner. Conversion from the driver value is
// delegated to database/sql's own rules via sql.Null.
func (n *Null[T]) Scan(src any) error {
	var s sql.Null[T]
	if err := s.Scan(src); err != nil {
		return err
	}
	n.V, n.Valid = s.V, s.Valid
	return nil
}

// Value implements driver.Valuer: the value, or nil for NULL.
func (n Null[T]) Value() (driver.Value, error) {
	return sql.Null[T]{V: n.V, Valid: n.Valid}.Value()
}

var jsonNull = []byte("null")

// MarshalJSON writes the value, or null.
func (n Null[T]) MarshalJSON() ([]byte, error) {
	if !n.Valid {
		return jsonNull, nil
	}
	return json.Marshal(n.V)
}

// UnmarshalJSON reads null as NULL and anything else as the value.
func (n *Null[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), jsonNull) {
		*n = Null[T]{}
		return nil
	}
	if err := json.Unmarshal(data, &n.V); err != nil {
		return err
	}
	n.Valid = true
	return nil
}
