package common

import (
	"context"
	"testing"

	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestLoggingHelpers_Round20_WithContextFallsBackToGlobalLogger(t *testing.T) {
	require.Same(t, Logger(), WithContext(context.Background()))
}

func TestLoggingHelpers_Round20_WithContextReturnsDerivedLogger(t *testing.T) {
	ctx := lambdacontext.NewContext(context.Background(), &lambdacontext.LambdaContext{
		AwsRequestID: "req-1",
	})

	require.NotSame(t, Logger(), WithContext(ctx))
}

func TestLoggingHelpers_Round20_WithFieldsReturnsDerivedLogger(t *testing.T) {
	require.NotSame(t, Logger(), WithFields(zap.String("key", "value")))
}

func TestLoggingHelpers_Round20_SyncDoesNotPanic(t *testing.T) {
	require.NotPanics(t, func() { Sync() })
}

