package models

import (
	"fmt"
	"time"
)

// UserPreference represents a single user preference key-value pair
// Key pattern: PK=USER#{username}, SK=PREFERENCE#{key}
type UserPreference struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key components
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "USER#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "PREFERENCE#{key}"

	// Preference data
	Username string `theorydb:"attr:username" json:"username"` // Who owns the preference
	Key      string `theorydb:"attr:key" json:"key"`           // Preference key (e.g., "language", "theme")
	Value    string `theorydb:"attr:value" json:"value"`       // Preference value (JSON encoded)

	// Metadata
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (UserPreference) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the UserPreference for creation
func (p *UserPreference) BeforeCreate() error {
	// Set timestamp if not already set
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now()
	}

	// Update keys
	p.UpdateKeys()

	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (p *UserPreference) UpdateKeys() {
	p.PK = fmt.Sprintf(KeyPatternUser, p.Username)
	p.SK = fmt.Sprintf("PREFERENCE#%s", p.Key)
}

// FollowRequestState represents a follow request state
// Key pattern: PK=FOLLOW_REQUEST#{requester_id}, SK=TARGET#{target_id}
type FollowRequestState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key components
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "FOLLOW_REQUEST#{requester_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "TARGET#{target_id}"

	// Follow request data
	RequesterID string `theorydb:"attr:requesterID" json:"requester_id"` // Who made the request
	TargetID    string `theorydb:"attr:targetID" json:"target_id"`       // Who the request is for
	State       string `theorydb:"attr:state" json:"state"`              // pending, accepted, rejected

	// Metadata
	CreatedAt time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt time.Time `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (FollowRequestState) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the FollowRequestState for creation
func (f *FollowRequestState) BeforeCreate() error {
	// Set timestamps if not already set
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = f.CreatedAt
	}

	// Update keys
	f.UpdateKeys()

	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (f *FollowRequestState) UpdateKeys() {
	f.PK = fmt.Sprintf("FOLLOW_REQUEST#%s", f.RequesterID)
	f.SK = fmt.Sprintf("TARGET#%s", f.TargetID)
}

// FieldVerification represents a verified field on a user's profile
// Key pattern: PK=USER#{username}, SK=FIELD_VERIFICATION#{field_name}
type FieldVerification struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key components
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "USER#{username}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "FIELD_VERIFICATION#{field_name}"

	// Field verification data
	Username   string    `theorydb:"attr:username" json:"username"`      // Who owns the field
	FieldName  string    `theorydb:"attr:fieldName" json:"field_name"`   // Field name (e.g., "website", "github")
	FieldValue string    `theorydb:"attr:fieldValue" json:"field_value"` // Field value
	VerifiedAt time.Time `theorydb:"attr:verifiedAt" json:"verified_at"` // When verified
	VerifiedBy string    `theorydb:"attr:verifiedBy" json:"verified_by"` // How verified (e.g., "link", "dns", "manual")
	ExpiresAt  time.Time `theorydb:"attr:expiresAt" json:"expires_at"`   // When verification expires
}

// TableName returns the DynamoDB table name
func (FieldVerification) TableName() string {
	return MainTableName
}

// BeforeCreate prepares the FieldVerification for creation
func (f *FieldVerification) BeforeCreate() error {
	// Set timestamp if not already set
	if f.VerifiedAt.IsZero() {
		f.VerifiedAt = time.Now()
	}

	// Update keys
	f.UpdateKeys()

	return nil
}

// UpdateKeys updates the primary key fields based on the current data
func (f *FieldVerification) UpdateKeys() {
	f.PK = fmt.Sprintf(KeyPatternUser, f.Username)
	f.SK = fmt.Sprintf("FIELD_VERIFICATION#%s", f.FieldName)
}

// IsExpired checks if the field verification has expired
func (f *FieldVerification) IsExpired() bool {
	return !f.ExpiresAt.IsZero() && time.Now().After(f.ExpiresAt)
}
