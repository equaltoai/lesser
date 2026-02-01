package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storage "github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusHandlersLift(t *testing.T) {
	cfg := round11TestConfig()
	now := time.Now()

	status := &storagemodels.Status{
		StatusID:       "s1",
		AuthorUsername: "alice",
		AuthorID:       cfg.ActorURL("alice"),
		Content:        "hello",
		PublishedAt:    now.Add(-1 * time.Hour),
		CreatedAt:      now.Add(-1 * time.Hour),
		UpdatedAt:      now.Add(-30 * time.Minute),
		Note: &storagemodels.NoteField{Note: &activitypub.Note{
			BaseObject: activitypub.BaseObject{
				ID:   cfg.BaseURL() + "/objects/s1",
				Type: activitypub.NoteType,
				To:   []string{activitypub.PublicAddress},
			},
			AttributedTo: cfg.ActorURL("alice"),
			Content:      "hello",
		}},
	}

	state := &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"alice": {PK: "USER#alice", SK: storagemodels.SKMetadata, Username: "alice", Role: "user", Approved: true, Version: 1, CreatedAt: now.Add(-48 * time.Hour)},
		},
		actorsByUser: map[string]storagemodels.Actor{
			"alice": {
				Username: "alice",
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   cfg.ActorURL("alice"),
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
				CreatedAt: now.Add(-24 * time.Hour),
				UpdatedAt: now.Add(-1 * time.Hour),
			},
		},
		statusByID: map[string]storagemodels.Status{
			"s1":     *status,
			"parent": {StatusID: "parent", AuthorUsername: "alice", AuthorID: cfg.ActorURL("alice"), PublishedAt: now.Add(-2 * time.Hour), CreatedAt: now.Add(-2 * time.Hour)},
		},
		objectsByID: map[string]storagemodels.Object{
			cfg.BaseURL() + "/objects/s1": {
				ID:           cfg.BaseURL() + "/objects/s1",
				Type:         activitypub.NoteType,
				Content:      "hello",
				AttributedTo: cfg.ActorURL("alice"),
				Published:    now.Add(-1 * time.Hour),
				Updated:      now.Add(-30 * time.Minute),
				To:           []string{activitypub.PublicAddress},
			},
		},
	}

	notesStub := &NotesServiceStub{
		CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
			require.Equal(t, "alice", cmd.AuthorID)
			return &notes.NoteResult{Note: status}, nil
		},
		GetNoteFunc: func(_ context.Context, statusID string) (*storagemodels.Status, error) {
			if statusID == "missing" {
				return nil, errors.New("not found")
			}
			return status, nil
		},
		DeleteNoteFunc: func(_ context.Context, cmd *notes.DeleteNoteCommand) error {
			require.Equal(t, "s1", cmd.StatusID)
			return nil
		},
		ListNotesFunc: func(_ context.Context, query *notes.ListNotesQuery) (*notes.Result, error) {
			return &notes.Result{
				Notes: []*storagemodels.Status{status},
				Pagination: &interfaces.PaginatedResult[*storagemodels.Status]{
					NextCursor: "next",
				},
			}, nil
		},
		GetUserTimelineFunc: func(_ context.Context, _ string, _ interfaces.PaginationOptions) (*notes.GetUserTimelineResult, error) {
			return &notes.GetUserTimelineResult{
				Items:      []*storagemodels.Status{status},
				NextCursor: "next",
			}, nil
		},
	}

	accountsStub := &AccountsServiceStub{
		GetAccountFunc: func(_ context.Context, _ string) (*storage.Account, error) {
			return &storage.Account{
				User: &storage.User{Username: "alice", DisplayName: "Alice", CreatedAt: now.Add(-48 * time.Hour)},
				Actor: &activitypub.Actor{
					BaseObject: activitypub.BaseObject{
						ID:   cfg.ActorURL("alice"),
						Type: "Person",
					},
					PreferredUsername: "alice",
					Name:              "Alice",
				},
			}, nil
		},
	}

	registry := &RegistryStub{
		NotesSvc:    notesStub,
		AccountsSvc: accountsStub,
	}

	handler, _, _ := round11NewHandler(t, cfg, state, registry)
	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{Status: "hello", Visibility: "public"})
	require.NoError(t, err)
	requireStatus(t, http.StatusCreated)(handler.HandleCreateStatusLift(ctxCreate))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctxDelete.Params["id"] = "s1"
	requireStatus(t, http.StatusOK)(handler.HandleDeleteStatusLift(ctxDelete))

	ctxGet, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1", headers, nil, nil)
	require.NoError(t, err)
	ctxGet.Params["id"] = "s1"
	requireStatus(t, http.StatusOK)(handler.HandleGetStatusLift(ctxGet))

	ctxHome, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/home", headers, map[string]string{"limit": "5"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetHomeTimelineLift(ctxHome))

	ctxPublic, err := round10NewLiftContext(http.MethodGet, "/api/v1/timelines/public", nil, map[string]string{"local": "true"}, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetPublicTimelineLift(ctxPublic))

	ctxAccount, err := round10NewLiftContext(http.MethodGet, "/api/v1/accounts/alice/statuses", nil, map[string]string{"only_media": "true"}, nil)
	require.NoError(t, err)
	ctxAccount.Params["id"] = "alice"
	requireStatus(t, http.StatusOK)(handler.HandleGetAccountStatusesLift(ctxAccount))

	ctxContext, err := round10NewLiftContext(http.MethodGet, "/api/v1/statuses/s1/context", nil, nil, nil)
	require.NoError(t, err)
	ctxContext.Params["id"] = "s1"
	requireStatus(t, http.StatusOK)(handler.HandleGetStatusContextLift(ctxContext))
}

func TestStatusHelpers(t *testing.T) {
	cfg := round11TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

	note := &activitypub.Note{
		AttributedTo: "https://example.com/users/alice",
		Content:      "hello",
		Attachment:   []activitypub.Attachment{{URL: "https://example.com/media/1"}},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
	require.NoError(t, err)

	req := &apimodels.UpdateStatusRequest{Status: "updated", SpoilerText: "warn", Sensitive: true}
	handler.applyStatusUpdates(note, req)
	require.Equal(t, "updated", note.Content)
	require.Equal(t, "warn", note.Summary)
	require.True(t, note.Sensitive)

	owned, resp, err := handler.convertObjectToNoteWithOwnershipCheck(ctx, note, "https://example.com/users/alice")
	require.NoError(t, err)
	require.Nil(t, resp)
	require.Equal(t, "updated", owned.Content)

	ctxForbidden, err := round10NewLiftContext(http.MethodGet, "/status", nil, nil, nil)
	require.NoError(t, err)
	_, respForbidden, err := handler.convertObjectToNoteWithOwnershipCheck(ctxForbidden, note, "https://example.com/users/bob")
	require.NoError(t, err)
	require.NotNil(t, respForbidden)
	require.Equal(t, http.StatusForbidden, respForbidden.Status)

	require.True(t, handler.objectHasMedia(note))
	require.False(t, handler.objectIsReply(note))
	require.False(t, handler.objectIsReblog(note))

	tagged := handler.objectHasHashtags(map[string]any{
		"hashtags": []string{"Go", "AI"},
	}, "go")
	require.True(t, tagged)

	required := handler.parseRequiredTags("Go,AI")
	require.Equal(t, []string{"go", "ai"}, required)
	require.True(t, handler.containsAllRequiredTags([]string{"go", "ai"}, required))

	link := handler.buildPaginationURL("alice", "cursor", accountStatusesParams{
		limit:          5,
		onlyMedia:      true,
		excludeReplies: true,
		tagged:         "go",
	})
	require.Contains(t, link, "only_media=true")
	require.Contains(t, link, "exclude_replies=true")
	require.Contains(t, link, "tagged=go")
}
