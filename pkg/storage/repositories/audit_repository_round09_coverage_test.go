package repositories

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dmerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
	"go.uber.org/zap"
)

func TestAuditRepository_StoreAuditLog_ValidatesCriticalFields(t *testing.T) {
	mockDB, _ := newMockDBQuery()
	repo := NewAuditRepository(mockDB, "tbl", zap.NewNop(), nil)

	err := repo.StoreAuditLog(context.Background(), &models.AuthAuditLog{ID: "", EventType: ""})
	require.Error(t, err)

	err = repo.StoreAuditLog(context.Background(), &models.AuthAuditLog{ID: "evt-1", EventType: ""})
	require.Error(t, err)
}

func TestAuditRepository_GetAuditLogByID_ValidationAndNotFound(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	_, err := repo.GetAuditLogByID(context.Background(), "", time.Now())
	require.Error(t, err)

	_, err = repo.GetAuditLogByID(context.Background(), "evt", time.Time{})
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		// return empty
		v := reflect.ValueOf(args.Get(0))
		if v.Kind() == reflect.Ptr && v.Elem().Kind() == reflect.Slice {
			v.Elem().Set(reflect.MakeSlice(v.Elem().Type(), 0, 0))
		}
	}).Return(nil).Once()

	_, err = repo.GetAuditLogByID(context.Background(), "evt-1", time.Now())
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_GetUserAuditLogs_UsesHelperQuery(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	_, err := repo.GetUserAuditLogs(context.Background(), "", 10, time.Now().Add(-time.Hour), time.Now())
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		s := reflect.MakeSlice(v.Elem().Type(), 0, 1)
		log := &models.AuthAuditLog{ID: "evt-1", EventType: "auth.login.failed", Success: false}
		s = reflect.Append(s, reflect.ValueOf(log))
		v.Elem().Set(s)
	}).Return(nil).Once()

	logs, err := repo.GetUserAuditLogs(context.Background(), "alice", 10, time.Time{}, time.Time{})
	require.NoError(t, err)
	require.Len(t, logs, 1)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_GetSessionAuditLogs_Paginates(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	call := 0
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		call++
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		slice := reflect.MakeSlice(v.Elem().Type(), 0, 0)
		if call == 1 {
			slice = reflect.Append(slice, reflect.ValueOf(&models.AuthAuditLog{ID: "evt-1"}))
			slice = reflect.Append(slice, reflect.ValueOf(&models.AuthAuditLog{ID: "evt-2"}))
		}
		v.Elem().Set(slice)
	}).Return(nil).Maybe()

	_, err := repo.GetSessionAuditLogs(context.Background(), "")
	require.Error(t, err)

	logs, err := repo.GetSessionAuditLogs(context.Background(), "sess-1")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(logs), 2)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_GetSecurityEvents_BoundsCursorAndNextCursor(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	_, _, err := repo.GetSecurityEvents(context.Background(), "", time.Time{}, time.Time{}, 10, "")
	require.Error(t, err)
	_, _, err = repo.GetSecurityEvents(context.Background(), "NOPE", time.Time{}, time.Time{}, 10, "")
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		// []models.AuthAuditLog (value slice)
		slice := reflect.MakeSlice(v.Elem().Type(), 0, 3)
		slice = reflect.Append(slice, reflect.ValueOf(models.AuthAuditLog{GSI4SK: "AUDIT#1"}))
		slice = reflect.Append(slice, reflect.ValueOf(models.AuthAuditLog{GSI4SK: "AUDIT#2"}))
		slice = reflect.Append(slice, reflect.ValueOf(models.AuthAuditLog{GSI4SK: "AUDIT#3"}))
		v.Elem().Set(slice)
	}).Return(nil).Once()

	events, next, err := repo.GetSecurityEvents(context.Background(), "HIGH", time.Time{}, time.Time{}, 2, "AUDIT#0")
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.NotEmpty(t, next)

	// Query error path
	mockQuery.On("All", mock.Anything).Return(dmerrors.NewError("Query", "AuthAuditLog", errors.New("boom"))).Once()
	_, _, err = repo.GetSecurityEvents(context.Background(), "LOW", time.Now().Add(-time.Hour), time.Now(), 1, "")
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_RecentFailedAndIPFailures_Counts(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		slice := reflect.MakeSlice(v.Elem().Type(), 0, 3)
		slice = reflect.Append(slice, reflect.ValueOf(&models.AuthAuditLog{EventType: "auth.login.failed", Success: false}))
		slice = reflect.Append(slice, reflect.ValueOf(&models.AuthAuditLog{EventType: "auth.login.failed", Success: true}))
		slice = reflect.Append(slice, reflect.ValueOf(&models.AuthAuditLog{EventType: "other", Success: false}))
		v.Elem().Set(slice)
	}).Return(nil).Maybe()

	n, err := repo.GetRecentFailedLogins(context.Background(), "alice", 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	n, err = repo.GetRecentIPFailures(context.Background(), "1.2.3.4", 10*time.Minute)
	require.NoError(t, err)
	require.Equal(t, 2, n)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_StoreAndGetAndQuerySuccessPaths(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	mockQuery.On("Create").Return(nil).Once()
	now := time.Now().UTC()
	entry := &models.AuthAuditLog{
		ID:        "evt-1",
		EventType: "auth.login",
		Severity:  "LOW",
		Username:  "alice",
		IPAddress: "1.2.3.4",
		Timestamp: now,
		Success:   true,
	}
	err := repo.StoreAuditLog(context.Background(), entry)
	require.NoError(t, err)

	// GetAuditLogByID success path
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if logs, ok := dst.(*[]*models.AuthAuditLog); ok {
			*logs = []*models.AuthAuditLog{{ID: "evt-1"}}
		}
	}).Return(nil).Once()
	got, err := repo.GetAuditLogByID(context.Background(), "evt-1", now)
	require.NoError(t, err)
	require.Equal(t, "evt-1", got.ID)

	// IP and user queries with time range + limit paths
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if logs, ok := dst.(*[]*models.AuthAuditLog); ok {
			*logs = []*models.AuthAuditLog{{ID: "evt-2"}, {ID: "evt-3"}}
		}
	}).Return(nil).Twice()
	_, err = repo.GetIPAuditLogs(context.Background(), "1.2.3.4", 10, now.Add(-time.Hour), now)
	require.NoError(t, err)
	_, err = repo.GetUserAuditLogs(context.Background(), "alice", 10, now.Add(-time.Hour), now)
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_GetSecurityEvents_LimitBounds(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if logs, ok := dst.(*[]models.AuthAuditLog); ok {
			*logs = []models.AuthAuditLog{{GSI4SK: "AUDIT#1"}}
		}
	}).Return(nil).Twice()

	events, _, err := repo.GetSecurityEvents(context.Background(), "MEDIUM", time.Now().Add(-time.Hour), time.Now(), 0, "")
	require.NoError(t, err)
	require.Len(t, events, 1)

	events, _, err = repo.GetSecurityEvents(context.Background(), "CRITICAL", time.Time{}, time.Time{}, 5000, "")
	require.NoError(t, err)
	require.Len(t, events, 1)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_CleanupOldLogs_ValidationAndBatchDeleteBranches(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	err := repo.CleanupOldLogs(context.Background(), 0)
	require.Error(t, err)
	err = repo.CleanupOldLogs(context.Background(), 366)
	require.Error(t, err)

	// FindWithPagination -> All returns one item, BatchDelete fails (but cleanup continues)
	created := &models.AuthAuditLog{ID: "evt", EventType: "x", Timestamp: time.Now().AddDate(0, 0, -2), IPAddress: "1.2.3.4"}
	_ = created.UpdateKeys()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if logs, ok := args.Get(0).(*[]*models.AuthAuditLog); ok {
			*logs = []*models.AuthAuditLog{created}
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(errors.New("boom")).Once()
	err = repo.CleanupOldLogs(context.Background(), 1)
	require.NoError(t, err)

	// Success delete path
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		if logs, ok := args.Get(0).(*[]*models.AuthAuditLog); ok {
			*logs = []*models.AuthAuditLog{created}
		}
	}).Return(nil).Once()
	mockQuery.On("Delete").Return(nil).Once()
	err = repo.CleanupOldLogs(context.Background(), 1)
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestAuditRepository_StoreAuditEvent_CoversMarshalAndIDGeneration(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := &AuditRepository{EnhancedBaseRepository: NewEnhancedBaseRepository[*models.AuthAuditLog](mockDB, "tbl", zap.NewNop(), nil, "AuditRepository", "audit")}

	mockQuery.On("Create").Return(nil).Twice()
	err := repo.StoreAuditEvent(context.Background(),
		"auth.login", "LOW", "alice", "u1", "1.2.3.4", "ua", "device", "sess", "req", true, "",
		map[string]interface{}{"bad": func() {}}, // json.Marshal failure branch
	)
	require.NoError(t, err)

	err = repo.StoreAuditEvent(context.Background(),
		"auth.logout", "LOW", "alice", "u1", "1.2.3.4", "ua", "device", "sess", "req", true, "", nil,
	)
	require.NoError(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
