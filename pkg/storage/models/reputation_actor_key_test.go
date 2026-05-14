package models_test

import (
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestReputationActorPartitionKeyForRecordBindsRemoteDomain(t *testing.T) {
	localRep := &storage.Reputation{
		InstanceURL:  "https://example.com",
		CalculatedAt: time.Now().UTC(),
	}
	localPK, err := models.ReputationActorPartitionKeyForRecord("https://example.com/users/alice", localRep)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#alice", localPK)

	remotePK, err := models.ReputationActorPartitionKeyForRecord("https://remote.example/users/alice", localRep)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#https://remote.example/users/alice", remotePK)
	require.NotEqual(t, localPK, remotePK)

	otherRemotePK, err := models.ReputationActorPartitionKeyForRecord("https://other.example/users/alice", localRep)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#https://other.example/users/alice", otherRemotePK)
	require.NotEqual(t, remotePK, otherRemotePK)
}

func TestReputationActorPartitionKeyCandidatesIncludeLegacyFallback(t *testing.T) {
	candidates, err := models.ReputationActorPartitionKeyCandidates("https://remote.example/users/alice")
	require.NoError(t, err)
	require.Equal(t, []string{
		"ACTOR#https://remote.example/users/alice",
		"ACTOR#alice",
	}, candidates)
}

func TestReputationActorIDHelpersCanonicalizeAndRejectUnsafeIDs(t *testing.T) {
	require.True(t, models.ReputationActorIDsMatch(
		"https://Example.com/users/alice/",
		"https://example.com/users/alice",
	))
	require.False(t, models.ReputationActorIDsMatch(
		"https://remote.example/users/alice",
		"https://other.example/users/alice",
	))
	require.False(t, models.ReputationActorIDsMatch(
		"acct:alice@example.com",
		"https://example.com/users/alice",
	))

	_, err := models.ReputationActorPartitionKeyCandidates("acct:alice@example.com")
	require.Error(t, err)

	_, err = models.ReputationActorPartitionKeyForRecord("https://example.com/users/", map[string]any{
		"instance_url": "https://example.com",
	})
	require.Error(t, err)
}

func TestReputationActorPartitionKeyForRecordReadsInstanceURLAliases(t *testing.T) {
	for _, reputation := range []map[string]any{
		{"instanceURL": "https://example.com"},
		{"instance_url": "https://example.com"},
		{"instance": "https://example.com"},
	} {
		pk, err := models.ReputationActorPartitionKeyForRecord("https://example.com/users/alice", reputation)
		require.NoError(t, err)
		require.Equal(t, "ACTOR#alice", pk)
	}
}

func TestReputationActorPartitionKeyForRecordForcedCanonicalForImportedRemote(t *testing.T) {
	remoteImported := &storage.Reputation{
		ActorID:                "https://remote1.example/users/alice",
		InstanceURL:            "https://remote1.example",
		ForceCanonicalActorKey: true,
		CalculatedAt:           time.Now().UTC(),
	}

	pk, err := models.ReputationActorPartitionKeyForRecord(remoteImported.ActorID, remoteImported)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#https://remote1.example/users/alice", pk)

	secondRemote := &storage.Reputation{
		ActorID:                "https://remote2.example/users/alice",
		InstanceURL:            "https://remote2.example",
		ForceCanonicalActorKey: true,
		CalculatedAt:           time.Now().UTC(),
	}
	secondPK, err := models.ReputationActorPartitionKeyForRecord(secondRemote.ActorID, secondRemote)
	require.NoError(t, err)
	require.Equal(t, "ACTOR#https://remote2.example/users/alice", secondPK)
	require.NotEqual(t, pk, secondPK)
}
