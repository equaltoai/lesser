package repositories

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v2/pkg/session"
	"github.com/theory-cloud/tabletheory/v2/pkg/testing/fakedb"
	"go.uber.org/zap"
)

type draftReviewRecordingDynamo struct {
	*fakedb.Fake
	putInputs    []*dynamodb.PutItemInput
	updateInputs []*dynamodb.UpdateItemInput
}

func (d *draftReviewRecordingDynamo) PutItem(ctx context.Context, input *dynamodb.PutItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	d.putInputs = append(d.putInputs, input)
	return d.Fake.PutItem(ctx, input, opts...)
}

func (d *draftReviewRecordingDynamo) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.updateInputs = append(d.updateInputs, input)
	return d.Fake.UpdateItem(ctx, input, opts...)
}

func TestDraftRepositoryCreateDraftReviewGrantUsesCreateBuilder(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DraftReviewGrant{}))

	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC(),
	}
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.CreateDraftReviewGrant(ctx, grant))

	require.Len(t, client.putInputs, 1, "first-time grants must use TableTheory's create path")
	require.Empty(t, client.updateInputs, "first-time grants must not use the version-conditioned update path")
	condition := strings.ToLower(aws.ToString(client.putInputs[0].ConditionExpression))
	require.NotContains(t, condition, "version")
	require.NotContains(t, condition, "=", "a create must not require a version value on a nonexistent item")
}

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

func TestDraftRepositoryRegrantDraftReviewGrantClearsRevocation(t *testing.T) {
	ctx := context.Background()
	grantedAt := time.Now().UTC()
	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: grantedAt,
		Version:   4,
	}
	require.NoError(t, grant.UpdateKeys())

	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", grant).Return(query).Once()
	query.On("Where", "PK", "=", "USER#owner#DRAFT#REVIEW").Return(query).Once()
	query.On("Where", "SK", "=", "GRANT#draft-1#REVIEWER#reviewer").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "GrantedAt", grantedAt).Return(update).Once()
	update.On("Set", "GSI2PK", "DRAFT#REVIEWER#reviewer").Return(update).Once()
	update.On("Set", "GSI2SK", grant.GSI2SK).Return(update).Once()
	update.On("Remove", "RevokedAt").Return(update).Once()
	update.On("ConditionVersion", int64(4)).Return(update).Once()
	update.On("Set", "Version", 5).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.RegrantDraftReviewGrant(ctx, grant))
	require.Equal(t, 5, grant.Version)
	require.Nil(t, grant.RevokedAt)
	require.NotEmpty(t, grant.GSI2PK)
	require.NotEmpty(t, grant.GSI2SK)

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
