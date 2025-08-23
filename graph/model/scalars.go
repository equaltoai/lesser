package model

import (
	"io"
	"time"

	"go.uber.org/zap"
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
		return ErrTimeNotString
	}
}

// MarshalGQL implements the graphql.Marshaler interface
func (t Time) MarshalGQL(w io.Writer) {
	if _, err := w.Write([]byte(`"` + time.Time(t).Format(time.RFC3339) + `"`)); err != nil {
		// Log error but don't return it as the interface doesn't support it
		// This is typical for GraphQL marshalers
		zap.L().Warn("failed to write time to GraphQL response", zap.Error(err))
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
		return ErrCursorNotString
	}
}

// MarshalGQL implements the graphql.Marshaler interface
func (c Cursor) MarshalGQL(w io.Writer) {
	if _, err := w.Write([]byte(`"` + string(c) + `"`)); err != nil {
		// Log error but don't return it as the interface doesn't support it
		// This is typical for GraphQL marshalers
		zap.L().Warn("failed to write cursor to GraphQL response", zap.Error(err))
	}
}
