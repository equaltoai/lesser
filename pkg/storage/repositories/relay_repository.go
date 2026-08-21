package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v3/pkg/core"
	"github.com/theory-cloud/tabletheory/v3/pkg/errors"
	"go.uber.org/zap"
)

// RelayRepository implements relay operations using enhanced patterns
type RelayRepository struct {
	*EnhancedBaseRepository[*models.Relay]
}

// NewRelayRepository creates a new relay repository with enhanced functionality
func NewRelayRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *RelayRepository {
	// Create enhanced repository optimized for relay operations
	enhancedRepo := NewEnhancedBaseRepository[*models.Relay](db, tableName, logger, costService, "RelayRepository", "relay")

	// Set up enhanced services for relay operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // Relay data cached for performance
	enhancedRepo.SetEventService(NewDefaultEventService())

	return &RelayRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// convertModelToRelayInfo converts a relay model to storage type
func (r *RelayRepository) convertModelToRelayInfo(model *models.Relay) *storage.RelayInfo {
	return &storage.RelayInfo{
		URL:        model.URL,
		InboxURL:   model.InboxURL,
		Active:     model.Active,
		CreatedAt:  model.CreatedAt,
		LastSeenAt: model.LastSeenAt,
		Domain:     model.Domain,
		Status:     model.Status,
		ErrorCount: model.ErrorCount,
		TTL:        model.TTL,
	}
}

// StoreRelayInfo stores relay information
func (r *RelayRepository) StoreRelayInfo(ctx context.Context, relay *storage.RelayInfo) error {
	logger := r.logger.With(zap.String("operation", "StoreRelayInfo"), zap.String("relay_url", relay.URL))

	// Extract domain from URL for indexing
	domain := relayExtractDomainFromURL(relay.URL)
	relay.Domain = domain

	// Set TTL for automatic cleanup (90 days for inactive relays, 365 days for active)
	ttlDays := 90
	if relay.Active {
		ttlDays = 365
	}
	relay.TTL = time.Now().Add(time.Duration(ttlDays) * 24 * time.Hour).Unix()

	// Create the model
	model := &models.Relay{
		URL:        relay.URL,
		InboxURL:   relay.InboxURL,
		Active:     relay.Active,
		CreatedAt:  relay.CreatedAt,
		LastSeenAt: relay.LastSeenAt,
		Domain:     relay.Domain,
		Status:     relay.Status,
		ErrorCount: relay.ErrorCount,
		TTL:        relay.TTL,
	}

	// Use BaseRepository Create method
	err := r.ValidateAndCreate(ctx, model)
	if err != nil {
		logger.Error("failed to store relay info", zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "relay", relay.URL)
	}

	logger.Info("stored relay info successfully")
	return nil
}

// GetRelayInfo retrieves relay information
func (r *RelayRepository) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	logger := r.logger.With(zap.String("operation", "GetRelayInfo"), zap.String("relay_url", relayURL))

	var model models.Relay
	err := r.Get(ctx, fmt.Sprintf("RELAY#%s", relayURL), "INFO", &model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrRelayNotFound, relayURL), "relay", relayURL)
		}
		logger.Error("failed to get relay info", zap.Error(err))
		return nil, ErrorHandler.HandleGetError(err, "relay", relayURL)
	}

	// Convert to storage type
	relay := r.convertModelToRelayInfo(&model)

	logger.Debug("retrieved relay info successfully")
	return relay, nil
}

// RemoveRelayInfo removes relay information
func (r *RelayRepository) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	logger := r.logger.With(zap.String("operation", "RemoveRelayInfo"), zap.String("relay_url", relayURL))

	err := r.Delete(ctx, fmt.Sprintf("RELAY#%s", relayURL), "INFO")

	if err != nil {
		logger.Error("failed to remove relay info", zap.Error(err))
		return ErrorHandler.HandleDeleteError(err, "relay", relayURL)
	}

	logger.Info("removed relay info successfully")
	return nil
}

// GetActiveRelays retrieves all active relays
func (r *RelayRepository) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	logger := r.logger.With(zap.String("operation", "GetActiveRelays"))

	// Use BaseRepository QueryGSI method to query active relays
	var relayModels []models.Relay
	err := r.GetDB().WithContext(ctx).Model(&models.Relay{}).
		Index("gsi1").
		Where("gsi1PK", "=", "ACTIVE_RELAYS").
		Limit(1000).
		All(&relayModels)

	if err != nil {
		logger.Error("failed to query active relays", zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "relay", "active relays")
	}

	// Convert to storage types
	results := make([]*storage.RelayInfo, 0, len(relayModels))
	for _, model := range relayModels {
		relay := r.convertModelToRelayInfo(&model)
		if relay != nil {
			results = append(results, relay)
		}
	}

	logger.Info("retrieved active relays", zap.Int("count", len(results)))
	return results, nil
}

// GetAllRelays retrieves all relays with pagination
func (r *RelayRepository) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	logger := r.logger.With(zap.String("operation", "GetAllRelays"))

	// Scan-free listing: query the relay listing partition on GSI8.
	query := r.GetDB().WithContext(ctx).Model(&models.Relay{}).
		Index("gsi8").
		Where("gsi8PK", "=", "RELAYS").
		OrderBy("gsi8SK", "ASC")

	// Resume after the last seen gsi8SK when a cursor is provided
	if cursor != "" {
		lastKey, err := decodeCursor(cursor)
		if err != nil {
			logger.Warn("invalid cursor provided", zap.String("cursor", cursor), zap.Error(err))
		} else if gsi8sk, ok := lastKey["gsi8SK"].(string); ok && gsi8sk != "" {
			query = query.Where("gsi8SK", ">", gsi8sk)
		}
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var relays []models.Relay
	err := query.All(&relays)
	if err != nil {
		logger.Error("failed to query all relays", zap.Error(err))
		return nil, "", ErrorHandler.HandleQueryError(err, "relay", "all relays")
	}

	// Generate next cursor
	var nextCursor string
	if len(relays) > limit {
		// We got more results than requested, so there are more pages
		lastRelay := relays[limit-1]
		lastKey := map[string]interface{}{
			"gsi8PK": lastRelay.GSI8PK,
			"gsi8SK": lastRelay.GSI8SK,
			"PK":     lastRelay.PK,
			"SK":     lastRelay.SK,
		}
		nextCursor = encodeCursor(lastKey)
		relays = relays[:limit] // Trim to requested limit
	}

	// Convert to storage types
	results := make([]*storage.RelayInfo, len(relays))
	for i, model := range relays {
		results[i] = r.convertModelToRelayInfo(&model)
	}

	logger.Info("retrieved all relays", zap.Int("count", len(relays)))
	return results, nextCursor, nil
}

// UpdateRelayStatus updates the active status of a relay
func (r *RelayRepository) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	logger := r.logger.With(zap.String("operation", "UpdateRelayStatus"),
		zap.String("relay_url", relayURL), zap.Bool("active", active))

	// First get the existing relay to update it properly
	var model models.Relay
	err := r.Get(ctx, fmt.Sprintf("RELAY#%s", relayURL), "INFO", &model)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrRelayNotFound, relayURL), "relay", relayURL)
		}
		logger.Error("failed to get relay for update", zap.Error(err))
		return ErrorHandler.HandleGetError(err, "relay", relayURL)
	}

	// Update the fields
	model.Active = active
	model.LastSeenAt = time.Now()

	// Use BaseRepository Create method which will handle UpdateKeys and Put operation
	err = r.ValidateAndCreateOrUpdate(ctx, &model)

	if err != nil {
		logger.Error("failed to update relay status", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "relay", relayURL)
	}

	logger.Info("updated relay status successfully")
	return nil
}

// CreateRelay creates a new relay
func (r *RelayRepository) CreateRelay(ctx context.Context, relay *storage.RelayInfo) error {
	// Delegate to StoreRelayInfo which handles creation
	relay.CreatedAt = time.Now()
	relay.LastSeenAt = relay.CreatedAt
	relay.Status = "pending"
	relay.ErrorCount = 0
	return r.StoreRelayInfo(ctx, relay)
}

// GetRelay retrieves a relay by URL (alias for GetRelayInfo)
func (r *RelayRepository) GetRelay(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	return r.GetRelayInfo(ctx, relayURL)
}

// UpdateRelayState updates multiple relay fields beyond just active status
func (r *RelayRepository) UpdateRelayState(ctx context.Context, relayURL string, state storage.RelayState) error {
	logger := r.logger.With(zap.String("operation", "UpdateRelayState"),
		zap.String("relay_url", relayURL))

	// First get the existing relay
	var model models.Relay
	err := r.Get(ctx, fmt.Sprintf("RELAY#%s", relayURL), "INFO", &model)

	if err != nil {
		if errors.IsNotFound(err) {
			return ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrRelayNotFound, relayURL), "relay", relayURL)
		}
		logger.Error("failed to get relay for update", zap.Error(err))
		return ErrorHandler.HandleGetError(err, "relay", relayURL)
	}

	// Update the fields from state
	model.Active = state.Active
	model.Status = state.Status
	model.ErrorCount = state.ErrorCount
	model.LastSeenAt = time.Now()

	// Use BaseRepository Create method which will handle UpdateKeys and Put operation
	err = r.ValidateAndCreateOrUpdate(ctx, &model)
	if err != nil {
		logger.Error("failed to update relay state", zap.Error(err))
		return ErrorHandler.HandleUpdateError(err, "relay", relayURL)
	}

	logger.Info("updated relay state successfully",
		zap.String("status", state.Status),
		zap.Bool("active", state.Active))
	return nil
}

// ListRelays retrieves all relays (alias for GetAllRelays without pagination)
func (r *RelayRepository) ListRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	relays, _, err := r.GetAllRelays(ctx, 1000, "") // Get up to 1000 relays
	return relays, err
}

// DeleteRelay removes a relay (alias for RemoveRelayInfo)
func (r *RelayRepository) DeleteRelay(ctx context.Context, relayURL string) error {
	return r.RemoveRelayInfo(ctx, relayURL)
}

// Helper functions

// relayExtractDomainFromURL extracts the domain from a URL
func relayExtractDomainFromURL(rawURL string) string {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	domain := parsedURL.Hostname()
	// Remove www. prefix if present
	domain = strings.TrimPrefix(domain, "www.")
	return domain
}

// encodeCursor encodes a DynamoDB last evaluated key into a cursor string
func encodeCursor(lastKey map[string]interface{}) string {
	data, err := json.Marshal(lastKey)
	if err != nil {
		return ""
	}
	return base64.URLEncoding.EncodeToString(data)
}

// decodeCursor decodes a cursor string into a DynamoDB exclusive start key
func decodeCursor(cursor string) (map[string]interface{}, error) {
	// Validate cursor format first
	if err := common.ValidateRepositoryCursor(cursor); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "relay cursor", "validation")
	}

	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "relay cursor", "decoding")
	}

	var key map[string]interface{}
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "relay cursor", "unmarshaling")
	}

	return key, nil
}
