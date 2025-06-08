package cost

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"

	"github.com/aron23/lesser/pkg/cost"
)

// dynamoStorage implements the Storage interface using DynamoDB
type dynamoStorage struct {
	client      *dynamodb.Client
	tableName   string
	logger      *zap.Logger
	costTracker *cost.Tracker
}

// NewDynamoStorage creates a new DynamoDB-backed storage implementation
func NewDynamoStorage(
	client *dynamodb.Client,
	tableName string,
	logger *zap.Logger,
	costTracker *cost.Tracker,
) Storage {
	return &dynamoStorage{
		client:      client,
		tableName:   tableName,
		logger:      logger,
		costTracker: costTracker,
	}
}

// RecordCost saves federation cost data to DynamoDB
func (s *dynamoStorage) RecordCost(ctx context.Context, cost *FederationCost) error {
	// Create DynamoDB item
	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{
			Value: fmt.Sprintf("FEDCOST#%s", cost.InstanceDomain),
		},
		"SK": &types.AttributeValueMemberS{
			Value: fmt.Sprintf("PERIOD#%s", cost.BillingPeriod),
		},
		"Type": &types.AttributeValueMemberS{
			Value: "FederationCost",
		},
		"InstanceDomain": &types.AttributeValueMemberS{
			Value: cost.InstanceDomain,
		},
		"IngressBytes": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", cost.IngressBytes),
		},
		"EgressBytes": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", cost.EgressBytes),
		},
		"RequestCount": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", cost.RequestCount),
		},
		"ErrorCount": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", cost.ErrorCount),
		},
		"ErrorRate": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%.4f", cost.ErrorRate),
		},
		"AverageCostUSD": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%.4f", cost.AverageCostUSD),
		},
		"BillingPeriod": &types.AttributeValueMemberS{
			Value: cost.BillingPeriod,
		},
		"LastUpdated": &types.AttributeValueMemberS{
			Value: cost.LastUpdated.Format(time.RFC3339),
		},
		"UpdatedAt": &types.AttributeValueMemberS{
			Value: time.Now().Format(time.RFC3339),
		},
		"TTL": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", time.Now().Add(90*24*time.Hour).Unix()), // 90 days retention
		},
	}

	// Add GSI for period-based queries
	item["GSI1PK"] = &types.AttributeValueMemberS{
		Value: fmt.Sprintf("PERIOD#%s", cost.BillingPeriod),
	}
	item["GSI1SK"] = &types.AttributeValueMemberS{
		Value: fmt.Sprintf("INSTANCE#%s", cost.InstanceDomain),
	}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		s.logger.Error("failed to record federation cost",
			zap.Error(err),
			zap.String("instance", cost.InstanceDomain))
		return fmt.Errorf("put federation cost: %w", err)
	}

	s.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceCost retrieves cost data for a specific instance and period
func (s *dynamoStorage) GetInstanceCost(ctx context.Context, domain string, period string) (*FederationCost, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("FEDCOST#%s", domain),
			},
			"SK": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("PERIOD#%s", period),
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("get federation cost: %w", err)
	}

	s.costTracker.TrackDynamoRead(1)

	if result.Item == nil {
		return nil, nil
	}

	var cost FederationCost
	if err := attributevalue.UnmarshalMap(result.Item, &cost); err != nil {
		return nil, fmt.Errorf("unmarshal federation cost: %w", err)
	}

	return &cost, nil
}

// GetCostMetrics retrieves aggregated cost metrics for a period
func (s *dynamoStorage) GetCostMetrics(ctx context.Context, period string) (*CostMetrics, error) {
	metrics := &CostMetrics{
		Period:        period,
		InstanceCosts: make(map[string]float64),
		ActivityCosts: make(map[string]float64),
	}

	// Query all costs for the period using GSI
	paginator := dynamodb.NewQueryPaginator(s.client, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("PERIOD#%s", period),
			},
		},
	})

	readUnits := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query cost metrics: %w", err)
		}
		readUnits++

		for _, item := range page.Items {
			var cost FederationCost
			if err := attributevalue.UnmarshalMap(item, &cost); err != nil {
				s.logger.Warn("failed to unmarshal cost item", zap.Error(err))
				continue
			}

			// Aggregate metrics
			instanceTotal := cost.AverageCostUSD * float64(cost.RequestCount)
			metrics.InstanceCosts[cost.InstanceDomain] = instanceTotal
			metrics.TotalCostUSD += instanceTotal
			metrics.DataTransferGB += float64(cost.EgressBytes+cost.IngressBytes) / (1024 * 1024 * 1024)
			metrics.RequestCount += int64(cost.RequestCount)
		}
	}

	s.costTracker.TrackDynamoRead(readUnits)
	return metrics, nil
}

// UpdateInstanceHealth updates health metrics for an instance
func (s *dynamoStorage) UpdateInstanceHealth(ctx context.Context, health *InstanceHealth) error {
	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{
			Value: fmt.Sprintf("INSTANCE#%s", health.Domain),
		},
		"SK": &types.AttributeValueMemberS{
			Value: "HEALTH",
		},
		"Type": &types.AttributeValueMemberS{
			Value: "InstanceHealth",
		},
		"Domain": &types.AttributeValueMemberS{
			Value: health.Domain,
		},
		"HealthScore": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%.4f", health.HealthScore),
		},
		"ResponseTimeP95": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", health.ResponseTimeP95),
		},
		"SuccessRate": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%.4f", health.SuccessRate),
		},
		"ConsecutiveFails": &types.AttributeValueMemberN{
			Value: fmt.Sprintf("%d", health.ConsecutiveFails),
		},
		"IsHealthy": &types.AttributeValueMemberBOOL{
			Value: health.IsHealthy,
		},
		"LastHealthCheck": &types.AttributeValueMemberS{
			Value: health.LastHealthCheck.Format(time.RFC3339),
		},
		"UpdatedAt": &types.AttributeValueMemberS{
			Value: time.Now().Format(time.RFC3339),
		},
	}

	// Add to unhealthy index if needed
	if !health.IsHealthy {
		item["GSI2PK"] = &types.AttributeValueMemberS{
			Value: "UNHEALTHY",
		}
		item["GSI2SK"] = &types.AttributeValueMemberS{
			Value: fmt.Sprintf("SCORE#%.4f#%s", health.HealthScore, health.Domain),
		}
	}

	_, err := s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("update instance health: %w", err)
	}

	s.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceHealth retrieves health data for an instance
func (s *dynamoStorage) GetInstanceHealth(ctx context.Context, domain string) (*InstanceHealth, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("INSTANCE#%s", domain),
			},
			"SK": &types.AttributeValueMemberS{
				Value: "HEALTH",
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("get instance health: %w", err)
	}

	s.costTracker.TrackDynamoRead(1)

	if result.Item == nil {
		return nil, nil
	}

	var health InstanceHealth
	if err := attributevalue.UnmarshalMap(result.Item, &health); err != nil {
		return nil, fmt.Errorf("unmarshal instance health: %w", err)
	}

	return &health, nil
}

// ListUnhealthyInstances returns all unhealthy instances
func (s *dynamoStorage) ListUnhealthyInstances(ctx context.Context) ([]*InstanceHealth, error) {
	var instances []*InstanceHealth

	paginator := dynamodb.NewQueryPaginator(s.client, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{
				Value: "UNHEALTHY",
			},
		},
		ScanIndexForward: aws.Bool(true), // Lowest health scores first
		Limit:            aws.Int32(50),  // Limit to 50 unhealthy instances
	})

	readUnits := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("query unhealthy instances: %w", err)
		}
		readUnits++

		for _, item := range page.Items {
			var health InstanceHealth
			if err := attributevalue.UnmarshalMap(item, &health); err != nil {
				s.logger.Warn("failed to unmarshal health item", zap.Error(err))
				continue
			}
			instances = append(instances, &health)
		}

		// Only get first page to limit results
		break
	}

	s.costTracker.TrackDynamoRead(readUnits)
	return instances, nil
}

// SaveInstanceConfig saves federation configuration for an instance
func (s *dynamoStorage) SaveInstanceConfig(ctx context.Context, config *InstanceConfig) error {
	item, err := attributevalue.MarshalMap(config)
	if err != nil {
		return fmt.Errorf("marshal instance config: %w", err)
	}

	// Add DynamoDB keys
	item["PK"] = &types.AttributeValueMemberS{
		Value: fmt.Sprintf("INSTANCE#%s", config.Domain),
	}
	item["SK"] = &types.AttributeValueMemberS{
		Value: "CONFIG",
	}
	item["Type"] = &types.AttributeValueMemberS{
		Value: "InstanceConfig",
	}
	item["UpdatedAt"] = &types.AttributeValueMemberS{
		Value: time.Now().Format(time.RFC3339),
	}

	// Add to tier index
	item["GSI3PK"] = &types.AttributeValueMemberS{
		Value: fmt.Sprintf("TIER#%s", config.Tier),
	}
	item["GSI3SK"] = &types.AttributeValueMemberS{
		Value: config.Domain,
	}

	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})

	if err != nil {
		return fmt.Errorf("save instance config: %w", err)
	}

	s.costTracker.TrackDynamoWrite(1)
	return nil
}

// GetInstanceConfig retrieves configuration for an instance
func (s *dynamoStorage) GetInstanceConfig(ctx context.Context, domain string) (*InstanceConfig, error) {
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{
				Value: fmt.Sprintf("INSTANCE#%s", domain),
			},
			"SK": &types.AttributeValueMemberS{
				Value: "CONFIG",
			},
		},
	})

	if err != nil {
		return nil, fmt.Errorf("get instance config: %w", err)
	}

	s.costTracker.TrackDynamoRead(1)

	if result.Item == nil {
		return nil, nil
	}

	var config InstanceConfig
	if err := attributevalue.UnmarshalMap(result.Item, &config); err != nil {
		return nil, fmt.Errorf("unmarshal instance config: %w", err)
	}

	return &config, nil
}

// ListInstanceConfigs returns all instance configurations
func (s *dynamoStorage) ListInstanceConfigs(ctx context.Context) ([]*InstanceConfig, error) {
	var configs []*InstanceConfig

	// Scan for all configs (could be optimized with a GSI if needed)
	paginator := dynamodb.NewScanPaginator(s.client, &dynamodb.ScanInput{
		TableName:        aws.String(s.tableName),
		FilterExpression: aws.String("#type = :type"),
		ExpressionAttributeNames: map[string]string{
			"#type": "Type",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":type": &types.AttributeValueMemberS{
				Value: "InstanceConfig",
			},
		},
	})

	readUnits := 0
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("scan instance configs: %w", err)
		}
		readUnits++

		for _, item := range page.Items {
			var config InstanceConfig
			if err := attributevalue.UnmarshalMap(item, &config); err != nil {
				s.logger.Warn("failed to unmarshal config item", zap.Error(err))
				continue
			}
			configs = append(configs, &config)
		}
	}

	s.costTracker.TrackDynamoRead(readUnits)
	return configs, nil
}
