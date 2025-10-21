package main

import (
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/observability"
)

func main() {
	// Example 1: Using the default configuration with just a log level
	defaultConfig := observability.NewDefaultLoggerConfig("info")
	fmt.Printf("Default config:\n%+v\n\n", defaultConfig)

	// Example 2: Using the configuration with custom options
	customConfig := observability.NewLoggerConfigWithOptions("debug",
		observability.WithBatchSize(25),
		observability.WithFlushInterval(5*time.Second),
		observability.WithBufferSize(100),
		observability.WithAsyncLogging(),
		observability.WithCallerInfo(),
	)
	fmt.Printf("Custom config:\n%+v\n\n", customConfig)

	// Example 3: Creating a logger with the config
	// Note: In a real Lambda function, you would create an actual CloudWatch client
	// This is just to show how the config would be used
	fmt.Println("To use this config with CloudWatch logger:")
	fmt.Println("logger, err := cloudwatch.NewCloudWatchLogger(config, cwClient)")
}
