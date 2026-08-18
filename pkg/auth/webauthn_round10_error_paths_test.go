package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type webAuthnRepoGetCredentialErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoGetCredentialErr) GetWebAuthnCredential(_ context.Context, _ string) (*storage.WebAuthnCredential, error) {
	return nil, r.err
}

type webAuthnRepoGetWalletErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoGetWalletErr) GetUserWalletCredentials(_ context.Context, _ string) ([]*storage.WalletCredential, error) {
	return nil, r.err
}

type webAuthnRepoUpdateAuthStateErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoUpdateAuthStateErr) UpdateWebAuthnAuthenticationState(
	_ context.Context,
	_ string,
	_ uint32,
	_ bool,
	_ bool,
	_ time.Time,
) error {
	r.updateCalls++
	return r.err
}

func TestWebAuthnService_FinishLogin_UpdateStateErrorStillReturnsCredential(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	credID := base64.StdEncoding.EncodeToString([]byte{7, 7})
	credentialRecord := &storage.WebAuthnCredential{ID: credID, UserID: "alice", PublicKey: []byte("pub"), SignCount: 1}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{credentialRecord}
	repo.credentialsByID[credID] = credentialRecord

	sessionData := webauthn.SessionData{Challenge: "chal-login", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)}
	sessionBytes, err := json.Marshal(sessionData)
	require.NoError(t, err)

	repo.challengesByChallenge["chal-login"] = &storage.WebAuthnChallenge{
		Challenge:   "chal-login",
		UserID:      "alice",
		SessionData: sessionBytes,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	}

	svc := &WebAuthnService{
		webAuthn: &fakeWebAuthnEngine{
			loginCredential: &webauthn.Credential{
				ID: []byte{7, 7},
				Authenticator: webauthn.Authenticator{
					SignCount:    2,
					CloneWarning: true,
				},
				Flags: webauthn.CredentialFlags{BackupState: true},
			},
		},
		repo:   &webAuthnRepoUpdateAuthStateErr{inMemoryWebAuthnRepo: repo, err: errors.New("update failed")},
		domain: "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	used, err := svc.FinishLogin(context.Background(), "alice", "chal-login", []byte("ignored"))
	require.NoError(t, err)
	require.Equal(t, credID, used.ID)
	require.Equal(t, uint32(2), used.SignCount)
	require.True(t, used.CloneWarning)
	require.True(t, used.BackupState)
	require.Equal(t, 1, repo.updateCalls)
	_, stillExists := repo.challengesByChallenge["chal-login"]
	require.False(t, stillExists)
}

func TestWebAuthnService_FinishLogin_ErrorBranches_InvalidSessionAndCredentialLoad(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.challengesByChallenge["bad-type"] = &storage.WebAuthnChallenge{
		Challenge:   "bad-type",
		UserID:      "alice",
		SessionData: 123,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	}

	svc := &WebAuthnService{
		webAuthn: &fakeWebAuthnEngine{},
		repo:     repo,
		domain:   "example.com",
		parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
		parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
			return &protocol.ParsedCredentialAssertionData{}, nil
		},
	}

	used, err := svc.FinishLogin(context.Background(), "alice", "bad-type", []byte("ignored"))
	require.Nil(t, used)
	require.ErrorIs(t, err, ErrInvalidSessionDataType)

	sessionData := webauthn.SessionData{Challenge: "cred-load-fail", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)}
	sessionBytes, marshalErr := json.Marshal(sessionData)
	require.NoError(t, marshalErr)

	repo2 := newInMemoryWebAuthnRepo()
	repo2.challengesByChallenge["cred-load-fail"] = &storage.WebAuthnChallenge{
		Challenge:   "cred-load-fail",
		UserID:      "alice",
		SessionData: sessionBytes,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	}

	svc.repo = &webAuthnRepoGetCredsFail{inMemoryWebAuthnRepo: repo2, err: errors.New("credential load failed")}
	used, err = svc.FinishLogin(context.Background(), "alice", "cred-load-fail", []byte("ignored"))
	require.Nil(t, used)
	require.ErrorIs(t, err, ErrCredentialRetrieval)
}

func TestWebAuthnService_ConstructorParsersAndRegistrationErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("default parsers are callable", func(t *testing.T) {
		t.Parallel()

		svc, err := NewWebAuthnService(testmocks.NewMockRepositoryStorage(), "example.com", "Lesser")
		require.NoError(t, err)

		_, err = svc.parseCreationResponse([]byte("not-json"))
		require.Error(t, err)

		_, err = svc.parseAssertionResponse([]byte("not-json"))
		require.Error(t, err)
	})

	t.Run("begin registration returns engine error", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		repo.usersByUsername["alice"] = &storage.User{Username: "alice"}

		svc := &WebAuthnService{
			webAuthn: webAuthnEngineError{err: errors.New("begin failed")},
			repo:     repo,
			domain:   "example.com",
		}

		_, _, err := svc.BeginRegistration(context.Background(), "alice")
		require.ErrorIs(t, err, ErrRegistrationBegin)
	})

	t.Run("finish registration returns user retrieval error after session decode", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		sessionData := webauthn.SessionData{Challenge: "chal-reg", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)}
		sessionBytes, err := json.Marshal(sessionData)
		require.NoError(t, err)

		repo.challengesByChallenge["chal-reg"] = &storage.WebAuthnChallenge{
			Challenge:   "chal-reg",
			UserID:      "alice",
			SessionData: sessionBytes,
			ExpiresAt:   time.Now().Add(time.Minute),
			Type:        "registration",
		}

		svc := &WebAuthnService{
			webAuthn: &fakeWebAuthnEngine{},
			repo:     repo,
			domain:   "example.com",
		}

		err = svc.FinishRegistration(context.Background(), "alice", "chal-reg", []byte("ignored"), "Passkey")
		require.ErrorIs(t, err, ErrUserRetrieval)
	})

	t.Run("finish login returns session deserialization error", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		repo.challengesByChallenge["bad-json"] = &storage.WebAuthnChallenge{
			Challenge:   "bad-json",
			UserID:      "alice",
			SessionData: []byte("{"),
			ExpiresAt:   time.Now().Add(time.Minute),
			Type:        "authentication",
		}

		svc := &WebAuthnService{
			webAuthn: &fakeWebAuthnEngine{},
			repo:     repo,
			domain:   "example.com",
			parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
				return &protocol.ParsedCredentialAssertionData{}, nil
			},
		}

		used, err := svc.FinishLogin(context.Background(), "alice", "bad-json", []byte("ignored"))
		require.Nil(t, used)
		require.ErrorIs(t, err, ErrSessionDataDeserialization)
	})
}

func TestWebAuthnService_DeleteCredentialAndRename_ErrorBranches(t *testing.T) {
	t.Parallel()

	t.Run("delete credential returns lookup error", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		svc := &WebAuthnService{repo: &webAuthnRepoGetCredentialErr{inMemoryWebAuthnRepo: repo, err: errors.New("lookup failed")}}

		require.ErrorContains(t, svc.DeleteCredential(context.Background(), "alice", "cred-1"), "lookup failed")
	})

	t.Run("delete credential returns list error after ownership check", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		cred := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
		repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{cred}
		repo.credentialsByID[cred.ID] = cred

		svc := &WebAuthnService{repo: &webAuthnRepoGetCredsFail{inMemoryWebAuthnRepo: repo, err: errors.New("list failed")}}
		require.ErrorContains(t, svc.DeleteCredential(context.Background(), "alice", cred.ID), "list failed")
	})

	t.Run("delete credential returns wallet error for last auth method fallback", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		cred := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
		repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{cred}
		repo.credentialsByID[cred.ID] = cred

		svc := &WebAuthnService{repo: &webAuthnRepoGetWalletErr{inMemoryWebAuthnRepo: repo, err: errors.New("wallet lookup failed")}}
		require.ErrorContains(t, svc.DeleteCredential(context.Background(), "alice", cred.ID), "wallet lookup failed")
	})

	t.Run("rename returns lookup error", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		svc := &WebAuthnService{repo: &webAuthnRepoGetCredentialErr{inMemoryWebAuthnRepo: repo, err: errors.New("lookup failed")}}

		require.ErrorContains(t, svc.UpdateCredentialName(context.Background(), "alice", "cred-1", "new-name"), "lookup failed")
	})
}
