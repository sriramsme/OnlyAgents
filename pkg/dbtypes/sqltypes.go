// Package dbtypes provides Go types that implement database/sql/driver
// interfaces for storage-driver-agnostic use. It is intentionally a
// zero-internal-dependency package so that any other package may import it
// without risk of import cycles.
package dbtypes

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"
)

// ── DBTime / NullDBTime ───────────────────────────────────────────────────────
//
// SQLite stores everything as TEXT/INTEGER/REAL. modernc.org/sqlite returns
// TEXT columns as plain strings. database/sql won't auto-convert string →
// time.Time, so we need Scanner + Valuer implementations.
//
// Usage in structs:  StartTime DBTime  /  EndedAt NullDBTime
// Access the time:   event.StartTime.Time   or   event.EndedAt.Time (check .Valid)

// DBTime is a non-nullable time.Time stored as RFC3339Nano TEXT in SQLite.
type DBTime struct{ time.Time }

func (t DBTime) Value() (driver.Value, error) {
	return t.UTC().Format(time.RFC3339Nano), nil
}

func (t *DBTime) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		t.Time = time.Time{}
	case int64:
		t.Time = time.Unix(v, 0).UTC()
	case string:
		return t.parse(v)
	case []byte:
		return t.parse(string(v))
	default:
		return fmt.Errorf("dbtypes.DBTime: unsupported scan type %T", src)
	}
	return nil
}

func (t *DBTime) parse(s string) error {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, s); err == nil {
			t.Time = parsed.UTC()
			return nil
		}
	}
	return fmt.Errorf("dbtypes.DBTime: cannot parse %q", s)
}

// NullDBTime is a nullable time.Time — stored as RFC3339Nano TEXT or NULL.
type NullDBTime struct {
	Time  time.Time
	Valid bool
}

func (t NullDBTime) Value() (driver.Value, error) {
	if !t.Valid {
		return nil, nil
	}
	return t.Time.UTC().Format(time.RFC3339Nano), nil
}

func (t *NullDBTime) Scan(src any) error {
	if src == nil {
		t.Valid = false
		return nil
	}
	var d DBTime
	if err := d.Scan(src); err != nil {
		return err
	}
	t.Time, t.Valid = d.Time, true
	return nil
}

// ── JSONSlice / JSONMap ───────────────────────────────────────────────────────

// JSONSlice is a generic slice stored as a JSON array TEXT column.
type JSONSlice[T any] []T

func (j JSONSlice[T]) Value() (driver.Value, error) {
	if j == nil {
		return "[]", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("dbtypes.JSONSlice: %w", err)
	}
	return string(b), nil
}

func (j *JSONSlice[T]) Scan(src any) error {
	b, err := srcBytes(src)
	if err != nil {
		return fmt.Errorf("dbtypes.JSONSlice: %w", err)
	}
	if b == nil {
		*j = []T{}
		return nil
	}
	return json.Unmarshal(b, j)
}

// JSONMap is a map[string]any stored as a JSON object TEXT column.
type JSONMap map[string]any

func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return "{}", nil
	}
	b, err := json.Marshal(j)
	if err != nil {
		return nil, fmt.Errorf("dbtypes.JSONMap: %w", err)
	}
	return string(b), nil
}

func (j *JSONMap) Scan(src any) error {
	b, err := srcBytes(src)
	if err != nil {
		return fmt.Errorf("dbtypes.JSONMap: %w", err)
	}
	if b == nil {
		*j = JSONMap{}
		return nil
	}
	return json.Unmarshal(b, j)
}

func srcBytes(src any) ([]byte, error) {
	switch v := src.(type) {
	case nil:
		return nil, nil
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported source type %T", src)
	}
}
