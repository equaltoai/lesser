package routing

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// InstanceHealthChecker monitors instance health
type InstanceHealthChecker struct {
	db         *dynamodb.Client
	tableName  string
	logger     *zap.Logger
	config     *RoutingConfig
	httpClient *http.Client

	// Active monitors
	monitors sync.Map // instanceID -> *monitor

	// Batch processing
	resultChan chan *healthCheckResult
}

type monitor struct {
	instance  *Instance
	ticker    *time.Ticker
	stopChan  chan struct{}
	lastCheck time.Time
}

type healthCheckResult struct {
	instanceID string
	health     *HealthStatus
	err        error
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(db *dynamodb.Client, tableName string, logger *zap.Logger, config *RoutingConfig) *InstanceHealthChecker {
	hc := &InstanceHealthChecker{
		db:        db,
		tableName: tableName,
		logger:    logger,
		config:    config,
		httpClient: &http.Client{
			Timeout: config.HealthCheckTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
		resultChan: make(chan *healthCheckResult, 1000),
	}

	// Start result processor
	go hc.processResults()

	return hc
}

// StartMonitoring starts health monitoring for an instance
func (hc *InstanceHealthChecker) StartMonitoring(instance *Instance) error {
	// Check if already monitoring
	if _, exists := hc.monitors.Load(instance.ID); exists {
		return nil
	}

	mon := &monitor{
		instance: instance,
		ticker:   time.NewTicker(hc.config.HealthCheckInterval),
		stopChan: make(chan struct{}),
	}

	hc.monitors.Store(instance.ID, mon)

	// Start monitoring goroutine
	go hc.monitorInstance(mon)

	hc.logger.Info("started health monitoring",
		zap.String("instanceID", instance.ID),
		zap.String("domain", instance.Domain))

	return nil
}

// StopMonitoring stops health monitoring for an instance
func (hc *InstanceHealthChecker) StopMonitoring(instanceID string) error {
	if mon, exists := hc.monitors.Load(instanceID); exists {
		m := mon.(*monitor)
		close(m.stopChan)
		m.ticker.Stop()
		hc.monitors.Delete(instanceID)

		hc.logger.Info("stopped health monitoring",
			zap.String("instanceID", instanceID))
	}

	return nil
}

// CheckHealth performs a health check on an instance
func (hc *InstanceHealthChecker) CheckHealth(instance *Instance) (*HealthStatus, error) {
	startTime := time.Now()

	health := &HealthStatus{
		Timestamp:    time.Now(),
		Reachable:    false,
		ResponseTime: 0,
		StatusCode:   0,
	}

	// Perform HTTP health check
	req, err := http.NewRequest("GET", instance.InboxURL, nil)
	if err != nil {
		health.ErrorMessage = fmt.Sprintf("invalid URL: %v", err)
		return health, err
	}

	// Add headers
	req.Header.Set("User-Agent", "Lesser/1.0 (Federation Health Check)")
	req.Header.Set("Accept", "application/activity+json")

	// Perform request
	resp, err := hc.httpClient.Do(req)
	if err != nil {
		health.ErrorMessage = fmt.Sprintf("request failed: %v", err)
		return health, nil
	}
	defer resp.Body.Close()

	// Update health status
	health.Reachable = true
	health.ResponseTime = time.Since(startTime)
	health.StatusCode = resp.StatusCode

	// Check status code
	if resp.StatusCode >= 500 {
		health.ErrorMessage = fmt.Sprintf("server error: %d", resp.StatusCode)
		health.ErrorRate = 1.0
	} else if resp.StatusCode >= 400 {
		health.ErrorMessage = fmt.Sprintf("client error: %d", resp.StatusCode)
		health.ErrorRate = 0.5
	}

	// Parse additional health info from headers if available
	if backlog := resp.Header.Get("X-Inbox-Backlog"); backlog != "" {
		if _, err := fmt.Sscanf(backlog, "%d", &health.InboxBacklog); err != nil {
			hc.logger.Warn("failed to parse X-Inbox-Backlog header",
				zap.String("value", backlog),
				zap.Error(err))
		}
	}
	if delay := resp.Header.Get("X-Processing-Delay"); delay != "" {
		duration, err := time.ParseDuration(delay)
		if err != nil {
			hc.logger.Warn("failed to parse X-Processing-Delay header",
				zap.String("value", delay),
				zap.Error(err))
		}
		health.ProcessingDelay = duration
	}

	return health, nil
}

// GetHealthHistory retrieves health history for an instance
func (hc *InstanceHealthChecker) GetHealthHistory(instanceID string, duration time.Duration) ([]*HealthStatus, error) {
	// Query health history from DynamoDB
	since := time.Now().Add(-duration)

	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(hc.tableName),
		KeyConditionExpression: aws.String("PK = :pk AND SK BETWEEN :start AND :end"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instanceID)},
			":start": &types.AttributeValueMemberS{Value: fmt.Sprintf("HEALTH#%d", since.UnixNano())},
			":end":   &types.AttributeValueMemberS{Value: fmt.Sprintf("HEALTH#%d", time.Now().UnixNano())},
		},
		ScanIndexForward: aws.Bool(false), // Most recent first
		Limit:            aws.Int32(100),
	}

	result, err := hc.db.Query(context.Background(), queryInput)
	if err != nil {
		return nil, fmt.Errorf("query health history: %w", err)
	}

	history := make([]*HealthStatus, 0, len(result.Items))
	for _, item := range result.Items {
		health, err := hc.parseHealthStatus(item)
		if err != nil {
			continue
		}
		history = append(history, health)
	}

	return history, nil
}

// GetAggregatedHealth returns aggregated health metrics
func (hc *InstanceHealthChecker) GetAggregatedHealth(instanceID string, window time.Duration) (*AggregatedHealth, error) {
	history, err := hc.GetHealthHistory(instanceID, window)
	if err != nil {
		return nil, err
	}

	if len(history) == 0 {
		return nil, fmt.Errorf("no health data available")
	}

	agg := &AggregatedHealth{
		InstanceID:  instanceID,
		Window:      window,
		SampleCount: len(history),
		LastCheck:   history[0].Timestamp,
	}

	// Calculate aggregates
	var totalResponseTime time.Duration
	reachableCount := 0
	errorCount := 0

	for _, h := range history {
		if h.Reachable {
			reachableCount++
			totalResponseTime += h.ResponseTime
		} else {
			errorCount++
		}

		// Track response codes
		if h.StatusCode > 0 {
			if agg.StatusCodes == nil {
				agg.StatusCodes = make(map[int]int)
			}
			agg.StatusCodes[h.StatusCode]++
		}

		// Update backlog stats
		if h.InboxBacklog > agg.MaxBacklog {
			agg.MaxBacklog = h.InboxBacklog
		}
		agg.AvgBacklog += h.InboxBacklog
	}

	// Calculate final metrics
	agg.Availability = float64(reachableCount) / float64(len(history))
	agg.ErrorRate = float64(errorCount) / float64(len(history))

	if reachableCount > 0 {
		agg.AvgResponseTime = totalResponseTime / time.Duration(reachableCount)
	}

	if len(history) > 0 {
		agg.AvgBacklog = agg.AvgBacklog / len(history)
	}

	// Determine health score (0-100)
	agg.HealthScore = hc.calculateHealthScore(agg)

	return agg, nil
}

// Private methods

func (hc *InstanceHealthChecker) monitorInstance(mon *monitor) {
	// Perform initial check
	hc.performHealthCheck(mon.instance)

	for {
		select {
		case <-mon.ticker.C:
			hc.performHealthCheck(mon.instance)

		case <-mon.stopChan:
			return
		}
	}
}

func (hc *InstanceHealthChecker) performHealthCheck(instance *Instance) {
	health, err := hc.CheckHealth(instance)

	// Send result to processor
	hc.resultChan <- &healthCheckResult{
		instanceID: instance.ID,
		health:     health,
		err:        err,
	}
}

func (hc *InstanceHealthChecker) processResults() {
	batch := make([]*healthCheckResult, 0, 25)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case result := <-hc.resultChan:
			batch = append(batch, result)

			// Process batch when full
			if len(batch) >= 25 {
				hc.processBatch(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			// Process any pending results
			if len(batch) > 0 {
				hc.processBatch(batch)
				batch = batch[:0]
			}
		}
	}
}

func (hc *InstanceHealthChecker) processBatch(batch []*healthCheckResult) {
	// Store results in DynamoDB using batch write
	writeRequests := make([]types.WriteRequest, 0, len(batch))

	for _, result := range batch {
		if result.err != nil {
			hc.logger.Error("health check failed",
				zap.String("instanceID", result.instanceID),
				zap.Error(result.err))
			continue
		}

		// Create DynamoDB item
		item := map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", result.instanceID)},
			"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HEALTH#%d", result.health.Timestamp.UnixNano())},

			"Reachable":    &types.AttributeValueMemberBOOL{Value: result.health.Reachable},
			"ResponseTime": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.health.ResponseTime.Milliseconds())},
			"StatusCode":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.health.StatusCode)},
			"ErrorRate":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%.4f", result.health.ErrorRate)},
			"Timestamp":    &types.AttributeValueMemberS{Value: result.health.Timestamp.Format(time.RFC3339)},

			// TTL for cleanup (7 days)
			"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(7*24*time.Hour).Unix())},
		}

		if result.health.ErrorMessage != "" {
			item["ErrorMessage"] = &types.AttributeValueMemberS{Value: result.health.ErrorMessage}
		}
		if result.health.InboxBacklog > 0 {
			item["InboxBacklog"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.health.InboxBacklog)}
		}
		if result.health.ProcessingDelay > 0 {
			item["ProcessingDelay"] = &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", result.health.ProcessingDelay.Milliseconds())}
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{Item: item},
		})

		// Log significant changes
		if !result.health.Reachable {
			hc.logger.Warn("instance unreachable",
				zap.String("instanceID", result.instanceID),
				zap.String("error", result.health.ErrorMessage))
		}
	}

	// Batch write to DynamoDB
	if len(writeRequests) > 0 {
		batchInput := &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				hc.tableName: writeRequests,
			},
		}

		_, err := hc.db.BatchWriteItem(context.Background(), batchInput)
		if err != nil {
			hc.logger.Error("failed to write health results", zap.Error(err))
		}
	}
}

func (hc *InstanceHealthChecker) parseHealthStatus(item map[string]types.AttributeValue) (*HealthStatus, error) {
	health := &HealthStatus{}

	if v, ok := item["Reachable"].(*types.AttributeValueMemberBOOL); ok {
		health.Reachable = v.Value
	}
	if v, ok := item["ResponseTime"].(*types.AttributeValueMemberN); ok {
		var ms int64
		if _, err := fmt.Sscanf(v.Value, "%d", &ms); err != nil {
			hc.logger.Warn("failed to parse ResponseTime", zap.String("value", v.Value), zap.Error(err))
		}
		health.ResponseTime = time.Duration(ms) * time.Millisecond
	}
	if v, ok := item["StatusCode"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &health.StatusCode); err != nil {
			hc.logger.Warn("failed to parse StatusCode", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["ErrorRate"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &health.ErrorRate); err != nil {
			hc.logger.Warn("failed to parse ErrorRate", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["ErrorMessage"].(*types.AttributeValueMemberS); ok {
		health.ErrorMessage = v.Value
	}
	if v, ok := item["InboxBacklog"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &health.InboxBacklog); err != nil {
			hc.logger.Warn("failed to parse InboxBacklog", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["ProcessingDelay"].(*types.AttributeValueMemberN); ok {
		var ms int64
		if _, err := fmt.Sscanf(v.Value, "%d", &ms); err != nil {
			hc.logger.Warn("failed to parse ProcessingDelay", zap.String("value", v.Value), zap.Error(err))
		}
		health.ProcessingDelay = time.Duration(ms) * time.Millisecond
	}
	if v, ok := item["Timestamp"].(*types.AttributeValueMemberS); ok {
		health.Timestamp, _ = time.Parse(time.RFC3339, v.Value)
	}

	return health, nil
}

func (hc *InstanceHealthChecker) calculateHealthScore(agg *AggregatedHealth) float64 {
	score := 100.0

	// Availability (40% weight)
	score -= (1.0 - agg.Availability) * 40.0

	// Response time (30% weight)
	if agg.AvgResponseTime > 1*time.Second {
		penalty := float64(agg.AvgResponseTime.Milliseconds()-1000) / 100.0 // -1 point per 100ms over 1s
		score -= min(penalty, 30.0)
	}

	// Error rate (20% weight)
	score -= agg.ErrorRate * 20.0

	// Backlog (10% weight)
	if agg.MaxBacklog > 1000 {
		penalty := float64(agg.MaxBacklog-1000) / 1000.0 // -1 point per 1000 messages
		score -= min(penalty, 10.0)
	}

	return max(score, 0.0)
}

// AggregatedHealth represents aggregated health metrics
type AggregatedHealth struct {
	InstanceID  string
	Window      time.Duration
	SampleCount int
	LastCheck   time.Time

	Availability    float64
	AvgResponseTime time.Duration
	ErrorRate       float64
	AvgBacklog      int
	MaxBacklog      int
	StatusCodes     map[int]int

	HealthScore float64 // 0-100
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
