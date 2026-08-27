// Package inmemory provides thread-safe in-memory implementations of repository interfaces.
package inmemory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

// DLQStatus* values match the status stored on DLQ messages.
const (
	DLQStatusNew    = "new"
	DLQStatusFailed = "failed"
)

// DLQRepository is a thread-safe in-memory implementation of interfaces.DLQRepository.
type DLQRepository struct {
	mu       sync.RWMutex
	messages map[string]*models.DLQMessage // keyed by ID
}

// NewDLQRepository creates a new in-memory DLQ repository.
func NewDLQRepository() *DLQRepository {
	return &DLQRepository{
		messages: make(map[string]*models.DLQMessage),
	}
}

// ===== Core DLQ Operations =====

// CreateDLQMessage creates a new DLQ message.
func (r *DLQRepository) CreateDLQMessage(_ context.Context, message *models.DLQMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message == nil || message.ID == "" {
		return fmt.Errorf("message ID is required")
	}

	if _, exists := r.messages[message.ID]; exists {
		return storage.ErrAlreadyExists
	}

	r.messages[message.ID] = message
	return nil
}

// GetDLQMessage retrieves a DLQ message by ID.
func (r *DLQRepository) GetDLQMessage(_ context.Context, id string) (*models.DLQMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	message, exists := r.messages[id]
	if !exists {
		return nil, storage.ErrNotFound
	}
	return message, nil
}

// UpdateDLQMessage updates an existing DLQ message.
func (r *DLQRepository) UpdateDLQMessage(_ context.Context, message *models.DLQMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message == nil || message.ID == "" {
		return fmt.Errorf("message ID is required")
	}

	if _, exists := r.messages[message.ID]; !exists {
		return storage.ErrNotFound
	}

	r.messages[message.ID] = message
	return nil
}

// DeleteDLQMessage deletes a DLQ message.
func (r *DLQRepository) DeleteDLQMessage(_ context.Context, message *models.DLQMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if message == nil || message.ID == "" {
		return fmt.Errorf("message ID is required")
	}

	if _, exists := r.messages[message.ID]; !exists {
		return storage.ErrNotFound
	}

	delete(r.messages, message.ID)
	return nil
}

// BatchUpdateDLQMessages updates multiple DLQ messages.
func (r *DLQRepository) BatchUpdateDLQMessages(ctx context.Context, messages []*models.DLQMessage) error {
	for _, message := range messages {
		if err := r.UpdateDLQMessage(ctx, message); err != nil {
			return err
		}
	}
	return nil
}

// ===== Query Operations =====

// GetDLQMessagesByService retrieves DLQ messages for a specific service with pagination.
func (r *DLQRepository) GetDLQMessagesByService(_ context.Context, service string, date time.Time, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dateStr := date.Format("2006-01-02")
	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if msg.Service == service && msg.FirstSeenAt.Format("2006-01-02") == dateStr {
			results = append(results, msg)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	return applyPagination(results, limit, cursor)
}

// GetDLQMessagesByServiceDateRange retrieves DLQ messages for a service across multiple dates.
func (r *DLQRepository) GetDLQMessagesByServiceDateRange(_ context.Context, service string, startDate, endDate time.Time, limit int) ([]*models.DLQMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if msg.Service == service &&
			(msg.FirstSeenAt.Equal(startDate) || msg.FirstSeenAt.After(startDate)) &&
			(msg.FirstSeenAt.Equal(endDate) || msg.FirstSeenAt.Before(endDate)) {
			results = append(results, msg)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// GetDLQMessagesByErrorType retrieves DLQ messages by error type with pagination.
func (r *DLQRepository) GetDLQMessagesByErrorType(_ context.Context, errorType string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if msg.ErrorType == errorType {
			results = append(results, msg)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	return applyPagination(results, limit, cursor)
}

// GetDLQMessagesForReprocessing retrieves messages that can be reprocessed.
func (r *DLQRepository) GetDLQMessagesForReprocessing(_ context.Context, service string, status string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if msg.Service == service && msg.Status == status && msg.CanReprocess() {
			results = append(results, msg)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	return applyPagination(results, limit, cursor)
}

// GetDLQMessagesByStatus retrieves messages by status.
func (r *DLQRepository) GetDLQMessagesByStatus(_ context.Context, service, status string, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if msg.Service == service && msg.Status == status {
			results = append(results, msg)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	return applyPagination(results, limit, cursor)
}

// SearchDLQMessages searches DLQ messages with various filters.
func (r *DLQRepository) SearchDLQMessages(_ context.Context, filter *interfaces.DLQSearchFilter) ([]*models.DLQMessage, string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage

	for _, msg := range r.messages {
		if !matchesDLQFilter(msg, filter) {
			continue
		}
		results = append(results, msg)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FirstSeenAt.After(results[j].FirstSeenAt)
	})

	return applyPagination(results, filter.Limit, filter.Cursor)
}

// GetDLQAnalytics returns analytics data for DLQ messages.
func (r *DLQRepository) GetDLQAnalytics(_ context.Context, service string, timeRange interfaces.DLQTimeRange) (*interfaces.DLQAnalytics, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	analytics := &interfaces.DLQAnalytics{
		Service:          service,
		TimeRange:        timeRange,
		ErrorTypeStats:   make(map[string]*interfaces.DLQErrorTypeStats),
		ServiceStats:     make(map[string]*interfaces.DLQServiceStats),
		SimilarityGroups: make(map[string]*interfaces.DLQSimilarityGroup),
	}

	for _, msg := range r.messages {
		if msg.Service != service {
			continue
		}
		if msg.FirstSeenAt.Before(timeRange.StartTime) || msg.FirstSeenAt.After(timeRange.EndTime) {
			continue
		}

		analytics.TotalMessages++

		switch msg.Status {
		case DLQStatusNew:
			analytics.NewMessages++
		case "reprocessing":
			analytics.ReprocessingMessages++
		case "resolved":
			analytics.ResolvedMessages++
		case DLQStatusFailed:
			analytics.FailedMessages++
		case "abandoned":
			analytics.AbandonedMessages++
		}

		if _, exists := analytics.ErrorTypeStats[msg.ErrorType]; !exists {
			analytics.ErrorTypeStats[msg.ErrorType] = &interfaces.DLQErrorTypeStats{
				ErrorType: msg.ErrorType,
			}
		}
		analytics.ErrorTypeStats[msg.ErrorType].Count++
	}

	if analytics.TotalMessages > 0 {
		analytics.ResolutionRate = float64(analytics.ResolvedMessages) / float64(analytics.TotalMessages) * 100
		analytics.AbandonmentRate = float64(analytics.AbandonedMessages) / float64(analytics.TotalMessages) * 100
	}

	return analytics, nil
}

// GetDLQTrends returns trend data for DLQ messages over time.
func (r *DLQRepository) GetDLQTrends(_ context.Context, service string, days int) (*interfaces.DLQTrends, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	trends := &interfaces.DLQTrends{
		Service:    service,
		Days:       days,
		DailyStats: make(map[string]*interfaces.DLQDailyStats),
	}

	endDate := time.Now()
	startDate := endDate.Add(-time.Duration(days) * 24 * time.Hour)

	for _, msg := range r.messages {
		if msg.Service != service {
			continue
		}
		if msg.FirstSeenAt.Before(startDate) || msg.FirstSeenAt.After(endDate) {
			continue
		}

		dateStr := msg.FirstSeenAt.Format("2006-01-02")
		if _, exists := trends.DailyStats[dateStr]; !exists {
			trends.DailyStats[dateStr] = &interfaces.DLQDailyStats{
				Date:         msg.FirstSeenAt.Truncate(24 * time.Hour),
				ErrorTypes:   make(map[string]int),
				StatusCounts: make(map[string]int),
			}
		}

		trends.DailyStats[dateStr].MessageCount++
		trends.DailyStats[dateStr].ErrorTypes[msg.ErrorType]++
		trends.DailyStats[dateStr].StatusCounts[msg.Status]++
	}

	return trends, nil
}

// AnalyzeFailurePatterns analyzes DLQ messages to identify common failure patterns.
func (r *DLQRepository) AnalyzeFailurePatterns(_ context.Context, service string, days int) (map[string]*interfaces.DLQSimilarityGroup, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	patterns := make(map[string]*interfaces.DLQSimilarityGroup)
	endDate := time.Now()
	startDate := endDate.Add(-time.Duration(days) * 24 * time.Hour)

	for _, msg := range r.messages {
		if msg.Service != service {
			continue
		}
		if msg.FirstSeenAt.Before(startDate) || msg.FirstSeenAt.After(endDate) {
			continue
		}

		if _, exists := patterns[msg.SimilarityHash]; !exists {
			patterns[msg.SimilarityHash] = &interfaces.DLQSimilarityGroup{
				SimilarityHash: msg.SimilarityHash,
				ErrorType:      msg.ErrorType,
				Service:        msg.Service,
				FirstSeen:      msg.FirstSeenAt,
				LastSeen:       msg.FirstSeenAt,
				SampleError:    msg.ErrorMessage,
			}
		}

		group := patterns[msg.SimilarityHash]
		group.MessageCount++
		group.MessageIDs = append(group.MessageIDs, msg.ID)
		if msg.FirstSeenAt.Before(group.FirstSeen) {
			group.FirstSeen = msg.FirstSeenAt
		}
		if msg.FirstSeenAt.After(group.LastSeen) {
			group.LastSeen = msg.FirstSeenAt
		}
	}

	return patterns, nil
}

// ===== Retry Operations =====

// SendToDeadLetterQueue creates and stores a DLQ message with proper error categorization.
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

// RetryFailedMessage attempts to reprocess a DLQ message with exponential backoff.
func (r *DLQRepository) RetryFailedMessage(_ context.Context, messageID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	message, exists := r.messages[messageID]
	if !exists {
		return storage.ErrNotFound
	}

	if !message.CanReprocess() {
		return fmt.Errorf("message cannot be reprocessed")
	}

	message.MarkForReprocessing()

	if message.ShouldAbandon() {
		message.MarkAbandoned()
	}

	return nil
}

// GetRetryableMessages returns messages that are ready for retry based on backoff schedule.
func (r *DLQRepository) GetRetryableMessages(_ context.Context, service string, limit int) ([]*models.DLQMessage, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*models.DLQMessage
	now := time.Now()

	for _, msg := range r.messages {
		if msg.Service != service {
			continue
		}
		if !msg.CanReprocess() {
			continue
		}
		if msg.NextRetryAt != nil && now.Before(*msg.NextRetryAt) {
			continue
		}
		results = append(results, msg)
	}

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// ===== Cleanup Operations =====

// CleanupExpiredMessages deletes expired DLQ messages.
func (r *DLQRepository) CleanupExpiredMessages(_ context.Context, before time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	count := 0
	beforeUnix := before.Unix()
	for id, msg := range r.messages {
		if msg.ExpiresAt > 0 && msg.ExpiresAt < beforeUnix {
			delete(r.messages, id)
			count++
		}
	}

	return count, nil
}

// ===== Health Monitoring =====

// MonitorDLQHealth provides health metrics for DLQ monitoring and alerting.
func (r *DLQRepository) MonitorDLQHealth(_ context.Context, service string) (*interfaces.DLQHealthStatus, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	health := &interfaces.DLQHealthStatus{
		Service:    service,
		CheckTime:  time.Now(),
		ErrorRates: make(map[string]int),
		IsHealthy:  true,
		Alerts:     []string{},
	}

	oneHourAgo := time.Now().Add(-1 * time.Hour)
	var totalRetries int

	for _, msg := range r.messages {
		if msg.Service != service {
			continue
		}
		if msg.FirstSeenAt.Before(oneHourAgo) {
			continue
		}

		health.TotalMessages++
		totalRetries += msg.ReprocessingCount
		health.ErrorRates[msg.ErrorType]++

		switch msg.Status {
		case "new":
			health.NewMessages++
		case "reprocessing":
			health.ReprocessingCount++
		case "abandoned":
			health.AbandonedCount++
		}
	}

	if health.TotalMessages > 0 {
		health.AverageRetryCount = float64(totalRetries) / float64(health.TotalMessages)
	}

	if health.TotalMessages > 100 {
		health.IsHealthy = false
		health.Alerts = append(health.Alerts, "High volume of DLQ messages in last hour")
	}

	if health.AbandonedCount > 10 {
		health.IsHealthy = false
		health.Alerts = append(health.Alerts, "High number of abandoned messages")
	}

	return health, nil
}

// ===== Test Helper Methods =====

// Clear clears all data.
func (r *DLQRepository) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = make(map[string]*models.DLQMessage)
}

// Count returns the number of messages.
func (r *DLQRepository) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.messages)
}

// ===== Helper Functions =====

func matchesDLQFilter(msg *models.DLQMessage, filter *interfaces.DLQSearchFilter) bool {
	if filter.Service != "" && msg.Service != filter.Service {
		return false
	}
	if filter.ErrorType != "" && msg.ErrorType != filter.ErrorType {
		return false
	}
	if filter.Status != "" && msg.Status != filter.Status {
		return false
	}
	if filter.Priority != "" && msg.Priority != filter.Priority {
		return false
	}
	if filter.IsPermanent != nil && msg.IsPermanent != *filter.IsPermanent {
		return false
	}
	if !filter.StartTime.IsZero() && msg.FirstSeenAt.Before(filter.StartTime) {
		return false
	}
	if !filter.EndTime.IsZero() && msg.FirstSeenAt.After(filter.EndTime) {
		return false
	}
	if filter.SearchText != "" {
		searchText := strings.ToLower(filter.SearchText)
		if !strings.Contains(strings.ToLower(msg.ErrorMessage), searchText) &&
			!strings.Contains(strings.ToLower(msg.FailureReason), searchText) &&
			!strings.Contains(strings.ToLower(msg.MessageBody), searchText) {
			return false
		}
	}
	return true
}

func applyPagination(messages []*models.DLQMessage, limit int, cursor string) ([]*models.DLQMessage, string, error) {
	if len(messages) == 0 {
		return messages, "", nil
	}

	startIdx := 0
	if cursor != "" {
		for i, msg := range messages {
			if msg.ID == cursor {
				startIdx = i + 1
				break
			}
		}
	}

	if startIdx >= len(messages) {
		return []*models.DLQMessage{}, "", nil
	}

	safeLimit := limit
	if safeLimit <= 0 {
		safeLimit = 20
	}
	if safeLimit > 100 {
		safeLimit = 100
	}

	endIdx := startIdx + safeLimit
	if endIdx > len(messages) {
		endIdx = len(messages)
	}

	results := messages[startIdx:endIdx]

	var nextCursor string
	if endIdx < len(messages) {
		nextCursor = results[len(results)-1].ID
	}

	return results, nextCursor, nil
}

// Ensure DLQRepository implements interfaces.DLQRepository
var _ interfaces.DLQRepository = (*DLQRepository)(nil)
