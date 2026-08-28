package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormErrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestRecoveryRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("Trustee CRUD branches", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("AllPaginated", mockMatchedByType[*[]models.Trustee]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.Trustee)
				*out = append(*out, models.Trustee{Username: "user-1", ActorID: "actor-1", AddedAt: baseTime, Confirmed: true})
			}).
			Return(&core.PaginatedResult{HasMore: false}, nil).
			Once()

		mockQuery.
			On("First", mockMatchedByType[*models.Trustee]()).
			Run(func(args mock.Arguments) {
				tr := args.Get(0).(*models.Trustee)
				tr.Username = "user-1"
				tr.ActorID = "actor-1"
				tr.AddedAt = baseTime
				tr.Confirmed = false
				_ = tr.UpdateKeys()
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)

		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.NoError(t, repo.StoreTrustee(ctx, "user-1", &storage.TrusteeConfig{ActorID: "actor-1", AddedAt: baseTime, Confirmed: false}))
		trustees, err := repo.GetTrustees(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, trustees, 1)

		require.NoError(t, repo.UpdateTrusteeConfirmed(ctx, "user-1", "actor-1", true))
		require.NoError(t, repo.DeleteTrustee(ctx, "user-1", "actor-1"))

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("AllPaginated", mock.Anything).Return(nil, dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewRecoveryRepository(mockDBNF, "test-table", zap.NewNop(), nil)
		empty, err := repoNF.GetTrustees(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, empty)
	})

	t.Run("Recovery requests and active filtering", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("First", mockMatchedByType[*models.RecoveryRequest]()).
			Run(func(args mock.Arguments) {
				req := args.Get(0).(*models.RecoveryRequest)
				req.ID = "r1"
				req.Username = "user-1"
				req.InitiatedAt = baseTime
				req.ExpiresAt = baseTime.Add(time.Hour)
				req.RequiredVotes = 2
				req.ReceivedVotes = map[string]bool{"t1": true, "t2": false}
				req.Status = models.StatusPending
				req.RecoveryToken = "tok"
				_ = req.UpdateKeys()
			}).
			Return(nil).
			Once()

		mockQuery.
			On("AllPaginated", mockMatchedByType[*[]models.RecoveryRequest]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.RecoveryRequest)
				active := models.RecoveryRequest{ID: "r1", Username: "user-1", InitiatedAt: baseTime, ExpiresAt: time.Now().Add(time.Hour), Status: models.StatusPending, ReceivedVotes: map[string]bool{"t1": true}}
				_ = active.UpdateKeys()
				expired := models.RecoveryRequest{ID: "r2", Username: "user-1", InitiatedAt: baseTime, ExpiresAt: time.Now().Add(-time.Hour), Status: models.StatusPending, ReceivedVotes: map[string]bool{"t1": true}}
				_ = expired.UpdateKeys()
				complete := models.RecoveryRequest{ID: "r3", Username: "user-1", InitiatedAt: baseTime, ExpiresAt: time.Now().Add(time.Hour), Status: models.StatusComplete, ReceivedVotes: map[string]bool{"t1": true}}
				_ = complete.UpdateKeys()
				*out = append(*out, active, expired, complete)
			}).
			Return(&core.PaginatedResult{HasMore: false}, nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.NoError(t, repo.StoreRecoveryRequest(ctx, &storage.SocialRecoveryRequest{
			ID:            "r1",
			Username:      "user-1",
			InitiatedAt:   baseTime,
			ExpiresAt:     baseTime.Add(time.Hour),
			RequiredVotes: 2,
			TrusteeVotes:  []string{"t1"},
			RecoveryToken: "tok",
			Status:        models.StatusPending,
		}))

		got, err := repo.GetRecoveryRequest(ctx, "r1")
		require.NoError(t, err)
		require.Equal(t, 1, got.ReceivedVotes)
		require.NotEmpty(t, got.TrusteeVotes)

		active, err := repo.GetActiveRecoveryRequests(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, active, 1)

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewRecoveryRepository(mockDBNF, "test-table", zap.NewNop(), nil)
		missing, err := repoNF.GetRecoveryRequest(ctx, "missing")
		require.Error(t, err)
		require.Nil(t, missing)
	})

	t.Run("Recovery codes and tokens", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("AllPaginated", mockMatchedByType[*[]models.RecoveryCode]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]models.RecoveryCode)
				c1 := models.RecoveryCode{Username: "user-1", Position: 1, CodeHash: "h1", CreatedAt: baseTime}
				_ = c1.UpdateKeys()
				c2 := models.RecoveryCode{Username: "user-1", Position: 2, CodeHash: "h2", CreatedAt: baseTime, UsedAt: &baseTime}
				_ = c2.UpdateKeys()
				*out = append(*out, c1, c2)
			}).
			Return(&core.PaginatedResult{HasMore: false}, nil).
			Maybe()

		mockQuery.
			On("First", mockMatchedByType[*models.RecoveryCode]()).
			Run(func(args mock.Arguments) {
				m := args.Get(0).(*models.RecoveryCode)
				m.Username = "user-1"
				m.Position = 1
				m.CodeHash = "h1"
				m.CreatedAt = baseTime
				_ = m.UpdateKeys()
			}).
			Return(nil).
			Once()

		mockQuery.
			On("First", mockMatchedByType[*models.RecoveryToken]()).
			Run(func(args mock.Arguments) {
				m := args.Get(0).(*models.RecoveryToken)
				m.PK = "key"
				m.SK = "TOKEN"
				m.Data = map[string]any{"x": "y"}
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewRecoveryRepository(mockDB, "test-table", zap.NewNop(), nil)

		require.NoError(t, repo.StoreRecoveryCode(ctx, "user-1", &storage.RecoveryCodeItem{CodeHash: "h1", CreatedAt: baseTime, Position: 1}))
		codes, err := repo.GetRecoveryCodes(ctx, "user-1")
		require.NoError(t, err)
		require.Len(t, codes, 2)

		require.NoError(t, repo.MarkRecoveryCodeUsed(ctx, "user-1", "h1"))
		unused, err := repo.CountUnusedRecoveryCodes(ctx, "user-1")
		require.NoError(t, err)
		require.Equal(t, 1, unused)

		require.NoError(t, repo.DeleteAllRecoveryCodes(ctx, "user-1"))

		require.NoError(t, repo.StoreRecoveryToken(ctx, "key", map[string]any{"x": "y"}))
		data, err := repo.GetRecoveryToken(ctx, "key")
		require.NoError(t, err)
		require.Equal(t, "y", data["x"])
		require.NoError(t, repo.DeleteRecoveryToken(ctx, "key"))

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("AllPaginated", mock.Anything).Return(nil, dynamormErrors.ErrItemNotFound).Once()
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewRecoveryRepository(mockDBNF, "test-table", zap.NewNop(), nil)
		empty, err := repoNF.GetRecoveryCodes(ctx, "user-1")
		require.NoError(t, err)
		require.Empty(t, empty)

		_, err = repoNF.GetRecoveryToken(ctx, "missing")
		require.Error(t, err)
	})
}
