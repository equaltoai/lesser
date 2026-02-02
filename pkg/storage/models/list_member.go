package models

import (
	"fmt"
	"time"
)

// ListMember represents membership of an account in a list
type ListMember struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys for list membership
	PK string `theorydb:"pk,attr:PK" json:"PK"` // LIST_MEMBERS#listID
	SK string `theorydb:"sk,attr:SK" json:"SK"` // accountID

	// GSI1 for reverse lookup (what lists is an account in)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK" json:"gsi1PK,omitempty"` // ACCOUNT_LISTS#accountID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK" json:"gsi1SK,omitempty"` // listID#username

	// Core fields
	ListID       string    `theorydb:"attr:listID" json:"list_id"`
	AccountID    string    `theorydb:"attr:accountID" json:"account_id"`
	ListUsername string    `theorydb:"attr:listUsername" json:"list_username"` // Owner of the list (for reverse index)
	AddedAt      time.Time `theorydb:"attr:addedAt" json:"added_at"`
}

// TableName returns the DynamoDB table name
func (ListMember) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (lm *ListMember) BeforeCreate() error {
	lm.PK = fmt.Sprintf("LIST_MEMBERS#%s", lm.ListID)
	lm.SK = lm.AccountID
	lm.AddedAt = time.Now()
	return lm.UpdateKeys()
}

// UpdateKeys updates the GSI keys based on current field values
func (lm *ListMember) UpdateKeys() error {
	// Validate required fields
	if lm.ListID == "" {
		return fmt.Errorf("ListID is required")
	}
	if lm.AccountID == "" {
		return fmt.Errorf("AccountID is required")
	}

	// Set primary keys
	lm.PK = fmt.Sprintf("LIST_MEMBERS#%s", lm.ListID)
	lm.SK = lm.AccountID

	// Set up GSI1 keys for reverse lookup
	lm.GSI1PK = fmt.Sprintf("ACCOUNT_LISTS#%s", lm.AccountID)
	lm.GSI1SK = fmt.Sprintf("%s#%s", lm.ListID, lm.ListUsername)
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (lm *ListMember) GetPK() string {
	return lm.PK
}

// GetSK returns the sort key (required by BaseModel)
func (lm *ListMember) GetSK() string {
	return lm.SK
}
