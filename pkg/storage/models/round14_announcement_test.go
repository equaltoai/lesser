package models

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnnouncement_KeysStatusAndSubmodels(t *testing.T) {
	t.Run("getStatusString respects start/end windows", func(t *testing.T) {
		now := time.Now()
		future := now.Add(time.Hour)
		past := now.Add(-time.Hour)

		a := &Announcement{StartsAt: &future}
		assert.Equal(t, "inactive", a.getStatusString(now))

		a = &Announcement{EndsAt: &past}
		assert.Equal(t, "inactive", a.getStatusString(now))

		a = &Announcement{}
		assert.Equal(t, StatusActive, a.getStatusString(now))
	})

	t.Run("UpdateKeys configures GSIs and admin index", func(t *testing.T) {
		published := time.Unix(1700000000, 0).UTC()
		a := &Announcement{
			ID:          "a1",
			PublishedAt: published,
			CreatedBy:   "admin",
		}
		require.NoError(t, a.UpdateKeys())
		assert.Equal(t, "ANNOUNCEMENT#a1", a.PK)
		assert.Equal(t, "ANNOUNCEMENT", a.SK)
		assert.Equal(t, "ANNOUNCEMENT#active", a.GSI1PK)
		assert.Equal(t, fmt.Sprintf("%010d", 9999999999-published.Unix()), a.GSI1SK)
		assert.Equal(t, "ADMIN#admin", a.GSI2PK)
		assert.Contains(t, a.GSI2SK, "a1")
		assert.Equal(t, a.PK, a.GetPK())
		assert.Equal(t, a.SK, a.GetSK())
		assert.Equal(t, MainTableName, a.TableName())
	})

	t.Run("setupGSIKeys clears admin keys when CreatedBy empty", func(t *testing.T) {
		a := &Announcement{ID: "a1", PublishedAt: time.Unix(1700000000, 0).UTC()}
		a.setupGSIKeys()
		assert.Empty(t, a.GSI2PK)
		assert.Empty(t, a.GSI2SK)
	})

	t.Run("IsActive reflects current time", func(t *testing.T) {
		a := &Announcement{}
		assert.True(t, a.IsActive())

		future := time.Now().Add(time.Hour)
		a.StartsAt = &future
		assert.False(t, a.IsActive())
	})

	t.Run("BeforeCreate generates ID and timestamps", func(t *testing.T) {
		a := &Announcement{CreatedBy: "admin"}
		require.NoError(t, a.BeforeCreate())
		assert.NotEmpty(t, a.ID)
		assert.False(t, a.PublishedAt.IsZero())
		assert.False(t, a.UpdatedAt.IsZero())
		assert.False(t, a.CreatedAt.IsZero())
		assert.Equal(t, "ANNOUNCEMENT#"+a.ID, a.PK)
	})

	t.Run("Dismissal and reaction models create keys and timestamps", func(t *testing.T) {
		d := &AnnouncementDismissal{Username: "alice", AnnouncementID: "a1"}
		require.NoError(t, d.BeforeCreate())
		assert.False(t, d.DismissedAt.IsZero())
		assert.Equal(t, "USER#alice", d.PK)
		assert.Equal(t, "ANNOUNCEMENT_DISMISSED#a1", d.SK)
		assert.Equal(t, d.PK, d.GetPK())
		assert.Equal(t, d.SK, d.GetSK())
		assert.Equal(t, MainTableName, d.TableName())

		r := &AnnouncementReaction{Username: "alice", AnnouncementID: "a1", EmojiName: "smile"}
		require.NoError(t, r.BeforeCreate())
		assert.False(t, r.ReactedAt.IsZero())
		assert.Equal(t, "ANNOUNCEMENT_REACTION#a1", r.PK)
		assert.Equal(t, "USER#alice#smile", r.SK)
		assert.Equal(t, r.PK, r.GetPK())
		assert.Equal(t, r.SK, r.GetSK())
		assert.Equal(t, MainTableName, r.TableName())
	})

	t.Run("Sub-struct TableName methods", func(t *testing.T) {
		assert.Equal(t, MainTableName, (Reaction{}).TableName())
		assert.Equal(t, MainTableName, (CustomEmoji{}).TableName())
		assert.Equal(t, MainTableName, (Mention{}).TableName())
	})
}
