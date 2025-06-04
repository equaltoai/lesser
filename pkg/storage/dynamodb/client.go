package dynamodb

import (
	"context"
	"fmt"
	"os"
	"sync"

	"github.com/aron23/lesser/pkg/common"
	cfg "github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"go.uber.org/zap"
)

// DynamoDBAPI defines the subset of DynamoDB operations we use
type DynamoDBAPI interface {
	PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error)
	GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error)
	UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error)
	DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error)
	Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error)
	Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error)
	BatchWriteItem(ctx context.Context, params *dynamodb.BatchWriteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.BatchWriteItemOutput, error)
}

// dynamoDBStorage implements the storage.Storage interface using DynamoDB
type dynamoDBStorage struct {
	client    DynamoDBAPI
	tableName string
}

var (
	// globalClient is reused across Lambda invocations
	globalClient DynamoDBAPI
	clientOnce   sync.Once
	clientErr    error
)

// init initializes the global DynamoDB client for Lambda reuse
func init() {
	// Skip initialization in test mode
	if os.Getenv("GO_ENV") == "test" {
		return
	}

	// Pre-initialize the client in Lambda environment
	if cfg.Get().DynamoTableName != "" {
		_, _ = getClient()
	}
}

// getClient returns the global DynamoDB client, initializing it if needed
func getClient() (DynamoDBAPI, error) {
	clientOnce.Do(func() {
		ctx := context.Background()
		awsCfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(cfg.Get().Region),
		)
		if err != nil {
			clientErr = fmt.Errorf("failed to load AWS config: %w", err)
			return
		}

		globalClient = dynamodb.NewFromConfig(awsCfg)
		common.Logger().Info("DynamoDB client initialized",
			zap.String("region", cfg.Get().Region),
		)
	})

	return globalClient, clientErr
}

// New creates a new DynamoDB storage instance
func New() (storage.Storage, error) {
	client, err := getClient()
	if err != nil {
		return nil, err
	}

	return &dynamoDBStorage{
		client:    client,
		tableName: cfg.Get().DynamoTableName,
	}, nil
}

// NewWithClient creates a new DynamoDB storage instance with a custom client (for testing)
func NewWithClient(client DynamoDBAPI, tableName string) storage.Storage {
	return &dynamoDBStorage{
		client:    client,
		tableName: tableName,
	}
}

// getTableName returns the table name with optional override for testing
func (s *dynamoDBStorage) getTableName() *string {
	return aws.String(s.tableName)
}

// GetCollection is implemented in collection.go
