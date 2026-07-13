package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type fakeTransactionBuilder struct{}

func (fakeTransactionBuilder) Put(any, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) Create(any, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) Update(any, []string, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) UpdateWithBuilder(any, func(core.UpdateBuilder) error, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) Delete(any, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) ConditionCheck(any, ...core.TransactCondition) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) WithContext(context.Context) core.TransactionBuilder {
	return fakeTransactionBuilder{}
}
func (fakeTransactionBuilder) Execute() error                           { return nil }
func (fakeTransactionBuilder) ExecuteWithContext(context.Context) error { return nil }

func TestInstanceRepository_GetSoulBodyBinding_ReturnsNilWhenMissing(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	binding, err := repo.GetSoulBodyBinding(ctx, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	require.NoError(t, err)
	require.Nil(t, binding)
}

func TestInstanceRepository_GetSoulBodyBindingByUsername_ResolvesBinding(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBindingUsername")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBodyBindingUsername)
		out.PK = models.SoulBodyBindingUsernamePartitionKey("alice")
		out.SK = models.SKSoulBodyBindingUsername
		out.Username = "alice"
		out.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		out.UpdatedAt = time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
	}).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBodyBinding)
		out.PK = storage.InstanceConfigKey
		out.SK = models.SoulBodyBindingSortKey("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		out.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		out.Username = "alice"
		out.PrincipalAddress = "0x1111111111111111111111111111111111111111"
		out.BoundAt = time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
		out.UpdatedAt = out.BoundAt
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	binding, err := repo.GetSoulBodyBindingByUsername(ctx, " alice ")
	require.NoError(t, err)
	require.NotNil(t, binding)
	assert.Equal(t, "alice", binding.Username)
	assert.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", binding.AgentID)
}

func TestInstanceRepository_BindSoulBody_ReturnsExistingBindingForSameSoulAndBody(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBodyBinding)
		out.PK = storage.InstanceConfigKey
		out.SK = models.SoulBodyBindingSortKey("0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
		out.AgentID = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		out.Username = "alice"
		out.PrincipalAddress = "0x1111111111111111111111111111111111111111"
		out.BoundAt = time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
		out.UpdatedAt = out.BoundAt
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	binding, err := repo.BindSoulBody(ctx,
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"alice",
		"0x1111111111111111111111111111111111111111",
	)
	require.NoError(t, err)
	require.NotNil(t, binding)
	assert.Equal(t, "alice", binding.Username)
}

func TestInstanceRepository_BindSoulBody_ReturnsConflictWhenUsernameAlreadyBound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBindingUsername")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBodyBindingUsername)
		out.PK = models.SoulBodyBindingUsernamePartitionKey("alice")
		out.SK = models.SKSoulBodyBindingUsername
		out.Username = "alice"
		out.AgentID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	}).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Run(func(args mock.Arguments) {
		out := args.Get(0).(*models.InstanceSoulBodyBinding)
		out.PK = storage.InstanceConfigKey
		out.SK = models.SoulBodyBindingSortKey("0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
		out.AgentID = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
		out.Username = "alice"
		out.BoundAt = time.Date(2026, 3, 8, 12, 0, 0, 0, time.UTC)
		out.UpdatedAt = out.BoundAt
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	binding, err := repo.BindSoulBody(ctx,
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"alice",
		"0x1111111111111111111111111111111111111111",
	)
	require.Error(t, err)
	require.Nil(t, binding)
	require.ErrorIs(t, err, ErrSoulBodyAlreadyHasBinding)
}

func TestInstanceRepository_BindSoulBody_CreatesBindingWhenUnbound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	setupMockDB(db, q)
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBinding")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceSoulBodyBindingUsername")).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	repo.transactWriteFn = func(ctx context.Context, fn func(core.TransactionBuilder) error) error {
		return fn(fakeTransactionBuilder{})
	}

	binding, err := repo.BindSoulBody(ctx,
		"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"alice",
		"0x1111111111111111111111111111111111111111",
	)
	require.NoError(t, err)
	require.NotNil(t, binding)
	assert.Equal(t, "alice", binding.Username)
	assert.Equal(t, "0x1111111111111111111111111111111111111111", binding.PrincipalAddress)
	assert.Equal(t, storage.InstanceConfigKey, binding.GetPK())
}

func TestBuildSoulBodyBindingModels_NormalizesAndIndexes(t *testing.T) {
	t.Parallel()

	binding, index, err := buildSoulBodyBindingModels(
		" 0XAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA ",
		" alice ",
		" 0X1111111111111111111111111111111111111111 ",
	)
	require.NoError(t, err)
	require.Equal(t, "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", binding.AgentID)
	require.Equal(t, "alice", binding.Username)
	require.Equal(t, "0x1111111111111111111111111111111111111111", binding.PrincipalAddress)
	require.Equal(t, "alice", index.Username)
	require.Equal(t, binding.AgentID, index.AgentID)
}
