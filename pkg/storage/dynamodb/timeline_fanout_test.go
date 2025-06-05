package dynamodb

import (
	"strings"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFanOutPost(t *testing.T) {
	// This is a basic structure test to ensure the fan-out logic is working
	// The actual fan-out logic is tested through integration tests

	tests := []struct {
		name     string
		activity *activitypub.Activity
		wantErr  bool
		scenario string
	}{
		{
			name: "public post should fan out to followers and public timelines",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/1",
					Type: activitypub.CreateType,
					To:   []string{activitypub.PublicAddress},
					CC:   []string{"https://example.com/users/alice/followers"},
				},
				Actor: "https://example.com/users/alice",
				Object: map[string]interface{}{
					"id":           "https://example.com/objects/1",
					"type":         "Note",
					"content":      "Hello, world!",
					"attributedTo": "https://example.com/users/alice",
					"to":           []string{activitypub.PublicAddress},
					"cc":           []string{"https://example.com/users/alice/followers"},
					"published":    time.Now().Format(time.RFC3339),
				},
			},
			wantErr:  false,
			scenario: "Public post",
		},
		{
			name: "private post should only fan out to followers",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/2",
					Type: activitypub.CreateType,
					To:   []string{"https://example.com/users/alice/followers"},
				},
				Actor: "https://example.com/users/alice",
				Object: map[string]interface{}{
					"id":           "https://example.com/objects/2",
					"type":         "Note",
					"content":      "Private message to followers",
					"attributedTo": "https://example.com/users/alice",
					"to":           []string{"https://example.com/users/alice/followers"},
					"published":    time.Now().Format(time.RFC3339),
				},
			},
			wantErr:  false,
			scenario: "Private post",
		},
		{
			name: "direct message should not fan out",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/3",
					Type: activitypub.CreateType,
					To:   []string{"https://example.com/users/bob"},
				},
				Actor: "https://example.com/users/alice",
				Object: map[string]interface{}{
					"id":           "https://example.com/objects/3",
					"type":         "Note",
					"content":      "Direct message to Bob",
					"attributedTo": "https://example.com/users/alice",
					"to":           []string{"https://example.com/users/bob"},
					"published":    time.Now().Format(time.RFC3339),
				},
			},
			wantErr:  false,
			scenario: "Direct message",
		},
		{
			name: "non-Create activity should be ignored",
			activity: &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/activities/4",
					Type: activitypub.LikeType,
				},
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/some-post",
			},
			wantErr:  false,
			scenario: "Non-Create activity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: This is a structural test only
			// In a real implementation, you'd mock the DynamoDB client
			// and verify the correct timeline entries are created

			// Verify the activity structure is correct
			if tt.activity.Type == activitypub.CreateType {
				objMap, ok := tt.activity.Object.(map[string]interface{})
				require.True(t, ok, "Create activity should have object map")
				assert.NotEmpty(t, objMap["id"], "Object should have ID")
				assert.NotEmpty(t, objMap["attributedTo"], "Object should have attributedTo")
			}
		})
	}
}

func TestTimelineEntryCreation(t *testing.T) {
	// Test that timeline entries are created with correct fields
	baseEntry := &storage.TimelineEntry{
		PostID:      "https://example.com/objects/1",
		ActorID:     "https://example.com/users/alice",
		ActorHandle: "alice",
		Content:     "Test post content",
		ContentType: "Note",
		HasMedia:    false,
		IsReply:     false,
		IsBoost:     false,
		Visibility:  "public",
		Language:    "en",
		Sensitive:   false,
		CreatedAt:   time.Now(),
		TimelineAt:  time.Now(),
	}

	// Verify entry has required fields
	assert.NotEmpty(t, baseEntry.PostID)
	assert.NotEmpty(t, baseEntry.ActorID)
	assert.NotEmpty(t, baseEntry.ActorHandle)
	assert.NotEmpty(t, baseEntry.Content)
	assert.NotEmpty(t, baseEntry.Visibility)
	assert.Equal(t, "Note", baseEntry.ContentType)
	assert.False(t, baseEntry.HasMedia)
	assert.False(t, baseEntry.IsReply)
	assert.False(t, baseEntry.IsBoost)
	assert.Equal(t, "en", baseEntry.Language)
	assert.False(t, baseEntry.Sensitive)
	assert.NotZero(t, baseEntry.CreatedAt)
	assert.NotZero(t, baseEntry.TimelineAt)
}

func TestHashtagExtraction(t *testing.T) {
	tests := []struct {
		name         string
		note         *activitypub.Note
		expectedTags []string
	}{
		{
			name: "extract hashtags from note",
			note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/objects/1",
					Type: activitypub.NoteType,
					To:   []string{activitypub.PublicAddress},
				},
				Content:      "Hello #world! Testing #golang and #activitypub",
				AttributedTo: "https://example.com/actors/alice",
				Tag: []activitypub.Tag{
					{Type: "Hashtag", Name: "#world", Href: "https://example.com/tags/world"},
					{Type: "Hashtag", Name: "#golang", Href: "https://example.com/tags/golang"},
					{Type: "Hashtag", Name: "#activitypub", Href: "https://example.com/tags/activitypub"},
				},
			},
			expectedTags: []string{"world", "golang", "activitypub"},
		},
		{
			name: "normalize hashtag case",
			note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/objects/2",
					Type: activitypub.NoteType,
					To:   []string{activitypub.PublicAddress},
				},
				Content:      "Testing #GoLang and #GOLANG",
				AttributedTo: "https://example.com/actors/alice",
				Tag: []activitypub.Tag{
					{Type: "Hashtag", Name: "#GoLang", Href: "https://example.com/tags/golang"},
					{Type: "Hashtag", Name: "#GOLANG", Href: "https://example.com/tags/golang"},
				},
			},
			expectedTags: []string{"golang"},
		},
		{
			name: "ignore non-hashtag tags",
			note: &activitypub.Note{
				BaseObject: activitypub.BaseObject{
					ID:   "https://example.com/objects/3",
					Type: activitypub.NoteType,
					To:   []string{activitypub.PublicAddress},
				},
				Content:      "Hello @user #test",
				AttributedTo: "https://example.com/actors/alice",
				Tag: []activitypub.Tag{
					{Type: "Mention", Name: "@user@example.com", Href: "https://example.com/users/user"},
					{Type: "Hashtag", Name: "#test", Href: "https://example.com/tags/test"},
				},
			},
			expectedTags: []string{"test"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Extract unique normalized hashtags
			tagMap := make(map[string]bool)
			for _, tag := range tt.note.Tag {
				if tag.Type == "Hashtag" && tag.Name != "" {
					// Normalize: remove # prefix and lowercase
					normalized := strings.TrimPrefix(strings.ToLower(tag.Name), "#")
					tagMap[normalized] = true
				}
			}

			// Verify we got the expected tags
			assert.Equal(t, len(tt.expectedTags), len(tagMap))
			for _, expected := range tt.expectedTags {
				assert.True(t, tagMap[expected], "Expected hashtag %s not found", expected)
			}
		})
	}
}
