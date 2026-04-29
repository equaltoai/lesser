package accounts

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/equaltoai/lesser/pkg/streaming"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type typedNilRelationshipStorage struct {
	*MockRepositoryStorage
}

func (s *typedNilRelationshipStorage) Relationship() interfaces.ConcreteRelationshipRepository {
	var repo *repositories.RelationshipRepository
	return repo
}

func TestService_RelationshipRepositoryTypedNilGuards(t *testing.T) {
	svc := NewService(&typedNilRelationshipStorage{MockRepositoryStorage: NewMockRepositoryStorage()}, streaming.NewMockPublisher(), nil, nil, nil, zap.NewNop(), "example.com")

	require.ErrorIs(t, svc.validateRelationshipStorage(), ErrRelationshipRepositoryNotAvailable)

	require.False(t, svc.checkBlocking(context.Background(), svc.storage.Relationship(), "alice", "bob"))
	require.False(t, svc.checkFollowingStatus(context.Background(), svc.storage.Relationship(), "alice", "bob"))
	require.False(t, svc.checkMutingStatus(context.Background(), svc.storage.Relationship(), "alice", "bob"))
	require.False(t, svc.checkMutingNotifications(context.Background(), svc.storage.Relationship(), "alice", "bob", true))
	require.False(t, svc.checkFollowRequest(context.Background(), svc.storage.Relationship(), "alice", "bob"))
}
