package handlers

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestGenerateVAPIDKeyPair_round28_rawScalarPrivateKey(t *testing.T) {
	t.Parallel()

	publicKey, privateKey, err := generateVAPIDKeyPair()
	require.NoError(t, err)
	require.NotEmpty(t, publicKey)
	require.NotEmpty(t, privateKey)
	require.False(t, strings.Contains(privateKey, "BEGIN EC PRIVATE KEY"))

	publicKeyBytes, err := base64.RawURLEncoding.DecodeString(publicKey)
	require.NoError(t, err)
	require.Len(t, publicKeyBytes, 65)
	require.Equal(t, byte(4), publicKeyBytes[0])

	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(privateKey)
	require.NoError(t, err)
	require.Len(t, privateKeyBytes, 32)
}

func TestValidateVAPIDKeysForProduction_round28_more_coverage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	logger := zap.NewNop()

	t.Run("push_repo_nil_non_prod", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return((*repositories.PushSubscriptionRepository)(nil)).Maybe()

		err := ValidateVAPIDKeysForProduction(ctx, &config.Config{Stage: "development"}, repos, logger)
		require.NoError(t, err)
	})

	t.Run("push_repo_nil_prod_errors", func(t *testing.T) {
		repos := &MockRepositoryStorage{}
		repos.On("PushSubscription").Return((*repositories.PushSubscriptionRepository)(nil)).Maybe()

		err := ValidateVAPIDKeysForProduction(ctx, &config.Config{Stage: EnvProduction}, repos, logger)
		require.Error(t, err)
	})

	t.Run("auto_generate_and_store_when_missing", func(t *testing.T) {
		handler, repos, _ := round11NewHandler(t)
		handler.cfg.VAPIDPublicKey = ""

		err := ValidateVAPIDKeysForProduction(ctx, handler.cfg, repos, logger)
		require.NoError(t, err)

		keys, err := repos.PushSubscription().GetVAPIDKeys(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, keys.PublicKey)
		require.NotEmpty(t, keys.PrivateKey)

		handler.cfg.VAPIDPublicKey = "present"
		err = ValidateVAPIDKeysForProduction(ctx, handler.cfg, repos, logger)
		require.NoError(t, err)
	})
}
