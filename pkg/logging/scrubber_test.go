package logging

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSensitiveDataScrubber_ScrubString_BearerToken(t *testing.T) {
	s := NewSensitiveDataScrubber()
	got := s.ScrubString("bearer abcdefghijklmnopqrstuvwx")
	require.Equal(t, "bearer [REDACTED]", got)
}

func TestSensitiveDataScrubber_ScrubString_Email(t *testing.T) {
	s := NewSensitiveDataScrubber()
	got := s.ScrubString("contact alice@example.com")
	require.Contains(t, got, "ali***@example.com")
	require.NotContains(t, got, "alice@example.com")
}

func TestSensitiveDataScrubber_EnableDisable(t *testing.T) {
	s := NewSensitiveDataScrubber()
	s.Disable()
	require.False(t, s.IsEnabled())
	require.Equal(t, "bearer abcdefghijklmnopqrstuvwx", s.ScrubString("bearer abcdefghijklmnopqrstuvwx"))

	s.Enable()
	require.True(t, s.IsEnabled())
	require.Equal(t, "bearer [REDACTED]", s.ScrubString("bearer abcdefghijklmnopqrstuvwx"))
}

func TestMin(t *testing.T) {
	require.Equal(t, 3, Min(3, 5))
	require.Equal(t, 2, Min(10, 2))
}
