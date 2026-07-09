package rt

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNullScan(t *testing.T) {
	var s Null[string]
	if err := s.Scan("hi"); err != nil || !s.Valid || s.V != "hi" {
		t.Errorf("Scan(hi) = %+v, %v", s, err)
	}
	if err := s.Scan(nil); err != nil || s.Valid {
		t.Errorf("Scan(nil) = %+v, %v", s, err)
	}

	var i Null[int64]
	if err := i.Scan(int64(42)); err != nil || !i.Valid || i.V != 42 {
		t.Errorf("Scan(42) = %+v, %v", i, err)
	}

	// driver bytes into a string-kinded named type, as enum columns scan
	type status string
	var st Null[status]
	if err := st.Scan([]byte("open")); err != nil || st.V != "open" {
		t.Errorf("Scan([]byte) = %+v, %v", st, err)
	}

	var ts Null[time.Time]
	now := time.Now()
	if err := ts.Scan(now); err != nil || !ts.V.Equal(now) {
		t.Errorf("Scan(time) = %+v, %v", ts, err)
	}
}

func TestNullValue(t *testing.T) {
	v, err := Some("x").Value()
	if err != nil || v != "x" {
		t.Errorf("Some(x).Value() = %v, %v", v, err)
	}
	v, err = Null[string]{}.Value()
	if err != nil || v != nil {
		t.Errorf("zero.Value() = %v, %v", v, err)
	}
}

func TestNullJSON(t *testing.T) {
	type row struct {
		Name  Null[string] `json:"name"`
		Score Null[int]    `json:"score"`
	}
	got, err := json.Marshal(row{Name: Some("ann")})
	if err != nil {
		t.Fatal(err)
	}
	// D13: the wire format is the value or null, never a wrapper object
	if want := `{"name":"ann","score":null}`; string(got) != want {
		t.Errorf("Marshal = %s, want %s", got, want)
	}

	var back row
	if err := json.Unmarshal([]byte(`{"name":null,"score":7}`), &back); err != nil {
		t.Fatal(err)
	}
	if back.Name.Valid || !back.Score.Valid || back.Score.V != 7 {
		t.Errorf("Unmarshal = %+v", back)
	}
}

func TestNullHelpers(t *testing.T) {
	if got := (Null[int]{}).Or(9); got != 9 {
		t.Errorf("zero.Or(9) = %d", got)
	}
	if got := Some(3).Or(9); got != 3 {
		t.Errorf("Some(3).Or(9) = %d", got)
	}
	if v, ok := Some("a").Get(); !ok || v != "a" {
		t.Errorf("Some(a).Get() = %q, %v", v, ok)
	}
}
