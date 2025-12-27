package monitoring

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAlertRateLimiter_ShouldAlert(t *testing.T) {
	rl := NewAlertRateLimiter(50 * time.Millisecond)
	require.True(t, rl.ShouldAlert("k"))
	require.False(t, rl.ShouldAlert("k"))

	time.Sleep(60 * time.Millisecond)
	require.True(t, rl.ShouldAlert("k"))
}

func TestParseInt(t *testing.T) {
	require.Equal(t, 123, parseInt("123"))
	require.Equal(t, 0, parseInt("not-a-number"))
}

func TestGetEvaluationWindow_Default(t *testing.T) {
	require.Equal(t, 5, getEvaluationWindow("unknown"))
}
