//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	awsdynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

const testTableName = "lesser-integration-test"

// setupTestEnvironment creates a test DynamoDB table and returns a storage instance
func setupTestEnvironment(t *testing.T) (*dynamodb.DynamoDBStorage, *awsdynamodb.Client) {
	// Initialize logger
	logger = zap.NewNop()

	// Setup DynamoDB client
	endpoint := os.Getenv("DYNAMODB_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://localhost:8000"
	}

	cfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, options ...interface{}) (aws.Endpoint, error) {
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

	// Create storage instance
	storage := dynamodb.NewWithClient(client, tableName)

	// Set the global storage variable for the handler
	store = storage

	// Cleanup function
	t.Cleanup(func() {
		_, err := client.DeleteTable(ctx, &awsdynamodb.DeleteTableInput{
			TableName: aws.String(tableName),
		})
		if err != nil {
			t.Logf("Failed to delete test table: %v", err)
		}
	})

	return storage, client
}

// createTestActor creates a test actor in the database
func createTestActor(t *testing.T, storage *dynamodb.DynamoDBStorage, username string) *activitypub.Actor {
	ctx := context.Background()

	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []interface{}{"https://www.w3.org/ns/activitystreams"},
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

	err := storage.CreateActor(ctx, actor, "test-private-key")
	require.NoError(t, err)

	return actor
}

func TestCreateActivityFullFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	storage, _ := setupTestEnvironment(t)
	ctx := context.Background()

	// Create test actor
	actor := createTestActor(t, storage, "alice")

	t.Run("create note flow", func(t *testing.T) {
		// Step 1: POST Create activity to outbox
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			"object": map[string]interface{}{
				"type":    "Note",
				"content": "Hello from integration test!",
			},
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		// Call the handler
		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		// Parse the response
		var createdActivity activitypub.Activity
		err = json.Unmarshal([]byte(response.Body), &createdActivity)
		assert.NoError(t, err)
		assert.Equal(t, "Create", createdActivity.Type)
		assert.NotEmpty(t, createdActivity.ID)

		// Extract the object ID
		obj, ok := createdActivity.Object.(map[string]interface{})
		assert.True(t, ok)
		objectID, ok := obj["id"].(string)
		assert.True(t, ok)
		assert.NotEmpty(t, objectID)

		// Step 2: Verify activity was stored
		storedActivity, err := storage.GetActivity(ctx, createdActivity.ID)
		assert.NoError(t, err)
		assert.Equal(t, createdActivity.ID, storedActivity.ID)
		assert.Equal(t, "Create", storedActivity.Type)

		// Step 3: Simulate activity processor extracting the object
		// In production, this would be done by the activity processor Lambda
		// For testing, we'll extract and store the object directly
		if createType, ok := storedActivity.Type.(string); ok && createType == "Create" {
			if objMap, ok := storedActivity.Object.(map[string]interface{}); ok {
				err = storage.CreateObject(ctx, objMap)
				assert.NoError(t, err)
			}
		}

		// Step 4: Verify object can be retrieved
		retrievedObj, err := storage.GetObject(ctx, objectID)
		assert.NoError(t, err)
		assert.NotNil(t, retrievedObj)

		objMap, ok := retrievedObj.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Note", objMap["type"])
		assert.Equal(t, "Hello from integration test!", objMap["content"])
		assert.Equal(t, actor.ID, objMap["attributedTo"])

		// Step 5: Verify activity appears in outbox
		activities, nextCursor, err := storage.GetOutboxActivities(ctx, "alice", 10, "")
		assert.NoError(t, err)
		assert.Len(t, activities, 1)
		assert.Empty(t, nextCursor)
		assert.Equal(t, createdActivity.ID, activities[0].ID)
	})

	t.Run("create note with rich content", func(t *testing.T) {
		// Test with attachments, tags, and multi-language content
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			"object": map[string]interface{}{
				"type":    "Note",
				"content": "Check out this photo! #photography",
				"contentMap": map[string]interface{}{
					"en": "Check out this photo! #photography",
					"es": "¡Mira esta foto! #fotografía",
				},
				"attachment": []interface{}{
					map[string]interface{}{
						"type":      "Image",
						"url":       "https://example.com/photo.jpg",
						"mediaType": "image/jpeg",
						"name":      "A beautiful sunset",
					},
				},
				"tag": []interface{}{
					map[string]interface{}{
						"type": "Hashtag",
						"name": "#photography",
						"href": "https://example.com/tags/photography",
					},
				},
			},
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		var createdActivity activitypub.Activity
		err = json.Unmarshal([]byte(response.Body), &createdActivity)
		assert.NoError(t, err)

		// Verify the object has all the rich content
		obj, ok := createdActivity.Object.(map[string]interface{})
		assert.True(t, ok)

		// Check contentMap
		contentMap, ok := obj["contentMap"].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Check out this photo! #photography", contentMap["en"])
		assert.Equal(t, "¡Mira esta foto! #fotografía", contentMap["es"])

		// Check attachments
		attachments, ok := obj["attachment"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, attachments, 1)

		attachment, ok := attachments[0].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Image", attachment["type"])
		assert.Equal(t, "https://example.com/photo.jpg", attachment["url"])

		// Check tags
		tags, ok := obj["tag"].([]interface{})
		assert.True(t, ok)
		assert.Len(t, tags, 1)

		tag, ok := tags[0].(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Hashtag", tag["type"])
		assert.Equal(t, "#photography", tag["name"])
	})

	t.Run("create article", func(t *testing.T) {
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			"object": map[string]interface{}{
				"type":    "Article",
				"name":    "My First Article",
				"content": "This is the content of my article...",
				"summary": "A brief summary of the article",
			},
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusCreated, response.StatusCode)

		var createdActivity activitypub.Activity
		err = json.Unmarshal([]byte(response.Body), &createdActivity)
		assert.NoError(t, err)

		obj, ok := createdActivity.Object.(map[string]interface{})
		assert.True(t, ok)
		assert.Equal(t, "Article", obj["type"])
		assert.Equal(t, "My First Article", obj["name"])
		assert.Equal(t, "This is the content of my article...", obj["content"])
		assert.Equal(t, "A brief summary of the article", obj["summary"])
	})

	t.Run("multiple creates maintain order", func(t *testing.T) {
		// Create multiple notes
		noteContents := []string{
			"First note",
			"Second note",
			"Third note",
		}

		createdIDs := make([]string, 0, len(noteContents))

		for _, content := range noteContents {
			createActivity := map[string]interface{}{
				"@context": "https://www.w3.org/ns/activitystreams",
				"type":     "Create",
				"object": map[string]interface{}{
					"type":    "Note",
					"content": content,
				},
			}

			body, err := json.Marshal(createActivity)
			require.NoError(t, err)

			request := events.APIGatewayV2HTTPRequest{
				HTTPMethod: http.MethodPost,
				PathParameters: map[string]string{
					"username": "alice",
				},
				Body: string(body),
				Headers: map[string]string{
					"Content-Type": "application/activity+json",
				},
			}

			response, err := handler(ctx, request)
			assert.NoError(t, err)
			assert.Equal(t, http.StatusCreated, response.StatusCode)

			var createdActivity activitypub.Activity
			err = json.Unmarshal([]byte(response.Body), &createdActivity)
			assert.NoError(t, err)

			createdIDs = append(createdIDs, createdActivity.ID)

			// Small delay to ensure different timestamps
			time.Sleep(10 * time.Millisecond)
		}

		// Verify outbox order (newest first)
		activities, _, err := storage.GetOutboxActivities(ctx, "alice", 10, "")
		assert.NoError(t, err)

		// The most recent activity should be first
		assert.Equal(t, createdIDs[2], activities[0].ID) // Third note
		assert.Equal(t, createdIDs[1], activities[1].ID) // Second note
		assert.Equal(t, createdIDs[0], activities[2].ID) // First note
	})
}

func TestCreateActivityValidation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	storage, _ := setupTestEnvironment(t)
	ctx := context.Background()

	// Create test actor
	_ = createTestActor(t, storage, "alice")

	t.Run("reject create without object", func(t *testing.T) {
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			// Missing object
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Contains(t, response.Body, "object is required")
	})

	t.Run("reject note without content", func(t *testing.T) {
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			"object": map[string]interface{}{
				"type": "Note",
				// Missing content
			},
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Contains(t, response.Body, "content is required for Note")
	})

	t.Run("reject article without name", func(t *testing.T) {
		createActivity := map[string]interface{}{
			"@context": "https://www.w3.org/ns/activitystreams",
			"type":     "Create",
			"object": map[string]interface{}{
				"type":    "Article",
				"content": "Article content",
				// Missing name
			},
		}

		body, err := json.Marshal(createActivity)
		require.NoError(t, err)

		request := events.APIGatewayV2HTTPRequest{
			HTTPMethod: http.MethodPost,
			PathParameters: map[string]string{
				"username": "alice",
			},
			Body: string(body),
			Headers: map[string]string{
				"Content-Type": "application/activity+json",
			},
		}

		response, err := handler(ctx, request)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Contains(t, response.Body, "name is required for Article")
	})
}
