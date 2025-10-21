package schema

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
)

// AutoMigrateOptions holds configuration for AutoMigrate operations
type AutoMigrateOptions struct {
	// BackupTable specifies a table name to backup data to before migration
	BackupTable string

	// DataCopy enables copying data from source to target table
	DataCopy bool

	// TargetModel specifies a different model to migrate data to
	TargetModel any

	// Transform is a function to transform data during copy
	Transform interface{}

	// BatchSize for data copy operations
	BatchSize int

	// Context for the operation
	Context context.Context
}

// AutoMigrateOption is a function that configures AutoMigrateOptions
type AutoMigrateOption func(*AutoMigrateOptions)

// WithBackupTable sets a backup table name
func WithBackupTable(tableName string) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BackupTable = tableName
	}
}

// WithDataCopy enables data copying
func WithDataCopy(enable bool) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.DataCopy = enable
	}
}

// WithTargetModel sets a different target model for migration
func WithTargetModel(model any) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.TargetModel = model
	}
}

// WithTransform sets a transformation function for data migration
func WithTransform(transform interface{}) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Transform = transform
	}
}

// WithBatchSize sets the batch size for data copy operations
func WithBatchSize(size int) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.BatchSize = size
	}
}

// WithContext sets the context for the operation
func WithContext(ctx context.Context) AutoMigrateOption {
	return func(opts *AutoMigrateOptions) {
		opts.Context = ctx
	}
}

// AutoMigrateWithOptions performs an enhanced auto-migration with data copy support
func (m *Manager) AutoMigrateWithOptions(sourceModel any, options ...AutoMigrateOption) error {
	// Apply options
	opts := &AutoMigrateOptions{
		BatchSize: 25, // Default batch size
		Context:   context.Background(),
	}
	for _, opt := range options {
		opt(opts)
	}

	// Register source model
	if err := m.registry.Register(sourceModel); err != nil {
		return fmt.Errorf("failed to register source model: %w", err)
	}

	sourceMetadata, err := m.registry.GetMetadata(sourceModel)
	if err != nil {
		return fmt.Errorf("failed to get source metadata: %w", err)
	}

	// Determine target model
	targetModel := sourceModel
	if opts.TargetModel != nil {
		targetModel = opts.TargetModel
		if err := m.registry.Register(targetModel); err != nil {
			return fmt.Errorf("failed to register target model: %w", err)
		}
	}

	targetMetadata, err := m.registry.GetMetadata(targetModel)
	if err != nil {
		return fmt.Errorf("failed to get target metadata: %w", err)
	}

	// Handle backup if requested
	if opts.BackupTable != "" {
		if err := m.createBackup(opts.Context, sourceMetadata.TableName, opts.BackupTable); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Create target table if it doesn't exist
	exists, err := m.TableExists(targetMetadata.TableName)
	if err != nil {
		return fmt.Errorf("failed to check target table existence: %w", err)
	}

	if !exists {
		if err := m.CreateTable(targetModel); err != nil {
			return fmt.Errorf("failed to create target table: %w", err)
		}
	}

	// Copy data if requested
	if opts.DataCopy && sourceMetadata.TableName != targetMetadata.TableName {
		// Validate transform function early, before processing any data
		if opts.Transform != nil {
			_, err := CreateModelTransform(opts.Transform, sourceMetadata, targetMetadata)
			if err != nil {
				return fmt.Errorf("invalid transform function: %w", err)
			}
		}

		if err := m.copyData(opts, sourceMetadata, targetMetadata); err != nil {
			return fmt.Errorf("failed to copy data: %w", err)
		}
	}

	return nil
}

// createBackup creates a point-in-time backup of the table
func (m *Manager) createBackup(ctx context.Context, sourceTable, backupName string) error {
	// Check if source table exists
	exists, err := m.TableExists(sourceTable)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("source table %s does not exist", sourceTable)
	}

	// Create backup using DynamoDB's backup feature
	backupRequest := &dynamodb.CreateBackupInput{
		TableName:  &sourceTable,
		BackupName: &backupName,
	}

	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for backup creation: %w", err)
	}

	_, err = client.CreateBackup(ctx, backupRequest)
	if err != nil {
		// If backup fails, try table copy instead
		return m.copyTable(ctx, sourceTable, backupName)
	}

	return nil
}

// copyTable creates a copy of a table
func (m *Manager) copyTable(ctx context.Context, sourceTable, targetTable string) error {
	// Get source table description
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table description: %w", err)
	}

	// Check if target table already exists and delete it
	exists, err := m.TableExists(targetTable)
	if err != nil {
		return fmt.Errorf("failed to check if backup table exists: %w", err)
	}
	if exists {
		// Delete existing backup table
		_, err = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
			TableName: &targetTable,
		})
		if err != nil {
			return fmt.Errorf("failed to delete existing backup table: %w", err)
		}

		// Wait for table to be deleted
		waiter := dynamodb.NewTableNotExistsWaiter(client)
		if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
			TableName: &targetTable,
		}, 2*time.Minute); err != nil {
			return fmt.Errorf("timeout waiting for table deletion: %w", err)
		}
	}

	desc, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: &sourceTable,
	})
	if err != nil {
		return fmt.Errorf("failed to describe source table: %w", err)
	}

	// Create target table with same schema
	createInput := &dynamodb.CreateTableInput{
		TableName:            &targetTable,
		KeySchema:            desc.Table.KeySchema,
		AttributeDefinitions: desc.Table.AttributeDefinitions,
		BillingMode:          desc.Table.BillingModeSummary.BillingMode,
	}

	// Convert GlobalSecondaryIndexDescriptions to GlobalSecondaryIndexes
	if len(desc.Table.GlobalSecondaryIndexes) > 0 {
		gsis := make([]types.GlobalSecondaryIndex, len(desc.Table.GlobalSecondaryIndexes))
		for i, gsi := range desc.Table.GlobalSecondaryIndexes {
			gsis[i] = types.GlobalSecondaryIndex{
				IndexName:  gsi.IndexName,
				KeySchema:  gsi.KeySchema,
				Projection: gsi.Projection,
			}
			if gsi.ProvisionedThroughput != nil {
				gsis[i].ProvisionedThroughput = &types.ProvisionedThroughput{
					ReadCapacityUnits:  gsi.ProvisionedThroughput.ReadCapacityUnits,
					WriteCapacityUnits: gsi.ProvisionedThroughput.WriteCapacityUnits,
				}
			}
		}
		createInput.GlobalSecondaryIndexes = gsis
	}

	// Convert LocalSecondaryIndexDescriptions to LocalSecondaryIndexes
	if len(desc.Table.LocalSecondaryIndexes) > 0 {
		lsis := make([]types.LocalSecondaryIndex, len(desc.Table.LocalSecondaryIndexes))
		for i, lsi := range desc.Table.LocalSecondaryIndexes {
			lsis[i] = types.LocalSecondaryIndex{
				IndexName:  lsi.IndexName,
				KeySchema:  lsi.KeySchema,
				Projection: lsi.Projection,
			}
		}
		createInput.LocalSecondaryIndexes = lsis
	}

	if desc.Table.ProvisionedThroughput != nil {
		createInput.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  desc.Table.ProvisionedThroughput.ReadCapacityUnits,
			WriteCapacityUnits: desc.Table.ProvisionedThroughput.WriteCapacityUnits,
		}
	}

	_, err = client.CreateTable(ctx, createInput)
	if err != nil {
		return fmt.Errorf("failed to create target table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: &targetTable,
	}, 5*time.Minute); err != nil {
		return fmt.Errorf("timeout waiting for table creation: %w", err)
	}

	// Copy data
	return m.copyTableData(ctx, sourceTable, targetTable, 25)
}

// copyData copies data from source to target table with optional transformation
func (m *Manager) copyData(opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata) error {
	ctx := opts.Context

	// Get client once for the entire operation
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for data copy: %w", err)
	}

	// Scan source table
	var lastEvaluatedKey map[string]types.AttributeValue
	for {
		scanInput := &dynamodb.ScanInput{
			TableName: &sourceMetadata.TableName,
			Limit:     int32Ptr(int32(opts.BatchSize)),
		}
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := client.Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}

		// Process items
		if len(result.Items) > 0 {
			if err := m.processItems(ctx, client, result.Items, opts, sourceMetadata, targetMetadata); err != nil {
				return fmt.Errorf("failed to process items: %w", err)
			}
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

// processItems processes and writes items to the target table
func (m *Manager) processItems(ctx context.Context, client *dynamodb.Client, items []map[string]types.AttributeValue,
	opts *AutoMigrateOptions, sourceMetadata, targetMetadata *model.Metadata) error {

	// Prepare batch write requests
	writeRequests := make([]types.WriteRequest, 0, len(items))

	for _, item := range items {
		// Apply transformation if provided
		transformedItem := item
		if opts.Transform != nil {
			var err error
			transformedItem, err = m.applyTransform(item, opts.Transform, sourceMetadata, targetMetadata)
			if err != nil {
				return fmt.Errorf("failed to transform item: %w", err)
			}
		}

		writeRequests = append(writeRequests, types.WriteRequest{
			PutRequest: &types.PutRequest{
				Item: transformedItem,
			},
		})
	}

	// Process in batches of 25 (DynamoDB limit)
	const maxBatchSize = 25
	for i := 0; i < len(writeRequests); i += maxBatchSize {
		end := i + maxBatchSize
		if end > len(writeRequests) {
			end = len(writeRequests)
		}

		batch := writeRequests[i:end]
		remainingRequests := batch
		retryCount := 0
		maxRetries := 5
		allProcessed := false

		for len(remainingRequests) > 0 && retryCount < maxRetries {
			batchInput := &dynamodb.BatchWriteItemInput{
				RequestItems: map[string][]types.WriteRequest{
					targetMetadata.TableName: remainingRequests,
				},
			}

			result, err := client.BatchWriteItem(ctx, batchInput)
			if err != nil {
				return fmt.Errorf("failed to write items to target table: %w", err)
			}

			// Check for unprocessed items
			if result.UnprocessedItems != nil && len(result.UnprocessedItems) > 0 {
				if unprocessed, exists := result.UnprocessedItems[targetMetadata.TableName]; exists && len(unprocessed) > 0 {
					remainingRequests = unprocessed
					retryCount++

					// Add exponential backoff with jitter
					if retryCount < maxRetries {
						backoff := time.Duration(retryCount*retryCount) * 100 * time.Millisecond
						select {
						case <-time.After(backoff):
							// Continue with retry
						case <-ctx.Done():
							return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
						}
					}
				} else {
					// No unprocessed items for our table
					allProcessed = true
					break
				}
			} else {
				// All items processed successfully
				allProcessed = true
				break
			}
		}

		// If we still have unprocessed items after retries, try individual puts
		// This is more compatible with DynamoDB Local
		if !allProcessed && len(remainingRequests) > 0 {
			for _, req := range remainingRequests {
				if req.PutRequest != nil {
					putInput := &dynamodb.PutItemInput{
						TableName: &targetMetadata.TableName,
						Item:      req.PutRequest.Item,
					}
					_, err := client.PutItem(ctx, putInput)
					if err != nil {
						return fmt.Errorf("failed to put individual item after batch failures: %w", err)
					}
				}
			}
		}
	}

	return nil
}

// applyTransform applies a transformation function to an item
func (m *Manager) applyTransform(item map[string]types.AttributeValue, transform interface{},
	sourceMetadata, targetMetadata *model.Metadata) (map[string]types.AttributeValue, error) {

	// Create the appropriate transform function
	transformFunc, err := CreateModelTransform(transform, sourceMetadata, targetMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to create transform function: %w", err)
	}

	// Apply the transform with validation
	return TransformWithValidation(item, transformFunc, sourceMetadata, targetMetadata)
}

// copyTableData copies all data from source to target table
func (m *Manager) copyTableData(ctx context.Context, sourceTable, targetTable string, batchSize int) error {
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table data copy: %w", err)
	}

	var lastEvaluatedKey map[string]types.AttributeValue

	for {
		// Scan source table
		scanInput := &dynamodb.ScanInput{
			TableName: &sourceTable,
			Limit:     int32Ptr(int32(batchSize)),
		}
		if lastEvaluatedKey != nil {
			scanInput.ExclusiveStartKey = lastEvaluatedKey
		}

		result, err := client.Scan(ctx, scanInput)
		if err != nil {
			return fmt.Errorf("failed to scan source table: %w", err)
		}

		// Batch write to target table
		if len(result.Items) > 0 {
			// Use smaller batch size for DynamoDB Local compatibility
			maxBatchSize := 10
			if batchSize < maxBatchSize {
				maxBatchSize = batchSize
			}

			for i := 0; i < len(result.Items); i += maxBatchSize {
				end := i + maxBatchSize
				if end > len(result.Items) {
					end = len(result.Items)
				}

				batch := result.Items[i:end]
				writeRequests := make([]types.WriteRequest, len(batch))
				for j, item := range batch {
					writeRequests[j] = types.WriteRequest{
						PutRequest: &types.PutRequest{
							Item: item,
						},
					}
				}

				// Batch write with retry logic for unprocessed items
				remainingRequests := writeRequests
				retryCount := 0
				maxRetries := 5

				for len(remainingRequests) > 0 && retryCount < maxRetries {
					batchInput := &dynamodb.BatchWriteItemInput{
						RequestItems: map[string][]types.WriteRequest{
							targetTable: remainingRequests,
						},
					}

					batchResult, err := client.BatchWriteItem(ctx, batchInput)
					if err != nil {
						return fmt.Errorf("failed to write batch: %w", err)
					}

					// Check for unprocessed items
					if batchResult.UnprocessedItems != nil && len(batchResult.UnprocessedItems) > 0 {
						if unprocessed, exists := batchResult.UnprocessedItems[targetTable]; exists && len(unprocessed) > 0 {
							remainingRequests = unprocessed
							retryCount++

							// Add exponential backoff with jitter
							if retryCount < maxRetries {
								backoff := time.Duration(retryCount*retryCount) * 100 * time.Millisecond
								select {
								case <-time.After(backoff):
									// Continue with retry
								case <-ctx.Done():
									return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
								}
							}
						} else {
							// No unprocessed items for our table
							break
						}
					} else {
						// All items processed successfully
						break
					}
				}

				// If we still have unprocessed items after retries, log and continue
				// This is more lenient for DynamoDB Local compatibility
				if len(remainingRequests) > 0 {
					// For DynamoDB Local, we'll try individual puts as fallback
					for _, req := range remainingRequests {
						if req.PutRequest != nil {
							putInput := &dynamodb.PutItemInput{
								TableName: &targetTable,
								Item:      req.PutRequest.Item,
							}
							_, err := client.PutItem(ctx, putInput)
							if err != nil {
								return fmt.Errorf("failed to put individual item after batch failures: %w", err)
							}
						}
					}
				}
			}
		}

		// Check if more items
		lastEvaluatedKey = result.LastEvaluatedKey
		if lastEvaluatedKey == nil {
			break
		}
	}

	return nil
}

// Helper function
func int32Ptr(i int32) *int32 {
	return &i
}
