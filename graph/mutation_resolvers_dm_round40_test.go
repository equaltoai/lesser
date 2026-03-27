package graph

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestRound40DirectMessageMutationsSupportExtendedFields(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	ctx := round12AuthContext("alice")

	sensitive := true
	spoilerText := "cw"
	language := "en"
	expectedMentionURL := resolver.Registry.GetConfig().BaseURL + "/users/bob"

	first, err := resolver.Mutation().SendDirectMessage(
		ctx,
		"bob",
		"hello bob",
		nil,
		&sensitive,
		&spoilerText,
		&language,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NotNil(t, first.Conversation)
	require.NotNil(t, first.Message)
	require.Equal(t, model.VisibilityDirect, first.Message.Visibility)
	require.True(t, first.Message.Sensitive)
	require.NotNil(t, first.Message.SpoilerText)
	require.Equal(t, "cw", *first.Message.SpoilerText)
	require.Len(t, first.Message.Mentions, 1)
	require.Equal(t, expectedMentionURL, first.Message.Mentions[0].ID)
	require.Equal(t, "bob", first.Message.Mentions[0].Username)
	require.Nil(t, first.Message.Mentions[0].Domain)
	firstStored, err := storage.Status().GetStatus(ctx, first.Message.ID)
	require.NoError(t, err)
	require.Equal(t, "en", firstStored.Language)
	require.Equal(t, []string{expectedMentionURL}, firstStored.Mentions)

	replySpoiler := "reply cw"
	reply, err := resolver.Mutation().SendMessage(
		ctx,
		first.Conversation.ID,
		"replying",
		nil,
		&sensitive,
		&replySpoiler,
		&language,
		&first.Message.ID,
		nil,
	)
	require.NoError(t, err)
	require.NotNil(t, reply)
	require.NotNil(t, reply.Message)
	require.NotNil(t, reply.Message.InReplyTo)
	require.Equal(t, first.Message.ID, reply.Message.InReplyTo.ID)
	require.NotNil(t, reply.Message.SpoilerText)
	require.Equal(t, "reply cw", *reply.Message.SpoilerText)
	require.Len(t, reply.Message.Mentions, 1)
	require.Equal(t, expectedMentionURL, reply.Message.Mentions[0].ID)
	replyStored, err := storage.Status().GetStatus(ctx, reply.Message.ID)
	require.NoError(t, err)
	require.Equal(t, "en", replyStored.Language)
	require.Equal(t, []string{expectedMentionURL}, replyStored.Mentions)
}

func TestRound40DirectMessageMutationsValidateEmptyInputs(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	ctx := round12AuthContext("alice")

	t.Run("sendDirectMessage rejects empty content", func(t *testing.T) {
		payload, err := resolver.Mutation().SendDirectMessage(
			ctx,
			"bob",
			"",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)
		require.Nil(t, payload)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to send direct message")
		require.ErrorContains(t, err, "content")
		require.ErrorContains(t, err, "required")
	})

	t.Run("sendDirectMessage rejects empty recipient", func(t *testing.T) {
		payload, err := resolver.Mutation().SendDirectMessage(
			ctx,
			" ",
			"hello",
			nil,
			nil,
			nil,
			nil,
			nil,
			nil,
		)
		require.Nil(t, payload)
		require.Error(t, err)
		require.ErrorContains(t, err, "failed to send direct message")
		require.ErrorContains(t, err, "invalid")
	})
}

func TestRound40ConvertStatusToObject_PreservesRemoteMentionMetadata(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	now := time.Now().UTC()
	status := &storagemodels.Status{
		StatusID:       "dm-remote",
		AuthorUsername: "alice",
		AuthorID:       resolver.Registry.GetConfig().BaseURL + "/users/alice",
		Visibility:     storagemodels.VisibilityDirect,
		Content:        "hi remote",
		Mentions:       []string{"https://remote.example/users/bob"},
		PublishedAt:    now,
		CreatedAt:      now,
		UpdatedAt:      now,
		ModifiedAt:     now,
		Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:      resolver.Registry.GetConfig().BaseURL + "/objects/dm-remote",
				Type:    "Note",
				To:      []string{"https://remote.example/users/bob"},
				Context: activitypub.Context,
			},
			Content:      "hi remote",
			AttributedTo: resolver.Registry.GetConfig().BaseURL + "/users/alice",
		},
	}

	object := resolver.convertStatusToObject(round12AuthContext("alice"), status)
	require.NotNil(t, object)
	require.Len(t, object.Mentions, 1)
	require.Equal(t, "https://remote.example/users/bob", object.Mentions[0].ID)
	require.Equal(t, "bob", object.Mentions[0].Username)
	require.NotNil(t, object.Mentions[0].Domain)
	require.Equal(t, "remote.example", *object.Mentions[0].Domain)
}
