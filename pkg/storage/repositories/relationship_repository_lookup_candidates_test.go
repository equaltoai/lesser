package repositories

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

func TestRelationshipRepository_relationshipIdentifierCandidates_NormalizesLocalAndRemoteActors(t *testing.T) {
	repo := NewRelationshipRepository(nil, "test-table", zap.NewNop())
	repo.localDomain = "example.com"

	require.Equal(t, []string{"alice"}, repo.relationshipIdentifierCandidates("https://example.com/users/Alice"))
	require.Equal(t, []string{"bob@remote.example", "@bob@remote.example"}, repo.relationshipIdentifierCandidates("https://remote.example/users/@Bob"))
	require.Equal(t, []string{"carol@remote.example", "@carol@remote.example"}, repo.relationshipIdentifierCandidates("@carol@remote.example"))
}

func TestRelationshipRepository_relationshipLookupCandidates_DeduplicatesLegacyRemoteVariants(t *testing.T) {
	repo := NewRelationshipRepository(nil, "test-table", zap.NewNop())
	repo.localDomain = "example.com"

	require.Equal(t, []relationshipLookupKey{
		{follower: "alice", following: "bob@remote.example"},
		{follower: "alice", following: "@bob@remote.example"},
	}, repo.relationshipLookupCandidates("alice", "https://remote.example/users/bob"))
}

func TestRelationshipRepository_relationshipIdentifierCandidates_PreservesUnknownIdentifiers(t *testing.T) {
	repo := NewRelationshipRepository(nil, "test-table", zap.NewNop())
	repo.localDomain = "example.com"

	require.Equal(t, []string{"https://remote.example/users/"}, repo.relationshipIdentifierCandidates(" https://remote.example/users/ "))
}

func TestRelationshipRepository_getRelationshipRecord_FallsBackToLegacyRemoteIdentity(t *testing.T) {
	ctx := context.Background()
	mockDB, mockQuery := setupPermissiveDBAndQuery()
	repo := NewRelationshipRepository(mockDB, "test-table", zap.NewNop())
	repo.localDomain = "example.com"

	mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Return(dynamormerrors.ErrItemNotFound).
		Once()
	mockQuery.On("First", mock.AnythingOfType("*models.RelationshipRecord")).
		Run(func(args mock.Arguments) {
			record := args.Get(0).(*models.RelationshipRecord)
			record.PK = "FOLLOW#alice"
			record.SK = "FOLLOWING#@bob@remote.example"
			record.GSI1SK = "FOLLOWER#alice"
			record.State = models.RelationshipAccepted
		}).
		Return(nil).
		Once()

	record, err := repo.getRelationshipRecord(ctx, "alice", "https://remote.example/users/bob")
	require.NoError(t, err)
	require.Equal(t, "FOLLOW#alice", record.PK)
	require.Equal(t, "FOLLOWING#@bob@remote.example", record.SK)
	require.Equal(t, models.RelationshipAccepted, record.State)
}
