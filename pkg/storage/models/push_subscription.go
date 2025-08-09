package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// PushSubscriptionAlerts represents which events trigger push notifications
type PushSubscriptionAlerts struct {
	Follow        bool `json:"follow"`
	Favourite     bool `json:"favourite"`
	Reblog        bool `json:"reblog"`
	Mention       bool `json:"mention"`
	Poll          bool `json:"poll"`
	FollowRequest bool `json:"follow_request"`
	Status        bool `json:"status"`
	Update        bool `json:"update"`
	AdminSignUp   bool `json:"admin_sign_up"`
	AdminReport   bool `json:"admin_report"`
}

// PushSubscription represents a push subscription stored in DynamoDB
type PushSubscription struct {
	// Primary keys
	PK string `dynamorm:"pk" json:"pk"` // PUSH#username
	SK string `dynamorm:"sk" json:"sk"` // SUB#subscriptionID

	// GSI for endpoint lookup (to prevent duplicates)
	GSI1PK string `dynamorm:"index:endpoint-index,pk" json:"gsi1_pk"` // PUSH_ENDPOINT#endpoint_hash
	GSI1SK string `dynamorm:"index:endpoint-index,sk" json:"gsi1_sk"` // username

	// Core subscription data
	ID        string                 `json:"id"`
	Username  string                 `json:"username"`
	Endpoint  string                 `json:"endpoint"`
	P256dh    string                 `json:"p256dh"` // Public key for encryption
	Auth      string                 `json:"auth"`   // Auth secret
	Alerts    PushSubscriptionAlerts `json:"alerts"` // Which notifications to send
	Policy    string                 `json:"policy,omitempty"`
	UserAgent string                 `json:"user_agent,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	LastUsed  time.Time `json:"last_used,omitempty"`
}

// TableName returns the DynamoDB table name
func (PushSubscription) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for the push subscription
func (p *PushSubscription) UpdateKeys() {
	p.PK = fmt.Sprintf("PUSH#%s", p.Username)
	p.SK = fmt.Sprintf("SUB#%s", p.ID)

	// Set GSI keys for endpoint lookup
	if p.Endpoint != "" {
		endpointHash := hashString(p.Endpoint)
		p.GSI1PK = fmt.Sprintf("PUSH_ENDPOINT#%s", endpointHash)
		p.GSI1SK = p.Username
	}
}

// BeforeCreate is called before creating a new push subscription
func (p *PushSubscription) BeforeCreate() error {
	if p.ID == "" {
		p.ID = uuid.New().String()
	}

	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	p.UpdateKeys()
	return p.Validate()
}

// BeforeUpdate is called before updating a push subscription
func (p *PushSubscription) BeforeUpdate() error {
	p.UpdatedAt = time.Now()
	p.UpdateKeys()
	return p.Validate()
}

// Validate performs validation on the PushSubscription
func (p *PushSubscription) Validate() error {
	if p.Username == "" {
		return fmt.Errorf("username is required")
	}
	if p.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	if p.P256dh == "" {
		return fmt.Errorf("p256dh public key is required")
	}
	if p.Auth == "" {
		return fmt.Errorf("auth secret is required")
	}
	return nil
}

// UpdateLastUsed updates the last used timestamp
func (p *PushSubscription) UpdateLastUsed() {
	p.LastUsed = time.Now()
}

// hashString creates a SHA256 hash of a string
func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
