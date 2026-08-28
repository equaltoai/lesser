package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
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

func TestDraftRepositoryUpdateDraftEditorialMediaUsesFieldScopedTableTheoryUpdate(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	position := 2
	persisted := &models.Draft{
		AuthorID: "owner", ID: "draft-media", ContentType: "Article",
		Content: "owner concurrent edit", ContentFormat: "markdown", Status: "draft",
		EditorialMedia: []models.DraftMediaUsage{{MediaID: "old", Role: models.EditorialMediaRoleInline, InlinePosition: &position}},
		CreatedAt:      time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	stale := *persisted
	stale.Content = "stale content that must not be written"
	stale.EditorialMedia = []models.DraftMediaUsage{{MediaID: "replacement", Role: models.EditorialMediaRoleHero}}
	stale.UpdatedAt = persisted.UpdatedAt.Add(time.Minute)
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", &stale))

	got, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.Equal(t, "owner concurrent edit", got.Content)
	require.Equal(t, stale.EditorialMedia, got.EditorialMedia)
	require.True(t, stale.UpdatedAt.Equal(got.UpdatedAt))

	stale.EditorialMedia = []models.DraftMediaUsage{}
	stale.UpdatedAt = stale.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", &stale))

	cleared, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.Empty(t, cleared.EditorialMedia,
		"the real TableTheory update expression must remove an omitempty slice when the replacement is empty")
	require.Equal(t, "owner concurrent edit", cleared.Content)
	require.Len(t, client.updateInputs, 2)
	require.Contains(t, aws.ToString(client.updateInputs[1].UpdateExpression), "REMOVE")
	require.Contains(t, aws.ToString(client.updateInputs[1].ConditionExpression), "attribute_exists")

	missing := stale
	missing.ID = "missing"
	err = repo.UpdateDraftEditorialMedia(ctx, "owner", &missing)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict),
		"a missing-row CAS write is indistinguishable from a stale version and surfaces as a conflict: %v", err)
}

func TestDraftRepositoryUpdateDraftSkipsStaleEditorialMedia(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	persisted := &models.Draft{
		AuthorID: "owner", ID: "draft-content", ContentType: "Article",
		Content: "original content", ContentFormat: "markdown", Status: "draft",
		EditorialMedia: []models.DraftMediaUsage{{MediaID: "stale-binding", Role: models.EditorialMediaRoleHero}},
		CreatedAt:      time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	stale, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.NotEmpty(t, stale.EditorialMedia, "the full-model writer must carry a non-empty stale association")

	replacement := *stale
	replacement.EditorialMedia = []models.DraftMediaUsage{{MediaID: "concurrent-replacement", Role: models.EditorialMediaRoleSocialCard}}
	replacement.UpdatedAt = replacement.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", &replacement))

	client.updateInputs = nil
	stale.Content = "content writer update"
	stale.UpdatedAt = replacement.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.UpdateDraft(ctx, "owner", stale))
	require.Len(t, client.updateInputs, 1)

	input := client.updateInputs[0]
	require.NotContains(t, strings.ToLower(aws.ToString(input.UpdateExpression)), "editorialmedia")
	for _, attributeName := range input.ExpressionAttributeNames {
		require.NotEqual(t, "editorialmedia", strings.ToLower(attributeName),
			"the sparse full-model update must not select the editorial-media attribute")
	}
	expressionValues, err := json.Marshal(input.ExpressionAttributeValues)
	require.NoError(t, err)
	require.NotContains(t, strings.ToLower(string(expressionValues)), "editorialmedia")
	require.NotContains(t, string(expressionValues), "stale-binding",
		"the stale association must not appear among update expression values")

	got, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.Equal(t, "content writer update", got.Content)
	require.Equal(t, replacement.EditorialMedia, got.EditorialMedia,
		"the concurrently replaced association must survive a stale full-model content update")
}

// TestDraftRepositoryStaleContentSaveCannotDowngradeMediaCASVersion pins the
// content-lane version choke point: UpdateDraft zeroes ModelVersion on the
// sparse copy, so a content autosave holding a pre-bump snapshot never selects
// the attribute. Without the choke point the stale save would write the old
// version back and a stale media-set CAS on that version would succeed,
// reopening the setDraftEditorialMedia lost-update seam (M4 fold-in b).
func TestDraftRepositoryStaleContentSaveCannotDowngradeMediaCASVersion(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Draft{}))

	persisted := &models.Draft{
		AuthorID: "owner", ID: "draft-version-lane", ContentType: "Article",
		Content: "original content", ContentFormat: "markdown", Status: "draft",
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, persisted.UpdateKeys())
	require.NoError(t, db.WithContext(ctx).Model(persisted).Create())

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	// Three media-sets advance the version to 3; the same v3 row is snapshotted
	// by both the media lane and the content lane.
	winner := *persisted
	for i, mediaID := range []string{"media-a", "media-b", "media-c"} {
		winner.EditorialMedia = []models.DraftMediaUsage{{MediaID: mediaID, Role: models.EditorialMediaRoleHero}}
		winner.UpdatedAt = winner.UpdatedAt.Add(time.Minute)
		require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", &winner))
		require.Equal(t, i+1, winner.ModelVersion, "the winning media-set snapshot carries the bumped version")
	}
	require.Equal(t, 3, winner.ModelVersion)
	staleContent, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.Equal(t, 3, staleContent.ModelVersion)
	staleLosingMedia := winner // a second v3 snapshot that will never win

	// The media lane wins again: v3 -> v4.
	winner.EditorialMedia = []models.DraftMediaUsage{{MediaID: "media-d", Role: models.EditorialMediaRoleHero}}
	winner.UpdatedAt = winner.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.UpdateDraftEditorialMedia(ctx, "owner", &winner))
	require.Equal(t, 4, winner.ModelVersion)

	// A stale v3 content autosave runs. The content lane must not select
	// modelVersion (zeroed choke point), so the stored version stays 4.
	client.updateInputs = nil
	staleContent.Content = "stale autosave carrying a v3 snapshot"
	staleContent.UpdatedAt = winner.UpdatedAt.Add(time.Minute)
	require.NoError(t, repo.UpdateDraft(ctx, "owner", staleContent))
	for _, input := range client.updateInputs {
		for _, attributeName := range input.ExpressionAttributeNames {
			require.NotEqual(t, "modelversion", strings.ToLower(attributeName),
				"the content lane must never select the media-CAS version attribute")
		}
	}

	// A stale v3 media-set CAS must now conflict: the stored version is still 4.
	err = repo.UpdateDraftEditorialMedia(ctx, "owner", &staleLosingMedia)
	require.Error(t, err)
	require.True(t, apperrors.HasCode(err, apperrors.CodeConflict),
		"a stale media-set after a stale content save must surface CONFLICT: %v", err)

	got, err := repo.GetDraft(ctx, "owner", persisted.ID)
	require.NoError(t, err)
	require.Equal(t, 4, got.ModelVersion, "the stale content save must not downgrade the media-CAS version")
	require.Equal(t, "stale autosave carrying a v3 snapshot", got.Content)
	require.Equal(t, []models.DraftMediaUsage{{MediaID: "media-d", Role: models.EditorialMediaRoleHero}}, got.EditorialMedia,
		"the losing media-set must not land")
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
	update.On("Remove", "ExpiresAt").Return(update).Once()
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

func TestDraftRepositoryRegrantDraftReviewGrantRefreshesExpiry(t *testing.T) {
	ctx := context.Background()
	grantedAt := time.Now().UTC()
	expiresAt := grantedAt.Add(time.Hour)
	grant := &models.DraftReviewGrant{
		OwnerID:   "owner",
		DraftID:   "draft-1",
		Reviewer:  "reviewer",
		GrantedAt: grantedAt,
		ExpiresAt: &expiresAt,
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
	update.On("Set", "ExpiresAt", expiresAt).Return(update).Once()
	update.On("Set", "GSI2PK", "DRAFT#REVIEWER#reviewer").Return(update).Once()
	update.On("Set", "GSI2SK", grant.GSI2SK).Return(update).Once()
	update.On("Remove", "RevokedAt").Return(update).Once()
	update.On("ConditionVersion", int64(4)).Return(update).Once()
	update.On("Set", "Version", 5).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.RegrantDraftReviewGrant(ctx, grant))
	require.Equal(t, 5, grant.Version)
	require.Equal(t, expiresAt, *grant.ExpiresAt)

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

func TestDraftRepositoryListsReviewAssignmentsByOwner(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.DraftReviewGrant")).Return(query).Once()
	query.On("Where", "PK", "=", "USER#owner#DRAFT#REVIEW").Return(query).Once()
	query.On("Where", "SK", "begins_with", "GRANT#").Return(query).Once()
	query.On("OrderBy", "SK", "ASC").Return(query).Once()
	query.On("Limit", mock.Anything).Return(query).Maybe()
	query.On("AllPaginated", mock.AnythingOfType("*[]models.DraftReviewGrant")).Run(func(args mock.Arguments) {
		rows := args.Get(0).(*[]models.DraftReviewGrant)
		*rows = []models.DraftReviewGrant{{DraftID: "d1", Reviewer: "reviewer"}}
	}).Return(&core.PaginatedResult{HasMore: false}, nil).Once()

	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)
	grants, err := repo.ListDraftReviewGrantsByOwner(ctx, " owner ")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "d1", grants[0].DraftID)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
}

func TestDraftRepositoryRegrantDraftReviewGrantExpiresAtRealExpression(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.DraftReviewGrant{}))
	repo := NewDraftRepository(db, "test-table", zap.NewNop(), nil)

	grantedAt := time.Now().UTC()
	first := &models.DraftReviewGrant{OwnerID: "owner", DraftID: "draft-1", Reviewer: "reviewer", GrantedAt: grantedAt}
	require.NoError(t, repo.CreateDraftReviewGrant(ctx, first))

	// Re-grant with a fresh expiry: the real expression must SET ExpiresAt and
	// the read-back must carry it.
	expiresAt := grantedAt.Add(time.Hour)
	regrant := &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: "draft-1", Reviewer: "reviewer",
		GrantedAt: grantedAt, ExpiresAt: &expiresAt, Version: first.Version,
	}
	require.NoError(t, repo.RegrantDraftReviewGrant(ctx, regrant))

	persisted, err := repo.GetDraftReviewGrant(ctx, "owner", "draft-1", "reviewer")
	require.NoError(t, err)
	require.NotNil(t, persisted.ExpiresAt)
	require.True(t, persisted.ExpiresAt.Equal(expiresAt))
	require.True(t, persisted.IsActive(time.Now().UTC()), "the re-granted expiry must make the grant active again")

	setInput := client.updateInputs[len(client.updateInputs)-1]
	require.True(t, updateExpressionReferencesAttribute(setInput, "ExpiresAt"),
		"the regrant must SET ExpiresAt through the real expression")

	// Re-grant without an expiry: the real expression must REMOVE ExpiresAt so
	// the attribute does not linger in the persisted row.
	noExpiry := &models.DraftReviewGrant{
		OwnerID: "owner", DraftID: "draft-1", Reviewer: "reviewer",
		GrantedAt: grantedAt, Version: persisted.Version,
	}
	require.NoError(t, repo.RegrantDraftReviewGrant(ctx, noExpiry))

	cleared, err := repo.GetDraftReviewGrant(ctx, "owner", "draft-1", "reviewer")
	require.NoError(t, err)
	require.Nil(t, cleared.ExpiresAt, "regrant without expiry must remove the ExpiresAt attribute")

	clearInput := client.updateInputs[len(client.updateInputs)-1]
	require.Contains(t, aws.ToString(clearInput.UpdateExpression), "REMOVE")
	require.True(t, updateExpressionReferencesAttribute(clearInput, "ExpiresAt"),
		"the removal must target ExpiresAt through the real expression")
}

// updateExpressionReferencesAttribute reports whether the compiled update
// expression references the named attribute through its placeholder mapping.
func updateExpressionReferencesAttribute(input *dynamodb.UpdateItemInput, attribute string) bool {
	if input == nil || input.UpdateExpression == nil {
		return false
	}
	expression := aws.ToString(input.UpdateExpression)
	for placeholder, name := range input.ExpressionAttributeNames {
		if strings.EqualFold(name, attribute) && strings.Contains(expression, placeholder) {
			return true
		}
	}
	return false
}
