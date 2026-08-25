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
	dmerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

func TestInstanceHealthRepository_SaveHealthCheckAndBatch(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewInstanceHealthRepository(mockDB, "tbl", zap.NewNop(), nil)

	err := repo.SaveHealthCheck(context.Background(), &models.InstanceHealth{Domain: ""})
	require.Error(t, err)

	mockQuery.On("Create").Return(nil).Once()
	h := &models.InstanceHealth{Domain: "example.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0, ResponseTime: 1500 * time.Millisecond}
	err = repo.SaveHealthCheck(context.Background(), h)
	require.NoError(t, err)
	require.False(t, h.Timestamp.IsZero())

	err = repo.SaveHealthChecks(context.Background(), nil)
	require.NoError(t, err)

	mockQuery.On("Create").Return(nil).Twice()
	err = repo.SaveHealthChecks(context.Background(), []*models.InstanceHealth{
		{Domain: "a.com", Reachable: false, StatusCode: 0, ErrorRate: 1.0},
		{Domain: "b.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0},
	})
	require.NoError(t, err)

	mockQuery.On("Create").Return(errors.New("boom")).Once()
	err = repo.SaveHealthChecks(context.Background(), []*models.InstanceHealth{{Domain: "a.com"}})
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestInstanceHealthRepository_GetLatestAndHistory(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewInstanceHealthRepository(mockDB, "tbl", zap.NewNop(), nil)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		s := reflect.MakeSlice(v.Elem().Type(), 0, 1)
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0, Timestamp: time.Now().UTC()}))
		v.Elem().Set(s)
	}).Return(nil).Once()
	_, err := repo.GetLatestHealthCheck(context.Background(), "example.com")
	require.NoError(t, err)

	mockQuery.On("All", mock.Anything).Return(nil).Once()
	_, err = repo.GetLatestHealthCheck(context.Background(), "missing.com")
	require.Error(t, err)

	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetLatestHealthCheck(context.Background(), "err.com")
	require.Error(t, err)

	// History analysis path including criticalPercentage warning
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		now := time.Now().UTC()
		s := reflect.MakeSlice(v.Elem().Type(), 0, 4)
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0, Timestamp: now}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: false, StatusCode: 0, ErrorRate: 1.0, Timestamp: now.Add(-time.Minute)}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: false, StatusCode: 500, ErrorRate: 1.0, Timestamp: now.Add(-2 * time.Minute)}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0, Timestamp: now.Add(-3 * time.Minute)}))
		v.Elem().Set(s)
	}).Return(nil).Once()
	_, err = repo.GetHealthHistory(context.Background(), "example.com", time.Time{}, 0)
	require.NoError(t, err)

	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetHealthHistory(context.Background(), "example.com", time.Now().Add(-time.Hour), 10)
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestInstanceHealthRepository_DomainsSummaryAndUnhealthy(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewInstanceHealthRepository(mockDB, "tbl", zap.NewNop(), nil)

	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		s := reflect.MakeSlice(v.Elem().Type(), 0, 3)
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealthSummary{Domain: "bad.com", HealthScore: 40, Availability: 0.8, ErrorRate: 0.2}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealthSummary{Domain: "meh.com", HealthScore: 70, Availability: 0.95, ErrorRate: 0.05}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealthSummary{Domain: "good.com", HealthScore: 99, Availability: 1.0, ErrorRate: 0.0}))
		v.Elem().Set(s)
	}).Return(nil).Twice()

	domains, err := repo.GetDomainsForHealthCheck(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, domains, 3)

	unhealthy, err := repo.GetUnhealthyInstances(context.Background(), 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(unhealthy), 2)

	// NotFound is allowed
	mockQuery.On("All", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	_, err = repo.GetDomainsForHealthCheck(context.Background(), 10)
	require.NoError(t, err)

	// Query error should fail
	mockQuery.On("All", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetDomainsForHealthCheck(context.Background(), 10)
	require.Error(t, err)

	requireNoMockExpectations(t, mockDB, mockQuery)
}

func TestInstanceHealthRepository_SaveAndGetSummaryAndCalculate(t *testing.T) {
	mockDB, mockQuery := newMockDBQuery()
	repo := NewInstanceHealthRepository(mockDB, "tbl", zap.NewNop(), nil)

	err := repo.SaveHealthSummary(context.Background(), &models.InstanceHealthSummary{Domain: ""})
	require.Error(t, err)

	mockQuery.On("CreateOrUpdate").Return(nil).Once()
	summary := &models.InstanceHealthSummary{Domain: "example.com", Window: 24 * time.Hour, Availability: 0.9, HealthScore: 70, ErrorRate: 0.2}
	err = repo.SaveHealthSummary(context.Background(), summary)
	require.NoError(t, err)
	require.False(t, summary.LastUpdated.IsZero())

	mockQuery.On("First", mock.Anything).Return(dmerrors.ErrItemNotFound).Once()
	_, err = repo.GetHealthSummary(context.Background(), "example.com", time.Hour)
	require.Error(t, err)

	mockQuery.On("First", mock.Anything).Return(errors.New("boom")).Once()
	_, err = repo.GetHealthSummary(context.Background(), "example.com", 24*time.Hour)
	require.Error(t, err)

	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		if s, ok := dst.(*models.InstanceHealthSummary); ok {
			s.Domain = "example.com"
			s.Window = time.Hour
			s.Availability = 1.0
			s.HealthScore = 99
			s.LastUpdated = time.Now().UTC()
		}
	}).Return(nil).Once()
	_, err = repo.GetHealthSummary(context.Background(), "example.com", time.Hour)
	require.NoError(t, err)

	// CalculateHealthSummary (uses GetHealthHistory)
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dst := args.Get(0)
		v := reflect.ValueOf(dst)
		if v.Kind() != reflect.Ptr || v.Elem().Kind() != reflect.Slice {
			return
		}
		now := time.Now().UTC()
		s := reflect.MakeSlice(v.Elem().Type(), 0, 2)
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: true, StatusCode: 200, ErrorRate: 0.0, ResponseTime: 500 * time.Millisecond, Timestamp: now, InboxBacklog: 10}))
		s = reflect.Append(s, reflect.ValueOf(&models.InstanceHealth{Domain: "example.com", Reachable: false, StatusCode: 0, ErrorRate: 1.0, Timestamp: now.Add(-time.Minute), InboxBacklog: 2000}))
		v.Elem().Set(s)
	}).Return(nil).Once()
	sum, err := repo.CalculateHealthSummary(context.Background(), "example.com", time.Hour)
	require.NoError(t, err)
	require.Equal(t, 2, sum.SampleCount)
	require.NotEmpty(t, sum.StatusCodeCounts)

	// Cleanup is a no-op but covered
	n, err := repo.CleanupOldHealthData(context.Background(), 24*time.Hour)
	require.NoError(t, err)
	require.Equal(t, 0, n)

	requireNoMockExpectations(t, mockDB, mockQuery)
}
