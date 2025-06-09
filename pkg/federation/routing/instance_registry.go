package routing

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"go.uber.org/zap"
)

// safeIntToInt32 safely converts int to int32, capping at math.MaxInt32
func safeIntToInt32(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(n)
}

// InstanceRegistry manages federated instance data with optimized queries
type InstanceRegistry struct {
	db        *dynamodb.Client
	tableName string
	logger    *zap.Logger

	// Local cache for frequently accessed instances
	cache    sync.Map
	cacheTTL time.Duration

	// Batch writer for efficient writes
	writeBatch  chan *Instance
	updateBatch chan *instanceUpdate
}

type instanceUpdate struct {
	instanceID string
	updates    map[string]interface{}
}

type cachedInstance struct {
	instance *Instance
	cachedAt time.Time
}

// NewInstanceRegistry creates a new instance registry
func NewInstanceRegistry(db *dynamodb.Client, tableName string, logger *zap.Logger) *InstanceRegistry {
	ir := &InstanceRegistry{
		db:          db,
		tableName:   tableName,
		logger:      logger,
		cacheTTL:    5 * time.Minute,
		writeBatch:  make(chan *Instance, 100),
		updateBatch: make(chan *instanceUpdate, 100),
	}

	// Start batch processors
	go ir.processBatchWrites()
	go ir.processBatchUpdates()

	return ir
}

// RegisterInstance registers a new federated instance
func (ir *InstanceRegistry) RegisterInstance(ctx context.Context, instance *Instance) error {
	if instance.ID == "" {
		instance.ID = generateInstanceID(instance.Domain)
	}

	instance.RegisteredAt = time.Now()
	instance.LastSeen = time.Now()
	instance.Status = InstanceStatusActive

	// Prepare DynamoDB item with optimized structure
	item := map[string]types.AttributeValue{
		// Primary key
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instance.ID)},
		"SK": &types.AttributeValueMemberS{Value: "METADATA"},

		// Core attributes
		"ID":             &types.AttributeValueMemberS{Value: instance.ID},
		"Domain":         &types.AttributeValueMemberS{Value: instance.Domain},
		"InboxURL":       &types.AttributeValueMemberS{Value: instance.InboxURL},
		"SharedInboxURL": &types.AttributeValueMemberS{Value: instance.SharedInboxURL},
		"PublicKeyPEM":   &types.AttributeValueMemberS{Value: instance.PublicKeyPEM},

		// Status
		"Status":       &types.AttributeValueMemberS{Value: string(instance.Status)},
		"LastSeen":     &types.AttributeValueMemberS{Value: instance.LastSeen.Format(time.RFC3339)},
		"RegisteredAt": &types.AttributeValueMemberS{Value: instance.RegisteredAt.Format(time.RFC3339)},

		// Performance metrics
		"AvgResponseTime": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.AvgResponseTime.Milliseconds())},
		"SuccessRate":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%.4f", instance.SuccessRate)},
		"ErrorRate":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%.4f", instance.ErrorRate)},

		// Cost tracking
		"TierLevel":    &types.AttributeValueMemberS{Value: string(instance.TierLevel)},
		"MonthlyQuota": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.MonthlyQuota)},
		"CurrentUsage": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.CurrentUsage)},

		// GSI for status-based queries
		"GSI1PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", instance.Status)},
		"GSI1SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("DOMAIN#%s", instance.Domain)},

		// GSI for tier-based queries
		"GSI2PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("TIER#%s", instance.TierLevel)},
		"GSI2SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("USAGE#%010d", instance.CurrentUsage)},

		// TTL for automatic cleanup
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(365*24*time.Hour).Unix())},
	}

	// Add capabilities
	if len(instance.SupportedTypes) > 0 {
		typeList := &types.AttributeValueMemberL{
			Value: make([]types.AttributeValue, len(instance.SupportedTypes)),
		}
		for i, t := range instance.SupportedTypes {
			typeList.Value[i] = &types.AttributeValueMemberS{Value: string(t)}
		}
		item["SupportedTypes"] = typeList
	}

	// Add rate limits
	if instance.RateLimits.MessagesPerMinute > 0 {
		item["RateLimits"] = &types.AttributeValueMemberM{
			Value: map[string]types.AttributeValue{
				"MessagesPerMinute": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.RateLimits.MessagesPerMinute)},
				"MessagesPerHour":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.RateLimits.MessagesPerHour)},
				"BytesPerMinute":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.RateLimits.BytesPerMinute)},
				"BytesPerHour":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.RateLimits.BytesPerHour)},
				"BurstSize":         &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", instance.RateLimits.BurstSize)},
			},
		}
	}

	// Conditional put to prevent overwrites
	putInput := &dynamodb.PutItemInput{
		TableName:           aws.String(ir.tableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(PK)"),
	}

	_, err := ir.db.PutItem(ctx, putInput)
	if err != nil {
		return fmt.Errorf("register instance: %w", err)
	}

	// Update cache
	ir.cache.Store(instance.ID, &cachedInstance{
		instance: instance,
		cachedAt: time.Now(),
	})

	ir.logger.Info("registered instance",
		zap.String("instanceID", instance.ID),
		zap.String("domain", instance.Domain),
		zap.String("tier", string(instance.TierLevel)))

	return nil
}

// GetInstance retrieves an instance by ID with caching
func (ir *InstanceRegistry) GetInstance(ctx context.Context, instanceID string) (*Instance, error) {
	// Check cache first
	if cached, ok := ir.cache.Load(instanceID); ok {
		if ci, ok := cached.(*cachedInstance); ok && time.Since(ci.cachedAt) < ir.cacheTTL {
			return ci.instance, nil
		}
	}

	// Query DynamoDB
	getInput := &dynamodb.GetItemInput{
		TableName: aws.String(ir.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instanceID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		// Use consistent read for critical data
		ConsistentRead: aws.Bool(true),
	}

	result, err := ir.db.GetItem(ctx, getInput)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}

	if result.Item == nil {
		return nil, fmt.Errorf("instance not found: %s", instanceID)
	}

	instance, err := ir.parseInstance(result.Item)
	if err != nil {
		return nil, err
	}

	// Update cache
	ir.cache.Store(instanceID, &cachedInstance{
		instance: instance,
		cachedAt: time.Now(),
	})

	return instance, nil
}

// ListHealthyInstances returns all healthy instances using GSI
func (ir *InstanceRegistry) ListHealthyInstances(ctx context.Context) ([]*Instance, error) {
	// Query using GSI1 (status index)
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(ir.tableName),
		IndexName:              aws.String("GSI1"),
		KeyConditionExpression: aws.String("GSI1PK = :status"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status": &types.AttributeValueMemberS{Value: "STATUS#active"},
		},
		// Limit for performance
		Limit: aws.Int32(100),
	}

	instances := []*Instance{}

	// Paginate through results
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		if lastEvaluatedKey != nil {
			queryInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := ir.db.Query(ctx, queryInput)
		if err != nil {
			return nil, fmt.Errorf("list healthy instances: %w", err)
		}

		for _, item := range result.Items {
			instance, err := ir.parseInstance(item)
			if err != nil {
				ir.logger.Warn("failed to parse instance", zap.Error(err))
				continue
			}
			instances = append(instances, instance)
		}

		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return instances, nil
}

// UpdateInstanceHealth updates instance health metrics efficiently
func (ir *InstanceRegistry) UpdateInstanceHealth(ctx context.Context, instanceID string, health *HealthStatus) error {
	// Calculate new metrics based on health status
	status := InstanceStatusActive
	if !health.Reachable {
		status = InstanceStatusUnreachable
	} else if health.ErrorRate > 0.1 {
		status = InstanceStatusDegraded
	}

	// Use UpdateItem for atomic updates
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(ir.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instanceID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String(`
			SET #status = :status,
			    LastSeen = :lastseen,
			    AvgResponseTime = :avgrt,
			    ErrorRate = :errorrate,
			    GSI1PK = :gsi1pk
		`),
		ExpressionAttributeNames: map[string]string{
			"#status": "Status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":status":    &types.AttributeValueMemberS{Value: string(status)},
			":lastseen":  &types.AttributeValueMemberS{Value: time.Now().Format(time.RFC3339)},
			":avgrt":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", health.ResponseTime.Milliseconds())},
			":errorrate": &types.AttributeValueMemberN{Value: fmt.Sprintf("%.4f", health.ErrorRate)},
			":gsi1pk":    &types.AttributeValueMemberS{Value: fmt.Sprintf("STATUS#%s", status)},
		},
	}

	_, err := ir.db.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("update instance health: %w", err)
	}

	// Store health history
	if err := ir.storeHealthHistory(ctx, instanceID, health); err != nil {
		ir.logger.Warn("failed to store health history",
			zap.String("instanceID", instanceID),
			zap.Error(err))
	}

	// Invalidate cache
	ir.cache.Delete(instanceID)

	return nil
}

// GetInstancesByTier retrieves instances by tier level using GSI2
func (ir *InstanceRegistry) GetInstancesByTier(ctx context.Context, tier TierLevel, limit int) ([]*Instance, error) {
	queryInput := &dynamodb.QueryInput{
		TableName:              aws.String(ir.tableName),
		IndexName:              aws.String("GSI2"),
		KeyConditionExpression: aws.String("GSI2PK = :tier"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":tier": &types.AttributeValueMemberS{Value: fmt.Sprintf("TIER#%s", tier)},
		},
		// Sort by usage (ascending) to get least used first
		ScanIndexForward: aws.Bool(true),
		Limit:            aws.Int32(safeIntToInt32(limit)),
	}

	result, err := ir.db.Query(ctx, queryInput)
	if err != nil {
		return nil, fmt.Errorf("get instances by tier: %w", err)
	}

	instances := make([]*Instance, 0, len(result.Items))
	for _, item := range result.Items {
		instance, err := ir.parseInstance(item)
		if err != nil {
			continue
		}
		instances = append(instances, instance)
	}

	return instances, nil
}

// BatchGetInstances retrieves multiple instances efficiently
func (ir *InstanceRegistry) BatchGetInstances(ctx context.Context, instanceIDs []string) ([]*Instance, error) {
	if len(instanceIDs) == 0 {
		return []*Instance{}, nil
	}

	// Check cache first
	instances := make([]*Instance, 0, len(instanceIDs))
	uncachedIDs := []string{}

	for _, id := range instanceIDs {
		if cached, ok := ir.cache.Load(id); ok {
			if ci, ok := cached.(*cachedInstance); ok && time.Since(ci.cachedAt) < ir.cacheTTL {
				instances = append(instances, ci.instance)
				continue
			}
		}
		uncachedIDs = append(uncachedIDs, id)
	}

	// Batch get uncached instances
	if len(uncachedIDs) > 0 {
		// DynamoDB BatchGetItem supports max 100 items
		for i := 0; i < len(uncachedIDs); i += 100 {
			end := i + 100
			if end > len(uncachedIDs) {
				end = len(uncachedIDs)
			}

			keys := make([]map[string]types.AttributeValue, 0, end-i)
			for _, id := range uncachedIDs[i:end] {
				keys = append(keys, map[string]types.AttributeValue{
					"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", id)},
					"SK": &types.AttributeValueMemberS{Value: "METADATA"},
				})
			}

			batchInput := &dynamodb.BatchGetItemInput{
				RequestItems: map[string]types.KeysAndAttributes{
					ir.tableName: {
						Keys: keys,
					},
				},
			}

			result, err := ir.db.BatchGetItem(ctx, batchInput)
			if err != nil {
				return nil, fmt.Errorf("batch get instances: %w", err)
			}

			// Parse results
			if items, ok := result.Responses[ir.tableName]; ok {
				for _, item := range items {
					instance, err := ir.parseInstance(item)
					if err != nil {
						continue
					}
					instances = append(instances, instance)

					// Update cache
					ir.cache.Store(instance.ID, &cachedInstance{
						instance: instance,
						cachedAt: time.Now(),
					})
				}
			}
		}
	}

	return instances, nil
}

// UpdateInstanceUsage updates usage counters efficiently
func (ir *InstanceRegistry) UpdateInstanceUsage(ctx context.Context, instanceID string, bytesUsed int64) error {
	// Use atomic counter update
	updateInput := &dynamodb.UpdateItemInput{
		TableName: aws.String(ir.tableName),
		Key: map[string]types.AttributeValue{
			"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instanceID)},
			"SK": &types.AttributeValueMemberS{Value: "METADATA"},
		},
		UpdateExpression: aws.String(`
			ADD CurrentUsage :bytes
			SET GSI2SK = :gsi2sk
		`),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":bytes":  &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", bytesUsed)},
			":gsi2sk": &types.AttributeValueMemberS{Value: fmt.Sprintf("USAGE#%010d", bytesUsed)},
		},
		ReturnValues: types.ReturnValueAllNew,
	}

	result, err := ir.db.UpdateItem(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("update instance usage: %w", err)
	}

	// Check if quota exceeded
	if usage, ok := result.Attributes["CurrentUsage"]; ok {
		if quota, ok := result.Attributes["MonthlyQuota"]; ok {
			var currentUsage, monthlyQuota int64
			if _, err := fmt.Sscanf(usage.(*types.AttributeValueMemberN).Value, "%d", &currentUsage); err != nil {
				ir.logger.Warn("failed to parse current usage", zap.Error(err))
			}
			if _, err := fmt.Sscanf(quota.(*types.AttributeValueMemberN).Value, "%d", &monthlyQuota); err != nil {
				ir.logger.Warn("failed to parse monthly quota", zap.Error(err))
			}

			if currentUsage > monthlyQuota {
				// Queue for batch update
				ir.updateBatch <- &instanceUpdate{
					instanceID: instanceID,
					updates: map[string]interface{}{
						"Status": InstanceStatusBlocked,
						"GSI1PK": "STATUS#blocked",
					},
				}
			}
		}
	}

	return nil
}

// Helper methods

func (ir *InstanceRegistry) parseInstance(item map[string]types.AttributeValue) (*Instance, error) {
	instance := &Instance{}

	// Parse basic fields
	if v, ok := item["ID"].(*types.AttributeValueMemberS); ok {
		instance.ID = v.Value
	}
	if v, ok := item["Domain"].(*types.AttributeValueMemberS); ok {
		instance.Domain = v.Value
	}
	if v, ok := item["InboxURL"].(*types.AttributeValueMemberS); ok {
		instance.InboxURL = v.Value
	}
	if v, ok := item["SharedInboxURL"].(*types.AttributeValueMemberS); ok {
		instance.SharedInboxURL = v.Value
	}
	if v, ok := item["PublicKeyPEM"].(*types.AttributeValueMemberS); ok {
		instance.PublicKeyPEM = v.Value
	}

	// Parse status
	if v, ok := item["Status"].(*types.AttributeValueMemberS); ok {
		instance.Status = InstanceStatus(v.Value)
	}

	// Parse timestamps
	if v, ok := item["LastSeen"].(*types.AttributeValueMemberS); ok {
		instance.LastSeen, _ = time.Parse(time.RFC3339, v.Value)
	}
	if v, ok := item["RegisteredAt"].(*types.AttributeValueMemberS); ok {
		instance.RegisteredAt, _ = time.Parse(time.RFC3339, v.Value)
	}

	// Parse metrics
	if v, ok := item["AvgResponseTime"].(*types.AttributeValueMemberN); ok {
		var ms int64
		if _, err := fmt.Sscanf(v.Value, "%d", &ms); err != nil {
			ir.logger.Warn("failed to parse AvgResponseTime", zap.String("value", v.Value), zap.Error(err))
		}
		instance.AvgResponseTime = time.Duration(ms) * time.Millisecond
	}
	if v, ok := item["SuccessRate"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &instance.SuccessRate); err != nil {
			ir.logger.Warn("failed to parse SuccessRate", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["ErrorRate"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%f", &instance.ErrorRate); err != nil {
			ir.logger.Warn("failed to parse ErrorRate", zap.String("value", v.Value), zap.Error(err))
		}
	}

	// Parse cost tracking
	if v, ok := item["TierLevel"].(*types.AttributeValueMemberS); ok {
		instance.TierLevel = TierLevel(v.Value)
	}
	if v, ok := item["MonthlyQuota"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &instance.MonthlyQuota); err != nil {
			ir.logger.Warn("failed to parse MonthlyQuota", zap.String("value", v.Value), zap.Error(err))
		}
	}
	if v, ok := item["CurrentUsage"].(*types.AttributeValueMemberN); ok {
		if _, err := fmt.Sscanf(v.Value, "%d", &instance.CurrentUsage); err != nil {
			ir.logger.Warn("failed to parse CurrentUsage", zap.String("value", v.Value), zap.Error(err))
		}
	}

	// Parse capabilities
	if v, ok := item["SupportedTypes"].(*types.AttributeValueMemberL); ok {
		instance.SupportedTypes = make([]MessageType, 0, len(v.Value))
		for _, t := range v.Value {
			if s, ok := t.(*types.AttributeValueMemberS); ok {
				instance.SupportedTypes = append(instance.SupportedTypes, MessageType(s.Value))
			}
		}
	}

	// Parse rate limits
	if v, ok := item["RateLimits"].(*types.AttributeValueMemberM); ok {
		limits := &RateLimits{}
		if rpm, ok := v.Value["MessagesPerMinute"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(rpm.Value, "%d", &limits.MessagesPerMinute); err != nil {
				ir.logger.Warn("failed to parse MessagesPerMinute", zap.String("value", rpm.Value), zap.Error(err))
			}
		}
		if rph, ok := v.Value["MessagesPerHour"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(rph.Value, "%d", &limits.MessagesPerHour); err != nil {
				ir.logger.Warn("failed to parse MessagesPerHour", zap.String("value", rph.Value), zap.Error(err))
			}
		}
		if bpm, ok := v.Value["BytesPerMinute"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(bpm.Value, "%d", &limits.BytesPerMinute); err != nil {
				ir.logger.Warn("failed to parse BytesPerMinute", zap.String("value", bpm.Value), zap.Error(err))
			}
		}
		if bph, ok := v.Value["BytesPerHour"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(bph.Value, "%d", &limits.BytesPerHour); err != nil {
				ir.logger.Warn("failed to parse BytesPerHour", zap.String("value", bph.Value), zap.Error(err))
			}
		}
		if bs, ok := v.Value["BurstSize"].(*types.AttributeValueMemberN); ok {
			if _, err := fmt.Sscanf(bs.Value, "%d", &limits.BurstSize); err != nil {
				ir.logger.Warn("failed to parse BurstSize", zap.String("value", bs.Value), zap.Error(err))
			}
		}
		instance.RateLimits = *limits
	}

	return instance, nil
}

func (ir *InstanceRegistry) storeHealthHistory(ctx context.Context, instanceID string, health *HealthStatus) error {
	// Store with time-based sort key for efficient queries
	item := map[string]types.AttributeValue{
		"PK": &types.AttributeValueMemberS{Value: fmt.Sprintf("INSTANCE#%s", instanceID)},
		"SK": &types.AttributeValueMemberS{Value: fmt.Sprintf("HEALTH#%d", health.Timestamp.UnixNano())},

		"Reachable":       &types.AttributeValueMemberBOOL{Value: health.Reachable},
		"ResponseTime":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", health.ResponseTime.Milliseconds())},
		"StatusCode":      &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", health.StatusCode)},
		"ErrorRate":       &types.AttributeValueMemberN{Value: fmt.Sprintf("%.4f", health.ErrorRate)},
		"InboxBacklog":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", health.InboxBacklog)},
		"ProcessingDelay": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", health.ProcessingDelay.Milliseconds())},

		// TTL for automatic cleanup (keep 7 days)
		"TTL": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", time.Now().Add(7*24*time.Hour).Unix())},
	}

	if health.ErrorMessage != "" {
		item["ErrorMessage"] = &types.AttributeValueMemberS{Value: health.ErrorMessage}
	}

	putInput := &dynamodb.PutItemInput{
		TableName: aws.String(ir.tableName),
		Item:      item,
	}

	_, err := ir.db.PutItem(ctx, putInput)
	return err
}

func (ir *InstanceRegistry) processBatchWrites() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	batch := make([]*Instance, 0, 25)

	for {
		select {
		case instance := <-ir.writeBatch:
			batch = append(batch, instance)

			// Process batch when full or on ticker
			if len(batch) >= 25 {
				ir.writeBatchToDynamoDB(batch)
				batch = batch[:0]
			}

		case <-ticker.C:
			if len(batch) > 0 {
				ir.writeBatchToDynamoDB(batch)
				batch = batch[:0]
			}
		}
	}
}

func (ir *InstanceRegistry) processBatchUpdates() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	updates := make([]*instanceUpdate, 0, 25)

	for {
		select {
		case update := <-ir.updateBatch:
			updates = append(updates, update)

			if len(updates) >= 25 {
				ir.applyBatchUpdates(updates)
				updates = updates[:0]
			}

		case <-ticker.C:
			if len(updates) > 0 {
				ir.applyBatchUpdates(updates)
				updates = updates[:0]
			}
		}
	}
}

func (ir *InstanceRegistry) writeBatchToDynamoDB(instances []*Instance) {
	// Convert to batch write requests
	writeRequests := make([]types.WriteRequest, 0, len(instances))

	for _, instance := range instances {
		// Convert instance to DynamoDB item (similar to RegisterInstance)
		// Omitted for brevity
		_ = instance // Mark as used
	}

	batchInput := &dynamodb.BatchWriteItemInput{
		RequestItems: map[string][]types.WriteRequest{
			ir.tableName: writeRequests,
		},
	}

	ctx := context.Background()
	_, err := ir.db.BatchWriteItem(ctx, batchInput)
	if err != nil {
		ir.logger.Error("batch write failed", zap.Error(err))
	}
}

func (ir *InstanceRegistry) applyBatchUpdates(updates []*instanceUpdate) {
	// Group updates by instance for efficiency
	// Apply using BatchWriteItem or TransactWrite
	// Implementation omitted for brevity
	_ = updates // Mark as used
}

func generateInstanceID(domain string) string {
	return fmt.Sprintf("%s-%d", domain, time.Now().Unix())
}
