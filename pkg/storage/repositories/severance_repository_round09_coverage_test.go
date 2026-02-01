package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormErrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestSeveranceRepository_Round09_Coverage(t *testing.T) {
	ctx := context.Background()
	baseTime := time.Date(2025, 12, 28, 1, 2, 3, 0, time.UTC)

	t.Run("Create/Get/List/Update severed relationships", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		// GetSeveredRelationship uses Scan(&severances).
		mockQuery.
			On("Scan", mockMatchedByType[*[]*models.SeveredRelationship]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.SeveredRelationship)
				s := models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)
				s.DetectedAt = baseTime
				s.ID = "local_remote_1"
				_ = s.UpdateKeys()
				*out = append(*out, s)
			}).
			Return(nil).
			Maybe()

		mockQuery.
			On("All", mockMatchedByType[*[]*models.SeveredRelationship]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.SeveredRelationship)
				a1 := models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)
				a1.DetectedAt = baseTime
				a1.ID = "local_remote_1"
				a1.Reason = models.SeveranceReasonOther
				_ = a1.UpdateKeys()

				a2 := models.NewSeveredRelationship("local", "other", models.SeveranceReasonDomainBlock)
				a2.DetectedAt = baseTime.Add(time.Minute)
				a2.ID = "local_other_2"
				a2.Reason = models.SeveranceReasonDomainBlock
				_ = a2.UpdateKeys()

				a3 := models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)
				a3.DetectedAt = baseTime.Add(2 * time.Minute)
				a3.ID = "local_remote_3"
				a3.Reason = models.SeveranceReasonOther
				_ = a3.UpdateKeys()

				*out = append(*out, a1, a2, a3)
			}).
			Return(nil).
			Maybe()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())

		require.Error(t, repo.CreateSeveredRelationship(ctx, nil))
		require.NoError(t, repo.CreateSeveredRelationship(ctx, models.NewSeveredRelationship("local", "remote", models.SeveranceReasonOther)))

		// Invalid ID format.
		_, err := repo.GetSeveredRelationship(ctx, "bad")
		require.Error(t, err)

		// Found.
		got, err := repo.GetSeveredRelationship(ctx, "local_remote_1")
		require.NoError(t, err)
		require.NotNil(t, got)

		// Status filter path uses GSI1; instance filter uses PK/SK.
		items, next, err := repo.ListSeveredRelationships(ctx, "local", SeveranceFilters{Status: models.SeveranceStatusActive}, 2, "")
		require.NoError(t, err)
		require.Len(t, items, 2)
		require.NotEmpty(t, next)

		items, _, err = repo.ListSeveredRelationships(ctx, "local", SeveranceFilters{Instance: "remote", Reason: models.SeveranceReasonOther}, 2, "cursor")
		require.NoError(t, err)
		require.NotEmpty(t, items)

		// Default path (no filters) + reason in-memory filter.
		items, _, err = repo.ListSeveredRelationships(ctx, "local", SeveranceFilters{Reason: models.SeveranceReasonDomainBlock}, 2, "")
		require.NoError(t, err)
		require.Len(t, items, 1)

		// UpdateSeveranceStatus acknowledges when requested.
		require.NoError(t, repo.UpdateSeveranceStatus(ctx, "local_remote_1", models.SeveranceStatusAcknowledged))
	})

	t.Run("Pagination not found returns empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)
		mockQuery.On("All", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())

		items, next, err := repo.ListSeveredRelationships(ctx, "local", SeveranceFilters{}, 2, "")
		require.NoError(t, err)
		require.Empty(t, items)
		require.Empty(t, next)
	})

	t.Run("Affected relationships and reconnection attempts", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockQuery.
			On("All", mockMatchedByType[*[]*models.AffectedRelationship]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.AffectedRelationship)
				a1 := models.NewAffectedRelationship("sev", "actor-1", "@a1", "example.com", "follower", baseTime)
				_ = a1.UpdateKeys()
				a2 := models.NewAffectedRelationship("sev", "actor-2", "@a2", "example.com", "following", baseTime)
				_ = a2.UpdateKeys()
				*out = append(*out, a1, a2)
			}).
			Return(nil).
			Maybe()

		mockQuery.
			On("Scan", mockMatchedByType[*[]*models.SeveranceReconnectionAttempt]()).
			Run(func(args mock.Arguments) {
				out := args.Get(0).(*[]*models.SeveranceReconnectionAttempt)
				*out = append(*out, models.NewSeveranceReconnectionAttempt("sev", "admin"))
			}).
			Return(nil).
			Maybe()

		mockQuery.
			On("First", mockMatchedByType[*models.SeveranceReconnectionAttempt]()).
			Run(func(args mock.Arguments) {
				a := args.Get(0).(*models.SeveranceReconnectionAttempt)
				a.SeveranceID = "sev"
				a.ID = "attempt-1"
				a.Status = "pending"
				_ = a.UpdateKeys()
			}).
			Return(nil).
			Once()

		setupPermissiveRound08Mocks(mockDB, mockQuery, nil, baseTime)
		repo := NewSeveranceRepository(mockDB, "test-table", zap.NewNop())

		require.Error(t, repo.CreateAffectedRelationship(ctx, nil))
		require.NoError(t, repo.CreateAffectedRelationship(ctx, models.NewAffectedRelationship("sev", "actor-1", "@a1", "example.com", "follower", baseTime)))

		affected, next, err := repo.GetAffectedRelationships(ctx, "sev", 1, "")
		require.NoError(t, err)
		require.Len(t, affected, 1)
		require.NotEmpty(t, next)

		require.Error(t, repo.CreateReconnectionAttempt(ctx, nil))
		require.NoError(t, repo.CreateReconnectionAttempt(ctx, models.NewSeveranceReconnectionAttempt("sev", "admin")))
		require.Error(t, repo.UpdateReconnectionAttempt(ctx, nil))
		require.NoError(t, repo.UpdateReconnectionAttempt(ctx, models.NewSeveranceReconnectionAttempt("sev", "admin")))

		attempt, err := repo.GetReconnectionAttempt(ctx, "sev", "attempt-1")
		require.NoError(t, err)
		require.NotNil(t, attempt)

		attempts, err := repo.GetReconnectionAttempts(ctx, "sev")
		require.NoError(t, err)
		require.NotEmpty(t, attempts)

		mockDBNF := new(mocks.MockDB)
		mockQueryNF := new(mocks.MockQuery)
		mockQueryNF.On("First", mock.Anything).Return(dynamormErrors.ErrItemNotFound).Once()
		setupPermissiveRound08Mocks(mockDBNF, mockQueryNF, nil, baseTime)
		repoNF := NewSeveranceRepository(mockDBNF, "test-table", zap.NewNop())
		missing, err := repoNF.GetReconnectionAttempt(ctx, "sev", "missing")
		require.Error(t, err)
		require.Nil(t, missing)
	})
}
