package graph

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	storagepkg "github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func graphActAsContext(username, indicator string) context.Context {
	ctx := context.WithValue(context.Background(), common.ContextKeyClaims, &auth.Claims{
		Username: username,
		Scopes:   []string{"read", "write"},
	})
	if indicator != "" {
		ctx = context.WithValue(ctx, common.ContextKeyActAsAgent, indicator)
	}
	return ctx
}

func seedGraphActAsAgentAndGrant(storage *round12GraphStorage) {
	storage.SeedAccountUser(&storagepkg.User{Username: "agent-one", IsAgent: true, Role: "user", Approved: true})
	storage.SeedAgentShareGrant(&models.AgentShareGrant{
		AgentUsername:   "agent-one",
		GranteeUsername: "alice",
		GrantedBy:       "owner",
	})
}

func TestGraphActAs_RequireActingIdentity(t *testing.T) {
	t.Run("no indicator is the byte-identical owner path", func(t *testing.T) {
		resolver, _ := newRound12GraphResolver(t)
		username, acting, err := resolver.requireActingIdentity(graphActAsContext("alice", ""))
		require.NoError(t, err)
		require.Equal(t, "alice", username)
		require.Nil(t, acting)
	})

	t.Run("grant holder resolves to the agent with caller attribution", func(t *testing.T) {
		resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
		seedGraphActAsAgentAndGrant(storage)

		username, acting, err := resolver.requireActingIdentity(graphActAsContext("alice", "agent-one"))
		require.NoError(t, err)
		require.Equal(t, "agent-one", username)
		require.NotNil(t, acting)
		require.Equal(t, "agent-one", acting.AgentUsername)
		require.Equal(t, "alice", acting.ActedBy)
	})

	t.Run("missing grant fails closed with forbidden", func(t *testing.T) {
		resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
		storage.SeedAccountUser(&storagepkg.User{Username: "agent-one", IsAgent: true, Role: "user", Approved: true})

		_, _, err := resolver.requireActingIdentity(graphActAsContext("alice", "agent-one"))
		require.Error(t, err)
		require.ErrorContains(t, err, "no active agent share grant")
	})

	t.Run("malformed indicator is a bad request", func(t *testing.T) {
		resolver, _ := newRound12GraphResolver(t)

		_, _, err := resolver.requireActingIdentity(graphActAsContext("alice", "agent-one@example.com"))
		require.Error(t, err)
		require.ErrorContains(t, err, "invalid "+auth.ActAsAgentHeader)
	})

	t.Run("unknown agent is a bad request", func(t *testing.T) {
		resolver, _ := newRound12GraphResolver(t)

		_, _, err := resolver.requireActingIdentity(graphActAsContext("alice", "ghost-agent"))
		require.Error(t, err)
		require.ErrorContains(t, err, "act-as agent not found")
	})
}

func TestGraphActAs_CreateDraftAgentScopedWithAttribution(t *testing.T) {
	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	seedGraphActAsAgentAndGrant(storage)

	ctx := graphActAsContext("alice", "agent-one")
	title := "Acted Draft"
	draft, err := resolver.Mutation().CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "body",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.Equal(t, "agent-one", draft.AuthorID)
	require.NotNil(t, draft.ActedBy, "caller attribution must be surfaced on the draft")
	require.Contains(t, draft.ActedBy.ID, "/users/alice")
}

func TestGraphActAs_CreateDraftNoGrantForbidden(t *testing.T) {
	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	storage.SeedAccountUser(&storagepkg.User{Username: "agent-one", IsAgent: true, Role: "user", Approved: true})

	ctx := graphActAsContext("alice", "agent-one")
	title := "Should Not Exist"
	_, err := resolver.Mutation().CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "body",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, "no active agent share grant")
}

func TestGraphActAs_CreateDraftOwnerPathUnchanged(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)

	ctx := graphActAsContext("alice", "")
	title := "Ordinary Draft"
	draft, err := resolver.Mutation().CreateDraft(ctx, model.CreateDraftInput{
		ContentType:   model.ObjectTypeArticle,
		Title:         &title,
		Content:       "body",
		ContentFormat: model.ContentFormatMarkdown,
	})
	require.NoError(t, err)
	require.NotNil(t, draft)
	require.Equal(t, "alice", draft.AuthorID)
	require.Nil(t, draft.ActedBy, "owner path must not gain act-as attribution")
}

func TestGraphActAs_AcceptMessageRequestNoGrantForbidden(t *testing.T) {
	resolver, storage, _, _, _ := newRound12GraphResolverWithMocks(t)
	storage.SeedAccountUser(&storagepkg.User{Username: "agent-one", IsAgent: true, Role: "user", Approved: true})

	ctx := graphActAsContext("alice", "agent-one")
	_, err := resolver.Mutation().AcceptMessageRequest(ctx, "conv-1")
	require.Error(t, err)
	require.ErrorContains(t, err, "no active agent share grant")
}
