package migrations

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// Projection type constants
const (
	projectionTypeInclude = "INCLUDE"
)

// GSIHelper provides utilities for managing Global Secondary Indexes
type GSIHelper struct {
	client    *dynamodb.DynamoDB
	tableName string
	logger    *zap.Logger
}

// NewGSIHelper creates a new GSI helper
func NewGSIHelper(tableName string, logger *zap.Logger) (*GSIHelper, error) {
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	return &GSIHelper{
		client:    dynamodb.New(sess),
		tableName: tableName,
		logger:    logger,
	}, nil
}

// GSIDefinition defines a Global Secondary Index
type GSIDefinition struct {
	Name           string
	HashKey        string
	HashKeyType    string // "S", "N", "B"
	RangeKey       string
	RangeKeyType   string // "S", "N", "B"
	ProjectionType string // "ALL", "KEYS_ONLY", "INCLUDE"
	IncludeFields  []string
	ReadCapacity   int64
	WriteCapacity  int64
}

// CreateGSI creates a new Global Secondary Index
func (h *GSIHelper) CreateGSI(ctx context.Context, gsi GSIDefinition) error {
	h.logger.Info("Creating GSI",
		zap.String("table", h.tableName),
		zap.String("gsi", gsi.Name))

	// First, describe the table to get current state
	describeOutput, err := h.client.DescribeTableWithContext(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(h.tableName),
	})
	if err != nil {
		return fmt.Errorf("failed to describe table: %w", err)
	}

	// Check if GSI already exists
	for _, existingGSI := range describeOutput.Table.GlobalSecondaryIndexes {
		if *existingGSI.IndexName == gsi.Name {
			return fmt.Errorf("GSI %s already exists", gsi.Name)
		}
	}

	// Build attribute definitions (only for new attributes)
	attributeDefinitions := []*dynamodb.AttributeDefinition{}
	existingAttrs := make(map[string]bool)

	// Track existing attributes
	for _, attr := range describeOutput.Table.AttributeDefinitions {
		existingAttrs[*attr.AttributeName] = true
	}

	// Add hash key if not exists
	if !existingAttrs[gsi.HashKey] {
		attributeDefinitions = append(attributeDefinitions, &dynamodb.AttributeDefinition{
			AttributeName: aws.String(gsi.HashKey),
			AttributeType: aws.String(gsi.HashKeyType),
		})
	}

	// Add range key if not exists
	if gsi.RangeKey != "" && !existingAttrs[gsi.RangeKey] {
		attributeDefinitions = append(attributeDefinitions, &dynamodb.AttributeDefinition{
			AttributeName: aws.String(gsi.RangeKey),
			AttributeType: aws.String(gsi.RangeKeyType),
		})
	}

	// Build key schema
	keySchema := []*dynamodb.KeySchemaElement{
		{
			AttributeName: aws.String(gsi.HashKey),
			KeyType:       aws.String("HASH"),
		},
	}

	// Add range key if specified
	if gsi.RangeKey != "" {
		keySchema = append(keySchema, &dynamodb.KeySchemaElement{
			AttributeName: aws.String(gsi.RangeKey),
			KeyType:       aws.String("RANGE"),
		})
	}

	// Build GSI creation input
	gsiCreate := &dynamodb.CreateGlobalSecondaryIndexAction{
		IndexName: aws.String(gsi.Name),
		KeySchema: keySchema,
		Projection: &dynamodb.Projection{
			ProjectionType: aws.String(gsi.ProjectionType),
		},
		ProvisionedThroughput: &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(gsi.ReadCapacity),
			WriteCapacityUnits: aws.Int64(gsi.WriteCapacity),
		},
	}

	// Add included fields for INCLUDE projection
	if gsi.ProjectionType == projectionTypeInclude && len(gsi.IncludeFields) > 0 {
		nonKeyAttributes := make([]*string, len(gsi.IncludeFields))
		for i, field := range gsi.IncludeFields {
			nonKeyAttributes[i] = aws.String(field)
		}
		gsiCreate.Projection.NonKeyAttributes = nonKeyAttributes
	}

	// Set provisioned throughput if not on-demand
	if *describeOutput.Table.BillingModeSummary.BillingMode != "PAY_PER_REQUEST" {
		gsiCreate.ProvisionedThroughput = &dynamodb.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(gsi.ReadCapacity),
			WriteCapacityUnits: aws.Int64(gsi.WriteCapacity),
		}
	}

	// Update table with new GSI
	updateInput := &dynamodb.UpdateTableInput{
		TableName:            aws.String(h.tableName),
		AttributeDefinitions: attributeDefinitions,
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{
			{
				Create: gsiCreate,
			},
		},
	}

	_, err = h.client.UpdateTableWithContext(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to create GSI: %w", err)
	}

	// Wait for GSI to be active
	return h.waitForGSIActive(ctx, gsi.Name)
}

// DeleteGSI deletes a Global Secondary Index
func (h *GSIHelper) DeleteGSI(ctx context.Context, gsiName string) error {
	h.logger.Info("Deleting GSI",
		zap.String("table", h.tableName),
		zap.String("gsi", gsiName))

	updateInput := &dynamodb.UpdateTableInput{
		TableName: aws.String(h.tableName),
		GlobalSecondaryIndexUpdates: []*dynamodb.GlobalSecondaryIndexUpdate{
			{
				Delete: &dynamodb.DeleteGlobalSecondaryIndexAction{
					IndexName: aws.String(gsiName),
				},
			},
		},
	}

	_, err := h.client.UpdateTableWithContext(ctx, updateInput)
	if err != nil {
		return fmt.Errorf("failed to delete GSI: %w", err)
	}

	// Wait for deletion to complete
	return h.waitForGSIDeletion(ctx, gsiName)
}

// waitForGSIActive waits for a GSI to become active
func (h *GSIHelper) waitForGSIActive(ctx context.Context, gsiName string) error {
	h.logger.Info("Waiting for GSI to become active",
		zap.String("table", h.tableName),
		zap.String("gsi", gsiName))

	maxAttempts := 60 // 10 minutes with 10-second intervals
	for i := 0; i < maxAttempts; i++ {
		describeOutput, err := h.client.DescribeTableWithContext(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(h.tableName),
		})
		if err != nil {
			return fmt.Errorf("failed to describe table: %w", err)
		}

		// Check GSI status
		for _, gsi := range describeOutput.Table.GlobalSecondaryIndexes {
			if *gsi.IndexName == gsiName {
				if *gsi.IndexStatus == StatusActive {
					h.logger.Info("GSI is now active",
						zap.String("table", h.tableName),
						zap.String("gsi", gsiName))
					return nil
				}
				h.logger.Debug("GSI status",
					zap.String("status", *gsi.IndexStatus),
					zap.Int("attempt", i+1))
				break
			}
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			// Continue waiting
		}
	}

	return fmt.Errorf("timeout waiting for GSI %s to become active", gsiName)
}

// waitForGSIDeletion waits for a GSI to be deleted
func (h *GSIHelper) waitForGSIDeletion(ctx context.Context, gsiName string) error {
	h.logger.Info("Waiting for GSI deletion to complete",
		zap.String("table", h.tableName),
		zap.String("gsi", gsiName))

	maxAttempts := 60 // 10 minutes with 10-second intervals
	for i := 0; i < maxAttempts; i++ {
		describeOutput, err := h.client.DescribeTableWithContext(ctx, &dynamodb.DescribeTableInput{
			TableName: aws.String(h.tableName),
		})
		if err != nil {
			return fmt.Errorf("failed to describe table: %w", err)
		}

		// Check if GSI still exists
		found := false
		for _, gsi := range describeOutput.Table.GlobalSecondaryIndexes {
			if *gsi.IndexName == gsiName {
				found = true
				h.logger.Debug("GSI still exists",
					zap.String("status", *gsi.IndexStatus),
					zap.Int("attempt", i+1))
				break
			}
		}

		if !found {
			h.logger.Info("GSI deletion completed",
				zap.String("table", h.tableName),
				zap.String("gsi", gsiName))
			return nil
		}

		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			// Continue waiting
		}
	}

	return fmt.Errorf("timeout waiting for GSI %s deletion", gsiName)
}

// GSIMigration is a helper base for GSI-related migrations
type GSIMigration struct {
	BaseMigration
	TableName string
	GSI       GSIDefinition
}

// NewGSIMigration creates a new GSI migration
func NewGSIMigration(id string, version int64, description string, tableName string, gsi GSIDefinition, dependencies ...string) GSIMigration {
	return GSIMigration{
		BaseMigration: NewBaseMigration(id, version, description, dependencies...),
		TableName:     tableName,
		GSI:           gsi,
	}
}

// Up creates the GSI
func (m GSIMigration) Up(ctx context.Context, _ core.DB) error {
	logger, _ := zap.NewProduction()
	helper, err := NewGSIHelper(m.TableName, logger)
	if err != nil {
		return err
	}

	return helper.CreateGSI(ctx, m.GSI)
}

// Down removes the GSI
func (m GSIMigration) Down(ctx context.Context, _ core.DB) error {
	logger, _ := zap.NewProduction()
	helper, err := NewGSIHelper(m.TableName, logger)
	if err != nil {
		return err
	}

	return helper.DeleteGSI(ctx, m.GSI.Name)
}
