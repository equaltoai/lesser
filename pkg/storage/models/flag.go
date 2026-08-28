package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// Flag represents a content moderation flag with DynamORM tags
type Flag struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Keys
	PK string `theorydb:"pk,attr:PK"` // FLAG#objectID (first object if multiple)
	SK string `theorydb:"sk,attr:SK"` // TIME#timestamp#flagID

	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty"` // ACTOR#actorID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty"` // FLAG#timestamp
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty"` // FLAG_STATUS#status
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty"` // TIME#timestamp

	// GSI3 — additive global flag-by-ID listing key (wave #1469, batch S2),
	// maintained on every write by the explicit UpdateKeys call at CreateFlag
	// (moderation_repository.go:2149), UpdateFlagStatus (:2374), and DeleteFlag
	// (:2446) — those call sites are the invariant. The model's
	// BeforeCreate/BeforeSave hooks also invoke UpdateKeys, but tabletheory
	// v3.0.6 never calls those hooks, so they are inert conveniences, not the
	// mechanism. Pre-existing gsi3 slot; no index addition. Legacy flag rows
	// written before this key shape existed are not findable by GetFlag until
	// the flag is next written.
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty"` // FLAGS#ALL
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty"` // ID#<flag id>

	// Flag fields
	ID         string     `theorydb:"attr:id" json:"id"`                  // The flag activity ID
	Actor      string     `theorydb:"attr:actor" json:"actor"`            // Who flagged
	Object     []string   `theorydb:"attr:object" json:"object"`          // What was flagged (can be multiple objects)
	Content    string     `theorydb:"attr:content" json:"content"`        // Reason/description for the flag
	Published  time.Time  `theorydb:"attr:published" json:"published"`    // When it was flagged
	Status     string     `theorydb:"attr:status" json:"status"`          // Current status of the flag (pending, reviewed, resolved, dismissed)
	ReviewedBy string     `theorydb:"attr:reviewedBy" json:"reviewed_by"` // Moderator who reviewed (if reviewed)
	ReviewedAt *time.Time `theorydb:"attr:reviewedAt" json:"reviewed_at"` // When it was reviewed
	ReviewNote string     `theorydb:"attr:reviewNote" json:"review_note"` // Note from reviewer
	CreatedAt  time.Time  `theorydb:"attr:createdAt" json:"created_at"`   // Database timestamp
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

	// GSI3 - Global flag-by-ID listing key (wave #1469, batch S2): additive
	// partition on the pre-existing gsi3 slot. The invariant is the explicit
	// UpdateKeys call at every flag write — CreateFlag
	// (moderation_repository.go:2149), UpdateFlagStatus (:2374), DeleteFlag
	// (:2446); the BeforeCreate/BeforeSave hooks also call UpdateKeys but
	// tabletheory v3.0.6 never invokes them, so they are inert conveniences,
	// not the mechanism. GetFlag reads this key pair exactly; legacy flag rows
	// written before this key shape existed are not findable until next
	// written.
	f.GSI3PK = "FLAGS#ALL"
	f.GSI3SK = fmt.Sprintf("ID#%s", f.ID)
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
