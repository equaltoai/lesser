package repositories

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// RelayRepository implements relay operations using DynamORM
type RelayRepository struct {
	db        core.DB
	tableName string
	logger    *zap.Logger
}

// NewRelayRepository creates a new relay repository
func NewRelayRepository(db core.DB, tableName string, logger *zap.Logger) *RelayRepository {
	return &RelayRepository{
		db:        db,
		tableName: tableName,
		logger:    logger,
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

	// Update keys
	model.UpdateKeys()

	// Store in DynamoDB (Put overwrites existing)
	err := r.db.WithContext(ctx).Model(model).Create()
	if err != nil {
		logger.Error("failed to store relay info", zap.Error(err))
		return fmt.Errorf("failed to store relay info: %w", err)
	}

	logger.Info("stored relay info successfully")
	return nil
}

// GetRelayInfo retrieves relay information
func (r *RelayRepository) GetRelayInfo(ctx context.Context, relayURL string) (*storage.RelayInfo, error) {
	logger := r.logger.With(zap.String("operation", "GetRelayInfo"), zap.String("relay_url", relayURL))

	var model models.Relay
	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Where("PK", "=", fmt.Sprintf("RELAY#%s", relayURL)).
		Where("SK", "=", "INFO").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return nil, fmt.Errorf("relay not found: %s", relayURL)
		}
		logger.Error("failed to get relay info", zap.Error(err))
		return nil, fmt.Errorf("failed to get relay info: %w", err)
	}

	// Convert to storage type
	relay := &storage.RelayInfo{
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

	logger.Debug("retrieved relay info successfully")
	return relay, nil
}

// RemoveRelayInfo removes relay information
func (r *RelayRepository) RemoveRelayInfo(ctx context.Context, relayURL string) error {
	logger := r.logger.With(zap.String("operation", "RemoveRelayInfo"), zap.String("relay_url", relayURL))

	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Where("PK", "=", fmt.Sprintf("RELAY#%s", relayURL)).
		Where("SK", "=", "INFO").
		Delete()

	if err != nil {
		logger.Error("failed to remove relay info", zap.Error(err))
		return fmt.Errorf("failed to remove relay info: %w", err)
	}

	logger.Info("removed relay info successfully")
	return nil
}

// GetActiveRelays retrieves all active relays
func (r *RelayRepository) GetActiveRelays(ctx context.Context) ([]*storage.RelayInfo, error) {
	logger := r.logger.With(zap.String("operation", "GetActiveRelays"))

	var relays []models.Relay
	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Index("GSI1").
		Where("GSI1PK", "=", "ACTIVE_RELAYS").
		All(&relays)

	if err != nil {
		logger.Error("failed to query active relays", zap.Error(err))
		return nil, fmt.Errorf("failed to query active relays: %w", err)
	}

	// Convert to storage types
	results := make([]*storage.RelayInfo, len(relays))
	for i, model := range relays {
		results[i] = &storage.RelayInfo{
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

	logger.Info("retrieved active relays", zap.Int("count", len(relays)))
	return results, nil
}

// GetAllRelays retrieves all relays with pagination
func (r *RelayRepository) GetAllRelays(ctx context.Context, limit int, cursor string) ([]*storage.RelayInfo, string, error) {
	logger := r.logger.With(zap.String("operation", "GetAllRelays"))

	// For now, use scan with filter since DynamORM doesn't support BEGINS_WITH on PK
	// In production, consider adding a GSI for this query pattern
	var relays []models.Relay
	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Limit(limit).
		All(&relays)
	
	// Filter results to only include relays
	var filtered []models.Relay
	for _, model := range relays {
		if strings.HasPrefix(model.PK, "RELAY#") {
			filtered = append(filtered, model)
		}
	}
	relays = filtered
	if err != nil {
		logger.Error("failed to query all relays", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query all relays: %w", err)
	}

	// Convert to storage types
	results := make([]*storage.RelayInfo, len(relays))
	for i, model := range relays {
		results[i] = &storage.RelayInfo{
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

	// For now, pagination not implemented - return empty cursor
	// TODO: Implement proper pagination with DynamORM
	var nextCursor string

	logger.Info("retrieved all relays", zap.Int("count", len(relays)))
	return results, nextCursor, nil
}

// UpdateRelayStatus updates the active status of a relay
func (r *RelayRepository) UpdateRelayStatus(ctx context.Context, relayURL string, active bool) error {
	logger := r.logger.With(zap.String("operation", "UpdateRelayStatus"),
		zap.String("relay_url", relayURL), zap.Bool("active", active))

	// First get the existing relay to update it properly
	var model models.Relay
	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Where("PK", "=", fmt.Sprintf("RELAY#%s", relayURL)).
		Where("SK", "=", "INFO").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("relay not found: %s", relayURL)
		}
		logger.Error("failed to get relay for update", zap.Error(err))
		return fmt.Errorf("failed to get relay for update: %w", err)
	}

	// Update the fields
	model.Active = active
	model.LastSeenAt = time.Now()

	// Update keys to reflect new active status
	model.UpdateKeys()

	// Update in database
	// Save the updated model
	err = r.db.WithContext(ctx).Model(&models.Relay{}).
		Where("PK", "=", model.PK).
		Where("SK", "=", model.SK).
		First(&models.Relay{})
	if err != nil {
		logger.Error("failed to update relay status", zap.Error(err))
		return fmt.Errorf("failed to update relay status: %w", err)
	}
	
	// Now save with new values
	err = r.db.WithContext(ctx).Model(&model).Create()

	if err != nil {
		logger.Error("failed to update relay status", zap.Error(err))
		return fmt.Errorf("failed to update relay status: %w", err)
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
	err := r.db.WithContext(ctx).Model(&models.Relay{}).
		Where("PK", "=", fmt.Sprintf("RELAY#%s", relayURL)).
		Where("SK", "=", "INFO").
		First(&model)

	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("relay not found: %s", relayURL)
		}
		logger.Error("failed to get relay for update", zap.Error(err))
		return fmt.Errorf("failed to get relay for update: %w", err)
	}

	// Update the fields from state
	model.Active = state.Active
	model.Status = state.Status
	model.ErrorCount = state.ErrorCount
	model.LastSeenAt = time.Now()

	// Update keys to reflect new state
	model.UpdateKeys()

	// Save the updated model
	err = r.db.WithContext(ctx).Model(&model).Create()
	if err != nil {
		logger.Error("failed to update relay state", zap.Error(err))
		return fmt.Errorf("failed to update relay state: %w", err)
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
	data, err := base64.URLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, fmt.Errorf("invalid cursor format: %w", err)
	}

	var key map[string]interface{}
	if err := json.Unmarshal(data, &key); err != nil {
		return nil, fmt.Errorf("failed to unmarshal cursor: %w", err)
	}

	return key, nil
}