package models

import (
	"fmt"
	"time"
)

// ListMember represents membership of an account in a list
type ListMember struct {
	// Primary keys for list membership
	PK string `dynamorm:"pk" json:"PK"` // LIST_MEMBERS#listID
	SK string `dynamorm:"sk" json:"SK"` // accountID

	// GSI1 for reverse lookup (what lists is an account in)
	GSI1PK string `dynamorm:"index:GSI1,pk" json:"GSI1PK,omitempty"` // ACCOUNT_LISTS#accountID
	GSI1SK string `dynamorm:"index:GSI1,sk" json:"GSI1SK,omitempty"` // listID#username

	// Core fields
	ListID       string    `json:"list_id"`
	AccountID    string    `json:"account_id"`
	ListUsername string    `json:"list_username"` // Owner of the list (for reverse index)
	AddedAt      time.Time `json:"added_at"`
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
