package cloudwatch_test

import (
	"context"
	"time"

	"github.com/pay-theory/lift/pkg/observability"
	cw "github.com/pay-theory/lift/pkg/observability/cloudwatch"
)

// Example creating a CloudWatch structured logger with the in-package mock client.
func ExampleNewCloudWatchLogger() {
	cfg := observability.LoggerConfig{
		LogGroup:      "my-app-logs",
		LogStream:     "app-1",
		BufferSize:    100,
		BatchSize:     25,
		FlushInterval: 2 * time.Second,
	}

	client := cw.NewMockCloudWatchLogsClient()
	logger, _ := cw.NewCloudWatchLogger(cfg, client)

	logger = logger.WithField("service", "users").(observability.StructuredLogger)
	logger.Info("application started")
	_ = logger.Flush(context.Background())
}
