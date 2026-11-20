package db

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a JSON document column: TEXT in SQLite, jsonb in PostgreSQL.
type JSON json.RawMessage

func (j *JSON) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case string:
		*j = JSON(v)
	case []byte:
		*j = append(JSON(nil), v...)
	default:
		return fmt.Errorf("cannot scan %T into JSON", src)
	}
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return string(j), nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSON) UnmarshalJSON(b []byte) error {
	*j = append(JSON(nil), b...)
	return nil
}
