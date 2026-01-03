package main

import (
	"testing"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActivityProcessor_NewActivityProcessor_Constructs(t *testing.T) {
	t.Setenv("DOMAIN", "example.com")
	t.Setenv("DYNAMO_TABLE_NAME", "test-table")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "fake")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "fake")
	t.Setenv("VAPID_SUBJECT", "https://example.com")
	config.ResetForTests()

	lambdaCtx := &common.LambdaContext{
		Config: config.Get(),
		Logger: zap.NewNop(),
	}

	ap := NewActivityProcessor(lambdaCtx)
	require.NotNil(t, ap)
	require.Equal(t, "test-table", ap.tableName)
	require.Equal(t, lambdaCtx.Config.BaseURL(), ap.baseURL)
	require.Equal(t, 3, ap.retryAttempts)
	require.NotNil(t, ap.fetchService)
}

func TestActivityProcessor_Main_UsesLambdaStartSeam(t *testing.T) {
	orig := lambdaStartFn
	t.Cleanup(func() { lambdaStartFn = orig })

	var called bool
	lambdaStartFn = func(_ interface{}) { called = true }

	main()
	require.True(t, called)
}
