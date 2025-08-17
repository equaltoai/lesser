package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/google/uuid"
)

// DLQMessage represents a failed message captured from a dead letter queue
type DLQMessage struct {
	// Primary key - service and date for partitioning
	PK string `dynamorm:"pk" json:"pk"` // Format: "DLQ#service#date"
	SK string `dynamorm:"sk" json:"sk"` // Format: "MSG#timestamp#messageId"

	// GSI1 - Error type analysis
	GSI1PK string `dynamorm:"index:error-index,pk" json:"gsi1_pk"` // Format: "DLQ_ERROR#errorType"
	GSI1SK string `dynamorm:"index:error-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{service}#{messageId}"

	// GSI2 - Retry analysis and reprocessing
	GSI2PK string `dynamorm:"index:retry-index,pk" json:"gsi2_pk"` // Format: "DLQ_RETRY#{service}#{status}"
	GSI2SK string `dynamorm:"index:retry-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{messageId}"

	// GSI3 - Service-wide analysis
	GSI3PK string `dynamorm:"index:service-index,pk" json:"gsi3_pk"` // Format: "DLQ_SERVICE#{service}"
	GSI3SK string `dynamorm:"index:service-index,sk" json:"gsi3_sk"` // Format: "{timestamp}#{errorType}#{messageId}"

	// Core message data
	ID                string `json:"id"`                  // Unique DLQ message ID
	OriginalMessageID string `json:"original_message_id"` // Original SQS message ID
	Service           string `json:"service"`             // Service that failed (e.g., "notification-processor")
	QueueName         string `json:"queue_name"`          // Name of the source DLQ
	SourceQueue       string `json:"source_queue"`        // Original queue name

	// Message content
	MessageBody       string                 `json:"message_body"`                 // Original message body
	MessageAttributes map[string]string      `json:"message_attributes,omitempty"` // SQS message attributes
	MessageMetadata   map[string]interface{} `json:"message_metadata,omitempty"`   // Additional metadata

	// Error information
	ErrorType     string `json:"error_type"`            // Categorized error type
	ErrorMessage  string `json:"error_message"`         // Full error message
	ErrorStack    string `json:"error_stack,omitempty"` // Stack trace if available
	FailureReason string `json:"failure_reason"`        // Human-readable failure reason
	IsPermanent   bool   `json:"is_permanent"`          // Whether this is a permanent failure

	// Processing context
	FunctionName    string `json:"function_name"`              // Lambda function that failed
	FunctionVersion string `json:"function_version,omitempty"` // Lambda function version
	LogGroup        string `json:"log_group,omitempty"`        // CloudWatch log group
	LogStream       string `json:"log_stream,omitempty"`       // CloudWatch log stream
	RequestID       string `json:"request_id,omitempty"`       // AWS request ID

	// Retry information
	OriginalRetryCount   int        `json:"original_retry_count"`    // How many times original message was retried
	ReprocessingCount    int        `json:"reprocessing_count"`      // How many times we've tried to reprocess
	MaxReprocessAttempts int        `json:"max_reprocess_attempts"`  // Maximum reprocessing attempts
	NextRetryAt          *time.Time `json:"next_retry_at,omitempty"` // When to retry next
	Status               string     `json:"status"`                  // "new", "reprocessing", "failed", "resolved", "abandoned"

	// Analysis metadata
	SimilarityHash string   `json:"similarity_hash"`           // Hash for grouping similar errors
	Tags           []string `json:"tags,omitempty"`            // Tags for categorization
	Priority       string   `json:"priority"`                  // "low", "medium", "high", "critical"
	BusinessImpact string   `json:"business_impact,omitempty"` // Impact assessment

	// Cost tracking
	ProcessingCostMicroCents   int64 `json:"processing_cost_micro_cents"`   // Cost of processing attempts
	ReprocessingCostMicroCents int64 `json:"reprocessing_cost_micro_cents"` // Cost of reprocessing

	// Timestamps
	FirstSeenAt     time.Time  `json:"first_seen_at"`               // When first captured in DLQ
	LastProcessedAt *time.Time `json:"last_processed_at,omitempty"` // Last processing attempt
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`       // When successfully reprocessed
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`

	// TTL for automatic cleanup (90 days for DLQ messages)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"` // Unix timestamp

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// DLQMessageBuilder helps create DLQ messages with proper defaults
type DLQMessageBuilder struct {
	message *DLQMessage
}

// TableName returns the DynamoDB table name for the DLQMessage model
func (DLQMessage) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (d *DLQMessage) BeforeCreate() error {
	now := time.Now()
	d.CreatedAt = now
	d.UpdatedAt = now
	d.FirstSeenAt = now

	// Generate ID if not provided
	if err := common.ValidateRequiredParam("ID", d.ID); err != nil {
		d.ID = uuid.New().String()
	}

	// Set defaults
	if err := common.ValidateRequiredParam("Status", d.Status); err != nil {
		d.Status = "new"
	}
	if err := common.ValidateRequiredParam("Priority", d.Priority); err != nil {
		d.Priority = string(AdvancedSeverityMedium)
	}
	if d.MaxReprocessAttempts == 0 {
		d.MaxReprocessAttempts = 3
	}

	// Set expiry to 90 days for DLQ messages
	d.ExpiresAt = now.Add(90 * 24 * time.Hour).Unix()

	// Generate similarity hash for error grouping
	if err := common.ValidateRequiredParam("SimilarityHash", d.SimilarityHash); err != nil {
		d.SimilarityHash = d.generateSimilarityHash()
	}

	// Set up primary key
	dateStr := d.FirstSeenAt.Format(common.CompactDateFormat)
	d.PK = fmt.Sprintf("DLQ#%s#%s", d.Service, dateStr)
	timestamp := d.FirstSeenAt.Format(common.CompactTimeFormat)
	d.SK = fmt.Sprintf("MSG#%s#%s", timestamp, d.OriginalMessageID)

	// Set up GSI keys
	d.setupGSIKeys()

	return d.Validate()
}

// BeforeUpdate sets up the model before update
func (d *DLQMessage) BeforeUpdate() error {
	d.UpdatedAt = time.Now()

	// Update GSI keys in case status or other indexed fields changed
	d.setupGSIKeys()

	return d.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (d *DLQMessage) setupGSIKeys() {
	timestampStr := d.FirstSeenAt.Format(time.RFC3339)

	// GSI1 - Error type analysis
	d.GSI1PK = "DLQ_ERROR#" + d.ErrorType
	d.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, d.Service, d.ID)

	// GSI2 - Retry analysis and reprocessing
	d.GSI2PK = fmt.Sprintf("DLQ_RETRY#%s#%s", d.Service, d.Status)
	d.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, d.ID)

	// GSI3 - Service-wide analysis
	d.GSI3PK = "DLQ_SERVICE#" + d.Service
	d.GSI3SK = fmt.Sprintf("%s#%s#%s", timestampStr, d.ErrorType, d.ID)
}

// Validate performs validation on the DLQMessage using centralized validation
func (d *DLQMessage) Validate() error {
	if err := common.ValidateRequiredParam("ID", strings.TrimSpace(d.ID)); err != nil {
		return fmt.Errorf("ID is required")
	}
	if err := common.ValidateRequiredParam("OriginalMessageID", strings.TrimSpace(d.OriginalMessageID)); err != nil {
		return fmt.Errorf("OriginalMessageID is required")
	}
	if err := common.ValidateRequiredParam("Service", strings.TrimSpace(d.Service)); err != nil {
		return fmt.Errorf("service is required")
	}
	if err := common.ValidateRequiredParam("MessageBody", strings.TrimSpace(d.MessageBody)); err != nil {
		return fmt.Errorf("MessageBody is required")
	}
	if err := common.ValidateRequiredParam("ErrorType", strings.TrimSpace(d.ErrorType)); err != nil {
		return fmt.Errorf("ErrorType is required")
	}
	if err := common.ValidateRequiredParam("ErrorMessage", strings.TrimSpace(d.ErrorMessage)); err != nil {
		return fmt.Errorf("ErrorMessage is required")
	}
	if !isValidDLQStatus(d.Status) {
		return fmt.Errorf("invalid status: %s", d.Status)
	}
	if !isValidPriority(d.Priority) {
		return fmt.Errorf("invalid priority: %s", d.Priority)
	}

	return nil
}

// generateSimilarityHash creates a hash for grouping similar errors
func (d *DLQMessage) generateSimilarityHash() string {
	// Create a hash based on service, error type, and sanitized error message
	sanitizedError := sanitizeErrorForGrouping(d.ErrorMessage)
	hashInput := fmt.Sprintf("%s:%s:%s", d.Service, d.ErrorType, sanitizedError)

	// Use a simple hash for grouping (in production, use a proper hash function)
	hash := 0
	for _, char := range hashInput {
		hash = hash*31 + int(char)
	}

	return fmt.Sprintf("%x", hash)
}

// sanitizeErrorForGrouping removes specific details to group similar errors
func sanitizeErrorForGrouping(errorMsg string) string {
	// Remove timestamps, IDs, and other variable content
	errorMsg = strings.ReplaceAll(errorMsg, "request-", "request-X")
	errorMsg = strings.ReplaceAll(errorMsg, "user-", "user-X")
	errorMsg = strings.ReplaceAll(errorMsg, "status-", "status-X")

	// Remove numbers and UUIDs
	words := strings.Fields(errorMsg)
	var sanitized []string
	for _, word := range words {
		if len(word) > 8 && (strings.Contains(word, "-") || isNumeric(word)) {
			sanitized = append(sanitized, "X")
		} else {
			sanitized = append(sanitized, word)
		}
	}

	return strings.Join(sanitized, " ")
}

// isNumeric checks if a string is numeric
func isNumeric(s string) bool {
	for _, char := range s {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

// Status management methods

// MarkForReprocessing marks the message for reprocessing
func (d *DLQMessage) MarkForReprocessing() {
	d.Status = "reprocessing"
	d.ReprocessingCount++

	// Set next retry with exponential backoff
	backoffMinutes := 1 << d.ReprocessingCount // 2^attempts minutes
	if backoffMinutes > 60 {
		backoffMinutes = 60 // Cap at 1 hour
	}
	nextRetry := time.Now().Add(time.Duration(backoffMinutes) * time.Minute)
	d.NextRetryAt = &nextRetry
}

// MarkResolved marks the message as successfully reprocessed
func (d *DLQMessage) MarkResolved() {
	d.Status = "resolved"
	now := time.Now()
	d.ResolvedAt = &now
	d.NextRetryAt = nil
}

// MarkFailed marks the message as failed during reprocessing
func (d *DLQMessage) MarkFailed(errorMsg string) {
	d.Status = DeliveryStatusFailed
	d.ErrorMessage = errorMsg
	now := time.Now()
	d.LastProcessedAt = &now
}

// MarkAbandoned marks the message as abandoned (too many failures)
func (d *DLQMessage) MarkAbandoned() {
	d.Status = "abandoned"
	d.NextRetryAt = nil
}

// CanReprocess determines if the message can be reprocessed
func (d *DLQMessage) CanReprocess() bool {
	if d.Status == "resolved" || d.Status == "abandoned" {
		return false
	}
	if d.IsPermanent {
		return false
	}
	if d.ReprocessingCount >= d.MaxReprocessAttempts {
		return false
	}
	if d.NextRetryAt != nil && time.Now().Before(*d.NextRetryAt) {
		return false
	}
	return true
}

// ShouldAbandon determines if the message should be abandoned
func (d *DLQMessage) ShouldAbandon() bool {
	return d.ReprocessingCount >= d.MaxReprocessAttempts || d.IsPermanent
}

// AddTag adds a tag to the message
func (d *DLQMessage) AddTag(tag string) {
	if d.Tags == nil {
		d.Tags = make([]string, 0)
	}

	// Check if tag already exists
	for _, existingTag := range d.Tags {
		if existingTag == tag {
			return
		}
	}

	d.Tags = append(d.Tags, tag)
}

// SetMetadata sets a metadata field
func (d *DLQMessage) SetMetadata(key string, value interface{}) {
	if d.MessageMetadata == nil {
		d.MessageMetadata = make(map[string]interface{})
	}
	d.MessageMetadata[key] = value
}

// GetMetadata gets a metadata field
func (d *DLQMessage) GetMetadata(key string) (interface{}, bool) {
	if d.MessageMetadata == nil {
		return nil, false
	}
	value, exists := d.MessageMetadata[key]
	return value, exists
}

// UpdateCosts updates the processing costs
func (d *DLQMessage) UpdateCosts(processingCost, reprocessingCost int64) {
	d.ProcessingCostMicroCents += processingCost
	d.ReprocessingCostMicroCents += reprocessingCost
}

// GetTotalCost returns the total cost for this message
func (d *DLQMessage) GetTotalCost() int64 {
	return d.ProcessingCostMicroCents + d.ReprocessingCostMicroCents
}

// isValidDLQStatus checks if the DLQ status is valid
func isValidDLQStatus(status string) bool {
	validStatuses := map[string]bool{
		"new":          true,
		"reprocessing": true,
		"failed":       true,
		"resolved":     true,
		"abandoned":    true,
	}
	return validStatuses[strings.ToLower(status)]
}

// isValidPriority checks if the priority is valid
func isValidPriority(priority string) bool {
	validPriorities := map[string]bool{
		"low":      true,
		"medium":   true,
		"high":     true,
		"critical": true,
	}
	return validPriorities[strings.ToLower(priority)]
}

// NewDLQMessageBuilder creates a new DLQ message builder
func NewDLQMessageBuilder() *DLQMessageBuilder {
	return &DLQMessageBuilder{
		message: &DLQMessage{},
	}
}

// ForService sets the service name
func (b *DLQMessageBuilder) ForService(service string) *DLQMessageBuilder {
	b.message.Service = service
	return b
}

// WithOriginalMessage sets the original message details
func (b *DLQMessageBuilder) WithOriginalMessage(messageID, body string) *DLQMessageBuilder {
	b.message.OriginalMessageID = messageID
	b.message.MessageBody = body
	return b
}

// WithQueue sets the queue information
func (b *DLQMessageBuilder) WithQueue(queueName, sourceQueue string) *DLQMessageBuilder {
	b.message.QueueName = queueName
	b.message.SourceQueue = sourceQueue
	return b
}

// WithError sets the error information
func (b *DLQMessageBuilder) WithError(errorType, errorMessage, errorStack string) *DLQMessageBuilder {
	b.message.ErrorType = errorType
	b.message.ErrorMessage = errorMessage
	b.message.ErrorStack = errorStack
	return b
}

// WithFailureReason sets a human-readable failure reason
func (b *DLQMessageBuilder) WithFailureReason(reason string) *DLQMessageBuilder {
	b.message.FailureReason = reason
	return b
}

// WithPriority sets the priority level
func (b *DLQMessageBuilder) WithPriority(priority string) *DLQMessageBuilder {
	b.message.Priority = priority
	return b
}

// MarkAsPermanent marks the failure as permanent
func (b *DLQMessageBuilder) MarkAsPermanent() *DLQMessageBuilder {
	b.message.IsPermanent = true
	return b
}

// WithContext sets the processing context
func (b *DLQMessageBuilder) WithContext(functionName, logGroup, logStream, requestID string) *DLQMessageBuilder {
	b.message.FunctionName = functionName
	b.message.LogGroup = logGroup
	b.message.LogStream = logStream
	b.message.RequestID = requestID
	return b
}

// WithRetryInfo sets retry information
func (b *DLQMessageBuilder) WithRetryInfo(originalRetryCount, maxReprocessAttempts int) *DLQMessageBuilder {
	b.message.OriginalRetryCount = originalRetryCount
	b.message.MaxReprocessAttempts = maxReprocessAttempts
	return b
}

// WithBusinessImpact sets the business impact assessment
func (b *DLQMessageBuilder) WithBusinessImpact(impact string) *DLQMessageBuilder {
	b.message.BusinessImpact = impact
	return b
}

// WithTags adds tags to the message
func (b *DLQMessageBuilder) WithTags(tags ...string) *DLQMessageBuilder {
	for _, tag := range tags {
		b.message.AddTag(tag)
	}
	return b
}

// WithAttributes sets message attributes
func (b *DLQMessageBuilder) WithAttributes(attributes map[string]string) *DLQMessageBuilder {
	b.message.MessageAttributes = attributes
	return b
}

// WithMetadata adds metadata
func (b *DLQMessageBuilder) WithMetadata(metadata map[string]interface{}) *DLQMessageBuilder {
	if b.message.MessageMetadata == nil {
		b.message.MessageMetadata = make(map[string]interface{})
	}
	for k, v := range metadata {
		b.message.MessageMetadata[k] = v
	}
	return b
}

// Build creates the DLQ message
func (b *DLQMessageBuilder) Build() *DLQMessage {
	return b.message
}

// UpdateKeys updates the GSI keys for this DLQ message (required by DynamORM)
func (d *DLQMessage) UpdateKeys() {
	d.setupGSIKeys()
}

// Convenience functions for creating DLQ messages

// NewValidationErrorDLQ creates a DLQ message for validation errors
func NewValidationErrorDLQ(service, messageID, body, errorMsg string) *DLQMessage {
	return NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(messageID, body).
		WithError("validation_error", errorMsg, "").
		WithFailureReason("Message failed validation").
		WithPriority("medium").
		MarkAsPermanent().
		Build()
}

// NewTransientErrorDLQ creates a DLQ message for transient errors
func NewTransientErrorDLQ(service, messageID, body, errorMsg string) *DLQMessage {
	return NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(messageID, body).
		WithError("transient_error", errorMsg, "").
		WithFailureReason("Temporary service error").
		WithPriority("high").
		WithRetryInfo(0, 5). // Allow more retries for transient errors
		Build()
}

// NewDependencyErrorDLQ creates a DLQ message for dependency errors
func NewDependencyErrorDLQ(service, messageID, body, errorMsg string) *DLQMessage {
	return NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(messageID, body).
		WithError("dependency_error", errorMsg, "").
		WithFailureReason("External dependency unavailable").
		WithPriority("high").
		WithRetryInfo(0, 3).
		Build()
}

// NewProcessingErrorDLQ creates a DLQ message for processing errors
func NewProcessingErrorDLQ(service, messageID, body, errorMsg string) *DLQMessage {
	return NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(messageID, body).
		WithError("processing_error", errorMsg, "").
		WithFailureReason("Message processing failed").
		WithPriority("medium").
		Build()
}
