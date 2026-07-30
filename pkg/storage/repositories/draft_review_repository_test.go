package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestDraftRepositoryRevokeDraftReviewGrantRemovesSparseIndexKeys(t *testing.T) {
	ctx := context.Background()
	revokedAt := time.Now().UTC()
	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: revokedAt.Add(-time.Hour),
		RevokedAt: &revokedAt,
		Version:   3,
	}

	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", grant).Return(query).Once()
	query.On("Where", "PK", "=", "USER#owner#DRAFT#REVIEW").Return(query).Once()
	query.On("Where", "SK", "=", "GRANT#draft-1#REVIEWER#reviewer").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "RevokedAt", revokedAt).Return(update).Once()
	update.On("Remove", "GSI2PK").Return(update).Once()
	update.On("Remove", "GSI2SK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "Version", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.RevokeDraftReviewGrant(ctx, grant))
	require.Equal(t, 4, grant.Version)
	require.Empty(t, grant.GSI2PK)
	require.Empty(t, grant.GSI2SK)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestDraftRepositoryListActiveDraftReviewGrantsRoundTripsCursor(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.DraftReviewGrant")).Return(query).Once()
	query.On("Index", "gsi2").Return(query).Once()
	query.On("Where", "gsi2PK", "=", "DRAFT#REVIEWER#reviewer").Return(query).Once()
	query.On("Filter", "RevokedAt", "attribute_not_exists", nil).Return(query).Once()
	query.On("OrderBy", "gsi2SK", "DESC").Return(query).Once()
	query.On("Where", "gsi2SK", "<", "cursor").Return(query).Once()
	query.On("Limit", 3).Return(query).Once()
	query.On("All", mock.Anything).Run(func(args mock.Arguments) {
		rows := args.Get(0).(*[]models.DraftReviewGrant)
		*rows = []models.DraftReviewGrant{
			{DraftID: "newer", GSI2SK: "TIME#3"},
			{DraftID: "middle", GSI2SK: "TIME#2"},
			{DraftID: "older", GSI2SK: "TIME#1"},
		}
	}).Return(nil).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	grants, next, err := repo.ListActiveDraftReviewGrants(ctx, "reviewer", 2, "cursor")
	require.NoError(t, err)
	require.Len(t, grants, 2)
	require.Equal(t, []string{"newer", "middle"}, []string{grants[0].DraftID, grants[1].DraftID})
	require.Equal(t, "TIME#2", next)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
}
