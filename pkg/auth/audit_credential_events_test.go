package auth

import (
	"context"
	"testing"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWebAuthnService_CredentialLifecycleAuditsExcludeSensitiveMaterial(t *testing.T) {
	t.Parallel()

	t.Run("authenticated_registration_emits_added_event", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		repo.usersByUsername["alice"] = &storage.User{Username: "alice", PasswordHash: "hash"}
		engine := &fakeWebAuthnEngine{
			beginRegChallenge: "chal-reg",
			createCredential:  &webauthnCredentialFixture,
		}
		auditRepo := newInMemoryAuditRepo()

		svc := &WebAuthnService{
			webAuthn:    engine,
			repo:        repo,
			domain:      "example.com",
			auditLogger: newAuditLoggerForTests(auditRepo),
			parseCreationResponse: func(_ []byte) (*protocol.ParsedCredentialCreationData, error) {
				return &protocol.ParsedCredentialCreationData{}, nil
			},
			parseAssertionResponse: func(_ []byte) (*protocol.ParsedCredentialAssertionData, error) {
				return &protocol.ParsedCredentialAssertionData{}, nil
			},
		}

		_, challenge, err := svc.BeginRegistration(context.Background(), "alice")
		require.NoError(t, err)
		require.NoError(t, svc.FinishRegistration(context.Background(), "alice", challenge, []byte("ignored"), ""))

		require.Equal(t, string(AuditWebAuthnRegistrationCompleted), auditRepo.lastStore.eventType)
		require.Equal(t, "webauthn", auditRepo.lastStore.metadata["authentication_method"])
		require.Equal(t, "added", auditRepo.lastStore.metadata["credential_event"])
		require.Equal(t, "authenticated", auditRepo.lastStore.metadata["registration_mode"])
		require.NotContains(t, auditRepo.lastStore.metadata, "credential_id")
	})

	t.Run("delete_emits_removed_event_without_credential_reference", func(t *testing.T) {
		t.Parallel()

		repo := newInMemoryWebAuthnRepo()
		cred := &storage.WebAuthnCredential{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")}
		repo.credentialsByUsername["alice"] = []*storage.WebAuthnCredential{cred}
		repo.credentialsByID[cred.ID] = cred
		repo.walletsByUsername["alice"] = []*storage.WalletCredential{{Username: "alice", Address: "0xabc", Type: "ethereum", ChainID: 1}}
		auditRepo := newInMemoryAuditRepo()

		svc := &WebAuthnService{
			repo:        repo,
			auditLogger: newAuditLoggerForTests(auditRepo),
		}

		require.NoError(t, svc.DeleteCredential(context.Background(), "alice", cred.ID))
		require.Equal(t, string(AuditWebAuthnCredentialRemoved), auditRepo.lastStore.eventType)
		require.Equal(t, "webauthn", auditRepo.lastStore.metadata["authentication_method"])
		require.Equal(t, "removed", auditRepo.lastStore.metadata["credential_event"])
		require.NotContains(t, auditRepo.lastStore.metadata, "credential_id")
	})
}

func TestWalletService_CredentialLifecycleAuditsExcludeSensitiveMaterial(t *testing.T) {
	t.Parallel()

	repo := newInMemoryWalletRepo()
	repo.passkeysByUser["alice"] = []*storage.WebAuthnCredential{
		{ID: "cred-1", UserID: "alice", PublicKey: []byte("pk")},
	}
	auditRepo := newInMemoryAuditRepo()
	svc := &WalletService{
		repo:        repo,
		logger:      zap.NewNop(),
		auditLogger: newAuditLoggerForTests(auditRepo),
	}

	created, err := svc.LinkWallet(context.Background(), "alice", "0xabc", 1, "ethereum")
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, string(AuditWalletConnected), auditRepo.lastStore.eventType)
	require.Equal(t, "wallet", auditRepo.lastStore.metadata["authentication_method"])
	require.Equal(t, "added", auditRepo.lastStore.metadata["credential_event"])
	require.Equal(t, "ethereum", auditRepo.lastStore.metadata["wallet_type"])
	require.NotContains(t, auditRepo.lastStore.metadata, "address")
	require.NotContains(t, auditRepo.lastStore.metadata, "signature")

	require.NoError(t, svc.UnlinkWallet(context.Background(), "alice", "0xabc"))
	require.Equal(t, string(AuditWalletDisconnected), auditRepo.lastStore.eventType)
	require.Equal(t, "wallet", auditRepo.lastStore.metadata["authentication_method"])
	require.Equal(t, "removed", auditRepo.lastStore.metadata["credential_event"])
	require.NotContains(t, auditRepo.lastStore.metadata, "address")
	require.NotContains(t, auditRepo.lastStore.metadata, "signature")
}

var webauthnCredentialFixture = webauthn.Credential{
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
}

func newAuditLoggerForTests(repo *inMemoryAuditRepo) *AuditLogger {
	return &AuditLogger{
		auditRepo: repo,
		logger:    zap.NewNop(),
		config: &AuditConfig{
			Enabled:     true,
			StoreToDB:   true,
			StoreToFile: false,
			StoreToSIEM: false,
		},
	}
}
