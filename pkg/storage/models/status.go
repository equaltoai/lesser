package models

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
)

// Visibility constants
const (
	VisibilityPublic   = "public"
	VisibilityUnlisted = "unlisted"
	VisibilityPrivate  = "private"
	VisibilityDirect   = "direct"
)

// Status represents an ActivityPub Note/status stored in DynamoDB using DynamORM
type Status struct {
	// Primary key - using status ID as the primary identifier
	PK string `dynamorm:"pk" json:"pk"` // Format: "status#{status_id}"
	SK string `dynamorm:"sk" json:"sk"` // Format: "status#{status_id}"

	// GSI1 - Author timeline (all statuses by an author)
	GSI1PK string `dynamorm:"index:author-timeline-index,pk" json:"gsi1_pk"` // Format: "AUTHOR#{author_id}"
	GSI1SK string `dynamorm:"index:author-timeline-index,sk" json:"gsi1_sk"` // Format: "{published_timestamp}#{status_id}"

	// GSI2 - Public timeline (public statuses)
	GSI2PK string `dynamorm:"index:public-timeline-index,pk" json:"gsi2_pk,omitempty"` // Format: "PUBLIC_TIMELINE"
	GSI2SK string `dynamorm:"index:public-timeline-index,sk" json:"gsi2_sk,omitempty"` // Format: "{published_timestamp}#{status_id}"

	// GSI3 - Conversation/thread tracking
	GSI3PK string `dynamorm:"index:conversation-index,pk" json:"gsi3_pk,omitempty"` // Format: "CONVERSATION#{conversation_id}"
	GSI3SK string `dynamorm:"index:conversation-index,sk" json:"gsi3_sk,omitempty"` // Format: "{published_timestamp}#{status_id}"

	// GSI4 - Replies to a specific status
	GSI4PK string `dynamorm:"index:replies-index,pk" json:"gsi4_pk,omitempty"` // Format: "REPLIES#{parent_status_id}"
	GSI4SK string `dynamorm:"index:replies-index,sk" json:"gsi4_sk,omitempty"` // Format: "{published_timestamp}#{status_id}"

	// GSI5 - Hashtag timeline
	GSI5PK string `dynamorm:"index:hashtag-index,pk" json:"gsi5_pk,omitempty"` // Format: "HASHTAG#{hashtag}"
	GSI5SK string `dynamorm:"index:hashtag-index,sk" json:"gsi5_sk,omitempty"` // Format: "{published_timestamp}#{status_id}"

	// Core status data
	StatusID       string            `json:"status_id"`
	Note           *activitypub.Note `dynamorm:"json" json:"note"`      // The actual ActivityPub Note
	AuthorID       string            `json:"author_id"`                 // AttributedTo from the Note
	AuthorUsername string            `json:"author_username"`           // Extracted username for efficient queries
	Content        string            `json:"content"`                   // Cached content for search
	ConversationID string            `json:"conversation_id,omitempty"` // Thread/conversation ID
	InReplyToID    string            `json:"in_reply_to_id,omitempty"`  // Parent status ID
	ReblogOfID     string            `json:"reblog_of_id,omitempty"`    // If this is a reblog, the original status ID
	Visibility     string            `json:"visibility"`                // public, unlisted, private, direct
	Sensitive      bool              `json:"sensitive"`                 // Content warning flag
	Language       string            `json:"language,omitempty"`        // Content language
	Hashtags       []string          `json:"hashtags,omitempty"`        // Extracted hashtags
	Mentions       []string          `json:"mentions,omitempty"`        // Extracted mentions
	URLs           []string          `json:"urls,omitempty"`            // Extracted URLs
	MediaCount     int               `json:"media_count"`               // Number of media attachments

	// Addressing fields for direct messages and limited visibility
	ToRecipients  []string `json:"to_recipients,omitempty"`  // Primary recipients (visible to all)
	CcRecipients  []string `json:"cc_recipients,omitempty"`  // Carbon copy recipients (visible to all)
	BtoRecipients []string `json:"bto_recipients,omitempty"` // Blind to recipients (hidden from others)
	BccRecipients []string `json:"bcc_recipients,omitempty"` // Blind carbon copy (hidden from all)

	// Engagement metrics (cached for performance)
	LikeCount   int `json:"like_count"`
	ReblogCount int `json:"reblog_count"`
	ReplyCount  int `json:"reply_count"`
	QuoteCount  int `json:"quote_count,omitempty"`

	// Timestamps
	PublishedAt time.Time `json:"published_at"`
	UpdatedAt   time.Time `json:"updated_at,omitempty"`
	CreatedAt   time.Time `dynamorm:"created_at" json:"created_at"`
	ModifiedAt  time.Time `dynamorm:"updated_at" json:"modified_at"`

	// Moderation and flags
	Flagged   bool       `json:"flagged,omitempty"`
	Deleted   bool       `json:"deleted,omitempty"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`

	// Version for optimistic locking
	Version int `dynamorm:"version" json:"version"`
}

// StatusAttachment represents a media attachment on a status
type StatusAttachment struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	MediaType string `json:"media_type,omitempty"`
	Name      string `json:"name,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
}

// StatusTag represents a tag (hashtag or mention) on a status
type StatusTag struct {
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
	Name string `json:"name"`
}

// TableName returns the DynamoDB table name for the Status model
func (Status) TableName() string {
	return MainTableName // Use the main table
}

// BeforeCreate sets up the model before creation
func (s *Status) BeforeCreate() error {
	now := time.Now()
	s.CreatedAt = now
	s.ModifiedAt = now

	// Set published time if not already set
	if s.PublishedAt.IsZero() {
		s.PublishedAt = now
	}

	// Extract data from the Note if present
	if s.Note != nil {
		s.extractFromNote()
	}

	// Set up primary key
	s.PK = "status#" + s.StatusID
	s.SK = "status#" + s.StatusID

	// Set up GSI keys
	s.setupGSIKeys()

	return nil
}

// BeforeUpdate sets up the model before update
func (s *Status) BeforeUpdate() error {
	s.ModifiedAt = time.Now()

	// Update extracted data from Note if present
	if s.Note != nil {
		s.extractFromNote()
	}

	// Update GSI keys in case visibility or other indexed fields changed
	s.setupGSIKeys()

	return nil
}

// extractFromNote extracts searchable data from the ActivityPub Note
func (s *Status) extractFromNote() {
	if s.Note == nil {
		return
	}

	// Extract basic fields
	s.Content = s.Note.Content
	s.AuthorID = s.Note.AttributedTo
	s.Sensitive = s.Note.Sensitive

	// Extract username from author ID
	s.AuthorUsername = extractUsernameFromActorID(s.AuthorID)

	// Extract conversation ID
	s.ConversationID = s.Note.ConversationID

	// Extract in reply to
	if s.Note.InReplyTo != "" {
		s.InReplyToID = extractStatusIDFromURL(s.Note.InReplyTo)
	}

	// Extract addressing fields
	s.ToRecipients = s.Note.To
	s.CcRecipients = s.Note.CC
	s.BtoRecipients = s.Note.BTo
	s.BccRecipients = s.Note.BCC

	// Extract visibility from Note or set default
	if s.Note.Visibility != "" {
		s.Visibility = s.Note.Visibility
	} else {
		s.Visibility = determineVisibilityFromAudience(s.Note.To, s.Note.CC)
	}

	// Extract hashtags and mentions from tags
	s.extractTagsFromNote()

	// Count media attachments
	s.MediaCount = len(s.Note.Attachment)

	// Set published time from Note if available
	if s.Note.Published != nil && !s.Note.Published.IsZero() {
		s.PublishedAt = *s.Note.Published
	}

	// Set updated time from Note if available
	if s.Note.Updated != nil && !s.Note.Updated.IsZero() {
		s.UpdatedAt = *s.Note.Updated
	}
}

// extractTagsFromNote extracts hashtags and mentions from the Note's tags
func (s *Status) extractTagsFromNote() {
	if s.Note == nil || s.Note.Tag == nil {
		return
	}

	s.Hashtags = []string{}
	s.Mentions = []string{}

	for _, tag := range s.Note.Tag {
		switch tag.Type {
		case "Hashtag":
			// Remove # prefix if present
			hashtag := strings.TrimPrefix(tag.Name, "#")
			if hashtag != "" {
				s.Hashtags = append(s.Hashtags, strings.ToLower(hashtag))
			}
		case "Mention":
			if tag.Href != "" {
				s.Mentions = append(s.Mentions, tag.Href)
			}
		}
	}
}

// setupGSIKeys configures all GSI partition and sort keys
func (s *Status) setupGSIKeys() {
	statusID := s.StatusID
	timestamp := s.PublishedAt.Unix()
	timestampStr := fmt.Sprintf("%d", timestamp)

	// GSI1 - Author timeline
	if s.AuthorID != "" {
		s.GSI1PK = "AUTHOR#" + s.AuthorID
		s.GSI1SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	}

	// GSI2 - Public timeline (only for public statuses)
	if s.Visibility == VisibilityPublic {
		s.GSI2PK = "PUBLIC_TIMELINE"
		s.GSI2SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI2PK = ""
		s.GSI2SK = ""
	}

	// GSI3 - Conversation tracking
	if s.ConversationID != "" {
		s.GSI3PK = "CONVERSATION#" + s.ConversationID
		s.GSI3SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI3PK = ""
		s.GSI3SK = ""
	}

	// GSI4 - Replies index
	if s.InReplyToID != "" {
		s.GSI4PK = "REPLIES#" + s.InReplyToID
		s.GSI4SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI4PK = ""
		s.GSI4SK = ""
	}

	// GSI5 - Hashtag index (we'll use the first hashtag for now)
	// In a real implementation, you might want to create multiple records for multiple hashtags
	if len(s.Hashtags) > 0 {
		s.GSI5PK = "HASHTAG#" + s.Hashtags[0]
		s.GSI5SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI5PK = ""
		s.GSI5SK = ""
	}
}

// Helper functions

// extractUsernameFromActorID extracts username from an ActivityPub actor ID
func extractUsernameFromActorID(actorID string) string {
	// Handle different actor ID formats
	// e.g., "https://example.com/users/username" -> "username"
	parts := strings.Split(actorID, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// extractStatusIDFromURL extracts status ID from a status URL
func extractStatusIDFromURL(url string) string {
	// Handle different status URL formats
	// e.g., "https://example.com/users/username/statuses/123" -> "123"
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return ""
}

// determineVisibilityFromAudience determines visibility based on To/CC fields
func determineVisibilityFromAudience(to, cc []string) string {
	publicAddress := "https://www.w3.org/ns/activitystreams#Public"

	// Check if public address is in To field
	for _, addr := range to {
		if addr == publicAddress {
			return VisibilityPublic
		}
	}

	// Check if public address is in CC field
	for _, addr := range cc {
		if addr == publicAddress {
			return VisibilityUnlisted
		}
	}

	// Check if it's addressed to followers only
	hasFollowersCollection := false
	for _, addr := range append(to, cc...) {
		if strings.Contains(addr, "/followers") {
			hasFollowersCollection = true
			break
		}
	}

	// If addressed to followers collection, it's private/followers-only
	if hasFollowersCollection {
		return VisibilityPrivate
	}

	// If only specific actors mentioned and no collections, it's direct
	return VisibilityDirect
}

// IsPublic returns true if the status is publicly visible
func (s *Status) IsPublic() bool {
	return s.Visibility == VisibilityPublic
}

// IsReply returns true if the status is a reply to another status
func (s *Status) IsReply() bool {
	return s.InReplyToID != ""
}

// HasMedia returns true if the status has media attachments
func (s *Status) HasMedia() bool {
	return s.MediaCount > 0
}

// HasHashtags returns true if the status contains hashtags
func (s *Status) HasHashtags() bool {
	return len(s.Hashtags) > 0
}

// HasMentions returns true if the status contains mentions
func (s *Status) HasMentions() bool {
	return len(s.Mentions) > 0
}

// IsDeleted returns true if the status has been deleted
func (s *Status) IsDeleted() bool {
	return s.Deleted
}

// IsFlagged returns true if the status has been flagged for moderation
func (s *Status) IsFlagged() bool {
	return s.Flagged
}

// IsRecipient checks if the given actor ID is a recipient of this status
func (s *Status) IsRecipient(actorID string) bool {
	// Check To recipients
	for _, recipient := range s.ToRecipients {
		if recipient == actorID || strings.Contains(recipient, "/followers") {
			return true
		}
	}

	// Check CC recipients
	for _, recipient := range s.CcRecipients {
		if recipient == actorID || strings.Contains(recipient, "/followers") {
			return true
		}
	}

	// Check BTo recipients (blind to - hidden from others but still a recipient)
	for _, recipient := range s.BtoRecipients {
		if recipient == actorID {
			return true
		}
	}

	// Check BCC recipients (blind carbon copy - hidden from all but still a recipient)
	for _, recipient := range s.BccRecipients {
		if recipient == actorID {
			return true
		}
	}

	return false
}

// IsVisibleTo checks if the status is visible to the given actor ID
func (s *Status) IsVisibleTo(actorID string) bool {
	switch s.Visibility {
	case VisibilityPublic, VisibilityUnlisted:
		return true
	case VisibilityPrivate:
		// Check if actor is a follower or the author
		return actorID == s.AuthorID || s.IsRecipient(actorID)
	case VisibilityDirect:
		// Only visible to author and direct recipients
		return actorID == s.AuthorID || s.IsRecipient(actorID)
	default:
		return false
	}
}

// GetVisibleRecipients returns recipients that should be visible to the requesting actor
func (s *Status) GetVisibleRecipients(requestingActorID string) []string {
	var visible []string

	// Add To recipients (always visible)
	visible = append(visible, s.ToRecipients...)

	// Add CC recipients (always visible)
	visible = append(visible, s.CcRecipients...)

	// BTo recipients are only visible to the recipients themselves, not to others
	if s.IsRecipient(requestingActorID) {
		visible = append(visible, s.BtoRecipients...)
	}

	// BCC recipients are never visible to anyone (not even other recipients)

	return visible
}

// IsDirect returns true if the status is a direct message
func (s *Status) IsDirect() bool {
	return s.Visibility == VisibilityDirect
}

// IsPrivate returns true if the status is private/followers-only
func (s *Status) IsPrivate() bool {
	return s.Visibility == VisibilityPrivate
}

// UpdateKeys updates the GSI keys for this status (required by DynamORM)
func (s *Status) UpdateKeys() {
	s.setupGSIKeys()
}

// ShouldAppearInTimeline checks if status should appear in a given timeline type
func (s *Status) ShouldAppearInTimeline(timelineType string, viewerID string) bool {
	switch timelineType {
	case VisibilityPublic:
		return s.Visibility == VisibilityPublic && !s.Deleted
	case "home":
		// Home timeline includes all visibility levels if viewer has access
		return !s.Deleted && s.IsVisibleTo(viewerID)
	case "conversations":
		// Conversations timeline only includes direct messages
		return s.Visibility == VisibilityDirect && s.IsVisibleTo(viewerID) && !s.Deleted
	case "local":
		// Local timeline includes public posts from local users
		return s.Visibility == VisibilityPublic && !s.Deleted && s.isLocalStatus()
	default:
		return false
	}
}

// isLocalStatus checks if this status is from a local user
func (s *Status) isLocalStatus() bool {
	// This assumes the author ID format includes the domain
	// Adjust logic based on your actual actor ID format
	return strings.Contains(s.AuthorID, os.Getenv("DOMAIN_NAME"))
}

// GetAllRecipients returns all recipients across all addressing fields
func (s *Status) GetAllRecipients() []string {
	recipients := make([]string, 0)
	recipients = append(recipients, s.ToRecipients...)
	recipients = append(recipients, s.CcRecipients...)
	recipients = append(recipients, s.BtoRecipients...)
	recipients = append(recipients, s.BccRecipients...)
	return recipients
}

// HasSpecificRecipients checks if status has specific actor recipients (not just collections)
func (s *Status) HasSpecificRecipients() bool {
	allRecipients := s.GetAllRecipients()
	publicAddr := "https://www.w3.org/ns/activitystreams#Public"

	for _, recipient := range allRecipients {
		// Skip public address and collections
		if recipient != publicAddr && !strings.Contains(recipient, "/followers") {
			return true
		}
	}
	return false
}

// IsConversation returns true if this status is part of a conversation (has replies or is a reply)
func (s *Status) IsConversation() bool {
	return s.InReplyToID != "" || s.ReplyCount > 0
}

// RequiresFollowCheck returns true if visibility requires checking follower relationship
func (s *Status) RequiresFollowCheck() bool {
	return s.Visibility == VisibilityPrivate
}

// GetPrivacyScore returns a numeric score representing how private this status is (higher = more private)
func (s *Status) GetPrivacyScore() int {
	switch s.Visibility {
	case VisibilityPublic:
		return 1
	case VisibilityUnlisted:
		return 2
	case VisibilityPrivate:
		return 3
	case VisibilityDirect:
		return 4
	default:
		return 4 // Default to most private
	}
}

// CanBeReblogged returns true if this status can be reblogged (boosted)
func (s *Status) CanBeReblogged() bool {
	// Direct messages and deleted posts cannot be reblogged
	return s.Visibility != VisibilityDirect && !s.Deleted
}

// SanitizeForActor removes sensitive information based on viewer's relationship to the post
func (s *Status) SanitizeForActor(viewerID string) *Status {
	// Create a copy to avoid modifying the original
	sanitized := *s

	// Remove BCC recipients (never visible to anyone)
	sanitized.BccRecipients = nil

	// Remove BTo recipients if viewer is not a recipient
	if !s.IsRecipient(viewerID) {
		sanitized.BtoRecipients = nil
	}

	return &sanitized
}
