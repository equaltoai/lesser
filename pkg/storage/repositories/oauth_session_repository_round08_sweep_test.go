package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestRound08_OAuthSessionRepository_CoverageSweep(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("create/update paths", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		session := &models.OAuthAuthSession{
			SessionID:   "sid",
			State:       "state",
			ClientID:    "cid",
			RedirectURI: "https://example.com/cb",
			CSRFToken:   "csrf",
			Username:    "user-1",
			FlowStep:    "initiated",
			CreatedAt:   baseTime,
			UpdatedAt:   baseTime,
			LastUsedAt:  baseTime,
			ExpiresAt:   baseTime.Add(30 * time.Minute).Unix(),
		}
		_ = session.UpdateKeys()

		require.NoError(t, repo.CreateOAuthSession(ctx, session))
		require.NoError(t, repo.UpdateOAuthSession(ctx, session))
		require.Error(t, repo.UpdateOAuthSession(ctx, nil))
	})

	t.Run("get paths", func(t *testing.T) {
		t.Run("generic get error is mapped", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetOAuthSession(ctx, "sid")
			require.Error(t, err)
		})

		t.Run("GetOAuthSessionByState success", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.OAuthAuthSession)
				*out = append(*out, models.OAuthAuthSession{
					SessionID:   "sid",
					ClientID:    "cid",
					RedirectURI: "https://example.com/cb",
					CSRFToken:   "csrf",
					Username:    "user-1",
					FlowStep:    "initiated",
					CreatedAt:   baseTime,
					UpdatedAt:   baseTime,
					ExpiresAt:   baseTime.Add(10 * time.Minute).Unix(),
				})
			}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			out, err := repo.GetOAuthSessionByState(ctx, "state")
			require.NoError(t, err)
			require.Equal(t, "sid", out.SessionID)
		})

		t.Run("GetUserOAuthSessions without limit and query error", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("All", mock.Anything).Return(errors.New("query failed")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetUserOAuthSessions(ctx, "user-1", 0)
			require.Error(t, err)
		})
	})

	t.Run("delete paths", func(t *testing.T) {
		t.Run("not found is ignored", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("Delete", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			require.NoError(t, repo.DeleteOAuthSession(ctx, "sid"))
		})

		t.Run("other delete error is returned", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("Delete", mock.Anything).Return(errors.New("delete failed")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			require.Error(t, repo.DeleteOAuthSession(ctx, "sid"))
		})
	})
}
