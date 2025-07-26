package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/internal/testutil/mocks"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// mockDynamoDBClient is defined in relationships_test.go and reused here
// If running tests separately, you may need to define it in a common test file

func TestCreateBlock(t *testing.T) {
	tests := []struct {
		name       string
		block      *storage.Block
		setupMock  func(*mocks.MockDynamoDBClient)
		wantErr    bool
		errMessage string
	}{
		{
			name: "successful block creation",
			block: &storage.Block{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/users/bob",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.AnythingOfType("*dynamodb.PutItemInput")).
					Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "block with existing ID",
			block: &storage.Block{
				Actor:     "https://example.com/users/alice",
				Object:    "https://example.com/users/bob",
				ID:        "https://example.com/activities/block-123",
				Published: time.Now(),
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.AnythingOfType("*dynamodb.PutItemInput")).
					Return(&dynamodb.PutItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name: "duplicate block",
			block: &storage.Block{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/users/bob",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.AnythingOfType("*dynamodb.PutItemInput")).
					Return(nil, &types.ConditionalCheckFailedException{})
			},
			wantErr:    true,
			errMessage: "already exists",
		},
		{
			name: "dynamodb error",
			block: &storage.Block{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/users/bob",
			},
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.AnythingOfType("*dynamodb.PutItemInput")).
					Return(nil, errors.New("dynamodb error"))
			},
			wantErr:    true,
			errMessage: "failed to create block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			err := s.CreateBlock(context.Background(), tt.block)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMessage)
			} else {
				assert.NoError(t, err)
				assert.NotEmpty(t, tt.block.ID)
				assert.False(t, tt.block.Published.IsZero())
				assert.False(t, tt.block.CreatedAt.IsZero())
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetBlock(t *testing.T) {
	tests := []struct {
		name         string
		actor        string
		blockedActor string
		setupMock    func(*mocks.MockDynamoDBClient)
		want         *storage.Block
		wantErr      bool
		errType      error
	}{
		{
			name:         "existing block",
			actor:        "https://example.com/users/alice",
			blockedActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				output := &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"Actor":     &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
						"Object":    &types.AttributeValueMemberS{Value: "https://example.com/users/bob"},
						"ID":        &types.AttributeValueMemberS{Value: "block-123"},
						"Published": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
					},
				}
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(output, nil)
			},
			want: &storage.Block{
				Actor:  "https://example.com/users/alice",
				Object: "https://example.com/users/bob",
				ID:     "block-123",
			},
			wantErr: false,
		},
		{
			name:         "block not found",
			actor:        "https://example.com/users/alice",
			blockedActor: "https://example.com/users/charlie",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(&dynamodb.GetItemOutput{}, nil)
			},
			wantErr: true,
		},
		{
			name:         "dynamodb error",
			actor:        "https://example.com/users/alice",
			blockedActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(nil, errors.New("dynamodb error"))
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
				tableName: "test-table",
			}

			result, err := s.GetBlock(context.Background(), tt.actor, tt.blockedActor)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errType != nil {
					assert.Contains(t, err.Error(), "not found")
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.want.Actor, result.Actor)
				assert.Equal(t, tt.want.Object, result.Object)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestDeleteBlock(t *testing.T) {
	tests := []struct {
		name         string
		actor        string
		blockedActor string
		setupMock    func(*mocks.MockDynamoDBClient)
		wantErr      bool
	}{
		{
			name:         "successful deletion",
			actor:        "https://example.com/users/alice",
			blockedActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", mock.Anything, mock.AnythingOfType("*dynamodb.DeleteItemInput")).
					Return(&dynamodb.DeleteItemOutput{}, nil)
			},
			wantErr: false,
		},
		{
			name:         "dynamodb error",
			actor:        "https://example.com/users/alice",
			blockedActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("DeleteItem", mock.Anything, mock.AnythingOfType("*dynamodb.DeleteItemInput")).
					Return(nil, errors.New("dynamodb error"))
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
				tableName: "test-table",
			}

			err := s.DeleteBlock(context.Background(), tt.actor, tt.blockedActor)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetBlockedActors(t *testing.T) {
	tests := []struct {
		name      string
		actor     string
		limit     int
		cursor    string
		setupMock func(*mocks.MockDynamoDBClient)
		wantCount int
		wantErr   bool
	}{
		{
			name:   "get blocked actors",
			actor:  "https://example.com/users/alice",
			limit:  10,
			cursor: "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				output := &dynamodb.QueryOutput{
					Items: []map[string]types.AttributeValue{
						{
							"Actor":     &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
							"Object":    &types.AttributeValueMemberS{Value: "https://example.com/users/bob"},
							"ID":        &types.AttributeValueMemberS{Value: "block-1"},
							"Published": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
							"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						},
						{
							"Actor":     &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
							"Object":    &types.AttributeValueMemberS{Value: "https://example.com/users/charlie"},
							"ID":        &types.AttributeValueMemberS{Value: "block-2"},
							"Published": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
							"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						},
					},
				}
				m.On("Query", mock.Anything, mock.AnythingOfType("*dynamodb.QueryInput")).
					Return(output, nil)
			},
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:   "empty result",
			actor:  "https://example.com/users/alice",
			limit:  10,
			cursor: "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.AnythingOfType("*dynamodb.QueryInput")).
					Return(&dynamodb.QueryOutput{}, nil)
			},
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:   "dynamodb error",
			actor:  "https://example.com/users/alice",
			limit:  10,
			cursor: "",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("Query", mock.Anything, mock.AnythingOfType("*dynamodb.QueryInput")).
					Return(nil, errors.New("dynamodb error"))
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
				tableName: "test-table",
			}

			blocks, _, err := s.GetBlockedActors(context.Background(), tt.actor, tt.limit, tt.cursor)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, blocks, tt.wantCount)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestIsBlocked(t *testing.T) {
	tests := []struct {
		name        string
		actor       string
		targetActor string
		setupMock   func(*mocks.MockDynamoDBClient)
		want        bool
		wantErr     bool
	}{
		{
			name:        "actor is blocked",
			actor:       "https://example.com/users/alice",
			targetActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				output := &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"Actor": &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
					},
				}
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(output, nil)
			},
			want:    true,
			wantErr: false,
		},
		{
			name:        "actor is not blocked",
			actor:       "https://example.com/users/alice",
			targetActor: "https://example.com/users/charlie",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(&dynamodb.GetItemOutput{}, nil)
			},
			want:    false,
			wantErr: false,
		},
		{
			name:        "dynamodb error",
			actor:       "https://example.com/users/alice",
			targetActor: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(nil, errors.New("dynamodb error"))
			},
			want:    false,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			result, err := s.IsBlocked(context.Background(), tt.actor, tt.targetActor)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}

func TestIsBlockedBidirectional(t *testing.T) {
	tests := []struct {
		name      string
		actor1    string
		actor2    string
		setupMock func(*mocks.MockDynamoDBClient)
		want      bool
		wantErr   bool
	}{
		{
			name:   "actor1 blocked actor2",
			actor1: "https://example.com/users/alice",
			actor2: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// First check: alice blocked bob
				output := &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"Actor": &types.AttributeValueMemberS{Value: "https://example.com/users/alice"},
					},
				}
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(output, nil).Once()
			},
			want:    true,
			wantErr: false,
		},
		{
			name:   "actor2 blocked actor1",
			actor1: "https://example.com/users/alice",
			actor2: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// First check: alice didn't block bob
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(&dynamodb.GetItemOutput{}, nil).Once()
				// Second check: bob blocked alice
				output := &dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"Actor": &types.AttributeValueMemberS{Value: "https://example.com/users/bob"},
					},
				}
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(output, nil).Once()
			},
			want:    true,
			wantErr: false,
		},
		{
			name:   "neither blocked",
			actor1: "https://example.com/users/alice",
			actor2: "https://example.com/users/bob",
			setupMock: func(m *mocks.MockDynamoDBClient) {
				// Both checks return empty
				m.On("GetItem", mock.Anything, mock.AnythingOfType("*dynamodb.GetItemInput")).
					Return(&dynamodb.GetItemOutput{}, nil).Twice()
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := new(mocks.MockDynamoDBClient)
			tt.setupMock(mockClient)

			s := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			result, err := s.IsBlockedBidirectional(context.Background(), tt.actor1, tt.actor2)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}

			mockClient.AssertExpectations(t)
		})
	}
}
