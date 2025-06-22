package dynamodb

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// FederationCostRecord represents how federation costs are stored in DynamoDB
type FederationCostRecord struct {
	PK        string                  `dynamodbav:"PK"`
	SK        string                  `dynamodbav:"SK"`
	GSI1PK    string                  `dynamodbav:"GSI1PK,omitempty"`
	GSI1SK    string                  `dynamodbav:"GSI1SK,omitempty"`
	Type      string                  `dynamodbav:"Type"`
	Activity  *storage.FederationActivity `dynamodbav:"Activity,omitempty"`
	Cost      *storage.FederationCost     `dynamodbav:"Cost,omitempty"`
	TTL       int64                   `dynamodbav:"TTL,omitempty"`
	CreatedAt time.Time               `dynamodbav:"CreatedAt"`
}

// RecordFederationActivity records a single federation activity for cost tracking
func (s *dynamoDBStorage) RecordFederationActivity(ctx context.Context, activity *storage.FederationActivity) error {
	log := common.Logger().With(
		zap.String("domain", activity.Domain),
		zap.String("type", activity.Type),
		zap.String("activity_type", activity.ActivityType),
	)

	if activity.ID == "" {
		activity.ID = fmt.Sprintf("fed_activity_%s", generateRandomString(12))
	}

	// Create the record with proper partition key for time-series queries
	now := time.Now()
	record := &FederationCostRecord{
		PK:        fmt.Sprintf("FEDERATION#%s#%s", activity.Domain, now.Format("2006-01")), // Monthly partition
		SK:        fmt.Sprintf("ACTIVITY#%s#%s", now.Format("20060102150405"), activity.ID),
		GSI1PK:    fmt.Sprintf("FEDERATION_DAILY#%s", now.Format("2006-01-02")),
		GSI1SK:    fmt.Sprintf("DOMAIN#%s#%s", activity.Domain, activity.ID),
		Type:      "ACTIVITY",
		Activity:  activity,
		TTL:       now.Add(90 * 24 * time.Hour).Unix(), // Keep raw activities for 90 days
		CreatedAt: now,
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("Failed to marshal federation activity", zap.Error(err))
		return fmt.Errorf("failed to marshal federation activity: %w", err)
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		log.Error("Failed to record federation activity", zap.Error(err))
		return fmt.Errorf("failed to record federation activity: %w", err)
	}

	// Update aggregated costs asynchronously
	go s.updateAggregatedCosts(context.Background(), activity)

	return nil
}

// GetFederationCosts retrieves aggregated federation costs
func (s *dynamoDBStorage) GetFederationCosts(ctx context.Context, startTime, endTime time.Time, limit int, cursor string) ([]*storage.FederationCost, string, error) {
	log := common.Logger().With(
		zap.Time("start_time", startTime),
		zap.Time("end_time", endTime),
		zap.Int("limit", limit),
	)

	// Query aggregated monthly costs
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", startTime.Format("2006-01"))
	
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
		Limit:            safeInt32(limit),
		ScanIndexForward: aws.Bool(false), // Most recent first
	}

	if cursor != "" {
		input.ExclusiveStartKey = map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: cursor},
		}
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("Failed to query federation costs", zap.Error(err))
		return nil, "", fmt.Errorf("failed to query federation costs: %w", err)
	}

	costs := make([]*storage.FederationCost, 0, len(result.Items))
	for _, item := range result.Items {
		var record FederationCostRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			log.Warn("Failed to unmarshal cost record", zap.Error(err))
			continue
		}

		if record.Type == "COST" && record.Cost != nil {
			costs = append(costs, record.Cost)
		}
	}

	var nextCursor string
	if result.LastEvaluatedKey != nil {
		if sk, ok := result.LastEvaluatedKey["SK"]; ok {
			if skStr, ok := sk.(*types.AttributeValueMemberS); ok {
				nextCursor = skStr.Value
			}
		}
	}

	return costs, nextCursor, nil
}

// GetInstanceHealthReport generates a health report for a specific instance
func (s *dynamoDBStorage) GetInstanceHealthReport(ctx context.Context, domain string, period time.Duration) (*storage.InstanceHealthReport, error) {
	log := common.Logger().With(
		zap.String("domain", domain),
		zap.Duration("period", period),
	)

	// Calculate time range
	endTime := time.Now()
	startTime := endTime.Add(-period)

	// Query recent activities for this domain
	pk := fmt.Sprintf("FEDERATION#%s#%s", domain, startTime.Format("2006-01"))
	
	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk AND SK > :start"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk":    &types.AttributeValueMemberS{Value: pk},
			":start": &types.AttributeValueMemberS{Value: fmt.Sprintf("ACTIVITY#%s", startTime.Format("20060102150405"))},
		},
		Limit: safeInt32(1000), // Sample up to 1000 recent activities
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("Failed to query instance activities", zap.Error(err))
		return nil, fmt.Errorf("failed to query instance activities: %w", err)
	}

	// Calculate metrics
	var totalResponseTime int64
	var errorCount int64
	var successCount int64
	var queueDepth int
	issues := []string{}
	recommendations := []string{}

	for _, item := range result.Items {
		var record FederationCostRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			continue
		}

		if record.Type == "ACTIVITY" && record.Activity != nil {
			totalResponseTime += record.Activity.ResponseTime
			if record.Activity.Success {
				successCount++
			} else {
				errorCount++
			}
		}
	}

	totalRequests := successCount + errorCount
	avgResponseTime := float64(0)
	errorRate := float64(0)

	if totalRequests > 0 {
		avgResponseTime = float64(totalResponseTime) / float64(totalRequests)
		errorRate = float64(errorCount) / float64(totalRequests)
	}

	// Determine status and generate recommendations
	status := "healthy"
	if errorRate > 0.1 {
		status = "critical"
		issues = append(issues, fmt.Sprintf("High error rate: %.2f%%", errorRate*100))
		recommendations = append(recommendations, "Consider temporarily blocking or rate limiting this instance")
	} else if errorRate > 0.05 {
		status = "warning"
		issues = append(issues, fmt.Sprintf("Elevated error rate: %.2f%%", errorRate*100))
		recommendations = append(recommendations, "Monitor this instance closely")
	}

	if avgResponseTime > 5000 { // 5 seconds
		if status == "healthy" {
			status = "warning"
		}
		issues = append(issues, fmt.Sprintf("Slow response time: %.2fs", avgResponseTime/1000))
		recommendations = append(recommendations, "Enable request caching for this instance")
	}

	// Get current queue depth (would need to query from SQS in production)
	// For now, estimate based on recent activity patterns
	queueDepth = int(math.Min(float64(errorCount)*2, 1000))

	report := &storage.InstanceHealthReport{
		Domain:          domain,
		Status:          status,
		ResponseTime:    avgResponseTime,
		ErrorRate:       errorRate,
		FederationDelay: avgResponseTime / 1000, // Convert to seconds
		QueueDepth:      queueDepth,
		Issues:          issues,
		Recommendations: recommendations,
		LastChecked:     time.Now(),
	}

	return report, nil
}

// GetCostProjections generates cost projections based on historical data
func (s *dynamoDBStorage) GetCostProjections(ctx context.Context, period string) (*storage.CostProjection, error) {
	log := common.Logger().With(zap.String("period", period))

	// Get current month's costs
	currentMonth := time.Now().Format("2006-01")
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", currentMonth)

	input := &dynamodb.QueryInput{
		TableName:              s.getTableName(),
		KeyConditionExpression: aws.String("PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: pk},
		},
	}

	result, err := s.client.Query(ctx, input)
	if err != nil {
		log.Error("Failed to query current costs", zap.Error(err))
		return nil, fmt.Errorf("failed to query current costs: %w", err)
	}

	// Calculate current total cost
	currentCost := float64(0)
	domainCosts := make(map[string]float64)

	for _, item := range result.Items {
		var record FederationCostRecord
		err = s.UnmarshalItem(item, &record)
		if err != nil {
			continue
		}

		if record.Type == "COST" && record.Cost != nil {
			currentCost += record.Cost.EstimatedCostUSD
			domainCosts[record.Cost.Domain] += record.Cost.EstimatedCostUSD
		}
	}

	// Simple projection: assume 15% growth rate
	growthRate := 0.15
	projectedCost := currentCost * (1 + growthRate)

	// Identify top cost drivers
	topDrivers := []storage.CostDriver{}
	
	// Sort domains by cost
	for domain, cost := range domainCosts {
		driver := storage.CostDriver{
			Type:           "Federation Traffic",
			Domain:         domain,
			Cost:           cost,
			PercentOfTotal: (cost / currentCost) * 100,
			Trend:          "stable", // Would need historical data to determine trend
		}
		topDrivers = append(topDrivers, driver)
	}

	// Sort top drivers by cost (descending)
	// In production, would use a proper sorting algorithm
	if len(topDrivers) > 3 {
		topDrivers = topDrivers[:3] // Keep top 3
	}

	recommendations := []string{
		"Enable progressive media loading to reduce bandwidth costs",
		"Implement federation rate limiting for high-traffic instances",
		"Consider archiving old media to cheaper storage tiers",
	}

	projection := &storage.CostProjection{
		Period:          period,
		CurrentCost:     currentCost,
		ProjectedCost:   projectedCost,
		Variance:        growthRate,
		TopDrivers:      topDrivers,
		Recommendations: recommendations,
	}

	return projection, nil
}

// updateAggregatedCosts updates the aggregated cost data asynchronously
func (s *dynamoDBStorage) updateAggregatedCosts(ctx context.Context, activity *storage.FederationActivity) {
	log := common.Logger().With(
		zap.String("domain", activity.Domain),
		zap.String("type", activity.Type),
	)

	// Get current aggregated cost record
	pk := fmt.Sprintf("FEDERATION_COSTS#%s", time.Now().Format("2006-01"))
	sk := fmt.Sprintf("DOMAIN#%s", activity.Domain)

	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: s.getTableName(),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: pk},
			"SK": &types.AttributeValueMemberS{Value: sk},
		},
	})

	var cost storage.FederationCost
	if err != nil || result.Item == nil {
		// Create new cost record
		cost = storage.FederationCost{
			Domain:       activity.Domain,
			Period:       "monthly",
			LastUpdated:  time.Now(),
		}
	} else {
		// Unmarshal existing record
		var record FederationCostRecord
		err = s.UnmarshalItem(result.Item, &record)
		if err != nil || record.Cost == nil {
			log.Error("Failed to unmarshal cost record", zap.Error(err))
			return
		}
		cost = *record.Cost
	}

	// Update metrics
	if activity.Type == "ingress" {
		cost.IngressBytes += activity.ByteSize
	} else {
		cost.EgressBytes += activity.ByteSize
	}

	cost.RequestCount++
	if !activity.Success {
		cost.ErrorCount++
	}

	// Update error rate
	if cost.RequestCount > 0 {
		cost.ErrorRate = float64(cost.ErrorCount) / float64(cost.RequestCount)
	}

	// Update average response time (simple moving average)
	cost.AvgResponseTime = (cost.AvgResponseTime*float64(cost.RequestCount-1) + float64(activity.ResponseTime)) / float64(cost.RequestCount)

	// Estimate cost (simplified calculation)
	// $0.09 per GB data transfer + $0.20 per million requests
	dataTransferGB := float64(cost.IngressBytes+cost.EgressBytes) / (1024 * 1024 * 1024)
	requestMillions := float64(cost.RequestCount) / 1000000
	cost.EstimatedCostUSD = (dataTransferGB * 0.09) + (requestMillions * 0.20)

	// Save updated cost record
	record := &FederationCostRecord{
		PK:        pk,
		SK:        sk,
		Type:      "COST",
		Cost:      &cost,
		CreatedAt: time.Now(),
	}

	av, err := s.MarshalItem(record)
	if err != nil {
		log.Error("Failed to marshal cost record", zap.Error(err))
		return
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: s.getTableName(),
		Item:      av,
	})

	if err != nil {
		log.Error("Failed to update aggregated costs", zap.Error(err))
	}
}