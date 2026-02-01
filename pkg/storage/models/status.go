package models

import (
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
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
	_ struct{} `theorydb:"naming:camelCase"`

	// Primary key - using status ID as the primary identifier
	PK string `theorydb:"pk,attr:PK" json:"pk"` // Format: "status#{status_id}"
	SK string `theorydb:"sk,attr:SK" json:"sk"` // Format: "status#{status_id}"

	// GSI1 - Author timeline (all statuses by an author)
	GSI1PK string `theorydb:"index:gsi1,pk,attr:gsi1PK,omitempty" json:"-"` // Format: "AUTHOR#{author_id}"
	GSI1SK string `theorydb:"index:gsi1,sk,attr:gsi1SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI2 - Public timeline (public statuses)
	GSI2PK string `theorydb:"index:gsi2,pk,attr:gsi2PK,omitempty" json:"-"` // Format: "PUBLIC_TIMELINE"
	GSI2SK string `theorydb:"index:gsi2,sk,attr:gsi2SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI3 - Conversation/thread tracking
	GSI3PK string `theorydb:"index:gsi3,pk,attr:gsi3PK,omitempty" json:"-"` // Format: "CONVERSATION#{conversation_id}"
	GSI3SK string `theorydb:"index:gsi3,sk,attr:gsi3SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI4 - Replies to a specific status
	GSI4PK string `theorydb:"index:gsi4,pk,attr:gsi4PK,omitempty" json:"-"` // Format: "REPLIES#{parent_status_id}"
	GSI4SK string `theorydb:"index:gsi4,sk,attr:gsi4SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI5 - Hashtag timeline
	GSI5PK string `theorydb:"index:gsi5,pk,attr:gsi5PK,omitempty" json:"-"` // Format: "HASHTAG#{hashtag}"
	GSI5SK string `theorydb:"index:gsi5,sk,attr:gsi5SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI6 - Flagged content moderation
	GSI6PK string `theorydb:"index:gsi6,pk,attr:gsi6PK,omitempty" json:"-"` // Format: "FLAGGED_CONTENT"
	GSI6SK string `theorydb:"index:gsi6,sk,attr:gsi6SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// GSI7 - URL index for link searches
	GSI7PK string `theorydb:"index:gsi7,pk,attr:gsi7PK,omitempty" json:"-"` // Format: "URL#{normalized_url}"
	GSI7SK string `theorydb:"index:gsi7,sk,attr:gsi7SK,omitempty" json:"-"` // Format: "{published_timestamp}#{status_id}"

	// Core status data
	StatusID            string     `theorydb:"attr:statusID" json:"status_id"`
	Note                *NoteField `theorydb:"attr:note" json:"note"`                                            // The actual ActivityPub Note (wrapped for proper DynamORM handling)
	AuthorID            string     `theorydb:"attr:authorID" json:"author_id"`                                   // AttributedTo from the Note
	AuthorUsername      string     `theorydb:"attr:authorUsername" json:"author_username"`                       // Extracted username for efficient queries
	Content             string     `theorydb:"attr:content" json:"content"`                                      // Cached content for search
	ConversationID      string     `theorydb:"attr:conversationID" json:"conversation_id,omitempty"`             // Thread/conversation ID
	InReplyToID         string     `theorydb:"attr:inReplyToID" json:"in_reply_to_id,omitempty"`                 // Parent status ID
	ReblogOfID          string     `theorydb:"attr:reblogOfID" json:"reblog_of_id,omitempty"`                    // If this is a reblog, the original status ID
	BoostOfStatusID     string     `theorydb:"attr:boostOfStatusID" json:"boost_of_status_id,omitempty"`         // Original status ID for boost contexts
	BoostOfAuthorID     string     `theorydb:"attr:boostOfAuthorID" json:"boost_of_author_id,omitempty"`         // Author ID of the boosted status
	BoostAnnounceID     string     `theorydb:"attr:boostActivityID" json:"boost_announce_id,omitempty"`          // ActivityPub Announce ID backing this boost (legacy attribute retained)
	QuoteTargetStatusID string     `theorydb:"attr:quoteTargetStatusID" json:"quote_target_status_id,omitempty"` // If this status quotes another, the original status ID
	QuoteTargetAuthorID string     `theorydb:"attr:quoteTargetAuthorID" json:"quote_target_author_id,omitempty"` // Author ID of the quoted status
	Visibility          string     `theorydb:"attr:visibility" json:"visibility"`                                // public, unlisted, private, direct
	Sensitive           bool       `theorydb:"attr:sensitive" json:"sensitive"`                                  // Content warning flag
	Language            string     `theorydb:"attr:language" json:"language,omitempty"`                          // Content language
	Hashtags            []string   `theorydb:"attr:hashtags" json:"hashtags,omitempty"`                          // Extracted hashtags
	Mentions            []string   `theorydb:"attr:mentions" json:"mentions,omitempty"`                          // Extracted mentions
	URLs                []string   `theorydb:"attr:urls" json:"urls,omitempty"`                                  // Extracted URLs
	MediaCount          int        `theorydb:"attr:mediaCount" json:"media_count"`                               // Number of media attachments

	// Addressing fields for direct messages and limited visibility
	ToRecipients  []string `theorydb:"attr:toRecipients" json:"to_recipients,omitempty"`   // Primary recipients (visible to all)
	CcRecipients  []string `theorydb:"attr:ccRecipients" json:"cc_recipients,omitempty"`   // Carbon copy recipients (visible to all)
	BtoRecipients []string `theorydb:"attr:btoRecipients" json:"bto_recipients,omitempty"` // Blind to recipients (hidden from others)
	BccRecipients []string `theorydb:"attr:bccRecipients" json:"bcc_recipients,omitempty"` // Blind carbon copy (hidden from all)

	// Engagement metrics (cached for performance)
	LikeCount   int `theorydb:"attr:likeCount" json:"like_count"`
	ReblogCount int `theorydb:"attr:reblogCount" json:"reblog_count"`
	ReplyCount  int `theorydb:"attr:replyCount" json:"reply_count"`
	QuoteCount  int `theorydb:"attr:quoteCount" json:"quote_count,omitempty"`

	// Timestamps
	PublishedAt time.Time `theorydb:"attr:publishedAt" json:"published_at"`
	UpdatedAt   time.Time `theorydb:"attr:updatedAt" json:"updated_at,omitempty"`
	CreatedAt   time.Time `theorydb:"attr:createdAt" json:"created_at"`
	ModifiedAt  time.Time `theorydb:"attr:modifiedAt" json:"modified_at"`

	// Moderation and flags
	Flagged   bool       `theorydb:"attr:flagged" json:"flagged,omitempty"`
	Deleted   bool       `theorydb:"attr:deleted" json:"deleted,omitempty"`
	DeletedAt *time.Time `theorydb:"attr:deletedAt" json:"deleted_at,omitempty"`

	// Version for optimistic locking
	Version int `theorydb:"version,attr:version" json:"version"`
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

// TableName returns the DynamoDB table backing StatusAttachment.
func (StatusAttachment) TableName() string {
	return MainTableName
}

// StatusTag represents a tag (hashtag or mention) on a status
type StatusTag struct {
	Type string `json:"type"`
	Href string `json:"href,omitempty"`
	Name string `json:"name"`
}

// TableName returns the DynamoDB table backing StatusTag.
func (StatusTag) TableName() string {
	return MainTableName
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
	s.UpdatedAt = now

	s.syncBoostReferenceFields()

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
	now := time.Now()
	s.ModifiedAt = now
	s.UpdatedAt = now

	s.syncBoostReferenceFields()

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

	note := s.Note.Get()
	if note == nil {
		return
	}

	// Extract basic fields
	s.Content = note.Content
	s.AuthorID = note.AttributedTo
	s.Sensitive = note.Sensitive

	// Extract username from author ID
	s.AuthorUsername = extractUsernameFromActorID(s.AuthorID)

	// Extract conversation ID
	s.ConversationID = note.ConversationID

	// Extract in reply to
	if note.InReplyTo != "" {
		s.InReplyToID = extractStatusIDFromURL(note.InReplyTo)
	}

	// Extract addressing fields
	s.ToRecipients = note.To
	s.CcRecipients = note.CC
	s.BtoRecipients = note.BTo
	s.BccRecipients = note.BCC

	// Extract visibility from Note or set default
	if note.Visibility != "" {
		s.Visibility = note.Visibility
	} else {
		s.Visibility = determineVisibilityFromAudience(note.To, note.CC)
	}

	// Extract hashtags and mentions from tags
	s.extractTagsFromNote()

	// Count media attachments
	s.MediaCount = len(note.Attachment)

	// Set published time from Note if available
	if note.Published != nil && !note.Published.IsZero() {
		s.PublishedAt = *note.Published
	}

	// Set updated time from Note if available
	if note.Updated != nil && !note.Updated.IsZero() {
		s.UpdatedAt = *note.Updated
	}
}

// extractTagsFromNote extracts hashtags and mentions from the Note's tags
func (s *Status) extractTagsFromNote() {
	if s.Note == nil {
		return
	}

	note := s.Note.Get()
	if note == nil || note.Tag == nil {
		return
	}

	s.Hashtags = []string{}
	s.Mentions = []string{}

	for _, tag := range note.Tag {
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

	// GSI5 - Primary hashtag index (use the first hashtag for primary record)
	// Additional hashtag records will be created separately via CreateHashtagIndexRecords
	if len(s.Hashtags) > 0 {
		s.GSI5PK = "HASHTAG#" + s.Hashtags[0]
		s.GSI5SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI5PK = ""
		s.GSI5SK = ""
	}

	// GSI6 - Flagged content moderation (only for flagged statuses)
	if s.Flagged {
		s.GSI6PK = "FLAGGED_CONTENT"
		s.GSI6SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI6PK = ""
		s.GSI6SK = ""
	}

	// GSI7 - URL index (use the first URL for primary record)
	// Additional URL records will be created separately if needed
	if len(s.URLs) > 0 {
		// Normalize the URL for consistent indexing
		normalizedURL := strings.ToLower(strings.TrimSpace(s.URLs[0]))
		s.GSI7PK = "URL#" + normalizedURL
		s.GSI7SK = fmt.Sprintf("%s#%s", timestampStr, statusID)
	} else {
		s.GSI7PK = ""
		s.GSI7SK = ""
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

// IsReblog returns true if the status is a reblog/boost of another status
func (s *Status) IsReblog() bool {
	return s.ReblogOfID != "" || s.BoostOfStatusID != ""
}

// syncBoostReferenceFields keeps boost metadata aligned so repositories can query consistently.
func (s *Status) syncBoostReferenceFields() {
	if s == nil {
		return
	}

	if s.BoostOfStatusID == "" {
		s.BoostOfStatusID = s.ReblogOfID
	}
	if s.ReblogOfID == "" {
		s.ReblogOfID = s.BoostOfStatusID
	}
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
func (s *Status) UpdateKeys() error {
	if err := common.ValidateRequiredParam("status.StatusID", s.StatusID); err != nil {
		return err
	}

	// Ensure derived fields stay in sync with the note payload
	if s.Note != nil {
		s.extractFromNote()
	}

	s.PK = "status#" + s.StatusID
	s.SK = "status#" + s.StatusID

	if s.PublishedAt.IsZero() {
		s.PublishedAt = time.Now()
	}

	s.setupGSIKeys()
	return nil
}

// GetPK returns the primary key for BaseRepository interface
func (s *Status) GetPK() string {
	return s.PK
}

// GetSK returns the sort key for BaseRepository interface
func (s *Status) GetSK() string {
	return s.SK
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
	cfg := config.Get()
	return strings.Contains(s.AuthorID, cfg.Domain)
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

// StatusHashtagIndex represents a separate record for each hashtag in a status
type StatusHashtagIndex struct {
	PK string `theorydb:"pk" json:"pk"` // Format: "HASHTAG_INDEX#{hashtag}"
	SK string `theorydb:"sk" json:"sk"` // Format: "{published_timestamp}#{status_id}"

	// Reference back to the original status
	StatusID    string    `json:"status_id"`
	AuthorID    string    `json:"author_id"`
	Hashtag     string    `json:"hashtag"`
	PublishedAt time.Time `json:"published_at"`
	Visibility  string    `json:"visibility"`

	// TTL for cleanup (optional - could be set to expire old hashtag indices)
	TTL int64 `json:"ttl,omitempty" theorydb:"ttl"`
}

// TableName returns the DynamoDB table backing StatusHashtagIndex.
func (StatusHashtagIndex) TableName() string {
	return MainTableName
}

// UpdateKeys sets the DynamoDB keys for hashtag index records
func (shi *StatusHashtagIndex) UpdateKeys() {
	timestampStr := shi.PublishedAt.Format("2006-01-02T15:04:05.000Z")
	shi.PK = fmt.Sprintf("HASHTAG_INDEX#%s", shi.Hashtag)
	shi.SK = fmt.Sprintf("%s#%s", timestampStr, shi.StatusID)

	// Set TTL to 1 year for hashtag indices (can be adjusted based on retention policy)
	shi.TTL = time.Now().Add(365 * 24 * time.Hour).Unix()
}

// CreateHashtagIndexRecords creates separate index records for each hashtag in the status
func (s *Status) CreateHashtagIndexRecords() []*StatusHashtagIndex {
	if len(s.Hashtags) <= 1 {
		// No additional records needed - primary record handles single hashtag
		return nil
	}

	// Create index records for hashtags 2 and beyond (first hashtag is handled by primary record)
	records := make([]*StatusHashtagIndex, 0, len(s.Hashtags)-1)

	for i := 1; i < len(s.Hashtags); i++ {
		hashtag := s.Hashtags[i]
		if err := common.ValidateRequiredParam("hashtag", hashtag); err != nil {
			continue
		}

		record := &StatusHashtagIndex{
			StatusID:    s.StatusID,
			AuthorID:    s.AuthorID,
			Hashtag:     hashtag,
			PublishedAt: s.PublishedAt,
			Visibility:  s.Visibility,
		}
		record.UpdateKeys()
		records = append(records, record)
	}

	return records
}

// GetAllHashtagIndexRecords creates index records for ALL hashtags (including the first one)
// This is useful when you need separate records for all hashtags regardless of the primary record
func (s *Status) GetAllHashtagIndexRecords() []*StatusHashtagIndex {
	if err := common.ValidateSliceNotEmpty("s.Hashtags", s.Hashtags); err != nil {
		return nil
	}

	records := make([]*StatusHashtagIndex, 0, len(s.Hashtags))

	for _, hashtag := range s.Hashtags {
		if err := common.ValidateRequiredParam("hashtag", hashtag); err != nil {
			continue
		}

		record := &StatusHashtagIndex{
			StatusID:    s.StatusID,
			AuthorID:    s.AuthorID,
			Hashtag:     hashtag,
			PublishedAt: s.PublishedAt,
			Visibility:  s.Visibility,
		}
		record.UpdateKeys()
		records = append(records, record)
	}

	return records
}

// DeleteHashtagIndexRecords creates delete operations for all hashtag index records
func (s *Status) DeleteHashtagIndexRecords() []map[string]interface{} {
	if err := common.ValidateSliceNotEmpty("s.Hashtags", s.Hashtags); err != nil {
		return nil
	}

	deleteOps := make([]map[string]interface{}, 0, len(s.Hashtags))
	timestampStr := s.PublishedAt.Format("2006-01-02T15:04:05.000Z")

	for _, hashtag := range s.Hashtags {
		if err := common.ValidateRequiredParam("hashtag", hashtag); err != nil {
			continue
		}

		deleteOp := map[string]interface{}{
			"PK": fmt.Sprintf("HASHTAG_INDEX#%s", hashtag),
			"SK": fmt.Sprintf("%s#%s", timestampStr, s.StatusID),
		}
		deleteOps = append(deleteOps, deleteOp)
	}

	return deleteOps
}
