package models

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// StatusModelTestSuite contains tests for Status model
type StatusModelTestSuite struct {
	suite.Suite
}

// TableName returns the DynamoDB table backing StatusModelTestSuite.
func (StatusModelTestSuite) TableName() string {
	return MainTableName
}

// Test TableName
func (suite *StatusModelTestSuite) TestTableName() {
	status := &Status{}
	assert.Equal(suite.T(), "lesser-main", status.TableName())
}

// Test BeforeCreate
func (suite *StatusModelTestSuite) TestBeforeCreate_SetsTimestamps() {
	status := &Status{
		StatusID: "123",
	}

	err := status.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.False(suite.T(), status.CreatedAt.IsZero())
	assert.False(suite.T(), status.ModifiedAt.IsZero())
	assert.False(suite.T(), status.PublishedAt.IsZero())
	assert.WithinDuration(suite.T(), time.Now(), status.CreatedAt, time.Second)
	assert.WithinDuration(suite.T(), time.Now(), status.ModifiedAt, time.Second)
	assert.WithinDuration(suite.T(), time.Now(), status.PublishedAt, time.Second)
}

func (suite *StatusModelTestSuite) TestBeforeCreate_SetsPrimaryKeys() {
	status := &Status{
		StatusID: "123",
	}

	err := status.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "status#123", status.PK)
	assert.Equal(suite.T(), "status#123", status.SK)
}

func (suite *StatusModelTestSuite) TestBeforeCreate_ExtractsFromNote() {
	now := time.Now()
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/123",
			Published: &now,
			InReplyTo: "https://example.com/users/bob/statuses/456",
			Sensitive: true,
			To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Content:      "Hello, world! #test @bob",
		AttributedTo: "https://example.com/users/alice",
		Tag: []activitypub.Tag{
			{
				Type: "Hashtag",
				Name: "#test",
			},
			{
				Type: "Mention",
				Href: "https://example.com/users/bob",
			},
		},
		Attachment: []activitypub.Attachment{
			{
				Type: "Image",
				URL:  "https://example.com/media/123.jpg",
			},
		},
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	err := status.BeforeCreate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Hello, world! #test @bob", status.Content)
	assert.Equal(suite.T(), "https://example.com/users/alice", status.AuthorID)
	assert.Equal(suite.T(), "alice", status.AuthorUsername)
	assert.Equal(suite.T(), "456", status.InReplyToID)
	assert.Equal(suite.T(), "public", status.Visibility)
	assert.True(suite.T(), status.Sensitive)
	assert.Equal(suite.T(), 1, status.MediaCount)
	assert.Equal(suite.T(), []string{"test"}, status.Hashtags)
	assert.Equal(suite.T(), []string{"https://example.com/users/bob"}, status.Mentions)
	assert.Equal(suite.T(), now, status.PublishedAt)
}

// Test BeforeUpdate
func (suite *StatusModelTestSuite) TestBeforeUpdate_UpdatesTimestamp() {
	status := &Status{
		StatusID:   "123",
		CreatedAt:  time.Now().Add(-time.Hour),
		ModifiedAt: time.Now().Add(-time.Hour),
	}

	err := status.BeforeUpdate()

	assert.NoError(suite.T(), err)
	assert.WithinDuration(suite.T(), time.Now(), status.ModifiedAt, time.Second)
}

func (suite *StatusModelTestSuite) TestBeforeUpdate_ExtractsFromNote() {
	now := time.Now()
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/123",
			Published: &now,
			Updated:   &now,
			To:        []string{},
			CC:        []string{"https://www.w3.org/ns/activitystreams#Public"},
		},
		Content:      "Updated content",
		AttributedTo: "https://example.com/users/alice",
		Visibility:   "unlisted", // Explicit visibility
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	err := status.BeforeUpdate()

	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), "Updated content", status.Content)
	assert.Equal(suite.T(), "unlisted", status.Visibility)
	assert.Equal(suite.T(), now, status.UpdatedAt)
}

// Test setupGSIKeys
func (suite *StatusModelTestSuite) TestSetupGSIKeys_AuthorTimeline() {
	status := &Status{
		StatusID:    "123",
		AuthorID:    "alice",
		PublishedAt: time.Unix(1609459200, 0), // 2021-01-01 00:00:00 UTC
	}

	status.setupGSIKeys()

	assert.Equal(suite.T(), "AUTHOR#alice", status.GSI1PK)
	assert.Equal(suite.T(), "1609459200#123", status.GSI1SK)
}

func (suite *StatusModelTestSuite) TestSetupGSIKeys_PublicTimeline() {
	testCases := []struct {
		name       string
		visibility string
		expectGSI2 bool
	}{
		{"public status", "public", true},
		{"unlisted status", "unlisted", false},
		{"private status", "private", false},
		{"direct status", "direct", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{
				StatusID:    "123",
				Visibility:  tc.visibility,
				PublishedAt: time.Unix(1609459200, 0), // 2021-01-01 00:00:00 UTC
			}

			status.setupGSIKeys()

			if tc.expectGSI2 {
				assert.Equal(t, "PUBLIC_TIMELINE", status.GSI2PK)
				assert.Equal(t, "1609459200#123", status.GSI2SK)
			} else {
				assert.Empty(t, status.GSI2PK)
				assert.Empty(t, status.GSI2SK)
			}
		})
	}
}

func (suite *StatusModelTestSuite) TestSetupGSIKeys_ConversationIndex() {
	testCases := []struct {
		name           string
		conversationID string
		expectGSI3     bool
	}{
		{"with conversation", "conv123", true},
		{"without conversation", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{
				StatusID:       "123",
				ConversationID: tc.conversationID,
				PublishedAt:    time.Unix(1609459200, 0), // 2021-01-01 00:00:00 UTC
			}

			status.setupGSIKeys()

			if tc.expectGSI3 {
				assert.Equal(t, "CONVERSATION#conv123", status.GSI3PK)
				assert.Equal(t, "1609459200#123", status.GSI3SK)
			} else {
				assert.Empty(t, status.GSI3PK)
				assert.Empty(t, status.GSI3SK)
			}
		})
	}
}

func (suite *StatusModelTestSuite) TestSetupGSIKeys_RepliesIndex() {
	testCases := []struct {
		name        string
		inReplyToID string
		expectGSI4  bool
	}{
		{"reply status", "456", true},
		{"original status", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{
				StatusID:    "123",
				InReplyToID: tc.inReplyToID,
				PublishedAt: time.Unix(1609459200, 0), // 2021-01-01 00:00:00 UTC
			}

			status.setupGSIKeys()

			if tc.expectGSI4 {
				assert.Equal(t, "REPLIES#456", status.GSI4PK)
				assert.Equal(t, "1609459200#123", status.GSI4SK)
			} else {
				assert.Empty(t, status.GSI4PK)
				assert.Empty(t, status.GSI4SK)
			}
		})
	}
}

func (suite *StatusModelTestSuite) TestSetupGSIKeys_HashtagIndex() {
	testCases := []struct {
		name       string
		hashtags   []string
		expectGSI5 bool
	}{
		{"with hashtags", []string{"test", "golang"}, true},
		{"without hashtags", []string{}, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{
				StatusID:    "123",
				Hashtags:    tc.hashtags,
				PublishedAt: time.Unix(1609459200, 0), // 2021-01-01 00:00:00 UTC
			}

			status.setupGSIKeys()

			if tc.expectGSI5 {
				assert.Equal(t, "HASHTAG#test", status.GSI5PK) // First hashtag used
				assert.Equal(t, "1609459200#123", status.GSI5SK)
			} else {
				assert.Empty(t, status.GSI5PK)
				assert.Empty(t, status.GSI5SK)
			}
		})
	}
}

// Test extractFromNote
func (suite *StatusModelTestSuite) TestExtractFromNote_NilNote() {
	status := &Status{
		StatusID: "123",
		Note:     nil,
	}

	status.extractFromNote()

	// Should not change anything
	assert.Empty(suite.T(), status.Content)
	assert.Empty(suite.T(), status.AuthorID)
	assert.Empty(suite.T(), status.Visibility)
}

func (suite *StatusModelTestSuite) TestExtractFromNote_BasicFields() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/123",
			Sensitive: true,
		},
		Content:      "Test content",
		AttributedTo: "https://example.com/users/alice",
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractFromNote()

	assert.Equal(suite.T(), "Test content", status.Content)
	assert.Equal(suite.T(), "https://example.com/users/alice", status.AuthorID)
	assert.Equal(suite.T(), "alice", status.AuthorUsername)
	assert.True(suite.T(), status.Sensitive)
}

func (suite *StatusModelTestSuite) TestExtractFromNote_ConversationAndReply() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/123",
			InReplyTo: "https://example.com/users/bob/statuses/456",
		},
		Content:        "Reply content",
		AttributedTo:   "https://example.com/users/alice",
		ConversationID: "conv789",
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractFromNote()

	assert.Equal(suite.T(), "conv789", status.ConversationID)
	assert.Equal(suite.T(), "456", status.InReplyToID)
}

func (suite *StatusModelTestSuite) TestExtractFromNote_Visibility() {
	testCases := []struct {
		name       string
		to         []string
		cc         []string
		visibility string
		expected   string
	}{
		{
			name:       "explicit visibility",
			visibility: "unlisted",
			expected:   "unlisted",
		},
		{
			name:     "public in To",
			to:       []string{"https://www.w3.org/ns/activitystreams#Public"},
			expected: "public",
		},
		{
			name:     "public in CC",
			cc:       []string{"https://www.w3.org/ns/activitystreams#Public"},
			expected: "unlisted",
		},
		{
			name:     "direct message",
			to:       []string{"https://example.com/users/bob"},
			expected: "direct",
		},
		{
			name:     "private message",
			to:       []string{"https://example.com/users/bob", "https://example.com/users/charlie"},
			expected: "private",
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			note := &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID: "https://example.com/users/alice/statuses/123",
					To: tc.to,
					CC: tc.cc,
				},
				Content:      "Test content",
				AttributedTo: "https://example.com/users/alice",
				Visibility:   tc.visibility,
			}

			status := &Status{
				StatusID: "123",
				Note:     &NoteField{Note: note},
			}

			status.extractFromNote()

			assert.Equal(t, tc.expected, status.Visibility)
		})
	}
}

func (suite *StatusModelTestSuite) TestExtractFromNote_Tags() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice/statuses/123",
		},
		Content:      "Test #hashtag @mention",
		AttributedTo: "https://example.com/users/alice",
		Tag: []activitypub.Tag{
			{
				Type: "Hashtag",
				Name: "#hashtag",
			},
			{
				Type: "Hashtag",
				Name: "noprefix", // No # prefix
			},
			{
				Type: "Mention",
				Href: "https://example.com/users/bob",
			},
		},
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractFromNote()

	assert.Len(suite.T(), status.Hashtags, 2)
	assert.Contains(suite.T(), status.Hashtags, "hashtag")
	assert.Contains(suite.T(), status.Hashtags, "noprefix")
	assert.Len(suite.T(), status.Mentions, 1)
	assert.Contains(suite.T(), status.Mentions, "https://example.com/users/bob")
}

func (suite *StatusModelTestSuite) TestExtractFromNote_MediaAttachments() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice/statuses/123",
		},
		Content:      "Test with media",
		AttributedTo: "https://example.com/users/alice",
		Attachment: []activitypub.Attachment{
			{
				Type: "Image",
				URL:  "https://example.com/media/1.jpg",
			},
			{
				Type: "Video",
				URL:  "https://example.com/media/2.mp4",
			},
		},
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractFromNote()

	assert.Equal(suite.T(), 2, status.MediaCount)
}

func (suite *StatusModelTestSuite) TestExtractFromNote_Timestamps() {
	published := time.Now().Add(-time.Hour)
	updated := time.Now().Add(-30 * time.Minute)

	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID:        "https://example.com/users/alice/statuses/123",
			Published: &published,
			Updated:   &updated,
		},
		Content:      "Test with timestamps",
		AttributedTo: "https://example.com/users/alice",
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractFromNote()

	assert.Equal(suite.T(), published, status.PublishedAt)
	assert.Equal(suite.T(), updated, status.UpdatedAt)
}

// Test extractTagsFromNote
func (suite *StatusModelTestSuite) TestExtractTagsFromNote_NilNote() {
	status := &Status{
		StatusID: "123",
		Note:     nil,
	}

	status.extractTagsFromNote()

	assert.Nil(suite.T(), status.Hashtags)
	assert.Nil(suite.T(), status.Mentions)
}

func (suite *StatusModelTestSuite) TestExtractTagsFromNote_NilTags() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice/statuses/123",
		},
		Content:      "Test without tags",
		AttributedTo: "https://example.com/users/alice",
		Tag:          nil,
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractTagsFromNote()

	assert.Empty(suite.T(), status.Hashtags)
	assert.Empty(suite.T(), status.Mentions)
}

func (suite *StatusModelTestSuite) TestExtractTagsFromNote_MixedTags() {
	note := &activitypub.Note{
		BaseObject: activitypub.BaseObject{
			ID: "https://example.com/users/alice/statuses/123",
		},
		Content:      "Test with mixed tags",
		AttributedTo: "https://example.com/users/alice",
		Tag: []activitypub.Tag{
			{
				Type: "Hashtag",
				Name: "#first",
			},
			{
				Type: "Mention",
				Href: "https://example.com/users/bob",
			},
			{
				Type: "Hashtag",
				Name: "#SECOND", // Test case normalization
			},
			{
				Type: "Mention",
				Href: "", // Empty href should be ignored
			},
			{
				Type: "Hashtag",
				Name: "", // Empty name should be ignored
			},
			{
				Type: "Unknown", // Unknown type should be ignored
				Name: "test",
			},
		},
	}

	status := &Status{
		StatusID: "123",
		Note:     &NoteField{Note: note},
	}

	status.extractTagsFromNote()

	assert.Len(suite.T(), status.Hashtags, 2)
	assert.Contains(suite.T(), status.Hashtags, "first")
	assert.Contains(suite.T(), status.Hashtags, "second") // Should be lowercase
	assert.Len(suite.T(), status.Mentions, 1)
	assert.Contains(suite.T(), status.Mentions, "https://example.com/users/bob")
}

// Test helper functions
func (suite *StatusModelTestSuite) TestExtractUsernameFromActorID() {
	testCases := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "standard ActivityPub URL",
			actorID:  "https://example.com/users/alice",
			expected: "alice",
		},
		{
			name:     "URL with trailing slash",
			actorID:  "https://example.com/users/bob/",
			expected: "",
		},
		{
			name:     "simple username",
			actorID:  "charlie",
			expected: "charlie",
		},
		{
			name:     "empty string",
			actorID:  "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			result := extractUsernameFromActorID(tc.actorID)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func (suite *StatusModelTestSuite) TestDetermineVisibilityFromAudience() {
	publicAddress := "https://www.w3.org/ns/activitystreams#Public"

	testCases := []struct {
		name     string
		to       []string
		cc       []string
		expected string
	}{
		{
			name:     "public in To",
			to:       []string{publicAddress},
			expected: "public",
		},
		{
			name:     "public in CC",
			cc:       []string{publicAddress},
			expected: "unlisted",
		},
		{
			name:     "direct message",
			to:       []string{"https://example.com/users/bob"},
			expected: "direct",
		},
		{
			name:     "private message",
			to:       []string{"https://example.com/users/bob", "https://example.com/users/charlie"},
			expected: "private",
		},
		{
			name:     "empty audience",
			to:       []string{},
			cc:       []string{},
			expected: "private",
		},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			result := determineVisibilityFromAudience(tc.to, tc.cc)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// Test boolean helper methods
func (suite *StatusModelTestSuite) TestIsPublic() {
	testCases := []struct {
		name       string
		visibility string
		expected   bool
	}{
		{"public status", "public", true},
		{"unlisted status", "unlisted", false},
		{"private status", "private", false},
		{"direct status", "direct", false},
		{"empty visibility", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{Visibility: tc.visibility}
			assert.Equal(t, tc.expected, status.IsPublic())
		})
	}
}

func (suite *StatusModelTestSuite) TestIsReply() {
	testCases := []struct {
		name        string
		inReplyToID string
		expected    bool
	}{
		{"reply status", "123", true},
		{"original status", "", false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{InReplyToID: tc.inReplyToID}
			assert.Equal(t, tc.expected, status.IsReply())
		})
	}
}

func (suite *StatusModelTestSuite) TestHasMedia() {
	testCases := []struct {
		name       string
		mediaCount int
		expected   bool
	}{
		{"with media", 2, true},
		{"without media", 0, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{MediaCount: tc.mediaCount}
			assert.Equal(t, tc.expected, status.HasMedia())
		})
	}
}

func (suite *StatusModelTestSuite) TestHasHashtags() {
	testCases := []struct {
		name     string
		hashtags []string
		expected bool
	}{
		{"with hashtags", []string{"test"}, true},
		{"without hashtags", []string{}, false},
		{"nil hashtags", nil, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{Hashtags: tc.hashtags}
			assert.Equal(t, tc.expected, status.HasHashtags())
		})
	}
}

func (suite *StatusModelTestSuite) TestHasMentions() {
	testCases := []struct {
		name     string
		mentions []string
		expected bool
	}{
		{"with mentions", []string{"https://example.com/users/bob"}, true},
		{"without mentions", []string{}, false},
		{"nil mentions", nil, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{Mentions: tc.mentions}
			assert.Equal(t, tc.expected, status.HasMentions())
		})
	}
}

func (suite *StatusModelTestSuite) TestIsDeleted() {
	testCases := []struct {
		name     string
		deleted  bool
		expected bool
	}{
		{"deleted status", true, true},
		{"active status", false, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{Deleted: tc.deleted}
			assert.Equal(t, tc.expected, status.IsDeleted())
		})
	}
}

func (suite *StatusModelTestSuite) TestIsFlagged() {
	testCases := []struct {
		name     string
		flagged  bool
		expected bool
	}{
		{"flagged status", true, true},
		{"unflagged status", false, false},
	}

	for _, tc := range testCases {
		suite.T().Run(tc.name, func(t *testing.T) {
			status := &Status{Flagged: tc.flagged}
			assert.Equal(t, tc.expected, status.IsFlagged())
		})
	}
}

// Run the test suite
// TestStatusModelTestSuite removed - complex suite-based model tests

// =============================================================================
// Standalone Status Model Tests (additional coverage)
// =============================================================================

func TestStatus_IsRecipient(t *testing.T) {
	t.Run("returns true for To recipient", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{"https://example.com/users/alice", "https://example.com/users/bob"},
		}

		assert.True(t, status.IsRecipient("https://example.com/users/alice"))
		assert.True(t, status.IsRecipient("https://example.com/users/bob"))
		assert.False(t, status.IsRecipient("https://example.com/users/charlie"))
	})

	t.Run("returns true for CC recipient", func(t *testing.T) {
		status := &Status{
			CcRecipients: []string{"https://example.com/users/alice"},
		}

		assert.True(t, status.IsRecipient("https://example.com/users/alice"))
		assert.False(t, status.IsRecipient("https://example.com/users/bob"))
	})

	t.Run("returns true for BTo recipient", func(t *testing.T) {
		status := &Status{
			BtoRecipients: []string{"https://example.com/users/secret"},
		}

		assert.True(t, status.IsRecipient("https://example.com/users/secret"))
	})

	t.Run("returns true for BCC recipient", func(t *testing.T) {
		status := &Status{
			BccRecipients: []string{"https://example.com/users/hidden"},
		}

		assert.True(t, status.IsRecipient("https://example.com/users/hidden"))
	})

	t.Run("returns true for followers collection in To", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{"https://example.com/users/alice/followers"},
		}

		// Any actor should match if followers collection is in To
		assert.True(t, status.IsRecipient("https://example.com/users/bob"))
	})

	t.Run("returns true for followers collection in CC", func(t *testing.T) {
		status := &Status{
			CcRecipients: []string{"https://example.com/users/alice/followers"},
		}

		assert.True(t, status.IsRecipient("https://example.com/users/random"))
	})

	t.Run("returns false when not a recipient", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"https://example.com/users/alice"},
			CcRecipients:  []string{"https://example.com/users/bob"},
			BtoRecipients: []string{"https://example.com/users/charlie"},
			BccRecipients: []string{"https://example.com/users/david"},
		}

		assert.False(t, status.IsRecipient("https://example.com/users/eve"))
	})

	t.Run("returns false for empty lists", func(t *testing.T) {
		status := &Status{}

		assert.False(t, status.IsRecipient("https://example.com/users/anyone"))
	})
}

func TestStatus_IsVisibleTo(t *testing.T) {
	authorID := "https://example.com/users/author"
	otherUserID := "https://example.com/users/other"
	recipientID := "https://example.com/users/recipient"

	t.Run("public status visible to everyone", func(t *testing.T) {
		status := &Status{
			Visibility: VisibilityPublic,
			AuthorID:   authorID,
		}

		assert.True(t, status.IsVisibleTo(otherUserID))
		assert.True(t, status.IsVisibleTo(authorID))
		assert.True(t, status.IsVisibleTo("https://some.random/user"))
	})

	t.Run("unlisted status visible to everyone", func(t *testing.T) {
		status := &Status{
			Visibility: VisibilityUnlisted,
			AuthorID:   authorID,
		}

		assert.True(t, status.IsVisibleTo(otherUserID))
		assert.True(t, status.IsVisibleTo(authorID))
	})

	t.Run("private status visible to author", func(t *testing.T) {
		status := &Status{
			Visibility: VisibilityPrivate,
			AuthorID:   authorID,
		}

		assert.True(t, status.IsVisibleTo(authorID))
		assert.False(t, status.IsVisibleTo(otherUserID))
	})

	t.Run("private status visible to recipients", func(t *testing.T) {
		status := &Status{
			Visibility:   VisibilityPrivate,
			AuthorID:     authorID,
			ToRecipients: []string{recipientID},
		}

		assert.True(t, status.IsVisibleTo(authorID))
		assert.True(t, status.IsVisibleTo(recipientID))
		assert.False(t, status.IsVisibleTo(otherUserID))
	})

	t.Run("direct status visible to author", func(t *testing.T) {
		status := &Status{
			Visibility: VisibilityDirect,
			AuthorID:   authorID,
		}

		assert.True(t, status.IsVisibleTo(authorID))
		assert.False(t, status.IsVisibleTo(otherUserID))
	})

	t.Run("direct status visible to recipients", func(t *testing.T) {
		status := &Status{
			Visibility:   VisibilityDirect,
			AuthorID:     authorID,
			ToRecipients: []string{recipientID},
		}

		assert.True(t, status.IsVisibleTo(authorID))
		assert.True(t, status.IsVisibleTo(recipientID))
		assert.False(t, status.IsVisibleTo(otherUserID))
	})

	t.Run("unknown visibility returns false", func(t *testing.T) {
		status := &Status{
			Visibility: "weird_visibility",
			AuthorID:   authorID,
		}

		assert.False(t, status.IsVisibleTo(otherUserID))
		assert.False(t, status.IsVisibleTo(authorID))
	})
}

func TestStatus_GetVisibleRecipients(t *testing.T) {
	t.Run("includes To and CC for all viewers", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{"https://example.com/users/alice"},
			CcRecipients: []string{"https://example.com/users/bob"},
		}

		visible := status.GetVisibleRecipients("https://example.com/users/random")

		assert.Contains(t, visible, "https://example.com/users/alice")
		assert.Contains(t, visible, "https://example.com/users/bob")
	})

	t.Run("includes BTo for recipients only", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"https://example.com/users/alice"},
			BtoRecipients: []string{"https://example.com/users/secret"},
		}

		// alice is a recipient, should see BTo
		visibleAlice := status.GetVisibleRecipients("https://example.com/users/alice")
		assert.Contains(t, visibleAlice, "https://example.com/users/secret")

		// random user is not a recipient, should not see BTo
		visibleRandom := status.GetVisibleRecipients("https://example.com/users/random")
		assert.NotContains(t, visibleRandom, "https://example.com/users/secret")
	})

	t.Run("never includes BCC", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"https://example.com/users/alice"},
			BccRecipients: []string{"https://example.com/users/hidden"},
		}

		// Even alice (a recipient) should not see BCC
		visible := status.GetVisibleRecipients("https://example.com/users/alice")
		assert.NotContains(t, visible, "https://example.com/users/hidden")
	})
}

func TestStatus_GetAllRecipients(t *testing.T) {
	status := &Status{
		ToRecipients:  []string{"to1", "to2"},
		CcRecipients:  []string{"cc1"},
		BtoRecipients: []string{"bto1"},
		BccRecipients: []string{"bcc1", "bcc2"},
	}

	all := status.GetAllRecipients()

	assert.Len(t, all, 6)
	assert.Contains(t, all, "to1")
	assert.Contains(t, all, "to2")
	assert.Contains(t, all, "cc1")
	assert.Contains(t, all, "bto1")
	assert.Contains(t, all, "bcc1")
	assert.Contains(t, all, "bcc2")
}

func TestStatus_HasSpecificRecipients(t *testing.T) {
	publicAddr := "https://www.w3.org/ns/activitystreams#Public"

	t.Run("returns true for specific actor", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{"https://example.com/users/alice"},
		}
		assert.True(t, status.HasSpecificRecipients())
	})

	t.Run("returns false for public address only", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{publicAddr},
		}
		assert.False(t, status.HasSpecificRecipients())
	})

	t.Run("returns false for followers collection only", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{"https://example.com/users/alice/followers"},
		}
		assert.False(t, status.HasSpecificRecipients())
	})

	t.Run("returns true for mixed recipients", func(t *testing.T) {
		status := &Status{
			ToRecipients: []string{publicAddr},
			CcRecipients: []string{"https://example.com/users/alice"},
		}
		assert.True(t, status.HasSpecificRecipients())
	})
}

func TestStatus_IsDirect(t *testing.T) {
	assert.True(t, (&Status{Visibility: VisibilityDirect}).IsDirect())
	assert.False(t, (&Status{Visibility: VisibilityPublic}).IsDirect())
	assert.False(t, (&Status{Visibility: VisibilityPrivate}).IsDirect())
}

func TestStatus_IsPrivate(t *testing.T) {
	assert.True(t, (&Status{Visibility: VisibilityPrivate}).IsPrivate())
	assert.False(t, (&Status{Visibility: VisibilityPublic}).IsPrivate())
	assert.False(t, (&Status{Visibility: VisibilityDirect}).IsPrivate())
}

func TestStatus_IsReblog(t *testing.T) {
	t.Run("true with ReblogOfID", func(t *testing.T) {
		status := &Status{ReblogOfID: "original-123"}
		assert.True(t, status.IsReblog())
	})

	t.Run("true with BoostOfStatusID", func(t *testing.T) {
		status := &Status{BoostOfStatusID: "original-456"}
		assert.True(t, status.IsReblog())
	})

	t.Run("false when neither set", func(t *testing.T) {
		status := &Status{}
		assert.False(t, status.IsReblog())
	})
}

func TestStatus_IsConversation(t *testing.T) {
	t.Run("true when is a reply", func(t *testing.T) {
		status := &Status{InReplyToID: "parent-123"}
		assert.True(t, status.IsConversation())
	})

	t.Run("true when has replies", func(t *testing.T) {
		status := &Status{ReplyCount: 5}
		assert.True(t, status.IsConversation())
	})

	t.Run("false when standalone", func(t *testing.T) {
		status := &Status{}
		assert.False(t, status.IsConversation())
	})
}

func TestStatus_RequiresFollowCheck(t *testing.T) {
	assert.True(t, (&Status{Visibility: VisibilityPrivate}).RequiresFollowCheck())
	assert.False(t, (&Status{Visibility: VisibilityPublic}).RequiresFollowCheck())
	assert.False(t, (&Status{Visibility: VisibilityDirect}).RequiresFollowCheck())
	assert.False(t, (&Status{Visibility: VisibilityUnlisted}).RequiresFollowCheck())
}

func TestStatus_GetPrivacyScore(t *testing.T) {
	testCases := []struct {
		visibility string
		expected   int
	}{
		{VisibilityPublic, 1},
		{VisibilityUnlisted, 2},
		{VisibilityPrivate, 3},
		{VisibilityDirect, 4},
		{"unknown", 4}, // Default to most private
	}

	for _, tc := range testCases {
		t.Run(tc.visibility, func(t *testing.T) {
			status := &Status{Visibility: tc.visibility}
			assert.Equal(t, tc.expected, status.GetPrivacyScore())
		})
	}
}

func TestStatus_CanBeReblogged(t *testing.T) {
	t.Run("public status can be reblogged", func(t *testing.T) {
		status := &Status{Visibility: VisibilityPublic}
		assert.True(t, status.CanBeReblogged())
	})

	t.Run("unlisted status can be reblogged", func(t *testing.T) {
		status := &Status{Visibility: VisibilityUnlisted}
		assert.True(t, status.CanBeReblogged())
	})

	t.Run("private status can be reblogged", func(t *testing.T) {
		status := &Status{Visibility: VisibilityPrivate}
		assert.True(t, status.CanBeReblogged())
	})

	t.Run("direct status cannot be reblogged", func(t *testing.T) {
		status := &Status{Visibility: VisibilityDirect}
		assert.False(t, status.CanBeReblogged())
	})

	t.Run("deleted status cannot be reblogged", func(t *testing.T) {
		status := &Status{Visibility: VisibilityPublic, Deleted: true}
		assert.False(t, status.CanBeReblogged())
	})
}

func TestStatus_SanitizeForActor(t *testing.T) {
	t.Run("removes BCC for all viewers", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"to1"},
			BccRecipients: []string{"bcc1"},
		}

		sanitized := status.SanitizeForActor("https://example.com/users/viewer")

		assert.Contains(t, sanitized.ToRecipients, "to1")
		assert.Nil(t, sanitized.BccRecipients)
	})

	t.Run("removes BTo for non-recipients", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"https://example.com/users/alice"},
			BtoRecipients: []string{"bto1"},
		}

		// non-recipient
		sanitized := status.SanitizeForActor("https://example.com/users/random")
		assert.Nil(t, sanitized.BtoRecipients)
	})

	t.Run("preserves BTo for recipients", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"https://example.com/users/alice"},
			BtoRecipients: []string{"bto1"},
		}

		// alice is a recipient
		sanitized := status.SanitizeForActor("https://example.com/users/alice")
		assert.Contains(t, sanitized.BtoRecipients, "bto1")
	})

	t.Run("does not modify original status", func(t *testing.T) {
		status := &Status{
			ToRecipients:  []string{"to1"},
			BccRecipients: []string{"bcc1"},
		}

		_ = status.SanitizeForActor("https://example.com/users/viewer")

		assert.Contains(t, status.BccRecipients, "bcc1")
	})
}

func TestStatus_SyncBoostReferenceFields(t *testing.T) {
	t.Run("syncs ReblogOfID to BoostOfStatusID", func(t *testing.T) {
		status := &Status{ReblogOfID: "original-123"}
		status.syncBoostReferenceFields()
		assert.Equal(t, "original-123", status.BoostOfStatusID)
	})

	t.Run("syncs BoostOfStatusID to ReblogOfID", func(t *testing.T) {
		status := &Status{BoostOfStatusID: "original-456"}
		status.syncBoostReferenceFields()
		assert.Equal(t, "original-456", status.ReblogOfID)
	})

	t.Run("handles nil status", func(t *testing.T) {
		var status *Status
		// Should not panic
		status.syncBoostReferenceFields()
	})
}

func TestStatus_UpdateKeys(t *testing.T) {
	t.Run("sets PK and SK from StatusID", func(t *testing.T) {
		status := &Status{
			StatusID:    "test-123",
			PublishedAt: time.Now(),
		}

		err := status.UpdateKeys()

		assert.NoError(t, err)
		assert.Equal(t, "status#test-123", status.PK)
		assert.Equal(t, "status#test-123", status.SK)
	})

	t.Run("returns error for missing StatusID", func(t *testing.T) {
		status := &Status{}

		err := status.UpdateKeys()

		assert.Error(t, err)
	})

	t.Run("sets PublishedAt if zero", func(t *testing.T) {
		status := &Status{StatusID: "test-123"}

		err := status.UpdateKeys()

		assert.NoError(t, err)
		assert.False(t, status.PublishedAt.IsZero())
	})
}

func TestDetermineVisibilityFromAudience_Extended(t *testing.T) {
	publicAddress := "https://www.w3.org/ns/activitystreams#Public"

	testCases := []struct {
		name     string
		to       []string
		cc       []string
		expected string
	}{
		{
			name:     "public with followers in CC",
			to:       []string{publicAddress},
			cc:       []string{"https://example.com/users/alice/followers"},
			expected: VisibilityPublic,
		},
		{
			name:     "followers only (private)",
			to:       []string{"https://example.com/users/alice/followers"},
			cc:       []string{},
			expected: VisibilityPrivate,
		},
		{
			name:     "multiple specific users",
			to:       []string{"https://example.com/users/bob", "https://example.com/users/charlie", "https://example.com/users/dave"},
			cc:       []string{},
			expected: VisibilityDirect, // Still direct per logic (>1 specific users but no followers)
		},
		{
			name:     "nil slices",
			to:       nil,
			cc:       nil,
			expected: VisibilityDirect, // No recipients = direct
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := determineVisibilityFromAudience(tc.to, tc.cc)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// =============================================================================
// StatusHashtagIndex Tests
// =============================================================================

func TestStatusHashtagIndex_UpdateKeys(t *testing.T) {
	publishedAt := time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC)
	index := &StatusHashtagIndex{
		StatusID:    "status-123",
		Hashtag:     "golang",
		PublishedAt: publishedAt,
	}

	index.UpdateKeys()

	assert.Equal(t, "HASHTAG_INDEX#golang", index.PK)
	assert.Contains(t, index.SK, "2024-01-15T12:30:45")
	assert.Contains(t, index.SK, "status-123")

	// TTL should be set to ~1 year from now
	expectedTTL := time.Now().Add(365 * 24 * time.Hour).Unix()
	assert.InDelta(t, expectedTTL, index.TTL, 60)
}

func TestStatusHashtagIndex_TableName(t *testing.T) {
	index := StatusHashtagIndex{}
	assert.Equal(t, MainTableName, index.TableName())
}

func TestStatus_CreateHashtagIndexRecords(t *testing.T) {
	t.Run("returns nil for single hashtag", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			AuthorID:    "author-456",
			Hashtags:    []string{"golang"},
			PublishedAt: time.Now(),
			Visibility:  VisibilityPublic,
		}

		records := status.CreateHashtagIndexRecords()

		assert.Nil(t, records)
	})

	t.Run("returns nil for empty hashtags", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			Hashtags:    []string{},
			PublishedAt: time.Now(),
		}

		records := status.CreateHashtagIndexRecords()

		assert.Nil(t, records)
	})

	t.Run("creates records for hashtags 2+", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			AuthorID:    "author-456",
			Hashtags:    []string{"first", "second", "third"},
			PublishedAt: time.Now(),
			Visibility:  VisibilityPublic,
		}

		records := status.CreateHashtagIndexRecords()

		assert.Len(t, records, 2) // Only 2nd and 3rd hashtag
		assert.Equal(t, "second", records[0].Hashtag)
		assert.Equal(t, "third", records[1].Hashtag)
		assert.Equal(t, "status-123", records[0].StatusID)
		assert.Equal(t, "author-456", records[0].AuthorID)
		assert.Equal(t, VisibilityPublic, records[0].Visibility)
	})
}

func TestStatus_GetAllHashtagIndexRecords(t *testing.T) {
	t.Run("returns nil for empty hashtags", func(t *testing.T) {
		status := &Status{
			StatusID: "status-123",
			Hashtags: []string{},
		}

		records := status.GetAllHashtagIndexRecords()

		assert.Nil(t, records)
	})

	t.Run("creates records for all hashtags", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			AuthorID:    "author-456",
			Hashtags:    []string{"first", "second", "third"},
			PublishedAt: time.Now(),
			Visibility:  VisibilityPublic,
		}

		records := status.GetAllHashtagIndexRecords()

		assert.Len(t, records, 3)
		assert.Equal(t, "first", records[0].Hashtag)
		assert.Equal(t, "second", records[1].Hashtag)
		assert.Equal(t, "third", records[2].Hashtag)
	})

	t.Run("skips empty hashtags", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			Hashtags:    []string{"valid", "", "another"},
			PublishedAt: time.Now(),
		}

		records := status.GetAllHashtagIndexRecords()

		assert.Len(t, records, 2)
	})
}

func TestStatus_DeleteHashtagIndexRecords(t *testing.T) {
	t.Run("returns nil for empty hashtags", func(t *testing.T) {
		status := &Status{
			StatusID: "status-123",
			Hashtags: []string{},
		}

		ops := status.DeleteHashtagIndexRecords()

		assert.Nil(t, ops)
	})

	t.Run("creates delete ops for all hashtags", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			Hashtags:    []string{"first", "second"},
			PublishedAt: time.Date(2024, 1, 15, 12, 30, 45, 0, time.UTC),
		}

		ops := status.DeleteHashtagIndexRecords()

		assert.Len(t, ops, 2)
		assert.Equal(t, "HASHTAG_INDEX#first", ops[0]["PK"])
		assert.Contains(t, ops[0]["SK"], "status-123")
		assert.Equal(t, "HASHTAG_INDEX#second", ops[1]["PK"])
	})

	t.Run("skips empty hashtags", func(t *testing.T) {
		status := &Status{
			StatusID:    "status-123",
			Hashtags:    []string{"valid", ""},
			PublishedAt: time.Now(),
		}

		ops := status.DeleteHashtagIndexRecords()

		assert.Len(t, ops, 1)
	})
}

func TestExtractStatusIDFromURL(t *testing.T) {
	testCases := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "standard ActivityPub URL",
			url:      "https://example.com/users/alice/statuses/123",
			expected: "123",
		},
		{
			name:     "just ID",
			url:      "simple-id",
			expected: "simple-id",
		},
		{
			name:     "empty string",
			url:      "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := extractStatusIDFromURL(tc.url)
			assert.Equal(t, tc.expected, result)
		})
	}
}
