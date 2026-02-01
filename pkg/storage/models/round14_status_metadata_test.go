package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStatusMetadata_QuotesFlagsAndReplyCounts(t *testing.T) {
	t.Run("NewStatusMetadata sets defaults and keys", func(t *testing.T) {
		sm := NewStatusMetadata("s1")
		require.NotNil(t, sm)
		assert.Equal(t, "STATUS_META#s1", sm.PK)
		assert.Equal(t, SKMetadata, sm.SK)
		assert.Equal(t, "public", sm.QuoteType)
		assert.True(t, sm.AllowQuotes)
		assert.True(t, sm.AllowReplies)
		assert.False(t, sm.DisableLikes)
		assert.False(t, sm.DisableReblogs)
		assert.Equal(t, MainTableName, sm.TableName())
	})

	t.Run("Withdraw/Restore and SetQuoteType toggle quotable behavior", func(t *testing.T) {
		sm := NewStatusMetadata("s1")
		assert.True(t, sm.IsQuotable())
		assert.True(t, sm.IsPubliclyQuotable())

		sm.WithdrawFromQuotes()
		assert.True(t, sm.WithdrawnFromQuotes)
		assert.False(t, sm.AllowQuotes)
		assert.Equal(t, StatusDisabled, sm.QuoteType)
		assert.False(t, sm.IsQuotable())

		sm.RestoreToQuotes()
		assert.False(t, sm.WithdrawnFromQuotes)
		assert.True(t, sm.AllowQuotes)
		assert.Equal(t, VisibilityPublic, sm.QuoteType)

		sm.SetQuoteType("followers")
		assert.True(t, sm.AllowQuotes)
		assert.False(t, sm.WithdrawnFromQuotes)
		assert.Equal(t, "followers", sm.QuoteType)

		sm.SetQuoteType("unknown")
		assert.Equal(t, VisibilityPublic, sm.QuoteType)
		assert.True(t, sm.AllowQuotes)
		assert.False(t, sm.WithdrawnFromQuotes)
	})

	t.Run("Moderation flag helpers de-dupe and remove", func(t *testing.T) {
		sm := NewStatusMetadata("s1")
		sm.AddModerationFlag("spam")
		sm.AddModerationFlag("spam")
		assert.Len(t, sm.ModerationFlags, 1)
		assert.True(t, sm.HasModerationFlag("spam"))

		sm.RemoveModerationFlag("spam")
		assert.False(t, sm.HasModerationFlag("spam"))
		assert.Empty(t, sm.ModerationFlags)
	})

	t.Run("Reply count helpers clamp to >= 0", func(t *testing.T) {
		sm := &StatusMetadata{}
		sm.DecrementReplyCount()
		assert.Equal(t, 0, sm.ReplyCount)

		sm.IncrementReplyCount()
		assert.Equal(t, 1, sm.ReplyCount)

		sm.SetReplyCount(-1)
		assert.Equal(t, 0, sm.ReplyCount)
		sm.SetReplyCount(3)
		assert.Equal(t, 3, sm.ReplyCount)
	})

	t.Run("BeforeCreate and BeforeUpdate set timestamps and keys", func(t *testing.T) {
		sm := &StatusMetadata{StatusID: "s2"}
		before := time.Now()
		require.NoError(t, sm.BeforeCreate())
		assert.WithinDuration(t, before, sm.CreatedAt, time.Second)
		assert.True(t, sm.CreatedAt.Equal(sm.UpdatedAt))
		assert.Equal(t, "STATUS_META#s2", sm.PK)
		assert.Equal(t, SKMetadata, sm.SK)

		prev := sm.UpdatedAt
		require.NoError(t, sm.BeforeUpdate())
		assert.True(t, sm.UpdatedAt.After(prev))
	})
}
