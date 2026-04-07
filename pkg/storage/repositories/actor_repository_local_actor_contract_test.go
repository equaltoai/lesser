package repositories

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/equaltoai/lesser/pkg/activitypub"
	lesserconfig "github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestActorRepository_Round34_LocalActorReadsCanonicalizeNestedBaseObjectRows(t *testing.T) {
	getters := map[string]func(*ActorRepository, context.Context, string) (*activitypub.Actor, error){
		"GetActor": func(repo *ActorRepository, ctx context.Context, username string) (*activitypub.Actor, error) {
			return repo.GetActor(ctx, username)
		},
		"GetActorByUsername": func(repo *ActorRepository, ctx context.Context, username string) (*activitypub.Actor, error) {
			return repo.GetActorByUsername(ctx, username)
		},
	}

	for name, getter := range getters {
		t.Run(name, func(t *testing.T) {
			restore := stubCanonicalLocalActorReadFallback(t, "alice")

			ctx := context.Background()
			mockDB, mockQuery := setupPermissiveDBAndQuery()
			repo := NewActorRepository(mockDB, "test-table", zap.NewNop())

			createdAt := time.Date(2026, 4, 7, 9, 30, 0, 0, time.UTC)
			updatedAt := createdAt.Add(5 * time.Minute)
			mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
				row := args.Get(0).(*models.Actor)
				row.Username = "alice"
				row.Actor = &activitypub.Actor{
					PreferredUsername: "alice",
					Inbox:             "https://example.com/users/alice/inbox",
					Outbox:            "https://example.com/users/alice/outbox",
				}
				row.CreatedAt = createdAt
				row.UpdatedAt = updatedAt
			}).Once()

			actor, err := getter(repo, ctx, "alice")
			restore()

			require.NoError(t, err)
			require.NotNil(t, actor)
			require.Equal(t, "https://example.com/users/alice", actor.ID)
			require.Equal(t, activitypub.PersonType, actor.Type)
			require.Equal(t, "alice", actor.PreferredUsername)
			require.Equal(t, "https://example.com/@alice", actor.URL)
			require.Equal(t, "https://example.com/users/alice/inbox", actor.Inbox)
			require.Equal(t, "https://example.com/users/alice/outbox", actor.Outbox)
			require.Equal(t, "https://example.com/users/alice/followers", actor.Followers)
			require.Equal(t, "https://example.com/users/alice/following", actor.Following)
			require.Equal(t, "https://example.com/users/alice/liked", actor.Liked)
			require.NotNil(t, actor.Endpoints)
			require.Equal(t, "https://example.com/inbox", actor.Endpoints.SharedInbox)
			require.NotNil(t, actor.CreatedAt)
			require.WithinDuration(t, createdAt, *actor.CreatedAt, time.Second)
			require.NotNil(t, actor.Published)
			require.WithinDuration(t, createdAt, *actor.Published, time.Second)
			require.NotNil(t, actor.Updated)
			require.WithinDuration(t, updatedAt, *actor.Updated, time.Second)

			mockDB.AssertExpectations(t)
			mockQuery.AssertExpectations(t)
		})
	}
}

func TestActorRepository_Round34_GetActorWithMetadataUsesCanonicalLocalActorContract(t *testing.T) {
	restore := stubCanonicalLocalActorReadFallback(t, "agent-0")
	defer restore()

	ctx := context.Background()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewActorRepository(mockDB, "test-table", zap.NewNop())

	createdAt := time.Date(2026, 4, 7, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(10 * time.Minute)
	mockQuery.On("First", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		row := args.Get(0).(*models.Actor)
		row.Username = "Agent-0"
		row.Actor = &activitypub.Actor{
			PreferredUsername: "Agent-0",
		}
		row.CreatedAt = createdAt
		row.UpdatedAt = updatedAt
		row.Fields = []models.ActorField{{Name: "Role", Value: "Steward"}}
	}).Once()

	actor, metadata, err := repo.GetActorWithMetadata(ctx, "Agent-0")
	require.NoError(t, err)
	require.NotNil(t, actor)
	require.NotNil(t, metadata)
	require.Equal(t, "agent-0", actor.PreferredUsername)
	require.Equal(t, "https://example.com/users/agent-0", actor.ID)
	require.Equal(t, activitypub.PersonType, actor.Type)
	require.Equal(t, "https://example.com/inbox", actor.Endpoints.SharedInbox)
	require.Len(t, metadata.Fields, 1)
	require.Equal(t, "Role", metadata.Fields[0].Name)
	require.Equal(t, "Steward", metadata.Fields[0].Value)

	mockDB.AssertExpectations(t)
	mockQuery.AssertExpectations(t)
}

func stubCanonicalLocalActorReadFallback(t *testing.T, username string) func() {
	t.Helper()

	cfg := lesserconfig.Get()
	previousDomain := cfg.Domain
	cfg.Domain = "example.com"
	t.Cleanup(func() {
		cfg.Domain = previousDomain
	})

	previousClient := getRepositoryDynamoClient
	rawActor, err := attributevalue.MarshalMap(map[string]any{
		"BaseObject": map[string]any{
			"ID":   fmt.Sprintf("https://example.com/users/%s", username),
			"Type": activitypub.PersonType,
		},
		"PreferredUsername": username,
		"Inbox":             fmt.Sprintf("https://example.com/users/%s/inbox", username),
		"Outbox":            fmt.Sprintf("https://example.com/users/%s/outbox", username),
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

	return func() {
		getRepositoryDynamoClient = previousClient
	}
}
