package handlers

import (
	"net/http"
	"testing"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
)

func TestAdminLift_Round10Coverage_ErrorBranches(t *testing.T) {
	cfg := round10TestConfig()
	logger := round10TestLogger(t)
	headers := map[string]string{"Authorization": "Bearer " + round10SignAccessToken(t, cfg.JWTSecret, "admin")}

	t.Run("accounts list returns 500 on storage error", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1},
			},
			allErrorOnce: auth.ErrInvalidToken, // any non-nil error
		}
		harness := round10NewDynamoHarness(t, state)

		userRepo := repositories.NewUserRepository(harness.db, cfg.DynamoTableName, logger)
		accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)

		repos := &MockRepositoryStorage{}
		repos.On("User").Return(userRepo).Maybe()
		repos.On("Account").Return(accountRepo).Maybe()
		repos.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()
		repos.On("GetDB").Return(harness.db).Maybe()

		h := &Handler{cfg: cfg, repos: repos, logger: logger}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/accounts", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetAccountsLift(ctx))
	})

	t.Run("reports list returns 500 on storage error", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1},
			},
			allErrorOnce: auth.ErrInvalidToken,
		}
		harness := round10NewDynamoHarness(t, state)

		accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
		moderationRepo := repositories.NewModerationRepository(harness.db, cfg.DynamoTableName, logger)

		repos := &MockRepositoryStorage{}
		repos.On("Account").Return(accountRepo).Maybe()
		repos.On("Moderation").Return(moderationRepo).Maybe()
		repos.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()
		repos.On("GetDB").Return(harness.db).Maybe()

		h := &Handler{cfg: cfg, repos: repos, logger: logger}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/reports", headers, map[string]string{"limit": "1"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetReportsLift(ctx))
	})

	t.Run("statuses list returns 500 on query error", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1},
			},
			allErrorOnce: auth.ErrInvalidToken,
		}
		harness := round10NewDynamoHarness(t, state)

		accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
		statusRepo := repositories.NewStatusRepository(harness.db, cfg.DynamoTableName, logger, nil)

		repos := &MockRepositoryStorage{}
		repos.On("Account").Return(accountRepo).Maybe()
		repos.On("Status").Return(statusRepo).Maybe()
		repos.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()
		repos.On("GetDB").Return(harness.db).Maybe()

		h := &Handler{cfg: cfg, repos: repos, logger: logger}

		ctx, err := round10NewLiftContext(http.MethodGet, "/api/v1/admin/statuses", headers, map[string]string{"limit": "1", "local": "true"}, nil)
		require.NoError(t, err)
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminGetStatusesLift(ctx))
	})

	t.Run("override moderation event returns 500 on read error", func(t *testing.T) {
		state := &round10QueryState{
			usersByUsername: map[string]storagemodels.User{
				"admin": {PK: "USER#admin", SK: storagemodels.SKMetadata, Username: "admin", Role: "admin", Approved: true, Version: 1},
			},
			firstErrorGSI3PK: map[string]error{
				"EVENTID#evt1": auth.ErrInvalidToken,
			},
		}
		harness := round10NewDynamoHarness(t, state)

		accountRepo := repositories.NewAccountRepository(harness.db, cfg.DynamoTableName, cfg.Domain, logger)
		moderationRepo := repositories.NewModerationRepository(harness.db, cfg.DynamoTableName, logger)

		repos := &MockRepositoryStorage{}
		repos.On("Account").Return(accountRepo).Maybe()
		repos.On("Moderation").Return(moderationRepo).Maybe()
		repos.On("Audit").Return((*repositories.AuditRepository)(nil)).Maybe()
		repos.On("GetDB").Return(harness.db).Maybe()

		h := &Handler{cfg: cfg, repos: repos, logger: logger}

		body := apimodels.AdminModerationEventOverrideRequest{Decision: "reject", Reason: "bad"}
		ctx, err := round10NewLiftContext(http.MethodPost, "/api/v1/admin/moderation/events/evt1/override", headers, nil, body)
		require.NoError(t, err)
		ctx.Params["id"] = "evt1"
		requireStatus(t, http.StatusInternalServerError)(h.HandleAdminOverrideModerationEventLift(ctx))
	})
}
