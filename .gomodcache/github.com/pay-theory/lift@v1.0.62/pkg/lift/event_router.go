package lift

import (
	"fmt"
	"strings"
	"sync"
)

// EventHandler represents a handler for non-HTTP events
type EventHandler interface {
	HandleEvent(ctx *Context) error
}

// EventHandlerFunc is an adapter to allow ordinary functions to be used as EventHandlers
type EventHandlerFunc func(ctx *Context) error

// HandleEvent calls f(ctx)
func (f EventHandlerFunc) HandleEvent(ctx *Context) error {
	return f(ctx)
}

// EventRoute represents a route for a specific event type
type EventRoute struct {
	// 8-byte aligned fields
	Handler EventHandler

	// Strings (16 bytes)
	Pattern string // For matching specific sources, queues, buckets, etc.

	// Smaller types
	TriggerType TriggerType
}

// EventRouter handles routing for non-HTTP Lambda events
type EventRouter struct {
	// 8-byte aligned fields
	routes map[TriggerType][]*EventRoute

	// 24-byte mutex
	mu sync.RWMutex
}

// NewEventRouter creates a new event router
func NewEventRouter() *EventRouter {
	return &EventRouter{
		routes: make(map[TriggerType][]*EventRoute),
	}
}

// AddEventRoute adds a route for a specific event type
func (er *EventRouter) AddEventRoute(triggerType TriggerType, pattern string, handler EventHandler) {
	route := &EventRoute{
		TriggerType: triggerType,
		Pattern:     pattern,
		Handler:     handler,
	}

	er.mu.Lock()
	defer er.mu.Unlock()
	er.routes[triggerType] = append(er.routes[triggerType], route)
}

// FindEventHandler finds the appropriate handler for an event
func (er *EventRouter) FindEventHandler(ctx *Context) (EventHandler, error) {
	triggerType := ctx.Request.TriggerType

	// Get routes for this trigger type
	er.mu.RLock()
	routes, exists := er.routes[triggerType]
	er.mu.RUnlock()

	if !exists || len(routes) == 0 {
		return nil, fmt.Errorf("no routes found for trigger type: %s", triggerType)
	}

	// For now, use simple pattern matching
	// In the future, this could be enhanced with more sophisticated matching
	for _, route := range routes {
		if er.matchesPattern(ctx, route) {
			return route.Handler, nil
		}
	}

	return nil, fmt.Errorf("no handler found for trigger type: %s", triggerType)
}

// matchesPattern checks if the event matches the route pattern
func (er *EventRouter) matchesPattern(ctx *Context, route *EventRoute) bool {
	// If pattern is empty or "*", it matches everything
	if route.Pattern == "" || route.Pattern == "*" {
		return true
	}

	switch route.TriggerType {
	case TriggerSQS:
		return er.matchSQSPattern(ctx, route.Pattern)
	case TriggerS3:
		return er.matchS3Pattern(ctx, route.Pattern)
	case TriggerEventBridge:
		return er.matchEventBridgePattern(ctx, route.Pattern)
	default:
		return true // Default to match for unknown types
	}
}

// matchSQSPattern matches SQS queue names or ARNs
func (er *EventRouter) matchSQSPattern(ctx *Context, pattern string) bool {
	// Extract queue information from the first record
	if len(ctx.Request.Records) == 0 {
		return false
	}

	if record, ok := ctx.Request.Records[0].(map[string]any); ok {
		if eventSourceARN, ok := record["eventSourceARN"].(string); ok {
			// Match against queue name or ARN
			return strings.Contains(eventSourceARN, pattern) ||
				strings.HasSuffix(eventSourceARN, ":"+pattern)
		}
	}

	return false
}

// matchS3Pattern matches S3 bucket names and object keys
func (er *EventRouter) matchS3Pattern(ctx *Context, pattern string) bool {
	extractor := newS3EventExtractor(ctx)
	bucketName, objectKey := extractor.extractS3Info()

	if bucketName == "" {
		return false
	}

	return er.matchS3PatternString(bucketName, objectKey, pattern)
}

// s3EventExtractor extracts S3 information from various event formats
type s3EventExtractor struct {
	ctx *Context
}

// newS3EventExtractor creates a new S3 event extractor
func newS3EventExtractor(ctx *Context) *s3EventExtractor {
	return &s3EventExtractor{ctx: ctx}
}

// extractS3Info extracts bucket name and object key from the event
func (e *s3EventExtractor) extractS3Info() (bucketName, objectKey string) {
	// Try EventBridge S3 events first
	if e.isEventBridgeS3Event() {
		return e.extractFromEventBridge()
	}

	// Try direct S3 events from records
	if e.hasS3Records() {
		return e.extractFromRecords()
	}

	return "", ""
}

// isEventBridgeS3Event checks if this is an S3 event through EventBridge
func (e *s3EventExtractor) isEventBridgeS3Event() bool {
	return e.ctx.Request.Source == "aws.s3" && e.ctx.Request.Detail != nil
}

// hasS3Records checks if the event has S3 records
func (e *s3EventExtractor) hasS3Records() bool {
	return len(e.ctx.Request.Records) > 0
}

// extractFromEventBridge extracts S3 info from EventBridge event format
func (e *s3EventExtractor) extractFromEventBridge() (bucketName, objectKey string) {
	// Extract bucket name
	if bucket, ok := e.ctx.Request.Detail["bucket"].(map[string]any); ok {
		if name, nameOk := bucket["name"].(string); nameOk {
			bucketName = name
		}
	}

	// Extract object key
	if object, ok := e.ctx.Request.Detail["object"].(map[string]any); ok {
		if key, keyOk := object["key"].(string); keyOk {
			objectKey = key
		}
	}

	return bucketName, objectKey
}

// extractFromRecords extracts S3 info from direct S3 event records
func (e *s3EventExtractor) extractFromRecords() (bucketName, objectKey string) {
	record, ok := e.ctx.Request.Records[0].(map[string]any)
	if !ok {
		return "", ""
	}

	s3Data, ok := record["s3"].(map[string]any)
	if !ok {
		return "", ""
	}

	// Extract bucket name
	if bucket, ok := s3Data["bucket"].(map[string]any); ok {
		if name, nameOk := bucket["name"].(string); nameOk {
			bucketName = name
		}
	}

	// Extract object key
	if object, ok := s3Data["object"].(map[string]any); ok {
		if key, keyOk := object["key"].(string); keyOk {
			objectKey = key
		}
	}

	return bucketName, objectKey
}

// matchS3PatternString matches S3 patterns against bucket names and object keys
func (er *EventRouter) matchS3PatternString(bucketName, objectKey, pattern string) bool {
	// Wildcard matches everything
	if pattern == "*" {
		return true
	}

	// If pattern starts with /, treat it as an object key pattern
	if strings.HasPrefix(pattern, "/") {
		return er.matchObjectKeyPattern(objectKey, pattern[1:]) // Remove leading /
	}

	// If pattern contains /, treat it as bucket/key pattern
	if strings.Contains(pattern, "/") {
		parts := strings.SplitN(pattern, "/", 2)
		bucketPattern := parts[0]
		keyPattern := parts[1]

		// Match bucket part
		if !er.matchWildcardPattern(bucketName, bucketPattern) {
			return false
		}

		// Match key part
		return er.matchObjectKeyPattern(objectKey, keyPattern)
	}

	// Otherwise, just match bucket name
	return er.matchWildcardPattern(bucketName, pattern)
}

// matchObjectKeyPattern matches patterns against object keys
func (er *EventRouter) matchObjectKeyPattern(objectKey, pattern string) bool {
	if pattern == "*" || pattern == "**" {
		return true
	}

	// Support path-like patterns
	// e.g., "uploads/*", "*/file.zip", "data/*/reports"
	return er.matchWildcardPattern(objectKey, pattern)
}

// matchWildcardPattern matches a string against a pattern with wildcards
func (er *EventRouter) matchWildcardPattern(str, pattern string) bool {
	matcher := newWildcardMatcher(str, pattern)
	return matcher.match()
}

// wildcardMatcher handles wildcard pattern matching
type wildcardMatcher struct {
	str     string
	pattern string
	parts   []string
}

// newWildcardMatcher creates a new wildcard matcher
func newWildcardMatcher(str, pattern string) *wildcardMatcher {
	return &wildcardMatcher{
		str:     str,
		pattern: pattern,
	}
}

// match performs the pattern matching
func (m *wildcardMatcher) match() bool {
	// Try simple matches first
	if m.matchExact() || m.matchSingleWildcard() {
		return true
	}

	// Try specific wildcard patterns
	if m.matchPrefix() || m.matchSuffix() || m.matchMiddle() {
		return true
	}

	// Handle complex patterns
	return m.matchMultipleWildcards()
}

// matchExact checks for exact string match
func (m *wildcardMatcher) matchExact() bool {
	return m.pattern == m.str
}

// matchSingleWildcard checks for single wildcard pattern
func (m *wildcardMatcher) matchSingleWildcard() bool {
	return m.pattern == "*"
}

// matchPrefix checks for prefix wildcard pattern
func (m *wildcardMatcher) matchPrefix() bool {
	if !strings.HasSuffix(m.pattern, "*") {
		return false
	}

	withoutSuffix := m.pattern[:len(m.pattern)-1]
	if strings.Contains(withoutSuffix, "*") {
		return false
	}

	return strings.HasPrefix(m.str, withoutSuffix)
}

// matchSuffix checks for suffix wildcard pattern
func (m *wildcardMatcher) matchSuffix() bool {
	if !strings.HasPrefix(m.pattern, "*") {
		return false
	}

	withoutPrefix := m.pattern[1:]
	if strings.Contains(withoutPrefix, "*") {
		return false
	}

	return strings.HasSuffix(m.str, withoutPrefix)
}

// matchMiddle checks for single middle wildcard pattern
func (m *wildcardMatcher) matchMiddle() bool {
	if strings.Count(m.pattern, "*") != 1 {
		return false
	}

	m.parts = strings.Split(m.pattern, "*")
	if len(m.parts) != 2 {
		return false
	}

	return strings.HasPrefix(m.str, m.parts[0]) && strings.HasSuffix(m.str, m.parts[1])
}

// matchMultipleWildcards handles patterns with multiple wildcards
func (m *wildcardMatcher) matchMultipleWildcards() bool {
	if !strings.Contains(m.pattern, "*") {
		return false
	}

	m.parts = strings.Split(m.pattern, "*")
	return m.matchParts()
}

// matchParts matches string parts sequentially
func (m *wildcardMatcher) matchParts() bool {
	lastIndex := 0

	for i, part := range m.parts {
		if part == "" {
			continue
		}

		index := strings.Index(m.str[lastIndex:], part)
		if index == -1 {
			return false
		}

		// First part must match at the beginning
		if i == 0 && index != 0 {
			return false
		}

		lastIndex = lastIndex + index + len(part)
	}

	// Check if last part should match at the end
	return m.checkLastPart()
}

// checkLastPart verifies the last part matches at string end
func (m *wildcardMatcher) checkLastPart() bool {
	if len(m.parts) == 0 {
		return true
	}

	lastPart := m.parts[len(m.parts)-1]
	if lastPart == "" {
		return true
	}

	return strings.HasSuffix(m.str, lastPart)
}

// matchEventBridgePattern matches EventBridge source patterns
func (er *EventRouter) matchEventBridgePattern(ctx *Context, pattern string) bool {
	// For scheduled events, match against the rule name in resources
	if ctx.Request.Source == "aws.events" && ctx.Request.DetailType == "Scheduled Event" {
		return er.matchScheduledEventPattern(ctx, pattern)
	}

	// For other EventBridge events, match against source
	source := ctx.Request.Source

	// Support simple wildcard matching
	if pattern == "*" {
		return true
	}

	// Support prefix matching with wildcards
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(source, prefix)
	}

	// Exact match
	return source == pattern
}

// matchScheduledEventPattern matches scheduled event rule patterns
func (er *EventRouter) matchScheduledEventPattern(ctx *Context, pattern string) bool {
	// Extract rule name from resources
	ruleName := ""
	for _, record := range ctx.Request.Records {
		if resourceStr, ok := record.(string); ok {
			// Rule ARN format: arn:aws:events:region:account:rule/rule-name
			parts := strings.Split(resourceStr, "/")
			if len(parts) > 1 && strings.Contains(resourceStr, ":rule/") {
				ruleName = parts[len(parts)-1]
				break
			}
		}
	}

	if ruleName == "" {
		return pattern == "*"
	}

	// Support wildcard matching
	if pattern == "*" {
		return true
	}

	// Support prefix matching with wildcards
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(ruleName, prefix)
	}

	// Support suffix matching with wildcards
	if strings.HasPrefix(pattern, "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.HasSuffix(ruleName, suffix)
	}

	// Support middle wildcards (e.g., "bin-*-daily")
	if strings.Contains(pattern, "*") {
		// Convert pattern to regex-like matching
		parts := strings.Split(pattern, "*")
		if len(parts) == 2 {
			return strings.HasPrefix(ruleName, parts[0]) && strings.HasSuffix(ruleName, parts[1])
		}
	}

	// Exact match
	return ruleName == pattern
}

// HandleEvent routes an event to the appropriate handler and applies the provided middleware chain.
func (er *EventRouter) HandleEvent(ctx *Context, middlewareChain []Middleware) error {
	handler, err := er.FindEventHandler(ctx)
	if err != nil {
		return err
	}

	finalHandler := Handler(eventHandlerAdapter{handler: handler})
	for i := len(middlewareChain) - 1; i >= 0; i-- {
		finalHandler = middlewareChain[i](finalHandler)
	}

	return finalHandler.Handle(ctx)
}

type eventHandlerAdapter struct {
	handler EventHandler
}

func (a eventHandlerAdapter) Handle(ctx *Context) error {
	return a.handler.HandleEvent(ctx)
}

// GetRoutes returns all routes for debugging/inspection
func (er *EventRouter) GetRoutes() map[TriggerType][]*EventRoute {
	er.mu.RLock()
	defer er.mu.RUnlock()

	// Create a copy to avoid external modifications
	routesCopy := make(map[TriggerType][]*EventRoute)
	for k, v := range er.routes {
		routesCopy[k] = append([]*EventRoute(nil), v...)
	}
	return routesCopy
}

// SQSMessage represents a parsed SQS message for type-safe handling
type SQSMessage struct {
	MessageID     string         `json:"messageId"`
	Body          string         `json:"body"`
	ReceiptHandle string         `json:"receiptHandle"`
	Attributes    map[string]any `json:"attributes"`
	EventSource   string         `json:"eventSource"`
}

// S3Event represents a parsed S3 event for type-safe handling
type S3Event struct {
	S3Data      map[string]any `json:"s3"`
	EventSource string         `json:"eventSource"`
	EventName   string         `json:"eventName"`
	EventTime   string         `json:"eventTime"`
	Bucket      string         `json:"bucket"`
	ObjectKey   string         `json:"objectKey"`
	ObjectSize  int64          `json:"objectSize"`
}

// EventBridgeEvent represents a parsed EventBridge event for type-safe handling
type EventBridgeEvent struct {
	Source     string         `json:"source"`
	DetailType string         `json:"detail-type"`
	Detail     map[string]any `json:"detail"`
	Time       string         `json:"time"`
	ID         string         `json:"id"`
	Resources  []string       `json:"resources"`
}

// ParseSQSMessages extracts SQS messages from the request
func (ctx *Context) ParseSQSMessages() ([]SQSMessage, error) {
	if ctx.Request.TriggerType != TriggerSQS {
		return nil, fmt.Errorf("not an SQS event")
	}

	var messages []SQSMessage
	for _, record := range ctx.Request.Records {
		if recordMap, ok := record.(map[string]any); ok {
			message := SQSMessage{
				MessageID:     getStringField(recordMap, "messageId"),
				Body:          getStringField(recordMap, "body"),
				ReceiptHandle: getStringField(recordMap, "receiptHandle"),
				EventSource:   getStringField(recordMap, "eventSource"),
				Attributes:    getMapField(recordMap, "attributes"),
			}
			messages = append(messages, message)
		}
	}

	return messages, nil
}

// ParseS3Event extracts S3 event information from the request
func (ctx *Context) ParseS3Event() (*S3Event, error) {
	if ctx.Request.TriggerType != TriggerS3 {
		return nil, fmt.Errorf("not an S3 event")
	}

	if len(ctx.Request.Records) == 0 {
		return nil, fmt.Errorf("no S3 records found")
	}

	// Use the first record
	if recordMap, ok := ctx.Request.Records[0].(map[string]any); ok {
		s3Data := getMapField(recordMap, "s3")
		bucket := getMapField(s3Data, "bucket")
		object := getMapField(s3Data, "object")

		event := &S3Event{
			EventSource: getStringField(recordMap, "eventSource"),
			EventName:   getStringField(recordMap, "eventName"),
			EventTime:   getStringField(recordMap, "eventTime"),
			Bucket:      getStringField(bucket, "name"),
			ObjectKey:   getStringField(object, "key"),
			S3Data:      s3Data,
		}

		// Parse object size if available
		if sizeStr := getStringField(object, "size"); sizeStr != "" {
			// Convert string to int64 if needed
			event.ObjectSize = 0 // Default value
		}

		return event, nil
	}

	return nil, fmt.Errorf("invalid S3 record format")
}

// ParseEventBridgeEvent extracts EventBridge event information
func (ctx *Context) ParseEventBridgeEvent() (*EventBridgeEvent, error) {
	if ctx.Request.TriggerType != TriggerEventBridge {
		return nil, fmt.Errorf("not an EventBridge event")
	}

	event := &EventBridgeEvent{
		Source:     ctx.Request.Source,
		DetailType: ctx.Request.DetailType,
		Detail:     ctx.Request.Detail,
		Time:       ctx.Request.Timestamp,
		ID:         ctx.Request.EventID,
	}

	// Convert Records to string slice if available
	for _, record := range ctx.Request.Records {
		if str, ok := record.(string); ok {
			event.Resources = append(event.Resources, str)
		}
	}

	return event, nil
}

// IsScheduledEvent checks if this EventBridge event is a scheduled event
func (ctx *Context) IsScheduledEvent() bool {
	return ctx.Request.TriggerType == TriggerEventBridge &&
		ctx.Request.Source == "aws.events" &&
		ctx.Request.DetailType == "Scheduled Event"
}

// GetScheduledRuleName extracts the rule name from a scheduled event
func (ctx *Context) GetScheduledRuleName() string {
	if !ctx.IsScheduledEvent() {
		return ""
	}

	// Extract rule name from resources
	for _, record := range ctx.Request.Records {
		if resourceStr, ok := record.(string); ok {
			// Rule ARN format: arn:aws:events:region:account:rule/rule-name
			parts := strings.Split(resourceStr, "/")
			if len(parts) > 1 && strings.Contains(resourceStr, ":rule/") {
				return parts[len(parts)-1]
			}
		}
	}

	return ""
}

// Helper functions for safe field extraction
func getStringField(data map[string]any, key string) string {
	if value, exists := data[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return ""
}

func getMapField(data map[string]any, key string) map[string]any {
	if value, exists := data[key]; exists {
		if mapValue, ok := value.(map[string]any); ok {
			return mapValue
		}
	}
	return make(map[string]any)
}
