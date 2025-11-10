package repositories

import (
	"context"
	"sort"
	"testing"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/equaltoai/lesser/pkg/storage/models"
)

type testBookmarkRepository struct {
	*BookmarkRepository
	store         map[string]*models.Bookmark
	batchGetCalls int
}

func newTestBookmarkRepository() *testBookmarkRepository {
	base := &BookmarkRepository{
		EnhancedBaseRepository: &EnhancedBaseRepository[*models.Bookmark]{
			BaseRepository: &BaseRepository[*models.Bookmark]{
				logger: zap.NewNop(),
			},
		},
	}
	base.initHooks()

	repo := &testBookmarkRepository{
		BookmarkRepository: base,
		store:              make(map[string]*models.Bookmark),
	}
	repo.overrideHooks()
	return repo
}

func (r *testBookmarkRepository) overrideHooks() {
	r.transactWriteFn = r.mockTransactWrite
	r.batchGetFn = r.mockBatchGet
	r.queryTimeBookmarksFn = r.mockQueryTimeBookmarks
	r.getObjectBookmarkFn = func(_ context.Context, username, objectID string) (*models.Bookmark, error) {
		key := r.makeKey(buildBookmarkPK(username), buildObjectSK(objectID))
		if bookmark, ok := r.store[key]; ok {
			return r.clone(bookmark), nil
		}
		return nil, dynamormerrors.ErrItemNotFound
	}
	r.findTimeBookmarkFn = func(_ context.Context, username, objectID string) (*models.Bookmark, error) {
		pk := buildBookmarkPK(username)
		for _, bookmark := range r.store {
			if bookmark.PK == pk && bookmark.RecordType == models.BookmarkRecordTypeTime &&
				bookmark.ObjectID == objectID {
				return r.clone(bookmark), nil
			}
		}
		return nil, nil
	}
}

func (r *testBookmarkRepository) makeKey(pk, sk string) string {
	return pk + "|" + sk
}

func (r *testBookmarkRepository) clone(b *models.Bookmark) *models.Bookmark {
	if b == nil {
		return nil
	}
	clone := *b
	return &clone
}

func (r *testBookmarkRepository) cloneStore() map[string]*models.Bookmark {
	dup := make(map[string]*models.Bookmark, len(r.store))
	for k, v := range r.store {
		dup[k] = r.clone(v)
	}
	return dup
}

func (r *testBookmarkRepository) mockTransactWrite(ctx context.Context, fn func(core.TransactionBuilder) error) error {
	builder := newMockTransactionBuilder(r)
	if err := fn(builder); err != nil {
		return err
	}
	return builder.apply()
}

func (r *testBookmarkRepository) mockBatchGet(_ context.Context, keys []any) ([]*models.Bookmark, error) {
	r.batchGetCalls++
	results := make([]*models.Bookmark, 0, len(keys))
	for _, raw := range keys {
		pair, ok := raw.(core.KeyPair)
		if !ok {
			continue
		}
		pk, _ := pair.PartitionKey.(string)
		sk, _ := pair.SortKey.(string)
		if bookmark, ok := r.store[r.makeKey(pk, sk)]; ok {
			results = append(results, r.clone(bookmark))
		}
	}
	return results, nil
}

func (r *testBookmarkRepository) mockQueryTimeBookmarks(_ context.Context, username string, limit int, cursor string) ([]models.Bookmark, string, error) {
	pk := buildBookmarkPK(username)
	all := make([]models.Bookmark, 0)
	for _, bookmark := range r.store {
		if bookmark.PK == pk && bookmark.RecordType == models.BookmarkRecordTypeTime && !bookmark.Locked {
			all = append(all, *r.clone(bookmark))
		}
	}

	sort.Slice(all, func(i, j int) bool {
		return all[i].SK > all[j].SK
	})

	start := 0
	if cursor != "" {
		for idx, item := range all {
			if item.SK == cursor {
				start = idx + 1
				break
			}
		}
	}

	if start > len(all) {
		start = len(all)
	}

	end := start + limit
	if end > len(all) {
		end = len(all)
	}

	window := all[start:end]
	nextCursor := ""
	if end < len(all) {
		nextCursor = all[end].SK
	}

	return window, nextCursor, nil
}

// --- Tests -----------------------------------------------------------------

func TestBookmarkRepositoryCreateDualWrite(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	bookmark, err := repo.CreateBookmark(ctx, "alice", "status-1")
	require.NoError(t, err)
	require.Equal(t, "OBJECT#status-1", bookmark.SK)
	require.NotEmpty(t, bookmark.TimeRecordSK)

	timeKey := repo.makeKey(buildBookmarkPK("alice"), bookmark.TimeRecordSK)
	timeRecord, ok := repo.store[timeKey]
	require.True(t, ok, "time record missing")
	require.False(t, timeRecord.Locked, "time record should be unlocked after create")
	require.Equal(t, models.BookmarkRecordTypeTime, timeRecord.RecordType)

	objectKey := repo.makeKey(buildBookmarkPK("alice"), bookmark.SK)
	objectRecord, ok := repo.store[objectKey]
	require.True(t, ok, "object record missing")
	require.Equal(t, bookmark.TimeRecordSK, objectRecord.TimeRecordSK)
}

func TestBookmarkRepositoryCreateDuplicateIsIdempotent(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	first, err := repo.CreateBookmark(ctx, "alice", "status-2")
	require.NoError(t, err)

	second, err := repo.CreateBookmark(ctx, "alice", "status-2")
	require.NoError(t, err)
	require.Equal(t, first.TimeRecordSK, second.TimeRecordSK)

	var timeRecords, objectRecords int
	for _, bookmark := range repo.store {
		if bookmark.RecordType == models.BookmarkRecordTypeTime {
			timeRecords++
		}
		if bookmark.RecordType == models.BookmarkRecordTypeObject {
			objectRecords++
		}
	}
	require.Equal(t, 1, timeRecords)
	require.Equal(t, 1, objectRecords)
}

func TestBookmarkRepositoryCreateRepairsLegacyTimeRecord(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	legacy, err := models.NewTimeOrderedBookmark("charlie", "status-4", time.Now().UTC())
	require.NoError(t, err)
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	bookmark, err := repo.CreateBookmark(ctx, "charlie", "status-4")
	require.NoError(t, err)
	require.Equal(t, legacy.SK, bookmark.TimeRecordSK)

	timeRecord := repo.store[repo.makeKey(legacy.PK, legacy.SK)]
	require.False(t, timeRecord.Locked)
}

func TestBookmarkRepositoryDeleteRemovesBothRecords(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	_, err := repo.CreateBookmark(ctx, "bob", "status-3")
	require.NoError(t, err)

	err = repo.DeleteBookmark(ctx, "bob", "status-3")
	require.NoError(t, err)
	require.Empty(t, repo.store)
}

func TestBookmarkRepositoryDeleteLegacyFallback(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	legacy, err := models.NewTimeOrderedBookmark("dana", "status-5", time.Now().UTC())
	require.NoError(t, err)
	legacy.Locked = false
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	err = repo.DeleteBookmark(ctx, "dana", "status-5")
	require.NoError(t, err)
	require.Empty(t, repo.store)
}

func TestBookmarkRepositoryCheckBookmarksForStatuses(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	ids := []string{"s1", "s2", "s3"}
	for _, id := range ids[:2] {
		_, err := repo.CreateBookmark(ctx, "carol", id)
		require.NoError(t, err)
	}

	legacy, err := models.NewTimeOrderedBookmark("carol", ids[2], time.Now().UTC())
	require.NoError(t, err)
	legacy.Locked = false
	repo.store[repo.makeKey(legacy.PK, legacy.SK)] = legacy

	statusIDs := append([]string{}, ids...)
	statusIDs = append(statusIDs, "missing-1", "missing-2")

	results, err := repo.CheckBookmarksForStatuses(ctx, "carol", statusIDs)
	require.NoError(t, err)
	require.Len(t, results, len(ids))
	for _, id := range ids[:2] {
		require.True(t, results[id])
	}
	require.True(t, results[ids[2]], "legacy fallback should report bookmark")
	require.Equal(t, 1, repo.batchGetCalls)
}

func TestBookmarkRepositoryGetUserBookmarksSkipsLocked(t *testing.T) {
	ctx := context.Background()
	repo := newTestBookmarkRepository()

	first, err := repo.CreateBookmark(ctx, "eve", "status-6")
	require.NoError(t, err)

	secondTime, err := models.NewTimeOrderedBookmark("eve", "status-7", time.Now().UTC())
	require.NoError(t, err)
	repo.store[repo.makeKey(secondTime.PK, secondTime.SK)] = secondTime

	bookmarks, _, err := repo.GetUserBookmarks(ctx, "eve", 10, "")
	require.NoError(t, err)
	require.Len(t, bookmarks, 1)
	require.Equal(t, first.ObjectID, bookmarks[0].ObjectID)
}

type mockTransactionBuilder struct {
	repo *testBookmarkRepository
	ops  []func() error
}

func newMockTransactionBuilder(repo *testBookmarkRepository) *mockTransactionBuilder {
	return &mockTransactionBuilder{
		repo: repo,
		ops:  make([]func() error, 0, 4),
	}
}

func (b *mockTransactionBuilder) apply() error {
	snapshot := b.repo.cloneStore()
	for _, op := range b.ops {
		if err := op(); err != nil {
			b.repo.store = snapshot
			return err
		}
	}
	return nil
}

func (b *mockTransactionBuilder) Create(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	bookmark := model.(*models.Bookmark)
	copy := b.repo.clone(bookmark)
	b.ops = append(b.ops, func() error {
		key := b.repo.makeKey(copy.PK, copy.SK)
		if err := b.evaluateConditions(key, conditions); err != nil {
			return err
		}
		if _, exists := b.repo.store[key]; exists {
			return dynamormerrors.ErrConditionFailed
		}
		b.repo.store[key] = copy
		return nil
	})
	return b
}

func (b *mockTransactionBuilder) Update(model any, fields []string, conditions ...core.TransactCondition) core.TransactionBuilder {
	panic("Update not implemented in mock builder")
}

func (b *mockTransactionBuilder) UpdateWithBuilder(model any, updateFn func(core.UpdateBuilder) error, conditions ...core.TransactCondition) core.TransactionBuilder {
	keyBookmark := model.(*models.Bookmark)
	key := b.repo.makeKey(keyBookmark.PK, keyBookmark.SK)
	builder := newMockUpdateBuilder()
	if err := updateFn(builder); err != nil {
		b.ops = append(b.ops, func() error { return err })
		return b
	}
	b.ops = append(b.ops, func() error {
		if err := b.evaluateConditions(key, conditions); err != nil {
			return err
		}
		target, ok := b.repo.store[key]
		if !ok {
			return dynamormerrors.ErrConditionFailed
		}
		for field, value := range builder.setOps {
			switch field {
			case "Locked":
				if boolVal, ok := value.(bool); ok {
					target.Locked = boolVal
				}
			}
		}
		return nil
	})
	return b
}

func (b *mockTransactionBuilder) Delete(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	keyBookmark := model.(*models.Bookmark)
	key := b.repo.makeKey(keyBookmark.PK, keyBookmark.SK)
	b.ops = append(b.ops, func() error {
		if err := b.evaluateConditions(key, conditions); err != nil {
			return err
		}
		delete(b.repo.store, key)
		return nil
	})
	return b
}

func (b *mockTransactionBuilder) Put(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	panic("Put not implemented in mock builder")
}

func (b *mockTransactionBuilder) ConditionCheck(model any, conditions ...core.TransactCondition) core.TransactionBuilder {
	panic("ConditionCheck not implemented in mock builder")
}

func (b *mockTransactionBuilder) WithContext(context.Context) core.TransactionBuilder {
	return b
}

func (b *mockTransactionBuilder) Execute() error {
	return b.apply()
}

func (b *mockTransactionBuilder) ExecuteWithContext(context.Context) error {
	return b.apply()
}

func (b *mockTransactionBuilder) evaluateConditions(key string, conditions []core.TransactCondition) error {
	current, exists := b.repo.store[key]
	for _, condition := range conditions {
		switch condition.Kind {
		case core.TransactConditionKindPrimaryKeyNotExists:
			if exists {
				return dynamormerrors.ErrConditionFailed
			}
		case core.TransactConditionKindPrimaryKeyExists:
			if !exists {
				return dynamormerrors.ErrConditionFailed
			}
		case core.TransactConditionKindField:
			if !exists {
				return dynamormerrors.ErrConditionFailed
			}
			switch condition.Field {
			case "Locked":
				expected, _ := condition.Value.(bool)
				if current.Locked != expected {
					return dynamormerrors.ErrConditionFailed
				}
			}
		}
	}
	return nil
}

type mockUpdateBuilder struct {
	setOps map[string]any
}

func newMockUpdateBuilder() *mockUpdateBuilder {
	return &mockUpdateBuilder{
		setOps: make(map[string]any),
	}
}

func (b *mockUpdateBuilder) Set(field string, value any) core.UpdateBuilder {
	b.setOps[field] = value
	return b
}

func (b *mockUpdateBuilder) SetIfNotExists(field string, value any, defaultValue any) core.UpdateBuilder {
	if _, exists := b.setOps[field]; !exists {
		b.setOps[field] = value
	}
	return b
}

func (b *mockUpdateBuilder) Add(string, any) core.UpdateBuilder              { return b }
func (b *mockUpdateBuilder) AddAll(string, any) core.UpdateBuilder           { return b }
func (b *mockUpdateBuilder) Increment(string) core.UpdateBuilder             { return b }
func (b *mockUpdateBuilder) Decrement(string) core.UpdateBuilder             { return b }
func (b *mockUpdateBuilder) Remove(string) core.UpdateBuilder                { return b }
func (b *mockUpdateBuilder) Delete(string, any) core.UpdateBuilder           { return b }
func (b *mockUpdateBuilder) AppendToList(string, any) core.UpdateBuilder     { return b }
func (b *mockUpdateBuilder) PrependToList(string, any) core.UpdateBuilder    { return b }
func (b *mockUpdateBuilder) RemoveFromListAt(string, int) core.UpdateBuilder { return b }
func (b *mockUpdateBuilder) SetListElement(string, int, any) core.UpdateBuilder {
	return b
}
func (b *mockUpdateBuilder) Condition(string, string, any) core.UpdateBuilder { return b }
func (b *mockUpdateBuilder) OrCondition(string, string, any) core.UpdateBuilder {
	return b
}
func (b *mockUpdateBuilder) ConditionExists(string) core.UpdateBuilder    { return b }
func (b *mockUpdateBuilder) ConditionNotExists(string) core.UpdateBuilder { return b }
func (b *mockUpdateBuilder) ConditionVersion(int64) core.UpdateBuilder    { return b }
func (b *mockUpdateBuilder) ReturnValues(string) core.UpdateBuilder       { return b }
func (b *mockUpdateBuilder) Execute() error                               { return nil }
func (b *mockUpdateBuilder) ExecuteWithResult(any) error                  { return nil }
