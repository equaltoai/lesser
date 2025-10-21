package models

import (
	"os"
	"time"
)

// RateLimitRecord is compatible with both DynamORM and the Limited library
type RateLimitRecord struct {
	ExpiresAt  time.Time `dynamorm:"ttl" json:"expires_at"`
	CreatedAt  time.Time `dynamorm:"created_at" json:"created_at"`
	UpdatedAt  time.Time `dynamorm:"updated_at" json:"updated_at"`
	Identifier string    `dynamorm:"pk" json:"identifier"`
	WindowTime string    `dynamorm:"sk" json:"window_time"`
	IPAddress  string    `dynamorm:"index:gsi-ip,pk" json:"ip_address,omitempty"`
	UserID     string    `dynamorm:"index:gsi-user,pk" json:"user_id,omitempty"`
	TenantID   string    `dynamorm:"index:gsi-tenant,pk" json:"tenant_id,omitempty"`
	BucketKey  string    `dynamorm:"index:gsi-bucket,pk" json:"bucket_key"`
	Count      int       `json:"count"`
}

// TableName returns the DynamoDB table name from environment
func (r *RateLimitRecord) TableName() string {
	return os.Getenv("RATE_LIMIT_TABLE_NAME")
}

// NewRateLimitRecord creates a new rate limit record with defaults
func NewRateLimitRecord(identifier string, window time.Time) *RateLimitRecord {
	return &RateLimitRecord{
		Identifier: identifier,
		WindowTime: window.Format(time.RFC3339),
		Count:      0,
		ExpiresAt:  window.Add(2 * time.Hour), // 2 hour buffer after window
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
}

// IncrementCount atomically increments the count
func (r *RateLimitRecord) IncrementCount() {
	r.Count++
	r.UpdatedAt = time.Now()
}

// IsExpired checks if the record has expired
func (r *RateLimitRecord) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// SetIdentifierMetadata sets IP, UserID, and TenantID based on the identifier type
func (r *RateLimitRecord) SetIdentifierMetadata(identifierType string, value string) {
	switch identifierType {
	case "ip":
		r.IPAddress = value
	case "user":
		r.UserID = value
	case "tenant":
		r.TenantID = value
	}
}

// RateLimitType constants for different rate limiting strategies
const (
	RateLimitByIP     = "ip"
	RateLimitByUser   = "user"
	RateLimitByTenant = "tenant"
	RateLimitByCustom = "custom"
)
