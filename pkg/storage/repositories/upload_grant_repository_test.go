package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v3"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"github.com/theory-cloud/tabletheory/v3/pkg/session"
	"github.com/theory-cloud/tabletheory/v3/pkg/testing/fakedb"
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

// uploadGrantRecordingDynamo wraps fakedb and records every real UpdateItem
// expression so a test can prove the single-use consume's CAS conditions are
// actually present in the mutation the repository sends to DynamoDB.
type uploadGrantRecordingDynamo struct {
	*fakedb.Fake
	updateInputs []*dynamodb.UpdateItemInput
}

func (d *uploadGrantRecordingDynamo) UpdateItem(ctx context.Context, input *dynamodb.UpdateItemInput, opts ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	d.updateInputs = append(d.updateInputs, input)
	return d.Fake.UpdateItem(ctx, input, opts...)
}

// newUploadGrantRealRepo builds the production repository adapter over the
// in-process fakedb DynamoDB (the M1/M2 real-expression precedent), so the
// version-conditioned consume is exercised against the real UpdateItem
// expression rather than mocks.
func newUploadGrantRealRepo(t *testing.T) (*UploadGrantRepository, *uploadGrantRecordingDynamo) {
	t.Helper()
	client := &uploadGrantRecordingDynamo{Fake: fakedb.New()}
	db, err := tabletheory.NewWithClient(session.Config{Region: "us-east-1"}, client)
	require.NoError(t, err)
	require.NoError(t, db.CreateTable(&models.UploadGrant{}))

	repo := NewUploadGrantRepository(db, "test-table", zap.NewNop(), nil)
	repo.SetValidationService(nil)
	repo.SetPermissionService(nil)
	repo.SetEventService(nil)
	repo.SetCachingService(nil)

	// The fakedb Create does not auto-run model key hooks; CreateUploadGrant
	// calls UpdateKeys itself, so this is only relevant for direct seeding.
	return repo, client
}

func TestUploadGrantRepositoryConsumeSingleUseRealExpression(t *testing.T) {
	ctx := context.Background()
	repo, client := newUploadGrantRealRepo(t)

	// Persist a MINTED v0 grant through the real adapter.
	grant := testUploadGrantFixture(t)
	require.NoError(t, repo.CreateUploadGrant(ctx, grant))

	// Two concurrent finalizers both observed the same MINTED v0 row; each runs
	// the version+MINTED-conditioned consume. Exactly one may win.
	staleConcurrentCopy := *grant
	now := time.Now().UTC()
	firstErr := repo.ConsumeUploadGrant(ctx, grant, models.UploadGrantStatusUsed, "", now)
	secondErr := repo.ConsumeUploadGrant(ctx, &staleConcurrentCopy, models.UploadGrantStatusUsed, "", now)

	wins := 0
	for _, err := range []error{firstErr, secondErr} {
		if err == nil {
			wins++
			continue
		}
		require.True(t, errors.Is(err, interfaces.ErrUploadGrantConsumed), "the losing consume must surface ErrUploadGrantConsumed, got %v", err)
	}
	require.Equal(t, 1, wins, "exactly one of the double-consume attempts wins")

	// Both attempts issued a real UpdateItem expression carrying the CAS
	// conditions; the loser's mutation was rejected with no state change.
	require.Len(t, client.updateInputs, 2, "both consume attempts must issue a real UpdateItem expression")
	for _, input := range client.updateInputs {
		require.NotNil(t, input.ConditionExpression, "the consume update must carry the MINTED+version condition")
	}

	loaded, err := repo.GetUploadGrant(ctx, "alice", grant.GrantID)
	require.NoError(t, err)
	require.Equal(t, models.UploadGrantStatusUsed, loaded.Status)
	require.Equal(t, 1, loaded.Version, "the winner bumps the version exactly once")
	require.NotNil(t, loaded.UsedAt)
}
