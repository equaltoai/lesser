package repositories

import (
	"context"
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
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
	"go.uber.org/zap"
)

func TestMediaRepositoryUpdateMediaPublishedStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "PublishedS3Key", "published/media/m1.png").Return(update).Once()
	update.On("Set", "PublishedURL", "https://cdn.example.test/published/media/m1.png").Return(update).Once()
	update.On("Set", "PublishedAt", now).Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	err := repo.UpdateMediaPublishedState(ctx, "m1", "published/media/m1.png", "https://cdn.example.test/published/media/m1.png", now, 3)
	require.NoError(t, err)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryUpdateMediaEditorialStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "EditorialState", models.EditorialLifecycleWithdrawn).Return(update).Once()
	update.On("Remove", "SupersededByMediaID").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	err := repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleWithdrawn, "", 3)
	require.NoError(t, err)
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryUpdateMediaEditorialStateSupersededNamesSuccessor(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Set", "EditorialState", models.EditorialLifecycleSuperseded).Return(update).Once()
	update.On("Set", "SupersededByMediaID", "m2").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleSuperseded, "m2", 3))
	require.Error(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleSuperseded, "  ", 3))
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

func TestMediaRepositoryClearMediaPublishedStateIsFieldScoped(t *testing.T) {
	ctx := context.Background()
	db := new(mocks.MockDB)
	query := new(mocks.MockQuery)
	update := new(mocks.MockUpdateBuilder)
	db.On("WithContext", mock.Anything).Return(db).Once()
	db.On("Model", mock.AnythingOfType("*models.Media")).Return(query).Once()
	query.On("Where", "PK", "=", "media#m1").Return(query).Once()
	query.On("Where", "SK", "=", "version#original").Return(query).Once()
	query.On("UpdateBuilder").Return(update).Once()
	update.On("Remove", "PublishedS3Key").Return(update).Once()
	update.On("Remove", "PublishedURL").Return(update).Once()
	update.On("Remove", "PublishedAt").Return(update).Once()
	update.On("ConditionExists", "PK").Return(update).Once()
	update.On("ConditionVersion", int64(3)).Return(update).Once()
	update.On("Set", "ModelVersion", 4).Return(update).Once()
	update.On("Execute").Return(nil).Once()

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ClearMediaPublishedState(ctx, "m1", 3))
	db.AssertExpectations(t)
	query.AssertExpectations(t)
	update.AssertExpectations(t)
}

// seedMediaEditorialRow prepares and creates a media row through the real
// TableTheory expression path. TableTheory does not auto-run model lifecycle
// hooks on direct Create, so the row is prepared like the production
// repository's CreateMedia does.
func seedMediaEditorialRow(t *testing.T, db core.DB, ctx context.Context, row *models.Media) {
	t.Helper()
	require.NoError(t, row.BeforeCreate())
	require.NoError(t, db.WithContext(ctx).Model(row).Create())
}

// mediaEditorialSeedRow builds a valid internal editorial media record that
// TableTheory can create against the fakedb-backed client.
func mediaEditorialSeedRow(id, owner, digest string) *models.Media {
	now := time.Now().UTC()
	return &models.Media{
		MediaID:      id,
		UserID:       owner,
		FileName:     id + ".png",
		ContentType:  "image/png",
		FileSize:     12,
		ContentHash:  digest,
		S3Bucket:     "media-private",
		S3Key:        "owner/" + id + ".png",
		Status:       models.StatusReady,
		Width:        120,
		Height:       80,
		Visibility:   models.MediaVisibilityInternal,
		ModelVersion: 1,
		UploadedAt:   now,
		CreatedAt:    now,
		UpdatedAt:    now,
		Provenance: &models.MediaProvenance{
			Origin:           models.EditorialMediaOriginSupplied,
			ResponsibleActor: owner,
			RecordedAt:       now,
			ContentIntegrity: digest,
		},
	}
}

func TestMediaRepositoryUpdateMediaPublishedStatePersistsRealExpression(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Media{}))

	digest := "sha256:" + strings.Repeat("a", 64)
	seeded := mediaEditorialSeedRow("m1", "alice", digest)
	seedMediaEditorialRow(t, db, ctx, seeded)

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	now := time.Now().UTC()
	require.NoError(t, repo.UpdateMediaPublishedState(ctx, "m1", "published/owner/m1.png", "https://cdn.example.test/published/owner/m1.png", now, 1))

	got, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Equal(t, "published/owner/m1.png", got.PublishedS3Key)
	require.Equal(t, "https://cdn.example.test/published/owner/m1.png", got.PublishedURL)
	require.NotNil(t, got.PublishedAt)
	require.True(t, got.PublishedAt.Equal(now))
	require.Equal(t, 2, got.ModelVersion, "a successful mint write must advance the model version")

	require.NotEmpty(t, client.updateInputs)
	input := client.updateInputs[len(client.updateInputs)-1]
	require.Contains(t, aws.ToString(input.UpdateExpression), "SET")
	require.Contains(t, aws.ToString(input.ConditionExpression), "attribute_exists",
		"the real expression must condition on row existence")
	require.True(t, expressionReferencesAttribute(input, "modelVersion"),
		"the real expression must condition on the observed model version: %s", aws.ToString(input.ConditionExpression))

	// A stale version (a concurrent lifecycle write interleaved) must fail
	// closed and leave the minted state untouched. The stale caller still
	// holds the pre-mint version it read.
	err = repo.UpdateMediaPublishedState(ctx, "m1", "published/owner/m1-new.png", "https://cdn.example.test/published/owner/m1-new.png", now.Add(time.Minute), 1)
	require.Error(t, err)
	var appErr *apperrors.AppError
	require.True(t, errors.As(err, &appErr))
	require.Equal(t, apperrors.CodeConflict, appErr.Code, "a version-mismatched mint must report the conditional-check failure")
	require.True(t, errors.Is(err, storage.ErrNotFound), "the joined not-found sentinel must remain discoverable")

	unchanged, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Equal(t, "published/owner/m1.png", unchanged.PublishedS3Key)
	require.Equal(t, "https://cdn.example.test/published/owner/m1.png", unchanged.PublishedURL)
	require.Equal(t, 2, unchanged.ModelVersion, "the rejected write must not advance the version or overwrite the mint")

	// A genuinely missing row also fails closed.
	require.Error(t, repo.UpdateMediaPublishedState(ctx, "missing", "published/x", "https://cdn.example.test/published/x", now, 1))
}

func TestMediaRepositoryUpdateMediaEditorialStateRoundTripClearsRealExpression(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Media{}))

	digest := "sha256:" + strings.Repeat("b", 64)
	seeded := mediaEditorialSeedRow("m1", "alice", digest)
	seedMediaEditorialRow(t, db, ctx, seeded)

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleWithdrawn, "", 1))
	withdrawn, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Equal(t, models.EditorialLifecycleWithdrawn, withdrawn.EditorialState)
	require.Empty(t, withdrawn.SupersededByMediaID)
	require.Equal(t, 2, withdrawn.ModelVersion)

	require.NoError(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleSuperseded, "m2", 2))
	superseded, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Equal(t, models.EditorialLifecycleSuperseded, superseded.EditorialState)
	require.Equal(t, "m2", superseded.SupersededByMediaID)
	require.Equal(t, 3, superseded.ModelVersion)

	// available must remove both lifecycle attributes through the real
	// expression, not just leave them empty in the in-memory snapshot.
	before := len(client.updateInputs)
	require.NoError(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleAvailable, "", 3))
	cleared, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Empty(t, cleared.EditorialState, "available must clear the editorial state")
	require.Empty(t, cleared.SupersededByMediaID, "available must clear the superseded-by attribute")
	require.Equal(t, 4, cleared.ModelVersion)

	clearInput := client.updateInputs[len(client.updateInputs)-1]
	require.True(t, before < len(client.updateInputs))
	require.Contains(t, aws.ToString(clearInput.UpdateExpression), "REMOVE")
	names := clearInput.ExpressionAttributeNames
	require.NotEmpty(t, names)
	namesJoined := strings.ToLower(strings.Join(mapValues(names), " "))
	require.Contains(t, namesJoined, "editorialstate")
	require.Contains(t, namesJoined, "supersededbymediaid")

	// A stale version interleaving a concurrent write must fail closed.
	require.Error(t, repo.UpdateMediaEditorialState(ctx, "m1", models.EditorialLifecycleWithdrawn, "", 2))
	after, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Empty(t, after.EditorialState)
	require.Equal(t, 4, after.ModelVersion, "the rejected lifecycle write must not advance the version")
}

func TestMediaRepositoryClearMediaPublishedStateRemovesRealExpression(t *testing.T) {
	ctx := context.Background()
	client := &draftReviewRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.Media{}))

	digest := "sha256:" + strings.Repeat("c", 64)
	seeded := mediaEditorialSeedRow("m1", "alice", digest)
	seedMediaEditorialRow(t, db, ctx, seeded)

	repo := NewMediaRepository(db, "test-table", zap.NewNop(), nil)
	now := time.Now().UTC()
	require.NoError(t, repo.UpdateMediaPublishedState(ctx, "m1", "published/owner/m1.png", "https://cdn.example.test/published/owner/m1.png", now, 1))

	// A stale clear must not clobber a concurrently re-minted serving.
	require.Error(t, repo.ClearMediaPublishedState(ctx, "m1", 5))

	require.NoError(t, repo.ClearMediaPublishedState(ctx, "m1", 2))
	got, err := repo.GetMedia(ctx, "m1")
	require.NoError(t, err)
	require.Empty(t, got.PublishedS3Key)
	require.Empty(t, got.PublishedURL)
	require.Nil(t, got.PublishedAt)
	require.Equal(t, 3, got.ModelVersion)

	clearInput := client.updateInputs[len(client.updateInputs)-1]
	require.Contains(t, aws.ToString(clearInput.UpdateExpression), "REMOVE")
	require.Contains(t, aws.ToString(clearInput.ConditionExpression), "attribute_exists")
	require.True(t, expressionReferencesAttribute(clearInput, "modelVersion"),
		"the clear expression must condition on the observed model version: %s", aws.ToString(clearInput.ConditionExpression))

}

// expressionReferencesAttribute reports whether the compiled expression
// references the named attribute through its placeholder mapping.
func expressionReferencesAttribute(input *dynamodb.UpdateItemInput, attribute string) bool {
	if input == nil || input.ConditionExpression == nil {
		return false
	}
	condition := aws.ToString(input.ConditionExpression)
	for _, name := range input.ExpressionAttributeNames {
		if strings.EqualFold(name, attribute) {
			for placeholder := range input.ExpressionAttributeNames {
				if strings.Contains(condition, placeholder) {
					return true
				}
			}
		}
	}
	return false
}

func mapValues[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+":"+anyString(v))
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
