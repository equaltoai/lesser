package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	pkgErrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dmerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestBlockRepository_CreateBlock_IdempotentOnConditionFailed(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &BlockRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Block](mockDB, "tbl", zap.NewNop(), nil, "BlockRepository", "block"),
		logger:                 zap.NewNop(),
		db:                     mockDB,
	}

	mockQuery.On("Create").Return(dmerrors.ErrConditionFailed).Once()
	err := repo.CreateBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b", "act-1")
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestBlockRepository_CreateAndDeleteAndIsBlocked(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &BlockRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Block](mockDB, "tbl", zap.NewNop(), nil, "BlockRepository", "block"),
		logger:                 zap.NewNop(),
		db:                     mockDB,
	}

	mockQuery.On("Create").Return(nil).Once()
	err := repo.CreateBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b", "act-1")
	require.NoError(t, err)

	// IsBlocked notfound variants
	mockQuery.On("First", mock.Anything).Return(dmerrors.NewError("GetItem", "Block", dmerrors.ErrItemNotFound)).Once()
	ok, err := repo.IsBlocked(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.False(t, ok)
	require.NoError(t, err)

	mockQuery.On("First", mock.Anything).Return(pkgErrors.ItemNotFoundWithID("block", "x")).Once()
	ok, err = repo.IsBlocked(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.False(t, ok)
	require.NoError(t, err)

	mockQuery.On("First", mock.Anything).Return(errors.New("not found")).Once()
	ok, err = repo.IsBlocked(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.False(t, ok)
	require.Error(t, err)

	// IsBlocked other error
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.IsBlocked(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.Error(t, err)

	// DeleteBlock notfound is idempotent
	mockQuery.On("Delete").Return(dmerrors.ErrItemNotFound).Once()
	err = repo.DeleteBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)

	// DeleteBlock other error
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	err = repo.DeleteBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.Error(t, err)

	// DeleteBlock success
	mockQuery.On("Delete").Return(nil).Once()
	err = repo.DeleteBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestBlockRepository_PaginationAndCounts(t *testing.T) {
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	mockDB.On("WithContext", mock.Anything).Return(mockDB).Maybe()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Index", mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("OrderBy", mock.Anything, mock.Anything).Return(mockQuery).Maybe()
	mockQuery.On("Limit", mock.Anything).Return(mockQuery).Maybe()

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		slice := reflect.MakeSlice(v.Elem().Type(), 0, 2)
		slice = reflect.Append(slice, reflect.ValueOf(models.Block{Object: "https://example.com/users/b", Actor: "https://example.com/users/a"}))
		slice = reflect.Append(slice, reflect.ValueOf(models.Block{Object: "https://example.com/users/c", Actor: "https://example.com/users/a"}))
		v.Elem().Set(slice)
	}).Return(nil).Maybe()

	mockQuery.On("Count").Return(int64(3), nil).Maybe()

	repo := &BlockRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Block](mockDB, "tbl", zap.NewNop(), nil, "BlockRepository", "block"),
		logger:                 zap.NewNop(),
		db:                     mockDB,
	}

	users, _, err := repo.GetBlockedUsers(context.Background(), "https://example.com/users/a", 2, "")
	require.NoError(t, err)
	require.Len(t, users, 2)

	users, _, err = repo.GetUsersWhoBlocked(context.Background(), "https://example.com/users/b", 2, "")
	require.NoError(t, err)
	require.Len(t, users, 2)

	n, err := repo.CountBlockedUsers(context.Background(), "https://example.com/users/a")
	require.NoError(t, err)
	require.Equal(t, 3, n)

	n, err = repo.CountUsersWhoBlocked(context.Background(), "https://example.com/users/b")
	require.NoError(t, err)
	require.Equal(t, 3, n)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestCircuitBreakerRepository_Branches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	updateBuilder := new(mocks.MockUpdateBuilder)

	mockQuery.On("UpdateBuilder").Return(updateBuilder).Maybe()
	updateBuilder.On("Set", mock.Anything, mock.Anything).Return(updateBuilder).Maybe()
	updateBuilder.On("Execute").Return(nil).Maybe()

	repo := &CircuitBreakerRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.CircuitBreakerState](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerRepository", "circuitbreaker"),
		eventRepo:              NewEnhancedBaseRepository[*models.CircuitBreakerEvent](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerEventRepository", "circuitbreakerevent"),
	}

	// GetCircuitState not found -> default closed
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	state, err := repo.GetCircuitState(context.Background(), "inst-1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, "closed", state.Status)

	// GetCircuitState other error
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetCircuitState(context.Background(), "inst-2")
	require.Error(t, err)

	// SaveCircuitState create ok
	mockQuery.On("Create").Return(nil).Once()
	err = repo.SaveCircuitState(context.Background(), &models.CircuitBreakerState{InstanceID: "inst-3"})
	require.NoError(t, err)

	// SaveCircuitState condition failed -> get and update
	mockQuery.On("Create").Return(dmerrors.ErrConditionFailed).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()
	err = repo.SaveCircuitState(context.Background(), &models.CircuitBreakerState{InstanceID: "inst-4", Status: "open"})
	require.NoError(t, err)

	// UpdateCircuitState updateFn error
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	_, err = repo.UpdateCircuitState(context.Background(), "inst-5", func(_ *models.CircuitBreakerState) error {
		return errors.New("bad update")
	})
	require.Error(t, err)

	// RecordEvent failure is swallowed
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err = repo.RecordEvent(context.Background(), &models.CircuitBreakerEvent{InstanceID: "inst-6", EventType: "metric", Timestamp: time.Now()})
	require.NoError(t, err)

	// GetRecentEvents error path
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetRecentEvents(context.Background(), "inst-7", 5)
	require.Error(t, err)

	// DeleteCircuitState notfound is ok
	mockQuery.On("Delete").Return(dmerrors.ErrItemNotFound).Once()
	err = repo.DeleteCircuitState(context.Background(), "inst-8")
	require.NoError(t, err)

	// GetAllCircuitStates scan error
	mockQuery.On("Scan", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetAllCircuitStates(context.Background())
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
	updateBuilder.AssertExpectations(t)
}

func TestBlockRepository_AdditionalBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &BlockRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Block](mockDB, "tbl", zap.NewNop(), nil, "BlockRepository", "block"),
		logger:                 zap.NewNop(),
		db:                     mockDB,
	}

	// IsBlocked success
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if blk, ok := args.Get(0).(*models.Block); ok {
			blk.Actor = "https://example.com/users/a"
			blk.Object = "https://example.com/users/b"
		}
	}).Return(nil).Once()
	blocked, err := repo.IsBlocked(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.True(t, blocked)

	// IsBlockedBidirectional short-circuit
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	ok, err := repo.IsBlockedBidirectional(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.True(t, ok)

	// GetBlock success conversion
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if blk, ok := args.Get(0).(*models.Block); ok {
			blk.Actor = "https://example.com/users/a"
			blk.Object = "https://example.com/users/b"
			blk.ID = "act-1"
			blk.Published = time.Now().UTC()
			blk.CreatedAt = time.Now().UTC()
		}
	}).Return(nil).Once()
	b, err := repo.GetBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.Equal(t, "act-1", b.ID)

	// CountBlockedUsers error path
	mockQuery.On("Count").Return(int64(0), errors.New("boom")).Once()
	_, err = repo.CountBlockedUsers(context.Background(), "https://example.com/users/a")
	require.Error(t, err)

	// CountUsersWhoBlocked error path
	mockQuery.On("Count").Return(int64(0), errors.New("boom")).Once()
	_, err = repo.CountUsersWhoBlocked(context.Background(), "https://example.com/users/b")
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestCircuitBreakerRepository_MoreCoverage(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &CircuitBreakerRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.CircuitBreakerState](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerRepository", "circuitbreaker"),
		eventRepo:              NewEnhancedBaseRepository[*models.CircuitBreakerEvent](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerEventRepository", "circuitbreakerevent"),
	}

	// UpdateCircuitState happy path (Get existing, apply updateFn, Save create)
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if st, ok := args.Get(0).(*models.CircuitBreakerState); ok {
			st.InstanceID = "inst-ok"
			st.Status = "closed"
		}
	}).Return(nil).Once()
	mockQuery.On("Create").Return(nil).Once()
	state, err := repo.UpdateCircuitState(context.Background(), "inst-ok", func(s *models.CircuitBreakerState) error {
		s.Status = "open"
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "open", state.Status)

	// RecordStateChange / RecordMetric convenience paths
	mockQuery.On("Create").Return(nil).Twice()
	require.NoError(t, repo.RecordStateChange(context.Background(), "inst-ok", "closed", "open", "reason"))
	require.NoError(t, repo.RecordMetric(context.Background(), "inst-ok", false, errors.New("err"), "type"))

	// GetRecentEvents success
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if events, ok := args.Get(0).(*[]*models.CircuitBreakerEvent); ok {
			*events = []*models.CircuitBreakerEvent{{InstanceID: "inst-ok", EventType: "metric"}}
		}
	}).Return(nil).Once()
	ev, err := repo.GetRecentEvents(context.Background(), "inst-ok", 10)
	require.NoError(t, err)
	require.Len(t, ev, 1)

	// DeleteCircuitState other error
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	err = repo.DeleteCircuitState(context.Background(), "inst-bad")
	require.Error(t, err)

	// GetAllCircuitStates success
	mockQuery.On("Scan", mock.Anything).Run(func(args mock.Arguments) {
		if states, ok := args.Get(0).(*[]*models.CircuitBreakerState); ok {
			*states = []*models.CircuitBreakerState{{InstanceID: "inst-ok"}}
		}
	}).Return(nil).Once()
	_, err = repo.GetAllCircuitStates(context.Background())
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestBlockRepository_ConstructorsAndBidirectionalBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	_ = NewBlockRepository(mockDB, "tbl", zap.NewNop(), nil) // constructor coverage

	repo := &BlockRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.Block](mockDB, "tbl", zap.NewNop(), nil, "BlockRepository", "block"),
		logger:                 zap.NewNop(),
		db:                     mockDB,
	}

	// blocked in first direction -> short-circuit
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	ok, err := repo.IsBlockedBidirectional(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.True(t, ok)

	// blocked only in second direction
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	ok, err = repo.IsBlockedBidirectional(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.True(t, ok)

	// neither direction blocked
	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Twice()
	ok, err = repo.IsBlockedBidirectional(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.NoError(t, err)
	require.False(t, ok)

	// error propagation from IsBlocked
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.IsBlockedBidirectional(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.Error(t, err)

	// GetBlock error path
	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b")
	require.Error(t, err)

	// CreateBlock non-conditional error path
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err = repo.CreateBlock(context.Background(), "https://example.com/users/a", "https://example.com/users/b", "act-err")
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestCircuitBreakerRepository_ConstructorsAndMoreSaveBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	_ = NewCircuitBreakerRepository(mockDB, "tbl", zap.NewNop(), nil)
	_ = NewCircuitBreakerRepositoryBasic(mockDB, "tbl", zap.NewNop())

	repo := &CircuitBreakerRepository{
		EnhancedBaseRepository: NewEnhancedBaseRepository[*models.CircuitBreakerState](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerRepository", "circuitbreaker"),
		eventRepo:              NewEnhancedBaseRepository[*models.CircuitBreakerEvent](mockDB, "tbl", zap.NewNop(), nil, "CircuitBreakerEventRepository", "circuitbreakerevent"),
	}

	// SaveCircuitState create error
	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err := repo.SaveCircuitState(context.Background(), &models.CircuitBreakerState{InstanceID: "inst"})
	require.Error(t, err)

	// DeleteCircuitState success
	mockQuery.On("Delete").Return(nil).Once()
	err = repo.DeleteCircuitState(context.Background(), "inst")
	require.NoError(t, err)

	// GetCircuitState success
	mockQuery.On("First", mock.Anything).Return(nil).Once()
	state, err := repo.GetCircuitState(context.Background(), "inst")
	require.NoError(t, err)
	require.Equal(t, "inst", state.InstanceID)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
