package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/auth"
	commonerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/notes"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestStatusesRound35_HandleCreateStatusLift_AcceptsCanonicalRemoteReplyParentURL(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now().UTC()
	replyParentURL := "https://remote.example/users/steward/statuses/seed-1"

	var gotCmd *notes.CreateNoteCommand
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(_ context.Context, cmd *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				gotCmd = cmd
				return &notes.NoteResult{
					Note: &storagemodels.Status{
						StatusID:       "status-1",
						AuthorUsername: cmd.AuthorID,
						AuthorID:       cfg.BaseURL() + "/users/" + cmd.AuthorID,
						Visibility:     cmd.Visibility,
						Sensitive:      cmd.Sensitive,
						Language:       cmd.Language,
						InReplyToID:    cmd.InReplyToID,
						PublishedAt:    now,
						CreatedAt:      now,
						UpdatedAt:      now,
						ModifiedAt:     now,
						Version:        1,
						Note: &activitypub.Note{
							BaseObject: activitypub.BaseObject{
								ID: cfg.BaseURL() + "/objects/status-1",
							},
							Content:      cmd.Content,
							AttributedTo: cfg.BaseURL() + "/users/" + cmd.AuthorID,
						},
					},
				}, nil
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{
		Status:      "reply",
		Visibility:  VisibilityPublic,
		InReplyToID: replyParentURL,
	})
	require.NoError(t, err)

	requireStatus(t, http.StatusCreated)(h.HandleCreateStatusLift(ctx))
	require.NotNil(t, gotCmd)
	require.Equal(t, replyParentURL, gotCmd.InReplyToID)
}

func TestStatusesRound35_HandleCreateStatusLift_RejectsInvalidRemoteReplyParentURL(t *testing.T) {
	cfg := round10TestConfig()
	called := false
	h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
		NotesSvc: &NotesServiceStub{
			CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
				called = true
				return nil, nil
			},
		},
	})

	token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
	headers := map[string]string{"Authorization": "Bearer " + token}

	ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{
		Status:      "reply",
		Visibility:  VisibilityPublic,
		InReplyToID: "ftp://remote.example/statuses/seed-1",
	})
	require.NoError(t, err)

	resp := requireStatus(t, http.StatusBadRequest)(h.HandleCreateStatusLift(ctx))
	var body map[string]any
	require.NoError(t, json.Unmarshal(resp.Body, &body))
	require.Equal(t, "BAD_REQUEST", body["error_code"])
	require.False(t, called)
}

func TestStatusesRound35_HandleCreateStatusLift_MapsReplyParentAppErrors(t *testing.T) {
	cfg := round10TestConfig()
	replyParentURL := "https://remote.example/users/steward/statuses/seed-1"

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "timeout surfaces 408",
			err:        commonerrors.NewAppError(commonerrors.CodeTimeout, commonerrors.CategoryExternal, "Remote reply parent fetch timed out"),
			wantStatus: http.StatusRequestTimeout,
			wantCode:   string(commonerrors.CodeTimeout),
		},
		{
			name:       "service unavailable surfaces 503",
			err:        commonerrors.NewAppError(commonerrors.CodeExternalServiceUnavailable, commonerrors.CategoryExternal, "Remote reply parent is temporarily unavailable"),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   string(commonerrors.CodeExternalServiceUnavailable),
		},
		{
			name:       "unprocessable parent surfaces 422",
			err:        commonerrors.NewAppError(commonerrors.CodeUnprocessableEntity, commonerrors.CategoryValidation, "Remote reply parent is not usable"),
			wantStatus: http.StatusUnprocessableEntity,
			wantCode:   string(commonerrors.CodeUnprocessableEntity),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, _ := round11NewHandler(t, cfg, &round10QueryState{}, &RegistryStub{
				NotesSvc: &NotesServiceStub{
					CreateNoteFunc: func(context.Context, *notes.CreateNoteCommand) (*notes.NoteResult, error) {
						return nil, tt.err
					},
				},
			})

			token := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{auth.ScopeRead, auth.ScopeWrite})
			headers := map[string]string{"Authorization": "Bearer " + token}

			ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/statuses", headers, nil, apimodels.CreateStatusRequest{
				Status:      "reply",
				Visibility:  VisibilityPublic,
				InReplyToID: replyParentURL,
			})
			require.NoError(t, err)

			resp := requireStatus(t, tt.wantStatus)(h.HandleCreateStatusLift(ctx))
			var body map[string]any
			require.NoError(t, json.Unmarshal(resp.Body, &body))
			require.Equal(t, tt.wantCode, body["error_code"])
		})
	}
}
