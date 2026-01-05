package auth

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type webAuthnRepoStoreCredentialErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoStoreCredentialErr) StoreWebAuthnCredential(_ context.Context, _ *storage.WebAuthnCredential) error {
	return r.err
}

func TestWebAuthnService_FinishRegistration_MoreErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}

	sessionDataBytes, err := json.Marshal(webauthn.SessionData{Challenge: "c", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)})
	require.NoError(t, err)

	svc := &WebAuthnService{
		webAuthn: webAuthnEngineError{err: errors.New("boom")},
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return nil, errors.New("parse failed")
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	// Missing challenge.
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "missing", []byte("x"), "name"), ErrChallengeNotFound)

	// Wrong user/type/expired.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "bob", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(time.Minute), Type: "registration"}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrChallengeNotFound)
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(time.Minute), Type: "authentication"}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrChallengeNotFound)
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(-time.Minute), Type: "registration"}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrChallengeNotFound)

	// Bad session JSON.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: []byte("{bad"), ExpiresAt: time.Now().Add(time.Minute), Type: "registration"}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrSessionDataDeserialization)

	// parseCreationResponse failure.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(time.Minute), Type: "registration"}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrCredentialResponse)

	// CreateCredential error.
	svc.parseCreationResponse = func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
		return &protocol.ParsedCredentialCreationData{}, nil
	}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrCredentialCreation)

	// Max credentials reached.
	fakeCred := &webauthn.Credential{ID: []byte{1}, PublicKey: []byte("pub")}
	svc.webAuthn = &fakeWebAuthnEngine{createCredential: fakeCred}
	repo.credentialsByUsername["alice"] = make([]*storage.WebAuthnCredential, MaxCredentialsPerUser)
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrMaxCredentialsReached)

	// Store credential error.
	repo.credentialsByUsername["alice"] = nil
	svc.repo = &webAuthnRepoStoreCredentialErr{inMemoryWebAuthnRepo: repo, err: errors.New("store failed")}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "c", []byte("x"), "name"), ErrCredentialStorage)
}

func TestWebAuthnService_FinishLogin_MoreErrorBranches(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{{ID: "AQ==", UserID: "alice", PublicKey: []byte("pub")}}
	repo.credentialsByID["AQ=="] = repo.credentialsByUsername["alice"][0]

	sessionDataBytes, err := json.Marshal(webauthn.SessionData{Challenge: "c", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)})
	require.NoError(t, err)

	svc := &WebAuthnService{
		webAuthn: webAuthnEngineError{err: errors.New("boom")},
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return nil, errors.New("parse failed")
		},
	}

	// Missing challenge.
	_, err = svc.FinishLogin(context.Background(), "alice", "missing", []byte("x"))
	require.ErrorIs(t, err, ErrChallengeNotFound)

	// Wrong type / expired.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(time.Minute), Type: "registration"}
	_, err = svc.FinishLogin(context.Background(), "alice", "c", []byte("x"))
	require.ErrorIs(t, err, ErrChallengeNotFound)
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(-time.Minute), Type: "authentication"}
	_, err = svc.FinishLogin(context.Background(), "alice", "c", []byte("x"))
	require.ErrorIs(t, err, ErrChallengeNotFound)

	// Invalid session data type.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: 123, ExpiresAt: time.Now().Add(time.Minute), Type: "authentication"}
	_, err = svc.FinishLogin(context.Background(), "alice", "c", []byte("x"))
	require.ErrorIs(t, err, ErrInvalidSessionDataType)

	// parseAssertionResponse failure.
	repo.challengesByChallenge["c"] = &storage.WebAuthnChallenge{Challenge: "c", UserID: "alice", SessionData: sessionDataBytes, ExpiresAt: time.Now().Add(time.Minute), Type: "authentication"}
	_, err = svc.FinishLogin(context.Background(), "alice", "c", []byte("x"))
	require.ErrorIs(t, err, ErrCredentialResponse)

	// ValidateLogin error.
	svc.parseAssertionResponse = func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
		return &protocol.ParsedCredentialAssertionData{}, nil
	}
	_, err = svc.FinishLogin(context.Background(), "alice", "c", []byte("x"))
	require.ErrorIs(t, err, ErrCredentialValidation)
}
