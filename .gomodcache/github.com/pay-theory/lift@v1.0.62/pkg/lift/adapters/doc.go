// Package adapters provides event adapters that normalize AWS Lambda events to
// a single internal request shape. The AdapterRegistry detects or applies a
// preferred adapter for API Gateway v1/v2, SQS, S3, EventBridge, and WebSocket
// events. Applications rarely use this package directly; the App parses events
// via the registry and routes them through the unified Context.
package adapters
