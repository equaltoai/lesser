package testutil

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	dynamodbsvc "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/stretchr/testify/require"
)

// GetTestDynamoDBClient returns a DynamoDB storage client for testing
func GetTestDynamoDBClient(t *testing.T) storage.Storage {
	// Setup DynamoDB client
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == dynamodbsvc.ServiceID {
					return aws.Endpoint{
						URL: endpoint,
					}, nil
				}
				return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
			})),
	)
	require.NoError(t, err)

	client := dynamodbsvc.NewFromConfig(cfg)

	// Use test table name
	tableName := "lesser-test"
	if envTable := os.Getenv("DYNAMODB_TEST_TABLE"); envTable != "" {
		tableName = envTable
	}

	return dynamodb.NewWithClient(client, tableName)
}
