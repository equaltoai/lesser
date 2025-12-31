package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMediaAttachment_KeysValidationAndFocalPoint(t *testing.T) {
	t.Run("UpdateKeys supports user, scheduled_status, and generic entity types", func(t *testing.T) {
		m := &MediaAttachment{MediaID: "m1"}
		m.UpdateKeys(EntityTypeUser, "alice")
		assert.Equal(t, "USER#alice", m.PK)
		assert.Equal(t, "MEDIA#m1", m.SK)

		m = &MediaAttachment{MediaID: "m2"}
		m.UpdateKeys(EntityTypeScheduledStatus, "s1")
		assert.Equal(t, "SCHEDULED_STATUS#s1", m.PK)
		assert.Equal(t, "MEDIA#m2", m.SK)

		m = &MediaAttachment{MediaID: "m3"}
		m.UpdateKeys("custom", "id")
		assert.Equal(t, "CUSTOM#id", m.PK)
		assert.Equal(t, "MEDIA#m3", m.SK)
	})

	t.Run("BeforeCreate and BeforeUpdate set timestamps and keep keys consistent", func(t *testing.T) {
		m := &MediaAttachment{
			EntityType: EntityTypeUser,
			EntityID:   "alice",
			MediaID:    "m1",
		}
		before := time.Now()
		require.NoError(t, m.BeforeCreate())
		assert.WithinDuration(t, before, m.AttachedAt, time.Second)
		assert.True(t, m.AttachedAt.Equal(m.UpdatedAt))
		assert.Equal(t, "USER#alice", m.PK)

		prevUpdated := m.UpdatedAt
		require.NoError(t, m.BeforeUpdate())
		assert.True(t, m.UpdatedAt.After(prevUpdated) || prevUpdated.IsZero())
		assert.Equal(t, "USER#alice", m.PK)
	})

	t.Run("Validate enforces required fields and constraints", func(t *testing.T) {
		m := &MediaAttachment{}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentIDRequired)

		m = &MediaAttachment{MediaID: "m1"}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentEntityTypeRequired)

		m = &MediaAttachment{MediaID: "m1", EntityType: EntityTypeUser}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentEntityIDRequired)

		m = &MediaAttachment{MediaID: "m1", EntityType: EntityTypeUser, EntityID: "alice", Order: -1}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentOrderNegative)

		m = &MediaAttachment{MediaID: "m1", EntityType: "nope", EntityID: "x"}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentInvalidEntityType)

		m = &MediaAttachment{MediaID: "m1", EntityType: EntityTypeUser, EntityID: "alice", FocalPoint: "1"}
		assert.ErrorIs(t, m.Validate(), ErrMediaAttachmentInvalidFocalPoint)
	})

	t.Run("Entity helpers", func(t *testing.T) {
		m := &MediaAttachment{EntityType: "USER"}
		assert.True(t, m.IsForUser())
		assert.False(t, m.IsForScheduledStatus())

		m = &MediaAttachment{EntityType: "scheduled_status"}
		assert.True(t, m.IsForScheduledStatus())
	})

	t.Run("Focal point helpers parse and reject invalid formats", func(t *testing.T) {
		m := &MediaAttachment{}
		m.SetFocalPoint(0.1, 0.2)
		x, y, ok := m.GetFocalPoint()
		assert.True(t, ok)
		assert.InDelta(t, 0.1, x, 0.000001)
		assert.InDelta(t, 0.2, y, 0.000001)

		m.FocalPoint = "a,b"
		_, _, ok = m.GetFocalPoint()
		assert.False(t, ok)

		m.FocalPoint = "1"
		_, _, ok = m.GetFocalPoint()
		assert.False(t, ok)
	})
}

