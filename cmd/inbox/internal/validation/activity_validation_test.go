package validation

import (
	"encoding/json"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestValidateRequestBody(t *testing.T) {
	logger := zap.NewNop()

	err := ValidateRequestBody(logger, nil)
	require.Error(t, err)
	require.IsType(t, &lift.LiftError{}, err)
	require.Equal(t, 400, err.(*lift.LiftError).StatusCode)

	err = ValidateRequestBody(logger, make([]byte, common.MaxActivitySize+1))
	require.Error(t, err)
	require.IsType(t, &lift.LiftError{}, err)
	require.Equal(t, 413, err.(*lift.LiftError).StatusCode)
	require.Equal(t, "PAYLOAD_TOO_LARGE", err.(*lift.LiftError).Code)
}

func TestParseActivity_InvalidTimestamp(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context":  activitypub.Context,
		"type":      activitypub.CreateType,
		"id":        "https://example.com/activities/1",
		"actor":     "https://remote.example/users/bob",
		"to":        []string{"https://example.com/users/alice"},
		"published": "not-a-time",
		"object":    "https://example.com/objects/1",
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	_, err = ParseActivity(logger, body)
	require.Error(t, err)
	require.IsType(t, &lift.LiftError{}, err)
	require.Equal(t, 400, err.(*lift.LiftError).StatusCode)
}

func TestParseActivity_InvalidJSON(t *testing.T) {
	logger := zap.NewNop()

	_, err := ParseActivity(logger, []byte("not-json"))
	require.Error(t, err)
	require.IsType(t, &lift.LiftError{}, err)
	require.Equal(t, 400, err.(*lift.LiftError).StatusCode)
}

func TestParseActivity_ValidActivity(t *testing.T) {
	logger := zap.NewNop()

	raw := map[string]any{
		"@context":  activitypub.Context,
		"type":      activitypub.CreateType,
		"id":        "https://example.com/activities/1",
		"actor":     "https://remote.example/users/bob",
		"to":        []string{"https://example.com/users/alice"},
		"published": "2026-01-01T00:00:00Z",
		"object": map[string]any{
			"type":    "Note",
			"id":      "https://example.com/objects/1",
			"content": "hello",
		},
	}
	body, err := json.Marshal(raw)
	require.NoError(t, err)

	activity, err := ParseActivity(logger, body)
	require.NoError(t, err)
	require.NotNil(t, activity)
	require.Equal(t, "https://example.com/activities/1", activity.ID)
	require.Equal(t, "https://remote.example/users/bob", activity.Actor)
}

func TestValidateActorUsername(t *testing.T) {
	require.Error(t, ValidateActorUsername("https://example.com/"))
	require.NoError(t, ValidateActorUsername("https://example.com/users/alice"))
}

func TestValidateActorUsername_InvalidURL(t *testing.T) {
	err := ValidateActorUsername("http://[::1")
	require.Error(t, err)
	require.IsType(t, &lift.LiftError{}, err)
	require.Equal(t, 400, err.(*lift.LiftError).StatusCode)
}

func TestIsAddressedTo(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	activity := &activitypub.Activity{
		BaseObject: activitypub.BaseObject{
			Type: activitypub.CreateType,
			ID:   "https://example.com/activities/1",
			To:   []string{actor.ID},
		},
		Actor:  "https://remote.example/users/bob",
		Object: "https://example.com/objects/1",
	}

	require.True(t, IsAddressedTo(activity, actor))
}

func TestIsAddressedTo_AcceptsCCBToBCCAndPublicAddress(t *testing.T) {
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://example.com/users/alice",
			Type: activitypub.PersonType,
		},
		Inbox: "https://example.com/users/alice/inbox",
	}

	base := activitypub.BaseObject{
		Type: activitypub.CreateType,
		ID:   "https://example.com/activities/1",
	}

	t.Run("cc inbox", func(t *testing.T) {
		bo := base
		bo.CC = []string{actor.Inbox}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("bto actor id", func(t *testing.T) {
		bo := base
		bo.BTo = []string{actor.ID}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("bcc actor id", func(t *testing.T) {
		bo := base
		bo.BCC = []string{actor.ID}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("public address", func(t *testing.T) {
		bo := base
		bo.To = []string{activitypub.PublicAddress}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("to inbox", func(t *testing.T) {
		bo := base
		bo.To = []string{actor.Inbox}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.True(t, IsAddressedTo(activity, actor))
	})

	t.Run("not addressed", func(t *testing.T) {
		bo := base
		bo.To = []string{"https://elsewhere.example/users/other"}
		bo.CC = []string{"https://elsewhere.example/users/other"}
		bo.BTo = []string{"https://elsewhere.example/users/other"}
		bo.BCC = []string{"https://elsewhere.example/users/other"}
		activity := &activitypub.Activity{
			BaseObject: bo,
			Actor:      "https://remote.example/users/bob",
			Object:     "https://example.com/objects/1",
		}
		require.False(t, IsAddressedTo(activity, actor))
	})
}
