package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
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

func TestDraftRepositoryUpdateDraftReviewFieldsDoesNotClobberOwnerContent(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	persisted := &models.Draft{
		AuthorID:      "owner",
		ID:            "draft-1",
		ContentType:   "Article",
		Title:         "owner title",
		Slug:          "owner-slug",
		Content:       "owner late edit",
		ContentFormat: "markdown",
		Status:        "draft",
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	staleReviewSnapshot := *persisted
	staleReviewSnapshot.Title = "stale title"
	staleReviewSnapshot.Slug = "stale-slug"
	staleReviewSnapshot.Content = "stale reviewed body"
	staleReviewSnapshot.ReviewedBy = "reviewer"
	staleReviewSnapshot.ReviewStatus = "APPROVED"
	staleReviewSnapshot.EditorNotes = "ready"

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateDraftReviewFields(ctx, "owner", &staleReviewSnapshot))

	got, err := repo.GetDraft(ctx, "owner", "draft-1")
	require.NoError(t, err)
	require.Equal(t, "owner title", got.Title)
	require.Equal(t, "owner-slug", got.Slug)
	require.Equal(t, "owner late edit", got.Content)
	require.Equal(t, "reviewer", got.ReviewedBy)
	require.Equal(t, "APPROVED", got.ReviewStatus)
	require.Equal(t, "ready", got.EditorNotes)
	require.Len(t, client.updateInputs, 1)
	require.Contains(t, aws.ToString(client.updateInputs[0].ConditionExpression), "attribute_exists")

	missing := staleReviewSnapshot
	missing.ID = "missing"
	require.ErrorIs(t, repo.UpdateDraftReviewFields(ctx, "owner", &missing), storage.ErrNotFound)
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
	require.Contains(t, aws.ToString(client.putInputs[0].ConditionExpression), "attribute_not_exists",
		"first-time grants must use a conditional PutItem")
	require.Contains(t, client.putInputs[0].ExpressionAttributeNames, "#n1")
	require.Equal(t, "PK", client.putInputs[0].ExpressionAttributeNames["#n1"])
}

func TestDraftRepositoryCreateDraftReviewGrantCodesKeyPreparationFailure(t *testing.T) {
	repo := NewDraftRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)

	err := repo.CreateDraftReviewGrant(context.Background(), &models.DraftReviewGrant{})
	require.Error(t, err)
	var appErr *apperrors.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, apperrors.CodeInternal, appErr.Code)
	require.Equal(t, "Failed to create draft review grant", appErr.Message)
}

func TestDraftRepositoryCreateDraftReviewGrantRejectsStaleCreateAfterRevoke(t *testing.T) {
	ctx := context.Background()
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, fakedb.New())
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DraftReviewGrant{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	// This stale grant represents caller A after its Get observed no grant.
	staleGrant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: time.Now().UTC().Add(-time.Minute),
	}

	// Caller B creates and then completes a revocation before A reaches Create.
	grant := &models.DraftReviewGrant{
		OwnerID:   staleGrant.OwnerID,
		DraftID:   staleGrant.DraftID,
		Reviewer:  staleGrant.Reviewer,
		GrantedAt: staleGrant.GrantedAt,
	}
	require.NoError(t, repo.CreateDraftReviewGrant(ctx, grant))
	revokedAt := time.Now().UTC()
	grant.RevokedAt = &revokedAt
	require.NoError(t, repo.RevokeDraftReviewGrant(ctx, grant))

	err = repo.CreateDraftReviewGrant(ctx, staleGrant)
	require.Error(t, err, "a stale create must fail instead of resurrecting a revoked grant")
	require.ErrorIs(t, err, dynamormerrors.ErrConditionFailed)

	persisted, getErr := repo.GetDraftReviewGrant(ctx, grant.OwnerID, grant.DraftID, grant.Reviewer)
	require.NoError(t, getErr)
	require.NotNil(t, persisted.RevokedAt, "the completed revocation must remain persisted")
	require.Empty(t, persisted.GSI2PK, "the revoked grant must stay out of the reviewer queue")
	require.Empty(t, persisted.GSI2SK, "the revoked grant must stay out of the reviewer queue")
}

func TestDraftRepositoryGetDraftReviewGrantMapsNotFoundSentinel(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.DraftReviewGrant")).Return(query).Once()
	query.On("Where", "PK", "=", "USER#owner#DRAFT#REVIEW").Return(query).Once()
	query.On("Where", "SK", "=", "GRANT#draft-1#REVIEWER#reviewer").Return(query).Once()
	query.On("First", mock.AnythingOfType("*models.DraftReviewGrant")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	grant, err := repo.GetDraftReviewGrant(ctx, "owner", "draft-1", "reviewer")
	require.Nil(t, grant)
	require.ErrorIs(t, err, storage.ErrNotFound)

	db.AssertExpectations(t)
	query.AssertExpectations(t)
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
