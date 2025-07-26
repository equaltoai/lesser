//go:build integration
// +build integration

package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testTableName = "lesser-test"

// setupTestTable creates a test table for integration tests
func setupTestTable(t *testing.T, client *dynamodb.Client) {
	ctx := context.Background()

	// Create table
	_, err := client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(testTableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("PK"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("SK"),
				KeyType:       types.KeyTypeRange,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("PK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("SK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("GSI1PK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("GSI1SK"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("GSI1"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("GSI1PK"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("GSI1SK"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		// Table might already exist
		var rie *types.ResourceInUseException
		if !errors.As(err, &rie) {
			t.Fatalf("failed to create test table: %v", err)
		}
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(testTableName),
	}, 30*time.Second)
	require.NoError(t, err)
}

// cleanupTestTable deletes all items from the test table
func cleanupTestTable(t *testing.T, client *dynamodb.Client) {
	ctx := context.Background()

	// Scan and delete all items
	result, err := client.Scan(ctx, &dynamodb.ScanInput{
		TableName: aws.String(testTableName),
	})
	require.NoError(t, err)

	for _, item := range result.Items {
		_, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
			TableName: aws.String(testTableName),
			Key: map[string]types.AttributeValue{
				"PK": item["PK"],
				"SK": item["SK"],
			},
		})
		require.NoError(t, err)
	}
}

func TestActorIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup DynamoDB client
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == dynamodb.ServiceID {
					return aws.Endpoint{
						URL: endpoint,
					}, nil
				}
				return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
			})),
	)
	require.NoError(t, err)

	client := dynamodb.NewFromConfig(cfg)
	setupTestTable(t, client)
	defer cleanupTestTable(t, client)

	// Create storage instance
	storage := NewWithClient(client, testTableName)

	// Test actor lifecycle
	t.Run("actor lifecycle", func(t *testing.T) {
		ctx := context.Background()

		// Create actor
		actor := createTestActor("testuser")
		privateKey := "test-private-key"

		err := storage.CreateActor(ctx, actor, privateKey)
		assert.NoError(t, err)

		// Get actor
		retrievedActor, err := storage.GetActor(ctx, "testuser")
		assert.NoError(t, err)
		assert.Equal(t, actor.ID, retrievedActor.ID)
		assert.Equal(t, actor.PreferredUsername, retrievedActor.PreferredUsername)

		// Get private key
		retrievedKey, err := storage.GetActorPrivateKey(ctx, "testuser")
		assert.NoError(t, err)
		assert.Equal(t, privateKey, retrievedKey)

		// Update actor
		actor.Name = "Updated Name"
		err = storage.UpdateActor(ctx, actor)
		assert.NoError(t, err)

		// Verify update
		updatedActor, err := storage.GetActor(ctx, "testuser")
		assert.NoError(t, err)
		assert.Equal(t, "Updated Name", updatedActor.Name)

		// Delete actor
		err = storage.DeleteActor(ctx, "testuser")
		assert.NoError(t, err)

		// Verify deletion
		_, err = storage.GetActor(ctx, "testuser")
		assert.Error(t, err)
		assert.IsType(t, common.ActorNotFoundError{}, err)
	})

	t.Run("duplicate actor creation", func(t *testing.T) {
		ctx := context.Background()

		// Create first actor
		actor := createTestActor("duplicate")
		err := storage.CreateActor(ctx, actor, "key1")
		assert.NoError(t, err)

		// Try to create duplicate
		err = storage.CreateActor(ctx, actor, "key2")
		assert.Error(t, err)
		assert.IsType(t, common.ConflictError{}, err)
	})
}

func TestActivityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup DynamoDB client
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == dynamodb.ServiceID {
					return aws.Endpoint{
						URL: endpoint,
					}, nil
				}
				return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
			})),
	)
	require.NoError(t, err)

	client := dynamodb.NewFromConfig(cfg)
	setupTestTable(t, client)
	defer cleanupTestTable(t, client)

	// Create storage instance
	storage := NewWithClient(client, testTableName)

	t.Run("activity outbox", func(t *testing.T) {
		ctx := context.Background()

		// Create test activities
		for i := 0; i < 5; i++ {
			activity := &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					ID:   fmt.Sprintf("https://example.com/activities/%d", i),
					Type: activitypub.CreateType,
				},
				Actor:  "https://example.com/users/alice",
				Object: map[string]any{"content": fmt.Sprintf("Test content %d", i)},
			}

			err := storage.CreateActivity(ctx, activity)
			assert.NoError(t, err)

			// Small delay to ensure different timestamps
			time.Sleep(10 * time.Millisecond)
		}

		// Get outbox activities with pagination
		activities, cursor, err := storage.GetOutboxActivities(ctx, "alice", 3, "")
		assert.NoError(t, err)
		assert.Len(t, activities, 3)
		assert.NotEmpty(t, cursor)

		// Verify order (newest first)
		assert.Equal(t, "https://example.com/activities/4", activities[0].ID)
		assert.Equal(t, "https://example.com/activities/3", activities[1].ID)
		assert.Equal(t, "https://example.com/activities/2", activities[2].ID)

		// Get next page
		activities2, cursor2, err := storage.GetOutboxActivities(ctx, "alice", 3, cursor)
		assert.NoError(t, err)
		assert.Len(t, activities2, 2)
		assert.Empty(t, cursor2) // No more pages

		assert.Equal(t, "https://example.com/activities/1", activities2[0].ID)
		assert.Equal(t, "https://example.com/activities/0", activities2[1].ID)
	})
}
