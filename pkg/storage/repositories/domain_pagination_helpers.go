package repositories

import (
	"context"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"go.uber.org/zap"
)

// DomainPaginationConfig holds configuration for paginated domain queries
type DomainPaginationConfig struct {
	GSIPKValue  string // Value for GSI1PK, e.g. "DOMAIN_BLOCKS", "EMAIL_DOMAIN_BLOCKS", "DOMAIN_ALLOWS"
	ErrorPrefix string // Prefix for error messages
}

const (
	domainDefaultLimit = 20
	domainMaxLimit     = 100
)

func clampDomainLimit(limit int) int {
	if limit <= 0 {
		return domainDefaultLimit
	}
	if limit > domainMaxLimit {
		return domainMaxLimit
	}
	return limit
}

// buildPaginationQuery creates a common paginated query for domain models
func buildPaginationQuery(
	ctx context.Context,
	db core.DB,
	limit int,
	cursor string,
	config DomainPaginationConfig,
	modelPtr interface{},
) (core.Query, int) {
	safeLimit := clampDomainLimit(limit)

	query := db.WithContext(ctx).Model(modelPtr).
		Index("gsi1").
		Where("gsi1PK", "=", config.GSIPKValue).
		OrderBy("gsi1SK", "DESC") // Newest first

	if cursor != "" {
		query = query.Where("gsi1SK", "<", cursor)
	}

	// Fetch one more item than requested to determine if there are more results
	query = query.Limit(safeLimit + 1)

	return query, safeLimit
}

// generateNextCursor creates a cursor from the last item if there are more results
func generateNextCursor(resultsLen, requestedLimit int, getGSI1SK func() string) string {
	if resultsLen > requestedLimit && requestedLimit > 0 {
		return getGSI1SK()
	}
	return ""
}

// getPaginatedInstanceDomainBlocks retrieves instance domain blocks with pagination
func getPaginatedInstanceDomainBlocks(
	ctx context.Context,
	db core.DB,
	_ *zap.Logger,
	limit int,
	cursor string,
	config DomainPaginationConfig,
) ([]*storage.InstanceDomainBlock, string, error) {
	query, safeLimit := buildPaginationQuery(ctx, db, limit, cursor, config, &models.InstanceDomainBlock{})

	var modelBlocks []models.InstanceDomainBlock
	err := query.All(&modelBlocks)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, EntityInstanceDomainBlock, "paginated query")
	}

	// Generate next cursor and trim results if needed
	nextCursor := generateNextCursor(len(modelBlocks), safeLimit, func() string {
		return modelBlocks[safeLimit-1].GSI1SK
	})

	if len(modelBlocks) > safeLimit {
		modelBlocks = modelBlocks[:safeLimit] // Trim to requested limit
	}

	// Convert to storage type
	blocks := make([]*storage.InstanceDomainBlock, 0, len(modelBlocks))
	for _, mb := range modelBlocks {
		blocks = append(blocks, &storage.InstanceDomainBlock{
			ID:             mb.ID,
			Domain:         mb.Domain,
			Severity:       mb.Severity,
			RejectMedia:    mb.RejectMedia,
			RejectReports:  mb.RejectReports,
			PrivateComment: mb.PrivateComment,
			PublicComment:  mb.PublicComment,
			Obfuscate:      mb.Obfuscate,
			CreatedBy:      mb.CreatedBy,
			CreatedByID:    mb.CreatedByID,
			CreatedAt:      mb.CreatedAt,
			UpdatedAt:      mb.UpdatedAt,
		})
	}

	return blocks, nextCursor, nil
}

// DomainConverter defines how to convert model items to storage items
type DomainConverter[M any, S any] interface {
	Convert(M) S
	GetGSI1SK(M) string
}

// EmailDomainBlockConverter converts models.EmailDomainBlock to storage.EmailDomainBlock
type EmailDomainBlockConverter struct{}

// Convert transforms a models.EmailDomainBlock into a storage.EmailDomainBlock
func (c EmailDomainBlockConverter) Convert(m models.EmailDomainBlock) *storage.EmailDomainBlock {
	return &storage.EmailDomainBlock{
		ID:        m.ID,
		Domain:    m.Domain,
		CreatedBy: m.CreatedBy,
		CreatedAt: m.CreatedAt,
	}
}

// GetGSI1SK returns the GSI1 sort key for the given EmailDomainBlock model
func (c EmailDomainBlockConverter) GetGSI1SK(m models.EmailDomainBlock) string {
	return m.GSI1SK
}

// DomainAllowConverter converts models.DomainAllow to storage.DomainAllow
type DomainAllowConverter struct{}

// Convert transforms a models.DomainAllow into a storage.DomainAllow
func (c DomainAllowConverter) Convert(m models.DomainAllow) *storage.DomainAllow {
	return &storage.DomainAllow{
		ID:        m.ID,
		Domain:    m.Domain,
		CreatedBy: m.CreatedBy,
		CreatedAt: m.CreatedAt,
	}
}

// GetGSI1SK returns the GSI1 sort key for the given DomainAllow model
func (c DomainAllowConverter) GetGSI1SK(m models.DomainAllow) string {
	return m.GSI1SK
}

// getPaginatedDomainItems is a generic function for paginated domain queries
func getPaginatedDomainItems[M any, S any](
	ctx context.Context,
	db core.DB,
	limit int,
	cursor string,
	config DomainPaginationConfig,
	modelPtr *M,
	converter DomainConverter[M, S],
) ([]S, string, error) {
	query, safeLimit := buildPaginationQuery(ctx, db, limit, cursor, config, modelPtr)

	var models []M
	err := query.All(&models)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "domain items", "paginated query")
	}

	// Generate next cursor and trim results if needed
	nextCursor := generateNextCursor(len(models), safeLimit, func() string {
		return converter.GetGSI1SK(models[safeLimit-1])
	})

	if len(models) > safeLimit {
		models = models[:safeLimit] // Trim to requested limit
	}

	// Convert to storage type
	results := make([]S, 0, len(models))
	for _, m := range models {
		results = append(results, converter.Convert(m))
	}

	return results, nextCursor, nil
}

// getPaginatedEmailDomainBlocks retrieves email domain blocks with pagination
func getPaginatedEmailDomainBlocks(
	ctx context.Context,
	db core.DB,
	_ *zap.Logger,
	limit int,
	cursor string,
	config DomainPaginationConfig,
) ([]*storage.EmailDomainBlock, string, error) {
	results, nextCursor, err := getPaginatedDomainItems(ctx, db, limit, cursor, config, &models.EmailDomainBlock{}, EmailDomainBlockConverter{})
	return results, nextCursor, err
}

// getPaginatedDomainAllows retrieves domain allows with pagination
func getPaginatedDomainAllows(
	ctx context.Context,
	db core.DB,
	_ *zap.Logger,
	limit int,
	cursor string,
	config DomainPaginationConfig,
) ([]*storage.DomainAllow, string, error) {
	results, nextCursor, err := getPaginatedDomainItems(ctx, db, limit, cursor, config, &models.DomainAllow{}, DomainAllowConverter{})
	return results, nextCursor, err
}

// DomainItem interface for models that support domain-based operations
type DomainItem interface {
	GetID() string
	GetPK() string
	GetSK() string
}

// DomainDeleteConfig holds configuration for domain deletion operations
type DomainDeleteConfig struct {
	ModelType   string // "email_domain_block", "domain_allow"
	ErrorPrefix string // Error message prefix for operations
}

// genericDeleteByID finds and deletes an item by ID from a domain collection
func genericDeleteByID[T DomainItem](ctx context.Context, db core.DB, _ *zap.Logger, id string, gsipkValue string, modelPtr T) error {
	// First, find the item by ID - need to query GSI1 and filter
	var items []T
	err := db.WithContext(ctx).Model(modelPtr).
		Index("gsi1").
		Where("gsi1PK", "=", gsipkValue).
		Limit(100). // Need to scan to find by ID
		All(&items)

	if err != nil {
		return ErrorHandler.HandleQueryError(err, "domain item", "lookup by GSI")
	}

	// Find the item with matching ID
	var item T
	var found bool
	for _, i := range items {
		if i.GetID() == id {
			item = i
			found = true
			break
		}
	}

	if !found {
		return storage.ErrNotFound
	}

	// Delete the found item
	err = db.WithContext(ctx).Model(modelPtr).
		Where("PK", "=", item.GetPK()).
		Where("SK", "=", item.GetSK()).
		Delete()

	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "domain item", id)
	}

	return nil
}

// deleteEmailDomainBlockByID finds and deletes an email domain block by ID
func deleteEmailDomainBlockByID(ctx context.Context, db core.DB, logger *zap.Logger, id string, gsipkValue string) error {
	return genericDeleteByID(ctx, db, logger, id, gsipkValue, &models.EmailDomainBlock{})
}

// deleteDomainAllowByID finds and deletes a domain allow by ID
func deleteDomainAllowByID(ctx context.Context, db core.DB, logger *zap.Logger, id string, gsipkValue string) error {
	return genericDeleteByID(ctx, db, logger, id, gsipkValue, &models.DomainAllow{})
}
