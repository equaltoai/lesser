package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func testUploadGrantFixture(t *testing.T) *models.UploadGrant {
	t.Helper()
	now := time.Now().UTC()
	grant := &models.UploadGrant{
		Owner:         "alice",
		GrantID:       "grant-abc",
		ContentType:   "image/png",
		MaxSizeBytes:  5 * 1024 * 1024,
		ContentSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		S3Bucket:      "media-private",
		S3Key:         "media/2026/08/24/grant-abc.png",
		MediaID:       "media-abc",
		FileName:      "grant-abc.png",
		Status:        models.UploadGrantStatusMinted,
		GrantedAt:     now,
		ExpiresAt:     now.Add(15 * time.Minute),
	}
	require.NoError(t, grant.UpdateKeys())
	return grant
}

func TestUploadGrantRepositoryCreate(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.MatchedBy(func(g *models.UploadGrant) bool { return true })).Return(mockQuery)
	mockQuery.On("IfNotExists").Return(mockQuery)
	mockQuery.On("Create").Return(nil)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.CreateUploadGrant(context.Background(), testUploadGrantFixture(t)))
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestUploadGrantRepositoryCreateConflict(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.MatchedBy(func(g *models.UploadGrant) bool { return true })).Return(mockQuery)
	mockQuery.On("IfNotExists").Return(mockQuery)
	mockQuery.On("Create").Return(dynamormerrors.ErrConditionFailed)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.CreateUploadGrant(context.Background(), testUploadGrantFixture(t))
	require.Error(t, err)
}

func TestUploadGrantRepositoryGet(t *testing.T) {
	grant := testUploadGrantFixture(t)

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", "USER#alice#UPLOAD").Return(mockQuery)
	mockQuery.On("Where", "SK", "=", "GRANT#grant-abc").Return(mockQuery)
	mockQuery.On("First", mock.MatchedBy(func(g *models.UploadGrant) bool { return true })).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.UploadGrant)
		*dest = *grant
	}).Return(nil)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	got, err := repo.GetUploadGrant(context.Background(), "alice", "grant-abc")
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, grant.GrantID, got.GrantID)
	require.Equal(t, "USER#alice#UPLOAD", got.PK)
	require.Equal(t, "GRANT#grant-abc", got.SK)
}

func TestUploadGrantRepositoryGetNotFound(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery)
	mockQuery.On("First", mock.MatchedBy(func(g *models.UploadGrant) bool { return true })).Return(dynamormerrors.ErrItemNotFound)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	_, err := repo.GetUploadGrant(context.Background(), "alice", "grant-abc")
	require.ErrorIs(t, err, storage.ErrNotFound)
}

func TestUploadGrantRepositoryGetRejectsBlankIdentity(t *testing.T) {
	repo := NewUploadGrantRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	_, err := repo.GetUploadGrant(context.Background(), "  ", "grant-abc")
	require.Error(t, err)
}

func TestUploadGrantRepositoryConsume(t *testing.T) {
	grant := testUploadGrantFixture(t)
	now := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", grant.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", grant.SK).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Status", models.UploadGrantStatusUsed).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "UsedAt", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", "Status", "=", models.UploadGrantStatusMinted).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Version", grant.Version+1).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ConsumeUploadGrant(context.Background(), grant, models.UploadGrantStatusUsed, "", now))
	require.Equal(t, models.UploadGrantStatusUsed, grant.Status)
	require.Equal(t, 1, grant.Version, "consume must bump the version exactly once")
	require.NotNil(t, grant.UsedAt)
	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
	mockUpdateBuilder.AssertExpectations(t)
}

func TestUploadGrantRepositoryConsumeFailedDigest(t *testing.T) {
	grant := testUploadGrantFixture(t)
	now := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", grant.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", grant.SK).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Status", models.UploadGrantStatusFailedDigest).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "FailedAt", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "FailureReason", "digest mismatch").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", "Status", "=", models.UploadGrantStatusMinted).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", "Version", grant.Version+1).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(nil)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	require.NoError(t, repo.ConsumeUploadGrant(context.Background(), grant, models.UploadGrantStatusFailedDigest, "digest mismatch", now))
	require.Equal(t, models.UploadGrantStatusFailedDigest, grant.Status)
	require.NotNil(t, grant.FailedAt)
	require.Equal(t, "digest mismatch", grant.FailureReason)
}

func TestUploadGrantRepositoryConsumeRace(t *testing.T) {
	// A concurrent consume fails the version condition and must surface
	// interfaces.ErrUploadGrantConsumed so the finalize path can fail closed
	// without admitting a second asset.
	grant := testUploadGrantFixture(t)
	now := time.Now().UTC()

	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", grant.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", grant.SK).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", "Status", "=", models.UploadGrantStatusMinted).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(dynamormerrors.ErrConditionFailed)

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.ConsumeUploadGrant(context.Background(), grant, models.UploadGrantStatusUsed, "", now)
	require.Error(t, err)
	require.True(t, errors.Is(err, interfaces.ErrUploadGrantConsumed))
}

func TestUploadGrantRepositoryConsumeRejectsInvalidStatus(t *testing.T) {
	grant := testUploadGrantFixture(t)
	repo := NewUploadGrantRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	err := repo.ConsumeUploadGrant(context.Background(), grant, "SOMETHING_ELSE", "", time.Now().UTC())
	require.Error(t, err)
}

func TestUploadGrantRepositoryCreateRejectsUnbounded(t *testing.T) {
	repo := NewUploadGrantRepository(new(mocks.MockDB), "test-table", zap.NewNop(), nil)
	grant := testUploadGrantFixture(t)
	grant.ExpiresAt = time.Time{}
	err := repo.CreateUploadGrant(context.Background(), grant)
	require.Error(t, err)
}

func TestUploadGrantRepositoryCreateGenericError(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.MatchedBy(func(g *models.UploadGrant) bool { return true })).Return(mockQuery)
	mockQuery.On("IfNotExists").Return(mockQuery)
	mockQuery.On("Create").Return(errors.New("database error"))

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.CreateUploadGrant(context.Background(), testUploadGrantFixture(t))
	require.Error(t, err)
}

func TestUploadGrantRepositoryConsumeGenericError(t *testing.T) {
	grant := testUploadGrantFixture(t)
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockUpdateBuilder := new(mocks.MockUpdateBuilder)
	mockDB.On("WithContext", context.Background()).Return(mockDB)
	mockDB.On("Model", mock.Anything).Return(mockQuery)
	mockQuery.On("Where", "PK", "=", grant.PK).Return(mockQuery)
	mockQuery.On("Where", "SK", "=", grant.SK).Return(mockQuery)
	mockQuery.On("UpdateBuilder").Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Set", mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Remove", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("ConditionVersion", mock.Anything).Return(mockUpdateBuilder)
	mockUpdateBuilder.On("Execute").Return(errors.New("database error"))

	repo := NewUploadGrantRepository(mockDB, "test-table", zap.NewNop(), nil)
	err := repo.ConsumeUploadGrant(context.Background(), grant, models.UploadGrantStatusUsed, "", time.Now().UTC())
	require.Error(t, err)
}
