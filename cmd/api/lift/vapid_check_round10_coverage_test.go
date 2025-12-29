package lift

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

	t.Run("non-production skips validation", func(t *testing.T) {
		cfg := &config.Config{Stage: "dev"}
		repos := &MockRepositoryStorage{}
		require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
	})

	t.Run("production validates and tolerates missing public key config", func(t *testing.T) {
		cfg := &config.Config{Stage: "production", VAPIDPublicKey: ""}
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
		pushRepo := repositories.NewPushSubscriptionRepository(h.db, "test-table", logger, nil, nil, "", "mailto:test@example.com")

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()

		require.NoError(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
	})

	t.Run("production returns error when keys missing", func(t *testing.T) {
		cfg := &config.Config{Stage: "prod", VAPIDPublicKey: "public"}
		state := &round10QueryState{forceVapidNotFound: true}
		h := round10NewDynamoHarness(t, state)
		pushRepo := repositories.NewPushSubscriptionRepository(h.db, "test-table", logger, nil, nil, "", "mailto:test@example.com")

		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return(pushRepo).Maybe()

		require.Error(t, ValidateVAPIDKeysForProduction(context.Background(), cfg, repos, logger))
	})
}
