package models

import (
	"fmt"
	"time"
)

// InstanceRule represents a server rule stored in DynamoDB
type InstanceRule struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // INSTANCE#RULES
	SK string `dynamorm:"sk,attr:SK" json:"-"` // RULE#{order}#{id}

	// GSI for active rules
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"-"` // INSTANCE#ACTIVE_RULES
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"-"` // {order}#{id}

	// Rule data
	ID          string     `dynamorm:"attr:id" json:"id"`
	Text        string     `dynamorm:"attr:text" json:"text"`
	Order       int        `dynamorm:"attr:order" json:"order"`                    // Display order
	Category    string     `dynamorm:"attr:category" json:"category,omitempty"`    // Optional category
	Severity    string     `dynamorm:"attr:severity" json:"severity,omitempty"`    // info, warning, critical
	Active      bool       `dynamorm:"attr:active" json:"active"`
	CreatedAt   time.Time  `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`
	CreatedBy   string     `dynamorm:"attr:createdBy" json:"created_by,omitempty"`         // Admin who created
	EnforcedAt  *time.Time `dynamorm:"attr:enforcedAt" json:"enforced_at,omitempty"`       // When enforcement started
	Description string     `dynamorm:"attr:description" json:"description,omitempty"`      // Extended description
	Examples    []string   `dynamorm:"attr:examples" json:"examples,omitempty"`            // Example violations
}

// TableName returns the DynamoDB table backing InstanceRule.
func (InstanceRule) TableName() string {
	return MainTableName
}

// UpdateKeys updates the GSI keys based on the rule data
func (r *InstanceRule) UpdateKeys() {
	// Primary key
	r.PK = "INSTANCE#RULES"
	r.SK = fmt.Sprintf("RULE#%03d#%s", r.Order, r.ID)

	// GSI for active rules only
	if r.Active {
		r.GSI1PK = "INSTANCE#ACTIVE_RULES"
		r.GSI1SK = fmt.Sprintf("%03d#%s", r.Order, r.ID)
	} else {
		r.GSI1PK = ""
		r.GSI1SK = ""
	}
}

// Activate marks the rule as active and enforced
func (r *InstanceRule) Activate() {
	r.Active = true
	now := time.Now()
	r.EnforcedAt = &now
	r.UpdatedAt = now
	r.UpdateKeys()
}

// Deactivate marks the rule as inactive
func (r *InstanceRule) Deactivate() {
	r.Active = false
	r.EnforcedAt = nil
	r.UpdatedAt = time.Now()
	r.UpdateKeys()
}

// SetOrder updates the display order
func (r *InstanceRule) SetOrder(order int) {
	r.Order = order
	r.UpdatedAt = time.Now()
	r.UpdateKeys()
}

// GetSeverityLevel returns a numeric severity level for sorting
func (r *InstanceRule) GetSeverityLevel() int {
	switch r.Severity {
	case "critical":
		return 3
	case "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}
