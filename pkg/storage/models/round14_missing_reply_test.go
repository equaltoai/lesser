package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingReply_LifecycleAndRetry(t *testing.T) {
	t.Run("NewMissingReply sets defaults", func(t *testing.T) {
		before := time.Now()
		m := NewMissingReply("root", "parent", "reply")
		require.NotNil(t, m)
		assert.Equal(t, "root", m.RootStatusID)
		assert.Equal(t, "parent", m.ParentStatusID)
		assert.Equal(t, "reply", m.ReplyID)
		assert.Equal(t, MissingReplyStatusPending, m.Status)
		assert.Equal(t, 0, m.AttemptCount)
		assert.Equal(t, 3, m.Priority)
		assert.True(t, time.Unix(m.TTL, 0).After(before.Add(29*24*time.Hour)))
		assert.Equal(t, MainTableName, (MissingReply{}).TableName())
	})

	t.Run("UpdateKeys sets PK/SK and parent-status GSI", func(t *testing.T) {
		m := &MissingReply{
			RootStatusID:   "root",
			ParentStatusID: "parent",
			ReplyID:        "reply",
		}
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "THREAD#root", m.PK)
		assert.Equal(t, "MISSING#reply", m.SK)
		assert.Equal(t, "STATUS#parent", m.GSI1PK)
		assert.Equal(t, "MISSING_REPLY", m.GSI1SK)
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("MarkFetching increments attempts and sets timestamps", func(t *testing.T) {
		m := &MissingReply{AttemptCount: 0}
		m.MarkFetching()
		assert.Equal(t, MissingReplyStatusFetching, m.Status)
		assert.Equal(t, 1, m.AttemptCount)
		require.NotNil(t, m.LastAttemptAt)
		assert.False(t, m.UpdatedAt.IsZero())
	})

	t.Run("MarkResolved clears errors and sets shorter TTL", func(t *testing.T) {
		m := &MissingReply{LastError: "x", FailureReason: "y"}
		before := time.Now()
		m.MarkResolved()
		assert.Equal(t, MissingReplyStatusResolved, m.Status)
		require.NotNil(t, m.ResolvedAt)
		assert.Empty(t, m.LastError)
		assert.Empty(t, m.FailureReason)
		assert.True(t, time.Unix(m.TTL, 0).After(before.Add(6*24*time.Hour)))
	})

	t.Run("MarkFailed schedules retries with backoff and handles permanent failures", func(t *testing.T) {
		now := time.Now()
		cases := []struct {
			attempts int
			minDelay time.Duration
		}{
			{1, 5 * time.Minute},
			{2, 15 * time.Minute},
			{3, time.Hour},
			{4, 6 * time.Hour},
		}

		for _, tc := range cases {
			m := &MissingReply{AttemptCount: tc.attempts, FailureReason: FailureReasonTimeout}
			m.MarkFailed("err", FailureReasonTimeout)
			assert.Equal(t, MissingReplyStatusFailed, m.Status)
			require.NotNil(t, m.NextRetryAt)
			assert.True(t, m.NextRetryAt.After(now.Add(tc.minDelay-time.Minute)))
		}

		m := &MissingReply{AttemptCount: 5, FailureReason: FailureReasonTimeout}
		m.MarkFailed("err", FailureReasonTimeout)
		assert.Nil(t, m.NextRetryAt)
		assert.True(t, m.IsPermanentFailure())

		m = &MissingReply{AttemptCount: 1, FailureReason: FailureReasonDeleted}
		m.MarkFailed("err", FailureReasonDeleted)
		assert.Nil(t, m.NextRetryAt)
		assert.True(t, m.IsPermanentFailure())
	})

	t.Run("ShouldRetry requires failed status, non-permanent, and next retry time passed", func(t *testing.T) {
		past := time.Now().Add(-time.Minute)
		m := &MissingReply{
			Status:        MissingReplyStatusFailed,
			FailureReason: FailureReasonTimeout,
			AttemptCount:  1,
			NextRetryAt:   &past,
		}
		assert.True(t, m.ShouldRetry())

		m.Status = MissingReplyStatusPending
		assert.False(t, m.ShouldRetry())

		m.Status = MissingReplyStatusFailed
		m.NextRetryAt = nil
		assert.False(t, m.ShouldRetry())

		m.FailureReason = FailureReasonForbidden
		assert.False(t, m.ShouldRetry())
	})

	t.Run("SetPriority clamps to [1,5]", func(t *testing.T) {
		m := &MissingReply{}
		m.SetPriority(0)
		assert.Equal(t, 1, m.Priority)
		m.SetPriority(6)
		assert.Equal(t, 5, m.Priority)
	})
}
