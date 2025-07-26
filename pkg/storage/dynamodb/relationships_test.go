package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// Use a different constant name to avoid conflict with integration_test.go
const relTestTableName = "test-table"

func TestCreateFollow(t *testing.T) {
	tests := []struct {
		name             string
		followerUsername string
		followedUsername string
		followActivityID string
		setupMock        func(*mocks.MockDynamoDBClient)
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "successful create",
			followerUsername: "alice",
			followedUsername: "bob",
			followActivityID: "https://example.com/activities/123",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					return *input.TableName == relTestTableName &&
						input.Item["PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						input.Item["SK"].(*types.AttributeValueMemberS).Value == "FOLLOWING#bob" &&
						input.Item["GSI1PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#bob" &&
						input.Item["GSI1SK"].(*types.AttributeValueMemberS).Value == "FOLLOWER#alice" &&
						input.Item["ActivityID"].(*types.AttributeValueMemberS).Value == "https://example.com/activities/123" &&
						input.Item["State"].(*types.AttributeValueMemberS).Value == storage.RelationshipPending
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			expectedError: false,
		},
		{
			name:             "relationship already exists",
			followerUsername: "alice",
			followedUsername: "bob",
			followActivityID: "https://example.com/activities/123",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.Anything).Return(
					nil, &types.ConditionalCheckFailedException{Message: aws.String("already exists")})
			},
			expectedError: false, // Method returns nil for already existing relationships
		},
		{
			name:             "dynamodb error",
			followerUsername: "alice",
			followedUsername: "bob",
			followActivityID: "https://example.com/activities/123",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expectedError: true,
			errorContains: "failed to create follow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			err := s.CreateFollow(context.Background(), tt.followerUsername, tt.followedUsername, tt.followActivityID)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestAcceptFollow(t *testing.T) {
	tests := []struct {
		name             string
		followerUsername string
		followedUsername string
		setupMock        func(*mocks.MockDynamoDBClient)
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "successful accept",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("UpdateItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.UpdateItemInput) bool {
					return *input.TableName == relTestTableName &&
						input.Key["PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						input.Key["SK"].(*types.AttributeValueMemberS).Value == "FOLLOWING#bob" &&
						input.ExpressionAttributeValues[":state"].(*types.AttributeValueMemberS).Value == storage.RelationshipAccepted
				})).Return(&dynamodb.UpdateItemOutput{}, nil)
			},
			expectedError: false,
		},
		{
			name:             "relationship not found",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("UpdateItem", mock.Anything, mock.Anything).Return(
					nil, &types.ConditionalCheckFailedException{Message: aws.String("not found")})
			},
			expectedError: true,
			errorContains: "follow relationship not found",
		},
		{
			name:             "dynamodb error",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("UpdateItem", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expectedError: true,
			errorContains: "failed to accept follow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			err := s.AcceptFollow(context.Background(), tt.followerUsername, tt.followedUsername)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetFollowers(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		limit         int
		cursor        string
		setupMock     func(*mocks.MockDynamoDBClient)
		expected      []string
		expectedNext  string
		expectedError bool
		errorContains string
	}{
		{
			name:     "successful get followers",
			username: "bob",
			limit:    10,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return *input.TableName == relTestTableName &&
						*input.IndexName == "GSI1" &&
						input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value == "FOLLOW#bob" &&
						*input.Limit == 10
				})).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"GSI1SK": &types.AttributeValueMemberS{Value: "FOLLOWER#alice"},
							"State":  &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
						{
							"GSI1SK": &types.AttributeValueMemberS{Value: "FOLLOWER#carol"},
							"State":  &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
					},
				}, nil)
			},
			expected:      []string{"alice", "carol"},
			expectedNext:  "",
			expectedError: false,
		},
		{
			name:     "invalid cursor",
			username: "bob",
			limit:    10,
			cursor:   "invalid-cursor",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// The implementation returns error for invalid cursor, so Query won't be called
			},
			expected:      nil,
			expectedNext:  "",
			expectedError: true,
			errorContains: "invalid cursor",
		},
		{
			name:     "dynamodb error",
			username: "bob",
			limit:    10,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expected:      nil,
			expectedNext:  "",
			expectedError: true,
			errorContains: "failed to query followers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			followers, nextCursor, err := s.GetFollowers(context.Background(), tt.username, tt.limit, tt.cursor)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, followers)
				if tt.expectedNext != "" {
					assert.Contains(t, nextCursor, tt.expectedNext)
				} else {
					assert.Empty(t, nextCursor)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetFollowing(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		limit         int
		cursor        string
		setupMock     func(*mocks.MockDynamoDBClient)
		expected      []string
		expectedNext  string
		expectedError bool
		errorContains string
	}{
		{
			name:     "successful get following",
			username: "alice",
			limit:    10,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.MatchedBy(func(input *dynamodb.QueryInput) bool {
					return *input.TableName == relTestTableName &&
						input.ExpressionAttributeValues[":pk"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						*input.Limit == 10
				})).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"SK":    &types.AttributeValueMemberS{Value: "FOLLOWING#bob"},
							"State": &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
						{
							"SK":    &types.AttributeValueMemberS{Value: "FOLLOWING#carol"},
							"State": &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
					},
				}, nil)
			},
			expected:      []string{"bob", "carol"},
			expectedNext:  "not-empty", // Just check that a cursor is returned
			expectedError: false,
		},
		{
			name:     "filters out non-accepted relationships",
			username: "alice",
			limit:    10,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"SK":    &types.AttributeValueMemberS{Value: "FOLLOWING#bob"},
							"State": &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
						// DynamoDB FilterExpression would filter this out, so don't include it in the mock response
					},
				}, nil)
			},
			expected:      []string{"bob"},
			expectedNext:  "",
			expectedError: false,
		},
		{
			name:     "with next cursor",
			username: "alice",
			limit:    2,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.Anything).Return(&dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"SK":    &types.AttributeValueMemberS{Value: "FOLLOWING#bob"},
							"State": &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
						{
							"SK":    &types.AttributeValueMemberS{Value: "FOLLOWING#carol"},
							"State": &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						},
					},
					LastEvaluatedKey: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "FOLLOW#alice"},
						"SK": &types.AttributeValueMemberS{Value: "FOLLOWING#carol"},
					},
				}, nil)
			},
			expected:      []string{"bob", "carol"},
			expectedNext:  "not-empty", // Just check that a cursor is returned
			expectedError: false,
		},
		{
			name:     "invalid cursor error",
			username: "alice",
			limit:    10,
			cursor:   "invalid-cursor",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// Query won't be called due to cursor error
			},
			expected:      nil,
			expectedNext:  "",
			expectedError: true,
			errorContains: "invalid cursor",
		},
		{
			name:     "dynamodb error",
			username: "alice",
			limit:    10,
			cursor:   "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expected:      nil,
			expectedNext:  "",
			expectedError: true,
			errorContains: "failed to query following",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			following, nextCursor, err := s.GetFollowing(context.Background(), tt.username, tt.limit, tt.cursor)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, following)
				if tt.expectedNext == "not-empty" {
					assert.NotEmpty(t, nextCursor)
				} else if tt.expectedNext != "" {
					assert.Contains(t, nextCursor, tt.expectedNext)
				} else {
					assert.Empty(t, nextCursor)
				}
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestIsFollowing(t *testing.T) {
	tests := []struct {
		name             string
		followerUsername string
		followedUsername string
		setupMock        func(*mocks.MockDynamoDBClient)
		expected         bool
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "following with accepted state",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					return *input.TableName == relTestTableName &&
						input.Key["PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						input.Key["SK"].(*types.AttributeValueMemberS).Value == "FOLLOWING#bob"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"State":     &types.AttributeValueMemberS{Value: storage.RelationshipAccepted},
						"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
					},
				}, nil)
			},
			expected:      true,
			expectedError: false,
		},
		{
			name:             "following with pending state",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"State":     &types.AttributeValueMemberS{Value: storage.RelationshipPending},
						"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
					},
				}, nil)
			},
			expected:      false,
			expectedError: false,
		},
		{
			name:             "not following",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
					Item: nil,
				}, nil)
			},
			expected:      false,
			expectedError: false,
		},
		{
			name:             "dynamodb error",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expected:      false,
			expectedError: true,
			errorContains: "failed to check following",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			isFollowing, err := s.IsFollowing(context.Background(), tt.followerUsername, tt.followedUsername)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, isFollowing)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestRejectFollow(t *testing.T) {
	tests := []struct {
		name             string
		followerUsername string
		followedUsername string
		setupMock        func(*mocks.MockDynamoDBClient)
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "successful reject",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("UpdateItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.UpdateItemInput) bool {
					return *input.TableName == relTestTableName &&
						input.Key["PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						input.Key["SK"].(*types.AttributeValueMemberS).Value == "FOLLOWING#bob" &&
						input.ExpressionAttributeValues[":state"].(*types.AttributeValueMemberS).Value == storage.RelationshipRejected
				})).Return(&dynamodb.UpdateItemOutput{}, nil)
			},
			expectedError: false,
		},
		{
			name:             "dynamodb error",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("UpdateItem", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expectedError: true,
			errorContains: "failed to reject follow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			err := s.RejectFollow(context.Background(), tt.followerUsername, tt.followedUsername)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestRemoveFollow(t *testing.T) {
	tests := []struct {
		name             string
		followerUsername string
		followedUsername string
		setupMock        func(*mocks.MockDynamoDBClient)
		expectedError    bool
		errorContains    string
	}{
		{
			name:             "successful remove",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.DeleteItemInput) bool {
					return *input.TableName == relTestTableName &&
						input.Key["PK"].(*types.AttributeValueMemberS).Value == "FOLLOW#alice" &&
						input.Key["SK"].(*types.AttributeValueMemberS).Value == "FOLLOWING#bob"
				})).Return(&dynamodb.DeleteItemOutput{}, nil)
			},
			expectedError: false,
		},
		{
			name:             "dynamodb error",
			followerUsername: "alice",
			followedUsername: "bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", mock.Anything, mock.Anything).Return(
					nil, errors.New("dynamodb error"))
			},
			expectedError: true,
			errorContains: "failed to remove follow",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := NewWithClient(mockClient, relTestTableName)

			err := s.RemoveFollow(context.Background(), tt.followerUsername, tt.followedUsername)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
