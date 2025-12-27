package trust

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultPropagationConfig(t *testing.T) {
	cfg := DefaultPropagationConfig()
	require.NotNil(t, cfg)
	require.Equal(t, 3, cfg.MaxDepth)
	require.Equal(t, 0.5, cfg.DecayFactor)
	require.Equal(t, 1.5, cfg.NegativeMultiplier)
	require.Equal(t, 0.1, cfg.MinPropagatedScore)
	require.Equal(t, 2, cfg.CacheDuration)
}
