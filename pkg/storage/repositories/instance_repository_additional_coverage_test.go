package repositories

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	dynamormmocks "github.com/theory-cloud/tabletheory/v3/pkg/mocks"
	"go.uber.org/zap"
)

func TestInstanceRepository_GetInstanceState_DefaultWhenMissingAndCaches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q)
	q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound)

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	state, err := repo.GetInstanceState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.True(t, state.Locked)

	// Cached fast path
	state2, err := repo.GetInstanceState(ctx)
	require.NoError(t, err)
	assert.Equal(t, state, state2)
}

func TestInstanceRepository_EnsureInstanceState_AndSetters(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	var (
		lastModel any
		stored    = models.NewDefaultInstanceState()
	)
	db.On("Model", mock.Anything).Run(func(args mock.Arguments) {
		lastModel = args.Get(0)
	}).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	// First call to EnsureInstanceState reads missing state -> create.
	q.On("First", mock.AnythingOfType("*models.InstanceState")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceState")).Run(func(args mock.Arguments) {
		state := args.Get(0).(*models.InstanceState)
		*state = *stored
	}).Return(nil).Maybe()

	q.On("Create").Run(func(_ mock.Arguments) {
		if s, ok := lastModel.(*models.InstanceState); ok {
			*stored = *s
		}
	}).Return(nil).Maybe()

	q.On("Update", mock.Anything).Run(func(_ mock.Arguments) {
		if s, ok := lastModel.(*models.InstanceState); ok {
			*stored = *s
		}
	}).Return(nil).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	state, err := repo.EnsureInstanceState(ctx)
	require.NoError(t, err)
	require.NotNil(t, state)

	require.NoError(t, repo.SetInstanceLocked(ctx, false))
	require.NoError(t, repo.SetBootstrapWalletAddress(ctx, " 0xAbC "))
	require.NoError(t, repo.SetPrimaryAdminUsername(ctx, " Alice "))

	cached, ok := repo.getCachedState()
	require.True(t, ok)
	assert.False(t, cached.Locked)
	assert.Equal(t, "0xabc", cached.BootstrapWalletAddress)
	assert.Equal(t, "Alice", cached.PrimaryAdminUsername)
}

func TestInstanceRepository_RulesAndDescriptions_SanitizeAndFilter(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		if cfg, ok := args.Get(0).(*models.InstanceConfig); ok {
			// Provide rules + an extended description in one config struct as needed by different getters.
			cfg.RulesJSON = `[{"id":"1","text":"No spam or solicitation."},{"id":"2","text":"Be respectful."}]`
			cfg.ExtendedDescription = `<script>alert(1)</script> javascript: on=click`
			cfg.UpdatedAt = time.Date(2024, 12, 28, 0, 0, 0, 0, time.UTC)
		}
	}).Return(nil).Maybe()

	q.On("Create").Return(nil).Maybe()
	q.On("CreateOrUpdate").Return(nil).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	rules, err := repo.GetInstanceRules(ctx)
	require.NoError(t, err)
	require.Len(t, rules, 2)

	require.NoError(t, repo.SetInstanceRules(ctx, []storage.InstanceRule{{Text: "A rule without ID"}}))

	desc, updatedAt, err := repo.GetExtendedDescription(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "&lt;script")
	assert.NotEmpty(t, updatedAt)

	require.NoError(t, repo.SetExtendedDescription(ctx, "<script>evil</script>"))

	filtered, err := repo.GetRulesByCategory(ctx, "spam")
	require.NoError(t, err)
	require.NotEmpty(t, filtered)
}

func TestInstanceRepository_MetricsAndHistory_Sweep(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("OrderBy", mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("Limit", mock.Anything).Return(q).Maybe()

	q.On("All", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *[]models.Status:
			*dest = []models.Status{
				{AuthorID: "alice@localhost", Deleted: false},
				{AuthorID: "bob@localhost", Deleted: true},
				{AuthorID: "carol@localhost", Deleted: false},
			}
		case *[]models.User:
			*dest = []models.User{{Username: "admin"}}
		case *[]models.InstanceHistory:
			*dest = []models.InstanceHistory{
				{Value: 10, Date: "2024-12-27"},
				{Value: 12, Date: "2024-12-28"},
			}
		default:
		}
	}).Return(nil).Maybe()

	q.On("First", mock.Anything).Run(func(args mock.Arguments) {
		switch dest := args.Get(0).(type) {
		case *models.InstanceMetrics:
			dest.Value = 123
			dest.TotalUsers = 5
			dest.TotalStatuses = 9
			dest.UpdatedAt = time.Date(2024, 12, 28, 0, 0, 0, 0, time.UTC)
		case *models.Actor:
			dest.PK = "ACTOR#admin"
			dest.SK = "PROFILE"
			dest.Username = "admin"
		case *models.WeeklyActivity:
			dest.Week = 123
			dest.Statuses = 1
			dest.Logins = 2
			dest.Registrations = 3
		case *models.InstanceHistory:
			dest.Value = 9
		default:
		}
	}).Return(nil).Maybe()

	q.On("Create").Return(nil).Maybe()
	q.On("CreateOrUpdate").Return(nil).Maybe()
	q.On("Update", mock.Anything).Return(nil).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	totalUsers, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(5), totalUsers)

	totalStatuses, err := repo.GetTotalStatusCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(9), totalStatuses)

	domains, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(123), domains)

	active, err := repo.GetActiveUserCount(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(123), active)

	_, _ = repo.GetDailyActiveUserCount(ctx)
	_, _ = repo.GetLocalPostCount(ctx)

	comments, err := repo.GetLocalCommentCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(123), comments)

	weekly, err := repo.GetWeeklyActivity(ctx, time.Now().Unix())
	require.NoError(t, err)
	require.NotNil(t, weekly)

	require.NoError(t, repo.RecordActivity(ctx, "post", "", time.Now()))

	contact, err := repo.GetContactAccount(ctx)
	require.NoError(t, err)
	require.NotNil(t, contact)
	assert.Equal(t, "admin", contact.Username)

	usage, err := repo.GetStorageUsage(ctx)
	require.NoError(t, err)
	require.NotNil(t, usage)

	storageHist, err := repo.GetStorageHistory(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, storageHist, 2)

	userGrowth, err := repo.GetUserGrowthHistory(ctx, 1)
	require.NoError(t, err)
	assert.Len(t, userGrowth, 2)

	_, err = repo.GetMetricsSummary(ctx, "week")
	require.NoError(t, err)

	metrics := map[string]interface{}{
		"total_users":      int64(10),
		"active_users":     int64(3),
		"new_users":        int64(1),
		"storage_bytes":    int64(100),
		"media_bytes":      int64(50),
		"database_bytes":   int64(50),
		"total_posts":      int64(20),
		"new_posts":        int64(2),
		"local_posts":      int64(10),
		"federated_posts":  int64(10),
		"known_instances":  int64(5),
		"active_instances": int64(4),
	}
	require.NoError(t, repo.RecordDailyMetrics(ctx, "", metrics))

	domainStats, err := repo.GetDomainStats(ctx, "example.com")
	require.NoError(t, err)
	require.NotNil(t, domainStats)

	history, err := repo.getMetricHistory(ctx, 1, "user_count", "user growth history", func(h models.InstanceHistory) map[string]interface{} {
		return map[string]interface{}{"date": h.Date, "value": h.Value}
	})
	require.NoError(t, err)
	assert.Len(t, history, 2)

	// Exercise getPreviousDayValue date parsing failure branch.
	_, err = repo.getPreviousDayValue(ctx, "not-a-date", "user_count")
	assert.Error(t, err)

	// Exercise default time-range path for GetMetricsSummary.
	summary, err := repo.GetMetricsSummary(ctx, "unknown-range")
	require.NoError(t, err)
	assert.Equal(t, "unknown-range", summary["time_range"])

	// Cover instance rule helpers and fuzzy categorization.
	defaultRules := repo.getDefaultInstanceRules()
	require.Len(t, defaultRules, 5)
	assert.NotEmpty(t, repo.generateDefaultDescription())
	assert.Greater(t, repo.calculateSimilarity("a b c", "a"), float64(0))
	assert.NotEmpty(t, repo.categorizeRulesSmartly(defaultRules, "safety"))

	// SanitizeDescription truncation path.
	long := "<script" + fmt.Sprintf("%010050d", 0)
	out := repo.sanitizeDescription(long)
	assert.Contains(t, out, "&lt;script")
}

func TestInstanceRepository_GetLocalCommentCount_ReturnsMetricWhenPresent(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Run(func(args mock.Arguments) {
		metric := args.Get(0).(*models.InstanceMetrics)
		metric.Value = 7
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	count, err := repo.GetLocalCommentCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(7), count)
}

func TestInstanceRepository_GetInstanceRules_DefaultsOnMissingOrInvalidJSON(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	q.On("First", mock.AnythingOfType("*models.InstanceConfig")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceConfig")).Run(func(args mock.Arguments) {
		cfg := args.Get(0).(*models.InstanceConfig)
		cfg.RulesJSON = ""
	}).Return(nil).Once()
	q.On("First", mock.AnythingOfType("*models.InstanceConfig")).Run(func(args mock.Arguments) {
		cfg := args.Get(0).(*models.InstanceConfig)
		cfg.RulesJSON = "{not-json"
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	rules, err := repo.GetInstanceRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rules, 5)

	rules, err = repo.GetInstanceRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rules, 5)

	rules, err = repo.GetInstanceRules(ctx)
	require.NoError(t, err)
	assert.Len(t, rules, 5)
}

func TestInstanceRepository_GetExtendedDescription_DefaultWhenMissing(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceConfig")).Return(dynamormerrors.ErrItemNotFound)

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	desc, _, err := repo.GetExtendedDescription(ctx)
	require.NoError(t, err)
	assert.Contains(t, desc, "Welcome to Lesser")
}

func TestInstanceRepository_ConstructorsAndCacheInvalidation(t *testing.T) {
	db := new(dynamormmocks.MockDB)
	repo := NewInstanceRepositoryWithCostTracking(db, "test-table", zap.NewNop(), nil)
	require.NotNil(t, repo)

	repo.setCachedState(models.NewDefaultInstanceState())
	repo.invalidateStateCache()
	_, ok := repo.getCachedState()
	assert.False(t, ok)
}

func TestInstanceRepository_GetContactAccount_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	userQuery := new(dynamormmocks.MockQuery)
	actorQuery := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()

	// No admin users -> not found.
	db.On("Model", mock.AnythingOfType("*models.User")).Return(userQuery).Once()
	userQuery.On("Index", mock.Anything).Return(userQuery).Once()
	userQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(userQuery).Once()
	userQuery.On("Limit", mock.Anything).Return(userQuery).Once()
	userQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{}
	}).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetContactAccount(ctx)
	assert.Error(t, err)

	// Admin exists but actor missing -> not found.
	db.On("Model", mock.AnythingOfType("*models.User")).Return(userQuery).Once()
	userQuery.On("Index", mock.Anything).Return(userQuery).Once()
	userQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(userQuery).Once()
	userQuery.On("Limit", mock.Anything).Return(userQuery).Once()
	userQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]models.User)
		*dest = []models.User{{Username: "admin"}}
	}).Return(nil).Once()

	db.On("Model", mock.AnythingOfType("*models.Actor")).Return(actorQuery).Once()
	actorQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(actorQuery).Twice()
	actorQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	_, err = repo.GetContactAccount(ctx)
	assert.Error(t, err)
}

func TestInstanceRepository_MetricGet_NotFoundBranches(t *testing.T) {
	ctx := context.Background()

	newRepoWithMetricFirstErr := func(firstErr error) *InstanceRepository {
		db := new(dynamormmocks.MockDB)
		q := new(dynamormmocks.MockQuery)
		db.On("WithContext", mock.Anything).Return(db).Maybe()
		db.On("Model", mock.Anything).Return(q).Maybe()
		q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
		q.On("Filter", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
		q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(firstErr).Maybe()
		// Lazy-seed paths: a missing counter triggers the one-time scan + persist
		// (see instance_counts.go) before the counter read.
		q.On("All", mock.Anything).Return(nil).Maybe()
		q.On("IfNotExists").Return(q).Maybe()
		ub := new(dynamormmocks.MockUpdateBuilder)
		q.On("UpdateBuilder").Return(ub).Maybe()
		ub.On("Set", mock.Anything, mock.Anything).Return(ub).Maybe()
		ub.On("Add", mock.Anything, mock.Anything).Return(ub).Maybe()
		ub.On("Condition", mock.Anything, mock.Anything, mock.Anything).Return(ub).Maybe()
		ub.On("Execute").Return(nil).Maybe()
		ub.On("ExecuteWithResult", mock.Anything).Return(nil).Maybe()
		return NewInstanceRepository(db, "test-table", zap.NewNop())
	}

	repo := newRepoWithMetricFirstErr(dynamormerrors.ErrItemNotFound)

	totalUsers, err := repo.GetTotalUserCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), totalUsers)

	totalStatuses, err := repo.GetTotalStatusCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), totalStatuses)

	totalDomains, err := repo.GetTotalDomainCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), totalDomains)

	activeUsers, err := repo.GetActiveUserCount(ctx, 7)
	require.NoError(t, err)
	assert.Equal(t, int64(0), activeUsers)
}

func TestInstanceRepository_GetStorageUsage_AndDomainStats_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	_, err := repo.GetStorageUsage(ctx)
	assert.Error(t, err)

	_, err = repo.GetDomainStats(ctx, "example.com")
	assert.Error(t, err)
}

func TestInstanceRepository_SetExtendedDescription_CreateFailure(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db)
	db.On("Model", mock.Anything).Return(q)
	q.On("CreateOrUpdate").Return(fmt.Errorf("create failed"))

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	assert.Error(t, repo.SetExtendedDescription(ctx, "desc"))
}

func TestInstanceRepository_LocalMetricsAndActivity_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetLocalPostCount(ctx)
	assert.Error(t, err)
	_, err = repo.GetLocalCommentCount(ctx)
	assert.Error(t, err)

	_, err = repo.GetWeeklyActivity(ctx, time.Now().Unix())
	assert.Error(t, err)
	assert.Error(t, repo.RecordActivity(ctx, "login", "", time.Now()))
}

func TestInstanceRepository_RuleValidationAndCategorization_Branches(t *testing.T) {
	repo := &InstanceRepository{logger: zap.NewNop()}

	// validateAndFilterRules: empty text dropped, long text truncated, duplicate IDs handled.
	longText := strings.Repeat("a", 600)
	rules := []storage.InstanceRule{
		{ID: "dup", Text: "First rule"},
		{ID: "dup", Text: "Duplicate ID rule"},
		{ID: "", Text: "No ID rule"},
		{ID: "empty", Text: "   "},
		{ID: "long", Text: longText},
	}
	filtered := repo.validateAndFilterRules(rules)
	require.GreaterOrEqual(t, len(filtered), 3)
	for _, r := range filtered {
		assert.NotEmpty(t, strings.TrimSpace(r.Text))
	}

	// categorizeRulesSmartly: fuzzy matching and fallback.
	categorized := repo.categorizeRulesSmartly(filtered, "rule")
	assert.NotEmpty(t, categorized)
	assert.GreaterOrEqual(t, repo.calculateSimilarity("posting rules", "posting"), float64(0))
}

func TestInstanceRepository_StateErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceState")).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetInstanceState(ctx)
	assert.Error(t, err)
	_, err = repo.EnsureInstanceState(ctx)
	assert.Error(t, err)
}

func TestInstanceRepository_MetricsSummary_AllQueryErrorsStillReturnsSummary(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	summary, err := repo.GetMetricsSummary(ctx, "week")
	require.NoError(t, err)
	assert.Equal(t, "week", summary["time_range"])
}

func TestInstanceRepository_GetLocalCommentCount_MissingMetricReturnsZero(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	count, err := repo.GetLocalCommentCount(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

func TestInstanceRepository_RecordActivity_SwitchBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.Anything).Return(nil).Maybe()
	q.On("Create").Return(nil).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	require.NoError(t, repo.RecordActivity(ctx, "registration", "", time.Now()))
	require.NoError(t, repo.RecordActivity(ctx, "status", "", time.Now()))
	require.NoError(t, repo.RecordActivity(ctx, "login", "", time.Now()))
}

func TestInstanceRepository_GetMetricHistory_InvalidDaysDefaults(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(nil).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetStorageHistory(ctx, 0)
	require.NoError(t, err)
}

func TestInstanceRepository_getPreviousDayValue_NotFoundAndErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())

	q.On("First", mock.AnythingOfType("*models.InstanceHistory")).Return(dynamormerrors.ErrItemNotFound).Once()
	val, err := repo.getPreviousDayValue(ctx, "2024-12-28", "user_count")
	require.NoError(t, err)
	assert.Equal(t, int64(0), val)

	q.On("First", mock.AnythingOfType("*models.InstanceHistory")).Return(fmt.Errorf("boom")).Once()
	_, err = repo.getPreviousDayValue(ctx, "2024-12-28", "user_count")
	assert.Error(t, err)
}

func TestInstanceRepository_EnsureInstanceState_CreateFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceState")).Return(dynamormerrors.ErrItemNotFound).Once()
	q.On("Create").Return(fmt.Errorf("create failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.EnsureInstanceState(ctx)
	assert.Error(t, err)
}

func TestInstanceRepository_CategorizeRulesSmartly_SafetyAndPostingBranches(t *testing.T) {
	repo := &InstanceRepository{logger: zap.NewNop()}

	rules := []storage.InstanceRule{
		{ID: "1", Text: "no harassment or abuse"},
		{ID: "2", Text: "no spam or advertising"},
		{ID: "3", Text: "use content warnings when needed"},
	}

	safety := repo.categorizeRulesSmartly(rules, "safety")
	assert.NotEmpty(t, safety)

	posting := repo.categorizeRulesSmartly(rules, "posting")
	assert.NotEmpty(t, posting)
}

func TestInstanceRepository_GetTotals_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetTotalUserCount(ctx)
	assert.Error(t, err)
	_, err = repo.GetTotalStatusCount(ctx)
	assert.Error(t, err)
	_, err = repo.GetTotalDomainCount(ctx)
	assert.Error(t, err)
}

func TestInstanceRepository_SetInstanceRules_CreateFailureReturnsError(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("CreateOrUpdate").Return(fmt.Errorf("create failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	assert.Error(t, repo.SetInstanceRules(ctx, []storage.InstanceRule{{Text: "rule"}}))
}

func TestInstanceRepository_GetContactAccount_QueryErrorBranch(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	userQuery := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.AnythingOfType("*models.User")).Return(userQuery).Once()
	userQuery.On("Index", mock.Anything).Return(userQuery).Once()
	userQuery.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(userQuery).Once()
	userQuery.On("Limit", mock.Anything).Return(userQuery).Once()
	userQuery.On("All", mock.Anything).Return(fmt.Errorf("query failed")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetContactAccount(ctx)
	assert.Error(t, err)
}

func TestInstanceRepository_GetActiveUserCount_ErrorBranch(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceMetrics")).Return(fmt.Errorf("boom")).Once()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetActiveUserCount(ctx, 7)
	assert.Error(t, err)
}

func TestInstanceRepository_Setters_ReturnErrorWhenEnsureStateFails(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("First", mock.AnythingOfType("*models.InstanceState")).Return(fmt.Errorf("boom")).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	assert.Error(t, repo.SetBootstrapWalletAddress(ctx, "0xabc"))
	assert.Error(t, repo.SetPrimaryAdminUsername(ctx, "alice"))
	assert.Error(t, repo.SetInstanceLocked(ctx, true))
}

func TestInstanceRepository_GetMetricsSummary_TimeRangeCases(t *testing.T) {
	ctx := context.Background()
	db := new(dynamormmocks.MockDB)
	q := new(dynamormmocks.MockQuery)

	db.On("WithContext", mock.Anything).Return(db).Maybe()
	db.On("Model", mock.Anything).Return(q).Maybe()
	q.On("Index", mock.Anything).Return(q).Maybe()
	q.On("Where", mock.Anything, mock.Anything, mock.Anything).Return(q).Maybe()
	q.On("All", mock.Anything).Return(nil).Maybe()

	repo := NewInstanceRepository(db, "test-table", zap.NewNop())
	_, err := repo.GetMetricsSummary(ctx, "month")
	require.NoError(t, err)
	_, err = repo.GetMetricsSummary(ctx, "quarter")
	require.NoError(t, err)
	_, err = repo.GetMetricsSummary(ctx, "year")
	require.NoError(t, err)
}
