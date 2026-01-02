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

	cred1 := &storage.WebAuthnCredential{ID: "AQ==", UserID: "alice", PublicKey: []byte("pub"), Name: "old"}
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

	// Multiple credentials -> delete allowed without checking password presence.
	repo.usersByUsername["alice"].PasswordHash = ""
	require.NoError(t, svc.DeleteCredential(context.Background(), "alice", cred1.ID))

	// Single credential with password -> delete allowed.
	repo2 := newInMemoryWebAuthnRepo()
	repo2.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}
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

	// Ensure last-used update path is exercised.
	repo2.credentialsByID[only.ID] = &storage.WebAuthnCredential{ID: only.ID, UserID: "alice", SignCount: 1, LastUsedAt: time.Now().Add(-time.Hour)}
	require.NoError(t, svc2.UpdateCredentialName(context.Background(), "alice", only.ID, "renamed"))
}

