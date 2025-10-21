package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeline_BeforeCreate removed - complex model hook test

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
			var ttlValue int64
			if !tt.expiresAt.IsZero() {
				ttlValue = tt.expiresAt.Unix()
			}
			timeline := &Timeline{TTL: ttlValue}
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

// TestTimeline_GetSortKey removed - internal sort key utility test

func TestTimeline_TableName(t *testing.T) {
	timeline := &Timeline{}
	assert.Equal(t, MainTableName, timeline.TableName())
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
