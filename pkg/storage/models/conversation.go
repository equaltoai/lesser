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

// ConversationParticipantType identifies the durable identity class for a
// conversation participant. It is intentionally explicit so remote actor URLs,
// local usernames, and future agent identities do not depend on ID-shape
// inference as the source of truth.
type ConversationParticipantType string

const (
	// ConversationParticipantTypeLocalUser represents a local lesser account.
	ConversationParticipantTypeLocalUser ConversationParticipantType = "local_user"
	// ConversationParticipantTypeRemoteActor represents a federated ActivityPub actor.
	ConversationParticipantTypeRemoteActor ConversationParticipantType = "remote_actor"
)

// ConversationParticipantRef is the typed participant identity stored alongside
// the legacy participant ID list. Participants remains the compatibility field;
// ParticipantRefs is the durable source of truth for federated conversations.
type ConversationParticipantRef struct {
	ParticipantType ConversationParticipantType `theorydb:"attr:participantType" json:"participant_type"`
	ParticipantID   string                      `theorydb:"attr:participantID" json:"participant_id"`
	Acct            string                      `theorydb:"attr:acct,omitempty" json:"acct,omitempty"`
	Domain          string                      `theorydb:"attr:domain,omitempty" json:"domain,omitempty"`
	ResolvedAt      *time.Time                  `theorydb:"attr:resolvedAt,omitempty" json:"resolved_at,omitempty"`
}

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
	ID              string                       `theorydb:"attr:id" json:"id"`
	Participants    []string                     `theorydb:"attr:participants" json:"participants"` // Compatibility IDs: local usernames or remote actor URIs.
	ParticipantRefs []ConversationParticipantRef `theorydb:"attr:participantRefs,omitempty" json:"participant_refs,omitempty"`
	LastStatusID    string                       `theorydb:"attr:lastStatusID" json:"last_status_id,omitempty"`
	Unread          bool                         `theorydb:"-" json:"unread"` // Per-viewer projection only; not stored on the shared row
	CreatedAt       time.Time                    `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt       time.Time                    `theorydb:"attr:updatedAt" json:"updated_at"`

	// Message counting fields
	TotalMessageCount int64     `theorydb:"attr:totalMessageCount" json:"total_message_count"`       // Total messages in conversation
	LastMessageTime   time.Time `theorydb:"attr:lastMessageTime" json:"last_message_time,omitempty"` // Time of last message

	// ViewerState is a runtime-only projection of the canonical per-user DM row that
	// powered a list read. It lets callers build list responses from canonical state
	// without reloading compatibility participant snapshots.
	ViewerState *UserConversationState `theorydb:"-" json:"-"`
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

// NormalizeConversationParticipantRef canonicalizes a typed participant ref for durable storage.
func NormalizeConversationParticipantRef(ref ConversationParticipantRef) ConversationParticipantRef {
	ref.ParticipantID = strings.TrimSpace(ref.ParticipantID)
	ref.Acct = strings.TrimSpace(strings.TrimPrefix(ref.Acct, "@"))
	ref.Domain = strings.ToLower(strings.TrimSpace(ref.Domain))

	switch ref.ParticipantType {
	case ConversationParticipantTypeRemoteActor:
		if ref.Acct != "" {
			ref.Acct = strings.ToLower(ref.Acct)
		}
	case ConversationParticipantTypeLocalUser:
		ref.ParticipantID = CanonicalConversationParticipantID(ref.ParticipantID)
	default:
		if strings.Contains(ref.ParticipantID, "://") {
			ref.ParticipantType = ConversationParticipantTypeRemoteActor
		} else {
			ref.ParticipantType = ConversationParticipantTypeLocalUser
			ref.ParticipantID = CanonicalConversationParticipantID(ref.ParticipantID)
		}
	}

	if ref.ResolvedAt != nil {
		resolvedAt := ref.ResolvedAt.UTC()
		ref.ResolvedAt = &resolvedAt
	}

	return ref
}

// NormalizeConversationParticipantRefs canonicalizes, de-duplicates, and sorts typed participants.
func NormalizeConversationParticipantRefs(refs []ConversationParticipantRef) []ConversationParticipantRef {
	if len(refs) == 0 {
		return nil
	}

	byKey := make(map[string]ConversationParticipantRef, len(refs))
	for _, ref := range refs {
		normalized := NormalizeConversationParticipantRef(ref)
		if normalized.ParticipantID == "" || normalized.ParticipantType == "" {
			continue
		}
		key := string(normalized.ParticipantType) + "\x00" + normalized.ParticipantID
		byKey[key] = normalized
	}

	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalized := make([]ConversationParticipantRef, 0, len(keys))
	for _, key := range keys {
		normalized = append(normalized, byKey[key])
	}
	return normalized
}

// ConversationParticipantIDsFromRefs returns the compatibility participant ID list for typed refs.
func ConversationParticipantIDsFromRefs(refs []ConversationParticipantRef) []string {
	if len(refs) == 0 {
		return nil
	}
	ids := make([]string, 0, len(refs))
	for _, ref := range NormalizeConversationParticipantRefs(refs) {
		if ref.ParticipantID != "" {
			ids = append(ids, ref.ParticipantID)
		}
	}
	return CanonicalConversationParticipants(ids)
}

// ConversationHasRemoteParticipants reports whether the conversation includes a remote actor.
func ConversationHasRemoteParticipants(conversation *Conversation) bool {
	if conversation == nil {
		return false
	}
	for _, ref := range NormalizeConversationParticipantRefs(conversation.ParticipantRefs) {
		if ref.ParticipantType == ConversationParticipantTypeRemoteActor {
			return true
		}
	}
	for _, participantID := range conversation.Participants {
		if strings.Contains(strings.TrimSpace(participantID), "://") {
			return true
		}
	}
	return false
}

// ConversationLocalParticipantIDs returns local-user participants that should own viewer state rows.
func ConversationLocalParticipantIDs(conversation *Conversation) []string {
	if conversation == nil {
		return nil
	}
	if len(conversation.ParticipantRefs) == 0 {
		return CanonicalConversationParticipants(conversation.Participants)
	}

	localParticipants := make([]string, 0, len(conversation.ParticipantRefs))
	for _, ref := range NormalizeConversationParticipantRefs(conversation.ParticipantRefs) {
		if ref.ParticipantType == ConversationParticipantTypeLocalUser && ref.ParticipantID != "" {
			localParticipants = append(localParticipants, ref.ParticipantID)
		}
	}
	return CanonicalConversationParticipants(localParticipants)
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
	UpdatedAt                time.Time      `theorydb:"-" json:"updated_at,omitempty"`

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
