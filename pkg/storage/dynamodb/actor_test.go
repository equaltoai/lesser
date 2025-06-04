package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockDynamoDBClient is a mock implementation of the DynamoDB client
type MockDynamoDBClient struct {
	mock.Mock
}

func (m *MockDynamoDBClient) PutItem(ctx context.Context, params *dynamodb.PutItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.PutItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) GetItem(ctx context.Context, params *dynamodb.GetItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.GetItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) UpdateItem(ctx context.Context, params *dynamodb.UpdateItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.UpdateItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) DeleteItem(ctx context.Context, params *dynamodb.DeleteItemInput, optFns ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.DeleteItemOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Query(ctx context.Context, params *dynamodb.QueryInput, optFns ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.QueryOutput), args.Error(1)
}

func (m *MockDynamoDBClient) Scan(ctx context.Context, params *dynamodb.ScanInput, optFns ...func(*dynamodb.Options)) (*dynamodb.ScanOutput, error) {
	args := m.Called(ctx, params)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*dynamodb.ScanOutput), args.Error(1)
}

// Helper function to create a test actor
func createTestActor(username string) *activitypub.Actor {
	baseURL := "https://example.com"
	actorID := baseURL + "/users/" + username

	return &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   actorID,
			Type: activitypub.PersonType,
		},
		PreferredUsername: username,
		Name:              "Test User",
		Summary:           "A test user",
		Inbox:             actorID + "/inbox",
		Outbox:            actorID + "/outbox",
		PublicKey: &activitypub.PublicKey{
			ID:           actorID + "#main-key",
			Owner:        actorID,
			PublicKeyPem: "-----BEGIN PUBLIC KEY-----\ntest-public-key\n-----END PUBLIC KEY-----",
		},
	}
}

func TestCreateActor(t *testing.T) {
	tests := []struct {
		name          string
		actor         *activitypub.Actor
		privateKey    string
		setupMock     func(*MockDynamoDBClient)
		expectedError error
		errorContains string
	}{
		{
			name:       "successful actor creation",
			actor:      createTestActor("alice"),
			privateKey: "test-private-key",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.PutItemInput) bool {
					// Verify table name
					if *input.TableName != "test-table" {
						return false
					}
					// Verify condition expression
					if input.ConditionExpression == nil || *input.ConditionExpression != "attribute_not_exists(PK)" {
						return false
					}
					// Verify PK and SK
					pk, pkOk := input.Item["PK"].(*types.AttributeValueMemberS)
					sk, skOk := input.Item["SK"].(*types.AttributeValueMemberS)
					return pkOk && skOk && pk.Value == "ACTOR#alice" && sk.Value == "PROFILE"
				})).Return(&dynamodb.PutItemOutput{}, nil)
			},
			expectedError: nil,
		},
		{
			name:       "actor already exists",
			actor:      createTestActor("alice"),
			privateKey: "test-private-key",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.Anything).Return(
					nil,
					&types.ConditionalCheckFailedException{Message: aws.String("The conditional request failed")},
				)
			},
			expectedError: common.ConflictError{Resource: "actor", Message: "actor alice already exists"},
		},
		{
			name:          "missing username",
			actor:         &activitypub.Actor{},
			privateKey:    "test-private-key",
			setupMock:     func(m *MockDynamoDBClient) {},
			expectedError: common.ValidationError{Field: "PreferredUsername", Message: "username is required"},
		},
		{
			name:       "dynamodb error",
			actor:      createTestActor("alice"),
			privateKey: "test-private-key",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("PutItem", mock.Anything, mock.Anything).Return(
					nil,
					errors.New("dynamodb error"),
				)
			},
			errorContains: "failed to create actor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := new(MockDynamoDBClient)
			tt.setupMock(mockClient)

			// Create storage with mock
			storage := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			// Execute
			err := storage.CreateActor(context.Background(), tt.actor, tt.privateKey)

			// Assert
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
			} else if tt.errorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}

			// Verify mock expectations
			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetActor(t *testing.T) {
	tests := []struct {
		name          string
		username      string
		setupMock     func(*MockDynamoDBClient)
		expectedActor *activitypub.Actor
		expectedError error
		errorContains string
	}{
		{
			name:     "successful get actor",
			username: "alice",
			setupMock: func(m *MockDynamoDBClient) {
				actor := createTestActor("alice")
				m.On("GetItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					pk, pkOk := input.Key["PK"].(*types.AttributeValueMemberS)
					sk, skOk := input.Key["SK"].(*types.AttributeValueMemberS)
					return pkOk && skOk && pk.Value == "ACTOR#alice" && sk.Value == "PROFILE"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK":        &types.AttributeValueMemberS{Value: "ACTOR#alice"},
						"SK":        &types.AttributeValueMemberS{Value: "PROFILE"},
						"Actor":     &types.AttributeValueMemberM{Value: marshalActorToAttributeValue(actor)},
						"CreatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
						"UpdatedAt": &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
					},
				}, nil)
			},
			expectedActor: createTestActor("alice"),
			expectedError: nil,
		},
		{
			name:     "actor not found",
			username: "nonexistent",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
					Item: nil,
				}, nil)
			},
			expectedActor: nil,
			expectedError: common.ActorNotFoundError{Username: "nonexistent"},
		},
		{
			name:     "dynamodb error",
			username: "alice",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(
					nil,
					errors.New("dynamodb error"),
				)
			},
			expectedActor: nil,
			errorContains: "failed to get actor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := new(MockDynamoDBClient)
			tt.setupMock(mockClient)

			// Create storage with mock
			storage := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			// Execute
			actor, err := storage.GetActor(context.Background(), tt.username)

			// Assert
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
				assert.Nil(t, actor)
			} else if tt.errorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, actor)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedActor.ID, actor.ID)
				assert.Equal(t, tt.expectedActor.PreferredUsername, actor.PreferredUsername)
			}

			// Verify mock expectations
			mockClient.AssertExpectations(t)
		})
	}
}

func TestGetActorPrivateKey(t *testing.T) {
	tests := []struct {
		name               string
		username           string
		setupMock          func(*MockDynamoDBClient)
		expectedPrivateKey string
		expectedError      error
		errorContains      string
	}{
		{
			name:     "successful get private key",
			username: "alice",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.MatchedBy(func(input *dynamodb.GetItemInput) bool {
					// Verify projection expression
					return input.ProjectionExpression != nil && *input.ProjectionExpression == "PrivateKey"
				})).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PrivateKey": &types.AttributeValueMemberS{Value: "test-private-key"},
					},
				}, nil)
			},
			expectedPrivateKey: "test-private-key",
			expectedError:      nil,
		},
		{
			name:     "actor not found",
			username: "nonexistent",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
					Item: nil,
				}, nil)
			},
			expectedPrivateKey: "",
			expectedError:      common.ActorNotFoundError{Username: "nonexistent"},
		},
		{
			name:     "private key not found",
			username: "alice",
			setupMock: func(m *MockDynamoDBClient) {
				m.On("GetItem", mock.Anything, mock.Anything).Return(&dynamodb.GetItemOutput{
					Item: map[string]types.AttributeValue{
						"PK": &types.AttributeValueMemberS{Value: "ACTOR#alice"},
					},
				}, nil)
			},
			expectedPrivateKey: "",
			errorContains:      "private key not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock client
			mockClient := new(MockDynamoDBClient)
			tt.setupMock(mockClient)

			// Create storage with mock
			storage := &dynamoDBStorage{
				client:    mockClient,
				tableName: "test-table",
			}

			// Execute
			privateKey, err := storage.GetActorPrivateKey(context.Background(), tt.username)

			// Assert
			if tt.expectedError != nil {
				assert.Equal(t, tt.expectedError, err)
			} else if tt.errorContains != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedPrivateKey, privateKey)

			// Verify mock expectations
			mockClient.AssertExpectations(t)
		})
	}
}

// Helper function to marshal actor to attribute values (simplified)
func marshalActorToAttributeValue(actor *activitypub.Actor) map[string]types.AttributeValue {
	attrs := map[string]types.AttributeValue{
		"ID":                &types.AttributeValueMemberS{Value: actor.ID},
		"Type":              &types.AttributeValueMemberS{Value: actor.Type},
		"PreferredUsername": &types.AttributeValueMemberS{Value: actor.PreferredUsername},
		"Inbox":             &types.AttributeValueMemberS{Value: actor.Inbox},
		"Outbox":            &types.AttributeValueMemberS{Value: actor.Outbox},
	}

	if actor.Name != "" {
		attrs["Name"] = &types.AttributeValueMemberS{Value: actor.Name}
	}
	if actor.Summary != "" {
		attrs["Summary"] = &types.AttributeValueMemberS{Value: actor.Summary}
	}

	return attrs
}
