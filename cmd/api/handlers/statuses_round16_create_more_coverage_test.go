package handlers

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesRound16_HandleCreateStatusLift(t *testing.T) {
	cfg := round10TestConfig()

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})
		ctx := round10NewLiftContextWithBodyBytes(http.MethodPost, "/api/v1/statuses", nil, nil, []byte("{bad"))
		requireStatus(t, http.StatusBadRequest)(h.HandleCreateStatusLift(ctx))
	})

	t.Run("insufficient scope returns 403", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
			Status: "hello",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusForbidden)(h.HandleCreateStatusLift(ctx))
	})

	t.Run("service error returns 500", func(t *testing.T) {
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					return nil, errors.New("boom")
				},
			},
		})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
			Status: "hello",
		})
		require.NoError(t, err)

		requireStatus(t, http.StatusInternalServerError)(h.HandleCreateStatusLift(ctx))
	})

	t.Run("success defaults visibility and returns 201", func(t *testing.T) {
		now := time.Now().UTC()

		var gotCmd *notes.CreateNoteCommand
		h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
			NotesSvc: &NotesServiceStub{
				CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
					gotCmd = cmd
					return &notes.NoteResult{
						Note: &storagemodels.Status{
							StatusID:       "s1",
							AuthorUsername: cmd.AuthorID,
							AuthorID:       cfg.BaseURL() + "/users/" + cmd.AuthorID,
							Visibility:     cmd.Visibility,
							Sensitive:      cmd.Sensitive,
							Language:       cmd.Language,
							InReplyToID:    cmd.InReplyToID,
							Hashtags:       []string{"Go"},
							Mentions:       []string{"bob"},
							PublishedAt:    now,
							CreatedAt:      now,
							UpdatedAt:      now,
							ModifiedAt:     now,
							Version:        1,
							Note: &activitypub.Note{
								BaseObject: activitypub.BaseObject{ID: cfg.BaseURL() + "/objects/s1"},
								Content:    cmd.Content,
								AttributedTo: cfg.BaseURL() + "/users/" + cmd.AuthorID,
							},
						},
					}, nil
				},
			},
		})

		token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
		headers := map[string]string{"Authorization": "Bearer " + token}

		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, models.CreateStatusRequest{
			Status:     "hello",
			Visibility: "",
			MediaIDs:   []string{"m1", "m2"},
			ScheduledAt: func() *string { s := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339); return &s }(),
		})
		require.NoError(t, err)

		resp := requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(ctx))
		require.NotEmpty(t, resp.Body)

		require.NotNil(t, gotCmd)
		require.Equal(t, "alice", gotCmd.AuthorID)
		require.Equal(t, VisibilityPublic, gotCmd.Visibility)
	})
}

func TestStatusesRound16_ObjectHasMediaBranches(t *testing.T) {
	h := &Handler{}

	require.True(t, h.objectHasMedia(&activitypub.Note{Attachment: []activitypub.Attachment{{URL: "https://example.com/a"}}}))
	require.True(t, h.objectHasMedia(map[string]any{"attachment": []any{"x"}}))
	require.False(t, h.objectHasMedia(map[string]any{}))
	require.False(t, h.objectHasMedia(nil))
}

func TestStatusesRound16_ObjectHasHashtagsAndExtractGenericMap(t *testing.T) {
	h := &Handler{}

	// Empty filter means "no filter", so objects match.
	require.True(t, h.objectHasHashtags(map[string]any{}, ""))

	require.True(t, h.objectHasHashtags(map[string]any{"hashtags": []string{"Go"}}, "go"))
	require.False(t, h.objectHasHashtags(map[string]any{"hashtags": []string{"Go"}}, "rust"))

	tags := h.extractFromGenericMap(map[string]any{
		"tag": []any{
			map[string]any{"type": "Hashtag", "name": "#Go"},
			map[string]any{"type": "Mention", "name": "@alice"},
		},
	})
	require.Equal(t, []string{"go"}, tags)
}
