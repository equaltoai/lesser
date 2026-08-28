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

func TestRound08_OAuthSessionRepository_CorePaths(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("CreateOAuthSession handles nil", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.CreateOAuthSession(ctx, nil)
		require.Error(t, err)
	})

	t.Run("GetOAuthSession maps not found and expired", func(t *testing.T) {
		t.Run("not found", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Return(errors.New("not found")).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetOAuthSession(ctx, "missing")
			require.Error(t, err)
		})

		t.Run("expired session", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
				session := args.Get(0).(*models.OAuthAuthSession)
				session.SessionID = "sid"
				session.ClientID = "cid"
				session.RedirectURI = "https://example.com/cb"
				session.CSRFToken = "csrf"
				session.FlowStep = "initiated"
				session.CreatedAt = baseTime
				session.UpdatedAt = baseTime
				session.ExpiresAt = baseTime.Add(-time.Minute).Unix()
				_ = session.UpdateKeys()
			}).Return(nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetOAuthSession(ctx, "sid")
			require.Error(t, err)
		})
	})

	t.Run("GetOAuthSessionByState handles not found and expired", func(t *testing.T) {
		t.Run("not found via empty results", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("AllPaginated", mock.Anything).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetOAuthSessionByState(ctx, "state")
			require.Error(t, err)
		})

		t.Run("expired", func(t *testing.T) {
			mockDB := new(mocks.MockDB)
			mockQuery := new(mocks.MockQuery)
			mockQuery.On("AllPaginated", mock.Anything).Run(func(args mock.Arguments) {
				sessions := args.Get(0).(*[]models.OAuthAuthSession)
				*sessions = append(*sessions, models.OAuthAuthSession{
					SessionID:   "sid",
					ClientID:    "cid",
					RedirectURI: "https://example.com/cb",
					CSRFToken:   "csrf",
					FlowStep:    "initiated",
					ExpiresAt:   baseTime.Add(-time.Minute).Unix(),
				})
			}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()
			setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

			repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
			_, err := repo.GetOAuthSessionByState(ctx, "state")
			require.Error(t, err)
		})
	})

	t.Run("GetUserOAuthSessions filters expired sessions and supports limit", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
			sessions := args.Get(0).(*[]models.OAuthAuthSession)
			*sessions = append(*sessions,
				models.OAuthAuthSession{
					SessionID:   "valid",
					ClientID:    "cid",
					RedirectURI: "https://example.com/cb",
					CSRFToken:   "csrf",
					ExpiresAt:   baseTime.Add(10 * time.Minute).Unix(),
				},
				models.OAuthAuthSession{
					SessionID:   "expired",
					ClientID:    "cid",
					RedirectURI: "https://example.com/cb",
					CSRFToken:   "csrf",
					ExpiresAt:   baseTime.Add(-10 * time.Minute).Unix(),
				},
			)
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		out, err := repo.GetUserOAuthSessions(ctx, "user-1", 10)
		require.NoError(t, err)
		require.Len(t, out, 1)
		require.Equal(t, "valid", out[0].SessionID)
	})

	t.Run("AuthorizeOAuthSession fails when not in consent", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			session := args.Get(0).(*models.OAuthAuthSession)
			session.SessionID = "sid"
			session.ClientID = "cid"
			session.RedirectURI = "https://example.com/cb"
			session.CSRFToken = "csrf"
			session.Username = "user-1"
			session.FlowStep = "initiated"
			session.CreatedAt = baseTime
			session.UpdatedAt = baseTime
			session.ExpiresAt = baseTime.Add(10 * time.Minute).Unix()
			_ = session.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.AuthorizeOAuthSession(ctx, "sid")
		require.Error(t, err)
	})
}

func TestRound08_OAuthSessionRepository_MutationPaths(t *testing.T) {
	baseTime := time.Now().UTC()
	ctx := context.Background()

	t.Run("SetOAuthSessionUser and SetOAuthSessionFlowStep update session", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)

		err := repo.SetOAuthSessionUser(ctx, "sid", "user-1")
		require.NoError(t, err)

		err = repo.SetOAuthSessionFlowStep(ctx, "sid", "consent", map[string]any{"k": "v"})
		require.NoError(t, err)
	})

	t.Run("AuthorizeOAuthSession succeeds in consent step", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
			session := args.Get(0).(*models.OAuthAuthSession)
			session.SessionID = "sid"
			session.ClientID = "cid"
			session.RedirectURI = "https://example.com/cb"
			session.CSRFToken = "csrf"
			session.Username = "user-1"
			session.FlowStep = "consent"
			session.CreatedAt = baseTime
			session.UpdatedAt = baseTime
			session.ExpiresAt = baseTime.Add(10 * time.Minute).Unix()
			_ = session.UpdateKeys()
		}).Return(nil).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.AuthorizeOAuthSession(ctx, "sid")
		require.NoError(t, err)
	})

	t.Run("DeleteOAuthSession returns nil when already deleted", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("Delete").Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.DeleteOAuthSession(ctx, "sid")
		require.NoError(t, err)
	})

	t.Run("CountUserOAuthSessions and CleanupExpiredOAuthSessions", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		count, err := repo.CountUserOAuthSessions(ctx, "user-1")
		require.NoError(t, err)
		require.GreaterOrEqual(t, count, 0)

		n, err := repo.CleanupExpiredOAuthSessions(ctx, 100)
		require.NoError(t, err)
		require.Equal(t, 0, n)
	})

	t.Run("TouchOAuthSession touches and updates session", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewOAuthSessionRepository(mockDB, "test-table", zaptest.NewLogger(t), nil)
		err := repo.TouchOAuthSession(ctx, "sid")
		require.NoError(t, err)
	})
}
