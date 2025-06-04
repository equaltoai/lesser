package dynamodb

import (
	"context"
	"testing"
	"time"

	"github.com/aron23/lesser/internal/testutil/mocks"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCreateLike(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		like      *storage.Like
		setupMock func(*mocks.MockDynamoDBClient)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful like creation",
			like: &storage.Like{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return input.TableName != nil && *input.TableName == testTableName &&
						input.Item["PK"] != nil &&
						input.Item["SK"] != nil &&
						input.Item["GSI3PK"] != nil &&
						input.Item["GSI3SK"] != nil
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "like with existing ID",
			like: &storage.Like{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
				ID:     "https://example.com/activities/like123",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return input.Item["Like"] != nil
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "duplicate like",
			like: &storage.Like{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{
					Message: aws.String("The conditional request failed"),
				})
			},
			wantErr: true,
			errMsg:  "already liked",
		},
		{
			name: "missing actor",
			like: &storage.Like{
				Object: "https://example.com/objects/post1",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {},
			wantErr:   true,
			errMsg:    "actor and object are required",
		},
		{
			name: "missing object",
			like: &storage.Like{
				Actor: "https://example.com/users/alice",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {},
			wantErr:   true,
			errMsg:    "actor and object are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: testTableName,
			}

			err := s.CreateLike(ctx, tt.like)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.like.ID)
				assert.False(t, tt.like.Published.IsZero())
				assert.False(t, tt.like.CreatedAt.IsZero())
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetLike(t *testing.T) {
	ctx := context.Background()
	testTime := time.Now()

	tests := []struct {
		name      string
		actor     string
		object    string
		setupMock func(*mocks.MockDynamoDBClient)
		wantLike  *storage.Like
		wantErr   bool
		errMsg    string
	}{
		{
			name:   "existing like",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					pk := input.Key["PK"].(*types.AttributeValueMemberS).Value
					sk := input.Key["SK"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#LIKES" &&
						sk == "ACTOR#https://example.com/users/alice"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1#LIKES"},
						"SK": &types.AttributeValueMemberS{Value: "ACTOR#https://example.com/users/alice"},
						"Like": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"actor":      &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
								"object":     &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
								"id":         &types.AttributeValueMemberS{Value: "https://example.com/activities/like123"},
								"published":  &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
								"created_at": &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
							},
						},
					},
				}, nil)
			},
			wantLike: &storage.Like{
				Actor:     "https://example.com/users/alice",
				Object:    "https://example.com/objects/post1",
				ID:        "https://example.com/activities/like123",
				Published: testTime,
				CreatedAt: testTime,
			},
			wantErr: false,
		},
		{
			name:   "like not found",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
			},
			wantErr: true,
			errMsg:  "like not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: testTableName,
			}

			like, err := s.GetLike(ctx, tt.actor, tt.object)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantLike.Actor, like.Actor)
				assert.Equal(t, tt.wantLike.Object, like.Object)
				assert.Equal(t, tt.wantLike.ID, like.ID)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestDeleteLike(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		actor     string
		object    string
		setupMock func(*mocks.MockDynamoDBClient)
		wantErr   bool
	}{
		{
			name:   "successful deletion",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", ctx, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
					pk := input.Key["PK"].(*types.AttributeValueMemberS).Value
					sk := input.Key["SK"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#LIKES" &&
						sk == "ACTOR#https://example.com/users/alice"
				})).Return(&dynamodb.DeleteItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:   "deletion error",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", ctx, mock.Anything).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: testTableName,
			}

			err := s.DeleteLike(ctx, tt.actor, tt.object)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetObjectLikes(t *testing.T) {
	ctx := context.Background()
	testTime := time.Now()

	tests := []struct {
		name       string
		objectID   string
		limit      int
		cursor     string
		setupMock  func(*mocks.MockDynamoDBClient)
		wantLikes  int
		wantCursor string
		wantErr    bool
	}{
		{
			name:     "get likes with default limit",
			objectID: "https://example.com/objects/post1",
			limit:    0,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return *input.Limit == 20 &&
						*input.ScanIndexForward == false
				})).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"Like": &types.AttributeValueMemberM{
								Value: map[string]types.AttributeValue{
									"actor":      &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
									"object":     &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
									"id":         &types.AttributeValueMemberS{Value: "like1"},
									"published":  &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
									"created_at": &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
								},
							},
						},
					},
				}, nil)
			},
			wantLikes: 1,
			wantErr:   false,
		},
		{
			name:     "query error",
			objectID: "https://example.com/objects/post1",
			limit:    20,
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", ctx, mock.Anything).Return(nil, assert.AnError)
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: testTableName,
			}

			likes, cursor, err := s.GetObjectLikes(ctx, tt.objectID, tt.limit, tt.cursor)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, likes, tt.wantLikes)
				assert.Equal(t, tt.wantCursor, cursor)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestCountObjectLikes(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		objectID  string
		setupMock func(*mocks.MockDynamoDBClient)
		wantCount int
		wantErr   bool
	}{
		{
			name:     "count with single page",
			objectID: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return input.Select == types.SelectCount
				})).Return(&dynamodb.QueryOutput{
					Count: 5,
				}, nil)
			},
			wantCount: 5,
			wantErr:   false,
		},
		{
			name:     "count with multiple pages",
			objectID: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// First page
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return input.Select == types.SelectCount && input.ExclusiveStartKey == nil
				})).Return(&dynamodb.QueryOutput{
					Count: 100,
					LastEvaluatedKey: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "key"},
					},
				}, nil).Once()

				// Second page
				m.On("Query", ctx, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return input.Select == types.SelectCount && input.ExclusiveStartKey != nil
				})).Return(&dynamodb.QueryOutput{
					Count: 23,
				}, nil).Once()
			},
			wantCount: 123,
			wantErr:   false,
		},
		{
			name:     "query error",
			objectID: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", ctx, mock.Anything).Return(nil, assert.AnError)
			},
			wantCount: 0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: testTableName,
			}

			count, err := s.CountObjectLikes(ctx, tt.objectID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCount, count)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
