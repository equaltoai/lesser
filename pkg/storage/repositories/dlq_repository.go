package repositories

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

// DLQ message status constants
const (
	DLQStatusNew          = "new"
	DLQStatusReprocessing = "reprocessing"
	DLQStatusResolved     = "resolved"
	DLQStatusFailed       = "failed"
	DLQStatusAbandoned    = "abandoned"
)

// DLQRepository handles dead letter queue message operations using enhanced patterns
type DLQRepository struct {
	*EnhancedBaseRepository[*models.DLQMessage]
}

// NewDLQRepository creates a new DLQ repository with enhanced functionality
func NewDLQRepository(db core.DB, tableName string, logger *zap.Logger, costService *cost.TrackingService) *DLQRepository {
	// Create enhanced repository optimized for DLQ operations
	enhancedRepo := NewEnhancedBaseRepository[*models.DLQMessage](db, tableName, logger, costService, "DLQRepository", "dlq")

	// Set up enhanced services for DLQ operations
	enhancedRepo.SetValidationService(NewDefaultValidationService())
	enhancedRepo.SetPermissionService(NewDefaultPermissionService())
	enhancedRepo.SetCachingService(NewInMemoryCachingService()) // DLQ messages cached for retry logic
	enhancedRepo.SetEventService(NewDefaultEventService())      // Critical for DLQ monitoring

	return &DLQRepository{
		EnhancedBaseRepository: enhancedRepo,
	}
}

// NewDLQRepositorySimple creates a new DLQ repository without cost tracking (backward compatibility)
func NewDLQRepositorySimple(db core.DB, tableName string, logger *zap.Logger) *DLQRepository {
	return NewDLQRepository(db, tableName, logger, nil)
}

// CreateDLQMessage creates a new DLQ message using BaseRepository
func (r *DLQRepository) CreateDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	if err := message.BeforeCreate(); err != nil {
		return ErrorHandler.HandleCreateError(err, "dlq", "message preparation")
	}

	// Use enhanced repository for validation and creation
	err := r.ValidateAndCreate(ctx, message)
	if err != nil {
		return ErrorHandler.HandleCreateError(err, "dlq", message.ID)
	}

	r.logger.Info("created DLQ message",
		zap.String("id", message.ID),
		zap.String("service", message.Service),
		zap.String("error_type", message.ErrorType),
		zap.String("status", message.Status),
	)

	return nil
}

// GetDLQMessage retrieves a DLQ message by ID (complex query - needs custom implementation)
func (r *DLQRepository) GetDLQMessage(ctx context.Context, id string) (*models.DLQMessage, error) {
	var message models.DLQMessage
	err := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Filter("ID", "=", id).
		First(&message)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, ErrorHandler.HandleGetError(storage.ErrNotFound, EntityDLQMessage, id)
		}
		return nil, ErrorHandler.HandleGetError(err, EntityDLQMessage, id)
	}

	return &message, nil
}

// GetDLQMessagesByService retrieves DLQ messages for a specific service with pagination (DLQ-specific business logic)
func (r *DLQRepository) GetDLQMessagesByService(ctx context.Context, service string, date time.Time, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	dateStr := date.Format(common.CompactDateFormat)
	pk := fmt.Sprintf("DLQ#%s#%s", service, dateStr)

	// Use BaseRepository for paginated query with business logic
	opts := BasePaginationOptions{
		Limit:  limit,
		Cursor: cursor,
		Order:  "DESC", // Most recent first
	}

	result, err := r.FindWithPagination(ctx, pk, opts)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "dlq", "service messages")
	}

	return result.Items, result.NextCursor, nil
}

// GetDLQMessagesByServiceDateRange retrieves DLQ messages for a service across multiple dates
func (r *DLQRepository) GetDLQMessagesByServiceDateRange(ctx context.Context, service string, startDate, endDate time.Time, limit int) ([]*models.DLQMessage, error) {
	var allMessages []*models.DLQMessage
	currentDate := startDate

	for currentDate.Before(endDate) || currentDate.Equal(endDate) {
		messages, _, err := r.GetDLQMessagesByService(ctx, service, currentDate, limit, "")
		if err != nil {
			r.logger.Warn("failed to get DLQ messages for date",
				zap.String("service", service),
				zap.String("date", currentDate.Format(common.DateFormat)),
				zap.Error(err),
			)
		} else {
			allMessages = append(allMessages, messages...)
		}

		currentDate = currentDate.Add(24 * time.Hour)

		// Break if we have enough messages
		if len(allMessages) >= limit {
			break
		}
	}

	// Sort by timestamp (newest first) and limit
	sort.Slice(allMessages, func(i, j int) bool {
		return allMessages[i].FirstSeenAt.After(allMessages[j].FirstSeenAt)
	})

	if err := common.ValidateSliceLength("allMessages", allMessages, limit); err == nil {
		allMessages = allMessages[:limit]
	}

	return allMessages, nil
}

// GetDLQMessagesByErrorType retrieves DLQ messages by error type with pagination (DLQ-specific business logic)
func (r *DLQRepository) GetDLQMessagesByErrorType(ctx context.Context, errorType string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	query := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Index("error-index").
		Where("GSI1PK", "=", "DLQ_ERROR#"+errorType).
		OrderBy("GSI1SK", "DESC")

	if cursor != "" {
		query = query.Where("GSI1SK", "<", cursor)
	}

	// Get one more item than requested to determine if there are more results
	query = query.Limit(limit + 1)

	var messages []*models.DLQMessage
	err := query.All(&messages)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "dlq", "error type messages")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("messages", messages, limit); err == nil {
		// We got more results than requested, so there are more pages
		nextCursor = messages[limit-1].GSI1SK
		messages = messages[:limit] // Trim to requested limit
	}

	return messages, nextCursor, nil
}

// GetDLQMessagesForReprocessing retrieves messages that can be reprocessed (DLQ-specific retry logic)
func (r *DLQRepository) GetDLQMessagesForReprocessing(ctx context.Context, service string, status string, limit int) ([]*models.DLQMessage, error) {
	query := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Index("retry-index").
		Where("GSI2PK", "=", fmt.Sprintf("DLQ_RETRY#%s#%s", service, status)).
		OrderBy("GSI2SK", "ASC"). // Oldest first for reprocessing
		Limit(limit)

	var messages []*models.DLQMessage
	err := query.All(&messages)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "dlq", "reprocessing messages")
	}

	// Filter to only include messages that can actually be reprocessed - DLQ business logic
	var reprocessableMessages []*models.DLQMessage
	for _, message := range messages {
		if message.CanReprocess() {
			reprocessableMessages = append(reprocessableMessages, message)
		}
	}

	return reprocessableMessages, nil
}

// GetDLQMessagesByStatus retrieves messages by status (DLQ-specific status querying)
func (r *DLQRepository) GetDLQMessagesByStatus(ctx context.Context, service, status string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	query := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Index("retry-index").
		Where("GSI2PK", "=", fmt.Sprintf("DLQ_RETRY#%s#%s", service, status)).
		OrderBy("GSI2SK", "DESC")

	if cursor != "" {
		query = query.Where("GSI2SK", "<", cursor)
	}

	query = query.Limit(limit + 1)

	var messages []*models.DLQMessage
	err := query.All(&messages)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "dlq", "status messages")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("messages", messages, limit); err == nil {
		nextCursor = messages[limit-1].GSI2SK
		messages = messages[:limit]
	}

	return messages, nextCursor, nil
}

// UpdateDLQMessage updates an existing DLQ message using BaseRepository
func (r *DLQRepository) UpdateDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	if err := message.BeforeUpdate(); err != nil {
		return ErrorHandler.HandleUpdateError(err, "dlq", "message preparation")
	}

	// Delegate to BaseRepository for CRUD operation
	err := r.Update(ctx, message)
	if err != nil {
		return ErrorHandler.HandleUpdateError(err, "dlq", message.ID)
	}

	r.logger.Info("updated DLQ message",
		zap.String("id", message.ID),
		zap.String("status", message.Status),
		zap.Int("reprocessing_count", message.ReprocessingCount),
	)

	return nil
}

// DeleteDLQMessage deletes a DLQ message using BaseRepository
func (r *DLQRepository) DeleteDLQMessage(ctx context.Context, message *models.DLQMessage) error {
	// Delegate to BaseRepository for CRUD operation
	err := r.Delete(ctx, message.GetPK(), message.GetSK())
	if err != nil {
		return ErrorHandler.HandleDeleteError(err, "dlq", message.ID)
	}

	r.logger.Info("deleted DLQ message",
		zap.String("id", message.ID),
		zap.String("service", message.Service),
	)

	return nil
}

// BatchUpdateDLQMessages updates multiple DLQ messages
func (r *DLQRepository) BatchUpdateDLQMessages(ctx context.Context, messages []*models.DLQMessage) error {
	if err := common.ValidateSliceNotEmpty("messages", messages); err != nil {
		return nil
	}

	// Update each message individually (DynamORM doesn't have batch update)
	var errors []error
	for _, message := range messages {
		if err := r.UpdateDLQMessage(ctx, message); err != nil {
			errors = append(errors, err)
			r.logger.Warn("failed to update DLQ message in batch",
				zap.String("id", message.ID),
				zap.Error(err),
			)
		}
	}

	if err := common.ValidateSliceNotEmpty("errors", errors); err == nil {
		return ErrorHandler.HandleUpdateError(fmt.Errorf("%w: %d of %d messages failed", ErrDLQBatchUpdateFailed, len(errors), len(messages)), "dlq", "batch update")
	}

	return nil
}

// GetDLQAnalytics returns analytics data for DLQ messages
func (r *DLQRepository) GetDLQAnalytics(ctx context.Context, service string, timeRange DLQTimeRange) (*DLQAnalytics, error) {
	analytics := &DLQAnalytics{
		Service:          service,
		TimeRange:        timeRange,
		ErrorTypeStats:   make(map[string]*DLQErrorTypeStats),
		ServiceStats:     make(map[string]*DLQServiceStats),
		SimilarityGroups: make(map[string]*DLQSimilarityGroup),
	}

	// Get messages for the time range
	messages, err := r.GetDLQMessagesByServiceDateRange(ctx, service, timeRange.StartTime, timeRange.EndTime, 1000)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "dlq", "analytics messages")
	}

	// Process messages for analytics
	for _, message := range messages {
		analytics.TotalMessages++
		analytics.TotalCostMicroCents += message.GetTotalCost()

		// Status counts
		switch message.Status {
		case DLQStatusNew:
			analytics.NewMessages++
		case DLQStatusReprocessing:
			analytics.ReprocessingMessages++
		case DLQStatusResolved:
			analytics.ResolvedMessages++
		case DLQStatusFailed:
			analytics.FailedMessages++
		case DLQStatusAbandoned:
			analytics.AbandonedMessages++
		}

		// Error type statistics
		if stats, exists := analytics.ErrorTypeStats[message.ErrorType]; exists {
			stats.Count++
			stats.TotalCostMicroCents += message.GetTotalCost()
			if message.Status == "resolved" {
				stats.ResolvedCount++
			}
		} else {
			analytics.ErrorTypeStats[message.ErrorType] = &DLQErrorTypeStats{
				ErrorType:           message.ErrorType,
				Count:               1,
				ResolvedCount:       0,
				TotalCostMicroCents: message.GetTotalCost(),
			}
			if message.Status == "resolved" {
				analytics.ErrorTypeStats[message.ErrorType].ResolvedCount = 1
			}
		}

		// Similarity grouping
		if group, exists := analytics.SimilarityGroups[message.SimilarityHash]; exists {
			group.MessageCount++
			group.MessageIDs = append(group.MessageIDs, message.ID)
			if message.FirstSeenAt.Before(group.FirstSeen) {
				group.FirstSeen = message.FirstSeenAt
			}
			if message.FirstSeenAt.After(group.LastSeen) {
				group.LastSeen = message.FirstSeenAt
			}
		} else {
			analytics.SimilarityGroups[message.SimilarityHash] = &DLQSimilarityGroup{
				SimilarityHash: message.SimilarityHash,
				ErrorType:      message.ErrorType,
				Service:        message.Service,
				MessageCount:   1,
				MessageIDs:     []string{message.ID},
				FirstSeen:      message.FirstSeenAt,
				LastSeen:       message.FirstSeenAt,
				SampleError:    message.ErrorMessage,
			}
		}
	}

	// Calculate rates
	if analytics.TotalMessages > 0 {
		analytics.ResolutionRate = float64(analytics.ResolvedMessages) / float64(analytics.TotalMessages) * 100
		analytics.AbandonmentRate = float64(analytics.AbandonedMessages) / float64(analytics.TotalMessages) * 100
	}

	// Calculate costs
	analytics.TotalCostDollars = float64(analytics.TotalCostMicroCents) / 1_000_000.0
	if analytics.TotalMessages > 0 {
		analytics.AverageCostPerMessage = analytics.TotalCostDollars / float64(analytics.TotalMessages)
	}

	return analytics, nil
}

// GetDLQTrends returns trend data for DLQ messages over time
func (r *DLQRepository) GetDLQTrends(ctx context.Context, service string, days int) (*DLQTrends, error) {
	trends := &DLQTrends{
		Service:    service,
		Days:       days,
		DailyStats: make(map[string]*DLQDailyStats),
	}

	endDate := time.Now()
	startDate := endDate.Add(-time.Duration(days) * 24 * time.Hour)

	// Get messages for each day
	for d := 0; d < days; d++ {
		date := startDate.Add(time.Duration(d) * 24 * time.Hour)
		dateStr := date.Format(common.DateFormat)

		messages, _, err := r.GetDLQMessagesByService(ctx, service, date, 1000, "")
		if err != nil {
			r.logger.Warn("failed to get DLQ messages for trend analysis",
				zap.String("service", service),
				zap.String("date", dateStr),
				zap.Error(err),
			)
			continue
		}

		dailyStats := &DLQDailyStats{
			Date:         date,
			MessageCount: len(messages),
			ErrorTypes:   make(map[string]int),
			StatusCounts: make(map[string]int),
		}

		var totalCost int64
		for _, message := range messages {
			totalCost += message.GetTotalCost()

			// Count error types
			dailyStats.ErrorTypes[message.ErrorType]++

			// Count statuses
			dailyStats.StatusCounts[message.Status]++
		}

		dailyStats.TotalCostMicroCents = totalCost
		dailyStats.TotalCostDollars = float64(totalCost) / 1_000_000.0

		trends.DailyStats[dateStr] = dailyStats
	}

	return trends, nil
}

// SearchDLQMessages searches DLQ messages with various filters (DLQ-specific search functionality)
func (r *DLQRepository) SearchDLQMessages(ctx context.Context, filter *DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	if err := common.ValidateRequiredParam("filter.Service", filter.Service); err != nil {
		return nil, "", ErrorHandler.HandleQueryError(ErrDLQServiceRequired, "dlq", "search validation")
	}

	// Use service-wide index for broad searches
	query := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Index("service-index").
		Where("GSI3PK", "=", "DLQ_SERVICE#"+filter.Service).
		OrderBy("GSI3SK", "DESC")

	// Apply filters
	if filter.ErrorType != "" {
		query = query.Filter("ErrorType", "=", filter.ErrorType)
	}

	if filter.Status != "" {
		query = query.Filter("Status", "=", filter.Status)
	}

	if filter.Priority != "" {
		query = query.Filter("Priority", "=", filter.Priority)
	}

	if filter.IsPermanent != nil {
		query = query.Filter("IsPermanent", "=", *filter.IsPermanent)
	}

	if !filter.StartTime.IsZero() {
		query = query.Filter("FirstSeenAt", ">=", filter.StartTime.Unix())
	}

	if !filter.EndTime.IsZero() {
		query = query.Filter("FirstSeenAt", "<=", filter.EndTime.Unix())
	}

	// Continue scanning from the previous page when a cursor is supplied
	if filter.Cursor != "" {
		query = query.Where("GSI3SK", "<", filter.Cursor)
	}

	// Set limit
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	query = query.Limit(limit + 1)

	var messages []*models.DLQMessage
	err := query.All(&messages)
	if err != nil {
		return nil, "", ErrorHandler.HandleQueryError(err, "dlq", "search messages")
	}

	// Generate next cursor
	var nextCursor string
	if err := common.ValidateSliceLength("messages", messages, limit); err == nil {
		nextCursor = messages[limit-1].GSI3SK
		messages = messages[:limit]
	}

	// Apply text search if specified - DLQ business logic
	if filter.SearchText != "" {
		messages = r.filterByText(messages, filter.SearchText)
	}

	return messages, nextCursor, nil
}

// filterByText performs in-memory text filtering (DLQ-specific search logic)
func (r *DLQRepository) filterByText(messages []*models.DLQMessage, searchText string) []*models.DLQMessage {
	searchText = strings.ToLower(searchText)
	var filtered []*models.DLQMessage

	for _, message := range messages {
		if r.messageMatchesText(message, searchText) {
			filtered = append(filtered, message)
		}
	}

	return filtered
}

// messageMatchesText checks if a message matches the search text (DLQ-specific text matching)
func (r *DLQRepository) messageMatchesText(message *models.DLQMessage, searchText string) bool {
	fields := []string{
		strings.ToLower(message.ErrorMessage),
		strings.ToLower(message.FailureReason),
		strings.ToLower(message.FunctionName),
		strings.ToLower(message.MessageBody),
	}

	for _, field := range fields {
		if strings.Contains(field, searchText) {
			return true
		}
	}

	return false
}

// CleanupExpiredMessages deletes expired DLQ messages (DLQ-specific cleanup logic)
func (r *DLQRepository) CleanupExpiredMessages(ctx context.Context, before time.Time) (int, error) {
	// DynamoDB TTL should handle most cleanup, but this provides manual cleanup
	var expiredMessages []*models.DLQMessage
	err := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Filter("ExpiresAt", "<", before.Unix()).
		Limit(100). // Process in batches
		All(&expiredMessages)

	if err != nil {
		return 0, ErrorHandler.HandleQueryError(err, "dlq", "expired messages")
	}

	if err := common.ValidateSliceNotEmpty("expiredMessages", expiredMessages); err != nil {
		return 0, nil
	}

	// Delete expired messages using BaseRepository
	deletedCount := 0
	for _, message := range expiredMessages {
		if err := r.DeleteDLQMessage(ctx, message); err != nil {
			r.logger.Warn("failed to delete expired DLQ message",
				zap.String("id", message.ID),
				zap.Error(err),
			)
		} else {
			deletedCount++
		}
	}

	r.logger.Info("cleaned up expired DLQ messages",
		zap.Int("deleted_count", deletedCount),
		zap.Int("total_found", len(expiredMessages)),
	)

	return deletedCount, nil
}

// GetSimilarMessages finds messages with the same similarity hash (DLQ-specific similarity analysis)
func (r *DLQRepository) GetSimilarMessages(ctx context.Context, similarityHash string, limit int) ([]*models.DLQMessage, error) {
	var messages []*models.DLQMessage
	err := r.GetDB().WithContext(ctx).Model(&models.DLQMessage{}).
		Filter("SimilarityHash", "=", similarityHash).
		OrderBy("FirstSeenAt", "DESC").
		Limit(limit).
		All(&messages)

	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "dlq", "similar messages")
	}

	return messages, nil
}

// ================= DLQ-SPECIFIC BUSINESS LOGIC METHODS =================

// SendToDeadLetterQueue creates and stores a DLQ message with proper error categorization
func (r *DLQRepository) SendToDeadLetterQueue(ctx context.Context, service, messageID, messageBody, errorType, errorMessage string, isPermanent bool) error {
	message := models.NewDLQMessageBuilder().
		ForService(service).
		WithOriginalMessage(messageID, messageBody).
		WithError(errorType, errorMessage, "").
		WithFailureReason("Message processing failed").
		Build()

	if isPermanent {
		message.IsPermanent = true
		message.Status = DLQStatusFailed
	} else {
		message.Status = DLQStatusNew
	}

	return r.CreateDLQMessage(ctx, message)
}

// RetryFailedMessage attempts to reprocess a DLQ message with exponential backoff
func (r *DLQRepository) RetryFailedMessage(ctx context.Context, messageID string) error {
	message, err := r.GetDLQMessage(ctx, messageID)
	if err != nil {
		return ErrorHandler.HandleGetError(err, "dlq", messageID)
	}
	if message == nil {
		return ErrorHandler.HandleGetError(fmt.Errorf("%w: %s", ErrDLQMessageNotFound, messageID), "dlq", messageID)
	}

	if !message.CanReprocess() {
		return ErrorHandler.HandleUpdateError(fmt.Errorf("%w: %s", ErrDLQMessageNotReprocessable, messageID), "dlq", messageID)
	}

	// Mark for reprocessing with exponential backoff
	message.MarkForReprocessing()

	if message.ShouldAbandon() {
		message.MarkAbandoned()
		r.logger.Warn("abandoning DLQ message after max retry attempts",
			zap.String("id", messageID),
			zap.Int("retry_count", message.ReprocessingCount),
		)
	}

	return r.UpdateDLQMessage(ctx, message)
}

// AnalyzeFailurePatterns analyzes DLQ messages to identify common failure patterns
func (r *DLQRepository) AnalyzeFailurePatterns(ctx context.Context, service string, days int) (map[string]*DLQSimilarityGroup, error) {
	endDate := time.Now()
	startDate := endDate.Add(-time.Duration(days) * 24 * time.Hour)

	messages, err := r.GetDLQMessagesByServiceDateRange(ctx, service, startDate, endDate, 1000)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "dlq", "pattern analysis")
	}

	patterns := make(map[string]*DLQSimilarityGroup)
	for _, message := range messages {
		if group, exists := patterns[message.SimilarityHash]; exists {
			group.MessageCount++
			group.MessageIDs = append(group.MessageIDs, message.ID)
		} else {
			patterns[message.SimilarityHash] = &DLQSimilarityGroup{
				SimilarityHash: message.SimilarityHash,
				ErrorType:      message.ErrorType,
				Service:        message.Service,
				MessageCount:   1,
				MessageIDs:     []string{message.ID},
				FirstSeen:      message.FirstSeenAt,
				LastSeen:       message.FirstSeenAt,
				SampleError:    message.ErrorMessage,
			}
		}
	}

	return patterns, nil
}

// GetRetryableMessages returns messages that are ready for retry based on backoff schedule
func (r *DLQRepository) GetRetryableMessages(ctx context.Context, service string, limit int) ([]*models.DLQMessage, error) {
	// Get messages with status "new" or "reprocessing" that are ready for retry
	newMessages, err := r.GetDLQMessagesForReprocessing(ctx, service, DLQStatusNew, limit/2)
	if err != nil {
		r.logger.Warn("failed to get new messages for retry", zap.Error(err))
		newMessages = []*models.DLQMessage{}
	}

	retryMessages, err := r.GetDLQMessagesForReprocessing(ctx, service, DLQStatusReprocessing, limit/2)
	if err != nil {
		r.logger.Warn("failed to get reprocessing messages for retry", zap.Error(err))
		retryMessages = []*models.DLQMessage{}
	}

	// Combine and filter by actual retry readiness
	allMessages := append(newMessages, retryMessages...)
	var retryableMessages []*models.DLQMessage

	now := time.Now()
	for _, message := range allMessages {
		if message.NextRetryAt == nil || now.After(*message.NextRetryAt) {
			if message.CanReprocess() {
				retryableMessages = append(retryableMessages, message)
			}
		}
	}

	// Limit to requested count
	if len(retryableMessages) > limit {
		retryableMessages = retryableMessages[:limit]
	}

	return retryableMessages, nil
}

// MonitorDLQHealth provides health metrics for DLQ monitoring and alerting
func (r *DLQRepository) MonitorDLQHealth(ctx context.Context, service string) (*DLQHealthStatus, error) {
	// Get recent messages for health assessment
	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour) // Last hour

	messages, err := r.GetDLQMessagesByServiceDateRange(ctx, service, startTime, endTime, 500)
	if err != nil {
		return nil, ErrorHandler.HandleQueryError(err, "dlq", "health monitoring")
	}

	health := &DLQHealthStatus{
		Service:           service,
		CheckTime:         endTime,
		TotalMessages:     len(messages),
		NewMessages:       0,
		ReprocessingCount: 0,
		AbandonedCount:    0,
		ErrorRates:        make(map[string]int),
		AverageRetryCount: 0.0,
		IsHealthy:         true,
		Alerts:            []string{},
	}

	var totalRetries int
	for _, message := range messages {
		switch message.Status {
		case DLQStatusNew:
			health.NewMessages++
		case DLQStatusReprocessing:
			health.ReprocessingCount++
		case DLQStatusAbandoned:
			health.AbandonedCount++
		}

		health.ErrorRates[message.ErrorType]++
		totalRetries += message.ReprocessingCount
	}

	if len(messages) > 0 {
		health.AverageRetryCount = float64(totalRetries) / float64(len(messages))
	}

	// Health checks and alerting
	if health.TotalMessages > 100 {
		health.IsHealthy = false
		health.Alerts = append(health.Alerts, "High volume of DLQ messages in last hour")
	}

	if health.AbandonedCount > 10 {
		health.IsHealthy = false
		health.Alerts = append(health.Alerts, "High number of abandoned messages")
	}

	if health.AverageRetryCount > 2.5 {
		health.IsHealthy = false
		health.Alerts = append(health.Alerts, "High average retry count indicates systemic issues")
	}

	return health, nil
}

// Data structures for analytics and search

// DLQTimeRange represents a time range for analytics
type DLQTimeRange struct {
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
}

// DLQAnalytics represents analytics data for DLQ messages
type DLQAnalytics struct {
	Service               string                         `json:"service"`
	TimeRange             DLQTimeRange                   `json:"time_range"`
	TotalMessages         int                            `json:"total_messages"`
	NewMessages           int                            `json:"new_messages"`
	ReprocessingMessages  int                            `json:"reprocessing_messages"`
	ResolvedMessages      int                            `json:"resolved_messages"`
	FailedMessages        int                            `json:"failed_messages"`
	AbandonedMessages     int                            `json:"abandoned_messages"`
	ResolutionRate        float64                        `json:"resolution_rate"`
	AbandonmentRate       float64                        `json:"abandonment_rate"`
	TotalCostMicroCents   int64                          `json:"total_cost_micro_cents"`
	TotalCostDollars      float64                        `json:"total_cost_dollars"`
	AverageCostPerMessage float64                        `json:"average_cost_per_message"`
	ErrorTypeStats        map[string]*DLQErrorTypeStats  `json:"error_type_stats"`
	ServiceStats          map[string]*DLQServiceStats    `json:"service_stats"`
	SimilarityGroups      map[string]*DLQSimilarityGroup `json:"similarity_groups"`
}

// DLQErrorTypeStats represents statistics for a specific error type
type DLQErrorTypeStats struct {
	ErrorType           string  `json:"error_type"`
	Count               int     `json:"count"`
	ResolvedCount       int     `json:"resolved_count"`
	ResolutionRate      float64 `json:"resolution_rate"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
}

// DLQServiceStats represents statistics for a specific service
type DLQServiceStats struct {
	Service             string  `json:"service"`
	MessageCount        int     `json:"message_count"`
	ErrorTypes          int     `json:"error_types"`
	ResolutionRate      float64 `json:"resolution_rate"`
	TotalCostMicroCents int64   `json:"total_cost_micro_cents"`
}

// DLQSimilarityGroup represents a group of similar error messages
type DLQSimilarityGroup struct {
	SimilarityHash string    `json:"similarity_hash"`
	ErrorType      string    `json:"error_type"`
	Service        string    `json:"service"`
	MessageCount   int       `json:"message_count"`
	MessageIDs     []string  `json:"message_ids"`
	FirstSeen      time.Time `json:"first_seen"`
	LastSeen       time.Time `json:"last_seen"`
	SampleError    string    `json:"sample_error"`
}

// DLQTrends represents trend data over time
type DLQTrends struct {
	Service    string                    `json:"service"`
	Days       int                       `json:"days"`
	DailyStats map[string]*DLQDailyStats `json:"daily_stats"`
}

// DLQDailyStats represents statistics for a single day
type DLQDailyStats struct {
	Date                time.Time      `json:"date"`
	MessageCount        int            `json:"message_count"`
	TotalCostMicroCents int64          `json:"total_cost_micro_cents"`
	TotalCostDollars    float64        `json:"total_cost_dollars"`
	ErrorTypes          map[string]int `json:"error_types"`
	StatusCounts        map[string]int `json:"status_counts"`
}

// DLQSearchFilter represents search criteria for DLQ messages
type DLQSearchFilter struct {
	Service     string    `json:"service"`
	ErrorType   string    `json:"error_type,omitempty"`
	Status      string    `json:"status,omitempty"`
	Priority    string    `json:"priority,omitempty"`
	IsPermanent *bool     `json:"is_permanent,omitempty"`
	StartTime   time.Time `json:"start_time,omitempty"`
	EndTime     time.Time `json:"end_time,omitempty"`
	SearchText  string    `json:"search_text,omitempty"`
	Limit       int       `json:"limit,omitempty"`
	Cursor      string    `json:"cursor,omitempty"`
}

// DLQHealthStatus represents the health status of DLQ for monitoring
type DLQHealthStatus struct {
	Service           string         `json:"service"`
	CheckTime         time.Time      `json:"check_time"`
	TotalMessages     int            `json:"total_messages"`
	NewMessages       int            `json:"new_messages"`
	ReprocessingCount int            `json:"reprocessing_count"`
	AbandonedCount    int            `json:"abandoned_count"`
	ErrorRates        map[string]int `json:"error_rates"`
	AverageRetryCount float64        `json:"average_retry_count"`
	IsHealthy         bool           `json:"is_healthy"`
	Alerts            []string       `json:"alerts"`
}
