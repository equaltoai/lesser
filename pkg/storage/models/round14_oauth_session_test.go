package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOAuthAuthSession_LifecycleAndHelpers(t *testing.T) {
	t.Run("BeforeCreate generates IDs/tokens, keys, GSIs, and defaults", func(t *testing.T) {
		s := &OAuthAuthSession{
			ClientID:     "client-1",
			RedirectURI:  "https://app.example/callback",
			Username:     "alice",
			State:        "state-123",
			IsSecure:     true,
			IPAddress:    "1.2.3.4",
			UserAgent:    "ua",
			CodeChallenge: "cc",
		}

		before := time.Now()
		require.NoError(t, s.BeforeCreate())

		assert.NotEmpty(t, s.SessionID)
		assert.NotEmpty(t, s.CSRFToken)
		assert.Equal(t, "initiated", s.FlowStep)

		assert.Equal(t, "OAUTH_AUTH#"+s.SessionID, s.PK)
		assert.Equal(t, "SESSION#"+s.SessionID, s.SK)
		assert.Equal(t, s.PK, s.GetPK())
		assert.Equal(t, s.SK, s.GetSK())
		assert.Equal(t, MainTableName, s.TableName())

		assert.Equal(t, "USER_OAUTH#alice", s.GSI1PK)
		assert.Contains(t, s.GSI1SK, s.SessionID)
		assert.Equal(t, "OAUTH_STATE#state-123", s.GSI2PK)
		assert.Equal(t, s.SessionID, s.GSI2SK)

		assert.WithinDuration(t, before.Add(time.Hour), time.Unix(s.ExpiresAt, 0), 2*time.Second)
	})

	t.Run("UpdateKeys requires SessionID", func(t *testing.T) {
		s := &OAuthAuthSession{}
		assert.Error(t, s.UpdateKeys())
	})

	t.Run("SetUser updates user-session GSI keys", func(t *testing.T) {
		s := &OAuthAuthSession{
			SessionID:   "sess-1",
			ClientID:    "client-1",
			RedirectURI: "https://cb",
			CSRFToken:   "csrf",
			CreatedAt:   time.Unix(1700000000, 0).UTC(),
			ExpiresAt:   time.Now().Add(time.Minute).Unix(),
		}
		s.SetUser("bob")
		assert.Equal(t, "USER_OAUTH#bob", s.GSI1PK)
		assert.Contains(t, s.GSI1SK, "sess-1")
	})

	t.Run("Authorize marks session authorized and sets timestamp", func(t *testing.T) {
		s := &OAuthAuthSession{}
		s.Authorize()
		assert.True(t, s.IsAuthorized)
		assert.Equal(t, "authorized", s.FlowStep)
		require.NotNil(t, s.AuthorizedAt)
	})

	t.Run("SetFlowStep merges flow data", func(t *testing.T) {
		s := &OAuthAuthSession{}
		s.SetFlowStep("login", map[string]interface{}{"a": 1})
		s.SetFlowStep("consent", map[string]interface{}{"b": 2})
		assert.Equal(t, "consent", s.FlowStep)
		assert.Equal(t, 1, s.FlowData["a"])
		assert.Equal(t, 2, s.FlowData["b"])
	})

	t.Run("Validity helpers: IsValid/IsExpired/CanAuthorize/RemainingTime", func(t *testing.T) {
		s := &OAuthAuthSession{
			Username:  "alice",
			FlowStep:  "consent",
			ExpiresAt: time.Now().Add(time.Minute).Unix(),
		}
		assert.True(t, s.IsValid())
		assert.False(t, s.IsExpired())
		assert.True(t, s.CanAuthorize())
		assert.True(t, s.RemainingTime() > 0)

		s.ExpiresAt = time.Now().Add(-time.Second).Unix()
		assert.False(t, s.IsValid())
		assert.True(t, s.IsExpired())
		assert.False(t, s.CanAuthorize())
	})

	t.Run("Touch and context helpers", func(t *testing.T) {
		s := &OAuthAuthSession{}
		before := s.LastUsedAt
		s.Touch()
		assert.True(t, s.LastUsedAt.After(before) || before.IsZero())

		s.SetContext("k", "v")
		got, ok := s.GetContext("k")
		assert.True(t, ok)
		assert.Equal(t, "v", got)

		_, ok = s.GetContext("missing")
		assert.False(t, ok)
	})
}

