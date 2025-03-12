package store

import (
	"database/sql"
	"encoding/json"
)

// NullString is a wrapper around sql.NullString for Swagger compatibility
type NullString struct {
	Value string `json:"value"` // The actual string value
	Valid bool   `json:"valid"` // Indicates if the value is non-null
}

// MarshalJSON implements the json.Marshaler interface
func (ns NullString) MarshalJSON() ([]byte, error) {
	if !ns.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ns.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (ns *NullString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ns.Valid = false
		ns.Value = ""
		return nil
	}
	ns.Valid = true
	return json.Unmarshal(data, &ns.Value)
}

// Convert from sql.NullString
func NewNullString(ns sql.NullString) NullString {
	return NullString{
		Value: ns.String,
		Valid: ns.Valid,
	}
}

// NullInt16 is a wrapper around sql.NullInt16 for Swagger compatibility
type NullInt16 struct {
	Value int16 `json:"value"` // The actual integer value
	Valid bool  `json:"valid"` // Indicates if the value is non-null
}

// MarshalJSON implements the json.Marshaler interface
func (ni NullInt16) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Value)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (ni *NullInt16) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ni.Valid = false
		ni.Value = 0
		return nil
	}
	ni.Valid = true
	return json.Unmarshal(data, &ni.Value)
}

// Convert from sql.NullInt16
func NewNullInt16(ni sql.NullInt16) NullInt16 {
	return NullInt16{
		Value: ni.Int16,
		Valid: ni.Valid,
	}
}
