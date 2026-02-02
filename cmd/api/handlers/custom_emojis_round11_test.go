package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestCustomEmojisHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	now := time.Now()
	adminUser := storagemodels.User{
		Username:  "admin",
		Role:      roleAdmin,
		Approved:  true,
		Version:   1,
		CreatedAt: now.Add(-24 * time.Hour),
		UpdatedAt: now.Add(-24 * time.Hour),
	}
	require.NoError(t, adminUser.UpdateKeys())

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"admin": adminUser,
		},
	})

	adminToken := round11SignAccessToken(t, cfg.JWTSecret, "admin", []string{"read", "write"})
	adminHeaders := map[string]string{"Authorization": "Bearer " + adminToken}

	handler.registry = &RegistryStub{
		EmojiSvc: &EmojiServiceStub{
			ListEmojisFunc: func(ctx context.Context, query *emoji.ListEmojisQuery) (*emoji.ListResult, error) {
				return &emoji.ListResult{
					Emojis: []*storage.CustomEmoji{
						{Shortcode: "party", URL: "https://example.com/party.png", StaticURL: "https://example.com/party.png", VisibleInPicker: true, Category: "fun"},
					},
				}, nil
			},
			CreateEmojiFunc: func(ctx context.Context, cmd *emoji.CreateEmojiCommand) (*emoji.Result, error) {
				return &emoji.Result{
					Emoji: &storage.CustomEmoji{
						Shortcode:       cmd.Shortcode,
						URL:             cmd.ImageURL,
						StaticURL:       cmd.ImageURL,
						VisibleInPicker: cmd.VisibleInPicker,
						Category:        cmd.Category,
					},
				}, nil
			},
			UpdateEmojiFunc: func(ctx context.Context, cmd *emoji.UpdateEmojiCommand) (*emoji.Result, error) {
				category := ""
				if cmd.Category != nil {
					category = *cmd.Category
				}
				visible := false
				if cmd.VisibleInPicker != nil {
					visible = *cmd.VisibleInPicker
				}
				return &emoji.Result{
					Emoji: &storage.CustomEmoji{
						Shortcode:       cmd.Shortcode,
						URL:             "https://example.com/updated.png",
						StaticURL:       "https://example.com/updated.png",
						VisibleInPicker: visible,
						Category:        category,
					},
				}, nil
			},
			DeleteEmojiFunc: func(ctx context.Context, cmd *emoji.DeleteEmojiCommand) error {
				return nil
			},
		},
		AccountsSvc: &AccountsServiceStub{
			GetAccountFunc: func(ctx context.Context, username string) (*storage.Account, error) {
				return &storage.Account{
					Actor: nil,
					User:  &storage.User{Username: username, Role: roleAdmin},
				}, nil
			},
		},
	}

	ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/custom_emojis", nil, nil, nil)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleGetCustomEmojisLift(ctx))

	createReq := models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png", Category: "misc"}
	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, createReq)
	require.NoError(t, err)
	requireStatus(t, http.StatusOK)(handler.HandleCreateCustomEmojiLift(ctxCreate))

	category := "updated"
	visible := true
	disabled := false
	updateReq := models.UpdateCustomEmojiRequest{Category: &category, VisibleInPicker: &visible, Disabled: &disabled}
	ctxUpdate, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, updateReq)
	require.NoError(t, err)
	ctxUpdate.Params["shortcode"] = "wave"
	requireStatus(t, http.StatusOK)(handler.HandleUpdateCustomEmojiLift(ctxUpdate))

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, nil)
	require.NoError(t, err)
	ctxDelete.Params["shortcode"] = "wave"
	requireStatus(t, http.StatusOK)(handler.HandleDeleteCustomEmojiLift(ctxDelete))
}
