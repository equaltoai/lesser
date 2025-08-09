package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// DomainBlockRepositoryV2 implements domain block operations using BaseRepository
// This demonstrates how to handle a repository with multiple model types
type DomainBlockRepositoryV2 struct {
	*BaseRepository[*models.UserDomainBlock]
	logger *zap.Logger
	db     core.DB // Keep db for other model types
}

// NewDomainBlockRepositoryV2 creates a new domain block repository using BaseRepository
func NewDomainBlockRepositoryV2(db core.DB, tableName string, logger *zap.Logger) *DomainBlockRepositoryV2 {
	return &DomainBlockRepositoryV2{
		BaseRepository: NewBaseRepository[*models.UserDomainBlock](db, tableName, logger),
		logger:         logger,
		db:             db, // Keep for instance/email blocks
	}
}

// ===== User Domain Blocks (uses BaseRepository) =====

// AddDomainBlock adds a domain to the user's block list
// BEFORE: 25+ lines with manual error handling
// AFTER: Focused on business logic
func (r *DomainBlockRepositoryV2) AddDomainBlock(ctx context.Context, username, domain string) error {
	block := &models.UserDomainBlock{
		Username:  username,
		Domain:    domain,
		CreatedAt: time.Now(),
	}

	// Use BaseRepository Create - saves ~20 lines of boilerplate
	err := r.Create(ctx, block)
	if err != nil {
		// Check if it's a duplicate (already exists)
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			// Already blocked, not an error
			return nil
		}
		r.logger.Error("failed to add domain block",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return fmt.Errorf("failed to add domain block: %w", err)
	}

	return nil
}

// RemoveDomainBlock removes a domain from the user's block list
// BEFORE: 20+ lines with error handling
// AFTER: Single BaseRepository Delete call
func (r *DomainBlockRepositoryV2) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	// Use BaseRepository Delete - saves ~15 lines of boilerplate
	err := r.Delete(ctx, fmt.Sprintf("USER#%s", username), fmt.Sprintf("DOMAIN_BLOCK#%s", domain))
	if err != nil {
		// Not found is not an error for removal
		if strings.Contains(err.Error(), "not found") {
			return nil
		}
		r.logger.Error("failed to remove domain block",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return fmt.Errorf("failed to remove domain block: %w", err)
	}

	return nil
}

// GetUserDomainBlocks retrieves all domains blocked by a user
// BEFORE: 30+ lines of query construction
// AFTER: Simple query with BaseRepository
func (r *DomainBlockRepositoryV2) GetUserDomainBlocks(ctx context.Context, username string) ([]*storage.DomainBlock, error) {
	// Use BaseRepository Query with SK prefix - saves ~20 lines
	blocks, err := r.QueryWithSKPrefix(ctx, fmt.Sprintf("USER#%s", username), "DOMAIN_BLOCK#", 1000)
	if err != nil {
		r.logger.Error("failed to get user domain blocks",
			zap.String("username", username),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get user domain blocks: %w", err)
	}

	// Convert to storage.DomainBlock slice
	result := make([]*storage.DomainBlock, len(blocks))
	for i, block := range blocks {
		result[i] = &storage.DomainBlock{
			Domain:    block.Domain,
			CreatedAt: block.CreatedAt,
		}
	}

	return result, nil
}

// IsDomainBlocked checks if a domain is blocked by a user
// BEFORE: 15+ lines with error handling
// AFTER: Simple existence check with BaseRepository
func (r *DomainBlockRepositoryV2) IsDomainBlocked(ctx context.Context, username, domain string) (bool, error) {
	// Use BaseRepository Exists - saves ~10 lines of boilerplate
	exists, err := r.Exists(ctx, fmt.Sprintf("USER#%s", username), fmt.Sprintf("DOMAIN_BLOCK#%s", domain))
	if err != nil {
		r.logger.Error("failed to check domain block",
			zap.String("username", username),
			zap.String("domain", domain),
			zap.Error(err))
		return false, fmt.Errorf("failed to check domain block: %w", err)
	}

	return exists, nil
}

// ===== Instance Domain Blocks (uses direct DynamORM) =====
// These use a different model, so they don't benefit from BaseRepository directly

// CreateInstanceDomainBlock creates an instance-level domain block
func (r *DomainBlockRepositoryV2) CreateInstanceDomainBlock(ctx context.Context, domain, severity, privateComment, publicComment, createdBy, createdByID string, rejectMedia, rejectReports, obfuscate bool) (*storage.InstanceDomainBlock, error) {
	block := &models.InstanceDomainBlock{
		ID:             uuid.New().String(),
		Domain:         domain,
		Severity:       severity,
		RejectMedia:    rejectMedia,
		RejectReports:  rejectReports,
		PrivateComment: privateComment,
		PublicComment:  publicComment,
		Obfuscate:      obfuscate,
		CreatedBy:      createdBy,
		CreatedByID:    createdByID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}
	block.UpdateKeys()

	// Direct DynamORM call (could create separate BaseRepository for this)
	err := r.db.WithContext(ctx).Model(block).Create()
	if err != nil {
		r.logger.Error("failed to create instance domain block",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create instance domain block: %w", err)
	}

	return r.modelToInstanceDomainBlock(block), nil
}

// GetInstanceDomainBlock retrieves an instance-level domain block
func (r *DomainBlockRepositoryV2) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	var block models.InstanceDomainBlock

	err := r.db.WithContext(ctx).Model(&block).
		Where("PK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		Where("SK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		First(&block)

	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, nil
		}
		r.logger.Error("failed to get instance domain block",
			zap.String("domain", domain),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get instance domain block: %w", err)
	}

	return r.modelToInstanceDomainBlock(&block), nil
}

// GetAllInstanceDomainBlocks retrieves all instance-level domain blocks
func (r *DomainBlockRepositoryV2) GetAllInstanceDomainBlocks(ctx context.Context) ([]*storage.InstanceDomainBlock, error) {
	var blocks []models.InstanceDomainBlock

	err := r.db.WithContext(ctx).Model(&models.InstanceDomainBlock{}).
		Index("GSI1").
		Where("GSI1PK", "=", "DOMAIN_BLOCKS").
		All(&blocks)

	if err != nil {
		r.logger.Error("failed to get all instance domain blocks", zap.Error(err))
		return nil, fmt.Errorf("failed to get all instance domain blocks: %w", err)
	}

	result := make([]*storage.InstanceDomainBlock, len(blocks))
	for i, block := range blocks {
		result[i] = r.modelToInstanceDomainBlock(&block)
	}

	return result, nil
}

// Helper to convert model to storage type
func (r *DomainBlockRepositoryV2) modelToInstanceDomainBlock(model *models.InstanceDomainBlock) *storage.InstanceDomainBlock {
	return &storage.InstanceDomainBlock{
		ID:             model.ID,
		Domain:         model.Domain,
		Severity:       model.Severity,
		RejectMedia:    model.RejectMedia,
		RejectReports:  model.RejectReports,
		PrivateComment: model.PrivateComment,
		PublicComment:  model.PublicComment,
		Obfuscate:      model.Obfuscate,
		CreatedBy:      model.CreatedBy,
		CreatedByID:    model.CreatedByID,
		CreatedAt:      model.CreatedAt,
		UpdatedAt:      model.UpdatedAt,
	}
}

// Code Reduction Summary:
// - AddDomainBlock: ~20 lines saved (error handling, key generation)
// - RemoveDomainBlock: ~15 lines saved (delete logic)
// - GetUserDomainBlocks: ~20 lines saved (query construction)
// - IsDomainBlocked: ~10 lines saved (existence check)
// Total: ~65 lines of boilerplate eliminated for user domain blocks!
//
// Note: Instance domain blocks and email domain blocks use different models,
// so they would need their own BaseRepository instances for full benefits.
// This demonstrates a hybrid approach where BaseRepository is used for the
// primary model type, while other operations use direct DynamORM.
