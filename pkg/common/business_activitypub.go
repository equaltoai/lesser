// Package common provides ActivityPub-specific business logic utilities for the Lesser project.
// This consolidates ActivityPub federation business patterns found across services to eliminate
// duplication and ensure consistency in federation behavior.
package common

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ActivityPubObject represents a generic ActivityPub object interface
type ActivityPubObject interface {
	GetID() string
	GetType() string
	GetActor() string
	GetObject() interface{}
	GetPublished() time.Time
}

// ActivityPubActor represents a generic ActivityPub actor interface
type ActivityPubActor interface {
	GetID() string
	GetType() string
	GetInbox() string
	GetOutbox() string
	GetFollowers() string
	GetFollowing() string
}

// FederationConfig configures federation behavior
type FederationConfig struct {
	Domain         string
	PrivateKeyPath string
	PublicKeyPath  string
	UserAgent      string
	MaxRetries     int
	RetryDelay     time.Duration
	RequestTimeout time.Duration
}

// ActivityPubValidationRules defines validation rules for ActivityPub objects
type ActivityPubValidationRules struct {
	RequireHTTPS       bool
	AllowedDomains     []string
	BlockedDomains     []string
	MaxObjectSize      int64
	RequireSignature   bool
	ValidActivityTypes []string
}

// ActivityPubBusinessLogic provides centralized ActivityPub business operations
type ActivityPubBusinessLogic struct {
	config *FederationConfig
	logger *zap.Logger
}

// NewActivityPubBusinessLogic creates a new ActivityPub business logic service
func NewActivityPubBusinessLogic(config *FederationConfig, logger *zap.Logger) *ActivityPubBusinessLogic {
	return &ActivityPubBusinessLogic{
		config: config,
		logger: logger,
	}
}

// Common ActivityPub Business Patterns
// Based on analysis of federation services

// 1. Actor URI Validation Pattern
// Found in: accounts, relationships, federation delivery

// ValidateActorURI validates ActivityPub actor URIs
func ValidateActorURI(actorURI string, rules ActivityPubValidationRules) error {
	if actorURI == "" {
		return ErrActorURIEmpty
	}

	parsedURI, err := url.Parse(actorURI)
	if err != nil {
		return fmt.Errorf("invalid actor URI format: %w", err)
	}

	// Check HTTPS requirement
	if rules.RequireHTTPS && parsedURI.Scheme != "https" {
		return fmt.Errorf("%w: %s", ErrActorURIMustUseHTTPS, actorURI)
	}

	// Check allowed/blocked domains
	domain := parsedURI.Host
	if len(rules.BlockedDomains) > 0 {
		for _, blocked := range rules.BlockedDomains {
			if strings.Contains(domain, blocked) {
				return fmt.Errorf("actor domain is blocked: %s", domain)
			}
		}
	}

	if len(rules.AllowedDomains) > 0 {
		allowed := false
		for _, allowedDomain := range rules.AllowedDomains {
			if strings.Contains(domain, allowedDomain) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("actor domain not in allowed list: %s", domain)
		}
	}

	return nil
}

// 2. Activity Type Validation Pattern
// Found in: inbox, outbox, activity processing

// ValidateActivityType validates ActivityPub activity types
func ValidateActivityType(activityType string, rules ActivityPubValidationRules) error {
	if activityType == "" {
		return ErrActivityTypeEmpty
	}

	if len(rules.ValidActivityTypes) > 0 {
		for _, validType := range rules.ValidActivityTypes {
			if activityType == validType {
				return nil
			}
		}
		return fmt.Errorf("unsupported activity type: %s", activityType)
	}

	// Default allowed activity types
	defaultTypes := []string{
		"Create", "Update", "Delete", "Follow", "Accept", "Reject",
		"Add", "Remove", "Like", "Announce", "Undo", "Block",
	}

	for _, allowedType := range defaultTypes {
		if activityType == allowedType {
			return nil
		}
	}

	return fmt.Errorf("unsupported activity type: %s", activityType)
}

// 3. Object ID Generation Pattern
// Found in: notes, actors, activities

// GenerateActivityPubID generates a standard ActivityPub ID
func GenerateActivityPubID(domain, objectType, localID string) string {
	return fmt.Sprintf("https://%s/%s/%s", domain, objectType, localID)
}

// GenerateActorID generates a standard actor ID
func GenerateActorID(domain, username string) string {
	return fmt.Sprintf("https://%s/users/%s", domain, username)
}

// GenerateObjectID generates a standard object ID
func GenerateObjectID(domain, objectType, objectID string) string {
	return fmt.Sprintf("https://%s/%s/%s", domain, objectType, objectID)
}

// 4. Activity Context Pattern
// Found in: all ActivityPub objects

// GetStandardActivityPubContext returns the standard ActivityPub JSON-LD context
func GetStandardActivityPubContext() []interface{} {
	return []interface{}{
		"https://www.w3.org/ns/activitystreams",
		"https://w3id.org/security/v1",
		map[string]interface{}{
			"manuallyApprovesFollowers": "as:manuallyApprovesFollowers",
			"toot":                      "http://joinmastodon.org/ns#",
			"featured":                  "toot:featured",
			"featuredTags":              "toot:featuredTags",
			"alsoKnownAs":               "as:alsoKnownAs",
			"movedTo":                   "as:movedTo",
			"schema":                    "http://schema.org#",
			"PropertyValue":             "schema:PropertyValue",
			"value":                     "schema:value",
			"discoverable":              "toot:discoverable",
			"Device":                    "toot:Device",
			"Ed25519Signature":          "toot:Ed25519Signature",
			"Ed25519Key":                "toot:Ed25519Key",
			"Curve25519Key":             "toot:Curve25519Key",
			"EncryptedMessage":          "toot:EncryptedMessage",
			"publicKeyBase64":           "toot:publicKeyBase64",
			"deviceId":                  "toot:deviceId",
			"claim":                     map[string]interface{}{"@type": "@id", "@id": "toot:claim"},
			"fingerprintKey":            map[string]interface{}{"@type": "@id", "@id": "toot:fingerprintKey"},
			"identityKey":               map[string]interface{}{"@type": "@id", "@id": "toot:identityKey"},
			"devices":                   map[string]interface{}{"@type": "@id", "@id": "toot:devices"},
			"messageFranking":           "toot:messageFranking",
			"messageType":               "toot:messageType",
			"cipherText":                "toot:cipherText",
			"suspended":                 "toot:suspended",
			"memorial":                  "toot:memorial",
			"indexable":                 "toot:indexable",
		},
	}
}

// 5. Audience Calculation Pattern
// Found in: notes, activities, delivery

// ActivityPubAudience represents the audience for an ActivityPub object
type ActivityPubAudience struct {
	To  []string `json:"to,omitempty"`
	CC  []string `json:"cc,omitempty"`
	BTo []string `json:"bto,omitempty"`
	BCC []string `json:"bcc,omitempty"`
}

// CalculateActivityPubAudience calculates the audience for an ActivityPub object
func CalculateActivityPubAudience(visibility VisibilityLevel, actorFollowers string, mentions []string) ActivityPubAudience {
	audience := ActivityPubAudience{
		To:  make([]string, 0),
		CC:  make([]string, 0),
		BTo: make([]string, 0),
		BCC: make([]string, 0),
	}

	publicCollection := "https://www.w3.org/ns/activitystreams#Public"

	switch visibility {
	case VisibilityPublic:
		audience.To = append(audience.To, publicCollection)
		if actorFollowers != "" {
			audience.CC = append(audience.CC, actorFollowers)
		}
		audience.CC = append(audience.CC, mentions...)

	case VisibilityUnlisted:
		if actorFollowers != "" {
			audience.To = append(audience.To, actorFollowers)
		}
		audience.To = append(audience.To, mentions...)
		audience.CC = append(audience.CC, publicCollection)

	case VisibilityPrivate:
		if actorFollowers != "" {
			audience.To = append(audience.To, actorFollowers)
		}
		audience.To = append(audience.To, mentions...)

	case VisibilityDirect:
		audience.To = append(audience.To, mentions...)
	}

	return audience
}

// 6. Federation Delivery Pattern
// Found in: federation delivery, outbox processing

// DeliveryTarget represents a federation delivery target
type DeliveryTarget struct {
	ActorID   string
	InboxURL  string
	SharedKey bool
}

// FederationDeliveryConfig configures federation delivery behavior
type FederationDeliveryConfig struct {
	MaxRetries     int
	RetryDelay     time.Duration
	RequestTimeout time.Duration
	MaxConcurrency int
	SignRequests   bool
}

// CalculateDeliveryTargets calculates delivery targets for an ActivityPub activity
func CalculateDeliveryTargets(_ context.Context, audience ActivityPubAudience, actorResolver func(string) (ActivityPubActor, error)) ([]DeliveryTarget, error) {
	targets := make([]DeliveryTarget, 0)
	processed := make(map[string]bool) // Avoid duplicates

	allRecipients := append(audience.To, audience.CC...)
	allRecipients = append(allRecipients, audience.BTo...)
	allRecipients = append(allRecipients, audience.BCC...)

	for _, recipient := range allRecipients {
		// Skip public collection
		if recipient == "https://www.w3.org/ns/activitystreams#Public" {
			continue
		}

		// Skip if already processed
		if processed[recipient] {
			continue
		}
		processed[recipient] = true

		// Resolve actor to get inbox
		actor, err := actorResolver(recipient)
		if err != nil {
			// Log error but continue with other recipients
			continue
		}

		if actor.GetInbox() != "" {
			targets = append(targets, DeliveryTarget{
				ActorID:   actor.GetID(),
				InboxURL:  actor.GetInbox(),
				SharedKey: false, // This would be determined by server capabilities
			})
		}
	}

	return targets, nil
}

// 7. Activity Wrapping Pattern
// Found in: outbox, activity creation

// WrapObjectInActivity wraps an object in an ActivityPub activity
func WrapObjectInActivity(activityType, actorID string, object interface{}, published time.Time) map[string]interface{} {
	activity := map[string]interface{}{
		"@context":  GetStandardActivityPubContext(),
		"type":      activityType,
		"actor":     actorID,
		"object":    object,
		"published": published.Format(time.RFC3339),
	}

	// Generate activity ID
	activityID := fmt.Sprintf("%s#activity", actorID)
	if objectMap, ok := object.(map[string]interface{}); ok {
		if id, exists := objectMap["id"]; exists {
			activityID = fmt.Sprintf("%s#%s", id, strings.ToLower(activityType))
		}
	}
	activity["id"] = activityID

	return activity
}

// 8. Signature Verification Pattern
// Found in: inbox processing, authentication

// HTTPSignatureInfo represents HTTP signature information
type HTTPSignatureInfo struct {
	KeyID     string
	Algorithm string
	Headers   []string
	Signature string
}

// ParseHTTPSignature parses an HTTP signature header
func ParseHTTPSignature(signatureHeader string) (*HTTPSignatureInfo, error) {
	if signatureHeader == "" {
		return nil, ErrSignatureHeaderEmpty
	}

	info := &HTTPSignatureInfo{}

	// Parse signature components
	// This is a simplified parser - in reality you'd need a more robust implementation
	parts := strings.Split(signatureHeader, ",")
	for _, part := range parts {
		keyValue := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(keyValue) != 2 {
			continue
		}

		key := keyValue[0]
		value := strings.Trim(keyValue[1], "\"")

		switch key {
		case "keyId":
			info.KeyID = value
		case "algorithm":
			info.Algorithm = value
		case "headers":
			info.Headers = strings.Split(value, " ")
		case "signature":
			info.Signature = value
		}
	}

	if info.KeyID == "" || info.Signature == "" {
		return nil, ErrInvalidSignature
	}

	return info, nil
}

// 9. Collection Management Pattern
// Found in: followers, following, outbox collections

// ActivityPubCollection represents a paginated ActivityPub collection
type ActivityPubCollection struct {
	Context      interface{}   `json:"@context"`
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	TotalItems   int           `json:"totalItems"`
	First        string        `json:"first,omitempty"`
	Last         string        `json:"last,omitempty"`
	Items        []interface{} `json:"items,omitempty"`
	OrderedItems []interface{} `json:"orderedItems,omitempty"`
}

// ActivityPubCollectionPage represents a page in an ActivityPub collection
type ActivityPubCollectionPage struct {
	Context      interface{}   `json:"@context"`
	ID           string        `json:"id"`
	Type         string        `json:"type"`
	PartOf       string        `json:"partOf"`
	Next         string        `json:"next,omitempty"`
	Prev         string        `json:"prev,omitempty"`
	Items        []interface{} `json:"items,omitempty"`
	OrderedItems []interface{} `json:"orderedItems,omitempty"`
}

// CreateActivityPubCollection creates a standard ActivityPub collection
func CreateActivityPubCollection(id, collectionType string, totalItems int, firstPage string) ActivityPubCollection {
	collection := ActivityPubCollection{
		Context:    GetStandardActivityPubContext(),
		ID:         id,
		Type:       collectionType,
		TotalItems: totalItems,
	}

	if firstPage != "" {
		collection.First = firstPage
	}

	return collection
}

// CreateActivityPubCollectionPage creates a standard ActivityPub collection page
func CreateActivityPubCollectionPage(id, pageType, partOf string, items []interface{}, nextPage, prevPage string) ActivityPubCollectionPage {
	page := ActivityPubCollectionPage{
		Context: GetStandardActivityPubContext(),
		ID:      id,
		Type:    pageType,
		PartOf:  partOf,
		Next:    nextPage,
		Prev:    prevPage,
	}

	if pageType == "OrderedCollectionPage" {
		page.OrderedItems = items
	} else {
		page.Items = items
	}

	return page
}

// 10. Error Handling Pattern for Federation
// Found in: federation delivery, inbox processing

// ActivityPubError represents an ActivityPub-specific error
type ActivityPubError struct {
	Type       string        `json:"type"`
	Message    string        `json:"message"`
	ActorID    string        `json:"actor,omitempty"`
	ObjectID   string        `json:"object,omitempty"`
	Temporary  bool          `json:"temporary"`
	RetryAfter time.Duration `json:"retry_after,omitempty"`
}

// Error implements the error interface
func (e ActivityPubError) Error() string {
	return fmt.Sprintf("activitypub error (%s): %s", e.Type, e.Message)
}

// IsTemporary returns whether the error is temporary and should be retried
func (e ActivityPubError) IsTemporary() bool {
	return e.Temporary
}

// NewActivityPubError creates a new ActivityPub error
func NewActivityPubError(errorType, message string, temporary bool) ActivityPubError {
	return ActivityPubError{
		Type:      errorType,
		Message:   message,
		Temporary: temporary,
	}
}

// MapActivityPubError maps common federation errors to ActivityPub error types
func MapActivityPubError(err error, actorID, objectID string) ActivityPubError {
	if err == nil {
		return ActivityPubError{}
	}

	// Map common error patterns
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "timeout"), strings.Contains(errMsg, "deadline"):
		return ActivityPubError{
			Type:       "timeout",
			Message:    "request timeout",
			ActorID:    actorID,
			ObjectID:   objectID,
			Temporary:  true,
			RetryAfter: 5 * time.Minute,
		}
	case strings.Contains(errMsg, "connection"), strings.Contains(errMsg, "network"):
		return ActivityPubError{
			Type:       "network",
			Message:    "network connection error",
			ActorID:    actorID,
			ObjectID:   objectID,
			Temporary:  true,
			RetryAfter: 2 * time.Minute,
		}
	case strings.Contains(errMsg, "401"), strings.Contains(errMsg, "403"):
		return ActivityPubError{
			Type:      "auth",
			Message:   "authentication or authorization failed",
			ActorID:   actorID,
			ObjectID:  objectID,
			Temporary: false,
		}
	case strings.Contains(errMsg, "404"):
		return ActivityPubError{
			Type:      "not_found",
			Message:   "resource not found",
			ActorID:   actorID,
			ObjectID:  objectID,
			Temporary: false,
		}
	case strings.Contains(errMsg, "5"):
		return ActivityPubError{
			Type:       "server",
			Message:    "server error",
			ActorID:    actorID,
			ObjectID:   objectID,
			Temporary:  true,
			RetryAfter: 10 * time.Minute,
		}
	default:
		return ActivityPubError{
			Type:      "unknown",
			Message:   errMsg,
			ActorID:   actorID,
			ObjectID:  objectID,
			Temporary: true,
		}
	}
}

// Business Logic Execution for ActivityPub Operations

// ExecuteActivityPubOperation executes a federation operation with standard error handling and retry logic
func (ap *ActivityPubBusinessLogic) ExecuteActivityPubOperation(ctx context.Context, operation ActivityPubOperation) error {
	if err := operation.Validate(ctx); err != nil {
		return fmt.Errorf("activitypub operation validation failed: %w", err)
	}

	maxRetries := ap.getMaxRetries()
	return ap.executeWithRetry(ctx, operation, maxRetries)
}

// getMaxRetries returns the configured max retries or default value
func (ap *ActivityPubBusinessLogic) getMaxRetries() int {
	if ap.config.MaxRetries == 0 {
		return 3
	}
	return ap.config.MaxRetries
}

// executeWithRetry executes the operation with retry logic for temporary failures
func (ap *ActivityPubBusinessLogic) executeWithRetry(ctx context.Context, operation ActivityPubOperation, maxRetries int) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := operation.Execute(ctx)
		if err == nil {
			ap.recordSuccessMetrics(ctx, operation, attempt)
			return nil
		}

		if shouldRetry := ap.handleExecutionError(ctx, operation, err, attempt, maxRetries); !shouldRetry {
			return err
		}
		lastErr = err
	}

	ap.recordMaxRetriesExceededMetrics(ctx, operation, maxRetries)
	return fmt.Errorf("activitypub operation failed after %d retries: %w", maxRetries, lastErr)
}

// recordSuccessMetrics records success metrics with error logging
func (ap *ActivityPubBusinessLogic) recordSuccessMetrics(ctx context.Context, operation ActivityPubOperation, attempt int) {
	if err := operation.RecordMetrics(ctx, "success", attempt); err != nil {
		zap.L().Error("Failed to record success metrics", zap.Error(err))
	}
}

// recordMaxRetriesExceededMetrics records max retries exceeded metrics with error logging
func (ap *ActivityPubBusinessLogic) recordMaxRetriesExceededMetrics(ctx context.Context, operation ActivityPubOperation, maxRetries int) {
	if err := operation.RecordMetrics(ctx, "max_retries_exceeded", maxRetries); err != nil {
		zap.L().Error("Failed to record max retries exceeded metrics", zap.Error(err))
	}
}

// handleExecutionError handles errors from operation execution and determines if retry should happen
func (ap *ActivityPubBusinessLogic) handleExecutionError(ctx context.Context, operation ActivityPubOperation, err error, attempt, maxRetries int) bool {
	if fedErr, ok := err.(ActivityPubError); ok {
		return ap.handleFederationError(ctx, operation, fedErr, attempt, maxRetries)
	}

	// Non-federation error - don't retry
	ap.recordErrorMetrics(ctx, operation, attempt)
	return false
}

// handleFederationError handles ActivityPub federation errors with retry logic
func (ap *ActivityPubBusinessLogic) handleFederationError(ctx context.Context, operation ActivityPubOperation, fedErr ActivityPubError, attempt, maxRetries int) bool {
	if !fedErr.IsTemporary() || attempt == maxRetries {
		ap.recordFailureMetrics(ctx, operation, attempt)
		return false
	}

	retryDelay := ap.calculateRetryDelay(fedErr, attempt)
	ap.logRetryAttempt(fedErr, attempt, retryDelay)

	return ap.waitForRetry(ctx, retryDelay)
}

// recordErrorMetrics records error metrics with logging
func (ap *ActivityPubBusinessLogic) recordErrorMetrics(ctx context.Context, operation ActivityPubOperation, attempt int) {
	if err := operation.RecordMetrics(ctx, "error", attempt); err != nil {
		zap.L().Error("Failed to record error metrics", zap.Error(err))
	}
}

// recordFailureMetrics records failure metrics with logging
func (ap *ActivityPubBusinessLogic) recordFailureMetrics(ctx context.Context, operation ActivityPubOperation, attempt int) {
	if metricErr := operation.RecordMetrics(ctx, "failure", attempt); metricErr != nil {
		zap.L().Error("Failed to record failure metrics", zap.Error(metricErr))
	}
}

// calculateRetryDelay calculates the delay before retrying based on configuration and error
func (ap *ActivityPubBusinessLogic) calculateRetryDelay(fedErr ActivityPubError, attempt int) time.Duration {
	retryDelay := ap.config.RetryDelay
	if retryDelay == 0 {
		retryDelay = time.Duration(attempt+1) * 30 * time.Second
	}
	if fedErr.RetryAfter > 0 {
		retryDelay = fedErr.RetryAfter
	}
	return retryDelay
}

// logRetryAttempt logs retry attempt information
func (ap *ActivityPubBusinessLogic) logRetryAttempt(err error, attempt int, retryDelay time.Duration) {
	ap.logger.Warn("ActivityPub operation failed, retrying",
		zap.Error(err),
		zap.Int("attempt", attempt+1),
		zap.Duration("retry_after", retryDelay))
}

// waitForRetry waits for the specified delay or context cancellation
func (ap *ActivityPubBusinessLogic) waitForRetry(ctx context.Context, retryDelay time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(retryDelay):
		return true
	}
}

// ActivityPubOperation defines the interface for ActivityPub operations
type ActivityPubOperation interface {
	Validate(ctx context.Context) error
	Execute(ctx context.Context) error
	RecordMetrics(ctx context.Context, outcome string, attempts int) error
}
