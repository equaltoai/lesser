package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12ListResolvers_QueryAndMutations(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateAll = true
	state.autoPopulateScan = true
	state.autoPopulateCount = 2

	ctx := round12AuthContext("alice")

	created, err := resolver.Mutation().CreateList(ctx, model.CreateListInput{
		Title: "My List",
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotEmpty(t, created.ID)

	newTitle := "Updated List"
	updated, err := resolver.Mutation().UpdateList(ctx, created.ID, model.UpdateListInput{
		Title: &newTitle,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)

	added, err := resolver.Mutation().AddAccountsToList(ctx, created.ID, []string{"bob", "carol"})
	require.NoError(t, err)
	require.NotNil(t, added)

	removed, err := resolver.Mutation().RemoveAccountsFromList(ctx, created.ID, []string{"bob"})
	require.NoError(t, err)
	require.NotNil(t, removed)

	lists, err := resolver.Query().Lists(ctx)
	require.NoError(t, err)
	require.NotNil(t, lists)

	list, err := resolver.Query().List(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, list)

	accounts, err := resolver.Query().ListAccounts(ctx, created.ID)
	require.NoError(t, err)
	require.NotNil(t, accounts)

	okBool, err := resolver.Mutation().DeleteList(ctx, created.ID)
	require.NoError(t, err)
	require.True(t, okBool)
}

func TestRound12ConversationResolvers_QueryAndMutations(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateScan = true
	state.autoPopulateCount = 2

	ctx := round12AuthContext("alice")

	convos, err := resolver.Query().Conversations(ctx, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, convos)

	convo, err := resolver.Query().Conversation(ctx, "conv_1")
	require.NoError(t, err)
	require.NotNil(t, convo)

	marked, err := resolver.Mutation().MarkConversationAsRead(ctx, "conv_1")
	require.NoError(t, err)
	require.NotNil(t, marked)

	okBool, err := resolver.Mutation().DeleteConversation(ctx, "conv_1")
	require.NoError(t, err)
	require.True(t, okBool)
}

func TestRound12AdminResolvers_ScheduledStatuses(t *testing.T) {
	resolver, _, _, _, state := newRound12GraphResolverWithMocks(t)
	state.autoPopulateAll = true
	state.autoPopulateCount = 2

	ctx := round12AuthContext("alice")

	first := 2
	statuses, err := resolver.Query().ScheduledStatuses(ctx, &first, nil)
	require.NoError(t, err)
	require.NotNil(t, statuses)

	status, err := resolver.Query().ScheduledStatus(ctx, "sched-1")
	require.NoError(t, err)
	require.NotNil(t, status)
}

