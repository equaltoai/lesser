package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/google/uuid"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// DomainBlockRepository implements domain block operations using DynamORM with BaseRepository pattern
type DomainBlockRepository struct {
	baseRepo  *BaseRepository[*models.UserDomainBlock] // BaseRepository for user domain blocks
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewDomainBlockRepository creates a new domain block repository with BaseRepository integration
func NewDomainBlockRepository(db core.DB, tableName string, logger *zap.Logger) *DomainBlockRepository {
	baseRepo := NewBaseRepository[*models.UserDomainBlock](db, tableName, logger)
	return &DomainBlockRepository{
		baseRepo:  baseRepo,
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// NewDomainBlockRepositoryWithCostTracking creates a new domain block repository with cost tracking
func NewDomainBlockRepositoryWithCostTracking(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *DomainBlockRepository {
	baseRepo := NewBaseRepositoryWithCostTracking[*models.UserDomainBlock](db, tableName, logger, costService, "DomainBlockRepository")
	return &DomainBlockRepository{
		baseRepo:  baseRepo,
		db:        db,
		tableName: tableName,
		logger:    logger,
	}
}

// AddDomainBlock adds a domain to the user's block list
func (r *DomainBlockRepository) AddDomainBlock(ctx context.Context, username, domain string) error {
	block := &models.UserDomainBlock{
		Username:  username,
		Domain:    domain,
		CreatedAt: time.Now(),
	}
	if err := block.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "domain block", domain)
	}

	// Use BaseRepository Create method which handles key updates automatically
	err := r.baseRepo.Create(ctx, block)

	if err != nil {
		// Check if it's a duplicate (already exists)
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			// Already blocked, not an error
			return nil
		}
		return ErrorHandler.HandleCreateError(err, "domain block", domain)
	}

	return nil
}

// RemoveDomainBlock removes a domain from the user's block list
func (r *DomainBlockRepository) RemoveDomainBlock(ctx context.Context, username, domain string) error {
	block := &models.UserDomainBlock{
		Username: username,
		Domain:   domain,
	}
	if err := block.UpdateKeys(); err != nil {
		return ErrorHandler.HandleDeleteError(err, "domain block", domain)
	}

	// Use BaseRepository Delete method with proper error handling
	err := r.baseRepo.Delete(ctx, block.PK, block.SK)

	if err != nil {
		// Handle not found gracefully (idempotent operation)
		if strings.Contains(err.Error(), "not found") {
			r.logger.Debug("domain block not found for removal",
				zap.String("username", username),
				zap.String("domain", domain))
			return nil
		}
		return ErrorHandler.HandleDeleteError(err, "domain block", domain)
	}

	r.logger.Info("removed domain block",
		zap.String("username", username),
		zap.String("domain", domain))
	return nil
}

// GetUserDomainBlocks retrieves all domains blocked by a user
func (r *DomainBlockRepository) GetUserDomainBlocks(ctx context.Context, username string, limit int, cursor string) ([]string, string, error) {
	if limit <= 0 {
		limit = 20
	}

	query := r.db.WithContext(ctx).Model(&models.UserDomainBlock{}).
		Where("PK", "=", fmt.Sprintf("USER#%s", username)).
		Limit(limit)

	if cursor != "" {
		query = query.Cursor(cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var blocks []models.UserDomainBlock
	err := query.All(&blocks)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "domain block", "blocking")
	}

	// Generate next cursor
	var nextCursor string
	if len(blocks) > limit {
		// We got more results than requested, so there are more pages
		nextCursor = blocks[limit-1].SK
		blocks = blocks[:limit] // Trim to requested limit
	}

	// Extract domains from the blocks
	domains := make([]string, 0, len(blocks))
	for _, block := range blocks {
		domains = append(domains, block.Domain)
	}

	return domains, nextCursor, nil
}

// IsBlockedDomain checks if a domain is blocked by a user
func (r *DomainBlockRepository) IsBlockedDomain(ctx context.Context, username, domain string) (bool, error) {
	// Normalize domain
	normalizedDomain := strings.ToLower(strings.TrimSpace(domain))
	pk := fmt.Sprintf("USER#%s", username)
	sk := fmt.Sprintf("DOMAIN_BLOCK#%s", normalizedDomain)

	// Use BaseRepository Exists method for efficient checking
	exists, err := r.baseRepo.Exists(ctx, pk, sk)
	if err != nil {
		r.logger.Error("failed to check if domain is blocked",
			zap.String("username", username),
			zap.String("domain", normalizedDomain),
			zap.Error(err))
		return false, ErrorHandler.HandleGetError(err, "blocked domain", normalizedDomain)
	}

	return exists, nil
}

// CreateInstanceDomainBlock creates an instance-level domain block
func (r *DomainBlockRepository) CreateInstanceDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	// Generate ID if not provided
	if err := common.ValidateRequiredParam("block.ID", block.ID); err != nil {
		block.ID = uuid.New().String()
	}

	// Set timestamps
	now := time.Now()
	block.CreatedAt = now
	block.UpdatedAt = now

	// Normalize domain to lowercase
	block.Domain = strings.ToLower(strings.TrimSpace(block.Domain))

	// Convert storage.InstanceDomainBlock to models.InstanceDomainBlock
	modelBlock := &models.InstanceDomainBlock{
		ID:             block.ID,
		Domain:         block.Domain,
		Severity:       block.Severity,
		RejectMedia:    block.RejectMedia,
		RejectReports:  block.RejectReports,
		PrivateComment: block.PrivateComment,
		PublicComment:  block.PublicComment,
		Obfuscate:      block.Obfuscate,
		CreatedBy:      block.CreatedBy,
		CreatedByID:    block.CreatedByID,
		CreatedAt:      block.CreatedAt,
		UpdatedAt:      block.UpdatedAt,
	}
	if err := modelBlock.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "domain block", block.Domain)
	}

	// Create with condition to prevent duplicates
	err := r.db.WithContext(ctx).Model(modelBlock).Create()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			r.logger.Debug("domain block already exists",
				zap.String("domain", block.Domain),
				zap.String("created_by", block.CreatedBy))
			return ErrorHandler.HandleCreateError(err, "domain block", block.Domain)
		}
		return ErrorHandler.HandleCreateError(err, "domain block", block.Domain)
	}

	r.logger.Info("created instance domain block",
		zap.String("domain", block.Domain),
		zap.String("severity", block.Severity),
		zap.String("created_by", block.CreatedBy))

	return nil
}

// GetInstanceDomainBlock retrieves a domain block by domain
func (r *DomainBlockRepository) GetInstanceDomainBlock(ctx context.Context, domain string) (*storage.InstanceDomainBlock, error) {
	domain = strings.ToLower(strings.TrimSpace(domain))

	var modelBlock models.InstanceDomainBlock
	err := r.db.WithContext(ctx).Model(&models.InstanceDomainBlock{}).
		Where("PK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		Where("SK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		First(&modelBlock)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, storage.ErrNotFound
		}
		return nil, ErrorHandler.HandleGetError(err, "domain block", domain)
	}

	// Convert models.InstanceDomainBlock to storage.InstanceDomainBlock
	return &storage.InstanceDomainBlock{
		ID:             modelBlock.ID,
		Domain:         modelBlock.Domain,
		Severity:       modelBlock.Severity,
		RejectMedia:    modelBlock.RejectMedia,
		RejectReports:  modelBlock.RejectReports,
		PrivateComment: modelBlock.PrivateComment,
		PublicComment:  modelBlock.PublicComment,
		Obfuscate:      modelBlock.Obfuscate,
		CreatedBy:      modelBlock.CreatedBy,
		CreatedByID:    modelBlock.CreatedByID,
		CreatedAt:      modelBlock.CreatedAt,
		UpdatedAt:      modelBlock.UpdatedAt,
	}, nil
}

// GetInstanceDomainBlockByID retrieves a domain block by ID
func (r *DomainBlockRepository) GetInstanceDomainBlockByID(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	// Query GSI1 to find the domain block by ID
	var blocks []models.InstanceDomainBlock
	err := r.db.WithContext(ctx).Model(&models.InstanceDomainBlock{}).
		Index("GSI1").
		Where("GSI1PK", "=", "DOMAIN_BLOCKS").
		Limit(100). // Need to scan to find by ID
		All(&blocks)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "domain block", "by ID")
	}

	// Find the block with matching ID
	var modelBlock *models.InstanceDomainBlock
	for _, block := range blocks {
		if block.ID == id {
			modelBlock = &block
			break
		}
	}

	if modelBlock == nil {
		return nil, storage.ErrNotFound
	}
	return &storage.InstanceDomainBlock{
		ID:             modelBlock.ID,
		Domain:         modelBlock.Domain,
		Severity:       modelBlock.Severity,
		RejectMedia:    modelBlock.RejectMedia,
		RejectReports:  modelBlock.RejectReports,
		PrivateComment: modelBlock.PrivateComment,
		PublicComment:  modelBlock.PublicComment,
		Obfuscate:      modelBlock.Obfuscate,
		CreatedBy:      modelBlock.CreatedBy,
		CreatedByID:    modelBlock.CreatedByID,
		CreatedAt:      modelBlock.CreatedAt,
		UpdatedAt:      modelBlock.UpdatedAt,
	}, nil
}

// ListInstanceDomainBlocks lists all instance domain blocks with pagination
func (r *DomainBlockRepository) ListInstanceDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	config := DomainPaginationConfig{
		GSIPKValue:  "DOMAIN_BLOCKS",
		ErrorPrefix: "list domain blocks",
	}
	return getPaginatedInstanceDomainBlocks(ctx, r.db, r.logger, limit, cursor, config)
}

// UpdateInstanceDomainBlock updates an existing domain block
func (r *DomainBlockRepository) UpdateInstanceDomainBlock(ctx context.Context, domain string, updates map[string]any) error {
	domain = strings.ToLower(strings.TrimSpace(domain))

	// Get existing block first
	var modelBlock models.InstanceDomainBlock
	err := r.db.WithContext(ctx).Model(&models.InstanceDomainBlock{}).
		Where("PK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		Where("SK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		First(&modelBlock)

	if err != nil {
		if errors.IsNotFound(err) {
			return storage.ErrNotFound
		}
		return ErrorHandler.HandleGetError(err, "domain block", domain)
	}

	// Apply updates to the model
	modelBlock.UpdatedAt = time.Now()

	if severity, ok := updates["severity"].(string); ok {
		modelBlock.Severity = severity
	}
	if rejectMedia, ok := updates["reject_media"].(bool); ok {
		modelBlock.RejectMedia = rejectMedia
	}
	if rejectReports, ok := updates["reject_reports"].(bool); ok {
		modelBlock.RejectReports = rejectReports
	}
	if privateComment, ok := updates["private_comment"].(string); ok {
		modelBlock.PrivateComment = privateComment
	}
	if publicComment, ok := updates["public_comment"].(string); ok {
		modelBlock.PublicComment = publicComment
	}
	if obfuscate, ok := updates["obfuscate"].(bool); ok {
		modelBlock.Obfuscate = obfuscate
	}

	// Update the model
	err = r.db.WithContext(ctx).Model(&modelBlock).Update()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return storage.ErrNotFound
		}
		return ErrorHandler.HandleUpdateError(err, "domain block", domain)
	}

	return nil
}

// DeleteInstanceDomainBlock deletes a domain block
func (r *DomainBlockRepository) DeleteInstanceDomainBlock(ctx context.Context, domain string) error {
	domain = strings.ToLower(strings.TrimSpace(domain))

	err := r.db.WithContext(ctx).Model(&models.InstanceDomainBlock{}).
		Where("PK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		Where("SK", "=", fmt.Sprintf("DOMAIN_BLOCK#%s", domain)).
		Delete()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return storage.ErrNotFound
		}
		return ErrorHandler.HandleDeleteError(err, "domain block", domain)
	}

	return nil
}

// IsInstanceDomainBlocked checks if a domain is blocked at the instance level
func (r *DomainBlockRepository) IsInstanceDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	block, err := r.GetInstanceDomainBlock(ctx, domain)
	if err != nil {
		if err == storage.ErrNotFound {
			// Check parent domains (e.g., if sub.example.com is queried, check example.com)
			parts := strings.Split(domain, ".")
			for i := 1; i < len(parts); i++ {
				parentDomain := strings.Join(parts[i:], ".")
				parentBlock, err := r.GetInstanceDomainBlock(ctx, parentDomain)
				if err == nil {
					return true, parentBlock, nil
				}
			}
			return false, nil, nil
		}
		return false, nil, err
	}

	return true, block, nil
}

// GetDomainBlocks retrieves instance-level domain blocks with pagination
func (r *DomainBlockRepository) GetDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.InstanceDomainBlock, string, error) {
	// This is the same as ListInstanceDomainBlocks
	return r.ListInstanceDomainBlocks(ctx, limit, cursor)
}

// GetDomainBlock retrieves a specific domain block by ID
func (r *DomainBlockRepository) GetDomainBlock(ctx context.Context, id string) (*storage.InstanceDomainBlock, error) {
	// This is the same as GetInstanceDomainBlockByID
	return r.GetInstanceDomainBlockByID(ctx, id)
}

// CreateDomainBlock creates a new instance-level domain block
func (r *DomainBlockRepository) CreateDomainBlock(ctx context.Context, block *storage.InstanceDomainBlock) error {
	// This is the same as CreateInstanceDomainBlock
	return r.CreateInstanceDomainBlock(ctx, block)
}

// UpdateDomainBlock updates an existing domain block
func (r *DomainBlockRepository) UpdateDomainBlock(ctx context.Context, id string, updates map[string]any) error {
	// First get the block to find the domain
	block, err := r.GetDomainBlock(ctx, id)
	if err != nil {
		return err
	}

	// Update using the domain
	return r.UpdateInstanceDomainBlock(ctx, block.Domain, updates)
}

// DeleteDomainBlock removes a domain block
func (r *DomainBlockRepository) DeleteDomainBlock(ctx context.Context, id string) error {
	// First get the block to find the domain
	block, err := r.GetDomainBlock(ctx, id)
	if err != nil {
		return err
	}

	// Delete using the domain
	return r.DeleteInstanceDomainBlock(ctx, block.Domain)
}

// IsDomainBlocked checks if a domain is blocked at the instance level
func (r *DomainBlockRepository) IsDomainBlocked(ctx context.Context, domain string) (bool, *storage.InstanceDomainBlock, error) {
	// This is the same as IsInstanceDomainBlocked
	return r.IsInstanceDomainBlocked(ctx, domain)
}

// CreateEmailDomainBlock creates an email domain block
func (r *DomainBlockRepository) CreateEmailDomainBlock(ctx context.Context, block *storage.EmailDomainBlock) error {
	// Generate ID if not provided
	if err := common.ValidateRequiredParam("block.ID", block.ID); err != nil {
		block.ID = uuid.New().String()
	}
	block.CreatedAt = time.Now()

	// Convert to model
	modelBlock := &models.EmailDomainBlock{
		ID:        block.ID,
		Domain:    block.Domain,
		CreatedBy: block.CreatedBy,
		CreatedAt: block.CreatedAt,
	}
	if err := modelBlock.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "email domain block", block.Domain)
	}

	// Create with condition to prevent duplicates
	err := r.db.WithContext(ctx).Model(modelBlock).Create()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return ErrorHandler.HandleCreateError(err, "email domain block", modelBlock.Domain)
		}
		return ErrorHandler.HandleCreateError(err, "email domain block", modelBlock.Domain)
	}

	return nil
}

// GetEmailDomainBlocks retrieves email domain blocks with pagination
func (r *DomainBlockRepository) GetEmailDomainBlocks(ctx context.Context, limit int, cursor string) ([]*storage.EmailDomainBlock, string, error) {
	config := DomainPaginationConfig{
		GSIPKValue:  "EMAIL_DOMAIN_BLOCKS",
		ErrorPrefix: "query email domain blocks",
	}
	return getPaginatedEmailDomainBlocks(ctx, r.db, r.logger, limit, cursor, config)
}

// DeleteEmailDomainBlock deletes an email domain block
func (r *DomainBlockRepository) DeleteEmailDomainBlock(ctx context.Context, id string) error {
	return deleteEmailDomainBlockByID(ctx, r.db, r.logger, id, "EMAIL_DOMAIN_BLOCKS")
}

// Domain allow operations (for allowlist mode)

// GetDomainAllows retrieves domain allows (for allowlist mode)
func (r *DomainBlockRepository) GetDomainAllows(ctx context.Context, limit int, cursor string) ([]*storage.DomainAllow, string, error) {
	config := DomainPaginationConfig{
		GSIPKValue:  "DOMAIN_ALLOWS",
		ErrorPrefix: "query domain allows",
	}
	return getPaginatedDomainAllows(ctx, r.db, r.logger, limit, cursor, config)
}

// CreateDomainAllow adds a domain to the allowlist
func (r *DomainBlockRepository) CreateDomainAllow(ctx context.Context, allow *storage.DomainAllow) error {
	// Generate ID if not provided
	if err := common.ValidateRequiredParam("allow.ID", allow.ID); err != nil {
		allow.ID = uuid.New().String()
	}
	allow.CreatedAt = time.Now()

	// Convert to model
	modelAllow := &models.DomainAllow{
		ID:        allow.ID,
		Domain:    allow.Domain,
		CreatedBy: allow.CreatedBy,
		CreatedAt: allow.CreatedAt,
	}
	if err := modelAllow.UpdateKeys(); err != nil {
		return ErrorHandler.HandleCreateError(err, "domain allow", allow.Domain)
	}

	// Create with condition to prevent duplicates
	err := r.db.WithContext(ctx).Model(modelAllow).Create()

	if err != nil {
		if strings.Contains(err.Error(), "ConditionalCheckFailedException") {
			return ErrorHandler.HandleCreateError(err, "domain allow", modelAllow.Domain)
		}
		return ErrorHandler.HandleCreateError(err, "domain allow", modelAllow.Domain)
	}

	return nil
}

// DeleteDomainAllow removes a domain from the allowlist
func (r *DomainBlockRepository) DeleteDomainAllow(ctx context.Context, id string) error {
	return deleteDomainAllowByID(ctx, r.db, r.logger, id, "DOMAIN_ALLOWS")
}
