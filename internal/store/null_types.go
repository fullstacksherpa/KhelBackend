package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strconv"
)

// NullString is a wrapper around sql.NullString for Swagger compatibility
type NullString struct {
	String string `json:"value"` // The actual string value
	Valid  bool   `json:"valid"` // Indicates if the value is non-null
}

// Scan implements the sql.Scanner interface.
func (ns *NullString) Scan(value interface{}) error {
	if value == nil {
		ns.String, ns.Valid = "", false
		return nil
	}
	switch v := value.(type) {
	case string:
		ns.String, ns.Valid = v, true
	case []byte:
		ns.String, ns.Valid = string(v), true
	default:
		return fmt.Errorf("cannot scan type %T into NullString: %v", value, value)
	}
	return nil
}

// Value implements the driver.Valuer interface.
func (ns NullString) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return ns.String, nil
}

// MarshalJSON implements the json.Marshaler interface
func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.String)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (ns *NullString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ns.Valid = false
		ns.String = ""
		return nil
	}
	ns.Valid = true
	return json.Unmarshal(data, &ns.String)
}

// Convert from sql.NullString
func NewNullString(ns sql.NullString) NullString {
	return NullString{
		String: ns.String,
		Valid:  ns.Valid,
	}
}

// / NullInt16 is a wrapper around sql.NullInt16 for Swagger compatibility.
type NullInt16 struct {
	Int   int16 `json:"value"` // The actual integer value.
	Valid bool  `json:"valid"` // Valid is true if Int is not NULL.
}

// Scan implements the sql.Scanner interface.
func (ns *NullInt16) Scan(value interface{}) error {
	if value == nil {
		ns.Int, ns.Valid = 0, false
		return nil
	}
	switch v := value.(type) {
	case int64:
		ns.Int = int16(v)
		ns.Valid = true
		return nil
	case int16:
		ns.Int = v
		ns.Valid = true
		return nil
	case []byte:
		i, err := strconv.ParseInt(string(v), 10, 16)
		if err != nil {
			return err
		}
		ns.Int = int16(i)
		ns.Valid = true
		return nil
	case string:
		i, err := strconv.ParseInt(v, 10, 16)
		if err != nil {
			return err
		}
		ns.Int = int16(i)
		ns.Valid = true
		return nil
	default:
		return fmt.Errorf("cannot scan type %T into NullInt16", value)
	}
}

// Value implements the driver.Valuer interface.
func (ns NullInt16) Value() (driver.Value, error) {
	if !ns.Valid {
		return nil, nil
	}
	return ns.Int, nil
}

// MarshalJSON implements the json.Marshaler interface.
func (ni NullInt16) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int)
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (ni *NullInt16) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ni.Valid = false
		ni.Int = 0
		return nil
	}
	var i int16
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	ni.Int = i
	ni.Valid = true
	return nil
}

// NewNullInt16 converts from sql.NullInt16 to our custom NullInt16.
func NewNullInt16(ni sql.NullInt16) NullInt16 {
	return NullInt16{
		Int:   ni.Int16,
		Valid: ni.Valid,
	}
}
