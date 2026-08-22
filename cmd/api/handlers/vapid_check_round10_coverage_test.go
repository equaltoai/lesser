package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

func TestValidateVAPIDKeysForProduction_Round10Coverage(t *testing.T) {
	logger := round10TestLogger(t)

	t.Run("non-production skips bootstrap without durable secret config", func(t *testing.T) {
		cfg := &config.Config{Stage: "dev", Domain: "test.local"}
		state := &round10QueryState{forceVapidNotFound: true}
		h := round10NewDynamoHarness(t, state)
		pushRepo := repositories.NewPushSubscriptionRepository(h.db, "test-table", logger, nil, nil, "", "mailto:test@example.com")

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()
		repos.On("Audit").Return(nil).Maybe()
		require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
		require.Nil(t, state.vapidKeys, "non-production startup must not claim an unpersisted key was bootstrapped")
	})

	t.Run("production validates and tolerates missing public key config", func(t *testing.T) {
		cfg := &config.Config{Stage: "production", VAPIDPublicKey: "", VAPIDSecretARN: "test-vapid-secret"}
		state := &round10QueryState{
			vapidKeys: &storage.VAPIDKeys{
				PublicKey:  "pub",
				PrivateKey: "priv",
				Subject:    "mailto:test@example.com",
				CreatedAt:  time.Now().Add(-24 * time.Hour),
				UpdatedAt:  time.Now(),
			},
		}
		h := round10NewDynamoHarness(t, state)
		pushRepo := repositories.NewPushSubscriptionRepository(
			h.db,
			"test-table",
			logger,
			nil,
			round10NewVAPIDSecretsClient(state),
			cfg.VAPIDSecretARN,
			"mailto:test@example.com",
		)

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()
		repos.On("Audit").Return(nil).Maybe()

		require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
	})

	t.Run("production auto-generates VAPID keys when missing", func(t *testing.T) {
		cfg := &config.Config{
			Stage:          "prod",
			VAPIDPublicKey: "public",
			VAPIDSecretARN: "test-vapid-secret",
			Domain:         "prod.example.com",
		}
		state := &round10QueryState{forceVapidNotFound: true}
		h := round10NewDynamoHarness(t, state)
		secretsClient := round10NewVAPIDSecretsClient(state)
		pushRepo := repositories.NewPushSubscriptionRepository(
			h.db,
			"test-table",
			logger,
			nil,
			secretsClient,
			cfg.VAPIDSecretARN,
			"mailto:test@example.com",
		)

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()
		repos.On("Audit").Return(nil).Maybe()

		require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
		keys, err := pushRepo.GetVAPIDKeys(context.Background())
		require.NoError(t, err)
		require.NotEmpty(t, keys.PrivateKey, "first boot must durably persist generated private key material")
	})

	t.Run("production fails closed when VAPID secret ARN is unset", func(t *testing.T) {
		cfg := &config.Config{Stage: "production", Domain: "prod.example.com"}
		state := &round10QueryState{forceVapidNotFound: true}
		h := round10NewDynamoHarness(t, state)
		pushRepo := repositories.NewPushSubscriptionRepository(h.db, "test-table", logger, nil, nil, "", "mailto:test@example.com")

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()
		repos.On("Audit").Return(nil).Maybe()

		err := ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger)
		require.ErrorContains(t, err, "VAPID_SECRET_ARN is required in production")
		require.Nil(t, state.vapidKeys, "startup must not generate or publish a replacement key without durable storage")
	})
}
