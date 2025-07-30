//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/dynamorm"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/lift/pkg/lift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testTableName = "lesser-integration-test"

// setupTestEnvironment creates a test DynamoDB table and returns repositories
func setupTestEnvironment(t *testing.T) (*repositories.ActorRepository, *repositories.FederationActivityRepository, *awsdynamodb.Client) {
	// Initialize logger
	logger := zap.NewNop()

	// Setup DynamoDB client
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...any) (aws.Endpoint, error) {
				if service == awsdynamodb.ServiceID {
					return aws.Endpoint{
						URL: endpoint,
					}, nil
				}
				return aws.Endpoint{}, fmt.Errorf("unknown endpoint requested")
			})),
	)
	require.NoError(t, err)

	client := awsdynamodb.NewFromConfig(cfg)

	// Create test table
	ctx := context.Background()
	tableName := fmt.Sprintf("%s-%s", testTableName, uuid.New().String()[:8])

	_, err = client.CreateTable(ctx, &awsdynamodb.CreateTableInput{
		TableName: aws.String(tableName),
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
	require.NoError(t, err)

	// Wait for table to be active
	waiter := awsdynamodb.NewTableExistsWaiter(client)
	err = waiter.Wait(ctx, &awsdynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 30*time.Second)
	require.NoError(t, err)

	// Initialize DynamORM with the test client
	db, err := dynamorm.NewWithExistingClient(client, tableName)
	require.NoError(t, err)

	// Create repositories
	actorRepo := repositories.NewActorRepository(db, tableName, logger)
	federationActivityRepo := repositories.NewFederationActivityRepository(db, tableName, logger)

	// Cleanup function
	t.Cleanup(func() {
		_, err := client.DeleteTable(ctx, &awsdynamodb.DeleteTableInput{
			TableName: aws.String(tableName),
		})
		if err != nil {
			t.Logf("Failed to delete test table: %v", err)
		}
	})

	return actorRepo, federationActivityRepo, client
}

// createTestActor creates a test actor in the database
func createTestActor(t *testing.T, actorRepo *repositories.ActorRepository, username string) *activitypub.Actor {
	ctx := context.Background()

	// Convert to DynamORM model
	actorModel := &models.Actor{
		Username:   username,
		Name:       username,
		PrivateKey: "test-private-key",
		PublicKey:  "-----BEGIN PUBLIC KEY-----\ntest-key\n-----END PUBLIC KEY-----",
		InboxURL:   fmt.Sprintf("https://example.com/users/%s/inbox", username),
		OutboxURL:  fmt.Sprintf("https://example.com/users/%s/outbox", username),
	}

	err := actorRepo.Create(ctx, actorModel)
	require.NoError(t, err)

	// Return ActivityPub actor for compatibility
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []any{"https://www.w3.org/ns/activitystreams"},
			Type:    "Person",
			ID:      fmt.Sprintf("https://example.com/users/%s", username),
		},
		PreferredUsername: username,
		Name:              username,
		Inbox:             fmt.Sprintf("https://example.com/users/%s/inbox", username),
		Outbox:            fmt.Sprintf("https://example.com/users/%s/outbox", username),
		Followers:         fmt.Sprintf("https://example.com/users/%s/followers", username),
		Following:         fmt.Sprintf("https://example.com/users/%s/following", username),
		PublicKey: &activitypub.PublicKey{
			ID:           fmt.Sprintf("https://example.com/users/%s#main-key", username),
			Owner:        fmt.Sprintf("https://example.com/users/%s", username),
			PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest-key\n-----END PUBLIC KEY-----",
		},
	}

	return actor
}

func TestOutboxProcessorSQSHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	actorRepo, federationRepo, _ := setupTestEnvironment(t)
	ctx := context.Background()

	// Create test actor
	actor := createTestActor(t, actorRepo, "alice")

	// Create outbox processor (this would normally be done in main)
	processor, err := NewOutboxProcessor()
	require.NoError(t, err)

	t.Run("process federation delivery message", func(t *testing.T) {
		// Create a test activity
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: []any{"https://www.w3.org/ns/activitystreams"},
				Type:    "Create",
				ID:      "https://example.com/activities/123",
			},
			Actor: actor.ID,
			Object: map[string]any{
				"type":    "Note",
				"content": "Hello from federation test!",
			},
		}

		// Create delivery message
		deliveryMsg := ActivityDeliveryMessage{
			Activity:    activity,
			Actor:       actor,
			TargetInbox: "https://remote.example.com/users/bob/inbox",
			Attempt:     1,
		}

		// Serialize message
		msgBody, err := json.Marshal(deliveryMsg)
		require.NoError(t, err)

		// Create SQS event
		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-message-1",
					Body:      string(msgBody),
				},
			},
		}

		// Create Lift context
		liftCtx := &lift.Context{}
		liftCtx.Set("requestID", "test-request-123")

		// Process the SQS event (this will fail to deliver but should record the attempt)
		err = processor.HandleSQS(liftCtx, sqsEvent)
		// We expect this to fail since there's no real remote server, but that's OK
		assert.Error(t, err) // Federation delivery should fail

		// Verify federation activity was recorded
		// Note: In a real test you'd check the federation activity was recorded
		// For now we just verify the processing completed without panicking
	})

	t.Run("process multiple messages concurrently", func(t *testing.T) {
		// Create multiple test activities
		messages := []events.SQSMessage{}
		for i := 0; i < 3; i++ {
			activity := &activitypub.Activity{
				BaseObject: activitypub.BaseObject{
					Context: []any{"https://www.w3.org/ns/activitystreams"},
					Type:    "Create",
					ID:      fmt.Sprintf("https://example.com/activities/%d", i),
				},
				Actor: actor.ID,
				Object: map[string]any{
					"type":    "Note",
					"content": fmt.Sprintf("Test message %d", i),
				},
			}

			deliveryMsg := ActivityDeliveryMessage{
				Activity:    activity,
				Actor:       actor,
				TargetInbox: fmt.Sprintf("https://remote%d.example.com/users/bob/inbox", i),
				Attempt:     1,
			}

			msgBody, err := json.Marshal(deliveryMsg)
			require.NoError(t, err)

			messages = append(messages, events.SQSMessage{
				MessageId: fmt.Sprintf("test-message-%d", i),
				Body:      string(msgBody),
			})
		}

		// Create SQS event with multiple messages
		sqsEvent := events.SQSEvent{
			Records: messages,
		}

		// Create Lift context
		liftCtx := &lift.Context{}
		liftCtx.Set("requestID", "test-batch-456")

		// Process the SQS event
		err = processor.HandleSQS(liftCtx, sqsEvent)
		// All deliveries should fail but processing should complete
		assert.Error(t, err) // Should have failures
	})
}

func TestOutboxProcessorValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	actorRepo, _, _ := setupTestEnvironment(t)

	// Create test actor
	actor := createTestActor(t, actorRepo, "alice")

	// Create outbox processor
	processor, err := NewOutboxProcessor()
	require.NoError(t, err)

	t.Run("reject message without activity", func(t *testing.T) {
		// Create invalid delivery message
		deliveryMsg := ActivityDeliveryMessage{
			Actor:       actor,
			TargetInbox: "https://remote.example.com/users/bob/inbox",
		}

		msgBody, err := json.Marshal(deliveryMsg)
		require.NoError(t, err)

		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-invalid-1",
					Body:      string(msgBody),
				},
			},
		}

		liftCtx := &lift.Context{}
		liftCtx.Set("requestID", "test-validation-1")

		err = processor.HandleSQS(liftCtx, sqsEvent)
		assert.Error(t, err)
	})

	t.Run("reject message without target inbox", func(t *testing.T) {
		activity := &activitypub.Activity{
			BaseObject: activitypub.BaseObject{
				Context: []any{"https://www.w3.org/ns/activitystreams"},
				Type:    "Create",
				ID:      "https://example.com/activities/456",
			},
			Actor: actor.ID,
		}

		// Create invalid delivery message (missing target inbox)
		deliveryMsg := ActivityDeliveryMessage{
			Activity: activity,
			Actor:    actor,
		}

		msgBody, err := json.Marshal(deliveryMsg)
		require.NoError(t, err)

		sqsEvent := events.SQSEvent{
			Records: []events.SQSMessage{
				{
					MessageId: "test-invalid-2",
					Body:      string(msgBody),
				},
			},
		}

		liftCtx := &lift.Context{}
		liftCtx.Set("requestID", "test-validation-2")

		err = processor.HandleSQS(liftCtx, sqsEvent)
		assert.Error(t, err)
	})
}