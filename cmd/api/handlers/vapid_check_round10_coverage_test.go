package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestValidateVAPIDKeysForProduction_Round10Coverage(t *testing.T) {
	logger := round10TestLogger(t)

	for _, stage := range []string{"dev", "development", "staging", "test", ""} {
		t.Run("non-production "+stage+" warns and skips bootstrap without durable secret config", func(t *testing.T) {
			logCore, observed := observer.New(zap.WarnLevel)
			cfg := &config.Config{Stage: stage, Domain: "test.local"}
			repos := &MockRepositoryStorage{}

			require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, zap.New(logCore)))
			require.Equal(t, 1, observed.FilterMessage(
				"VAPID_SECRET_ARN is not configured; skipping non-production VAPID bootstrap because generated keys could not be persisted durably",
			).Len())
			repos.AssertNotCalled(t, "PushSubscription")
		})
	}

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

	for _, stage := range []string{"live", "production", "prod"} {
		t.Run(stage+" fails closed when VAPID secret ARN is unset", func(t *testing.T) {
			cfg := &config.Config{Stage: stage, Domain: "prod.example.com"}
			repos := &MockRepositoryStorage{}

			err := ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger)
			require.ErrorContains(t, err, "VAPID_SECRET_ARN is required in production")
			repos.AssertNotCalled(t, "PushSubscription")
		})
	}
}
