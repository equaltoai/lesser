package dynamodb

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const announcesTestTableName = "test-table"

func TestCreateAnnounce(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		announce  *storage.Announce
		setupMock func(*mocks.MockDynamoDBClient)
		wantErr   bool
		errMsg    string
	}{
		{
			name: "successful announce creation",
			announce: &storage.Announce{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
				To:     []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:     []string{"https://example.com/users/alice/followers"},
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return input.TableName != nil && *input.TableName == announcesTestTableName &&
						input.Item["PK"] != nil &&
						input.Item["SK"] != nil &&
						input.Item["GSI4PK"] != nil &&
						input.Item["GSI4SK"] != nil
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "announce with existing ID",
			announce: &storage.Announce{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
				ID:     "https://example.com/activities/announce123",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return input.Item["Announce"] != nil
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "duplicate announce",
			announce: &storage.Announce{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/objects/post1",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", ctx, mock.Anything).Return(nil, &types.ConditionalCheckFailedException{
					Message: aws.String("The conditional request failed"),
				})
			},
			wantErr: true,
			errMsg:  "already announced",
		},
		{
			name: "missing actor",
			announce: &storage.Announce{
				Object: "https://example.com/objects/post1",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {},
			wantErr:   true,
			errMsg:    "actor and object are required",
		},
		{
			name: "missing object",
			announce: &storage.Announce{
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
				tableName: announcesTestTableName,
			}

			err := s.CreateAnnounce(ctx, tt.announce)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.announce.ID)
				assert.False(t, tt.announce.Published.IsZero())
				assert.False(t, tt.announce.CreatedAt.IsZero())
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetAnnounce(t *testing.T) {
	ctx := context.Background()
	testTime := time.Now()

	tests := []struct {
		name         string
		actor        string
		object       string
		setupMock    func(*mocks.MockDynamoDBClient)
		wantAnnounce *storage.Announce
		wantErr      bool
		errMsg       string
	}{
		{
			name:   "existing announce",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					pk := input.Key["PK"].(*types.AttributeValueMemberS).Value
					sk := input.Key["SK"].(*types.AttributeValueMemberS).Value
					return pk == "OBJECT#https://example.com/objects/post1#ANNOUNCES" &&
						sk == "ACTOR#https://example.com/users/alice"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "OBJECT#https://example.com/objects/post1#ANNOUNCES"},
						"SK": &types.AttributeValueMemberS{Value: "ACTOR#https://example.com/users/alice"},
						"Announce": &types.AttributeValueMemberM{
							Value: map[string]types.AttributeValue{
								"actor":      &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
								"object":     &types.AttributeValueMemberS{Value: "https://example.com/objects/post1"},
								"id":         &types.AttributeValueMemberS{Value: "https://example.com/activities/announce123"},
								"published":  &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
								"created_at": &types.AttributeValueMemberS{Value: testTime.Format(time.RFC3339)},
								"to": &types.AttributeValueMemberL{Value: []types.AttributeValue{
									&types.AttributeValueMemberS{Value: "https://www.w3.org/ns/activitystreams#Public"},
								}},
								"cc": &types.AttributeValueMemberL{Value: []types.AttributeValue{
									&types.AttributeValueMemberS{Value: "https://example.com/users/alice/followers"},
								}},
							},
						},
					},
				}, nil)
			},
			wantAnnounce: &storage.Announce{
				Actor:     "https://example.com/users/alice",
				Object:    "https://example.com/objects/post1",
				ID:        "https://example.com/activities/announce123",
				Published: testTime,
				CreatedAt: testTime,
				To:        []string{"https://www.w3.org/ns/activitystreams#Public"},
				CC:        []string{"https://example.com/users/alice/followers"},
			},
			wantErr: false,
		},
		{
			name:   "announce not found",
			actor:  "https://example.com/users/alice",
			object: "https://example.com/objects/post1",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", ctx, mock.Anything).Return(&dynamodb.GetItemOutput{}, nil)
			},
			wantErr: true,
			errMsg:  "announce not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: announcesTestTableName,
			}

			announce, err := s.GetAnnounce(ctx, tt.actor, tt.object)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantAnnounce.Actor, announce.Actor)
				assert.Equal(t, tt.wantAnnounce.Object, announce.Object)
				assert.Equal(t, tt.wantAnnounce.ID, announce.ID)
				assert.Equal(t, tt.wantAnnounce.To, announce.To)
				assert.Equal(t, tt.wantAnnounce.CC, announce.CC)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestDeleteAnnounce(t *testing.T) {
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
					return pk == "OBJECT#https://example.com/objects/post1#ANNOUNCES" &&
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
				tableName: announcesTestTableName,
			}

			err := s.DeleteAnnounce(ctx, tt.actor, tt.object)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestCountObjectAnnounces(t *testing.T) {
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
					Count: 47,
				}, nil).Once()
			},
			wantCount: 147,
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
				tableName: announcesTestTableName,
			}

			count, err := s.CountObjectAnnounces(ctx, tt.objectID)

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
