package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// AgentAccessLeaseRepository persists agent access lease challenges and leases.
type AgentAccessLeaseRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAgentAccessLeaseRepository creates a new AgentAccessLeaseRepository.
func NewAgentAccessLeaseRepository(db core.DB, tableName string, logger *zap.Logger) *AgentAccessLeaseRepository {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &AgentAccessLeaseRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// CreateChallenge stores a new access lease challenge after preparing its keys and defaults.
func (r *AgentAccessLeaseRepository) CreateChallenge(ctx context.Context, challenge *models.AgentAccessLeaseChallenge) error {
	if r == nil {
		return storage.ErrDatabaseConnectionFailed
	}

	return createPreparedModel(
		ctx,
		r.db,
		r.logger,
		challenge,
		"failed to prepare agent access lease challenge",
		"failed to create agent access lease challenge",
		func(challenge *models.AgentAccessLeaseChallenge) []zap.Field {
			if challenge == nil {
				return nil
			}
			return []zap.Field{
				zap.String("challenge_id", strings.TrimSpace(challenge.ID)),
				zap.String("lease_id", strings.TrimSpace(challenge.LeaseID)),
			}
		},
	)
}

// GetChallenge loads an access lease challenge by ID.
func (r *AgentAccessLeaseRepository) GetChallenge(ctx context.Context, challengeID string) (*models.AgentAccessLeaseChallenge, error) {
	if r == nil || r.db == nil {
		return nil, storage.ErrDatabaseConnectionFailed
	}

	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return nil, storage.ErrInvalidInput
	}

	challenge := &models.AgentAccessLeaseChallenge{}
	if err := r.db.WithContext(ctx).
		Model(&models.AgentAccessLeaseChallenge{}).
		Where("PK", "=", fmt.Sprintf("AGENT_ACCESS_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(challenge); err != nil {
		r.logger.Warn("failed to load agent access lease challenge",
			zap.String("challenge_id", challengeID),
			zap.Error(err))
		return nil, err
	}

	return challenge, nil
}

// MarkChallengeUsed marks an access lease challenge as used when it is still valid.
func (r *AgentAccessLeaseRepository) MarkChallengeUsed(ctx context.Context, challengeID string, now time.Time) error {
	if r == nil {
		return storage.ErrDatabaseConnectionFailed
	}
	return markChallengeUsed(
		ctx,
		r.db,
		r.logger,
		challengeID,
		now,
		&models.AgentAccessLeaseChallenge{
			PK: fmt.Sprintf("AGENT_ACCESS_CHALLENGE#%s", strings.TrimSpace(challengeID)),
			SK: "CHALLENGE",
		},
		"failed to mark agent access lease challenge used",
	)
}

// CreateLease stores a new access lease after preparing its keys and defaults.
func (r *AgentAccessLeaseRepository) CreateLease(ctx context.Context, lease *models.AgentAccessLease) error {
	if r == nil {
		return storage.ErrDatabaseConnectionFailed
	}

	return createPreparedModel(
		ctx,
		r.db,
		r.logger,
		lease,
		"failed to prepare agent access lease",
		"failed to create agent access lease",
		func(lease *models.AgentAccessLease) []zap.Field {
			if lease == nil {
				return nil
			}
			return []zap.Field{
				zap.String("lease_id", strings.TrimSpace(lease.ID)),
				zap.String("username", strings.TrimSpace(lease.Username)),
			}
		},
	)
}

func (r *AgentAccessLeaseRepository) updateLease(
	ctx context.Context,
	lease *models.AgentAccessLease,
	warnMessage string,
	build func(core.UpdateBuilder) core.UpdateBuilder,
) error {
	if r == nil || r.db == nil {
		return storage.ErrDatabaseConnectionFailed
	}
	if lease == nil {
		return storage.ErrInvalidInput
	}
	if build == nil {
		return storage.ErrInvalidInput
	}

	update, err := keyedUpdateBuilder(ctx, r.db, lease)
	if err != nil {
		return err
	}

	if err := build(update).Execute(); err != nil {
		r.logger.Warn(warnMessage,
			zap.String("lease_id", strings.TrimSpace(lease.ID)),
			zap.String("username", strings.TrimSpace(lease.Username)),
			zap.String("pk", strings.TrimSpace(lease.GetPK())),
			zap.String("sk", strings.TrimSpace(lease.GetSK())),
			zap.Error(err))
		return err
	}

	return nil
}

// RevokeLease updates a lease to the revoked state.
func (r *AgentAccessLeaseRepository) RevokeLease(
	ctx context.Context,
	lease *models.AgentAccessLease,
	revokedBy string,
	reason string,
	now time.Time,
) error {
	revokedBy = strings.TrimSpace(revokedBy)
	reason = strings.TrimSpace(reason)
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return r.updateLease(ctx, lease, "failed to revoke agent access lease", func(update core.UpdateBuilder) core.UpdateBuilder {
		return update.
			Set("TTL", now.UTC().Unix()).
			Set("Status", "revoked").
			Set("UpdatedAt", now).
			Set("RevokedAt", now).
			Set("RevokedBy", revokedBy).
			Set("RevokedReason", reason)
	})
}

// AuthorizeSessionKey stores the active session key metadata for a lease.
func (r *AgentAccessLeaseRepository) AuthorizeSessionKey(
	ctx context.Context,
	lease *models.AgentAccessLease,
	sessionPublicKey string,
	sessionKeyType string,
	now time.Time,
) error {
	sessionPublicKey = strings.TrimSpace(sessionPublicKey)
	sessionKeyType = strings.TrimSpace(sessionKeyType)
	if sessionPublicKey == "" || sessionKeyType == "" {
		return storage.ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return r.updateLease(ctx, lease, "failed to authorize agent access lease session key", func(update core.UpdateBuilder) core.UpdateBuilder {
		return update.
			Set("SessionPublicKey", sessionPublicKey).
			Set("SessionKeyType", sessionKeyType).
			Set("SessionKeyCreatedAt", now).
			Set("UpdatedAt", now)
	})
}

// RecordLeaseUse refreshes lease activity timestamps after successful token exchange.
func (r *AgentAccessLeaseRepository) RecordLeaseUse(
	ctx context.Context,
	lease *models.AgentAccessLease,
	idleExpiresAt time.Time,
	now time.Time,
	sessionKeyUsed bool,
) error {
	if idleExpiresAt.IsZero() {
		return storage.ErrInvalidInput
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return r.updateLease(ctx, lease, "failed to update agent access lease activity", func(update core.UpdateBuilder) core.UpdateBuilder {
		update = update.
			Set("LastUsedAt", now).
			Set("IdleExpiresAt", idleExpiresAt).
			Set("UpdatedAt", now)
		if sessionKeyUsed {
			update = update.Set("SessionKeyLastUsedAt", now)
		}
		return update
	})
}

// GetLease loads a single agent access lease.
func (r *AgentAccessLeaseRepository) GetLease(ctx context.Context, username, leaseID string) (*models.AgentAccessLease, error) {
	if r == nil || r.db == nil {
		return nil, storage.ErrDatabaseConnectionFailed
	}

	username = strings.TrimSpace(username)
	leaseID = strings.TrimSpace(leaseID)
	if username == "" || leaseID == "" {
		return nil, storage.ErrInvalidInput
	}

	lease := &models.AgentAccessLease{}
	if err := r.db.WithContext(ctx).
		Model(&models.AgentAccessLease{}).
		Where("PK", "=", fmt.Sprintf("AGENT_ACCESS_LEASE#%s", username)).
		Where("SK", "=", fmt.Sprintf("LEASE#%s", leaseID)).
		First(lease); err != nil {
		r.logger.Warn("failed to load agent access lease",
			zap.String("username", username),
			zap.String("lease_id", leaseID),
			zap.Error(err))
		return nil, err
	}

	return lease, nil
}

// ListLeases loads all access leases for an agent username.
func (r *AgentAccessLeaseRepository) ListLeases(ctx context.Context, username string) ([]models.AgentAccessLease, error) {
	if r == nil || r.db == nil {
		return nil, storage.ErrDatabaseConnectionFailed
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, storage.ErrInvalidInput
	}

	// The whole keyed AGENT_ACCESS_LEASE#<username> partition must be read to
	// return every lease, so the read is a bounded page walk (wave #1469):
	// Limit(500)/page, 100-page cap, fail-closed on exhaustion.
	var leases []models.AgentAccessLease
	err := walkKeyedPages(
		r.db.WithContext(ctx).
			Model(&models.AgentAccessLease{}).
			Where("PK", "=", fmt.Sprintf("AGENT_ACCESS_LEASE#%s", username)).
			Where("SK", "BEGINS_WITH", "LEASE#"),
		500, 100,
		func(page []models.AgentAccessLease) (bool, error) {
			leases = append(leases, page...)
			return false, nil
		},
	)
	if err != nil {
		r.logger.Warn("failed to list agent access leases",
			zap.String("username", username),
			zap.Error(err))
		return nil, err
	}

	return leases, nil
}
