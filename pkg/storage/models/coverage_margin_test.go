package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceConfigModelHelpers_CoverageMargin(t *testing.T) {
	t.Run("translation config", func(t *testing.T) {
		cfg := NewInstanceTranslationConfig()
		require.Equal(t, MainTableName, cfg.TableName())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTranslationConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
		require.False(t, cfg.UpdatedAt.IsZero())

		cfg = &InstanceTranslationConfig{}
		require.NoError(t, cfg.UpdateKeys())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTranslationConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
	})

	t.Run("trust config", func(t *testing.T) {
		cfg := NewInstanceTrustConfig()
		require.Equal(t, MainTableName, cfg.TableName())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTrustConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
		require.False(t, cfg.UpdatedAt.IsZero())

		cfg = &InstanceTrustConfig{}
		require.NoError(t, cfg.UpdateKeys())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTrustConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
	})

	t.Run("tips config", func(t *testing.T) {
		cfg := NewInstanceTipsConfig()
		require.Equal(t, MainTableName, cfg.TableName())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTipsConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
		require.False(t, cfg.Managed.Enabled)
		require.False(t, cfg.UpdatedAt.IsZero())

		cfg = &InstanceTipsConfig{}
		require.NoError(t, cfg.UpdateKeys())
		require.Equal(t, instanceConfigPK, cfg.GetPK())
		require.Equal(t, SKTipsConfig, cfg.GetSK())
		require.NotNil(t, cfg.Managed)
	})

	t.Run("well-known soul agent", func(t *testing.T) {
		record := NewInstanceWellKnownLesserSoulAgent()
		require.Equal(t, MainTableName, record.TableName())
		require.Equal(t, instanceConfigPK, record.GetPK())
		require.Equal(t, SKWellKnownLesserSoulAgent, record.GetSK())
		require.Empty(t, record.ProofValue)
		require.True(t, record.ExpiresAt.IsZero())
		require.False(t, record.UpdatedAt.IsZero())

		record = &InstanceWellKnownLesserSoulAgent{}
		require.NoError(t, record.UpdateKeys())
		require.Equal(t, instanceConfigPK, record.GetPK())
		require.Equal(t, SKWellKnownLesserSoulAgent, record.GetSK())
	})
}

func TestVisibilityAndModerationModelHelpers_CoverageMargin(t *testing.T) {
	t.Run("direct message tombstone", func(t *testing.T) {
		record := &DirectMessageTombstone{}
		require.Error(t, record.UpdateKeys())

		record.ViewerUsername = "alice"
		require.Error(t, record.UpdateKeys())

		record.StatusID = "status-1"
		require.NoError(t, record.UpdateKeys())
		require.Equal(t, MainTableName, record.TableName())
		require.Equal(t, "DM_MESSAGE_TOMBSTONE#alice", record.GetPK())
		require.Equal(t, "STATUS#status-1", record.GetSK())
		require.False(t, record.CreatedAt.IsZero())
	})

	t.Run("graphql stream subscription", func(t *testing.T) {
		required := []GraphQLStreamSubscription{
			{},
			{Stream: "public"},
			{Stream: "public", ConnectionID: "conn-1"},
			{Stream: "public", ConnectionID: "conn-1", SubscriptionID: "sub-1"},
			{Stream: "public", ConnectionID: "conn-1", SubscriptionID: "sub-1", Field: "timeline"},
		}
		for _, record := range required {
			require.Error(t, record.UpdateKeys())
		}

		record := &GraphQLStreamSubscription{
			Stream:         "public",
			ConnectionID:   "conn-1",
			SubscriptionID: "sub-1",
			Field:          "timeline",
			UserID:         "alice",
		}
		require.NoError(t, record.UpdateKeys())
		require.Equal(t, MainTableName, record.TableName())
		require.Equal(t, "GQLSUB#public", record.GetPK())
		require.Equal(t, "CONN#conn-1#SUB#sub-1", record.GetSK())
		require.Equal(t, "CONN#conn-1", record.GSI1PK)
		require.Equal(t, "SUB#sub-1#STREAM#public", record.GSI1SK)
	})

	t.Run("domain block records", func(t *testing.T) {
		createdAt := time.Unix(1700000000, 0).UTC()

		userBlock := &UserDomainBlock{Username: "alice", Domain: "bad.example", CreatedAt: createdAt}
		require.NoError(t, userBlock.UpdateKeys())
		require.Equal(t, MainTableName, userBlock.TableName())
		require.Equal(t, "USER#alice", userBlock.GetPK())
		require.Equal(t, "DOMAIN_BLOCK#bad.example", userBlock.GetSK())

		instanceBlock := &InstanceDomainBlock{Domain: "bad.example", CreatedAt: createdAt}
		require.NoError(t, instanceBlock.UpdateKeys())
		require.Equal(t, MainTableName, instanceBlock.TableName())
		require.Equal(t, "DOMAIN_BLOCK#bad.example", instanceBlock.GetPK())
		require.Equal(t, "DOMAIN_BLOCK#bad.example", instanceBlock.GetSK())
		require.Equal(t, "DOMAIN_BLOCKS", instanceBlock.GSI1PK)
		require.Equal(t, "1700000000#bad.example", instanceBlock.GSI1SK)
		require.Equal(t, "INSTANCE_DOMAIN_BLOCK", instanceBlock.Type)

		emailBlock := &EmailDomainBlock{ID: "email-1", Domain: "mail.example", CreatedAt: createdAt}
		require.NoError(t, emailBlock.UpdateKeys())
		require.Equal(t, MainTableName, emailBlock.TableName())
		require.Equal(t, "email-1", emailBlock.GetID())
		require.Equal(t, "EMAIL_DOMAIN_BLOCK#mail.example", emailBlock.GetPK())
		require.Equal(t, "EMAIL_DOMAIN_BLOCK#mail.example", emailBlock.GetSK())
		require.Equal(t, "mail.example", emailBlock.GetDomain())
		require.Equal(t, "EMAIL_DOMAIN_BLOCKS", emailBlock.GSI1PK)
		require.Equal(t, createdAt.Format(time.RFC3339), emailBlock.GSI1SK)

		allow := &DomainAllow{ID: "allow-1", Domain: "good.example", CreatedAt: createdAt}
		require.NoError(t, allow.UpdateKeys())
		require.Equal(t, MainTableName, allow.TableName())
		require.Equal(t, "allow-1", allow.GetID())
		require.Equal(t, "DOMAIN_ALLOW#good.example", allow.GetPK())
		require.Equal(t, "DOMAIN_ALLOW#good.example", allow.GetSK())
		require.Equal(t, "good.example", allow.GetDomain())
		require.Equal(t, "DOMAIN_ALLOWS", allow.GSI1PK)
		require.Equal(t, createdAt.Format(time.RFC3339), allow.GSI1SK)
	})
}

func TestDiscoveryFollowModelHelpers_CoverageMargin(t *testing.T) {
	t.Run("hashtag follow", func(t *testing.T) {
		follow := &HashtagFollow{}
		require.EqualError(t, follow.UpdateKeys(), "UserID is required")

		follow.UserID = "alice"
		require.EqualError(t, follow.UpdateKeys(), "Hashtag is required")

		follow.Hashtag = "lesser"
		require.NoError(t, follow.UpdateKeys())
		require.Equal(t, "user#alice", follow.GetPK())
		require.Equal(t, "hashtag#lesser", follow.GetSK())
		require.Equal(t, MainTableName, follow.TableName())

		follow.UpdateKeysWithParams("bob", "fediverse")
		require.Equal(t, "bob", follow.UserID)
		require.Equal(t, "fediverse", follow.Hashtag)
		require.Equal(t, "user#bob", follow.GetPK())
		require.Equal(t, "hashtag#fediverse", follow.GetSK())
	})

	t.Run("dns cache", func(t *testing.T) {
		cache := &DNSCache{}
		require.NoError(t, cache.UpdateKeys())
		require.Empty(t, cache.GetPK())
		require.Empty(t, cache.GetSK())

		cache.Hostname = "remote.example"
		require.NoError(t, cache.UpdateKeys())
		require.Equal(t, MainTableName, cache.TableName())
		require.Equal(t, "DNSCACHE#remote.example", cache.GetPK())
		require.Equal(t, SKEntry, cache.GetSK())
	})

	t.Run("account note keys", func(t *testing.T) {
		note := &AccountNote{PK: "ACCOUNT_NOTE#alice", SK: "NOTE#remote"}
		assert.Equal(t, "ACCOUNT_NOTE#alice", note.GetPK())
		assert.Equal(t, "NOTE#remote", note.GetSK())
	})
}
