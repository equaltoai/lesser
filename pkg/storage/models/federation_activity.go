package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FederationActivity represents activity from federated instances
type FederationActivity struct {
	// Primary key - using domain as partition key with timestamp sort key
	PK string `dynamorm:"pk" json:"pk"` // Format: "fed_activity#{domain}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "activity#{timestamp}#{id}"

	// GSI1 - Activity type queries
	GSI1PK string `dynamorm:"index:type-index,pk" json:"gsi1_pk"` // Format: "FED_TYPE#{type}"
	GSI1SK string `dynamorm:"index:type-index,sk" json:"gsi1_sk"` // Format: "{timestamp}#{domain}#{id}"

	// GSI2 - Actor queries
	GSI2PK string `dynamorm:"index:actor-index,pk" json:"gsi2_pk"` // Format: "FED_ACTOR#{actorID}"
	GSI2SK string `dynamorm:"index:actor-index,sk" json:"gsi2_sk"` // Format: "{timestamp}#{id}"

	// Core activity data
	ID           string    `json:"id"`
	Domain       string    `json:"domain"`        // Remote instance domain
	ActivityType string    `json:"activity_type"` // Create, Update, Delete, Follow, etc.
	ActorID      string    `json:"actor_id"`      // Remote actor ID (full URL)
	ObjectID     string    `json:"object_id"`     // Object being acted upon
	ObjectType   string    `json:"object_type"`   // Note, Actor, etc.
	Timestamp    time.Time `json:"timestamp"`

	// Instance information
	InstanceInfo *InstanceInfo `json:"instance_info,omitempty"`

	// Activity details
	Success      bool    `json:"success"`
	ErrorMessage string  `json:"error_message,omitempty"`
	ResponseTime float64 `json:"response_time_ms"` // Response time in milliseconds

	// Volume tracking
	InboundSize  int64 `json:"inbound_size"`  // Size of inbound data in bytes
	OutboundSize int64 `json:"outbound_size"` // Size of outbound data in bytes

	// Additional metadata
	UserAgent  string                 `json:"user_agent,omitempty"`
	RemoteIP   string                 `json:"remote_ip,omitempty"`
	Headers    map[string]string      `json:"headers,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// TTL for automatic cleanup (90 days)
	ExpiresAt int64 `dynamorm:"ttl" json:"expires_at"` // Unix timestamp
}

// InstanceInfo contains information about a federated instance
type InstanceInfo struct {
	Domain      string    `json:"domain"`
	Software    string    `json:"software,omitempty"`     // Mastodon, Pleroma, etc.
	Version     string    `json:"version,omitempty"`       // Software version
	PublicKey   string    `json:"public_key,omitempty"`    // Instance public key
	SharedInbox string    `json:"shared_inbox,omitempty"`  // Shared inbox URL
	LastSeen    time.Time `json:"last_seen"`
	FirstSeen   time.Time `json:"first_seen"`
}

// TableName returns the DynamoDB table name
func (FederationActivity) TableName() string {
	return "lesser-main"
}

// BeforeCreate sets up the model before creation
func (fa *FederationActivity) BeforeCreate() error {
	now := time.Now()
	fa.CreatedAt = now
	fa.UpdatedAt = now

	// Generate ID if not provided
	if fa.ID == "" {
		fa.ID = uuid.New().String()
	}

	// Set timestamp if not provided
	if fa.Timestamp.IsZero() {
		fa.Timestamp = now
	}

	// Set expiry to 90 days
	fa.ExpiresAt = now.Add(90 * 24 * time.Hour).Unix()

	// Set up primary key
	fa.PK = "fed_activity#" + fa.Domain
	timestamp := fa.Timestamp.Format("20060102150405")
	fa.SK = fmt.Sprintf("activity#%s#%s", timestamp, fa.ID)

	// Set up GSI keys
	fa.setupGSIKeys()

	return fa.Validate()
}

// BeforeUpdate sets up the model before update
func (fa *FederationActivity) BeforeUpdate() error {
	fa.UpdatedAt = time.Now()

	// Update GSI keys in case indexed fields changed
	fa.setupGSIKeys()

	return fa.Validate()
}

// setupGSIKeys configures all GSI partition and sort keys
func (fa *FederationActivity) setupGSIKeys() {
	timestampStr := fa.Timestamp.Format(time.RFC3339)

	// GSI1 - Activity type queries
	fa.GSI1PK = "FED_TYPE#" + fa.ActivityType
	fa.GSI1SK = fmt.Sprintf("%s#%s#%s", timestampStr, fa.Domain, fa.ID)

	// GSI2 - Actor queries
	if fa.ActorID != "" {
		fa.GSI2PK = "FED_ACTOR#" + fa.ActorID
		fa.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, fa.ID)
	} else {
		fa.GSI2PK = ""
		fa.GSI2SK = ""
	}
}

// Validate performs validation on the FederationActivity
func (fa *FederationActivity) Validate() error {
	if strings.TrimSpace(fa.ID) == "" {
		return fmt.Errorf("ID is required")
	}
	if strings.TrimSpace(fa.Domain) == "" {
		return fmt.Errorf("Domain is required")
	}
	if strings.TrimSpace(fa.ActivityType) == "" {
		return fmt.Errorf("ActivityType is required")
	}
	if strings.TrimSpace(fa.ActorID) == "" {
		return fmt.Errorf("ActorID is required")
	}

	return nil
}

// ExtractDomain extracts the domain from an ActivityPub ID URL
func ExtractDomain(actorID string) string {
	// Handle URLs like https://example.com/users/alice
	if strings.Contains(actorID, "://") {
		parts := strings.Split(actorID, "/")
		if len(parts) >= 3 {
			return strings.ToLower(parts[2])
		}
	}
	return ""
}

// IsRemote checks if the activity is from a remote instance
func (fa *FederationActivity) IsRemote(localDomain string) bool {
	return fa.Domain != "" && fa.Domain != localDomain
}

// SetProperty sets a custom property
func (fa *FederationActivity) SetProperty(key string, value interface{}) {
	if fa.Properties == nil {
		fa.Properties = make(map[string]interface{})
	}
	fa.Properties[key] = value
}

// GetProperty gets a custom property
func (fa *FederationActivity) GetProperty(key string) (interface{}, bool) {
	if fa.Properties == nil {
		return nil, false
	}
	value, exists := fa.Properties[key]
	return value, exists
}

// SetHeader sets a header value
func (fa *FederationActivity) SetHeader(key, value string) {
	if fa.Headers == nil {
		fa.Headers = make(map[string]string)
	}
	fa.Headers[key] = value
}

// GetHeader gets a header value
func (fa *FederationActivity) GetHeader(key string) (string, bool) {
	if fa.Headers == nil {
		return "", false
	}
	value, exists := fa.Headers[key]
	return value, exists
}

// MarkSuccess marks the activity as successful
func (fa *FederationActivity) MarkSuccess() {
	fa.Success = true
	fa.ErrorMessage = ""
}

// MarkFailed marks the activity as failed
func (fa *FederationActivity) MarkFailed(errorMsg string) {
	fa.Success = false
	fa.ErrorMessage = errorMsg
}

// FederationActivityBuilder helps create federation activities
type FederationActivityBuilder struct {
	activity *FederationActivity
}

// NewFederationActivityBuilder creates a new builder
func NewFederationActivityBuilder() *FederationActivityBuilder {
	return &FederationActivityBuilder{
		activity: &FederationActivity{
			Headers:    make(map[string]string),
			Properties: make(map[string]interface{}),
		},
	}
}

// FromDomain sets the source domain
func (fab *FederationActivityBuilder) FromDomain(domain string) *FederationActivityBuilder {
	fab.activity.Domain = domain
	return fab
}

// WithType sets the activity type
func (fab *FederationActivityBuilder) WithType(activityType string) *FederationActivityBuilder {
	fab.activity.ActivityType = activityType
	return fab
}

// WithActor sets the actor ID
func (fab *FederationActivityBuilder) WithActor(actorID string) *FederationActivityBuilder {
	fab.activity.ActorID = actorID
	return fab
}

// WithObject sets the object information
func (fab *FederationActivityBuilder) WithObject(objectID, objectType string) *FederationActivityBuilder {
	fab.activity.ObjectID = objectID
	fab.activity.ObjectType = objectType
	return fab
}

// WithResponseTime sets the response time
func (fab *FederationActivityBuilder) WithResponseTime(ms float64) *FederationActivityBuilder {
	fab.activity.ResponseTime = ms
	return fab
}

// WithVolume sets the data volume
func (fab *FederationActivityBuilder) WithVolume(inbound, outbound int64) *FederationActivityBuilder {
	fab.activity.InboundSize = inbound
	fab.activity.OutboundSize = outbound
	return fab
}

// WithInstanceInfo sets the instance information
func (fab *FederationActivityBuilder) WithInstanceInfo(info *InstanceInfo) *FederationActivityBuilder {
	fab.activity.InstanceInfo = info
	return fab
}

// WithError marks the activity as failed with an error
func (fab *FederationActivityBuilder) WithError(err error) *FederationActivityBuilder {
	fab.activity.MarkFailed(err.Error())
	return fab
}

// Build creates the federation activity
func (fab *FederationActivityBuilder) Build() *FederationActivity {
	return fab.activity
}