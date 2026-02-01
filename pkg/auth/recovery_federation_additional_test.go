package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/config"
	testmocks "github.com/equaltoai/lesser/pkg/testing/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRecoveryFederationService_GetSystemPublicKey_VariousSources(t *testing.T) {
	t.Parallel()

	// Cached in actor repository.
	actorRepo := testmocks.NewMockActorRepository()
	notifRepo := testmocks.NewMockNotificationRepository()
	repos := recoveryFedRepos{actor: actorRepo, notification: notifRepo}

	actorRepo.On("GetActor", mock.Anything, "system").Return(&activitypub.Actor{
		PublicKey: &activitypub.PublicKey{PublicKeyPem: "cached"},
	}, nil)

	svc := &RecoveryFederationService{
		repos:      repos,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{},
		fedService: &fakeDelivery{},
	}
	require.Equal(t, "cached", svc.getSystemPublicKey())

	// No secrets manager fallback.
	actorRepo2 := testmocks.NewMockActorRepository()
	repos2 := recoveryFedRepos{actor: actorRepo2, notification: notifRepo}
	actorRepo2.On("GetActor", mock.Anything, "system").Return(nil, errors.New("missing"))

	svc2 := &RecoveryFederationService{
		repos:      repos2,
		logger:     zap.NewNop(),
		domain:     "example.com",
		config:     &config.Config{},
		fedService: &fakeDelivery{},
	}
	require.Empty(t, svc2.getSystemPublicKey())

	// Secrets manager generates a key when retrieval fails.
	actorRepo3 := testmocks.NewMockActorRepository()
	repos3 := recoveryFedRepos{actor: actorRepo3, notification: notifRepo}
	actorRepo3.On("GetActor", mock.Anything, "system").Return(nil, errors.New("missing"))
	actorRepo3.On("CreateActor", mock.Anything, mock.Anything, "").Return(nil)

	svc3 := &RecoveryFederationService{
		repos:          repos3,
		logger:         zap.NewNop(),
		domain:         "example.com",
		config:         &config.Config{},
		fedService:     &fakeDelivery{},
		secretsManager: &fakeSecretsManager{errGet: errors.New("not found")},
	}
	require.Equal(t, "pub", svc3.getSystemPublicKey())

	// Secrets manager derives public key from existing private key.
	actorRepo4 := testmocks.NewMockActorRepository()
	repos4 := recoveryFedRepos{actor: actorRepo4, notification: notifRepo}
	actorRepo4.On("GetActor", mock.Anything, "system").Return(nil, errors.New("missing"))
	actorRepo4.On("CreateActor", mock.Anything, mock.Anything, "").Return(nil)

	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(rsaKey)
	require.NoError(t, err)
	privateKeyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))

	svc4 := &RecoveryFederationService{
		repos:          repos4,
		logger:         zap.NewNop(),
		domain:         "example.com",
		config:         &config.Config{},
		fedService:     &fakeDelivery{},
		secretsManager: &fakeSecretsManager{privateKey: privateKeyPEM},
	}
	require.Contains(t, svc4.getSystemPublicKey(), "PUBLIC KEY")
}

func TestRecoveryFederationService_GetSystemActorPrivateKey(t *testing.T) {
	t.Parallel()

	svc := &RecoveryFederationService{logger: zap.NewNop(), domain: "example.com"}
	_, err := svc.GetSystemActorPrivateKey(context.Background())
	require.ErrorIs(t, err, ErrSecretsManagerNotAvailable)

	svc.secretsManager = &fakeSecretsManager{errGet: errors.New("boom")}
	_, err = svc.GetSystemActorPrivateKey(context.Background())
	require.ErrorIs(t, err, ErrSystemActorKeyRetrievalFailed)

	svc.secretsManager = &fakeSecretsManager{privateKey: "priv"}
	got, err := svc.GetSystemActorPrivateKey(context.Background())
	require.NoError(t, err)
	require.Equal(t, "priv", got)
}

func TestRecoveryActivity_MarshalJSON_AddsCustomFields(t *testing.T) {
	t.Parallel()

	act := RecoveryActivity{
		Activity: activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create", ID: "id"},
			Actor:      "actor",
			Object:     map[string]any{"type": "Note"},
		},
		RecoveryType: "social",
		RecoveryData: map[string]any{"k": "v"},
	}

	data, err := act.MarshalJSON()
	require.NoError(t, err)

	var m map[string]any
	require.NoError(t, json.Unmarshal(data, &m))
	require.Equal(t, "social", m["lesser:recoveryType"])
	require.Equal(t, map[string]any{"k": "v"}, m["lesser:recoveryData"])

	// Marshal error when base activity isn't JSON encodable.
	bad := RecoveryActivity{
		Activity: activitypub.Activity{
			BaseObject: activitypub.BaseObject{Type: "Create", ID: "id"},
			Object:     make(chan int),
		},
	}
	_, err = bad.MarshalJSON()
	require.Error(t, err)
}
