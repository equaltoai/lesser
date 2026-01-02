package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type storageProviderNoAccount struct{}

func (s storageProviderNoAccount) Account() *repositories.AccountRepository          { return nil }
func (s storageProviderNoAccount) Actor() storageinterfaces.ActorRepository         { return nil }
func (s storageProviderNoAccount) Activity() storageinterfaces.ActivityRepository   { return nil }
func (s storageProviderNoAccount) Notification() storageinterfaces.NotificationRepository {
	return nil
}
func (s storageProviderNoAccount) Recovery() *repositories.RecoveryRepository { return nil }
func (s storageProviderNoAccount) Audit() *repositories.AuditRepository       { return nil }

type webAuthnEngineError struct{ err error }

func (e webAuthnEngineError) BeginRegistration(_ webauthn.User, _ ...webauthn.RegistrationOption) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	return nil, nil, e.err
}
func (e webAuthnEngineError) CreateCredential(_ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	return nil, e.err
}
func (e webAuthnEngineError) BeginLogin(_ webauthn.User, _ ...webauthn.LoginOption) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	return nil, nil, e.err
}
func (e webAuthnEngineError) ValidateLogin(_ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	return nil, e.err
}

type webAuthnRepoGetCredsErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoGetCredsErr) GetUserWebAuthnCredentials(_ context.Context, _ string) ([]*storage.WebAuthnCredential, error) {
	return nil, r.err
}

type webAuthnRepoStoreChallengeErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoStoreChallengeErr) StoreWebAuthnChallenge(_ context.Context, _ *storage.WebAuthnChallenge) error {
	return r.err
}

func TestWebAuthnService_ConstructorsAndHelpers(t *testing.T) {
	t.Parallel()

	_, err := NewWebAuthnService(nil, "example.com", "Lesser")
	require.ErrorIs(t, err, ErrWebAuthnServiceInit)

	_, err = NewWebAuthnService(storageProviderNoAccount{}, "example.com", "Lesser")
	require.ErrorIs(t, err, ErrWebAuthnServiceInit)

	repos := testmocks.NewMockRepositoryStorage()
	svc, err := NewWebAuthnService(repos, "example.com", "Lesser")
	require.NoError(t, err)
	require.NotNil(t, svc)

	user := &webAuthnUser{id: "id", name: "name", displayName: "display", credentials: []webauthn.Credential{{ID: []byte{1}}}}
	require.Equal(t, []byte("id"), user.WebAuthnID())
	require.Equal(t, "name", user.WebAuthnName())
	require.Equal(t, "display", user.WebAuthnDisplayName())
	require.Empty(t, user.WebAuthnIcon())
	require.Len(t, user.WebAuthnCredentials(), 1)
}

func TestWebAuthnService_BeginLogin_ErrorAndSuccessPaths(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice"}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{
		{ID: "AA==", UserID: "alice", PublicKey: []byte("pub")},
	}

	svc := &WebAuthnService{
		webAuthn: webAuthnEngineError{err: errors.New("boom")},
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	// Repo error returns ErrCredentialRetrieval.
	repoErr := &webAuthnRepoGetCredsErr{inMemoryWebAuthnRepo: repo, err: errors.New("db down")}
	svc.repo = repoErr
	_, _, err := svc.BeginLogin(context.Background(), "alice")
	require.ErrorIs(t, err, ErrCredentialRetrieval)

	// Engine BeginLogin error returns ErrLoginBegin.
	svc.repo = repo
	_, _, err = svc.BeginLogin(context.Background(), "alice")
	require.ErrorIs(t, err, ErrLoginBegin)

	// Store challenge failure returns ErrWebAuthnChallengeStorage.
	svc.webAuthn = &fakeWebAuthnEngine{beginLoginChallenge: "chal-login"}
	repoStoreErr := &webAuthnRepoStoreChallengeErr{inMemoryWebAuthnRepo: repo, err: errors.New("write failed")}
	svc.repo = repoStoreErr
	_, _, err = svc.BeginLogin(context.Background(), "alice")
	require.ErrorIs(t, err, ErrWebAuthnChallengeStorage)

	// Success path stores the challenge.
	svc.webAuthn = &fakeWebAuthnEngine{beginLoginChallenge: "chal-login"}
	svc.repo = repo
	_, challenge, err := svc.BeginLogin(context.Background(), "alice")
	require.NoError(t, err)
	require.Equal(t, "chal-login", challenge)
	require.Contains(t, repo.challengesByChallenge, "chal-login")

	creds, err := svc.GetUserCredentials(context.Background(), "alice")
	require.NoError(t, err)
	require.Len(t, creds, 1)
}
