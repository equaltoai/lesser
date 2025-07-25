package model

import (
	"fmt"
	"io"
	"log"
	"time"
)

// Time is a custom GraphQL scalar
type Time time.Time

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (t *Time) UnmarshalGQL(v any) error {
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
	if _, err := w.Write([]byte(`"` + time.Time(t).Format(time.RFC3339) + `"`)); err != nil {
		// Log error but don't return it as the interface doesn't support it
		// This is typical for GraphQL marshalers
		log.Printf("Warning: failed to write time to GraphQL response: %v", err)
	}
}

// Cursor is a custom GraphQL scalar for pagination
type Cursor string

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (c *Cursor) UnmarshalGQL(v any) error {
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
	if _, err := w.Write([]byte(`"` + string(c) + `"`)); err != nil {
		// Log error but don't return it as the interface doesn't support it
		// This is typical for GraphQL marshalers
		log.Printf("Warning: failed to write cursor to GraphQL response: %v", err)
	}
}
