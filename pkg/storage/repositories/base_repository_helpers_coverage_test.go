package repositories

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/v2/pkg/mocks"
	"go.uber.org/zap"
)

type helperModel struct {
	PK     string
	SK     string
	GSI1SK string

	updateErr error
}

func (m helperModel) UpdateKeys() error { return m.updateErr }
func (m helperModel) GetPK() string     { return m.PK }
func (m helperModel) GetSK() string     { return m.SK }

func newTestCostService(t *testing.T) *cost.TrackingService {
	t.Helper()

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 1000

	svc := cost.NewTrackingService(nil, zap.NewNop(), cfg)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })
	return svc
}

type errHTTPClient struct{}

func (errHTTPClient) Do(*http.Request) (*http.Response, error) { return nil, assert.AnError }

func newFailingCostService(t *testing.T) *cost.TrackingService {
	t.Helper()

	awsCfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("test", "test", "")),
		HTTPClient:  errHTTPClient{},
	}

	cw := cloudwatch.NewFromConfig(awsCfg)

	cfg := cost.DefaultTrackingServiceConfig()
	cfg.MetricsFlushInterval = time.Hour
	cfg.MetricsBatchSize = 1

	svc := cost.NewTrackingService(cw, zap.NewNop(), cfg)
	t.Cleanup(func() { _ = svc.Close(context.Background()) })
	return svc
}

func TestBaseRepositoryHelpers_BatchCreateHelper(t *testing.T) {
	t.Run("empty input returns nil", func(t *testing.T) {
		repo := NewBaseRepository[helperModel](nil, "table", zap.NewNop())
		err := repo.BatchCreateHelper(context.Background(), nil, "helper")
		require.NoError(t, err)
	})

	t.Run("item key update failure returns create error", func(t *testing.T) {
		repo := NewBaseRepository[helperModel](nil, "table", zap.NewNop())
		err := repo.BatchCreateHelper(context.Background(), []helperModel{
			{PK: "PK#1", SK: "SK#1", updateErr: assert.AnError},
		}, "helper")
		require.Error(t, err)
	})

	t.Run("db batch create error is wrapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("BatchCreate", mock.Anything).Return(assert.AnError).Once()

		repo := NewBaseRepository[helperModel](mockDB, "table", zap.NewNop())
		err := repo.BatchCreateHelper(context.Background(), []helperModel{
			{PK: "PK#1", SK: "SK#1"},
		}, "helper")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("success calls batch create", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("BatchCreate", mock.Anything).Return(nil).Once()

		repo := NewBaseRepository[helperModel](mockDB, "table", zap.NewNop())
		err := repo.BatchCreateHelper(context.Background(), []helperModel{
			{PK: "PK#1", SK: "SK#1"},
			{PK: "PK#2", SK: "SK#2"},
		}, "helper")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}

func TestBaseRepositoryHelpers_QueryGSIWithTimeRangeHelper(t *testing.T) {
	t.Run("default limit + order with empty results", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "PK#1").Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "GSI1SK", SortOrderDesc).Return(mockQuery).Once()
		mockQuery.On("Limit", defaultBaseQueryLimit+1).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]helperModel)
			*dest = nil
		}).Once()

		repo := NewBaseRepositoryWithCostTracking[helperModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		start := time.Unix(0, 0).UTC()
		end := start.Add(time.Hour)

		results, cursor, err := repo.QueryGSIWithTimeRangeHelper(context.Background(), "gsi1", "gsi1PK", "GSI1SK", "PK#1", start, end, 0, "", "", "op")
		require.NoError(t, err)
		assert.Empty(t, results)
		assert.Empty(t, cursor)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("cursor pagination + hasMore uses extracted cursor", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "PK#1").Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "GSI1SK", SortOrderAsc).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">", "CURSOR#prev").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]helperModel)
			*dest = []helperModel{
				{PK: "PK#1", SK: "SK#1", GSI1SK: "CURSOR#next"},
				{PK: "PK#1", SK: "SK#2", GSI1SK: "CURSOR#sentinel"},
			}
		}).Once()

		repo := NewBaseRepositoryWithCostTracking[helperModel](mockDB, "table", zap.NewNop(), newTestCostService(t), "repo")
		start := time.Unix(0, 0).UTC()
		end := start.Add(time.Hour)

		results, cursor, err := repo.QueryGSIWithTimeRangeHelper(context.Background(), "gsi1", "gsi1PK", "GSI1SK", "PK#1", start, end, 1, "CURSOR#prev", SortOrderAsc, "op")
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Equal(t, "CURSOR#next", cursor)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("desc cursor uses less-than comparator", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "PK#1").Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "GSI1SK", SortOrderDesc).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<", "CURSOR#prev").Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Once()

		repo := NewBaseRepository[helperModel](mockDB, "table", zap.NewNop())
		start := time.Unix(0, 0).UTC()
		end := start.Add(time.Hour)

		_, _, err := repo.QueryGSIWithTimeRangeHelper(context.Background(), "gsi1", "gsi1PK", "GSI1SK", "PK#1", start, end, 1, "CURSOR#prev", SortOrderDesc, "op")
		require.NoError(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("query error is mapped", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "PK#1").Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "GSI1SK", SortOrderDesc).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(assert.AnError).Once()

		repo := NewBaseRepository[helperModel](mockDB, "table", zap.NewNop())
		start := time.Unix(0, 0).UTC()
		end := start.Add(time.Hour)

		_, _, err := repo.QueryGSIWithTimeRangeHelper(context.Background(), "gsi1", "gsi1PK", "GSI1SK", "PK#1", start, end, 1, "", "", "op")
		require.Error(t, err)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})

	t.Run("hasMore without cursor field leaves nextCursor empty", func(t *testing.T) {
		mockDB := new(mocks.MockDB)
		mockQuery := new(mocks.MockQuery)

		mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
		mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Index", "gsi1").Return(mockQuery).Once()
		mockQuery.On("Where", "gsi1PK", "=", "PK#1").Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", ">=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("Where", "GSI1SK", "<=", mock.Anything).Return(mockQuery).Once()
		mockQuery.On("OrderBy", "GSI1SK", SortOrderDesc).Return(mockQuery).Once()
		mockQuery.On("Limit", 2).Return(mockQuery).Once()
		mockQuery.On("All", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
			dest := args.Get(0).(*[]helperModel)
			*dest = []helperModel{
				{PK: "PK#1", SK: "SK#1"},
				{PK: "PK#1", SK: "SK#2"},
			}
		}).Once()

		repo := NewBaseRepository[helperModel](mockDB, "table", zap.NewNop())
		start := time.Unix(0, 0).UTC()
		end := start.Add(time.Hour)

		results, cursor, err := repo.QueryGSIWithTimeRangeHelper(context.Background(), "gsi1", "gsi1PK", "GSI1SK", "PK#1", start, end, 1, "", "", "op")
		require.NoError(t, err)
		require.Len(t, results, 1)
		assert.Empty(t, cursor)

		mockDB.AssertExpectations(t)
		mockQuery.AssertExpectations(t)
	})
}
