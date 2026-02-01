package cost

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeDynamoClient struct {
	putCalls   int
	getCalls   int
	queryCalls int

	lastPut   *dynamodb.PutItemInput
	lastGet   *dynamodb.GetItemInput
	lastQuery *dynamodb.QueryInput

	putFn   func(ctx context.Context, in *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error)
	getFn   func(ctx context.Context, in *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error)
	queryFn func(ctx context.Context, in *dynamodb.QueryInput) (*dynamodb.QueryOutput, error)
}

func (f *fakeDynamoClient) PutItem(ctx context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.putCalls++
	f.lastPut = in
	return f.putFn(ctx, in)
}

func (f *fakeDynamoClient) GetItem(ctx context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.getCalls++
	f.lastGet = in
	return f.getFn(ctx, in)
}

func (f *fakeDynamoClient) Query(ctx context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	f.queryCalls++
	f.lastQuery = in
	return f.queryFn(ctx, in)
}

func TestStorage_SaveOperationCost_WritesExpectedKeys(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	client := &fakeDynamoClient{
		putFn: func(_ context.Context, in *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			require.Equal(t, "tbl", aws.ToString(in.TableName))
			require.Contains(t, in.Item, "PK")
			require.Contains(t, in.Item, "SK")
			return &dynamodb.PutItemOutput{}, nil
		},
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{}, nil
		},
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())

	err := st.SaveOperationCost(context.Background(), &OperationCost{
		RequestID:           "req",
		OperationType:       "op",
		Timestamp:           now,
		TotalCostMicroCents: 123,
	})
	require.NoError(t, err)
	require.Equal(t, 1, client.putCalls)
}

func TestStorage_SaveOperationCost_ReturnsErrorOnPutFailure(t *testing.T) {
	client := &fakeDynamoClient{
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return nil, errors.New("boom")
		},
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{}, nil
		},
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())
	err := st.SaveOperationCost(context.Background(), &OperationCost{RequestID: "req", OperationType: "op", Timestamp: time.Now()})
	require.Error(t, err)
}

func TestStorage_GetDailyCosts_SkipsErrorsAndEmptyItems(t *testing.T) {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 3, 0, 0, 0, 0, time.UTC)

	client := &fakeDynamoClient{
		getFn: func(_ context.Context, in *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			pk := in.Key["PK"].(*types.AttributeValueMemberS).Value
			switch pk {
			case "COST_DAILY#2024-01-01":
				return nil, errors.New("boom")
			case "COST_DAILY#2024-01-02":
				return &dynamodb.GetItemOutput{Item: nil}, nil
			case "COST_DAILY#2024-01-03":
				return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
					"Date": &types.AttributeValueMemberS{Value: "2024-01-03"},
				}}, nil
			default:
				t.Fatalf("unexpected pk: %s", pk)
				return nil, nil
			}
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())
	out, err := st.GetDailyCosts(context.Background(), start, end)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, "2024-01-03", out[0].Date)
}

func TestStorage_GetMonthlyCost_ReturnsEmptyAggregateWhenMissing(t *testing.T) {
	client := &fakeDynamoClient{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: nil}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())
	agg, err := st.GetMonthlyCost(context.Background(), 2024, time.January)
	require.NoError(t, err)
	require.Equal(t, 2024, agg.Year)
	require.Equal(t, 1, agg.Month)
}

func TestStorage_GetMonthlyCost_UnmarshalsWhenPresent(t *testing.T) {
	client := &fakeDynamoClient{
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
				"Year":  &types.AttributeValueMemberN{Value: "2024"},
				"Month": &types.AttributeValueMemberN{Value: "1"},
			}}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())
	agg, err := st.GetMonthlyCost(context.Background(), 2024, time.January)
	require.NoError(t, err)
	require.Equal(t, 2024, agg.Year)
	require.Equal(t, 1, agg.Month)
}

func TestStorage_QueryCostsByDate_DelegatesAndSurfacesErrors(t *testing.T) {
	client := &fakeDynamoClient{
		queryFn: func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
			return &dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{{"PK": &types.AttributeValueMemberS{Value: "x"}}}}, nil
		},
		putFn: func(_ context.Context, _ *dynamodb.PutItemInput) (*dynamodb.PutItemOutput, error) {
			return &dynamodb.PutItemOutput{}, nil
		},
		getFn: func(_ context.Context, _ *dynamodb.GetItemInput) (*dynamodb.GetItemOutput, error) {
			return &dynamodb.GetItemOutput{}, nil
		},
	}

	st := NewStorage(client, "tbl", zap.NewNop())
	items, err := st.QueryCostsByDate(context.Background(), "2024-01-01")
	require.NoError(t, err)
	require.Len(t, items, 1)

	client.queryFn = func(_ context.Context, _ *dynamodb.QueryInput) (*dynamodb.QueryOutput, error) {
		return nil, errors.New("boom")
	}
	_, err = st.QueryCostsByDate(context.Background(), "2024-01-01")
	require.Error(t, err)
}
