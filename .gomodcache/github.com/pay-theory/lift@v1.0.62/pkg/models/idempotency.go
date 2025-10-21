package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"time"
)

// IdempotencyRecord stores idempotent request data
type IdempotencyRecord struct {
	LockedUntil    time.Time `json:"locked_until,omitempty"`
	CompletedAt    time.Time `json:"completed_at,omitempty"`
	UpdatedAt      time.Time `dynamorm:"updated_at" json:"updated_at"`
	CreatedAt      time.Time `dynamorm:"created_at" json:"created_at"`
	ExpiresAt      time.Time `dynamorm:"ttl" json:"expires_at"`
	Timestamp      time.Time `dynamorm:"index:gsi-timestamp,pk" json:"timestamp"`
	Status         string    `dynamorm:"index:gsi-status,pk" json:"status"`
	RequestBody    string    `dynamorm:"json" json:"request_body"`
	Response       string    `dynamorm:"json" json:"response"`
	LockToken      string    `json:"lock_token,omitempty"`
	RequestHash    string    `json:"request_hash"`
	IdempotencyKey string    `dynamorm:"pk" json:"idempotency_key"`
	TenantID       string    `dynamorm:"index:gsi-tenant,pk" json:"tenant_id,omitempty"`
	FunctionName   string    `dynamorm:"index:gsi-function,pk" json:"function_name"`
	SK             string    `dynamorm:"sk" json:"sk" default:"IDEMPOTENCY"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	StatusCode     int       `json:"status_code"`
	RetryCount     int       `json:"retry_count,omitempty"`
}

// TableName returns the DynamoDB table name from environment
func (i *IdempotencyRecord) TableName() string {
	return os.Getenv("IDEMPOTENCY_TABLE_NAME")
}

// Status constants
const (
	IdempotencyStatusPending    = "PENDING"
	IdempotencyStatusProcessing = "PROCESSING"
	IdempotencyStatusCompleted  = "COMPLETED"
	IdempotencyStatusFailed     = "FAILED"
)

// NewIdempotencyRecord creates a new idempotency record
func NewIdempotencyRecord(key string, functionName string) *IdempotencyRecord {
	now := time.Now()
	return &IdempotencyRecord{
		IdempotencyKey: key,
		SK:             "IDEMPOTENCY",
		FunctionName:   functionName,
		Status:         IdempotencyStatusPending,
		Timestamp:      now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ExpiresAt:      now.Add(24 * time.Hour), // 24 hour default TTL
	}
}

// HashRequest creates a SHA256 hash of the request for comparison
func HashRequest(request interface{}) (string, error) {
	data, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

// SetRequest stores the request body and hash
func (i *IdempotencyRecord) SetRequest(request interface{}) error {
	// Calculate hash
	hash, err := HashRequest(request)
	if err != nil {
		return err
	}
	i.RequestHash = hash

	// Store request body
	data, err := json.Marshal(request)
	if err != nil {
		return err
	}
	i.RequestBody = string(data)

	return nil
}

// SetResponse stores the response and marks as completed
func (i *IdempotencyRecord) SetResponse(response interface{}, statusCode int) error {
	data, err := json.Marshal(response)
	if err != nil {
		return err
	}

	i.Response = string(data)
	i.StatusCode = statusCode
	i.Status = IdempotencyStatusCompleted
	i.CompletedAt = time.Now()
	i.UpdatedAt = time.Now()

	return nil
}

// SetError marks the record as failed with an error message
func (i *IdempotencyRecord) SetError(err error) {
	i.Status = IdempotencyStatusFailed
	i.ErrorMessage = err.Error()
	i.UpdatedAt = time.Now()
	i.RetryCount++
}

// IsLocked checks if the record is currently locked for processing
func (i *IdempotencyRecord) IsLocked() bool {
	return i.Status == IdempotencyStatusProcessing && time.Now().Before(i.LockedUntil)
}

// CanRetry checks if the record can be retried
func (i *IdempotencyRecord) CanRetry() bool {
	return i.Status == IdempotencyStatusFailed && i.RetryCount < 3
}

// GetResponse unmarshals the stored response
func (i *IdempotencyRecord) GetResponse(target interface{}) error {
	if i.Response == "" {
		return nil
	}
	return json.Unmarshal([]byte(i.Response), target)
}

// GetRequest unmarshals the stored request
func (i *IdempotencyRecord) GetRequest(target interface{}) error {
	if i.RequestBody == "" {
		return nil
	}
	return json.Unmarshal([]byte(i.RequestBody), target)
}
