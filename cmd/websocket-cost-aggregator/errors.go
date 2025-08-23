package main

import "errors"

// Error constants for websocket-cost-aggregator
var (
	// Alert-related errors
	ErrAllAlertMethodsFailed = errors.New("all alert methods failed")
	ErrMarshalAlertMessage   = errors.New("failed to marshal alert message")
	
	// AWS/Repository errors
	ErrGetIdleConnections  = errors.New("failed to get idle connections")
	ErrGetHighCostUsers    = errors.New("failed to get high cost users")
	ErrGetStaleConnections = errors.New("failed to get stale connections")
	ErrPublishSNSMessage   = errors.New("failed to publish SNS message")
	
	// HTTP/Webhook errors
	ErrCreateWebhookRequest = errors.New("failed to create webhook request")
	ErrWebhookRequestFailed = errors.New("webhook request failed")
	ErrWebhookNon2xxStatus  = errors.New("webhook returned non-2xx status")
	
	// Processing errors
	ErrTrackIdleConnections = errors.New("failed to track idle connections")
)