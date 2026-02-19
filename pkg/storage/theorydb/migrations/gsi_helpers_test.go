package migrations

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap/zaptest"
)

func TestGSIHelper_CreateGSI_PersistsRecord(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)

	helper, err := NewGSIHelper(mockDB, "test-table", logger)
	require.NoError(t, err)

	include := []string{"field1", "field2"}
	definition := GSIDefinition{
		Name:           "GSI7",
		HashKey:        "gsi7PK",
		HashKeyType:    "S",
		RangeKey:       "gsi7SK",
		RangeKeyType:   "S",
		ProjectionType: projectionTypeInclude,
		IncludeFields:  append([]string(nil), include...),
		ReadCapacity:   5,
		WriteCapacity:  10,
	}

	var persistedRecord *GSIMigrationRecord

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		rec, ok := model.(*GSIMigrationRecord)
		if ok {
			persistedRecord = rec
		}
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	err = helper.CreateGSI(ctx, definition)
	require.NoError(t, err)

	require.NotNil(t, persistedRecord)
	assert.Equal(t, helper.partitionKey(), persistedRecord.PK)
	assert.Equal(t, helper.sortKey(definition.Name), persistedRecord.SK)
	assert.Equal(t, "test-table", persistedRecord.TableName)
	assert.Equal(t, definition.Name, persistedRecord.GSIName)
	assert.Equal(t, definition.HashKey, persistedRecord.HashKey)
	assert.Equal(t, definition.HashKeyType, persistedRecord.HashKeyType)
	assert.Equal(t, definition.RangeKey, persistedRecord.RangeKey)
	assert.Equal(t, definition.RangeKeyType, persistedRecord.RangeKeyType)
	assert.Equal(t, definition.ProjectionType, persistedRecord.ProjectionType)
	assert.Equal(t, definition.ReadCapacity, persistedRecord.ReadCapacity)
	assert.Equal(t, definition.WriteCapacity, persistedRecord.WriteCapacity)
	assert.Equal(t, gsiStatusCreated, persistedRecord.Status)
	assert.WithinDuration(t, time.Now(), persistedRecord.CreatedAt, time.Second)
	assert.Equal(t, include, persistedRecord.IncludeFields)

	// Ensure IncludeFields were copied defensively
	include[0] = "mutated"
	assert.Equal(t, "field1", persistedRecord.IncludeFields[0])

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGSIHelper_DeleteGSI_UpdatesStatus(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)

	helper, err := NewGSIHelper(mockDB, "test-table", logger)
	require.NoError(t, err)

	var recordUnderTest *GSIMigrationRecord

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		rec, ok := model.(*GSIMigrationRecord)
		if ok {
			recordUnderTest = rec
		}
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", helper.sortKey("GSI7")).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*GSIMigrationRecord)
		dest.PK = helper.partitionKey()
		dest.SK = helper.sortKey("GSI7")
		dest.TableName = "test-table"
		dest.GSIName = "GSI7"
		dest.Status = gsiStatusCreated
		dest.CreatedAt = time.Unix(0, 0)
	}).Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(nil).Once()

	err = helper.DeleteGSI(ctx, "GSI7")
	require.NoError(t, err)

	require.NotNil(t, recordUnderTest)
	assert.Equal(t, gsiStatusDeleted, recordUnderTest.Status)
	assert.Equal(t, helper.partitionKey(), recordUnderTest.PK)
	assert.Equal(t, helper.sortKey("GSI7"), recordUnderTest.SK)
	assert.Equal(t, "test-table", recordUnderTest.TableName)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGSIHelper_GetGSIStatus_ReturnsRecord(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)

	helper, err := NewGSIHelper(mockDB, "test-table", logger)
	require.NoError(t, err)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*GSIMigrationRecord)
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", helper.sortKey("GSI7")).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*GSIMigrationRecord)
		dest.PK = helper.partitionKey()
		dest.SK = helper.sortKey("GSI7")
		dest.TableName = "test-table"
		dest.GSIName = "GSI7"
		dest.Status = gsiStatusCreated
		dest.CreatedAt = time.Unix(0, 0)
	}).Return(nil).Once()

	record, err := helper.GetGSIStatus(ctx, "GSI7")
	require.NoError(t, err)
	require.NotNil(t, record)

	assert.Equal(t, helper.partitionKey(), record.PK)
	assert.Equal(t, helper.sortKey("GSI7"), record.SK)
	assert.Equal(t, "GSI7", record.GSIName)
	assert.Equal(t, gsiStatusCreated, record.Status)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGSIHelper_ListGSIMigrations_ReturnsRecords(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)
	logger := zaptest.NewLogger(t)

	helper, err := NewGSIHelper(mockDB, "test-table", logger)
	require.NoError(t, err)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		_, ok := model.(*GSIMigrationRecord)
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*[]*GSIMigrationRecord)
		*dest = []*GSIMigrationRecord{
			{
				PK:        helper.partitionKey(),
				SK:        helper.sortKey("GSI7"),
				TableName: "test-table",
				GSIName:   "GSI7",
				Status:    gsiStatusCreated,
				CreatedAt: time.Unix(0, 0),
			},
		}
	}).Return(nil).Once()

	records, err := helper.ListGSIMigrations(ctx)
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "GSI7", records[0].GSIName)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestNewGSIHelper_Validation(t *testing.T) {
	_, err := NewGSIHelper(nil, "test-table", zaptest.NewLogger(t))
	require.Error(t, err)

	_, err = NewGSIHelper(new(mocks.MockDB), "", zaptest.NewLogger(t))
	require.Error(t, err)

	helper, err := NewGSIHelper(new(mocks.MockDB), "test-table", nil)
	require.NoError(t, err)
	require.NotNil(t, helper)
	require.NotNil(t, helper.logger)
}

func TestGSIHelper_CreateGSI_DefaultsProjectionTypeWhenEmpty(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	helper, err := NewGSIHelper(mockDB, "test-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	definition := GSIDefinition{
		Name:        "GSI7",
		HashKey:     "gsi7PK",
		HashKeyType: "S",
	}

	var persistedRecord *GSIMigrationRecord

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		rec, ok := model.(*GSIMigrationRecord)
		if ok {
			persistedRecord = rec
		}
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Create").Return(nil).Once()

	require.NoError(t, helper.CreateGSI(ctx, definition))
	require.NotNil(t, persistedRecord)
	require.Equal(t, projectionTypeAll, persistedRecord.ProjectionType)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestGSIHelper_CreateGSI_RejectsInvalidDefinition(t *testing.T) {
	ctx := context.Background()
	helper, err := NewGSIHelper(new(mocks.MockDB), "test-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	err = helper.CreateGSI(ctx, GSIDefinition{
		Name:        "GSI7",
		HashKey:     "gsi7PK",
		HashKeyType: "INVALID",
	})
	require.Error(t, err)

	err = helper.CreateGSI(ctx, GSIDefinition{
		Name:           "GSI7",
		HashKey:        "gsi7PK",
		HashKeyType:    "S",
		ProjectionType: projectionTypeInclude,
	})
	require.Error(t, err)
}

func TestGSIHelper_DeleteGSI_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	helper, err := NewGSIHelper(mockDB, "test-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	require.Error(t, helper.DeleteGSI(ctx, ""))

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", helper.sortKey("GSI7")).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(errors.New("load-failed")).Once()
	require.Error(t, helper.DeleteGSI(ctx, "GSI7"))
}

func TestGSIHelper_DeleteGSI_UpdateErrorAndTableNameFill(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	helper, err := NewGSIHelper(mockDB, "test-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	var recordUnderTest *GSIMigrationRecord

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.MatchedBy(func(model any) bool {
		rec, ok := model.(*GSIMigrationRecord)
		if ok {
			recordUnderTest = rec
		}
		return ok
	})).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", helper.sortKey("GSI7")).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*GSIMigrationRecord)
		dest.GSIName = "GSI7"
		dest.Status = gsiStatusCreated
	}).Return(nil).Once()
	mockQuery.On("Update", mock.Anything).Return(errors.New("update-failed")).Once()

	err = helper.DeleteGSI(ctx, "GSI7")
	require.Error(t, err)
	require.NotNil(t, recordUnderTest)
	require.Equal(t, helper.tableName, recordUnderTest.TableName)
}

func TestGSIHelper_GetGSIStatus_And_ListGSIMigrations_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	helper, err := NewGSIHelper(mockDB, "test-table", zaptest.NewLogger(t))
	require.NoError(t, err)

	_, err = helper.GetGSIStatus(ctx, "")
	require.Error(t, err)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", helper.sortKey("GSI7")).Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(errors.New("load-failed")).Once()
	_, err = helper.GetGSIStatus(ctx, "GSI7")
	require.Error(t, err)

	mockDB.On("WithContext", mock.Anything).Return(mockDB).Once()
	mockDB.On("Model", mock.Anything).Return(mockQuery).Once()
	mockQuery.On("Where", "PK", "=", helper.partitionKey()).Return(mockQuery).Once()
	mockQuery.On("All", mock.Anything).Return(errors.New("list-failed")).Once()
	_, err = helper.ListGSIMigrations(ctx)
	require.Error(t, err)
}
