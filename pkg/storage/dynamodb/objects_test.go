package dynamodb

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const objectsTestTableName = "test-table"

func TestCreateObject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to known issues with DynamoDB mocking")
	}
	ctx := context.Background()
	mockClient := new(mocks.MockDynamoDBClient)
	storage := NewWithClient(mockClient, "test-table")

	t.Run("success", func(t *testing.T) {
		obj := &Object{
			ID:           "https://example.com/objects/123",
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Hello, world!",
			Published:    time.Now(),
			To:           []string{"https://www.w3.org/ns/activitystreams#Public"},
			CC:           []string{"https://example.com/users/alice/followers"},
			URL:          "https://example.com/objects/123",
		}

		mockClient.On("PutItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			return *input.TableName == "test-table" &&
				*input.ConditionExpression == "attribute_not_exists(PK)" &&
				input.Item["PK"].(*types.AttributeValueMemberS).Value == "OBJECT#https://example.com/objects/123" &&
				input.Item["SK"].(*types.AttributeValueMemberS).Value == "METADATA" &&
				input.Item["GSI1PK"].(*types.AttributeValueMemberS).Value == "ACTOR#alice#OBJECTS"
		})).Return(&dynamodb.PutItemOutput{}, nil).Once()

		err := storage.CreateObject(ctx, obj)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("validation errors", func(t *testing.T) {
		tests := []struct {
			name string
			obj  *Object
			err  string
		}{
			{
				name: "missing ID",
				obj: &Object{
					Type:         "Note",
					AttributedTo: "https://example.com/users/alice",
				},
				err: "object ID is required",
			},
			{
				name: "missing type",
				obj: &Object{
					ID:           "https://example.com/objects/123",
					AttributedTo: "https://example.com/users/alice",
				},
				err: "object type is required",
			},
			{
				name: "missing attributedTo",
				obj: &Object{
					ID:   "https://example.com/objects/123",
					Type: "Note",
				},
				err: "attributedTo is required",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				err := storage.CreateObject(ctx, tt.obj)
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.err)
			})
		}
	})

	t.Run("already exists", func(t *testing.T) {
		obj := &Object{
			ID:           "https://example.com/objects/123",
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Hello, world!",
			Published:    time.Now(),
		}

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("PutItem", mock.Anything, mock.Anything).Return(
			nil, &types.ConditionalCheckFailedException{
				Message: aws.String("The conditional request failed"),
			},
		).Once()

		storage := NewWithClient(mockClient, "test-table")
		err := storage.CreateObject(ctx, obj)
		assert.Error(t, err)
		conflictErr, ok := err.(common.ConflictError)
		assert.True(t, ok)
		assert.Contains(t, conflictErr.Message, "already exists")
	})
}

func TestGetObject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to known issues with DynamoDB mocking")
	}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		objectID := "https://example.com/objects/123"
		expectedObj := &Object{
			ID:           objectID,
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Hello, world!",
			Published:    time.Now(),
		}

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("GetItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Key["PK"].(*types.AttributeValueMemberS).Value == "OBJECT#https://example.com/objects/123" &&
				input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA"
		})).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"PK": &types.AttributeValueMemberS{Value: "OBJECT#" + objectID},
				"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				"Object": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"ID":           &types.AttributeValueMemberS{Value: expectedObj.ID},
						"Type":         &types.AttributeValueMemberS{Value: expectedObj.Type},
						"AttributedTo": &types.AttributeValueMemberS{Value: expectedObj.AttributedTo},
						"Content":      &types.AttributeValueMemberS{Value: expectedObj.Content},
						"Published":    &types.AttributeValueMemberS{Value: expectedObj.Published.Format(time.RFC3339)},
					},
				},
			},
		}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		obj, err := storage.GetObject(ctx, objectID)
		assert.NoError(t, err)
		assert.NotNil(t, obj)

		objTyped, ok := obj.(*Object)
		require.True(t, ok)
		assert.Equal(t, expectedObj.ID, objTyped.ID)
		assert.Equal(t, expectedObj.Type, objTyped.Type)
		assert.Equal(t, expectedObj.Content, objTyped.Content)
		mockClient.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{Item: nil}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		obj, err := storage.GetObject(ctx, "https://example.com/objects/456")
		assert.Error(t, err)
		assert.Nil(t, obj)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestUpdateObject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows due to known issues with DynamoDB mocking")
	}
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		obj := &Object{
			ID:           "https://example.com/objects/123",
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Updated content!",
			Published:    time.Now(),
		}

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("UpdateItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.UpdateItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Key["PK"].(*types.AttributeValueMemberS).Value == "OBJECT#https://example.com/objects/123" &&
				input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA" &&
				*input.ConditionExpression == "attribute_exists(PK)"
		})).Return(&dynamodb.UpdateItemOutput{}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		err := storage.UpdateObject(ctx, obj)
		assert.NoError(t, err)
		assert.NotNil(t, obj.Updated)
		mockClient.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		obj := &Object{
			ID: "https://example.com/objects/456",
		}

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("UpdateItem", mock.Anything, mock.Anything).Return(
			nil, &types.ConditionalCheckFailedException{
				Message: aws.String("The conditional request failed"),
			},
		).Once()

		storage := NewWithClient(mockClient, "test-table")
		err := storage.UpdateObject(ctx, obj)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestDeleteObject(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		objectID := "https://example.com/objects/123"

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("DeleteItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
			return *input.TableName == "test-table" &&
				input.Key["PK"].(*types.AttributeValueMemberS).Value == "OBJECT#https://example.com/objects/123" &&
				input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA" &&
				*input.ConditionExpression == "attribute_exists(PK)"
		})).Return(&dynamodb.DeleteItemOutput{}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		err := storage.DeleteObject(ctx, objectID)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("not found", func(t *testing.T) {
		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("DeleteItem", mock.Anything, mock.Anything).Return(
			nil, &types.ConditionalCheckFailedException{
				Message: aws.String("The conditional request failed"),
			},
		).Once()

		storage := NewWithClient(mockClient, "test-table")
		err := storage.DeleteObject(ctx, "https://example.com/objects/456")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestGetObjectsByActor(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		actorID := "https://example.com/users/alice"

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
			return *input.TableName == "test-table" &&
				*input.IndexName == "GSI1" &&
				input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value == "ACTOR#alice#OBJECTS" &&
				*input.ScanIndexForward == false // Newest first
		})).Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"GSI1PK": &types.AttributeValueMemberS{Value: "ACTOR#alice#OBJECTS"},
					"GSI1SK": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
					"Object": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"ID":           &types.AttributeValueMemberS{Value: "https://example.com/objects/1"},
							"Type":         &types.AttributeValueMemberS{Value: "Note"},
							"Content":      &types.AttributeValueMemberS{Value: "First note"},
							"AttributedTo": &types.AttributeValueMemberS{Value: actorID},
							"Published":    &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						},
					},
				},
				{
					"GSI1PK": &types.AttributeValueMemberS{Value: "ACTOR#alice#OBJECTS"},
					"GSI1SK": &types.AttributeValueMemberS{Value: time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
					"Object": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"ID":           &types.AttributeValueMemberS{Value: "https://example.com/objects/2"},
							"Type":         &types.AttributeValueMemberS{Value: "Article"},
							"Name":         &types.AttributeValueMemberS{Value: "My Article"},
							"AttributedTo": &types.AttributeValueMemberS{Value: actorID},
							"Published":    &types.AttributeValueMemberS{Value: time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
						},
					},
				},
			},
		}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		objects, cursor, err := storage.GetObjectsByActor(ctx, actorID, "", 10)
		assert.NoError(t, err)
		assert.Len(t, objects, 2)
		assert.Empty(t, cursor) // No next page

		// Verify first object
		obj1, ok := objects[0].(*Object)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/objects/1", obj1.ID)
		assert.Equal(t, "Note", obj1.Type)

		// Verify second object
		obj2, ok := objects[1].(*Object)
		require.True(t, ok)
		assert.Equal(t, "https://example.com/objects/2", obj2.ID)
		assert.Equal(t, "Article", obj2.Type)
		mockClient.AssertExpectations(t)
	})

	t.Run("with pagination", func(t *testing.T) {
		actorID := "https://example.com/users/alice"
		limit := 1

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("Query", mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
			return *input.Limit == int32(limit+1) // Requesting one extra
		})).Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"Object": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"ID":        &types.AttributeValueMemberS{Value: "https://example.com/objects/1"},
							"Type":      &types.AttributeValueMemberS{Value: "Note"},
							"Published": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						},
					},
				},
				{
					"Object": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"ID":        &types.AttributeValueMemberS{Value: "https://example.com/objects/2"},
							"Type":      &types.AttributeValueMemberS{Value: "Note"},
							"Published": &types.AttributeValueMemberS{Value: time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
						},
					},
				},
			},
		}, nil).Once()

		storage := NewWithClient(mockClient, "test-table")
		objects, cursor, err := storage.GetObjectsByActor(ctx, actorID, "", limit)
		assert.NoError(t, err)
		assert.Len(t, objects, limit) // Should only return the limit
		assert.NotEmpty(t, cursor)    // Should have a cursor for the next page
	})

	t.Run("invalid actor ID", func(t *testing.T) {
		storage := NewWithClient(new(mocks.MockDynamoDBClient), "test-table")
		objects, cursor, err := storage.GetObjectsByActor(ctx, "invalid-actor-id", "", 10)
		assert.Error(t, err)
		assert.Nil(t, objects)
		assert.Empty(t, cursor)
		assert.Contains(t, err.Error(), "invalid actor ID format")
	})
}

func TestExtractUsernameFromActorID(t *testing.T) {
	tests := []struct {
		name     string
		actorID  string
		expected string
	}{
		{
			name:     "standard format",
			actorID:  "https://example.com/users/alice",
			expected: "alice",
		},
		{
			name:     "with port",
			actorID:  "https://example.com:8080/users/bob",
			expected: "bob",
		},
		{
			name:     "with path after username",
			actorID:  "https://example.com/users/charlie/profile",
			expected: "charlie",
		},
		{
			name:     "invalid format - no users segment",
			actorID:  "https://example.com/actors/alice",
			expected: "",
		},
		{
			name:     "invalid format - empty",
			actorID:  "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractUsernameFromActorID(tt.actorID)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestTombstoneObject(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		objectID  string
		deletedBy string
		setupMock func(*mocks.MockDynamoDBClient)
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "successful tombstone creation",
			objectID:  "https://example.com/objects/post1",
			deletedBy: "https://example.com/users/alice",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// GetObject call
				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					return input.Key["SK"].(*types.AttributeValueMemberS).Value == "TOMBSTONE"
				})).Return(&dynamodb.GetItemOutput{}, nil).Once()

				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					return input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1"},
						"SK": &types.AttributeValueMemberS{Value: "METADATA"},
						"Object": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"id":           &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
								"type":         &types.AttributeValueMemberS{Value: "Note"},
								"content":      &types.AttributeValueMemberS{Value: "Hello world"},
								"attributedTo": &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
								"published":    &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
							},
						},
					},
				}, nil).Once()

				// Create tombstone
				m.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return input.Item["SK"].(*types.AttributeValueMemberS).Value == "TOMBSTONE"
				})).Return(&dynamodb.PutItemOutput{}, nil)

				// Delete original
				m.On("DeleteItem", ctx, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
					return input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA"
				})).Return(&dynamodb.DeleteItemOutput{}, nil)

				// Cascade delete likes
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#LIKES"
				})).Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)

				// Cascade delete announces
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#ANNOUNCES"
				})).Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)
			},
			wantErr: false,
		},
		{
			name:      "object not found",
			objectID:  "https://example.com/objects/nonexistent",
			deletedBy: "https://example.com/users/alice",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// GetObject returns not found
				m.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
			},
			wantErr: true,
			errMsg:  "failed to get object for tombstoning",
		},
		{
			name:      "cascade delete likes with items",
			objectID:  "https://example.com/objects/post1",
			deletedBy: "https://example.com/users/alice",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// GetObject call
				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					return input.Key["SK"].(*types.AttributeValueMemberS).Value == "TOMBSTONE"
				})).Return(&dynamodb.GetItemOutput{}, nil).Once()

				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					return input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1"},
						"SK": &types.AttributeValueMemberS{Value: "METADATA"},
						"Object": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"id":   &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
								"type": &types.AttributeValueMemberS{Value: "Note"},
							},
						},
					},
				}, nil).Once()

				// Create tombstone
				m.On("PutItem", ctx, mock.Anything).Return(&dynamodb.PutItemOutput{}, nil)

				// Delete original - allow it to be called
				m.On("DeleteItem", ctx, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
					return true // Accept any delete
				})).Return(&dynamodb.DeleteItemOutput{}, nil).Maybe()

				// Cascade delete likes with items
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#LIKES"
				})).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1#LIKES"},
							"SK": &types.AttributeValueMemberS{Value: "ACTOR#https://example.com/users/bob"},
						},
					},
				}, nil).Once()

				// Cascade delete announces
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#ANNOUNCES"
				})).Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: objectsTestTableName,
			}

			err := s.TombstoneObject(ctx, tt.objectID, tt.deletedBy)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetTombstone(t *testing.T) {
	ctx := context.Background()
	testTime := time.Now()

	tests := []struct {
		name          string
		objectID      string
		setupMock     func(*mocks.MockDynamoDBClient)
		wantTombstone *storage.Tombstone
		wantErr       bool
		errMsg        string
	}{
		{
			name:     "existing tombstone",
			objectID: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					pk := input.Key["PK"].(*types.AttributeValueMemberS).Value
					sk := input.Key["SK"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1" && sk == "TOMBSTONE"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1"},
						"SK": &types.AttributeValueMemberS{Value: "TOMBSTONE"},
						"Tombstone": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"id":         &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
								"type":       &types.AttributeValueMemberS{Value: "Tombstone"},
								"formerType": &types.AttributeValueMemberS{Value: "Note"},
								"deleted":    &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
								"deletedBy":  &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
							},
						},
					},
				}, nil)
			},
			wantTombstone: &storage.Tombstone{
				ID:         "https://example.com/objects/post1",
				Type:       "Tombstone",
				FormerType: "Note",
				Deleted:    testTime,
				DeletedBy:  "https://example.com/users/alice",
			},
			wantErr: false,
		},
		{
			name:     "tombstone not found",
			objectID: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
			},
			wantErr: true,
			errMsg:  "tombstone not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: objectsTestTableName,
			}

			tombstone, err := s.GetTombstone(ctx, tt.objectID)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantTombstone.ID, tombstone.ID)
				assert.Equal(t, tt.wantTombstone.Type, tombstone.Type)
				assert.Equal(t, tt.wantTombstone.FormerType, tombstone.FormerType)
				assert.Equal(t, tt.wantTombstone.DeletedBy, tombstone.DeletedBy)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestUpdateHistory(t *testing.T) {
	ctx := context.Background()

	t.Run("UpdateObject with history", func(t *testing.T) {
		// Original object
		originalObj := &Object{
			ID:           "https://example.com/objects/123",
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Original content",
			Published:    time.Now().Add(-1 * time.Hour),
		}

		// Updated object
		updatedObj := &Object{
			ID:           "https://example.com/objects/123",
			Type:         "Note",
			AttributedTo: "https://example.com/users/alice",
			Content:      "Updated content",
			Published:    originalObj.Published, // Keep original published
		}

		mockClient := new(mocks.MockDynamoDBClient)

		// GetObject returns original
		mockClient.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return input.Key["SK"].(*types.AttributeValueMemberS).Value == "TOMBSTONE"
		})).Return(&dynamodb.GetItemOutput{}, nil).Once()

		mockClient.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
			return input.Key["SK"].(*types.AttributeValueMemberS).Value == "METADATA"
		})).Return(&dynamodb.GetItemOutput{
			Item: map[string]types.AttributeValue{
				"Object": &types.AttributeValueMemberM{
					Value: map[string]types.AttributeValue{
						"id":           &types.AttributeValueMemberS{Value: originalObj.ID},
						"type":         &types.AttributeValueMemberS{Value: originalObj.Type},
						"attributedTo": &types.AttributeValueMemberS{Value: originalObj.AttributedTo},
						"content":      &types.AttributeValueMemberS{Value: originalObj.Content},
						"published":    &types.AttributeValueMemberS{Value: originalObj.Published.Format(time.RFC3339)},
					},
				},
			},
		}, nil).Once()

		// GetUpdateHistory returns empty (first update)
		mockClient.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
			pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
			return pk == "OBJECT#https://example.com/objects/123#HISTORY"
		})).Return(&dynamodb.QueryOutput{Items: []map[string]types.AttributeValue{}}, nil)

		// CreateUpdateHistory
		mockClient.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			pk := input.Item["PK"].(*types.AttributeValueMemberS).Value
			return pk == "OBJECT#https://example.com/objects/123#HISTORY"
		})).Return(&dynamodb.PutItemOutput{}, nil)

		// UpdateObject
		mockClient.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			pk := input.Item["PK"].(*types.AttributeValueMemberS).Value
			return pk == "OBJECT#https://example.com/objects/123"
		})).Return(&dynamodb.PutItemOutput{}, nil)

		storage := &dynamoDBStorage{
			client:    mockClient,
			tableName: objectsTestTableName,
		}

		err := storage.UpdateObject(ctx, updatedObj)
		assert.NoError(t, err)
		assert.False(t, updatedObj.Updated.IsZero())
		mockClient.AssertExpectations(t)
	})

	t.Run("CreateUpdateHistory", func(t *testing.T) {
		history := &storage.UpdateHistory{
			ObjectID:      "https://example.com/objects/123",
			Version:       1,
			UpdatedAt:     time.Now(),
			UpdatedBy:     "https://example.com/users/alice",
			PreviousState: `{"content":"old content"}`,
			Summary:       "Fixed typo",
		}

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
			pk := input.Item["PK"].(*types.AttributeValueMemberS).Value
			sk := input.Item["SK"].(*types.AttributeValueMemberS).Value
			return pk == "OBJECT#https://example.com/objects/123#HISTORY" &&
				sk == "VERSION#00001"
		})).Return(&dynamodb.PutItemOutput{}, nil)

		storage := &dynamoDBStorage{
			client:    mockClient,
			tableName: objectsTestTableName,
		}

		err := storage.CreateUpdateHistory(ctx, history)
		assert.NoError(t, err)
		mockClient.AssertExpectations(t)
	})

	t.Run("GetUpdateHistory", func(t *testing.T) {
		objectID := "https://example.com/objects/123"
		limit := 10

		mockClient := new(mocks.MockDynamoDBClient)
		mockClient.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
			pk := input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value
			return pk == "OBJECT#https://example.com/objects/123#HISTORY" &&
				*input.Limit == int32(limit) &&
				*input.ScanIndexForward == false // Most recent first
		})).Return(&dynamodb.QueryOutput{
			Items: []map[string]types.AttributeValue{
				{
					"History": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"objectId":      &types.AttributeValueMemberS{Value: objectID},
							"version":       &types.AttributeValueMemberN{Value: "2"},
							"updatedAt":     &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
							"updatedBy":     &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
							"previousState": &types.AttributeValueMemberS{Value: `{"content":"version 1"}`},
							"summary":       &types.AttributeValueMemberS{Value: "Updated content"},
						},
					},
				},
				{
					"History": &types.AttributeValueMemberM{
						Value: map[string]types.AttributeValue{
							"objectId":      &types.AttributeValueMemberS{Value: objectID},
							"version":       &types.AttributeValueMemberN{Value: "1"},
							"updatedAt":     &types.AttributeValueMemberS{Value: time.Now().Add(-1 * time.Hour).Format(time.RFC3339)},
							"updatedBy":     &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
							"previousState": &types.AttributeValueMemberS{Value: `{"content":"original"}`},
							"summary":       &types.AttributeValueMemberS{Value: "Initial edit"},
						},
					},
				},
			},
		}, nil)

		storage := &dynamoDBStorage{
			client:    mockClient,
			tableName: objectsTestTableName,
		}

		histories, err := storage.GetUpdateHistory(ctx, objectID, limit)
		assert.NoError(t, err)
		assert.Len(t, histories, 2)
		assert.Equal(t, 2, histories[0].Version)
		assert.Equal(t, 1, histories[1].Version)
		mockClient.AssertExpectations(t)
	})
}
