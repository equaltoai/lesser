package lambda

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/require"
)

func TestParseSignature(t *testing.T) {
	parsed := parseSignature(`Signature keyId="abc",headers="(request-target) host date",signature="xyz"`)
	require.Equal(t, map[string]string{
		"keyId":     "abc",
		"headers":   "(request-target) host date",
		"signature": "xyz",
	}, parsed)
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()
	require.Equal(t, 3, cfg.MaxAttempts)
	require.Equal(t, 1*time.Second, cfg.InitialDelay)
	require.Contains(t, cfg.PermanentErrors, 404)
}

func TestDefaultMainConfig(t *testing.T) {
	api := DefaultMainConfig("svc", common.LambdaTypeAPI)
	require.True(t, api.EnableCORS)
	require.True(t, api.EnableRateLimit)

	processor := DefaultMainConfig("svc", common.LambdaTypeProcessor)
	require.False(t, processor.EnableCORS)
	require.False(t, processor.EnableRateLimit)
}
