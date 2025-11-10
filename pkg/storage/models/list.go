package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// List represents a user-created list for organizing followed accounts
type List struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `dynamorm:"pk,attr:PK" json:"PK"` // LIST#listID
	SK string `dynamorm:"sk,attr:SK" json:"SK"` // METADATA

	// GSI1 for user's lists index
	GSI1PK string `dynamorm:"index:GSI1,pk,attr:gsi1PK" json:"gsi1PK,omitempty"` // USER_LISTS#username
	GSI1SK string `dynamorm:"index:GSI1,sk,attr:gsi1SK" json:"gsi1SK,omitempty"` // listID

	// Core fields from legacy
	ID            string    `dynamorm:"attr:id" json:"id"`
	Username      string    `dynamorm:"attr:username" json:"username"`           // Owner of the list
	Title         string    `dynamorm:"attr:title" json:"title"`
	RepliesPolicy string    `dynamorm:"attr:repliesPolicy" json:"replies_policy"` // list, followed, none
	Exclusive     bool      `dynamorm:"attr:exclusive" json:"exclusive"`          // Whether list is exclusive
	CreatedAt     time.Time `dynamorm:"attr:createdAt" json:"created_at"`
	UpdatedAt     time.Time `dynamorm:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name
func (List) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating
func (l *List) BeforeCreate() error {
	l.PK = fmt.Sprintf(KeyPatternList, l.ID)
	l.SK = SKMetadata
	l.CreatedAt = time.Now()
	l.UpdatedAt = time.Now()
	return l.UpdateKeys()
}

// BeforeUpdate sets the updated timestamp
func (l *List) BeforeUpdate() error {
	l.UpdatedAt = time.Now()
	return l.UpdateKeys()
}

// UpdateKeys updates the GSI keys based on current field values
func (l *List) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("l.ID", l.ID); err != nil {
		return err
	}
	if err := common.ValidateRequiredParam("l.Username", l.Username); err != nil {
		return err
	}

	// Set primary keys
	l.PK = fmt.Sprintf(KeyPatternList, l.ID)
	l.SK = SKMetadata

	// Set up GSI1 keys for user's lists index
	l.GSI1PK = fmt.Sprintf("USER_LISTS#%s", l.Username)
	l.GSI1SK = l.ID
	return nil
}

// GetPK returns the partition key (required by BaseModel)
func (l *List) GetPK() string {
	return l.PK
}

// GetSK returns the sort key (required by BaseModel)
func (l *List) GetSK() string {
	return l.SK
}
