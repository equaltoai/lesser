package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
)

// UserConversationFolder identifies the viewer-visible placement of a DM thread.
type UserConversationFolder string

const (
	// UserConversationFolderInbox is the normal visible inbox placement for a DM thread.
	UserConversationFolderInbox UserConversationFolder = "INBOX"
	// UserConversationFolderRequests is the visible folder for inbound DM requests.
	UserConversationFolderRequests UserConversationFolder = "REQUESTS"
	// UserConversationFolderDeclined is the visible folder for declined DM requests.
	UserConversationFolderDeclined UserConversationFolder = "DECLINED"
	// UserConversationFolderHidden is the hidden/tombstoned viewer-specific folder.
	UserConversationFolderHidden UserConversationFolder = "HIDDEN"
)

// UserConversationState is the canonical per-user DM state row.
type UserConversationState struct {
	_ struct{} `theorydb:"naming:camelCase"`

	PK string `theorydb:"pk,attr:PK" json:"PK"`
	SK string `theorydb:"sk,attr:SK" json:"SK"`

	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"gsi1PK,omitempty"`
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"gsi1SK,omitempty"`
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"gsi2PK,omitempty"`
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"gsi2SK,omitempty"`
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"gsi3PK,omitempty"`
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"gsi3SK,omitempty"`

	ViewerID                 string                      `theorydb:"attr:viewerID" json:"viewer_id"`
	ConversationID           string                      `theorydb:"attr:conversationID" json:"conversation_id"`
	CounterpartID            string                      `theorydb:"attr:counterpartID" json:"counterpart_id"`
	CounterpartType          ConversationParticipantType `theorydb:"attr:counterpartType,omitempty" json:"counterpart_type,omitempty"`
	CounterpartAcct          string                      `theorydb:"attr:counterpartAcct,omitempty" json:"counterpart_acct,omitempty"`
	CounterpartDomain        string                      `theorydb:"attr:counterpartDomain,omitempty" json:"counterpart_domain,omitempty"`
	CounterpartResolvedAt    *time.Time                  `theorydb:"attr:counterpartResolvedAt,omitempty" json:"counterpart_resolved_at,omitempty"`
	Folder                   UserConversationFolder      `theorydb:"attr:folder" json:"folder"`
	RequestState             DmRequestState              `theorydb:"attr:requestState,omitempty" json:"request_state,omitempty"`
	PreviewStatusID          string                      `theorydb:"attr:previewStatusID,omitempty" json:"preview_status_id,omitempty"`
	PreviewStatusPublishedAt time.Time                   `theorydb:"attr:previewStatusPublishedAt,omitempty" json:"preview_status_published_at,omitempty"`
	SortAt                   time.Time                   `theorydb:"attr:sortAt" json:"sort_at"`
	Unread                   bool                        `theorydb:"attr:unread" json:"unread"`
	UnreadCount              int                         `theorydb:"attr:unreadCount" json:"unread_count"`
	LastReadAt               *time.Time                  `theorydb:"attr:lastReadAt" json:"last_read_at,omitempty"`
	DeletedAt                *time.Time                  `theorydb:"attr:deletedAt" json:"deleted_at,omitempty"`
	RequestedAt              *time.Time                  `theorydb:"attr:requestedAt" json:"requested_at,omitempty"`
	AcceptedAt               *time.Time                  `theorydb:"attr:acceptedAt" json:"accepted_at,omitempty"`
	DeclinedAt               *time.Time                  `theorydb:"attr:declinedAt" json:"declined_at,omitempty"`
	CreatedAt                time.Time                   `theorydb:"attr:createdAt" json:"created_at"`
	UpdatedAt                time.Time                   `theorydb:"attr:updatedAt" json:"updated_at"`
}

// TableName returns the DynamoDB table name.
func (UserConversationState) TableName() string {
	return MainTableName
}

// BeforeCreate initializes keys and timestamps before create.
func (s *UserConversationState) BeforeCreate() error {
	if err := s.prepareForWrite(true); err != nil {
		return err
	}
	return s.UpdateKeys()
}

// BeforeUpdate refreshes keys and timestamps before update.
func (s *UserConversationState) BeforeUpdate() error {
	if err := s.prepareForWrite(false); err != nil {
		return err
	}
	return s.UpdateKeys()
}

func (s *UserConversationState) prepareForWrite(isCreate bool) error {
	if s == nil {
		return ErrConversationDataRequired
	}
	s.ViewerID = CanonicalConversationParticipantID(s.ViewerID)
	s.CounterpartID = CanonicalConversationParticipantID(s.CounterpartID)
	s.CounterpartAcct = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s.CounterpartAcct, "@")))
	s.CounterpartDomain = strings.ToLower(strings.TrimSpace(s.CounterpartDomain))
	s.ConversationID = strings.TrimSpace(s.ConversationID)

	if err := common.ValidateRequiredParam("ViewerID", s.ViewerID); err != nil {
		return ErrConversationViewerIDRequired
	}
	if err := common.ValidateRequiredParam("ConversationID", s.ConversationID); err != nil {
		return ErrConversationIDRequired
	}
	if err := common.ValidateRequiredParam("CounterpartID", s.CounterpartID); err != nil {
		return ErrConversationCounterpartIDRequired
	}
	if err := common.ValidateRequiredParam("Folder", string(s.Folder)); err != nil {
		return ErrConversationFolderRequired
	}

	now := time.Now().UTC()
	if isCreate && s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	s.CreatedAt = s.CreatedAt.UTC()

	if s.UpdatedAt.IsZero() {
		s.UpdatedAt = now
	}
	s.UpdatedAt = s.UpdatedAt.UTC()

	if s.PreviewStatusPublishedAt.IsZero() {
		s.PreviewStatusPublishedAt = time.Time{}
	} else {
		s.PreviewStatusPublishedAt = s.PreviewStatusPublishedAt.UTC()
	}

	if s.SortAt.IsZero() {
		switch {
		case !s.PreviewStatusPublishedAt.IsZero():
			s.SortAt = s.PreviewStatusPublishedAt
		case !s.UpdatedAt.IsZero():
			s.SortAt = s.UpdatedAt
		default:
			s.SortAt = now
		}
	}
	s.SortAt = s.SortAt.UTC()

	s.LastReadAt = normalizeOptionalConversationTime(s.LastReadAt)
	s.DeletedAt = normalizeOptionalConversationTime(s.DeletedAt)
	s.RequestedAt = normalizeOptionalConversationTime(s.RequestedAt)
	s.AcceptedAt = normalizeOptionalConversationTime(s.AcceptedAt)
	s.DeclinedAt = normalizeOptionalConversationTime(s.DeclinedAt)
	s.CounterpartResolvedAt = normalizeOptionalConversationTime(s.CounterpartResolvedAt)

	return nil
}

func normalizeOptionalConversationTime(value *time.Time) *time.Time {
	if value == nil || value.IsZero() {
		return nil
	}
	t := value.UTC()
	return &t
}

// UpdateKeys recalculates the primary and index keys from canonical row fields.
func (s *UserConversationState) UpdateKeys() error {
	if s == nil {
		return ErrConversationDataRequired
	}
	if strings := []struct {
		value string
		err   error
	}{
		{value: s.ViewerID, err: ErrConversationViewerIDRequired},
		{value: s.ConversationID, err: ErrConversationIDRequired},
		{value: s.CounterpartID, err: ErrConversationCounterpartIDRequired},
		{value: string(s.Folder), err: ErrConversationFolderRequired},
	}; true {
		for _, item := range strings {
			if err := common.ValidateRequiredParam("value", item.value); err != nil {
				return item.err
			}
		}
	}

	s.PK = fmt.Sprintf("USER_CONVERSATION_STATE#%s", s.ViewerID)
	s.SK = fmt.Sprintf("CONVERSATION#%s", s.ConversationID)
	s.GSI1PK = fmt.Sprintf("USER_CONVERSATION_FOLDER#%s#%s", s.ViewerID, s.Folder)
	s.GSI1SK = fmt.Sprintf("%s#%s", s.SortAt.Format(time.RFC3339Nano), s.ConversationID)

	if s.Unread && s.UnreadQueryVisible() {
		s.GSI2PK = fmt.Sprintf("USER_CONVERSATION_UNREAD#%s", s.ViewerID)
		s.GSI2SK = s.GSI1SK
	} else {
		s.GSI2PK = ""
		s.GSI2SK = ""
	}

	s.GSI3PK = fmt.Sprintf("CONVERSATION#%s", s.ConversationID)
	s.GSI3SK = fmt.Sprintf("USER#%s", s.ViewerID)

	return nil
}

// GetPK returns the partition key.
func (s *UserConversationState) GetPK() string {
	return s.PK
}

// GetSK returns the sort key.
func (s *UserConversationState) GetSK() string {
	return s.SK
}

// UnreadQueryVisible reports whether the row should participate in the sparse unread index.
func (s *UserConversationState) UnreadQueryVisible() bool {
	if s == nil {
		return false
	}
	if s.DeletedAt != nil && !s.DeletedAt.IsZero() {
		return false
	}
	return s.Folder == UserConversationFolderInbox || s.Folder == UserConversationFolderRequests
}

// LegacyListCursor returns the sortable cursor shape historically used by conversation list reads.
func (s *UserConversationState) LegacyListCursor() string {
	if s == nil {
		return ""
	}
	return fmt.Sprintf("%s#%s", s.SortAt.Format(time.RFC3339Nano), s.ConversationID)
}
