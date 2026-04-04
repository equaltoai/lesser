package handlers

import (
	"context"
	"testing"
	"time"

	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
)

func TestNotificationDeliveryKeys_DedupesConfiguredKeys(t *testing.T) {
	t.Helper()

	cfg := round11TestConfig()
	cfg.InstanceAPIKey = " instance-key "
	cfg.LesserHostInstanceKey = "instance-key"

	keys, err := (&Handler{cfg: cfg}).notificationDeliveryKeys(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"instance-key"}, keys)
}

func TestNotificationDeliveryRecipient_FallsBackToPrimaryAdmin(t *testing.T) {
	t.Helper()

	cfg := round11TestConfig()
	cfg.AdminUsername = ""

	handler, _, _ := round11NewHandler(t, cfg, &round10QueryState{
		instanceState: &storagemodels.InstanceState{
			PrimaryAdminUsername: " sim-admin ",
		},
	})

	require.Equal(t, "sim-admin", handler.notificationDeliveryRecipient(context.Background()))
}

func TestNotificationDeliveryResolvedUsername_UsesCanonicalAccountUsername(t *testing.T) {
	t.Helper()

	now := time.Date(2026, time.April, 4, 12, 0, 0, 0, time.UTC)
	handler, _, _ := round11NewHandler(t, round11TestConfig(), &round10QueryState{
		usersByUsername: map[string]storagemodels.User{
			"agent-bob": {
				Username:  "Agent-Bob",
				Role:      "user",
				Approved:  true,
				Version:   1,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	})

	require.Equal(t, "Agent-Bob", handler.notificationDeliveryResolvedUsername(context.Background(), " agent-bob "))
}
