package cost

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type fakeDB struct {
	query core.Query

	transactionErr      error
	transactionFnCalled bool
}

func (db *fakeDB) Model(model any) core.Query {
	return db.query
}

func (db *fakeDB) Transaction(fn func(tx *core.Tx) error) error {
	if db.transactionErr != nil {
		return db.transactionErr
	}
	if fn == nil {
		return nil
	}
	db.transactionFnCalled = true
	tx := &core.Tx{}
	tx.SetDB(db)
	return fn(tx)
}

func (db *fakeDB) Migrate() error { return nil }

func (db *fakeDB) AutoMigrate(models ...any) error { return nil }

func (db *fakeDB) Close() error { return nil }

func (db *fakeDB) WithContext(ctx context.Context) core.DB { return db }

type fakeQuery struct {
	fillAllCount int

	firstErr error
	allErr   error

	allPaginatedResult *core.PaginatedResult
	allPaginatedErr    error

	count    int64
	countErr error

	createErr          error
	createOrUpdateErr  error
	updateErr          error
	deleteErr          error
	scanErr            error
	scanAllSegmentsErr error

	batchGetErr            error
	batchGetWithOptionsErr error
	batchCreateErr         error
	batchDeleteErr         error
	batchWriteErr          error
	batchUpdateErr         error

	setCursorErr error

	updateBuilder   core.UpdateBuilder
	batchGetBuilder core.BatchGetBuilder
}

func (q *fakeQuery) Where(field string, op string, value any) core.Query    { return q }
func (q *fakeQuery) Index(indexName string) core.Query                      { return q }
func (q *fakeQuery) Filter(field string, op string, value any) core.Query   { return q }
func (q *fakeQuery) OrFilter(field string, op string, value any) core.Query { return q }
func (q *fakeQuery) FilterGroup(fn func(core.Query)) core.Query             { return q }
func (q *fakeQuery) OrFilterGroup(fn func(core.Query)) core.Query           { return q }
func (q *fakeQuery) IfNotExists() core.Query                                { return q }
func (q *fakeQuery) IfExists() core.Query                                   { return q }
func (q *fakeQuery) WithCondition(field, operator string, value any) core.Query {
	return q
}
func (q *fakeQuery) WithConditionExpression(expr string, values map[string]any) core.Query {
	return q
}
func (q *fakeQuery) OrderBy(field string, order string) core.Query { return q }
func (q *fakeQuery) Limit(limit int) core.Query                    { return q }
func (q *fakeQuery) Offset(offset int) core.Query                  { return q }
func (q *fakeQuery) Select(fields ...string) core.Query            { return q }
func (q *fakeQuery) ConsistentRead() core.Query                    { return q }
func (q *fakeQuery) WithRetry(maxRetries int, initialDelay time.Duration) core.Query {
	return q
}
func (q *fakeQuery) First(dest any) error { return q.firstErr }

func (q *fakeQuery) All(dest any) error {
	if q.allErr != nil {
		return q.allErr
	}
	if q.fillAllCount <= 0 {
		return nil
	}

	rv := reflect.ValueOf(dest)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return nil
	}
	ev := rv.Elem()
	if ev.Kind() != reflect.Slice {
		return nil
	}

	ev.Set(reflect.MakeSlice(ev.Type(), q.fillAllCount, q.fillAllCount))
	return nil
}

func (q *fakeQuery) AllPaginated(dest any) (*core.PaginatedResult, error) {
	return q.allPaginatedResult, q.allPaginatedErr
}

func (q *fakeQuery) Count() (int64, error) { return q.count, q.countErr }
func (q *fakeQuery) Create() error         { return q.createErr }
func (q *fakeQuery) CreateOrUpdate() error { return q.createOrUpdateErr }
func (q *fakeQuery) Update(fields ...string) error {
	return q.updateErr
}
func (q *fakeQuery) UpdateBuilder() core.UpdateBuilder { return q.updateBuilder }
func (q *fakeQuery) Delete() error                     { return q.deleteErr }
func (q *fakeQuery) Scan(dest any) error               { return q.scanErr }
func (q *fakeQuery) ParallelScan(segment int32, totalSegments int32) core.Query {
	return q
}
func (q *fakeQuery) ScanAllSegments(dest any, totalSegments int32) error { return q.scanAllSegmentsErr }
func (q *fakeQuery) BatchGet(keys []any, dest any) error                 { return q.batchGetErr }
func (q *fakeQuery) BatchGetWithOptions(keys []any, dest any, opts *core.BatchGetOptions) error {
	return q.batchGetWithOptionsErr
}
func (q *fakeQuery) BatchGetBuilder() core.BatchGetBuilder { return q.batchGetBuilder }
func (q *fakeQuery) BatchCreate(items any) error           { return q.batchCreateErr }
func (q *fakeQuery) BatchDelete(keys []any) error          { return q.batchDeleteErr }
func (q *fakeQuery) BatchWrite(putItems []any, deleteKeys []any) error {
	return q.batchWriteErr
}
func (q *fakeQuery) BatchUpdateWithOptions(items []any, fields []string, options ...any) error {
	return q.batchUpdateErr
}
func (q *fakeQuery) Cursor(cursor string) core.Query { return q }
func (q *fakeQuery) SetCursor(cursor string) error   { return q.setCursorErr }
func (q *fakeQuery) WithContext(ctx context.Context) core.Query {
	return q
}

type fakeUpdateBuilder struct{}

func (b *fakeUpdateBuilder) Set(field string, value any) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) Add(field string, value any) core.UpdateBuilder           { return b }
func (b *fakeUpdateBuilder) Increment(field string) core.UpdateBuilder                { return b }
func (b *fakeUpdateBuilder) Decrement(field string) core.UpdateBuilder                { return b }
func (b *fakeUpdateBuilder) Remove(field string) core.UpdateBuilder                   { return b }
func (b *fakeUpdateBuilder) Delete(field string, value any) core.UpdateBuilder        { return b }
func (b *fakeUpdateBuilder) AppendToList(field string, values any) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) PrependToList(field string, values any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) RemoveFromListAt(field string, index int) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) SetListElement(field string, index int, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) Condition(field string, operator string, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) OrCondition(field string, operator string, value any) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) ConditionExists(field string) core.UpdateBuilder    { return b }
func (b *fakeUpdateBuilder) ConditionNotExists(field string) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) ConditionVersion(currentVersion int64) core.UpdateBuilder {
	return b
}
func (b *fakeUpdateBuilder) ReturnValues(option string) core.UpdateBuilder { return b }
func (b *fakeUpdateBuilder) Execute() error                                { return nil }
func (b *fakeUpdateBuilder) ExecuteWithResult(result any) error            { return nil }

type fakeBatchGetBuilder struct {
	executeErr error
}

func (b *fakeBatchGetBuilder) Keys(keys []any) core.BatchGetBuilder { return b }
func (b *fakeBatchGetBuilder) ChunkSize(size int) core.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) ConsistentRead() core.BatchGetBuilder             { return b }
func (b *fakeBatchGetBuilder) Parallel(maxConcurrency int) core.BatchGetBuilder { return b }
func (b *fakeBatchGetBuilder) WithRetry(policy *core.RetryPolicy) core.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) Select(fields ...string) core.BatchGetBuilder { return b }
func (b *fakeBatchGetBuilder) OnProgress(callback core.BatchProgressCallback) core.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) OnError(handler core.BatchChunkErrorHandler) core.BatchGetBuilder {
	return b
}
func (b *fakeBatchGetBuilder) Execute(dest any) error { return b.executeErr }

type modelWithTableName struct{}

func (modelWithTableName) TableName() string { return "round15_table" }

func TestTrackingDB_ModelAndQueryTracking_Round15Coverage(t *testing.T) {
	t.Parallel()

	logger := zap.NewNop()
	tracker := New()
	tracker.circuitBreaker = nil

	fq := &fakeQuery{
		fillAllCount:           2,
		allPaginatedResult:     &core.PaginatedResult{Count: 3},
		updateBuilder:          &fakeUpdateBuilder{},
		batchGetBuilder:        &fakeBatchGetBuilder{},
		scanAllSegmentsErr:     nil,
		batchGetWithOptionsErr: nil,
	}
	db := &fakeDB{query: fq}
	trackingDB := NewTrackingDB(db, tracker, logger)

	q := trackingDB.Model(&modelWithTableName{})
	tq, ok := q.(*TrackingQuery)
	require.True(t, ok)
	require.NotNil(t, tq.enhancedTracker)
	require.NotNil(t, tq.operationMetadata)
	require.Equal(t, "round15_table", tq.operationMetadata.TableName)

	// Cover chaining methods + metadata mutation.
	q = q.
		Where("pk", "=", "1").
		Index("gsi1").
		Filter("status", "=", "active").
		OrFilter("x", "=", 1).
		FilterGroup(func(core.Query) {}).
		OrFilterGroup(func(core.Query) {}).
		IfNotExists().
		IfExists().
		WithCondition("a", "=", "b").
		WithConditionExpression("a = :a", map[string]any{":a": "b"}).
		OrderBy("created_at", "desc").
		Limit(10).
		Offset(5).
		Select("a", "b").
		ConsistentRead().
		WithRetry(1, time.Millisecond).
		Cursor("cur").
		WithContext(context.Background()).
		ParallelScan(1, 2)

	tq = q.(*TrackingQuery)
	require.Equal(t, "gsi1", tq.operationMetadata.IndexName)
	require.True(t, tq.operationMetadata.ConsistentRead)
	require.Len(t, tq.operationMetadata.Conditions, 1)
	require.Len(t, tq.operationMetadata.FilterExpressions, 1)
	require.ElementsMatch(t, []string{"a", "b"}, tq.operationMetadata.ProjectionFields)

	// Cursor setter passthrough.
	require.NoError(t, q.SetCursor("cur2"))
	require.NotNil(t, q.UpdateBuilder())

	// Terminal methods (success paths) update read/write counters.
	var allDest []string
	require.NoError(t, q.First(&struct{}{})) // ConsistentRead => 2 read units
	require.NoError(t, q.All(&allDest))      // 2 items + ConsistentRead => 4 read units

	_, err := q.AllPaginated(&allDest) // Count=3 => 3 read units
	require.NoError(t, err)

	_, err = q.Count() // 1 read unit
	require.NoError(t, err)

	require.NoError(t, q.Create())         // 1 write
	require.NoError(t, q.CreateOrUpdate()) // 1 write
	require.NoError(t, q.Update("a"))      // 1 write
	require.NoError(t, q.Delete())         // 1 write

	require.NoError(t, q.Scan(&allDest))               // 100 read units
	require.NoError(t, q.ScanAllSegments(&allDest, 2)) // 200 read units

	require.NoError(t, q.BatchGet([]any{"k1", "k2"}, &allDest))                               // 2 reads
	require.NoError(t, q.BatchGetWithOptions([]any{"k3"}, &allDest, &core.BatchGetOptions{})) // 1 read

	require.NoError(t, q.BatchCreate(make([]any, 26)))                                   // 26 writes (batch_count=2)
	require.NoError(t, q.BatchDelete([]any{"d1", "d2"}))                                 // 2 writes
	require.NoError(t, q.BatchWrite([]any{"p1"}, []any{"d3", "d4"}))                     // 3 writes
	require.NoError(t, q.BatchUpdateWithOptions([]any{"u1", "u2", "u3"}, []string{"a"})) // 3 writes

	builder := q.BatchGetBuilder()
	require.NoError(t, builder.
		Keys([]any{"k1", "k2", "k3"}).
		ChunkSize(1).
		ConsistentRead().
		Parallel(2).
		WithRetry(nil).
		Select("a").
		OnProgress(func(retrieved, total int) {}).
		OnError(func(chunk []any, err error) error { return err }).
		Execute(&allDest),
	)

	// Reads: First(2) + All(4) + AllPaginated(3) + Count(1) + Scan(100) + ScanAllSegments(200) +
	// BatchGet(2) + BatchGetWithOptions(1) + Builder(3) = 316
	require.Equal(t, int64(316), tracker.dynamoReads.Load())

	// Writes: Create(1) + CreateOrUpdate(1) + Update(1) + Delete(1) + BatchCreate(26) + BatchDelete(2) +
	// BatchWrite(3) + BatchUpdate(3) = 38
	require.Equal(t, int64(38), tracker.dynamoWrites.Load())

	require.NotEmpty(t, tq.enhancedTracker.GetAllOperations())
}

func TestTrackingQuery_All_ScanFilterMultiplier_Round15Coverage(t *testing.T) {
	t.Parallel()

	tracker := New()
	tracker.circuitBreaker = nil

	fq := &fakeQuery{fillAllCount: 2, updateBuilder: &fakeUpdateBuilder{}, batchGetBuilder: &fakeBatchGetBuilder{}}
	db := &fakeDB{query: fq}
	q := NewTrackingDB(db, tracker, zap.NewNop()).Model(struct{ Name string }{})

	// No index => scan. Add a filter expression to trigger the 1.5x multiplier.
	q = q.Filter("status", "=", "active")

	var dest []string
	require.NoError(t, q.All(&dest))

	// itemCount=2 => base 2, scan+filter => 3 reads (int(2*1.5)).
	require.Equal(t, int64(3), tracker.dynamoReads.Load())
}

func TestDynamORMCostTracker_ContextTrackingFailuresAreNonFatal_Round15Coverage(t *testing.T) {
	t.Parallel()

	ct := NewDynamORMCostTracker(&MockDB{}, zap.NewNop())

	contextTracker := New()
	contextTracker.circuitBreaker = NewCostCircuitBreaker(CostCircuitBreakerConfig{
		MaxCostPerHour:    0,
		MaxCostPerRequest: 0,
		WindowSize:        time.Hour,
		FailureThreshold:  1,
		RecoveryTimeout:   time.Millisecond,
	})

	ctx := WithTracker(context.Background(), contextTracker)
	err := ct.TrackOperation(ctx, "op", func() error {
		// Bypass circuit breaker so we can create a non-zero delta.
		ct.dynamoReads.Add(1)
		ct.dynamoWrites.Add(1)
		return nil
	})
	require.NoError(t, err)
}

func TestTrackingDB_TransactionTrackingAndWarnings_Round15Coverage(t *testing.T) {
	t.Parallel()

	db := &fakeDB{query: &fakeQuery{}}

	tracker := New()
	tracker.circuitBreaker = nil
	logger := zap.NewNop()

	ctdb := NewTrackingDB(db, tracker, logger)

	require.NoError(t, ctdb.Transaction(func(tx *core.Tx) error { return nil }))
	require.True(t, db.transactionFnCalled)
	require.Equal(t, int64(1), tracker.dynamoReads.Load())
	require.Equal(t, int64(3), tracker.dynamoWrites.Load())

	// Force tracking errors so warn paths are covered (Transaction still succeeds).
	errTracker := New()
	errTracker.circuitBreaker = NewCostCircuitBreaker(CostCircuitBreakerConfig{
		MaxCostPerHour:    0,
		MaxCostPerRequest: 0,
		WindowSize:        time.Hour,
		FailureThreshold:  1,
		RecoveryTimeout:   time.Millisecond,
	})

	require.NoError(t, NewTrackingDB(db, errTracker, logger).Transaction(func(tx *core.Tx) error { return nil }))
	require.NoError(t, NewTrackingDB(db, tracker, nil).Transaction(func(tx *core.Tx) error { return nil }))

	db.transactionErr = errors.New("tx failed")
	require.Error(t, NewTrackingDB(db, tracker, logger).Transaction(func(tx *core.Tx) error { return nil }))
}

func TestEnhancedOperationTrackerAndHelpers_Round15Coverage(t *testing.T) {
	t.Parallel()

	eot := NewEnhancedOperationTracker(nil)
	meta := &OperationMetadata{OperationType: "GetItem", TableName: "t"}
	eot.TrackOperation("op1", meta)

	got, ok := eot.GetOperationMetadata("op1")
	require.True(t, ok)
	require.Equal(t, meta, got)

	all := eot.GetAllOperations()
	require.Len(t, all, 1)
	delete(all, "op1")
	_, stillThere := eot.GetOperationMetadata("op1")
	require.True(t, stillThere)

	eot.ClearOperations()
	require.Empty(t, eot.GetAllOperations())

	// Helpers: countResultItems
	var nilSlice []string
	require.Equal(t, 0, countResultItems(nil))
	require.Equal(t, 0, countResultItems((*[]string)(nil)))
	require.Equal(t, 0, countResultItems(&nilSlice))
	require.Equal(t, 2, countResultItems(&[]string{"a", "b"}))
	require.Equal(t, 1, countResultItems(struct{ A string }{A: "x"}))
	require.Equal(t, 2, countResultItems(map[string]int{"a": 1, "b": 2}))

	var iface any = []int{1, 2, 3}
	require.Equal(t, 3, countResultItems(&iface)) // pointer-to-interface path

	i0 := 0
	i1 := 1
	require.Equal(t, 0, countResultItems(&i0))
	require.Equal(t, 1, countResultItems(&i1))

	// Helpers: countBatchItems
	require.Equal(t, 0, countBatchItems(nil))
	require.Equal(t, 0, countBatchItems((*[]string)(nil)))
	require.Equal(t, 2, countBatchItems([]string{"a", "b"}))

	// Helpers: extractTableName
	type plainModel struct{}

	require.Equal(t, "unknown", extractTableName(nil))
	require.Equal(t, "modelWithTableName", extractTableName(modelWithTableName{}))
	require.Equal(t, "round15_table", extractTableName(&modelWithTableName{}))
	require.Equal(t, "plainModel", extractTableName(plainModel{}))
	require.Equal(t, "plainModel", extractTableName(&plainModel{}))
	require.Equal(t, "modelWithTableName", extractTableName([]modelWithTableName{{}}))
	require.Equal(t, "modelWithTableName", extractTableName([]*modelWithTableName{{}}))
	require.Equal(t, "unknown", extractTableName(123))

	// Cover request tracking helpers.
	NewDynamORMCostTracker(&MockDB{}, zap.NewNop()).TrackComprehendRequest("sentiment", 1)
	NewDynamORMCostTracker(&MockDB{}, nil).TrackTranscribeRequest("job", 1)
}
