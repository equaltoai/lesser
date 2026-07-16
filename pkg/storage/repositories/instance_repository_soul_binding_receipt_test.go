package repositories

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

func TestInstanceRepository_GetSoulBindingIdempotencyReceipt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockDB(db, q)

	q.On("First", mock.AnythingOfType("*models.InstanceSoulBindingIdempotencyReceipt")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBindingIdempotencyReceipt)
		out.PK = models.SoulBindingIdempotencyPartitionKey("lesser-body")
		out.SK = models.SoulBindingIdempotencySortKey("bind-key")
		out.CallerID = "lesser-body"
		out.IdempotencyKeyHash = models.SoulBindingIdempotencyKeyHash("bind-key")
		out.PayloadHash = "sha256:payload"
		out.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		out.ActorUsername = "drone-ada"
		out.Status = "received"
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	receipt, err := repo.GetSoulBindingIdempotencyReceipt(ctx, " Lesser-Body ", " bind-key ")
	require.NoError(t, err)
	require.NotNil(t, receipt)
	require.Equal(t, "lesser-body", receipt.CallerID)
	require.Equal(t, "sha256:payload", receipt.PayloadHash)
}

func TestInstanceRepository_GetSoulBindingIdempotencyReceipt_ReturnsNilForEmptyMissingAndBlankRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	var nilRepo *InstanceRepository
	receipt, err := nilRepo.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", "bind-key")
	require.NoError(t, err)
	require.Nil(t, receipt)

	repoWithNoReceiptStore := &InstanceRepository{}
	receipt, err = repoWithNoReceiptStore.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", "bind-key")
	require.NoError(t, err)
	require.Nil(t, receipt)

	repoWithDB := NewInstanceRepository(new(dynamormmocks.MockDB), "test-table", zap.NewNop())
	receipt, err = repoWithDB.GetSoulBindingIdempotencyReceipt(ctx, " ", "bind-key")
	require.NoError(t, err)
	require.Nil(t, receipt)
	receipt, err = repoWithDB.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", " ")
	require.NoError(t, err)
	require.Nil(t, receipt)

	t.Run("missing row", func(t *testing.T) {
		t.Parallel()

		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceSoulBindingIdempotencyReceipt")).Return(dynamormerrors.ErrItemNotFound).Once()

		repo := NewInstanceRepository(db, "test-table", zap.NewNop())
		receipt, err := repo.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", "bind-key")
		require.NoError(t, err)
		require.Nil(t, receipt)
	})

	t.Run("blank persisted row", func(t *testing.T) {
		t.Parallel()

		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		setupMockDB(db, q)
		q.On("First", mock.AnythingOfType("*models.InstanceSoulBindingIdempotencyReceipt")).Return(nil).Once()

		repo := NewInstanceRepository(db, "test-table", zap.NewNop())
		receipt, err := repo.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", "bind-key")
		require.NoError(t, err)
		require.Nil(t, receipt)
	})
}

func TestInstanceRepository_GetSoulBindingIdempotencyReceipt_PropagatesReadError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBindingIdempotencyReceipt")).Return(errors.New("read failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	receipt, err := repo.GetSoulBindingIdempotencyReceipt(ctx, "lesser-body", "bind-key")
	require.Error(t, err)
	require.Nil(t, receipt)
}

func TestInstanceRepository_CreateAndUpdateSoulBindingIdempotencyReceipt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	receipt := models.NewInstanceSoulBindingIdempotencyReceipt(
		"lesser-body",
		"bind-key",
		"sha256:payload",
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"drone-ada",
		testSoulBindingReceiptTTL(),
	)

	var nilRepo *InstanceRepository
	require.NoError(t, nilRepo.CreateSoulBindingIdempotencyReceipt(ctx, receipt))
	require.NoError(t, nilRepo.UpdateSoulBindingIdempotencyReceipt(ctx, receipt))
	require.NoError(t, (&InstanceRepository{}).CreateSoulBindingIdempotencyReceipt(ctx, receipt))
	require.NoError(t, (&InstanceRepository{}).UpdateSoulBindingIdempotencyReceipt(ctx, receipt))

	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)
	setupMockDB(db, q)
	q.On("IfNotExists").Return(q).Once()
	q.On("Create").Return(nil).Once()
	q.On("Update", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.CreateSoulBindingIdempotencyReceipt(ctx, receipt))

	receipt.Status = "bound"
	receipt.BindingState = "bound"
	require.NoError(t, repo.UpdateSoulBindingIdempotencyReceipt(ctx, receipt))
}

func testSoulBindingReceiptTTL() time.Time {
	return time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
}
