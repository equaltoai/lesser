package graph

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	apperrors "github.com/equaltoai/lesser/pkg/errors"

	"github.com/stretchr/testify/require"
)

func TestRound12MutationResolvers_Accounts_RegisterAccount_AndVisibility(t *testing.T) {
	const expectedGraphQLError = "registration is not supported over GraphQL; use POST /api/v1/accounts"

	resolver, _ := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// Required username validation.
	_, err := mut.RegisterAccount(context.Background(), model.RegisterAccountInput{})
	require.Error(t, err)

	// Register attempt through real service (may fail due to existing account); error path still exercises resolver.
	_, err = mut.RegisterAccount(context.Background(), model.RegisterAccountInput{
		Username:  "alice",
		Agreement: true,
	})
	require.Error(t, err)
	require.ErrorContains(t, err, expectedGraphQLError)
	var appErr *apperrors.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, 400, appErr.HTTPStatusCode)

	require.Equal(t, "public", postingVisibilityFromGraphQL(model.VisibilityPublic))
	require.Equal(t, "unlisted", postingVisibilityFromGraphQL(model.VisibilityUnlisted))
	require.Equal(t, "private", postingVisibilityFromGraphQL(model.VisibilityFollowers))
	require.Equal(t, "direct", postingVisibilityFromGraphQL(model.VisibilityDirect))
	require.Equal(t, "", postingVisibilityFromGraphQL(model.Visibility("nope")))
}

func TestRound12MutationResolvers_Accounts_UpdateAccountQuotePermissions_Paths(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// Auth required.
	_, err := mut.UpdateAccountQuotePermissions(context.Background(), model.UpdateAccountQuotePermissionsInput{})
	require.Error(t, err)

	// Success path updates fields and ensures block list is non-nil.
	blockList := []string{"spammer"}
	updated, err := mut.UpdateAccountQuotePermissions(round12AuthContext("alice"), model.UpdateAccountQuotePermissionsInput{
		AllowPublic:    ptrBool(true),
		AllowFollowers: ptrBool(false),
		AllowMentioned: ptrBool(true),
		BlockList:      blockList,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	require.NotNil(t, updated.BlockList)

	// Not-found path uses dedicated username to force create-then-update.
	created, err := mut.UpdateAccountQuotePermissions(round12AuthContext("missing-perms"), model.UpdateAccountQuotePermissionsInput{
		AllowPublic: ptrBool(false),
		BlockList:   nil,
	})
	require.NoError(t, err)
	require.NotNil(t, created)
	require.NotNil(t, created.BlockList)
}

func TestRound12MutationResolvers_Accounts_SaveMarkers_Paths(t *testing.T) {
	resolver, _ := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// Auth required.
	_, err := mut.SaveMarkers(context.Background(), []*model.SaveMarkerInput{{Timeline: model.MarkerTimelineHome, LastReadID: "1"}})
	require.Error(t, err)

	// Empty marker list validation.
	_, err = mut.SaveMarkers(round12AuthContext("alice"), nil)
	require.Error(t, err)

	// Success path persists markers and returns a marker set (may contain nil marker entries with this harness).
	set, err := mut.SaveMarkers(round12AuthContext("alice"), []*model.SaveMarkerInput{
		{Timeline: model.MarkerTimelineHome, LastReadID: "status-1"},
		nil,
		{Timeline: model.MarkerTimelineNotifications, LastReadID: "notif-1"},
	})
	require.NoError(t, err)
	require.NotNil(t, set)

	// LastReadID validation.
	_, err = mut.SaveMarkers(round12AuthContext("alice"), []*model.SaveMarkerInput{
		{Timeline: model.MarkerTimelineHome, LastReadID: ""},
	})
	require.Error(t, err)
}
