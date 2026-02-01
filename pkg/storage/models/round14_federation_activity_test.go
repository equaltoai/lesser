package models

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFederationActivity_BeforeCreate_AndHelpers(t *testing.T) {
	t.Run("BeforeCreate sets keys, timestamps, TTL, and validates", func(t *testing.T) {
		fa := &FederationActivity{
			Domain:       "example.com",
			ActivityType: "Create",
			ActorID:      "https://remote.example/users/alice",
		}

		before := time.Now()
		require.NoError(t, fa.BeforeCreate())

		assert.False(t, fa.CreatedAt.IsZero())
		assert.True(t, fa.CreatedAt.Equal(fa.UpdatedAt))
		assert.False(t, fa.Timestamp.IsZero())

		assert.NotEmpty(t, fa.ID)
		assert.Equal(t, "fed_activity#example.com", fa.PK)
		assert.Contains(t, fa.SK, "activity#")

		assert.Equal(t, "FED_TYPE#Create", fa.GSI1PK)
		assert.Contains(t, fa.GSI1SK, "example.com")
		assert.Equal(t, "FED_ACTOR#https://remote.example/users/alice", fa.GSI2PK)
		assert.NotEmpty(t, fa.GSI2SK)

		assert.WithinDuration(t, before.Add(90*24*time.Hour), time.Unix(fa.ExpiresAt, 0), 2*time.Second)

		assert.Equal(t, fa.PK, fa.GetPK())
		assert.Equal(t, fa.SK, fa.GetSK())
		assert.Equal(t, MainTableName, fa.TableName())
	})

	t.Run("setupGSIKeys clears actor index when ActorID empty", func(t *testing.T) {
		fa := &FederationActivity{
			Domain:       "example.com",
			ActivityType: "Update",
			ActorID:      "",
			ID:           "id-1",
			Timestamp:    time.Unix(1700000000, 0).UTC(),
		}

		fa.setupGSIKeys()
		assert.Equal(t, "FED_TYPE#Update", fa.GSI1PK)
		assert.Empty(t, fa.GSI2PK)
		assert.Empty(t, fa.GSI2SK)
	})

	t.Run("UpdateKeys updates primary and GSI keys", func(t *testing.T) {
		ts := time.Unix(1700000000, 0).UTC()
		fa := &FederationActivity{
			ID:           "id-1",
			Domain:       "example.com",
			ActivityType: "Follow",
			ActorID:      "https://example.net/users/bob",
			Timestamp:    ts,
		}

		require.NoError(t, fa.UpdateKeys())
		assert.Equal(t, "fed_activity#example.com", fa.PK)
		assert.Contains(t, fa.SK, "activity#")
		assert.Equal(t, "FED_TYPE#Follow", fa.GSI1PK)
		assert.Equal(t, "FED_ACTOR#https://example.net/users/bob", fa.GSI2PK)
	})

	t.Run("ExtractDomain parses ActivityPub actor IDs", func(t *testing.T) {
		assert.Equal(t, "example.com", ExtractDomain("https://Example.com/users/alice"))
		assert.Equal(t, "", ExtractDomain("not-a-url"))
	})

	t.Run("IsRemote compares against local domain", func(t *testing.T) {
		fa := &FederationActivity{Domain: "remote.example"}
		assert.True(t, fa.IsRemote("local.example"))
		assert.False(t, fa.IsRemote("remote.example"))
	})

	t.Run("SetProperty/GetProperty handle nil map", func(t *testing.T) {
		fa := &FederationActivity{}
		fa.SetProperty("k", "v")
		got, ok := fa.GetProperty("k")
		assert.True(t, ok)
		assert.Equal(t, "v", got)

		_, ok = fa.GetProperty("missing")
		assert.False(t, ok)
	})

	t.Run("SetHeader/GetHeader handle nil map", func(t *testing.T) {
		fa := &FederationActivity{}
		fa.SetHeader("x-test", "1")
		got, ok := fa.GetHeader("x-test")
		assert.True(t, ok)
		assert.Equal(t, "1", got)

		_, ok = fa.GetHeader("missing")
		assert.False(t, ok)
	})

	t.Run("MarkSuccess and MarkFailed toggle fields", func(t *testing.T) {
		fa := &FederationActivity{}
		fa.MarkFailed("boom")
		assert.False(t, fa.Success)
		assert.Equal(t, "boom", fa.ErrorMessage)

		fa.MarkSuccess()
		assert.True(t, fa.Success)
		assert.Empty(t, fa.ErrorMessage)
	})

	t.Run("Builder populates fields and supports error helper", func(t *testing.T) {
		info := &InstanceInfo{Domain: "remote.example", LastSeen: time.Now()}

		activity := NewFederationActivityBuilder().
			FromDomain("remote.example").
			WithType("Delete").
			WithActor("https://remote.example/users/alice").
			WithObject("https://remote.example/objects/1", "Note").
			WithResponseTime(12.3).
			WithVolume(10, 20).
			WithInstanceInfo(info).
			WithError(errors.New("failed")).
			Build()

		assert.Equal(t, "remote.example", activity.Domain)
		assert.Equal(t, "Delete", activity.ActivityType)
		assert.Equal(t, "https://remote.example/users/alice", activity.ActorID)
		assert.Equal(t, "https://remote.example/objects/1", activity.ObjectID)
		assert.Equal(t, "Note", activity.ObjectType)
		assert.InDelta(t, 12.3, activity.ResponseTime, 0.000001)
		assert.Equal(t, int64(10), activity.InboundSize)
		assert.Equal(t, int64(20), activity.OutboundSize)
		assert.Same(t, info, activity.InstanceInfo)
		assert.False(t, activity.Success)
		assert.Equal(t, "failed", activity.ErrorMessage)

		assert.Equal(t, MainTableName, (FederationActivityBuilder{}).TableName())
		assert.Equal(t, MainTableName, (InstanceInfo{}).TableName())
	})
}
