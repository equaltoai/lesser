package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/pkg/core"
	"github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

const (
	// EnabledValue represents the string "true" for environment variables
	EnabledValue = "true"

	// MaxConnectionsPerUser defines the maximum connections allowed per user
	MaxConnectionsPerUser = 10
	// MaxTotalConnections defines the maximum total connections allowed globally
	MaxTotalConnections = 10000
	// DefaultIdleThreshold defines the default time before a connection is considered idle
	DefaultIdleThreshold = time.Minute * 30
	// connectionQueryLimit provides a defensive cap on unbounded connection queries
	connectionQueryLimit = 100
)

// StreamingConnectionRepository handles WebSocket connections using enhanced patterns
type StreamingConnectionRepository struct {
	*EnhancedBaseRepository[*models.WebSocketConnection]
	subscriptionRepo  *EnhancedBaseRepository[*models.WebSocketSubscription]
	subscriptionDB    core.DB
	subscriptionTable string
	logger            *zap.Logger
}

// NewStreamingConnectionRepository creates a new streaming connection repository with enhanced functionality
func NewStreamingConnectionRepository(db core.DB, tableName string, subscriptionDB core.DB, subscriptionTable string, logger *zap.Logger, costService *cost.TrackingService) *StreamingConnectionRepository {
	// Create enhanced repository for WebSocket connections
	connectionRepo := NewEnhancedBaseRepository[*models.WebSocketConnection](db, tableName, logger, costService, "StreamingConnectionRepository", "streamingconnection")
	connectionRepo.SetValidationService(NewDefaultValidationService())
	connectionRepo.SetPermissionService(NewDefaultPermissionService())
	connectionRepo.SetCachingService(NewInMemoryCachingService()) // Connections cached for real-time performance
	connectionRepo.SetEventService(NewDefaultEventService())

	// Create enhanced repository for subscriptions
	subscriptionRepo := NewEnhancedBaseRepository[*models.WebSocketSubscription](subscriptionDB, subscriptionTable, logger, costService, "WebSocketSubscriptionRepository", "websocketsubscription")
	subscriptionRepo.SetValidationService(NewDefaultValidationService())
	subscriptionRepo.SetPermissionService(NewDefaultPermissionService())
	subscriptionRepo.SetCachingService(NewInMemoryCachingService())
	subscriptionRepo.SetEventService(NewDefaultEventService())

	return &StreamingConnectionRepository{
		EnhancedBaseRepository: connectionRepo,
		subscriptionRepo:       subscriptionRepo,
		subscriptionDB:         subscriptionDB,
		subscriptionTable:      subscriptionTable,
		logger:                 logger,
	}
}

// WriteConnection stores a WebSocket connection with full lifecycle initialization and connection pooling
func (r *StreamingConnectionRepository) WriteConnection(ctx context.Context, connectionID, userID, username string, streams []string) (*models.WebSocketConnection, error) {
	// Check connection limits before creating new connection (Count() queries keep this efficient)
	if err := r.checkConnectionLimits(ctx, userID); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, "streaming connection", "connection limit check")
	}

	now := time.Now()
	connection := &models.WebSocketConnection{
		ConnectionID:   connectionID,
		UserID:         userID,
		Username:       username,
		Streams:        streams,
		Established:    now,
		LastActivity:   now,
		State:          models.ConnectionStateConnecting,
		StateChangedAt: now,
		MaxRetries:     5,
		IdleTimeout:    time.Hour * 2, // 2 hour idle timeout
		MaxMessageSize: 1024 * 64,     // 64KB max message size
		RateLimit:      100,           // 100 messages per minute
		RateLimitReset: now.Add(time.Minute),
		TTL:            now.Add(24 * time.Hour).Unix(),
		Metrics: models.ConnectionMetrics{
			ConnectionQuality: 1.0, // Start with good quality
		},
		Info: models.ConnectionInfo{
			Protocol:   "websocket",
			APIVersion: "v1",
		},
	}

	// Use BaseRepository Create method
	if err := r.ValidateAndCreate(ctx, connection); err != nil {
		return nil, ErrorHandler.HandleCreateError(err, "streaming connection", connectionID)
	}

	r.logger.Info("connection created",
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.String("state", string(models.ConnectionStateConnecting)))

	return connection, nil
}

// GetConnection retrieves a WebSocket connection by connection ID
func (r *StreamingConnectionRepository) GetConnection(ctx context.Context, connectionID string) (*models.WebSocketConnection, error) {
	connection := &models.WebSocketConnection{}
	pk := fmt.Sprintf("CONN#%s", connectionID)
	sk := fmt.Sprintf("CONN#%s", connectionID)

	err := r.Get(ctx, pk, sk, connection)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(ErrStreamingConnectionNotFound, "streaming connection", connectionID)
		}
		return nil, ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	return connection, nil
}

// UpdateConnection updates an existing WebSocket connection
func (r *StreamingConnectionRepository) UpdateConnection(ctx context.Context, connection *models.WebSocketConnection) error {
	// Use BaseRepository Update method
	if err := r.Update(ctx, connection); err != nil {
		return ErrorHandler.HandleUpdateError(err, "streaming connection", "update")
	}

	return nil
}

// UpdateConnectionState updates the connection state with proper state transition
func (r *StreamingConnectionRepository) UpdateConnectionState(ctx context.Context, connectionID string, newState models.ConnectionState, reason string) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	oldState := connection.State
	connection.UpdateState(newState)
	if reason != "" {
		connection.CloseReason = reason
	}

	if err := r.UpdateConnection(ctx, connection); err != nil {
		return ErrorHandler.HandleUpdateError(err, "streaming connection", connectionID)
	}

	r.logger.Info("connection state updated",
		zap.String("connection_id", connectionID),
		zap.String("old_state", string(oldState)),
		zap.String("new_state", string(newState)),
		zap.String("reason", reason))

	return nil
}

// DeleteConnection removes a WebSocket connection
func (r *StreamingConnectionRepository) DeleteConnection(ctx context.Context, connectionID string) error {
	pk := fmt.Sprintf("CONN#%s", connectionID)
	sk := fmt.Sprintf("CONN#%s", connectionID)

	// Use BaseRepository Delete method
	if err := r.Delete(ctx, pk, sk); err != nil {
		return ErrorHandler.HandleDeleteError(err, "streaming connection", connectionID)
	}

	return nil
}

// WriteSubscription stores a stream subscription
func (r *StreamingConnectionRepository) WriteSubscription(ctx context.Context, connectionID, userID, stream string) error {
	start := time.Now()
	var deadline string
	if dl, ok := ctx.Deadline(); ok {
		deadline = dl.UTC().Format(time.RFC3339Nano)
	}
	r.logger.Info("attempting to write websocket subscription",
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.String("stream", stream),
		zap.String("deadline", deadline))

	subscription := &models.WebSocketSubscription{
		ConnectionID: connectionID,
		UserID:       userID,
		Stream:       stream,
		SubscribedAt: time.Now(),
		TTL:          time.Now().Add(24 * time.Hour).Unix(),
	}

	// Use subscription BaseRepository Create method
	if err := r.subscriptionRepo.ValidateAndCreate(ctx, subscription); err != nil {
		r.logger.Error("websocket subscription write failed",
			zap.String("connection_id", connectionID),
			zap.String("user_id", userID),
			zap.String("stream", stream),
			zap.Duration("elapsed", time.Since(start)),
			zap.String("context_err", fmt.Sprint(ctx.Err())),
			zap.Error(err))
		return ErrorHandler.HandleCreateError(err, "websocket subscription", connectionID)
	}

	r.logger.Info("websocket subscription written",
		zap.String("connection_id", connectionID),
		zap.String("user_id", userID),
		zap.String("stream", stream),
		zap.Duration("elapsed", time.Since(start)))

	return nil
}

// DeleteSubscription removes a stream subscription
func (r *StreamingConnectionRepository) DeleteSubscription(ctx context.Context, connectionID, stream string) error {
	pk := fmt.Sprintf("SUB#%s", stream)
	sk := fmt.Sprintf("CONN#%s", connectionID)

	// Use subscription BaseRepository Delete method
	if err := r.subscriptionRepo.Delete(ctx, pk, sk); err != nil {
		return ErrorHandler.HandleDeleteError(err, "websocket subscription", connectionID)
	}

	return nil
}

// DeleteAllSubscriptions removes all subscriptions for a connection
func (r *StreamingConnectionRepository) DeleteAllSubscriptions(ctx context.Context, connectionID string) error {
	// Query all subscriptions for this connection using GSI
	var subscriptions []models.WebSocketSubscription

	err := r.subscriptionRepo.GetDB().WithContext(ctx).Model(&models.WebSocketSubscription{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("CONN#%s", connectionID)).
		All(&subscriptions)

	if err != nil {
		if errors.IsNotFound(err) || isResourceNotFound(err) {
			return nil // No subscriptions to delete or index/table unavailable yet
		}
		return ErrorHandler.HandleQueryError(err, "websocket subscription", "all subscriptions for connection")
	}

	// Delete each subscription
	for _, subscription := range subscriptions {
		if err := r.DeleteSubscription(ctx, subscription.ConnectionID, subscription.Stream); err != nil {
			r.logger.Warn("failed to delete subscription",
				zap.String("connection_id", subscription.ConnectionID),
				zap.String("stream", subscription.Stream),
				zap.Error(err),
			)
		}
	}

	return nil
}

// GetConnectionsByUser gets all connections for a user
func (r *StreamingConnectionRepository) GetConnectionsByUser(ctx context.Context, userID string) ([]models.WebSocketConnection, error) {
	var connections []models.WebSocketConnection

	err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("USER#%s", userID)).
		Limit(connectionQueryLimit).
		All(&connections)

	if err != nil {
		if errors.IsNotFound(err) || isResourceNotFound(err) {
			return []models.WebSocketConnection{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, "streaming connection", "connections by user")
	}

	if len(connections) == connectionQueryLimit {
		r.logger.Warn("connections by user query reached limit; results may be truncated",
			zap.String("user_id", userID),
			zap.Int("limit", connectionQueryLimit))
	}

	return connections, nil
}

// GetSubscriptionsForStream gets all subscriptions for a specific stream
func (r *StreamingConnectionRepository) GetSubscriptionsForStream(ctx context.Context, stream string) ([]models.WebSocketSubscription, error) {
	pk := fmt.Sprintf("SUB#%s", stream)

	const subscriptionChunkLimit = 200

	var (
		allSubscriptions []*models.WebSocketSubscription
		cursor           string
	)

	for {
		page, err := r.subscriptionRepo.FindWithPagination(ctx, pk, BasePaginationOptions{
			Limit:  subscriptionChunkLimit,
			Cursor: cursor,
			Order:  SortOrderAsc,
		})
		if err != nil {
			return nil, ErrorHandler.HandleQueryError(err, "websocket subscription", "subscriptions for stream")
		}

		allSubscriptions = append(allSubscriptions, page.Items...)

		if page.NextCursor == "" || len(page.Items) == 0 {
			break
		}
		cursor = page.NextCursor
	}

	// Convert from pointers to values for backward compatibility
	result := make([]models.WebSocketSubscription, len(allSubscriptions))
	for i, sub := range allSubscriptions {
		result[i] = *sub
	}

	return result, nil
}

// GetIdleConnections gets WebSocket connections that have been idle past the threshold
func (r *StreamingConnectionRepository) GetIdleConnections(ctx context.Context, idleThreshold time.Time) ([]models.WebSocketConnection, error) {
	r.logger.Info("scanning for idle WebSocket connections",
		zap.Time("idle_threshold", idleThreshold))

	// Strategy: Scan all connections and filter by LastActivity
	// Note: In production, you might want to implement pagination for large datasets
	var allConnections []models.WebSocketConnection
	err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Scan(&allConnections)
	if err != nil {
		r.logger.Error("failed to scan WebSocket connections",
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "streaming connection", "idle connections scan")
	}

	// Filter connections that are idle (LastActivity before threshold)
	var idleConnections []models.WebSocketConnection
	now := time.Now()

	for _, conn := range allConnections {
		if conn.LastActivity.Before(idleThreshold) {
			idleConnections = append(idleConnections, conn)

			r.logger.Debug("found idle connection",
				zap.String("connection_id", conn.ConnectionID),
				zap.String("user_id", conn.UserID),
				zap.String("username", conn.Username),
				zap.Time("last_activity", conn.LastActivity),
				zap.Duration("idle_duration", now.Sub(conn.LastActivity)))
		}
	}

	r.logger.Info("idle connection scan completed",
		zap.Int("total_connections", len(allConnections)),
		zap.Int("idle_connections", len(idleConnections)))

	return idleConnections, nil
}

// MarkConnectionsIdle marks inactive connections as idle
func (r *StreamingConnectionRepository) MarkConnectionsIdle(ctx context.Context, idleThreshold time.Duration) (int, error) {
	// Get all connected connections
	connections, err := r.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "streaming connection", "connected connections")
	}

	markedCount := 0
	now := time.Now()

	for _, conn := range connections {
		// Check if connection has been inactive
		if now.Sub(conn.LastActivity) > idleThreshold {
			conn.UpdateState(models.ConnectionStateIdle)
			if err := r.UpdateConnection(ctx, &conn); err != nil {
				r.logger.Error("failed to mark connection as idle",
					zap.String("connection_id", conn.ConnectionID),
					zap.Error(err))
				continue
			}
			markedCount++
		}
	}

	r.logger.Info("marked connections as idle",
		zap.Int("marked_count", markedCount),
		zap.Duration("idle_threshold", idleThreshold))

	return markedCount, nil
}

// CloseTimedOutConnections closes connections that have exceeded their idle timeout
func (r *StreamingConnectionRepository) CloseTimedOutConnections(ctx context.Context) (int, error) {
	// Get all idle connections
	idleConnections, err := r.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "streaming connection", "idle connections for close timeout")
	}

	closedCount := 0
	now := time.Now()

	for _, conn := range idleConnections {
		// Check if connection has exceeded its idle timeout
		idleDuration := now.Sub(conn.LastActivity)
		if idleDuration > conn.IdleTimeout {
			conn.UpdateState(models.ConnectionStateClosing)
			conn.CloseReason = fmt.Sprintf("Idle timeout after %v", idleDuration)
			conn.CloseCode = 1001 // Going Away

			if err := r.UpdateConnection(ctx, &conn); err != nil {
				r.logger.Error("failed to mark connection as closing",
					zap.String("connection_id", conn.ConnectionID),
					zap.Error(err))
				continue
			}
			closedCount++
		}
	}

	r.logger.Info("marked idle connections for closing",
		zap.Int("closed_count", closedCount))

	return closedCount, nil
}

// Connection Pooling and Resource Management

// checkConnectionLimits enforces connection pool limits
func (r *StreamingConnectionRepository) checkConnectionLimits(ctx context.Context, userID string) error {
	// Check user connection limit
	userConnectionCount, err := r.GetActiveConnectionsCount(ctx, userID)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, "streaming connection", "user connection count")
	}

	if userConnectionCount >= MaxConnectionsPerUser {
		return ErrorHandler.HandleCreateError(fmt.Errorf("%w: %s (%d)", ErrStreamingConnectionUserLimitReached, userID, MaxConnectionsPerUser), "streaming connection", "user connection limit")
	}

	// Check global connection limit
	totalConnections, err := r.GetTotalActiveConnectionsCount(ctx)
	if err != nil {
		return ErrorHandler.HandleQueryError(err, "streaming connection", "total connection count")
	}

	if totalConnections >= MaxTotalConnections {
		return ErrorHandler.HandleCreateError(fmt.Errorf("%w: (%d)", ErrStreamingConnectionGlobalLimitReached, MaxTotalConnections), "streaming connection", "global connection limit")
	}

	return nil
}

// GetTotalActiveConnectionsCount gets the total count of active connections across all users
func (r *StreamingConnectionRepository) GetTotalActiveConnectionsCount(ctx context.Context) (int, error) {
	connectedCount, err := r.GetConnectionCountByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return 0, err
	}

	idleCount, err := r.GetConnectionCountByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return 0, err
	}

	return connectedCount + idleCount, nil
}

// EnforceResourceLimits enforces resource limits on connections
func (r *StreamingConnectionRepository) EnforceResourceLimits(ctx context.Context, connectionID string, messageSize int64) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	// Check message size limit
	if messageSize > connection.MaxMessageSize {
		return ErrorHandler.HandleCreateError(fmt.Errorf("%w: %d exceeds limit %d", ErrStreamingConnectionMessageSizeExceeded, messageSize, connection.MaxMessageSize), "streaming connection", "message size limit")
	}

	// Check rate limit
	now := time.Now()
	if now.After(connection.RateLimitReset) {
		// Reset rate limit counter
		connection.CurrentRate = 0
		connection.RateLimitReset = now.Add(time.Minute)
	}

	if connection.CurrentRate >= connection.RateLimit {
		return ErrorHandler.HandleCreateError(fmt.Errorf("%w: %d messages per minute", ErrStreamingConnectionRateLimitExceeded, connection.RateLimit), "streaming connection", "rate limit")
	}

	return nil
}

// GetConnectionPool returns current connection pool statistics
func (r *StreamingConnectionRepository) GetConnectionPool(ctx context.Context) (map[string]interface{}, error) {
	totalActive, err := r.GetTotalActiveConnectionsCount(ctx)
	if err != nil {
		return nil, err
	}

	connected, err := r.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return nil, err
	}

	idle, err := r.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return nil, err
	}

	errorConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateError)
	if err != nil {
		return nil, err
	}

	closing, err := r.GetConnectionsByState(ctx, models.ConnectionStateClosing)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_active":    totalActive,
		"connected":       len(connected),
		"idle":            len(idle),
		"error":           len(errorConns),
		"closing":         len(closing),
		"max_per_user":    MaxConnectionsPerUser,
		"max_total":       MaxTotalConnections,
		"utilization_pct": float64(totalActive) / float64(MaxTotalConnections) * 100,
	}, nil
}

// ReclaimIdleConnections proactively closes old idle connections to free resources
func (r *StreamingConnectionRepository) ReclaimIdleConnections(ctx context.Context, maxIdleConnections int) (int, error) {
	idleConnections, err := r.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "streaming connection", "idle connections for reclaim")
	}

	if len(idleConnections) <= maxIdleConnections {
		return 0, nil // No need to reclaim
	}

	// Sort by last activity (oldest first)
	// Sort idle connections by last activity (oldest first) using a simple sort
	for i := 0; i < len(idleConnections)-1; i++ {
		for j := 0; j < len(idleConnections)-i-1; j++ {
			if idleConnections[j].LastActivity.After(idleConnections[j+1].LastActivity) {
				// Swap
				idleConnections[j], idleConnections[j+1] = idleConnections[j+1], idleConnections[j]
			}
		}
	}

	// Mark excess connections for closing
	excessCount := len(idleConnections) - maxIdleConnections
	reclaimedCount := 0

	for i := 0; i < excessCount && i < len(idleConnections); i++ {
		conn := &idleConnections[i]
		conn.UpdateState(models.ConnectionStateClosing)
		conn.CloseReason = "Resource reclamation - idle connection cleanup"
		conn.CloseCode = 1001 // Going Away

		if err := r.UpdateConnection(ctx, conn); err != nil {
			r.logger.Error("failed to mark connection for reclamation",
				zap.String("connection_id", conn.ConnectionID),
				zap.Error(err))
			continue
		}
		reclaimedCount++
	}

	r.logger.Info("reclaimed idle connections",
		zap.Int("reclaimed_count", reclaimedCount),
		zap.Int("total_idle", len(idleConnections)),
		zap.Int("max_allowed", maxIdleConnections))

	return reclaimedCount, nil
}

// GetStaleConnections gets WebSocket connections that are considered stale (very old with no recent activity)
func (r *StreamingConnectionRepository) GetStaleConnections(ctx context.Context, staleThreshold time.Time) ([]models.WebSocketConnection, error) {
	r.logger.Info("scanning for stale WebSocket connections",
		zap.Time("stale_threshold", staleThreshold))

	// Strategy: Scan all connections and filter by LastActivity and TTL expiration
	var allConnections []models.WebSocketConnection
	err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Scan(&allConnections)
	if err != nil {
		r.logger.Error("failed to scan WebSocket connections for stale detection",
			zap.Error(err))
		return nil, ErrorHandler.HandleQueryError(err, "streaming connection", "stale connections scan")
	}

	// Filter connections that are stale (very old LastActivity, expired TTL, or both)
	var staleConnections []models.WebSocketConnection
	now := time.Now()
	currentUnixTime := now.Unix()

	for _, conn := range allConnections {
		isStale := false

		// Connection is stale if LastActivity is before the stale threshold
		if conn.LastActivity.Before(staleThreshold) {
			isStale = true
		}

		// Connection is also stale if TTL has expired
		if conn.TTL > 0 && currentUnixTime > conn.TTL {
			isStale = true
		}

		if isStale {
			staleConnections = append(staleConnections, conn)

			r.logger.Debug("found stale connection",
				zap.String("connection_id", conn.ConnectionID),
				zap.String("user_id", conn.UserID),
				zap.String("username", conn.Username),
				zap.Time("last_activity", conn.LastActivity),
				zap.Int64("ttl", conn.TTL),
				zap.Bool("ttl_expired", conn.TTL > 0 && currentUnixTime > conn.TTL),
				zap.Duration("stale_duration", now.Sub(conn.LastActivity)))
		}
	}

	r.logger.Info("stale connection scan completed",
		zap.Int("total_connections", len(allConnections)),
		zap.Int("stale_connections", len(staleConnections)))

	return staleConnections, nil
}

// UpdateConnectionActivity updates the last activity timestamp for a connection
func (r *StreamingConnectionRepository) UpdateConnectionActivity(ctx context.Context, connectionID string) error {
	// Get the existing connection first
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	// Update last activity
	connection.LastActivity = time.Now()

	// If connection is idle, move it to connected
	if connection.State == models.ConnectionStateIdle {
		connection.UpdateState(models.ConnectionStateConnected)
	}

	// Update the connection
	return r.UpdateConnection(ctx, connection)
}

// RecordConnectionMessage records message statistics and updates activity
func (r *StreamingConnectionRepository) RecordConnectionMessage(ctx context.Context, connectionID string, sent bool, messageSize int64) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	// Record the message
	connection.RecordMessage(sent, messageSize)

	// Update rate limiting
	now := time.Now()
	if now.After(connection.RateLimitReset) {
		connection.CurrentRate = 0
		connection.RateLimitReset = now.Add(time.Minute)
	}
	connection.CurrentRate++

	// Update connection state if needed
	if connection.State == models.ConnectionStateIdle {
		connection.UpdateState(models.ConnectionStateConnected)
	}

	return r.UpdateConnection(ctx, connection)
}

// RecordConnectionError records an error for a connection
func (r *StreamingConnectionRepository) RecordConnectionError(ctx context.Context, connectionID string, errorMsg string) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	// Record the error
	connection.IncrementError(errorMsg)

	// If too many errors, mark as error state
	if connection.Metrics.ErrorCount >= 10 {
		connection.UpdateState(models.ConnectionStateError)
		connection.CloseReason = "Too many errors"
	}

	return r.UpdateConnection(ctx, connection)
}

// RecordPing records a ping for a connection
func (r *StreamingConnectionRepository) RecordPing(ctx context.Context, connectionID string) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	connection.RecordPing()
	return r.UpdateConnection(ctx, connection)
}

// RecordPong records a pong for a connection
func (r *StreamingConnectionRepository) RecordPong(ctx context.Context, connectionID string) error {
	connection, err := r.GetConnection(ctx, connectionID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "streaming connection", connectionID)
	}

	connection.RecordPong()
	return r.UpdateConnection(ctx, connection)
}

// GetActiveConnectionsCount gets the count of active connections for a user
func (r *StreamingConnectionRepository) GetActiveConnectionsCount(ctx context.Context, userID string) (int, error) {
	return r.GetUserConnectionCount(ctx, userID)
}

// GetConnectionCountByState returns the number of connections currently recorded in the provided state
func (r *StreamingConnectionRepository) GetConnectionCountByState(ctx context.Context, state models.ConnectionState) (int, error) {
	count, err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("STATE#%s", state)).
		Count()
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		if isResourceNotFound(err) {
			if r.logger != nil {
				r.logger.Warn("streaming connections state index missing; treating count as zero",
					zap.String("index", "gsi2"),
					zap.Error(err))
			}
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, "streaming connection", fmt.Sprintf("connection count state %s", state))
	}

	return int(count), nil
}

// GetUserConnectionCount returns the number of connections associated with the supplied user
func (r *StreamingConnectionRepository) GetUserConnectionCount(ctx context.Context, userID string) (int, error) {
	count, err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Index("gsi1").
		Where("gsi1PK", "=", fmt.Sprintf("USER#%s", userID)).
		Count()
	if err != nil {
		if errors.IsNotFound(err) {
			return 0, nil
		}
		if isResourceNotFound(err) {
			if r.logger != nil {
				r.logger.Warn("streaming connections user index missing; treating count as zero",
					zap.String("index", "gsi1"),
					zap.Error(err))
			}
			return 0, nil
		}
		return 0, ErrorHandler.HandleQueryError(err, "streaming connection", "user connection count")
	}

	return int(count), nil
}

// GetConnectionsByState gets all connections in a specific state
func (r *StreamingConnectionRepository) GetConnectionsByState(ctx context.Context, state models.ConnectionState) ([]models.WebSocketConnection, error) {
	var connections []models.WebSocketConnection

	err := r.GetDB().WithContext(ctx).Model(&models.WebSocketConnection{}).
		Index("gsi2").
		Where("gsi2PK", "=", fmt.Sprintf("STATE#%s", state)).
		Limit(connectionQueryLimit).
		All(&connections)

	if err != nil {
		if errors.IsNotFound(err) {
			return []models.WebSocketConnection{}, nil
		}
		if isResourceNotFound(err) {
			if r.logger != nil {
				r.logger.Warn("streaming connections state index missing; returning empty set",
					zap.String("index", "gsi2"),
					zap.Error(err))
			}
			return []models.WebSocketConnection{}, nil
		}
		return nil, ErrorHandler.HandleQueryError(err, "streaming connection", "connections by state")
	}

	if len(connections) == connectionQueryLimit {
		r.logger.Warn("connections by state query reached limit; results may be truncated",
			zap.String("state", string(state)),
			zap.Int("limit", connectionQueryLimit))
	}

	return connections, nil
}

func isResourceNotFound(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	markers := []string{
		"requested resource not found",
		"does not have the specified index",
		"index not found",
		"index: gsi",
	}
	for _, marker := range markers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// GetHealthyConnections gets all healthy connections
func (r *StreamingConnectionRepository) GetHealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	connectedConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return nil, err
	}

	idleConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return nil, err
	}

	// Combine and filter for health
	allConns := append(connectedConns, idleConns...)
	healthyConns := make([]models.WebSocketConnection, 0)

	for _, conn := range allConns {
		if conn.IsHealthy() {
			healthyConns = append(healthyConns, conn)
		}
	}

	return healthyConns, nil
}

// GetUnhealthyConnections gets connections that need attention
func (r *StreamingConnectionRepository) GetUnhealthyConnections(ctx context.Context) ([]models.WebSocketConnection, error) {
	errorConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateError)
	if err != nil {
		return nil, err
	}

	// Also check connected/idle connections with high error counts
	connectedConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateConnected)
	if err != nil {
		return nil, err
	}

	idleConns, err := r.GetConnectionsByState(ctx, models.ConnectionStateIdle)
	if err != nil {
		return nil, err
	}

	unhealthyConns := errorConns // Start with error connections

	// Add connected/idle connections that are unhealthy
	allActiveConns := append(connectedConns, idleConns...)
	for _, conn := range allActiveConns {
		if !conn.IsHealthy() {
			unhealthyConns = append(unhealthyConns, conn)
		}
	}

	return unhealthyConns, nil
}

// CleanupExpiredConnections removes connections that have exceeded their TTL
// This is typically handled by DynamoDB TTL, but can be called manually for immediate cleanup
func (r *StreamingConnectionRepository) CleanupExpiredConnections(_ context.Context) (int, error) {
	// DynamoDB TTL automatically handles cleanup of expired connections
	// This method is kept for backward compatibility but TTL does the actual work
	r.logger.Info("CleanupExpiredConnections called but DynamoDB TTL handles cleanup automatically")

	// Return 0 since TTL cleanup happens automatically and we don't track count
	return 0, nil
}

// GetDB returns the main database connection for direct access
func (r *StreamingConnectionRepository) GetDB() core.DB {
	return r.BaseRepository.GetDB()
}
