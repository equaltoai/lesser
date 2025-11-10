package models

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
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

// TableName returns the DynamoDB table backing PushSubscriptionAlerts.
func (PushSubscriptionAlerts) TableName() string {
	return MainTableName
}

// PushSubscription represents a push subscription stored in DynamoDB
type PushSubscription struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys
	PK string `dynamorm:"pk,attr:PK" json:"pk"` // PUSH#username
	SK string `dynamorm:"sk,attr:SK" json:"sk"` // SUB#subscriptionID

	// GSI for endpoint lookup (to prevent duplicates)
	GSI1PK string `dynamorm:"index:endpoint-index,pk,attr:gsI1PK" json:"gsi1_pk"` // PUSH_ENDPOINT#endpoint_hash
	GSI1SK string `dynamorm:"index:endpoint-index,sk,attr:gsI1SK" json:"gsi1_sk"` // username

	// Core subscription data
	ID        string                 `dynamorm:"attr:id" json:"id"`
	Username  string                 `dynamorm:"attr:username" json:"username"`
	Endpoint  string                 `dynamorm:"attr:endpoint" json:"endpoint"`
	P256dh    string                 `dynamorm:"attr:p256dh" json:"p256dh"` // Public key for encryption
	Auth      string                 `dynamorm:"attr:auth" json:"auth"`     // Auth secret
	Alerts    PushSubscriptionAlerts `dynamorm:"attr:alerts" json:"alerts"` // Which notifications to send
	Policy    string                 `dynamorm:"attr:policy" json:"policy,omitempty"`
	UserAgent string                 `dynamorm:"attr:userAgent" json:"user_agent,omitempty"`

	// Timestamps
	CreatedAt time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
	LastUsed  time.Time `dynamorm:"attr:lastUsed" json:"last_used,omitempty"`
}

// TableName returns the DynamoDB table name
func (PushSubscription) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys for the push subscription
func (p *PushSubscription) UpdateKeys() error {
	p.PK = fmt.Sprintf("PUSH#%s", p.Username)
	p.SK = fmt.Sprintf("SUB#%s", p.ID)

	// Set GSI keys for endpoint lookup
	if p.Endpoint != "" {
		endpointHash := hashString(p.Endpoint)
		p.GSI1PK = fmt.Sprintf("PUSH_ENDPOINT#%s", endpointHash)
		p.GSI1SK = p.Username
	}
	return nil
}

// GetPK returns the partition key
func (p *PushSubscription) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *PushSubscription) GetSK() string {
	return p.SK
}

// BeforeCreate is called before creating a new push subscription
func (p *PushSubscription) BeforeCreate() error {
	if err := common.ValidateRequiredParam("id", p.ID); err != nil {
		p.ID = uuid.New().String()
	}

	now := time.Now()
	p.CreatedAt = now
	p.UpdatedAt = now

	if err := p.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return p.Validate()
}

// BeforeUpdate is called before updating a push subscription
func (p *PushSubscription) BeforeUpdate() error {
	p.UpdatedAt = time.Now()
	if err := p.UpdateKeys(); err != nil {
		return fmt.Errorf("failed to update keys: %w", err)
	}
	return p.Validate()
}

// Validate performs validation on the PushSubscription
func (p *PushSubscription) Validate() error {
	if err := common.ValidateRequiredParam("username", p.Username); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("endpoint", p.Endpoint); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("p256dh", p.P256dh); err != nil {
		return ErrPushSubscriptionP256dhRequired
	}
	if err := common.ValidateRequiredParam("auth", p.Auth); err != nil {
		return ErrPushSubscriptionAuthRequired
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
