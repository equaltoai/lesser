package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestRound40DirectMessageMutationsSupportExtendedFields(t *testing.T) {
	t.Setenv("DISABLE_RATE_LIMITING", "true")
	config.ResetForTests()
	t.Cleanup(config.ResetForTests)

	resolver, _, _, _, _ := newRound12GraphResolverWithMocks(t)
	ctx := round12AuthContext("alice")

	sensitive := true
	spoilerText := "cw"
	language := "en"

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
}
