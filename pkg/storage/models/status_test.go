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
		Note:     note,
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
		Note:     note,
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
		Note:     note,
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
		Note:     note,
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
				Note:     note,
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
		Note:     note,
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
		Note:     note,
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
		Note:     note,
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
		Note:     note,
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
		Note:     note,
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
func TestStatusModelTestSuite(t *testing.T) {
	suite.Run(t, new(StatusModelTestSuite))
}
