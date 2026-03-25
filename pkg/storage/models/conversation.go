package models

import (
	"fmt"
	"slices"
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
	Unread       bool      `theorydb:"attr:unread" json:"unread"` // Whether conversation has unread messages
	CreatedAt    time.Time `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt    time.Time `theorydb:"attr:updatedAt" json:"updated_at"`

	// Message counting fields
	TotalMessageCount int64     `theorydb:"attr:totalMessageCount" json:"total_message_count"`       // Total messages in conversation
	LastMessageTime   time.Time `theorydb:"attr:lastMessageTime" json:"last_message_time,omitempty"` // Time of last message
}

// ConversationSnapshot stores the participant-facing conversation payload nested under
// ConversationParticipantRecord. It intentionally avoids theorydb/json tags so the
// nested map round-trips using stable exported field names.
type ConversationSnapshot struct {
	ID                string
	Participants      []string
	LastStatusID      string
	Unread            bool
	CreatedAt         time.Time
	UpdatedAt         time.Time
	TotalMessageCount int64
	LastMessageTime   time.Time
}

// ToConversation converts the embedded snapshot back into the full conversation model used by callers.
func (s *ConversationSnapshot) ToConversation() *Conversation {
	if s == nil {
		return nil
	}

	return &Conversation{
		ID:                s.ID,
		Participants:      slices.Clone(s.Participants),
		LastStatusID:      s.LastStatusID,
		Unread:            s.Unread,
		CreatedAt:         s.CreatedAt,
		UpdatedAt:         s.UpdatedAt,
		TotalMessageCount: s.TotalMessageCount,
		LastMessageTime:   s.LastMessageTime,
	}
}

// ConversationSnapshotFromConversation converts a full conversation into the embedded participant snapshot shape.
func ConversationSnapshotFromConversation(conversation *Conversation) *ConversationSnapshot {
	if conversation == nil {
		return nil
	}

	return &ConversationSnapshot{
		ID:                conversation.ID,
		Participants:      slices.Clone(conversation.Participants),
		LastStatusID:      conversation.LastStatusID,
		Unread:            conversation.Unread,
		CreatedAt:         conversation.CreatedAt,
		UpdatedAt:         conversation.UpdatedAt,
		TotalMessageCount: conversation.TotalMessageCount,
		LastMessageTime:   conversation.LastMessageTime,
	}
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

// ConversationParticipantRecord represents a participant's view of a conversation
// This is used for querying conversations by user
type ConversationParticipantRecord struct {
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary keys for participant record
	PK string `theorydb:"pk,attr:PK" json:"PK"` // USER_CONVERSATIONS#username
	SK string `theorydb:"sk,attr:SK" json:"SK"` // timestamp#conversationID (for sorting by recent)

	// GSI1 for reverse lookup (find participants by conversation)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK"` // CONVERSATION#conversationID
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK"` // PARTICIPANT#username

	// Per-participant DM metadata (folder/request lifecycle, deletion, unread).
	RequestState DmRequestState `theorydb:"attr:requestState" json:"request_state,omitempty"`
	RequestedAt  *time.Time     `theorydb:"attr:requestedAt" json:"requested_at,omitempty"`
	AcceptedAt   *time.Time     `theorydb:"attr:acceptedAt" json:"accepted_at,omitempty"`
	DeclinedAt   *time.Time     `theorydb:"attr:declinedAt" json:"declined_at,omitempty"`
	DeletedAt    *time.Time     `theorydb:"attr:deletedAt" json:"deleted_at,omitempty"`
	Unread       bool           `theorydb:"attr:unread" json:"unread"`
	LastReadAt   *time.Time     `theorydb:"attr:lastReadAt" json:"last_read_at,omitempty"`

	// ConversationData is the durable embedded snapshot stored in DynamoDB.
	ConversationData *ConversationSnapshot `theorydb:"attr:conversation" json:"-"`

	// Conversation is the hydrated runtime view used by repository callers.
	Conversation *Conversation `theorydb:"-" json:"-"`
}

// TableName returns the DynamoDB table name
func (ConversationParticipantRecord) TableName() string {
	return MainTableName
}

// HydrateConversation rebuilds the runtime conversation model from the durable snapshot.
func (p *ConversationParticipantRecord) HydrateConversation() *Conversation {
	if p == nil {
		return nil
	}

	if p.Conversation == nil {
		p.Conversation = p.ConversationData.ToConversation()
	}
	if p.Conversation != nil {
		p.Conversation.Unread = p.Unread
	}
	return p.Conversation
}

// SyncConversationData refreshes the durable snapshot from the runtime conversation model.
func (p *ConversationParticipantRecord) SyncConversationData() *Conversation {
	if p == nil {
		return nil
	}

	if p.Conversation == nil {
		p.Conversation = p.ConversationData.ToConversation()
	}
	if p.Conversation == nil {
		p.ConversationData = nil
		return nil
	}

	p.Conversation.Unread = p.Unread
	p.ConversationData = ConversationSnapshotFromConversation(p.Conversation)
	return p.Conversation
}

// BeforeCreate sets up the keys for a participant record
func (p *ConversationParticipantRecord) BeforeCreate(participantID string) error {
	conversation := p.SyncConversationData()
	if conversation == nil || conversation.ID == "" {
		return ErrConversationDataRequired
	}

	p.PK = fmt.Sprintf("USER_CONVERSATIONS#%s", participantID)
	p.SK = fmt.Sprintf("%s#%s", conversation.UpdatedAt.Format(time.RFC3339), conversation.ID)
	p.GSI1PK = fmt.Sprintf(KeyPatternConversation, conversation.ID)
	p.GSI1SK = fmt.Sprintf("PARTICIPANT#%s", participantID)

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
