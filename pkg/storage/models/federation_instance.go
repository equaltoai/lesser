package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/federation/types"
)

// FederationInstanceRegistry represents a federated instance in the routing registry
type FederationInstanceRegistry struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - using pattern: PK=INSTANCE#<domain>, SK=METADATA
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// GSI keys for status-based queries (GSI1)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1_pk"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1_sk"`

	// GSI keys for tier-based queries (GSI2)
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2_pk"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2_sk"`

	// Core instance attributes
	ID             string `theorydb:"attr:id" json:"id"`
	Domain         string `theorydb:"attr:domain" json:"domain"`
	InboxURL       string `theorydb:"attr:inboxURL" json:"inbox_url"`
	SharedInboxURL string `theorydb:"attr:sharedInboxURL" json:"shared_inbox_url"`
	PublicKeyPEM   string `theorydb:"attr:publicKeyPEM" json:"public_key_pem"`

	// Capabilities
	SupportedTypes []string `theorydb:"attr:supportedTypes" json:"supported_types"`
	MaxMessageSize int64    `theorydb:"attr:maxMessageSize" json:"max_message_size"`

	// Rate limits (stored as nested JSON)
	RateLimits map[string]interface{} `theorydb:"attr:rateLimits" json:"rate_limits"`

	// Status and timestamps
	Status       string    `theorydb:"attr:status" json:"status"`
	LastSeen     time.Time `theorydb:"attr:lastSeen" json:"last_seen"`
	RegisteredAt time.Time `theorydb:"attr:registeredAt" json:"registered_at"`

	// Performance metrics
	AvgResponseTime int64   `theorydb:"attr:avgResponseTime" json:"avg_response_time"` // milliseconds
	SuccessRate     float64 `theorydb:"attr:successRate" json:"success_rate"`
	ErrorRate       float64 `theorydb:"attr:errorRate" json:"error_rate"`

	// Cost tracking
	TierLevel    string `theorydb:"attr:tierLevel" json:"tier_level"`
	MonthlyQuota int64  `theorydb:"attr:monthlyQuota" json:"monthly_quota"`
	CurrentUsage int64  `theorydb:"attr:currentUsage" json:"current_usage"`

	// TTL for automatic cleanup
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
}

// TableName returns the DynamoDB table backing FederationInstanceRegistry.
func (FederationInstanceRegistry) TableName() string {
	return MainTableName
}

// UpdateKeys ensures GSI keys are properly set before saving
func (f *FederationInstanceRegistry) UpdateKeys() error {
	// Set primary keys
	f.PK = fmt.Sprintf("INSTANCE#%s", f.Domain)
	f.SK = SKMetadata

	// Set GSI1 keys for status-based queries
	f.GSI1PK = fmt.Sprintf(KeyPatternStatus, f.Status)
	f.GSI1SK = fmt.Sprintf("DOMAIN#%s", f.Domain)

	// Set GSI2 keys for tier-based queries
	f.GSI2PK = fmt.Sprintf("TIER#%s", f.TierLevel)
	f.GSI2SK = fmt.Sprintf("USAGE#%010d", f.CurrentUsage)

	// Set TTL (1 year from now)
	f.TTL = time.Now().Add(365 * 24 * time.Hour).Unix()

	return nil
}

// GetPK returns the partition key
func (f *FederationInstanceRegistry) GetPK() string {
	return f.PK
}

// GetSK returns the sort key
func (f *FederationInstanceRegistry) GetSK() string {
	return f.SK
}

// FederationInstanceRegistryHealthHistory represents health history records
type FederationInstanceRegistryHealthHistory struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - using pattern: PK=INSTANCE#<instanceID>, SK=HEALTH#<timestamp>
	PK string `theorydb:"pk,attr:PK" json:"pk"`
	SK string `theorydb:"sk,attr:SK" json:"sk"`

	// Health status fields
	Reachable       bool      `theorydb:"attr:reachable" json:"reachable"`
	ResponseTime    int64     `theorydb:"attr:responseTime" json:"response_time"` // milliseconds
	StatusCode      int       `theorydb:"attr:statusCode" json:"status_code"`
	ErrorRate       float64   `theorydb:"attr:errorRate" json:"error_rate"`
	InboxBacklog    int       `theorydb:"attr:inboxBacklog" json:"inbox_backlog"`
	ProcessingDelay int64     `theorydb:"attr:processingDelay" json:"processing_delay"` // milliseconds
	ErrorMessage    string    `theorydb:"attr:errorMessage" json:"error_message,omitempty"`
	Timestamp       time.Time `theorydb:"attr:timestamp" json:"timestamp"`

	// TTL for automatic cleanup (keep 7 days)
	TTL int64 `theorydb:"ttl,attr:ttl" json:"ttl"`
}

// TableName returns the DynamoDB table backing FederationInstanceRegistryHealthHistory.
func (FederationInstanceRegistryHealthHistory) TableName() string {
	return MainTableName
}

// UpdateKeys ensures keys are properly set for health history
func (h *FederationInstanceRegistryHealthHistory) UpdateKeys() error {
	h.PK = fmt.Sprintf("INSTANCE#%s", h.extractInstanceIDFromPK())
	h.SK = fmt.Sprintf("HEALTH#%d", h.Timestamp.UnixNano())
	h.TTL = time.Now().Add(7 * 24 * time.Hour).Unix()
	return nil
}

// GetPK returns the partition key
func (h *FederationInstanceRegistryHealthHistory) GetPK() string {
	return h.PK
}

// GetSK returns the sort key
func (h *FederationInstanceRegistryHealthHistory) GetSK() string {
	return h.SK
}

// extractInstanceIDFromPK extracts instance ID from PK (assumes it's already set)
func (h *FederationInstanceRegistryHealthHistory) extractInstanceIDFromPK() string {
	if h.PK != "" {
		// Extract from existing PK format: INSTANCE#<instanceID>
		parts := strings.Split(h.PK, "#")
		if len(parts) >= 2 {
			return parts[1]
		}
	}
	return ""
}

// ToInstance converts the model to a federation types Instance
func (f *FederationInstanceRegistry) ToInstance() *types.Instance {
	instance := &types.Instance{
		ID:              f.ID,
		Domain:          f.Domain,
		InboxURL:        f.InboxURL,
		SharedInboxURL:  f.SharedInboxURL,
		PublicKeyPEM:    f.PublicKeyPEM,
		Status:          types.InstanceStatus(f.Status),
		LastSeen:        f.LastSeen,
		RegisteredAt:    f.RegisteredAt,
		AvgResponseTime: time.Duration(f.AvgResponseTime) * time.Millisecond,
		SuccessRate:     f.SuccessRate,
		ErrorRate:       f.ErrorRate,
		TierLevel:       types.TierLevel(f.TierLevel),
		MonthlyQuota:    f.MonthlyQuota,
		CurrentUsage:    f.CurrentUsage,
		MaxMessageSize:  f.MaxMessageSize,
	}

	// Convert supported types
	if len(f.SupportedTypes) > 0 {
		instance.SupportedTypes = make([]types.MessageType, len(f.SupportedTypes))
		for i, t := range f.SupportedTypes {
			instance.SupportedTypes[i] = types.MessageType(t)
		}
	}

	// Convert rate limits from map
	if f.RateLimits != nil {
		limits := &types.RateLimits{}
		if v, ok := f.RateLimits["MessagesPerMinute"]; ok {
			if rpm, ok := v.(float64); ok {
				limits.MessagesPerMinute = int(rpm)
			}
		}
		if v, ok := f.RateLimits["MessagesPerHour"]; ok {
			if rph, ok := v.(float64); ok {
				limits.MessagesPerHour = int(rph)
			}
		}
		if v, ok := f.RateLimits["BytesPerMinute"]; ok {
			if bpm, ok := v.(float64); ok {
				limits.BytesPerMinute = int64(bpm)
			}
		}
		if v, ok := f.RateLimits["BytesPerHour"]; ok {
			if bph, ok := v.(float64); ok {
				limits.BytesPerHour = int64(bph)
			}
		}
		if v, ok := f.RateLimits["BurstSize"]; ok {
			if bs, ok := v.(float64); ok {
				limits.BurstSize = int(bs)
			}
		}
		instance.RateLimits = *limits
	}

	return instance
}
