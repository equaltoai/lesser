package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
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

	var leases []models.AgentAccessLease
	if err := r.db.WithContext(ctx).
		Model(&models.AgentAccessLease{}).
		Where("PK", "=", fmt.Sprintf("AGENT_ACCESS_LEASE#%s", username)).
		Where("SK", "BEGINS_WITH", "LEASE#").
		All(&leases); err != nil {
		r.logger.Warn("failed to list agent access leases",
			zap.String("username", username),
			zap.Error(err))
		return nil, err
	}

	return leases, nil
}
