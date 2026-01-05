package graph

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/reputation"
	"github.com/stretchr/testify/require"
)

func TestRound12ReputationServiceHelpers_GetReputationService_Guards(t *testing.T) {
	var r *Resolver
	_, err := r.getReputationService()
	require.Error(t, err)

	resolver := &Resolver{}
	_, err = resolver.getReputationService()
	require.Error(t, err)
}

func TestRound12ReputationParity_Mutations_InvalidDocumentsAndValidation(t *testing.T) {
	resolver, storageRepo := newRound12GraphResolver(t)
	mut := &mutationResolver{resolver}

	// Seed actors so vouch conversion can resolve from/to.
	actorRepo := storageRepo.Actor()
	require.NoError(t, actorRepo.CreateActor(context.Background(), &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://localhost/users/alice",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "alice",
	}, ""))
	require.NoError(t, actorRepo.CreateActor(context.Background(), &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			ID:   "https://localhost/users/bob",
			Type: activitypub.PersonType,
		},
		PreferredUsername: "bob",
	}, ""))

	// ExportReputation requires auth.
	_, err := mut.ExportReputation(context.Background())
	require.Error(t, err)

	// ImportReputation validation.
	_, err = mut.ImportReputation(round12AuthContext("alice"), "   ")
	require.Error(t, err)

	// Invalid JSON produces a structured (non-error) result from service; mutation should return it.
	imported, err := mut.ImportReputation(round12AuthContext("alice"), "{not-json")
	require.NoError(t, err)
	require.NotNil(t, imported)
	require.False(t, imported.Success)
	require.NotNil(t, imported.Error)

	verified, err := mut.VerifyReputation(context.Background(), "{not-json")
	require.NoError(t, err)
	require.NotNil(t, verified)
	require.False(t, verified.Valid)
	require.NotNil(t, verified.Error)

	// CreateVouch validation paths.
	_, err = mut.CreateVouch(round12AuthContext("alice"), model.CreateVouchInput{To: "", Confidence: 0.5})
	require.Error(t, err)
	_, err = mut.CreateVouch(round12AuthContext("alice"), model.CreateVouchInput{To: "bob", Confidence: 2})
	require.Error(t, err)

	// Service error path (target normalized when config is present).
	_, err = mut.CreateVouch(round12AuthContext("alice"), model.CreateVouchInput{To: "bob", Confidence: 0.5})
	require.Error(t, err)

	// RevokeVouch validation and error path.
	_, err = mut.RevokeVouch(round12AuthContext("alice"), "   ")
	require.Error(t, err)
	_, err = mut.RevokeVouch(round12AuthContext("alice"), "v1")
	require.Error(t, err)

	// Vouch conversion helpers.
	require.Nil(t, resolver.convertVouchToGraphQL(context.Background(), nil))
	require.Nil(t, resolver.convertVouchToGraphQL(context.Background(), &reputation.Vouch{From: "", To: ""}))

	now := time.Now().UTC()
	revokedAt := now.Add(-time.Hour)
	converted := resolver.convertVouchToGraphQL(context.Background(), &reputation.Vouch{
		ID:                "v1",
		From:              "https://localhost/users/alice",
		To:                "https://localhost/users/bob",
		Confidence:        0.9,
		Context:           "ctx",
		VoucherReputation: 1,
		CreatedAt:         time.Time{},
		ExpiresAt:         time.Time{},
		Active:            true,
		Revoked:           true,
		RevokedAt:         &revokedAt,
	})
	require.NotNil(t, converted)
	require.NotNil(t, converted.RevokedAt)
	require.NotNil(t, converted.From)
	require.NotNil(t, converted.To)
}

