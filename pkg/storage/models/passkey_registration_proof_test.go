package models

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPasskeyRegistrationProof_BeforeCreateSetsKeysTTLAndDefaults(t *testing.T) {
	t.Parallel()

	proof := &PasskeyRegistrationProof{
		ID:           "proof-1",
		Username:     "alice",
		CeremonyID:   "ceremony-1",
		CredentialID: "cred-1",
		PublicKey:    []byte("pk"),
	}

	before := time.Now().UTC()
	require.NoError(t, proof.BeforeCreate())

	require.Equal(t, "PASSKEY_REGISTRATION_PROOF#proof-1", proof.PK)
	require.Equal(t, SKPasskeyRegistrationProof, proof.SK)
	require.False(t, proof.CreatedAt.Before(before))
	require.True(t, proof.ExpiresAt.After(proof.CreatedAt))
	require.Equal(t, proof.ExpiresAt.Unix(), proof.TTL)
	require.WithinDuration(t, proof.CreatedAt.Add(DefaultPasskeyRegistrationProofTTL), proof.ExpiresAt, 2*time.Second)
}

func TestPasskeyRegistrationProof_ValidateRejectsMissingRequiredFields(t *testing.T) {
	t.Parallel()

	proof := &PasskeyRegistrationProof{}
	require.Error(t, proof.BeforeCreate())

	proof = &PasskeyRegistrationProof{
		ID:           "proof-1",
		Username:     "alice",
		CeremonyID:   "ceremony-1",
		CredentialID: "cred-1",
		PublicKey:    []byte("pk"),
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(-1 * time.Minute),
	}
	require.Error(t, proof.Validate())
}

func TestPasskeyRegistrationProof_TableName(t *testing.T) {
	t.Parallel()

	require.Equal(t, MainTableName, PasskeyRegistrationProof{}.TableName())
}

func TestPasskeyRegistrationProof_BeforeUpdateTrimsKeysAndAccessors(t *testing.T) {
	t.Parallel()

	createdAt := time.Now().UTC()
	expiresAt := createdAt.Add(5 * time.Minute)
	proof := &PasskeyRegistrationProof{
		ID:           "  proof-2  ",
		Username:     "  alice  ",
		CeremonyID:   "  ceremony-2  ",
		CredentialID: "  cred-2  ",
		PublicKey:    []byte("pk"),
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	}

	require.NoError(t, proof.BeforeUpdate())
	require.Equal(t, "proof-2", proof.ID)
	require.Equal(t, "alice", proof.Username)
	require.Equal(t, "ceremony-2", proof.CeremonyID)
	require.Equal(t, "cred-2", proof.CredentialID)
	require.Equal(t, "PASSKEY_REGISTRATION_PROOF#proof-2", proof.GetPK())
	require.Equal(t, SKPasskeyRegistrationProof, proof.GetSK())
	require.Equal(t, expiresAt.Unix(), proof.TTL)
}

func TestPasskeyRegistrationProof_IsExpired(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	proof := &PasskeyRegistrationProof{
		ExpiresAt: now.Add(1 * time.Minute),
	}
	require.False(t, proof.IsExpired(now))

	expired := &PasskeyRegistrationProof{
		ExpiresAt: now.Add(-1 * time.Minute),
	}
	require.True(t, expired.IsExpired(time.Time{}))
}

func TestPasskeyRegistrationProof_ValidateMissingFields(t *testing.T) {
	t.Parallel()

	base := PasskeyRegistrationProof{
		ID:           "proof-3",
		Username:     "alice",
		CeremonyID:   "ceremony-3",
		CredentialID: "cred-3",
		PublicKey:    []byte("pk"),
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(5 * time.Minute),
	}

	testCases := []struct {
		name   string
		mutate func(*PasskeyRegistrationProof)
	}{
		{
			name: "missing username",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.Username = ""
			},
		},
		{
			name: "missing ceremony",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.CeremonyID = ""
			},
		},
		{
			name: "missing credential",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.CredentialID = ""
			},
		},
		{
			name: "missing public key",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.PublicKey = nil
			},
		},
		{
			name: "missing created at",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.CreatedAt = time.Time{}
			},
		},
		{
			name: "missing expires at",
			mutate: func(proof *PasskeyRegistrationProof) {
				proof.ExpiresAt = time.Time{}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			proof := base
			tc.mutate(&proof)
			require.Error(t, proof.Validate())
		})
	}
}
