package models

import (
	"fmt"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// ThreadContext represents the context of a status thread stored in DynamoDB
type ThreadContext struct {
	_ struct{} `dynamorm:"naming:camelCase"`

	// Primary key fields
	PK string `dynamorm:"pk,attr:PK" json:"-"` // THREAD#{rootStatusID}
	SK string `dynamorm:"sk,attr:SK" json:"-"` // CONTEXT#{statusID}

	// GSI for querying by status
	GSI1PK string `dynamorm:"index:gsi1,pk,attr:gsi1PK" json:"-"` // STATUS#{statusID}
	GSI1SK string `dynamorm:"index:gsi1,sk,attr:gsi1SK" json:"-"` // THREAD

	// Thread data
	RootStatusID   string     `dynamorm:"attr:rootStatusID" json:"root_status_id"`              // The root/original status ID
	StatusID       string     `dynamorm:"attr:statusID" json:"status_id"`                       // The status this context is for
	ParentID       string     `dynamorm:"attr:parentID" json:"parent_id"`                       // Direct parent status ID
	Depth          int        `dynamorm:"attr:depth" json:"depth"`                              // Depth in the thread (0 for root)
	Path           string     `dynamorm:"attr:path" json:"path"`                                // Path from root (e.g., "root/reply1/reply2")
	AuthorID       string     `dynamorm:"attr:authorID" json:"author_id"`                       // Author of this status
	AuthorHandle   string     `dynamorm:"attr:authorHandle" json:"author_handle"`               // For quick display
	CreatedAt      time.Time  `dynamorm:"attr:createdAt" json:"created_at"`                     // When the status was created
	UpdatedAt      time.Time  `dynamorm:"attr:updatedAt" json:"updated_at"`                     // Last update
	ReplyCount     int        `dynamorm:"attr:replyCount" json:"reply_count"`                   // Number of direct replies
	TotalReplies   int        `dynamorm:"attr:totalReplies" json:"total_replies"`               // Total replies in subtree
	Participants   []string   `dynamorm:"attr:participants" json:"participants"`                // Unique participants in this branch
	LastReplyAt    *time.Time `dynamorm:"attr:lastReplyAt" json:"last_reply_at,omitempty"`      // When the last reply was made
	Visibility     string     `dynamorm:"attr:visibility" json:"visibility"`                    // Visibility of this status
	Sensitive      bool       `dynamorm:"attr:sensitive" json:"sensitive"`                      // Content warning flag
	ConversationID string     `dynamorm:"attr:conversationID" json:"conversation_id,omitempty"` // Associated conversation ID

	// TTL for auto-cleanup (7 days after last activity)
	TTL int64 `dynamorm:"ttl,attr:ttl" json:"ttl,omitempty"`
}

// UpdateKeys updates the GSI keys based on the thread context data
func (t *ThreadContext) UpdateKeys() {
	// Primary key - for thread hierarchy
	t.PK = fmt.Sprintf("THREAD#%s", t.RootStatusID)
	t.SK = fmt.Sprintf("CONTEXT#%s", t.StatusID)

	// GSI - for finding thread context by status
	t.GSI1PK = fmt.Sprintf(KeyPatternStatus, t.StatusID)
	t.GSI1SK = "THREAD"

	// Set TTL based on last activity
	lastActivity := t.UpdatedAt
	if t.LastReplyAt != nil && t.LastReplyAt.After(lastActivity) {
		lastActivity = *t.LastReplyAt
	}
	t.TTL = lastActivity.Add(7 * 24 * time.Hour).Unix()
}

// IncrementReplyCount increments the reply count and updates last reply time
func (t *ThreadContext) IncrementReplyCount() {
	t.ReplyCount++
	t.TotalReplies++
	now := time.Now()
	t.LastReplyAt = &now
	t.UpdatedAt = now
}

// AddParticipant adds a participant if not already in the list
func (t *ThreadContext) AddParticipant(participantID string) {
	for _, p := range t.Participants {
		if p == participantID {
			return
		}
	}
	t.Participants = append(t.Participants, participantID)
}

// IsRoot checks if this is the root of the thread
func (t *ThreadContext) IsRoot() bool {
	return t.Depth == 0 && t.ParentID == ""
}

// GetPathElements returns the path split into elements
func (t *ThreadContext) GetPathElements() []string {
	if err := common.ValidateRequiredParam("t.Path", t.Path); err != nil {
		return []string{}
	}
	return []string{} // Would implement proper path parsing
}

// TableName returns the DynamoDB table backing ThreadContext.
func (ThreadContext) TableName() string {
	return MainTableName
}
