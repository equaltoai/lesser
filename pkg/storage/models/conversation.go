package models

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// DmRequestState represents the per-user request lifecycle state for a DM thread.
// It is stored on ConversationParticipantRecord (not on the shared Conversation).
type DmRequestState string

const (
	// DmRequestStatePending indicates the participant has not yet accepted or declined the request.
	DmRequestStatePending DmRequestState = "PENDING"
	// DmRequestStateAccepted indicates the participant has accepted the request.
	DmRequestStateAccepted DmRequestState = "ACCEPTED"
	// DmRequestStateDeclined indicates the participant has declined the request.
	DmRequestStateDeclined DmRequestState = "DECLINED"
)

// Conversation represents a direct message conversation between users
type Conversation struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys - MUST match legacy exactly
	PK string `theorydb:"pk,attr:PK" json:"PK"` // CONVERSATION#conversationID
	SK string `theorydb:"sk,attr:SK" json:"SK"` // METADATA

	// GSI1 is used for participant records (additional records per participant)
	// Note: The main conversation record doesn't use GSI1, only participant records do
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK,omitempty"`

	// Core fields from legacy
	ID           string    `theorydb:"attr:id" json:"id"`
	Participants []string  `theorydb:"attr:participants" json:"participants"` // Actor IDs/usernames
	LastStatusID string    `theorydb:"attr:lastStatusID" json:"last_status_id,omitempty"`
	Unread       bool      `theorydb:"-" json:"unread"` // Per-viewer projection only; not stored on the shared row
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Message counting fields
	TotalMessageCount int64     `theorydb:"attr:totalMessageCount" json:"total_message_count"`       // Total messages in conversation
	LastMessageTime   time.Time `theorydb:"attr:lastMessageTime" json:"last_message_time,omitempty"` // Time of last message
}

// CanonicalConversationParticipantID normalizes local conversation participant identifiers
// to the lowercase form used by conversation lookup keys.
func CanonicalConversationParticipantID(participantID string) string {
	participantID = strings.TrimSpace(participantID)
	if participantID == "" || strings.Contains(participantID, "://") {
		return participantID
	}

	return strings.ToLower(participantID)
}

// CanonicalConversationParticipants returns a sorted, normalized participant list for comparison keys.
func CanonicalConversationParticipants(participants []string) []string {
	normalized := make([]string, 0, len(participants))
	for _, participant := range participants {
		canonicalParticipant := CanonicalConversationParticipantID(participant)
		if canonicalParticipant == "" {
			continue
		}
		normalized = append(normalized, canonicalParticipant)
	}

	sort.Strings(normalized)
	return normalized
}

// TableName returns the DynamoDB table name
func (Conversation) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys before creating a conversation
func (c *Conversation) BeforeCreate() error {
	if err := common.ValidateRequiredParam("c.ID", c.ID); err != nil {
		return ErrConversationIDRequired
	}

	c.PK = fmt.Sprintf(KeyPatternConversation, c.ID)
	c.SK = SKMetadata

	if c.CreatedAt.IsZero() {
		c.CreatedAt = time.Now()
	}
	if c.UpdatedAt.IsZero() {
		c.UpdatedAt = time.Now()
	}

	// GSI keys are not set on the main record, only on participant records
	c.GSI1PK = ""
	c.GSI1SK = ""

	return nil
}

// UpdateKeys updates GSI keys - for conversation, this is mainly used for participant records
func (c *Conversation) UpdateKeys() error {
	// Validate required fields
	if err := common.ValidateRequiredParam("c.ID", c.ID); err != nil {
		return ErrConversationIDRequired
	}

	// Set primary keys
	c.PK = fmt.Sprintf(KeyPatternConversation, c.ID)
	c.SK = SKMetadata

	// For the main conversation record, GSI keys are empty
	// GSI keys are only used for participant records which are created separately
	c.GSI1PK = ""
	c.GSI1SK = ""

	return nil
}

// GetPK returns the partition key
func (c *Conversation) GetPK() string {
	return c.PK
}

// GetSK returns the sort key
func (c *Conversation) GetSK() string {
	return c.SK
}

// ConversationParticipantRecord is a compatibility projection used by older DM call sites.
// The canonical stored row is UserConversationState; this type intentionally does not own
// any nested conversation snapshot.
type ConversationParticipantRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Legacy key material preserved for compatibility with older tests and cursors.
	PK string `theorydb:"pk,attr:PK" json:"PK"` // USER_CONVERSATIONS#username
	SK string `theorydb:"sk,attr:SK" json:"SK"` // timestamp#conversationID (for sorting by recent)

	// GSI1 for reverse lookup (find participants by conversation)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK"` // CONVERSATION#conversationID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK"` // PARTICIPANT#username

	ViewerID       string                 `theorydb:"-" json:"viewer_id,omitempty"`
	ConversationID string                 `theorydb:"-" json:"conversation_id,omitempty"`
	CounterpartID  string                 `theorydb:"-" json:"counterpart_id,omitempty"`
	Folder         UserConversationFolder `theorydb:"-" json:"folder,omitempty"`

	// Per-participant DM metadata (folder/request lifecycle, deletion, unread).
	RequestState             DmRequestState `theorydb:"attr:requestState" json:"request_state,omitempty"`
	RequestedAt              *time.Time     `theorydb:"attr:requestedAt" json:"requested_at,omitempty"`
	AcceptedAt               *time.Time     `theorydb:"attr:acceptedAt" json:"accepted_at,omitempty"`
	DeclinedAt               *time.Time     `theorydb:"attr:declinedAt" json:"declined_at,omitempty"`
	DeletedAt                *time.Time     `theorydb:"attr:deletedAt" json:"deleted_at,omitempty"`
	Unread                   bool           `theorydb:"attr:unread" json:"unread"`
	LastReadAt               *time.Time     `theorydb:"attr:lastReadAt" json:"last_read_at,omitempty"`
	PreviewStatusID          string         `theorydb:"-" json:"preview_status_id,omitempty"`
	PreviewStatusPublishedAt time.Time      `theorydb:"-" json:"preview_status_published_at,omitempty"`
	SortAt                   time.Time      `theorydb:"-" json:"sort_at,omitempty"`

	// Conversation is the hydrated runtime view used by repository callers.
	Conversation *Conversation `theorydb:"-" json:"-"`
}

// TableName returns the DynamoDB table name
func (ConversationParticipantRecord) TableName() string {
	return MainTableName
}

// BeforeCreate sets up the keys for a participant record
func (p *ConversationParticipantRecord) BeforeCreate(participantID string) error {
	if p == nil {
		return ErrConversationDataRequired
	}

	conversationID := strings.TrimSpace(p.ConversationID)
	var updatedAt time.Time
	if p.Conversation != nil {
		if conversationID == "" {
			conversationID = p.Conversation.ID
		}
		if updatedAt.IsZero() {
			updatedAt = p.Conversation.UpdatedAt
		}
	}
	if conversationID == "" {
		return ErrConversationDataRequired
	}
	if updatedAt.IsZero() {
		if !p.SortAt.IsZero() {
			updatedAt = p.SortAt
		} else {
			updatedAt = time.Now().UTC()
		}
	}
	updatedAt = updatedAt.UTC()

	participantID = CanonicalConversationParticipantID(participantID)
	p.ViewerID = participantID
	p.ConversationID = conversationID
	p.PK = fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)
	p.SK = fmt.Sprintf("%s#%s", updatedAt.Format(time.RFC3339Nano), conversationID)
	p.GSI1PK = fmt.Sprintf(KeyPatternConversation, conversationID)
	p.GSI1SK = fmt.Sprintf("PARTICIPANT#%s", participantID)
	p.SortAt = updatedAt

	return nil
}

// UpdateKeys updates the GSI keys for the participant record
func (p *ConversationParticipantRecord) UpdateKeys() error {
	// Keys are set in BeforeCreate
	return nil
}

// GetPK returns the partition key
func (p *ConversationParticipantRecord) GetPK() string {
	return p.PK
}

// GetSK returns the sort key
func (p *ConversationParticipantRecord) GetSK() string {
	return p.SK
}

// ConversationParticipantKey is used for looking up conversations by exact participants
// Legacy uses GSI1PK = CONVERSATION_PARTICIPANTS#sorted_participants_list
type ConversationParticipantKey struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK     string `theorydb:"pk,attr:PK" json:"PK"`
	SK     string `theorydb:"sk,attr:SK" json:"SK"`
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK,omitempty"`

	ConversationID string `theorydb:"attr:conversationID" json:"conversation_id"`
}

// TableName returns the DynamoDB table name
func (ConversationParticipantKey) TableName() string {
	return MainTableName
}

// UpdateKeys updates the composite keys based on conversation ID
func (k *ConversationParticipantKey) UpdateKeys() error {
	// Keys are set when creating the lookup key
	return nil
}

// GetPK returns the partition key
func (k *ConversationParticipantKey) GetPK() string {
	return k.PK
}

// GetSK returns the sort key
func (k *ConversationParticipantKey) GetSK() string {
	return k.SK
}
