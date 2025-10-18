// Package common provides runtime environment helpers for AWS Lambda functions.
//
//nolint:revive // package name "common" is intentional and widely used throughout the codebase
package common

import (
	"os"
	"strconv"
)

// LambdaRuntimeInfo provides access to AWS Lambda runtime environment variables
type LambdaRuntimeInfo struct{}

// NewLambdaRuntime creates a new LambdaRuntimeInfo instance
func NewLambdaRuntime() *LambdaRuntimeInfo {
	return &LambdaRuntimeInfo{}
}

// GetFunctionName returns the AWS Lambda function name from AWS_LAMBDA_FUNCTION_NAME
func (l *LambdaRuntimeInfo) GetFunctionName() string {
	return os.Getenv("AWS_LAMBDA_FUNCTION_NAME")
}

// GetFunctionVersion returns the AWS Lambda function version from AWS_LAMBDA_FUNCTION_VERSION
func (l *LambdaRuntimeInfo) GetFunctionVersion() string {
	return os.Getenv("AWS_LAMBDA_FUNCTION_VERSION")
}

// GetMemorySize returns the AWS Lambda function memory size from AWS_LAMBDA_FUNCTION_MEMORY_SIZE
func (l *LambdaRuntimeInfo) GetMemorySize() string {
	return os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
}

// GetMemorySizeInt returns the AWS Lambda function memory size as an integer
func (l *LambdaRuntimeInfo) GetMemorySizeInt() (int, error) {
	memStr := os.Getenv("AWS_LAMBDA_FUNCTION_MEMORY_SIZE")
	if memStr == "" {
		return 0, nil
	}
	return strconv.Atoi(memStr)
}

// GetInitializationType returns the AWS Lambda initialization type from AWS_LAMBDA_INITIALIZATION_TYPE
func (l *LambdaRuntimeInfo) GetInitializationType() string {
	return os.Getenv("AWS_LAMBDA_INITIALIZATION_TYPE")
}

// GetLogGroupName returns the AWS Lambda log group name from AWS_LAMBDA_LOG_GROUP_NAME
func (l *LambdaRuntimeInfo) GetLogGroupName() string {
	return os.Getenv("AWS_LAMBDA_LOG_GROUP_NAME")
}

// GetLogStreamName returns the AWS Lambda log stream name from AWS_LAMBDA_LOG_STREAM_NAME
func (l *LambdaRuntimeInfo) GetLogStreamName() string {
	return os.Getenv("AWS_LAMBDA_LOG_STREAM_NAME")
}

// GetXRayTraceID returns the X-Ray trace ID from _X_AMZN_TRACE_ID
func (l *LambdaRuntimeInfo) GetXRayTraceID() string {
	return os.Getenv("_X_AMZN_TRACE_ID")
}

// IsRunningInLambda returns true if running in AWS Lambda environment
func (l *LambdaRuntimeInfo) IsRunningInLambda() bool {
	return l.GetFunctionName() != ""
}

// IsXRayEnabled returns true if X-Ray tracing is enabled
func (l *LambdaRuntimeInfo) IsXRayEnabled() bool {
	return l.GetXRayTraceID() != ""
}

// IsColdStart returns true if this is a cold start (initialization type is on-demand)
func (l *LambdaRuntimeInfo) IsColdStart() bool {
	return l.GetInitializationType() != "provisioned-concurrency"
}

// Package-level convenience functions for backward compatibility
var defaultRuntime = NewLambdaRuntime()

// GetLambdaFunctionName returns the AWS Lambda function name
func GetLambdaFunctionName() string {
	return defaultRuntime.GetFunctionName()
}

// GetLambdaFunctionVersion returns the AWS Lambda function version
func GetLambdaFunctionVersion() string {
	return defaultRuntime.GetFunctionVersion()
}

// GetLambdaMemorySize returns the AWS Lambda function memory size
func GetLambdaMemorySize() string {
	return defaultRuntime.GetMemorySize()
}

// GetLambdaMemorySizeInt returns the AWS Lambda function memory size as an integer
func GetLambdaMemorySizeInt() (int, error) {
	return defaultRuntime.GetMemorySizeInt()
}

// GetLambdaInitializationType returns the AWS Lambda initialization type
func GetLambdaInitializationType() string {
	return defaultRuntime.GetInitializationType()
}

// GetLambdaLogGroupName returns the AWS Lambda log group name
func GetLambdaLogGroupName() string {
	return defaultRuntime.GetLogGroupName()
}

// GetLambdaLogStreamName returns the AWS Lambda log stream name
func GetLambdaLogStreamName() string {
	return defaultRuntime.GetLogStreamName()
}

// GetXRayTraceID returns the X-Ray trace ID
func GetXRayTraceID() string {
	return defaultRuntime.GetXRayTraceID()
}

// IsRunningInLambda returns true if running in AWS Lambda environment
func IsRunningInLambda() bool {
	return defaultRuntime.IsRunningInLambda()
}

// IsXRayEnabled returns true if X-Ray tracing is enabled
func IsXRayEnabled() bool {
	return defaultRuntime.IsXRayEnabled()
}

// IsColdStart returns true if this is a cold start
func IsColdStart() bool {
	return defaultRuntime.IsColdStart()
}
