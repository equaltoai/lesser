package lift

import (
	"context"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/services/emoji"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/require"
)

func TestCustomEmojisHandlers_Round11(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{})

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
	require.NoError(t, handler.HandleGetCustomEmojisLift(ctx))
	require.Equal(t, http.StatusOK, ctx.Response.StatusCode)

	createReq := models.CreateCustomEmojiRequest{Shortcode: "wave", URL: "https://example.com/wave.png", Category: "misc"}
	ctxCreate, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/custom_emojis", adminHeaders, nil, createReq)
	require.NoError(t, err)
	require.NoError(t, handler.HandleCreateCustomEmojiLift(ctxCreate))
	require.Equal(t, http.StatusOK, ctxCreate.Response.StatusCode)

	category := "updated"
	visible := true
	disabled := false
	updateReq := models.UpdateCustomEmojiRequest{Category: &category, VisibleInPicker: &visible, Disabled: &disabled}
	ctxUpdate, err := round10NewLiftContext(http.MethodPut, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, updateReq)
	require.NoError(t, err)
	ctxUpdate.SetParam("shortcode", "wave")
	require.NoError(t, handler.HandleUpdateCustomEmojiLift(ctxUpdate))
	require.Equal(t, http.StatusOK, ctxUpdate.Response.StatusCode)

	ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/admin/custom_emojis/wave", adminHeaders, nil, nil)
	require.NoError(t, err)
	ctxDelete.SetParam("shortcode", "wave")
	require.NoError(t, handler.HandleDeleteCustomEmojiLift(ctxDelete))
	require.Equal(t, http.StatusOK, ctxDelete.Response.StatusCode)
}
