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

// AgentKeyChallengeRepository persists self-sovereign agent key challenges.
type AgentKeyChallengeRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewAgentKeyChallengeRepository creates a new AgentKeyChallengeRepository.
func NewAgentKeyChallengeRepository(db core.DB, tableName string, logger *zap.Logger) *AgentKeyChallengeRepository {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &AgentKeyChallengeRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// Create stores a new key challenge after preparing its keys and defaults.
func (r *AgentKeyChallengeRepository) Create(ctx context.Context, challenge *models.AgentKeyChallenge) error {
	if r == nil {
		return storage.ErrDatabaseConnectionFailed
	}

	return createPreparedModel(
		ctx,
		r.db,
		r.logger,
		challenge,
		"failed to prepare agent key challenge",
		"failed to create agent key challenge",
		func(challenge *models.AgentKeyChallenge) []zap.Field {
			if challenge == nil {
				return nil
			}
			return []zap.Field{
				zap.String("challenge_id", strings.TrimSpace(challenge.ID)),
				zap.String("username", strings.TrimSpace(challenge.Username)),
			}
		},
	)
}

// Get loads a key challenge by ID.
func (r *AgentKeyChallengeRepository) Get(ctx context.Context, challengeID string) (*models.AgentKeyChallenge, error) {
	if r == nil || r.db == nil {
		return nil, storage.ErrDatabaseConnectionFailed
	}

	challengeID = strings.TrimSpace(challengeID)
	if challengeID == "" {
		return nil, storage.ErrInvalidInput
	}

	challenge := &models.AgentKeyChallenge{}
	if err := r.db.WithContext(ctx).
		Model(&models.AgentKeyChallenge{}).
		Where("PK", "=", fmt.Sprintf("AGENT_KEY_CHALLENGE#%s", challengeID)).
		Where("SK", "=", "CHALLENGE").
		First(challenge); err != nil {
		r.logger.Warn("failed to load agent key challenge",
			zap.String("challenge_id", challengeID),
			zap.Error(err))
		return nil, err
	}

	return challenge, nil
}

// MarkUsed marks a key challenge as used when it is still valid.
func (r *AgentKeyChallengeRepository) MarkUsed(ctx context.Context, challengeID string, now time.Time) error {
	if r == nil {
		return storage.ErrDatabaseConnectionFailed
	}
	return markChallengeUsed(
		ctx,
		r.db,
		r.logger,
		challengeID,
		now,
		&models.AgentKeyChallenge{
			PK: fmt.Sprintf("AGENT_KEY_CHALLENGE#%s", strings.TrimSpace(challengeID)),
			SK: "CHALLENGE",
		},
		"failed to mark agent key challenge used",
	)
}
