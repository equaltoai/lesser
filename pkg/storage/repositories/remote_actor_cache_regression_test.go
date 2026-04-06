package repositories

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"github.com/theory-cloud/tabletheory/pkg/mocks"
	"go.uber.org/zap"
)

func TestActorRepository_GetCachedRemoteActor_NormalizesActorURLAndFallsBackToLegacyHandle(t *testing.T) {
	ctx := context.Background()
	mockDB := new(mocks.MockDB)
	mockQuery := new(mocks.MockQuery)

	mockDB.On("WithContext", mock.Anything).Return(mockDB)
	mockDB.On("Model", mock.AnythingOfType("*models.RemoteActor")).Return(mockQuery)

	mockQuery.On("Where", "PK", "=", "REMOTE_ACTOR#alice@remote.example").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Return(dynamormerrors.ErrItemNotFound).Once()

	mockQuery.On("Where", "PK", "=", "REMOTE_ACTOR#@alice@remote.example").Return(mockQuery).Once()
	mockQuery.On("Where", "SK", "=", "PROFILE").Return(mockQuery).Once()
	mockQuery.On("First", mock.Anything).Run(func(args mock.Arguments) {
		dest := args.Get(0).(*models.RemoteActor)
		dest.Handle = "@alice@remote.example"
		dest.Actor = &activitypub.Actor{
			BaseObject: activitypub.BaseObject{
				ID:   "https://remote.example/users/alice",
				Type: activitypub.PersonType,
			},
			PreferredUsername: "alice",
			Inbox:             "https://remote.example/users/alice/inbox",
		}
		dest.ExpiresAt = time.Now().Add(time.Minute)
	}).Return(nil).Once()

	repo := NewActorRepository(mockDB, "test-table", zap.NewNop())

	actor, err := repo.GetCachedRemoteActor(ctx, "https://remote.example/users/Alice")
	require.NoError(t, err)
	require.NotNil(t, actor)
	assert.Equal(t, "https://remote.example/users/alice", actor.ID)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func TestLoadCachedRemoteActorFromDynamo_DecodesEmbeddedBaseObject(t *testing.T) {
	previousClient := getRepositoryDynamoClient
	t.Cleanup(func() {
		getRepositoryDynamoClient = previousClient
	})

	rawActor, err := attributevalue.MarshalMap(map[string]any{
		"BaseObject": map[string]any{
			"ID":   "https://remote.example/users/alice",
			"Type": activitypub.PersonType,
		},
		"PreferredUsername": "alice",
		"Inbox":             "https://remote.example/users/alice/inbox",
	})
	require.NoError(t, err)

	getRepositoryDynamoClient = func(context.Context) (dynamoGetItemClient, error) {
		return fakeDynamoGetItemClient{
			output: &dynamodb.GetItemOutput{
				Item: map[string]types.AttributeValue{
					"actor": &types.AttributeValueMemberM{Value: rawActor},
				},
			},
		}, nil
	}

	actor, err := loadCachedRemoteActorFromDynamo(context.Background(), "test-table", "alice@remote.example")
	require.NoError(t, err)
	require.NotNil(t, actor)
	assert.Equal(t, "https://remote.example/users/alice", actor.ID)
	assert.Equal(t, activitypub.PersonType, actor.Type)
	assert.Equal(t, "alice", actor.PreferredUsername)
	assert.Equal(t, "https://remote.example/users/alice/inbox", actor.Inbox)
}
