package models

import (
	"fmt"
	"time"
)

// InstanceRule represents a server rule stored in DynamoDB
type InstanceRule struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key fields
	PK string `theorydb:"pk,attr:PK" json:"-"` // INSTANCE#RULES
	SK string `theorydb:"sk,attr:SK" json:"-"` // RULE#{order}#{id}

	// GSI for active rules
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"` // INSTANCE#ACTIVE_RULES
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"` // {order}#{id}

	// Rule data
	ID          string     `theorydb:"attr:id" json:"id"`
	Text        string     `theorydb:"attr:text" json:"text"`
	Order       int        `theorydb:"attr:order" json:"order"`                 // Display order
	Category    string     `theorydb:"attr:category" json:"category,omitempty"` // Optional category
	Severity    string     `theorydb:"attr:severity" json:"severity,omitempty"` // info, warning, critical
	Active      bool       `theorydb:"attr:active" json:"active"`
	CreatedAt   time.Time  `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt   time.Time  `theorydb:"attr:updatedAt" json:"updated_at"`
	CreatedBy   string     `theorydb:"attr:createdBy" json:"created_by,omitempty"`    // Admin who created
	EnforcedAt  *time.Time `theorydb:"attr:enforcedAt" json:"enforced_at,omitempty"`  // When enforcement started
	Description string     `theorydb:"attr:description" json:"description,omitempty"` // Extended description
	Examples    []string   `theorydb:"attr:examples" json:"examples,omitempty"`       // Example violations
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
