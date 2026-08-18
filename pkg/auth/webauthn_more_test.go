package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type webAuthnRepoStoreChallengeFail struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoStoreChallengeFail) StoreWebAuthnChallenge(_ context.Context, _ *storage.WebAuthnChallenge) error {
	return r.err
}

type webAuthnRepoGetCredsFail struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoGetCredsFail) GetUserWebAuthnCredentials(_ context.Context, _ string) ([]*storage.WebAuthnCredential, error) {
	return nil, r.err
}

func TestWebAuthnService_BeginRegistration_ErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	engine := &fakeWebAuthnEngine{beginRegChallenge: "chal-reg"}

	svc := &WebAuthnService{
		webAuthn: engine,
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	// User retrieval failure.
	_, _, err := svc.BeginRegistration(context.Background(), "missing")
	require.ErrorIs(t, err, ErrUserRetrieval)

	repo.usersByUsername["alice"] = &storage.User{Username: "alice"}

	// Credential retrieval error is logged but does not fail registration.
	svc.repo = &webAuthnRepoGetCredsFail{inMemoryWebAuthnRepo: repo, err: errors.New("db down")}
	_, challenge, err := svc.BeginRegistration(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "chal-reg", challenge)

	// Store challenge failure.
	svc.repo = &webAuthnRepoStoreChallengeFail{inMemoryWebAuthnRepo: repo, err: errors.New("store failed")}
	_, _, err = svc.BeginRegistration(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWebAuthnChallengeStorage)
}

func TestWebAuthnService_DeleteCredential_AndUpdateCredentialName_SuccessPaths(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}

	cred1LastUsedAt := time.Unix(100, 0).UTC()
	cred1 := &storage.WebAuthnCredential{
		ID:         "AQ==",
		UserID:     "alice",
		PublicKey:  []byte("pub"),
		Name:       "old",
		LastUsedAt: cred1LastUsedAt,
	}
	cred2 := &storage.WebAuthnCredential{ID: "Ag==", UserID: "alice", PublicKey: []byte("pub2"), Name: "old2"}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{cred1, cred2}
	repo.credentialsByID[cred1.ID] = cred1
	repo.credentialsByID[cred2.ID] = cred2

	svc := &WebAuthnService{
		webAuthn: &fakeWebAuthnEngine{
			beginLoginChallenge: "chal",
			loginCredential:     &webauthn.Credential{ID: []byte{1}},
		},
		repo:   repo,
		domain: "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	require.NoError(t, svc.UpdateCredentialName(context.Background(), "alice", cred1.ID, "new"))
	require.Equal(t, "new", repo.credentialsByID[cred1.ID].Name)
	require.Equal(t, cred1LastUsedAt, repo.credentialsByID[cred1.ID].LastUsedAt)
	require.Equal(t, 1, repo.renameCalls)
	require.Zero(t, repo.updateCalls)

	// Multiple credentials -> delete allowed without checking password presence.
	repo.usersByUsername["alice"].PasswordHash = ""
	require.NoError(t, svc.DeleteCredential(context.Background(), "alice", cred1.ID))

	// Single credential with wallet -> delete allowed.
	repo2 := newInMemoryWebAuthnRepo()
	repo2.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}
	repo2.walletsByUsername["alice"] = []*storage.WalletCredential{{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}}
	only := &storage.WebAuthnCredential{ID: "Aw==", UserID: "alice", PublicKey: []byte("pub")}
	repo2.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{only}
	repo2.credentialsByID[only.ID] = only

	svc2 := &WebAuthnService{
		webAuthn: &fakeWebAuthnEngine{beginRegChallenge: "c"},
		repo:     repo2,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	require.NoError(t, svc2.DeleteCredential(context.Background(), "alice", only.ID))

	// Renames persist the requested name without moving LastUsedAt.
	lastUsedAt := time.Unix(200, 0).UTC()
	repo2.credentialsByID[only.ID] = &storage.WebAuthnCredential{ID: only.ID, UserID: "alice", SignCount: 1, LastUsedAt: lastUsedAt}
	require.NoError(t, svc2.UpdateCredentialName(context.Background(), "alice", only.ID, "renamed"))
	require.Equal(t, "renamed", repo2.credentialsByID[only.ID].Name)
	require.Equal(t, lastUsedAt, repo2.credentialsByID[only.ID].LastUsedAt)
	require.Equal(t, 1, repo2.renameCalls)
	require.Zero(t, repo2.updateCalls)
}
