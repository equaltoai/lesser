package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type recoveryFedRepos struct {
	actor        *testmocks.MockActorRepository
	notification *testmocks.MockNotificationRepository
}

func (r recoveryFedRepos) Account() *repositories.AccountRepository        { return nil }
func (r recoveryFedRepos) Actor() interfaces.ActorRepository               { return r.actor }
func (r recoveryFedRepos) Activity() interfaces.ActivityRepository         { return nil }
func (r recoveryFedRepos) Notification() interfaces.NotificationRepository { return r.notification }
func (r recoveryFedRepos) Recovery() *repositories.RecoveryRepository      { return nil }
func (r recoveryFedRepos) Audit() *repositories.AuditRepository            { return nil }

type fakeDelivery struct {
	calls int
	err   error
}

func (f *fakeDelivery) DeliverActivity(_ context.Context, _ *activitypub.Activity, _ string, _ *activitypub.Actor) error {
	f.calls++
	return f.err
}

type fakeConfirmer struct {
	calls int
	err   error
}

func (f *fakeConfirmer) ConfirmRecovery(_ context.Context, _, _ string) error {
	f.calls++
	return f.err
}

type fakeSecretsManager struct {
	privateKey string
	errGet     error

	publicKeyOnRotate  string
	privateKeyOnRotate string
	errRotate          error
}

func (f *fakeSecretsManager) StorePrivateKey(_ context.Context, _, _ string) error { return nil }
func (f *fakeSecretsManager) RetrievePrivateKey(_ context.Context, _ string) (string, error) {
	if f.errGet != nil {
		return "", f.errGet
	}
	return f.privateKey, nil
}
func (f *fakeSecretsManager) DeletePrivateKey(_ context.Context, _ string) error { return nil }
func (f *fakeSecretsManager) GenerateAndStoreKeyPair(_ context.Context, _ string) (string, string, error) {
	return "pub", "priv", nil
}
func (f *fakeSecretsManager) RotateKey(_ context.Context, _ string) (string, string, error) {
	if f.errRotate != nil {
		return "", "", f.errRotate
	}
	return f.publicKeyOnRotate, f.privateKeyOnRotate, nil
}

func TestRecoveryFederationService_SendTrusteeInvitation(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil)

	delivery := &fakeDelivery{}
	svc := &RecoveryFederationService{
		repos:      repos,
		fedService: delivery,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}

	require.NoError(t, svc.SendTrusteeInvitation(context.Background(), "alice", "https://remote.example/users/bob"))
	require.Equal(t, 1, delivery.calls)
	actorRepo.AssertExpectations(t)
}

func TestRecoveryFederationService_SendRecoveryRequest_CreatesSystemActor(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("GetActor", mock.Anything, "system").Return(nil, errors.New("missing"))
	actorRepo.On("CreateActor", mock.Anything, mock.Anything, "").Return(nil)

	delivery := &fakeDelivery{}
	svc := &RecoveryFederationService{
		repos:      repos,
		fedService: delivery,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}

	req := &storage.SocialRecoveryRequest{ID: "req-1", Username: "alice", ExpiresAt: time.Now().Add(1 * time.Hour)}
	require.NoError(t, svc.SendRecoveryRequest(context.Background(), req, "https://remote.example/users/bob"))
	require.Equal(t, 1, delivery.calls)
	actorRepo.AssertExpectations(t)
}

func TestRecoveryFederationService_HandleTrusteeConfirmation_ParsesAndDispatches(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	confirmer := &fakeConfirmer{}
	svc := &RecoveryFederationService{
		repos:          repos,
		fedService:     &fakeDelivery{},
		logger:         zap.NewNop(),
		domain:         "example.com",
		config:         &config.Config{SystemActorPublicKey: "pub"},
		socialRecovery: confirmer,
	}

	err := svc.HandleTrusteeConfirmation(context.Background(), &activitypub.Activity{
		BaseObject: activitypub.BaseObject{Type: "Create"},
		Actor:      "https://remote.example/users/bob",
		Object: map[string]any{
			"lesser:recoveryConfirmation": map[string]any{
				"requestId": "req-1",
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, 1, confirmer.calls)

	require.ErrorIs(t, svc.HandleTrusteeConfirmation(context.Background(), &activitypub.Activity{Object: "bad"}), ErrInvalidActivityObject)
	require.ErrorIs(t, svc.HandleTrusteeConfirmation(context.Background(), &activitypub.Activity{Object: map[string]any{}}), ErrNotRecoveryConfirmationActivity)
	require.ErrorIs(t, svc.HandleTrusteeConfirmation(context.Background(), &activitypub.Activity{Object: map[string]any{"lesser:recoveryConfirmation": map[string]any{}}}), ErrMissingRequestID)
}

func TestRecoveryFederationService_SendRecoveryApprovalNotification_StoresAndOptionallyDelivers(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{
		BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"},
		Inbox:      "https://remote.example/inbox",
	}, nil)
	actorRepo.On("GetActor", mock.Anything, "system").Return(nil, errors.New("missing"))
	notifRepo.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)

	delivery := &fakeDelivery{err: errors.New("deliver failed")}
	svc := &RecoveryFederationService{
		repos:      repos,
		fedService: delivery,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}

	require.NoError(t, svc.SendRecoveryApprovalNotification(context.Background(), "alice", "tok"))
	require.Equal(t, 1, delivery.calls)

	// No inbox -> no deliver call.
	delivery.calls = 0
	actorRepo2 := testmocks.NewMockActorRepository()
	notifRepo2 := testmocks.NewMockNotificationRepository()
	repos2 := recoveryFedRepos{actor: actorRepo2, notification: notifRepo2}
	actorRepo2.On("GetActor", mock.Anything, "alice").Return(&activitypub.Actor{BaseObject: activitypub.BaseObject{ID: "https://example.com/users/alice"}}, nil)
	notifRepo2.On("CreateNotification", mock.Anything, mock.Anything).Return(nil)
	svc2 := &RecoveryFederationService{
		repos:      repos2,
		fedService: delivery,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{SystemActorPublicKey: "pub"},
	}
	require.NoError(t, svc2.SendRecoveryApprovalNotification(context.Background(), "alice", "tok"))
	require.Equal(t, 0, delivery.calls)
}

func TestRecoveryFederationService_PublicKeyHelpers(t *testing.T) {
	t.Parallel()

	svc := &RecoveryFederationService{
		logger: zap.NewNop(),
		domain: "example.com",
		config: &config.Config{SystemActorPublicKey: "static"},
	}
	require.Equal(t, "static", svc.getSystemPublicKey())

	// derivePublicKeyFromPrivate supports PKCS8 RSA, rejects bad data and non-RSA keys.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rsaDer, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)
	rsaPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: rsaDer}))
	pub, err := svc.derivePublicKeyFromPrivate(rsaPEM)
	require.NoError(t, err)
	require.Contains(t, pub, "PUBLIC KEY")

	_, err = svc.derivePublicKeyFromPrivate("not-pem")
	require.ErrorIs(t, err, ErrFailedToDecodePEM)

	ecdsaKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	ecdsaDer, err := x509.MarshalPKCS8PrivateKey(ecdsaKey)
	require.NoError(t, err)
	ecdsaPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: ecdsaDer}))
	_, err = svc.derivePublicKeyFromPrivate(ecdsaPEM)
	require.ErrorIs(t, err, ErrUnsupportedPrivateKeyType)
}

func TestRecoveryFederationService_SystemActorKeyRotation(t *testing.T) {
	t.Parallel()

	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("CreateActor", mock.Anything, mock.Anything, "").Return(nil)

	secrets := &fakeSecretsManager{publicKeyOnRotate: "pub", privateKeyOnRotate: "priv"}
	svc := &RecoveryFederationService{
		repos:          repos,
		fedService:     &fakeDelivery{},
		logger:         zap.NewNop(),
		domain:         "example.com",
		secretsManager: secrets,
		config:         &config.Config{},
	}

	require.NoError(t, svc.RotateSystemActorKey(context.Background()))
	actorRepo.AssertExpectations(t)
}
