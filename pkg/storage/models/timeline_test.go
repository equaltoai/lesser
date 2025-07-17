package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeline_BeforeCreate(t *testing.T) {
	tests := []struct {
		name     string
		timeline *Timeline
		wantErr  bool
		validate func(t *testing.T, timeline *Timeline)
	}{
		{
			name: "successful creation with all fields",
			timeline: &Timeline{
				TimelineType: "HOME",
				TimelineID:   "testuser",
				PostID:       "post123",
				ActorID:      "actor456",
				ActorHandle:  "@testuser@example.com",
				Content:      "Test content",
				ContentType:  "Note",
				HasMedia:     true,
				IsReply:      false,
				Visibility:   "public",
				Language:     "en",
				Sensitive:    false,
				CreatedAt:    time.Now().Add(-1 * time.Hour),
				TimelineAt:   time.Now(),
			},
			wantErr: false,
			validate: func(t *testing.T, timeline *Timeline) {
				assert.Equal(t, "timeline#HOME#testuser", timeline.PK)
				assert.Contains(t, timeline.SK, "#")
				assert.Equal(t, "POST#post123", timeline.GSI1PK)
				assert.Equal(t, "ACTOR#actor456", timeline.GSI2PK)
				assert.Equal(t, "VISIBILITY#public", timeline.GSI3PK)
				assert.Equal(t, "LANGUAGE#en", timeline.GSI4PK)
				assert.NotEmpty(t, timeline.EntryID)
				assert.False(t, timeline.ModifiedAt.IsZero())
			},
		},
		{
			name: "creation with minimal fields",
			timeline: &Timeline{
				TimelineType: "PUBLIC",
				TimelineID:   "FEDERATED",
				PostID:       "post789",
				ActorID:      "actor123",
				Visibility:   "unlisted",
			},
			wantErr: false,
			validate: func(t *testing.T, timeline *Timeline) {
				assert.Equal(t, "timeline#PUBLIC#FEDERATED", timeline.PK)
				assert.Equal(t, "POST#post789", timeline.GSI1PK)
				assert.Equal(t, "ACTOR#actor123", timeline.GSI2PK)
				assert.Equal(t, "VISIBILITY#unlisted", timeline.GSI3PK)
				assert.Empty(t, timeline.GSI4PK) // No language specified
				assert.False(t, timeline.CreatedAt.IsZero())
				assert.False(t, timeline.TimelineAt.IsZero())
			},
		},
		{
			name: "creation with private visibility",
			timeline: &Timeline{
				TimelineType: "HOME",
				TimelineID:   "privateuser",
				PostID:       "privatepost",
				ActorID:      "privateactor",
				Visibility:   "private",
			},
			wantErr: false,
			validate: func(t *testing.T, timeline *Timeline) {
				assert.Equal(t, "timeline#HOME#privateuser", timeline.PK)
				assert.Equal(t, "POST#privatepost", timeline.GSI1PK)
				assert.Equal(t, "ACTOR#privateactor", timeline.GSI2PK)
				assert.Empty(t, timeline.GSI3PK) // Private content not in visibility index
				assert.Empty(t, timeline.GSI3SK)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.timeline.BeforeCreate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, tt.timeline)
				}
			}
		})
	}
}

func TestTimeline_BeforeUpdate(t *testing.T) {
	timeline := &Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		ActorID:      "actor456",
		Visibility:   "public",
		Language:     "en",
		TimelineAt:   time.Now(),
	}

	// Set up initial state
	err := timeline.BeforeCreate()
	require.NoError(t, err)

	originalModifiedAt := timeline.ModifiedAt
	time.Sleep(10 * time.Millisecond) // Ensure time difference

	// Update the timeline
	timeline.Visibility = "unlisted"
	timeline.Language = "es"

	err = timeline.BeforeUpdate()
	require.NoError(t, err)

	// Verify update behavior
	assert.True(t, timeline.ModifiedAt.After(originalModifiedAt))
	assert.Equal(t, "VISIBILITY#unlisted", timeline.GSI3PK)
	assert.Equal(t, "LANGUAGE#es", timeline.GSI4PK)
}

func TestTimeline_SetupGSIKeys(t *testing.T) {
	timeline := &Timeline{
		EntryID:    "entry123",
		PostID:     "post456",
		ActorID:    "actor789",
		Visibility: "public",
		Language:   "fr",
		TimelineAt: time.Unix(1640995200, 0), // Fixed timestamp for testing
	}

	timeline.setupGSIKeys()

	expectedTimestamp := "1640995200"
	assert.Equal(t, "POST#post456", timeline.GSI1PK)
	assert.Equal(t, expectedTimestamp+"#entry123", timeline.GSI1SK)
	assert.Equal(t, "ACTOR#actor789", timeline.GSI2PK)
	assert.Equal(t, expectedTimestamp+"#entry123", timeline.GSI2SK)
	assert.Equal(t, "VISIBILITY#public", timeline.GSI3PK)
	assert.Equal(t, expectedTimestamp+"#entry123", timeline.GSI3SK)
	assert.Equal(t, "LANGUAGE#fr", timeline.GSI4PK)
	assert.Equal(t, expectedTimestamp+"#entry123", timeline.GSI4SK)
}

func TestTimeline_SetupGSIKeys_EmptyFields(t *testing.T) {
	timeline := &Timeline{
		EntryID:    "entry123",
		TimelineAt: time.Unix(1640995200, 0),
		// No PostID, ActorID, Visibility, or Language
	}

	timeline.setupGSIKeys()

	// All GSI keys should be empty when required fields are missing
	assert.Empty(t, timeline.GSI1PK)
	assert.Empty(t, timeline.GSI1SK)
	assert.Empty(t, timeline.GSI2PK)
	assert.Empty(t, timeline.GSI2SK)
	assert.Empty(t, timeline.GSI3PK)
	assert.Empty(t, timeline.GSI3SK)
	assert.Empty(t, timeline.GSI4PK)
	assert.Empty(t, timeline.GSI4SK)
}

func TestTimeline_SetupGSIKeys_PrivateVisibility(t *testing.T) {
	timeline := &Timeline{
		EntryID:    "entry123",
		PostID:     "post456",
		ActorID:    "actor789",
		Visibility: "private", // Private visibility should not be indexed
		Language:   "en",
		TimelineAt: time.Unix(1640995200, 0),
	}

	timeline.setupGSIKeys()

	// Post and Actor GSIs should be set
	assert.Equal(t, "POST#post456", timeline.GSI1PK)
	assert.Equal(t, "ACTOR#actor789", timeline.GSI2PK)
	
	// Visibility GSI should be empty for private content
	assert.Empty(t, timeline.GSI3PK)
	assert.Empty(t, timeline.GSI3SK)
	
	// Language GSI should still be set
	assert.Equal(t, "LANGUAGE#en", timeline.GSI4PK)
}

func TestTimeline_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "not expired",
			expiresAt: time.Now().Add(1 * time.Hour),
			want:      false,
		},
		{
			name:      "expired",
			expiresAt: time.Now().Add(-1 * time.Hour),
			want:      true,
		},
		{
			name:      "no expiration",
			expiresAt: time.Time{},
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeline := &Timeline{ExpiresAt: tt.expiresAt}
			assert.Equal(t, tt.want, timeline.IsExpired())
		})
	}
}

func TestTimeline_VisibilityChecks(t *testing.T) {
	tests := []struct {
		name       string
		visibility string
		isPublic   bool
		isUnlisted bool
		isPrivate  bool
		isDirect   bool
	}{
		{
			name:       "public visibility",
			visibility: "public",
			isPublic:   true,
			isUnlisted: false,
			isPrivate:  false,
			isDirect:   false,
		},
		{
			name:       "unlisted visibility",
			visibility: "unlisted",
			isPublic:   false,
			isUnlisted: true,
			isPrivate:  false,
			isDirect:   false,
		},
		{
			name:       "private visibility",
			visibility: "private",
			isPublic:   false,
			isUnlisted: false,
			isPrivate:  true,
			isDirect:   false,
		},
		{
			name:       "direct visibility",
			visibility: "direct",
			isPublic:   false,
			isUnlisted: false,
			isPrivate:  false,
			isDirect:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeline := &Timeline{Visibility: tt.visibility}
			assert.Equal(t, tt.isPublic, timeline.IsPublic())
			assert.Equal(t, tt.isUnlisted, timeline.IsUnlisted())
			assert.Equal(t, tt.isPrivate, timeline.IsPrivate())
			assert.Equal(t, tt.isDirect, timeline.IsDirect())
		})
	}
}

func TestTimeline_TimelineTypeChecks(t *testing.T) {
	tests := []struct {
		name         string
		timelineType string
		isHome       bool
		isPublic     bool
		isList       bool
		isDirect     bool
		isHashtag    bool
	}{
		{
			name:         "home timeline",
			timelineType: "HOME",
			isHome:       true,
			isPublic:     false,
			isList:       false,
			isDirect:     false,
			isHashtag:    false,
		},
		{
			name:         "public timeline",
			timelineType: "PUBLIC",
			isHome:       false,
			isPublic:     true,
			isList:       false,
			isDirect:     false,
			isHashtag:    false,
		},
		{
			name:         "list timeline",
			timelineType: "LIST",
			isHome:       false,
			isPublic:     false,
			isList:       true,
			isDirect:     false,
			isHashtag:    false,
		},
		{
			name:         "direct timeline",
			timelineType: "DIRECT",
			isHome:       false,
			isPublic:     false,
			isList:       false,
			isDirect:     true,
			isHashtag:    false,
		},
		{
			name:         "hashtag timeline",
			timelineType: "HASHTAG",
			isHome:       false,
			isPublic:     false,
			isList:       false,
			isDirect:     false,
			isHashtag:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeline := &Timeline{TimelineType: tt.timelineType}
			assert.Equal(t, tt.isHome, timeline.IsHomeTimeline())
			assert.Equal(t, tt.isPublic, timeline.IsPublicTimeline())
			assert.Equal(t, tt.isList, timeline.IsListTimeline())
			assert.Equal(t, tt.isDirect, timeline.IsDirectTimeline())
			assert.Equal(t, tt.isHashtag, timeline.IsHashtagTimeline())
		})
	}
}

func TestTimeline_GetTimelineKey(t *testing.T) {
	timeline := &Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
	}

	expected := "HOME#testuser"
	assert.Equal(t, expected, timeline.GetTimelineKey())
}

func TestTimeline_GetSortKey(t *testing.T) {
	timeline := &Timeline{
		EntryID:    "entry123",
		TimelineAt: time.Unix(1640995200, 0),
	}

	expected := "1640995200#entry123"
	assert.Equal(t, expected, timeline.GetSortKey())
}

func TestTimeline_TableName(t *testing.T) {
	timeline := &Timeline{}
	assert.Equal(t, "lesser-main", timeline.TableName())
}

func TestTimeline_EntryIDGeneration(t *testing.T) {
	timeline := &Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		TimelineAt:   time.Unix(1640995200, 0),
		// EntryID is empty, should be generated
	}

	err := timeline.BeforeCreate()
	require.NoError(t, err)

	expected := "1640995200_post123"
	assert.Equal(t, expected, timeline.EntryID)
}

func TestTimeline_PreserveExistingEntryID(t *testing.T) {
	existingEntryID := "custom_entry_id"
	timeline := &Timeline{
		TimelineType: "HOME",
		TimelineID:   "testuser",
		PostID:       "post123",
		EntryID:      existingEntryID,
		TimelineAt:   time.Unix(1640995200, 0),
	}

	err := timeline.BeforeCreate()
	require.NoError(t, err)

	// Should preserve existing EntryID
	assert.Equal(t, existingEntryID, timeline.EntryID)
}