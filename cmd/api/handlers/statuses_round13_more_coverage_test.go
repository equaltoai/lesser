package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesRound13_AccountStatusesFilteringAndHelpers(t *testing.T) {
	cfg := round10TestConfig()

	now := time.Now().UTC()
	s1 := &storagemodels.Status{
		StatusID:       "s1",
		AuthorUsername: "alice",
		Hashtags:       []string{"go", "test"},
		PublishedAt:    now.Add(-1 * time.Hour),
	}
	s2 := &storagemodels.Status{
		StatusID:       "s2",
		AuthorUsername: "alice",
		ReblogOfID:     "orig",
		Hashtags:       []string{"test"},
		PublishedAt:    now.Add(-2 * time.Hour),
	}

	reg := &RegistryStub{
		NotesSvc: &NotesServiceStub{
			ListNotesFunc: func(_ context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
				if query != nil && query.OnlyMedia {
					return &notes.Result{
						Notes: []*storagemodels.Status{},
						Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
							NextCursor: "cursor",
						},
					}, nil
				}
				return &notes.Result{
					Notes: []*storagemodels.Status{s1, s2},
					Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
						NextCursor: "cursor",
					},
				}, nil
			},
		},
	}

	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, reg)

	t.Run("filters by tagged and excludes reblogs", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, map[string]string{
			"exclude_reblogs": "true",
			"tagged":          "go",
			"limit":           "2",
		}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAccountStatusesLift(ctx))
		require.NotEmpty(t, resp.Headers["link"])

		var out []map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 1)
	})

	t.Run("only_media filters storage statuses out", func(t *testing.T) {
		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, map[string]string{
			"only_media": "true",
			"limit":      "2",
		}, nil)
		require.NoError(t, err)
		ctx.Params["id"] = "alice"

		resp := requireStatus(t, http.StatusOK)(h.HandleGetAccountStatusesLift(ctx))

		var out []map[string]any
		require.NoError(t, json.Unmarshal(resp.Body, &out))
		require.Len(t, out, 0)
	})

	t.Run("extractInReplyToViaReflection covers struct variants", func(t *testing.T) {
		type withString struct{ InReplyTo string }
		type withPtr struct{ InReplyTo *string }

		ptrValue := "https://example.com/objects/1"
		require.Equal(t, "x", h.extractInReplyToViaReflection(withString{InReplyTo: "x"}))
		require.Equal(t, ptrValue, h.extractInReplyToViaReflection(&withPtr{InReplyTo: &ptrValue}))
		require.Equal(t, "", h.extractInReplyToViaReflection(7))
	})

	t.Run("objectIsReply and objectIsReblog cover map and activity types", func(t *testing.T) {
		require.True(t, h.objectIsReply(map[string]any{"inReplyTo": "https://example.com/objects/1"}))
		require.False(t, h.objectIsReply(map[string]any{}))

		require.True(t, h.objectIsReblog(&activitypub.Activity{BaseObject: activitypub.BaseObject{Type: activitypub.AnnounceType}}))
		require.True(t, h.objectIsReblog(map[string]any{"type": "Announce"}))
		require.True(t, h.objectIsReblog(map[string]any{"reblog_of_id": "s1"}))
	})
}

func TestStatusesRound13_ShouldFilterObject_TaggedValidation(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	params := accountStatusesParams{tagged: ""}
	// When tagged is empty, objectHasHashtags returns true and the object should not be filtered.
	require.False(t, h.shouldFilterObject(&storagemodels.Status{StatusID: "s1"}, params))
}

func TestStatusesRound13_GetAccountStatuses_AccountIDURLBranch(t *testing.T) {
	cfg := round10TestConfig()
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		NotesSvc: &NotesServiceStub{
			ListNotesFunc: func(context.Context, *notes.ListNotesQuery) (*notes.Result, error) {
				return &notes.Result{Notes: []*storagemodels.Status{}}, nil
			},
		},
	})

	userToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
	headers := map[string]string{"Authorization": "Bearer " + userToken}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/https://example.com/users/alice/statuses", headers, nil, nil)
	require.NoError(t, err)
	ctx.Params["id"] = cfg.ActorURL("alice")

	requireStatus(t, http.StatusOK)(h.HandleGetAccountStatusesLift(ctx))
}
