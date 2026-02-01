package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMuteAndConversationMute(t *testing.T) {
	t.Run("Mute keys and lifecycle", func(t *testing.T) {
		m := &Mute{
			Actor:  "https://example.com/users/alice",
			Object: "https://example.com/users/bob",
		}
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, "MUTE#alice", m.PK)
		assert.Equal(t, "MUTED#bob", m.SK)
		assert.Equal(t, "MUTED#bob", m.GSI1PK)
		assert.Equal(t, "MUTER#alice", m.GSI1SK)
		assert.Equal(t, MainTableName, m.TableName())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())

		m2 := &Mute{
			Actor:  "https://example.com/users/alice",
			Object: "https://example.com/users/bob",
		}
		require.NoError(t, m2.BeforeCreate())
		assert.Equal(t, "Mute", m2.Type)
		assert.NotEmpty(t, m2.ID)
		assert.False(t, m2.Published.IsZero())
		assert.False(t, m2.CreatedAt.IsZero())
	})

	t.Run("ConversationMute validates and sets TTL when expires set", func(t *testing.T) {
		cm := &ConversationMute{}
		assert.ErrorIs(t, cm.BeforeCreate(), ErrConversationMuteUsernameRequired)

		cm.Username = "alice"
		assert.ErrorIs(t, cm.BeforeCreate(), ErrConversationMuteConversationIDRequired)

		expires := time.Now().Add(10 * time.Minute)
		cm.ConversationID = "c1"
		cm.ExpiresAt = expires
		require.NoError(t, cm.BeforeCreate())
		assert.Equal(t, "USER#alice", cm.PK)
		assert.Equal(t, "CONVERSATION_MUTE#c1", cm.SK)
		assert.Equal(t, expires.Unix(), cm.TTL)
		assert.False(t, cm.CreatedAt.IsZero())
		assert.Equal(t, MainTableName, cm.TableName())
		assert.Equal(t, cm.PK, cm.GetPK())
		assert.Equal(t, cm.SK, cm.GetSK())

		cm.Username = "alice"
		cm.ConversationID = "c2"
		require.NoError(t, cm.UpdateKeys())
		assert.Equal(t, "CONVERSATION_MUTE#c2", cm.SK)
	})
}

func TestFollowAndHashtagFollows(t *testing.T) {
	t.Run("Follow state transitions update derived index", func(t *testing.T) {
		f := NewFollow("alice", "bob", "act1")
		assert.Equal(t, FollowStatePending, f.State)
		assert.Contains(t, f.GSI2PK, FollowStatePending)
		assert.Equal(t, MainTableName, f.TableName())

		f.Accept()
		assert.Equal(t, FollowStateAccepted, f.State)
		assert.NotNil(t, f.AcceptedAt)
		assert.Contains(t, f.GSI2PK, FollowStateAccepted)

		f.Reject()
		assert.Equal(t, FollowStateRejected, f.State)
		assert.Contains(t, f.GSI2PK, FollowStateRejected)
	})

	t.Run("HashtagFollow validates required fields", func(t *testing.T) {
		h := &HashtagFollow{}
		assert.ErrorContains(t, h.UpdateKeys(), "UserID is required")

		h.UserID = "alice"
		assert.ErrorContains(t, h.UpdateKeys(), "Hashtag is required")

		h.Hashtag = "golang"
		require.NoError(t, h.UpdateKeys())
		assert.Equal(t, "user#alice", h.PK)
		assert.Equal(t, "hashtag#golang", h.SK)
		assert.Equal(t, MainTableName, h.TableName())

		h2 := &HashtagFollow{}
		h2.UpdateKeysWithParams("alice", "golang")
		assert.Equal(t, "user#alice", h2.PK)
		assert.Equal(t, "hashtag#golang", h2.SK)
	})

	t.Run("HashtagNotificationSettings validates required fields", func(t *testing.T) {
		var nf NotificationFilter
		assert.Equal(t, MainTableName, nf.TableName())

		s := &HashtagNotificationSettings{}
		assert.ErrorContains(t, s.UpdateKeys(), "UserID is required")
		s.UserID = "alice"
		assert.ErrorContains(t, s.UpdateKeys(), "Hashtag is required")
		s.Hashtag = "golang"
		require.NoError(t, s.UpdateKeys())
		assert.Equal(t, "user#alice", s.PK)
		assert.Equal(t, "settings#golang", s.SK)
		assert.Equal(t, MainTableName, s.TableName())
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())

		s2 := &HashtagNotificationSettings{}
		s2.UpdateKeysWithParams("alice", "golang")
		assert.Equal(t, "user#alice", s2.PK)
		assert.Equal(t, "settings#golang", s2.SK)
	})

	t.Run("HashtagMute sets composite keys when fields present", func(t *testing.T) {
		hm := &HashtagMute{Username: "alice", Hashtag: "golang"}
		hm.UpdateKeys()
		assert.Equal(t, "user#alice", hm.PK)
		assert.Equal(t, "mute#golang", hm.SK)
		assert.Equal(t, MainTableName, hm.TableName())
	})
}

func TestListAndMembers(t *testing.T) {
	t.Run("List lifecycle and UpdateKeys validation", func(t *testing.T) {
		l := &List{}
		assert.Error(t, l.UpdateKeys())

		l.ID = "l1"
		assert.Error(t, l.UpdateKeys())

		l.Username = "alice"
		require.NoError(t, l.BeforeCreate())
		assert.Equal(t, "LIST#l1", l.PK)
		assert.Equal(t, SKMetadata, l.SK)
		assert.Equal(t, "USER_LISTS#alice", l.GSI1PK)
		assert.Equal(t, "l1", l.GSI1SK)
		assert.Equal(t, MainTableName, l.TableName())
		assert.Equal(t, l.PK, l.GetPK())
		assert.Equal(t, l.SK, l.GetSK())

		before := l.UpdatedAt
		require.NoError(t, l.BeforeUpdate())
		assert.True(t, l.UpdatedAt.After(before) || l.UpdatedAt.Equal(before))
	})

	t.Run("ListMember validates required fields and builds reverse lookup keys", func(t *testing.T) {
		lm := &ListMember{}
		assert.ErrorContains(t, lm.UpdateKeys(), "ListID is required")
		lm.ListID = "l1"
		assert.ErrorContains(t, lm.UpdateKeys(), "AccountID is required")

		lm.AccountID = "acct1"
		lm.ListUsername = "alice"
		require.NoError(t, lm.UpdateKeys())
		assert.Equal(t, "LIST_MEMBERS#l1", lm.PK)
		assert.Equal(t, "acct1", lm.SK)
		assert.Equal(t, "ACCOUNT_LISTS#acct1", lm.GSI1PK)
		assert.Equal(t, "l1#alice", lm.GSI1SK)
		assert.Equal(t, MainTableName, lm.TableName())

		lm2 := &ListMember{ListID: "l2", AccountID: "acct2", ListUsername: "alice"}
		require.NoError(t, lm2.BeforeCreate())
		assert.Equal(t, "LIST_MEMBERS#l2", lm2.PK)
		assert.Equal(t, "acct2", lm2.SK)
		assert.False(t, lm2.AddedAt.IsZero())
	})
}

func TestMarkers(t *testing.T) {
	t.Run("Marker model sets keys", func(t *testing.T) {
		m := &Marker{Username: "alice", Timeline: "home"}
		require.NoError(t, m.BeforeCreate())
		assert.Equal(t, "USER#alice", m.PK)
		assert.Equal(t, "MARKER#home", m.SK)
		assert.False(t, m.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, m.TableName())

		before := m.UpdatedAt
		require.NoError(t, m.BeforeUpdate())
		assert.True(t, m.UpdatedAt.After(before) || m.UpdatedAt.Equal(before))
		require.NoError(t, m.UpdateKeys())
		assert.Equal(t, m.PK, m.GetPK())
		assert.Equal(t, m.SK, m.GetSK())
	})

	t.Run("TimelineMarker validates and updates timestamps", func(t *testing.T) {
		tm := &TimelineMarker{}
		assert.ErrorContains(t, tm.UpdateKeys(), "username is required")
		tm.Username = "alice"
		assert.ErrorContains(t, tm.UpdateKeys(), "timeline is required")

		tm.Timeline = "home"
		require.NoError(t, tm.BeforeCreate())
		assert.Equal(t, "USER#alice", tm.PK)
		assert.Equal(t, "MARKER#home", tm.SK)
		assert.False(t, tm.UpdatedAt.IsZero())
		assert.Equal(t, MainTableName, tm.TableName())
		assert.Equal(t, tm.PK, tm.GetPK())
		assert.Equal(t, tm.SK, tm.GetSK())

		before := tm.UpdatedAt
		require.NoError(t, tm.BeforeUpdate())
		assert.True(t, tm.UpdatedAt.After(before) || tm.UpdatedAt.Equal(before))
		require.NoError(t, tm.UpdateKeys())
	})
}
