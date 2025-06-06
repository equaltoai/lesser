package model

import (
	"fmt"
	"io"
	"time"
)

// Time is a custom GraphQL scalar
type Time time.Time

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (t *Time) UnmarshalGQL(v interface{}) error {
	switch v := v.(type) {
	case string:
		parsed, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		*t = Time(parsed)
		return nil
	default:
		return fmt.Errorf("time must be a string")
	}
}

// MarshalGQL implements the graphql.Marshaler interface
func (t Time) MarshalGQL(w io.Writer) {
	w.Write([]byte(`"` + time.Time(t).Format(time.RFC3339) + `"`))
}

// Cursor is a custom GraphQL scalar for pagination
type Cursor string

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (c *Cursor) UnmarshalGQL(v interface{}) error {
	switch v := v.(type) {
	case string:
		*c = Cursor(v)
		return nil
	default:
		return fmt.Errorf("cursor must be a string")
	}
}

// MarshalGQL implements the graphql.Marshaler interface
func (c Cursor) MarshalGQL(w io.Writer) {
	w.Write([]byte(`"` + string(c) + `"`))
}
