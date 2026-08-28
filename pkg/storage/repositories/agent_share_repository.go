package repositories

import (
	"context"
	"fmt"
	"strings"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	tabletheoryerrors "github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// AgentShareRepository persists per-agent share-list grants in the main table.
type AgentShareRepository struct {
	db     core.DB
	logger *zap.Logger
}

// NewAgentShareRepository creates an agent share repository.
func NewAgentShareRepository(db core.DB, _ string, logger *zap.Logger) *AgentShareRepository {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &AgentShareRepository{db: db, logger: logger}
}

// CreateAgentShareGrant creates a first-time grant without overwriting an existing row.
func (r *AgentShareRepository) CreateAgentShareGrant(ctx context.Context, grant *models.AgentShareGrant) error {
	if grant == nil {
		return fmt.Errorf("agent share grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}
	err := r.db.WithContext(ctx).Model(grant).IfNotExists().Create()
	if tabletheoryerrors.IsConditionFailed(err) {
		return tabletheoryerrors.ErrConditionFailed
	}
	return err
}

// RegrantAgentShareGrant refreshes attribution and restores the sparse active index.
func (r *AgentShareRepository) RegrantAgentShareGrant(ctx context.Context, grant *models.AgentShareGrant) error {
	if grant == nil || grant.RevokedAt != nil || strings.TrimSpace(grant.RevokedBy) != "" {
		return fmt.Errorf("active agent share grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("GrantedBy", grant.GrantedBy)
	builder.Set("GrantedAt", grant.GrantedAt)
	builder.Set("GSI2PK", grant.GSI2PK)
	builder.Set("GSI2SK", grant.GSI2SK)
	builder.Remove("RevokedAt")
	builder.Remove("RevokedBy")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		return err
	}
	grant.Version = nextVersion
	return nil
}

// RevokeAgentShareGrant persists revocation and immediately removes discovery keys.
func (r *AgentShareRepository) RevokeAgentShareGrant(ctx context.Context, grant *models.AgentShareGrant) error {
	if grant == nil || grant.RevokedAt == nil || strings.TrimSpace(grant.RevokedBy) == "" {
		return fmt.Errorf("revoked agent share grant is required")
	}
	if err := grant.UpdateKeys(); err != nil {
		return err
	}

	nextVersion := grant.Version + 1
	if nextVersion <= 0 {
		nextVersion = 1
	}
	builder := r.db.WithContext(ctx).
		Model(grant).
		Where("PK", "=", grant.PK).
		Where("SK", "=", grant.SK).
		UpdateBuilder()
	builder.Set("RevokedAt", grant.RevokedAt.UTC())
	builder.Set("RevokedBy", grant.RevokedBy)
	builder.Remove("GSI2PK")
	builder.Remove("GSI2SK")
	builder.ConditionVersion(int64(grant.Version))
	builder.Set("Version", nextVersion)
	if err := builder.Execute(); err != nil {
		return err
	}
	grant.Version = nextVersion
	grant.GSI2PK = ""
	grant.GSI2SK = ""
	return nil
}

// GetAgentShareGrant loads one grant through its direct-read primary key.
func (r *AgentShareRepository) GetAgentShareGrant(ctx context.Context, agentUsername, granteeUsername string) (*models.AgentShareGrant, error) {
	var grant models.AgentShareGrant
	err := r.db.WithContext(ctx).
		Model(&models.AgentShareGrant{}).
		Where("PK", "=", models.AgentShareGrantPK(agentUsername)).
		Where("SK", "=", models.AgentShareGrantSK(granteeUsername)).
		First(&grant)
	if err != nil {
		if tabletheoryerrors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	return &grant, nil
}

// IsActiveAgentShareGrant performs the uncached per-request membership check. This is
// the authorization read: it uses a strongly consistent direct GetItem so revocation is
// observed immediately, and it never consults the discovery GSI.
func (r *AgentShareRepository) IsActiveAgentShareGrant(ctx context.Context, agentUsername, granteeUsername string) (bool, error) {
	var grant models.AgentShareGrant
	err := r.db.WithContext(ctx).
		Model(&models.AgentShareGrant{}).
		Where("PK", "=", models.AgentShareGrantPK(agentUsername)).
		Where("SK", "=", models.AgentShareGrantSK(granteeUsername)).
		ConsistentRead().
		First(&grant)
	if err != nil {
		if tabletheoryerrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return grant.RevokedAt == nil, nil
}

// ListAgentShareGrantsByAgent returns the complete owner-view grant history.
func (r *AgentShareRepository) ListAgentShareGrantsByAgent(ctx context.Context, agentUsername string) ([]*models.AgentShareGrant, error) {
	// The whole keyed owner-view partition must be read to return the complete
	// grant history, so the read is a bounded page walk (wave #1469):
	// Limit(500)/page, 100-page cap, fail-closed on exhaustion. The OrderBy ASC
	// is preserved across pages via cursors.
	var rows []models.AgentShareGrant
	err := walkKeyedPages(
		r.db.WithContext(ctx).
			Model(&models.AgentShareGrant{}).
			Where("PK", "=", models.AgentShareGrantPK(agentUsername)).
			Where("SK", "begins_with", models.AgentShareGrantSKPrefix).
			OrderBy("SK", "ASC"),
		500, 100,
		func(page []models.AgentShareGrant) (bool, error) {
			rows = append(rows, page...)
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return agentShareGrantPointers(rows), nil
}

// ListActiveAgentShareGrantsByGrantee returns agents currently shared with one account.
func (r *AgentShareRepository) ListActiveAgentShareGrantsByGrantee(ctx context.Context, granteeUsername string) ([]*models.AgentShareGrant, error) {
	// The whole keyed gsi2 grantee partition must be read to return every active
	// share, so the read is a bounded page walk (wave #1469): Limit(500)/page,
	// 100-page cap, fail-closed on exhaustion. The RevokedAt filter stays on the
	// chain and applies per page; OrderBy ASC is preserved via cursors.
	var rows []models.AgentShareGrant
	err := walkKeyedPages(
		r.db.WithContext(ctx).
			Model(&models.AgentShareGrant{}).
			Index(models.IndexGSI2).
			Where("gsi2PK", "=", models.AgentShareGrantGranteeGSI2PK(granteeUsername)).
			Filter("RevokedAt", "attribute_not_exists", nil).
			OrderBy("gsi2SK", "ASC"),
		500, 100,
		func(page []models.AgentShareGrant) (bool, error) {
			rows = append(rows, page...)
			return false, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return agentShareGrantPointers(rows), nil
}

func agentShareGrantPointers(rows []models.AgentShareGrant) []*models.AgentShareGrant {
	out := make([]*models.AgentShareGrant, len(rows))
	for i := range rows {
		out[i] = &rows[i]
	}
	return out
}
