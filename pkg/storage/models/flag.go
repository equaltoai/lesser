package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Flag represents a content moderation flag with DynamORM tags
type Flag struct {
	// Keys
	PK     string `dynamorm:"pk"`                                  // FLAG#objectID (first object if multiple)
	SK     string `dynamorm:"sk"`                                  // TIME#timestamp#flagID
	GSI1PK string `dynamorm:"index:GSI1,pk,omitempty"` // ACTOR#actorID
	GSI1SK string `dynamorm:"index:GSI1,sk,omitempty"` // FLAG#timestamp
	GSI2PK string `dynamorm:"index:GSI2,pk,omitempty"` // FLAG_STATUS#status
	GSI2SK string `dynamorm:"index:GSI2,sk,omitempty"` // TIME#timestamp

	// Flag fields
	ID         string     `json:"id"`          // The flag activity ID
	Actor      string     `json:"actor"`       // Who flagged
	Object     []string   `json:"object"`      // What was flagged (can be multiple objects)
	Content    string     `json:"content"`     // Reason/description for the flag
	Published  time.Time  `json:"published"`   // When it was flagged
	Status     string     `json:"status"`      // Current status of the flag (pending, reviewed, resolved, dismissed)
	ReviewedBy string     `json:"reviewed_by"` // Moderator who reviewed (if reviewed)
	ReviewedAt *time.Time `json:"reviewed_at"` // When it was reviewed
	ReviewNote string     `json:"review_note"` // Note from reviewer
	CreatedAt  time.Time  `json:"created_at"`  // Database timestamp
}

// TableName returns the DynamoDB table name
func (Flag) TableName() string {
	return MainTableName
}

// UpdateKeys updates the DynamORM keys based on flag data
func (f *Flag) UpdateKeys() {
	// Use first object ID for primary key (flags can reference multiple objects)
	firstObjectID := ""
	if len(f.Object) > 0 {
		firstObjectID = f.Object[0]
	}

	// Primary keys - group by object for moderation efficiency
	f.PK = fmt.Sprintf("FLAG#%s", firstObjectID)
	f.SK = fmt.Sprintf("TIME#%s#%s", f.Published.Format(time.RFC3339Nano), f.ID)

	// GSI1 - Query flags by actor
	f.GSI1PK = fmt.Sprintf(KeyPatternActor, f.Actor)
	f.GSI1SK = fmt.Sprintf("FLAG#%s", f.Published.Format(time.RFC3339Nano))

	// GSI2 - Query flags by status (for pending/reviewed lists)
	f.GSI2PK = fmt.Sprintf("FLAG_STATUS#%s", f.Status)
	f.GSI2SK = fmt.Sprintf("TIME#%s", f.Published.Format(time.RFC3339Nano))
}

// BeforeCreate hook to set timestamps and update keys
func (f *Flag) BeforeCreate() error {
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now()
	}
	if f.Published.IsZero() {
		f.Published = time.Now()
	}
	if err := common.ValidateRequiredParam("f.Status", f.Status); err != nil {
		f.Status = StatusPending
	}
	f.UpdateKeys()
	return nil
}

// BeforeSave hook to update keys
func (f *Flag) BeforeSave() error {
	f.UpdateKeys()
	return nil
}
