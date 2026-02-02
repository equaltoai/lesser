package handlers

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/equaltoai/lesser/pkg/services/conversations"
	"github.com/stretchr/testify/require"
)

func TestConversationHandlers_DeleteConversation_ErrorBranches_Round12(t *testing.T) {
	cfg := round10TestConfig()
	handler, _, _ := round11NewHandler(t, cfg, nil)

	handler.registry = &RegistryStub{
		ConversationsSvc: &ConversationsServiceStub{
			DeleteConversationFunc: func(_ context.Context, cmd *conversations.DeleteConversationCommand) (*conversations.ConversationResult, error) {
				switch cmd.ConversationID {
				case "conv-not-found":
					return nil, fmt.Errorf("not found")
				case "conv-not-participant":
					return nil, fmt.Errorf("not a participant")
				case "conv-error":
					return nil, fmt.Errorf("boom")
				default:
					return &conversations.ConversationResult{}, nil
				}
			},
		},
	}

	writeToken := round11SignAccessToken(t, cfg.JWTSecret, "alice", []string{"write"})
	writeHeaders := map[string]string{"Authorization": "Bearer " + writeToken}

	t.Run("missing conversation id returns 400", func(t *testing.T) {
		ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/conversations/", writeHeaders, nil, nil)
		require.NoError(t, err)

		requireStatus(t, http.StatusBadRequest)(handler.HandleDeleteConversationLift(ctxDelete))
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/conversations/conv-not-found", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDelete.Params["id"] = "conv-not-found"

		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteConversationLift(ctxDelete))
	})

	t.Run("not a participant returns 404", func(t *testing.T) {
		ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/conversations/conv-not-participant", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDelete.Params["id"] = "conv-not-participant"

		requireStatus(t, http.StatusNotFound)(handler.HandleDeleteConversationLift(ctxDelete))
	})

	t.Run("other error returns 500", func(t *testing.T) {
		ctxDelete, err := round10NewLiftContext(http.MethodDelete, "/api/v1/conversations/conv-error", writeHeaders, nil, nil)
		require.NoError(t, err)
		ctxDelete.Params["id"] = "conv-error"

		requireStatus(t, http.StatusInternalServerError)(handler.HandleDeleteConversationLift(ctxDelete))
	})
}
