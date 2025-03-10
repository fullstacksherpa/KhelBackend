package store

import (
	"database/sql"
	"encoding/json"
)

// NullString is a wrapper around sql.NullString for Swagger compatibility

type NullString struct {
	sql.NullString
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
		return nil
	}
	ns.Valid = true
	return json.Unmarshal(data, &ns.String)
}

// NullInt16 is a wrapper around sql.NullInt16 for Swagger compatibility

type NullInt16 struct {
	sql.NullInt16
}

// MarshalJSON implements the json.Marshaler interface
func (ni NullInt16) MarshalJSON() ([]byte, error) {
	if !ni.Valid {
		return []byte("null"), nil
	}
	return json.Marshal(ni.Int16)
}

// UnmarshalJSON implements the json.Unmarshaler interface
func (ni *NullInt16) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		ni.Valid = false
		return nil
	}
	ni.Valid = true
	return json.Unmarshal(data, &ni.Int16)
}
