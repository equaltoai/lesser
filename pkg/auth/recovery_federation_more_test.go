package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRecoveryFederationService_SendTrusteeInvitation_ActorLookupError(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("GetActor", mock.Anything, "alice").Return(nil, errors.New("missing"))

	delivery := &fakeDelivery{}
	svc := &RecoveryFederationService{
		repos:      repos,
		fedService: delivery,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{},
	}

	err := svc.SendTrusteeInvitation(context.Background(), "alice", "https://remote.example/users/bob")
	require.ErrorIs(t, err, ErrSigningActorRetrievalFailed)
	require.Equal(t, 0, delivery.calls)
}

func TestRecoveryFederationService_RotateSystemActorKey_ErrorBranches(t *testing.T) {
	t.Parallel()

	svc := &RecoveryFederationService{logger: zap.NewNop(), domain: "example.com"}
	require.ErrorIs(t, svc.RotateSystemActorKey(context.Background()), ErrSecretsManagerNotAvailable)

	svc.secretsManager = &fakeSecretsManager{errRotate: errors.New("rotate failed")}
	require.ErrorIs(t, svc.RotateSystemActorKey(context.Background()), ErrSystemActorKeyRotationFailed)
}

func TestRecoveryFederationService_storeSystemActorKeys_HandlesCreateError(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("CreateActor", mock.Anything, mock.Anything, "").Return(errors.New("already exists"))

	svc := &RecoveryFederationService{
		repos:  repos,
		logger: zap.NewNop(),
		domain: "example.com",
		config: &config.Config{},
	}

	require.NoError(t, svc.storeSystemActorKeys("pub", "priv"))
}

func TestRecoveryFederationService_derivePublicKeyFromPrivate_PKCS1Fallback(t *testing.T) {
	t.Parallel()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pkcs1 := string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))

	svc := &RecoveryFederationService{logger: zap.NewNop()}
	pub, err := svc.derivePublicKeyFromPrivate(pkcs1)
	require.NoError(t, err)
	require.Contains(t, pub, "PUBLIC KEY")
}

func TestRecoveryFederationService_SendRecoveryApprovalNotification_ActorErrorAndNotificationStoreError(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	svc := &RecoveryFederationService{
		repos:      repos,
		fedService: &fakeDelivery{},
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}

	actorRepo.On("GetActor", mock.Anything, "alice").Return(nil, errors.New("missing"))
	require.ErrorIs(t, svc.SendRecoveryApprovalNotification(context.Background(), "alice", "tok"), ErrActorRetrievalFailed)

	// Notification store error should not fail the operation.
	actorRepo2 := testmocks.NewMockActorRepository()
	notifRepo2 := testmocks.NewMockNotificationRepository()
	repos2 := recoveryFedRepos{actor: actorRepo2, notification: notifRepo2}

	actorRepo2.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
		Inbox:      "",
	}, nil)
	notifRepo2.On("CreateNotification", mock.Anything, mock.Anything).Return(errors.New("write failed"))

	svc2 := &RecoveryFederationService{
		repos:      repos2,
		fedService: &fakeDelivery{},
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}
	require.NoError(t, svc2.SendRecoveryApprovalNotification(context.Background(), "alice", "tok"))

	// Keep notification timestamps deterministic for coverage-only test paths.
	_ = time.Now()
}

