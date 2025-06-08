package model

import (
	"fmt"
	"io"
	"strconv"
	"time"
)

// Duration represents a time duration in seconds
type Duration int

// UnmarshalGQL implements the graphql.Unmarshaler interface
func (d *Duration) UnmarshalGQL(v interface{}) error {
	switch v := v.(type) {
	case int:
		*d = Duration(v)
		return nil
	case int64:
		*d = Duration(v)
		return nil
	case float64:
		*d = Duration(int(v))
		return nil
	case string:
		// Try to parse as a number of seconds
		seconds, err := strconv.Atoi(v)
		if err != nil {
			// Try to parse as a duration string (e.g., "5m30s")
			duration, err := time.ParseDuration(v)
			if err != nil {
				return fmt.Errorf("Duration must be an integer (seconds) or a duration string")
			}
			*d = Duration(int(duration.Seconds()))
			return nil
		}
		*d = Duration(seconds)
		return nil
	default:
		return fmt.Errorf("Duration must be an integer (seconds) or a duration string")
	}
}

// MarshalGQL implements the graphql.Marshaler interface
func (d Duration) MarshalGQL(w io.Writer) {
	fmt.Fprintf(w, "%d", d)
}

// String returns a formatted duration string
func (d Duration) String() string {
	duration := time.Duration(d) * time.Second
	return duration.String()
}

// Seconds returns the duration in seconds
func (d Duration) Seconds() int {
	return int(d)
}
