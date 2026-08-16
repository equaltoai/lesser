package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type inMemoryWebAuthnRepo struct {
	usersByUsername       map[string]*storage.User
	credentialsByUsername map[string][]*storage.WebAuthnCredential
	credentialsByID       map[string]*storage.WebAuthnCredential
	challengesByChallenge map[string]*storage.WebAuthnChallenge
	walletsByUsername     map[string][]*storage.WalletCredential

	updateCalls int
	renameCalls int
}

func newInMemoryWebAuthnRepo() *inMemoryWebAuthnRepo {
	return &inMemoryWebAuthnRepo{
		usersByUsername:       make(map[string]*storage.User),
		credentialsByUsername: make(map[string][]*storage.WebAuthnCredential),
		credentialsByID:       make(map[string]*storage.WebAuthnCredential),
		challengesByChallenge: make(map[string]*storage.WebAuthnChallenge),
		walletsByUsername:     make(map[string][]*storage.WalletCredential),
	}
}

func (r *inMemoryWebAuthnRepo) GetUser(_ context.Context, username string) (*storage.User, error) {
	user, ok := r.usersByUsername[username]
	if !ok {
		return nil, errors.New("not found")
	}
	return user, nil
}

func (r *inMemoryWebAuthnRepo) GetUserWebAuthnCredentials(_ context.Context, username string) ([]*storage.WebAuthnCredential, error) {
	return append([]*storage.WebAuthnCredential(nil), r.credentialsByUsername[username]...), nil
}

func (r *inMemoryWebAuthnRepo) GetUserWalletCredentials(_ context.Context, username string) ([]*storage.WalletCredential, error) {
	return append([]*storage.WalletCredential(nil), r.walletsByUsername[username]...), nil
}

func (r *inMemoryWebAuthnRepo) StoreWebAuthnChallenge(_ context.Context, challenge *storage.WebAuthnChallenge) error {
	r.challengesByChallenge[challenge.Challenge] = challenge
	return nil
}

func (r *inMemoryWebAuthnRepo) GetWebAuthnChallenge(_ context.Context, challenge string) (*storage.WebAuthnChallenge, error) {
	item, ok := r.challengesByChallenge[challenge]
	if !ok {
		return nil, errors.New("not found")
	}
	return item, nil
}

func (r *inMemoryWebAuthnRepo) DeleteWebAuthnChallenge(_ context.Context, challenge string) error {
	delete(r.challengesByChallenge, challenge)
	return nil
}

func (r *inMemoryWebAuthnRepo) StoreWebAuthnCredential(_ context.Context, credential *storage.WebAuthnCredential) error {
	r.credentialsByUsername[credential.UserID] = append(r.credentialsByUsername[credential.UserID], credential)
	r.credentialsByID[credential.ID] = credential
	return nil
}

func (r *inMemoryWebAuthnRepo) GetWebAuthnCredential(_ context.Context, credentialID string) (*storage.WebAuthnCredential, error) {
	cred, ok := r.credentialsByID[credentialID]
	if !ok {
		return nil, errors.New("not found")
	}
	return cred, nil
}

func (r *inMemoryWebAuthnRepo) DeleteWebAuthnCredential(_ context.Context, credentialID string) error {
	delete(r.credentialsByID, credentialID)
	return nil
}

func (r *inMemoryWebAuthnRepo) UpdateWebAuthnCredentialName(_ context.Context, credentialID string, name string) error {
	r.renameCalls++
	cred, ok := r.credentialsByID[credentialID]
	if ok {
		cred.Name = name
	}
	return nil
}

func (r *inMemoryWebAuthnRepo) UpdateWebAuthnAuthenticationState(
	_ context.Context,
	credentialID string,
	signCount uint32,
	cloneWarning bool,
	backupState bool,
	lastUsedAt time.Time,
) error {
	r.updateCalls++
	cred, ok := r.credentialsByID[credentialID]
	if ok {
		cred.SignCount = signCount
		cred.CloneWarning = cloneWarning
		cred.BackupState = backupState
		cred.LastUsedAt = lastUsedAt
	}
	return nil
}

type fakeWebAuthnEngine struct {
	beginRegChallenge   string
	beginLoginChallenge string
	createCredential    *webauthn.Credential
	loginCredential     *webauthn.Credential
}

func (f *fakeWebAuthnEngine) BeginRegistration(user webauthn.User, _ ...webauthn.RegistrationOption) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	return &protocol.CredentialCreation{}, &webauthn.SessionData{
		Challenge: f.beginRegChallenge,
		UserID:    user.WebAuthnID(),
		Expires:   time.Now().Add(time.Minute),
	}, nil
}

func (f *fakeWebAuthnEngine) CreateCredential(_ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialCreationData) (*webauthn.Credential, error) {
	return f.createCredential, nil
}

func (f *fakeWebAuthnEngine) BeginLogin(user webauthn.User, _ ...webauthn.LoginOption) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	return &protocol.CredentialAssertion{}, &webauthn.SessionData{
		Challenge: f.beginLoginChallenge,
		UserID:    user.WebAuthnID(),
		Expires:   time.Now().Add(time.Minute),
	}, nil
}

func (f *fakeWebAuthnEngine) ValidateLogin(_ webauthn.User, _ webauthn.SessionData, _ *protocol.ParsedCredentialAssertionData) (*webauthn.Credential, error) {
	return f.loginCredential, nil
}

func TestWebAuthnService_BeginAndFinishRegistration_SuccessAndErrorPaths(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hashed"}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{
		{ID: base64.StdEncoding.EncodeToString([]byte{1}), UserID: "alice", PublicKey: []byte("pk")},
	}

	engine := &fakeWebAuthnEngine{
		beginRegChallenge: "chal-reg",
		createCredential: &webauthn.Credential{
			ID:              []byte{9, 9},
			PublicKey:       []byte("pub"),
			AttestationType: "none",
			Flags: webauthn.CredentialFlags{
				BackupEligible: true,
				BackupState:    false,
			},
			Authenticator: webauthn.Authenticator{
				AAGUID:    []byte{0, 1, 2},
				SignCount: 7,
			},
		},
	}

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

	options, challenge, err := svc.BeginRegistration(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, options)
	require.Equal(t, "chal-reg", challenge)

	// Invalid session data type error.
	repo.challengesByChallenge["bad"] = &storage.WebAuthnChallenge{
		Challenge:   "bad",
		UserID:      "alice",
		SessionData: 123,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "registration",
	}
	require.ErrorIs(t, svc.FinishRegistration(context.Background(), "alice", "bad", []byte("x"), ""), ErrInvalidSessionDataType)

	// Success path.
	require.NoError(t, svc.FinishRegistration(context.Background(), "alice", challenge, []byte("ignored"), ""))
	_, stillExists := repo.challengesByChallenge[challenge]
	require.False(t, stillExists)
	require.Len(t, repo.credentialsByUsername["alice"], 2)
}

func TestWebAuthnService_BeginLoginAndFinishLogin_SuccessAndCredentialNotFound(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hashed"}

	credID := base64.StdEncoding.EncodeToString([]byte{7, 7})
	previousLastUsedAt := time.Unix(123, 0).UTC()
	credentialRecord := &storage.WebAuthnCredential{ID: credID, UserID: "alice", PublicKey: []byte("pub"), SignCount: 1, LastUsedAt: previousLastUsedAt}
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{
		credentialRecord,
	}
	repo.credentialsByID[credID] = credentialRecord

	engine := &fakeWebAuthnEngine{
		beginLoginChallenge: "chal-login",
		loginCredential: &webauthn.Credential{
			ID: []byte{7, 7},
			Authenticator: webauthn.Authenticator{
				SignCount:    2,
				CloneWarning: true,
			},
			Flags: webauthn.CredentialFlags{BackupState: true},
		},
	}

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

	// No credentials -> ErrUserHasNoCredentials.
	repo.credentialsByUsername["bob"] = nil
	_, _, err := svc.BeginLogin(context.Background(), "bob")
	require.ErrorIs(t, err, ErrUserHasNoCredentials)

	// Set up login challenge (store sessionData as string to cover type switch).
	sessionData := webauthn.SessionData{Challenge: "chal-login", UserID: []byte("alice"), Expires: time.Now().Add(time.Minute)}
	sessionBytes, err := json.Marshal(sessionData)
	require.NoError(t, err)
	repo.challengesByChallenge["chal-login"] = &storage.WebAuthnChallenge{
		Challenge:   "chal-login",
		UserID:      "alice",
		SessionData: string(sessionBytes),
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	}

	used, err := svc.FinishLogin(context.Background(), "alice", "chal-login", []byte("ignored"))
	require.NoError(t, err)
	require.Equal(t, credID, used.ID)
	require.Equal(t, uint32(2), used.SignCount)
	require.True(t, used.CloneWarning)
	require.True(t, used.BackupState)
	require.True(t, used.LastUsedAt.After(previousLastUsedAt))
	require.GreaterOrEqual(t, repo.updateCalls, 1)
	require.Equal(t, used.LastUsedAt, repo.credentialsByID[credID].LastUsedAt)
	require.Equal(t, uint32(2), repo.credentialsByID[credID].SignCount)
	require.True(t, repo.credentialsByID[credID].CloneWarning)
	require.True(t, repo.credentialsByID[credID].BackupState)

	// Credential not found in map.
	engine.loginCredential = &webauthn.Credential{ID: []byte{8, 8}}
	repo.challengesByChallenge["chal-login"] = &storage.WebAuthnChallenge{
		Challenge:   "chal-login",
		UserID:      "alice",
		SessionData: sessionBytes,
		ExpiresAt:   time.Now().Add(time.Minute),
		Type:        "authentication",
	}
	_, err = svc.FinishLogin(context.Background(), "alice", "chal-login", []byte("ignored"))
	require.ErrorIs(t, err, ErrCredentialNotFound)
}

func TestWebAuthnService_DeleteAndUpdateCredentialName(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: ""}

	credID := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
	repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{
		{ID: credID, UserID: "alice", PublicKey: []byte("pub"), Name: "old"},
	}
	repo.credentialsByID[credID] = repo.credentialsByUsername["alice"][0]

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

	// Cannot delete last auth method when no password exists.
	require.ErrorIs(t, svc.DeleteCredential(context.Background(), "alice", credID), ErrLastAuthMethodDelete)

	// Credential ownership mismatch.
	repo.credentialsByID[credID].UserID = "bob"
	require.ErrorIs(t, svc.DeleteCredential(context.Background(), "alice", credID), ErrCredentialNotFound)

	// Update credential name ownership mismatch.
	require.ErrorIs(t, svc.UpdateCredentialName(context.Background(), "alice", credID, "new"), ErrCredentialNotFound)
}
