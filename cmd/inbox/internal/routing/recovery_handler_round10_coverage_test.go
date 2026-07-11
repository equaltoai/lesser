package routing

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	pkgerrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/federation"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestRecoveryHandler_Round10_Coverage(t *testing.T) {
	ctx := context.Background()

	t.Run("HandleActivity ignores non-map objects", func(t *testing.T) {
		h := NewRecoveryActivityHandler(nil, nil, nil, zap.NewNop())
		require.NoError(t, h.HandleActivity(ctx, &activitypub.Activity{Object: "not-a-map"}))
	})

	t.Run("HandleActivity trustee confirmation error branches", func(t *testing.T) {
		h := NewRecoveryActivityHandler(nil, nil, nil, zap.NewNop())

		activityMissingRequestID := &activitypub.Activity{
			Actor: "https://remote.example/users/trustee",
			Object: map[string]any{
				"lesser:recoveryConfirmation": map[string]any{},
			},
		}
		require.ErrorIs(t, h.HandleActivity(ctx, activityMissingRequestID), federation.ErrMissingRequestIDInConfirmation)

		activityMissingActor := &activitypub.Activity{
			Object: map[string]any{
				"lesser:recoveryConfirmation": map[string]any{"requestId": "r1"},
			},
		}
		require.ErrorIs(t, h.HandleActivity(ctx, activityMissingActor), federation.ErrMissingActorInConfirmation)
	})

	t.Run("HandleActivity trustee acceptance success and failure", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("WithContext", mock.Anything).Return(mockQuery).Maybe()
		mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()

		mockQuery.
			On("First", mock.AnythingOfType("*models.Trustee")).
			Run(func(args mock.Arguments) {
				tr := args.Get(0).(*models.Trustee)
				tr.Username = "alice"
				tr.ActorID = "https://remote.example/users/trustee"
				_ = tr.UpdateKeys()
			}).
			Return(nil).
			Maybe()

		repo := repositories.NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		activity := &activitypub.Activity{
			Actor: "https://remote.example/users/trustee",
			Object: map[string]any{
				"lesser:trusteeAcceptance": map[string]any{
					"inviterUsername": "alice",
				},
			},
		}

		t.Run("missing inviter username", func(t *testing.T) {
			h := NewRecoveryActivityHandler(nil, repo, nil, zap.NewNop())
			err := h.HandleActivity(ctx, &activitypub.Activity{
				Actor: "https://remote.example/users/trustee",
				Object: map[string]any{
					"lesser:trusteeAcceptance": map[string]any{},
				},
			})
			require.ErrorIs(t, err, federation.ErrMissingInviterUsername)
		})

		t.Run("missing actor", func(t *testing.T) {
			h := NewRecoveryActivityHandler(nil, repo, nil, zap.NewNop())
			err := h.HandleActivity(ctx, &activitypub.Activity{
				Object: map[string]any{
					"lesser:trusteeAcceptance": map[string]any{
						"inviterUsername": "alice",
					},
				},
			})
			require.ErrorIs(t, err, federation.ErrMissingActorInAcceptance)
		})

		t.Run("update error", func(t *testing.T) {
			mockQuery.On("Update", mock.Anything).Return(errors.New("update failed")).Once()
			h := NewRecoveryActivityHandler(nil, repo, nil, zap.NewNop())
			err := h.HandleActivity(ctx, activity)
			require.Error(t, err)
			var appErr *pkgerrors.AppError
			require.True(t, errors.As(err, &appErr))
		})

		t.Run("success", func(t *testing.T) {
			mockQuery.On("Update", mock.Anything).Return(nil).Once()
			h := NewRecoveryActivityHandler(nil, repo, nil, zap.NewNop())
			require.NoError(t, h.HandleActivity(ctx, activity))
		})
	})

	t.Run("Activity constructors", func(t *testing.T) {
		confirmation := CreateRecoveryConfirmationActivity("req-1", "https://remote.example/users/trustee", "https://example.com/actor/system")
		require.NotNil(t, confirmation)
		require.Equal(t, "Create", confirmation.Type)
		require.NotNil(t, confirmation.Published)

		acceptance := CreateTrusteeAcceptanceActivity("alice", "https://remote.example/users/trustee", "https://example.com/users/alice")
		require.NotNil(t, acceptance)
		require.Equal(t, "Accept", acceptance.Type)
		require.NotNil(t, acceptance.Published)
	})
}
