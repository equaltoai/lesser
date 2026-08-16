package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
)

type webAuthnRepoStoreProofErr struct {
	*inMemoryWebAuthnRepo
	err error
}

func (r *webAuthnRepoStoreProofErr) StorePasskeyRegistrationProof(context.Context, *storagemodels.PasskeyRegistrationProof) error {
	return r.err
}

func TestSignupRegistrationOptions_RequireUserVerification(t *testing.T) {
	t.Parallel()

	options := &protocol.PublicKeyCredentialCreationOptions{}
	for _, opt := range signupRegistrationOptions() {
		opt(options)
	}

	require.Equal(t, protocol.VerificationRequired, options.AuthenticatorSelection.UserVerification)
}

func TestWebAuthnService_BeginAndFinishSignup_Success(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWebAuthnRepo()
	svc := &WebAuthnService{
		webAuthn: &fakeWebAuthnEngine{
			beginRegChallenge: "chal-signup",
			createCredential: &webauthn.Credential{
				ID:              []byte("cred-signup"),
				PublicKey:       []byte("public-key"),
				AttestationType: "packed",
				Flags: webauthn.CredentialFlags{
					UserPresent:    true,
					UserVerified:   true,
					BackupEligible: true,
					BackupState:    true,
				},
				Authenticator: webauthn.Authenticator{
					AAGUID:       []byte("aaguid"),
					SignCount:    7,
					CloneWarning: true,
				},
			},
		},
		repo: repo,
		parseCreationResponse: func([]byte) (*protocol.ParsedCredentialCreationData, error) {
			return &protocol.ParsedCredentialCreationData{}, nil
		},
	}

	options, challenge, err := svc.BeginSignup(context.Background(), "alice")
	require.NoError(t, err)
	require.NotNil(t, options)
	require.Equal(t, "chal-signup", challenge)

	storedChallenge, ok := repo.challengesByChallenge[challenge]
	require.True(t, ok)
	require.Equal(t, "alice", storedChallenge.UserID)
	require.Equal(t, webAuthnChallengeTypeSignup, storedChallenge.Type)

	proofID, err := svc.FinishSignup(context.Background(), "alice", challenge, []byte("ignored"))
	require.NoError(t, err)
	require.NotEmpty(t, proofID)

	proof, ok := repo.proofsByID[proofID]
	require.True(t, ok)
	require.Equal(t, "alice", proof.Username)
	require.Equal(t, challenge, proof.CeremonyID)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("cred-signup")), proof.CredentialID)
	require.Equal(t, []byte("public-key"), proof.PublicKey)
	require.Equal(t, "packed", proof.AttestationType)
	require.Equal(t, []byte("aaguid"), proof.AAGUID)
	require.EqualValues(t, 7, proof.SignCount)
	require.True(t, proof.CloneWarning)
	require.True(t, proof.BackupEligible)
	require.True(t, proof.BackupState)
	require.False(t, proof.Consumed)

	_, challengeStillStored := repo.challengesByChallenge[challenge]
	require.False(t, challengeStillStored)
}

func TestWebAuthnService_FinishSignup_ErrorBranches(t *testing.T) {
	t.Parallel()

	baseChallenge := func() *storage.WebAuthnChallenge {
		return &storage.WebAuthnChallenge{
			Challenge:   "chal-signup",
			UserID:      "alice",
			SessionData: []byte(`{"challenge":"chal-signup"}`),
			ExpiresAt:   time.Now().Add(time.Minute),
			Type:        webAuthnChallengeTypeSignup,
		}
	}

	baseCredential := &webauthn.Credential{
		ID:              []byte("cred-signup"),
		PublicKey:       []byte("public-key"),
		AttestationType: "packed",
		Flags: webauthn.CredentialFlags{
			UserPresent:  true,
			UserVerified: true,
		},
		Authenticator: webauthn.Authenticator{
			AAGUID:    []byte("aaguid"),
			SignCount: 9,
		},
	}

	t.Run("missing challenge", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		svc := &WebAuthnService{repo: repo}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "missing", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrChallengeNotFound)
	})

	t.Run("username mismatch", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		repo.challengesByChallenge["chal-signup"] = baseChallenge()
		svc := &WebAuthnService{repo: repo}
		proofID, err := svc.FinishSignup(context.Background(), "mallory", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrChallengeNotFound)
	})

	t.Run("expired challenge", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		challenge := baseChallenge()
		challenge.ExpiresAt = time.Now().Add(-time.Minute)
		repo.challengesByChallenge["chal-signup"] = challenge
		svc := &WebAuthnService{repo: repo}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrChallengeNotFound)
	})

	t.Run("invalid session data type", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		challenge := baseChallenge()
		challenge.SessionData = 123
		repo.challengesByChallenge["chal-signup"] = challenge
		svc := &WebAuthnService{repo: repo}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrInvalidSessionDataType)
	})

	t.Run("parse response failure", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		repo.challengesByChallenge["chal-signup"] = baseChallenge()
		svc := &WebAuthnService{
			repo: repo,
			parseCreationResponse: func([]byte) (*protocol.ParsedCredentialCreationData, error) {
				return nil, errors.New("bad-response")
			},
		}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrCredentialResponse)
	})

	t.Run("credential verification failure", func(t *testing.T) {
		repo := newInMemoryWebAuthnRepo()
		repo.challengesByChallenge["chal-signup"] = baseChallenge()
		svc := &WebAuthnService{
			webAuthn: webAuthnEngineError{err: errors.New("credential failed")},
			repo:     repo,
			parseCreationResponse: func([]byte) (*protocol.ParsedCredentialCreationData, error) {
				return &protocol.ParsedCredentialCreationData{}, nil
			},
		}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrCredentialCreation)
	})

	t.Run("proof storage failure leaves challenge intact", func(t *testing.T) {
		repo := &webAuthnRepoStoreProofErr{
			inMemoryWebAuthnRepo: newInMemoryWebAuthnRepo(),
			err:                  errors.New("store failed"),
		}
		repo.challengesByChallenge["chal-signup"] = baseChallenge()
		svc := &WebAuthnService{
			webAuthn: &fakeWebAuthnEngine{createCredential: baseCredential},
			repo:     repo,
			parseCreationResponse: func([]byte) (*protocol.ParsedCredentialCreationData, error) {
				return &protocol.ParsedCredentialCreationData{}, nil
			},
		}
		proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
		require.Empty(t, proofID)
		require.ErrorIs(t, err, ErrCredentialStorage)
		_, ok := repo.challengesByChallenge["chal-signup"]
		require.True(t, ok)
	})
}

func TestWebAuthnService_FinishSignup_RejectsNonSignupChallengeTypes(t *testing.T) {
	t.Parallel()

	baseChallenge := &storage.WebAuthnChallenge{
		Challenge:   "chal-signup",
		UserID:      "alice",
		SessionData: []byte(`{"challenge":"chal-signup"}`),
		ExpiresAt:   time.Now().Add(time.Minute),
	}

	for _, challengeType := range []string{webAuthnChallengeTypeRegistration, webAuthnChallengeTypeAuthentication} {
		t.Run(challengeType, func(t *testing.T) {
			repo := newInMemoryWebAuthnRepo()
			challenge := *baseChallenge
			challenge.Type = challengeType
			repo.challengesByChallenge["chal-signup"] = &challenge

			svc := &WebAuthnService{repo: repo}
			proofID, err := svc.FinishSignup(context.Background(), "alice", "chal-signup", []byte("ignored"))
			require.Empty(t, proofID)
			require.ErrorIs(t, err, ErrChallengeNotFound)
		})
	}
}
