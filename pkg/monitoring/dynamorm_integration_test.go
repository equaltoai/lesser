package monitoring

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	cwTypes "github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbTypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type stubDynamoDBClient struct {
	getItemFunc          func(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	putItemFunc          func(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	updateItemFunc       func(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	deleteItemFunc       func(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	queryFunc            func(context.Context, *dynamodb.QueryInput, ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	scanFunc             func(context.Context, *dynamodb.ScanInput, ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	batchGetItemFunc      func(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error)
	batchWriteItemFunc    func(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
	transactWriteItemsFunc func(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error)
	transactGetItemsFunc   func(context.Context, *dynamodb.TransactGetItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error)
}

func (s *stubDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	if s.getItemFunc == nil {
		return nil, nil
	}
	return s.getItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	if s.putItemFunc == nil {
		return nil, nil
	}
	return s.putItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	if s.updateItemFunc == nil {
		return nil, nil
	}
	return s.updateItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	if s.deleteItemFunc == nil {
		return nil, nil
	}
	return s.deleteItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	if s.queryFunc == nil {
		return nil, nil
	}
	return s.queryFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	if s.scanFunc == nil {
		return nil, nil
	}
	return s.scanFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) BatchGetItem(ctx context.Context, params *dynamodb.BatchGetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
	if s.batchGetItemFunc == nil {
		return nil, nil
	}
	return s.batchGetItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
	if s.batchWriteItemFunc == nil {
		return nil, nil
	}
	return s.batchWriteItemFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) TransactWriteItems(ctx context.Context, params *dynamodb.TransactWriteItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
	if s.transactWriteItemsFunc == nil {
		return nil, nil
	}
	return s.transactWriteItemsFunc(ctx, params, optFns...)
}

func (s *stubDynamoDBClient) TransactGetItems(ctx context.Context, params *dynamodb.TransactGetItemsInput, optFns ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
	if s.transactGetItemsFunc == nil {
		return nil, nil
	}
	return s.transactGetItemsFunc(ctx, params, optFns...)
}

func testCloudWatchMetrics() *CloudWatchMetrics {
	return &CloudWatchMetrics{
		logger:      zap.NewNop(),
		environment: "test",
		namespace:   "ns",
		dimensions:  map[string]string{},
		buffer: &EnhancedMetricBuffer{
			metrics:   make([]cwTypes.MetricDatum, 0, 1000),
			maxSize:   1000,
			flushSize: 1000,
			flushFunc: func([]cwTypes.MetricDatum) error { return nil },
		},
	}
}

func TestDynamORMMetricsWrapper_GetItemRecordsMetrics(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	client := &stubDynamoDBClient{
		getItemFunc: func(context.Context, *dynamodb.GetItemInput, ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{
				Item: map[string]ddbTypes.AttributeValue{
					"pk": &ddbTypes.AttributeValueMemberS{Value: "pk"},
				},
				ConsumedCapacity: &ddbTypes.ConsumedCapacity{
					ReadCapacityUnits:  aws.Float64(1.25),
					WriteCapacityUnits: aws.Float64(0),
				},
			}, nil
		},
	}

	wrapper := NewDynamORMMetricsWrapper(client, cwm, zap.NewNop())
	out, err := wrapper.GetItem(context.Background(), &dynamodb.GetItemInput{TableName: aws.String("table")})
	require.NoError(t, err)
	require.NotNil(t, out)

	assert.GreaterOrEqual(t, cwm.buffer.Size(), 2)
}

func TestDynamORMMetricsWrapper_SingleItemWrappers(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	putErr := errors.New("put failed")

	client := &stubDynamoDBClient{
		putItemFunc: func(context.Context, *dynamodb.PutItemInput, ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{
				ConsumedCapacity: &ddbTypes.ConsumedCapacity{
					ReadCapacityUnits:  aws.Float64(1),
					WriteCapacityUnits: aws.Float64(2),
				},
			}, nil
		},
		updateItemFunc: func(context.Context, *dynamodb.UpdateItemInput, ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			return &dynamodb.UpdateItemOutput{
				ConsumedCapacity: &ddbTypes.ConsumedCapacity{
					ReadCapacityUnits:  aws.Float64(0.5),
					WriteCapacityUnits: aws.Float64(1.5),
				},
			}, nil
		},
		deleteItemFunc: func(context.Context, *dynamodb.DeleteItemInput, ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
			return nil, putErr
		},
	}

	wrapper := NewDynamORMMetricsWrapper(client, cwm, zap.NewNop())

	_, err := wrapper.PutItem(context.Background(), &dynamodb.PutItemInput{TableName: aws.String("table")})
	require.NoError(t, err)

	_, err = wrapper.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{TableName: aws.String("table")})
	require.NoError(t, err)

	_, err = wrapper.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{TableName: aws.String("table")})
	require.ErrorIs(t, err, putErr)
}

func TestDynamORMMetricsWrapper_QueryHelpers(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	wrapper := NewDynamORMMetricsWrapper(&stubDynamoDBClient{}, cwm, zap.NewNop())

	_, err := wrapper.wrapQueryOperation(context.Background(), "Query", "table", 0, func() (interface{}, error) {
		return &dynamodb.QueryOutput{
			Count: 2,
			ConsumedCapacity: &ddbTypes.ConsumedCapacity{
				ReadCapacityUnits: aws.Float64(1),
			},
		}, nil
	})
	require.NoError(t, err)

	_, err = wrapper.wrapQueryOperation(context.Background(), "Scan", "table", 0, func() (interface{}, error) {
		return &dynamodb.ScanOutput{
			Count: 3,
			ConsumedCapacity: &ddbTypes.ConsumedCapacity{
				ReadCapacityUnits: aws.Float64(2),
			},
		}, nil
	})
	require.NoError(t, err)

	assert.Equal(t, "unknown", getTableName(nil))
	assert.Equal(t, 0.0, getCapacityUnits(nil))
	assert.Equal(t, int64(0), getItemCount(&dynamodb.GetItemOutput{}, errors.New("nope")))
	assert.Equal(t, int64(0), getItemCount(&dynamodb.GetItemOutput{}, nil))
	assert.Equal(t, int64(2), getItemCount(&dynamodb.QueryOutput{Count: 2}, nil))
	assert.Equal(t, int64(3), getItemCount(&dynamodb.ScanOutput{Count: 3}, nil))
}

func TestDynamORMMetricsWrapper_BatchAndTransactions(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()

	client := &stubDynamoDBClient{
		batchGetItemFunc: func(context.Context, *dynamodb.BatchGetItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			return &dynamodb.BatchGetItemOutput{
				ConsumedCapacity: []ddbTypes.ConsumedCapacity{
					{ReadCapacityUnits: aws.Float64(1), WriteCapacityUnits: aws.Float64(2)},
					{ReadCapacityUnits: aws.Float64(3), WriteCapacityUnits: aws.Float64(4)},
				},
			}, nil
		},
		batchWriteItemFunc: func(context.Context, *dynamodb.BatchWriteItemInput, ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error) {
			return &dynamodb.BatchWriteItemOutput{
				ConsumedCapacity: []ddbTypes.ConsumedCapacity{
					{ReadCapacityUnits: aws.Float64(1), WriteCapacityUnits: aws.Float64(1)},
				},
			}, nil
		},
		transactWriteItemsFunc: func(context.Context, *dynamodb.TransactWriteItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactWriteItemsOutput, error) {
			return &dynamodb.TransactWriteItemsOutput{
				ConsumedCapacity: []ddbTypes.ConsumedCapacity{
					{ReadCapacityUnits: aws.Float64(1), WriteCapacityUnits: aws.Float64(2)},
				},
			}, nil
		},
		transactGetItemsFunc: func(context.Context, *dynamodb.TransactGetItemsInput, ...func(*dynamodb.Options)) (*dynamodb.TransactGetItemsOutput, error) {
			return &dynamodb.TransactGetItemsOutput{
				ConsumedCapacity: []ddbTypes.ConsumedCapacity{
					{ReadCapacityUnits: aws.Float64(5), WriteCapacityUnits: aws.Float64(0)},
				},
			}, nil
		},
	}

	wrapper := NewDynamORMMetricsWrapper(client, cwm, zap.NewNop())

	_, err := wrapper.BatchGetItem(context.Background(), &dynamodb.BatchGetItemInput{
		RequestItems: map[string]ddbTypes.KeysAndAttributes{
			"t1": {Keys: []map[string]ddbTypes.AttributeValue{{}}},
			"t2": {Keys: []map[string]ddbTypes.AttributeValue{{}, {}}},
		},
	})
	require.NoError(t, err)

	_, err = wrapper.BatchWriteItem(context.Background(), &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]ddbTypes.WriteRequest{
			"t": {{}, {}},
		},
	})
	require.NoError(t, err)

	_, err = wrapper.TransactWriteItems(context.Background(), &dynamodb.TransactWriteItemsInput{TransactItems: []ddbTypes.TransactWriteItem{{}}})
	require.NoError(t, err)

	_, err = wrapper.TransactGetItems(context.Background(), &dynamodb.TransactGetItemsInput{TransactItems: []ddbTypes.TransactGetItem{{}}})
	require.NoError(t, err)

	factory := NewWrapperFactory(cwm, zap.NewNop())
	require.NotNil(t, factory.WrapClient(client))
}

func TestDynamORMMetricsWrapper_WrapSingleItemOperationUnsupportedType(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	wrapper := NewDynamORMMetricsWrapper(&stubDynamoDBClient{}, cwm, zap.NewNop())

	_, err := wrapper.wrapSingleItemOperation(context.Background(), "PutItem", "table", func() (interface{}, error) {
		return 123, nil
	})
	require.NoError(t, err)
}

func TestDynamORMMetricsWrapper_WrapTransactionOperationNoConsumedCapacity(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	wrapper := NewDynamORMMetricsWrapper(&stubDynamoDBClient{}, cwm, zap.NewNop())

	_, err := wrapper.wrapTransactionOperation(context.Background(), "TransactWriteItems", 2, func() (interface{}, error) {
		return &dynamodb.TransactWriteItemsOutput{}, nil
	})
	require.NoError(t, err)
}

func TestDynamORMMetricsWrapper_WrapQueryOperationError(t *testing.T) {
	t.Parallel()

	cwm := testCloudWatchMetrics()
	wrapper := NewDynamORMMetricsWrapper(&stubDynamoDBClient{}, cwm, zap.NewNop())

	expectedErr := errors.New("query failed")
	_, err := wrapper.wrapQueryOperation(context.Background(), "Query", "table", time.Hour, func() (interface{}, error) {
		return &dynamodb.QueryOutput{}, expectedErr
	})
	require.ErrorIs(t, err, expectedErr)
}
