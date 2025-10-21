package schema

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
)

// Manager handles DynamoDB table schema operations
type Manager struct {
	session  *session.Session
	registry *model.Registry
}

// NewManager creates a new schema manager
func NewManager(session *session.Session, registry *model.Registry) *Manager {
	return &Manager{
		session:  session,
		registry: registry,
	}
}

// TableOption configures table creation options
type TableOption func(*dynamodb.CreateTableInput)

// WithBillingMode sets the billing mode for the table
func WithBillingMode(mode types.BillingMode) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.BillingMode = mode
		// If provisioned, remove any existing throughput settings
		if mode == types.BillingModePayPerRequest {
			input.ProvisionedThroughput = nil
		}
	}
}

// WithThroughput sets provisioned throughput for the table
func WithThroughput(rcu, wcu int64) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.BillingMode = types.BillingModeProvisioned
		input.ProvisionedThroughput = &types.ProvisionedThroughput{
			ReadCapacityUnits:  aws.Int64(rcu),
			WriteCapacityUnits: aws.Int64(wcu),
		}
	}
}

// WithStreamSpecification enables DynamoDB streams
func WithStreamSpecification(spec types.StreamSpecification) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.StreamSpecification = &spec
	}
}

// WithSSESpecification enables server-side encryption
func WithSSESpecification(spec types.SSESpecification) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		input.SSESpecification = &spec
	}
}

// CreateTable creates a DynamoDB table based on the model struct
func (m *Manager) CreateTable(model any, opts ...TableOption) error {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	input := &dynamodb.CreateTableInput{
		TableName:   aws.String(metadata.TableName),
		BillingMode: types.BillingModePayPerRequest, // Default to on-demand
	}

	// Build key schema
	input.KeySchema = m.buildKeySchema(metadata)

	// Build attribute definitions
	input.AttributeDefinitions = m.buildAttributeDefinitions(metadata)

	// Build GSI/LSI from unified indexes
	gsiList, lsiList := m.buildIndexes(metadata)
	if len(gsiList) > 0 {
		input.GlobalSecondaryIndexes = gsiList
	}
	if len(lsiList) > 0 {
		input.LocalSecondaryIndexes = lsiList
	}

	// Apply options
	for _, opt := range opts {
		opt(input)
	}

	// Create table
	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table creation: %w", err)
	}

	_, err = client.CreateTable(ctx, input)
	if err != nil {
		// Check if table already exists
		var existsErr *types.ResourceInUseException
		if errors.As(err, &existsErr) {
			// Table already exists, which is fine
			return nil
		}
		return fmt.Errorf("failed to create table %s: %w", metadata.TableName, err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(client)
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(metadata.TableName),
	}, 5*time.Minute)
}

// buildKeySchema builds the primary key schema
func (m *Manager) buildKeySchema(metadata *model.Metadata) []types.KeySchemaElement {
	schema := []types.KeySchemaElement{
		{
			AttributeName: aws.String(metadata.PrimaryKey.PartitionKey.DBName),
			KeyType:       types.KeyTypeHash,
		},
	}

	if metadata.PrimaryKey.SortKey != nil {
		schema = append(schema, types.KeySchemaElement{
			AttributeName: aws.String(metadata.PrimaryKey.SortKey.DBName),
			KeyType:       types.KeyTypeRange,
		})
	}

	return schema
}

// buildAttributeDefinitions builds attribute definitions for all keys
func (m *Manager) buildAttributeDefinitions(metadata *model.Metadata) []types.AttributeDefinition {
	// Use a map to avoid duplicates
	attrs := make(map[string]types.ScalarAttributeType)

	// Primary key attributes
	attrs[metadata.PrimaryKey.PartitionKey.DBName] = m.getAttributeType(metadata.PrimaryKey.PartitionKey.Type.Kind())
	if metadata.PrimaryKey.SortKey != nil {
		attrs[metadata.PrimaryKey.SortKey.DBName] = m.getAttributeType(metadata.PrimaryKey.SortKey.Type.Kind())
	}

	// Index attributes
	for _, index := range metadata.Indexes {
		if index.PartitionKey != nil {
			attrs[index.PartitionKey.DBName] = m.getAttributeType(index.PartitionKey.Type.Kind())
		}
		if index.SortKey != nil {
			attrs[index.SortKey.DBName] = m.getAttributeType(index.SortKey.Type.Kind())
		}
	}

	// Convert map to slice
	definitions := make([]types.AttributeDefinition, 0, len(attrs))
	for name, attrType := range attrs {
		definitions = append(definitions, types.AttributeDefinition{
			AttributeName: aws.String(name),
			AttributeType: attrType,
		})
	}

	return definitions
}

// getAttributeType converts Go reflect.Kind to DynamoDB attribute type
func (m *Manager) getAttributeType(kind reflect.Kind) types.ScalarAttributeType {
	switch kind {
	case reflect.String:
		return types.ScalarAttributeTypeS
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return types.ScalarAttributeTypeN
	case reflect.Slice:
		return types.ScalarAttributeTypeB
	default:
		return types.ScalarAttributeTypeS
	}
}

// buildIndexes separates and builds GSI and LSI from metadata
func (m *Manager) buildIndexes(metadata *model.Metadata) ([]types.GlobalSecondaryIndex, []types.LocalSecondaryIndex) {
	var gsiList []types.GlobalSecondaryIndex
	var lsiList []types.LocalSecondaryIndex

	for _, index := range metadata.Indexes {
		if index.Type == model.GlobalSecondaryIndex {
			gsi := types.GlobalSecondaryIndex{
				IndexName: aws.String(index.Name),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(index.PartitionKey.DBName),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll, // Default to ALL
				},
			}

			if index.SortKey != nil {
				gsi.KeySchema = append(gsi.KeySchema, types.KeySchemaElement{
					AttributeName: aws.String(index.SortKey.DBName),
					KeyType:       types.KeyTypeRange,
				})
			}

			// Set projection type based on metadata
			if index.ProjectionType != "" {
				gsi.Projection.ProjectionType = types.ProjectionType(index.ProjectionType)

				// If INCLUDE projection, add non-key attributes
				if index.ProjectionType == "INCLUDE" && len(index.ProjectedFields) > 0 {
					gsi.Projection.NonKeyAttributes = index.ProjectedFields
				}
			}

			gsiList = append(gsiList, gsi)
		} else if index.Type == model.LocalSecondaryIndex {
			lsi := types.LocalSecondaryIndex{
				IndexName: aws.String(index.Name),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String(metadata.PrimaryKey.PartitionKey.DBName),
						KeyType:       types.KeyTypeHash,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll, // Default to ALL
				},
			}

			if index.SortKey != nil {
				lsi.KeySchema = append(lsi.KeySchema, types.KeySchemaElement{
					AttributeName: aws.String(index.SortKey.DBName),
					KeyType:       types.KeyTypeRange,
				})
			}

			// Set projection type based on metadata
			if index.ProjectionType != "" {
				lsi.Projection.ProjectionType = types.ProjectionType(index.ProjectionType)

				// If INCLUDE projection, add non-key attributes
				if index.ProjectionType == "INCLUDE" && len(index.ProjectedFields) > 0 {
					lsi.Projection.NonKeyAttributes = index.ProjectedFields
				}
			}

			lsiList = append(lsiList, lsi)
		}
	}

	return gsiList, lsiList
}

// waitForTableActive waits for a table to become active
func (m *Manager) waitForTableActive(tableName string) error {
	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table waiter: %w", err)
	}

	waiter := dynamodb.NewTableExistsWaiter(client)

	// Wait up to 5 minutes for table to be active
	err = waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)

	if err != nil {
		return fmt.Errorf("failed waiting for table %s to be active: %w", tableName, err)
	}

	return nil
}

// TableExists checks if a table exists
func (m *Manager) TableExists(tableName string) (bool, error) {
	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return false, fmt.Errorf("failed to get client for table exists check: %w", err)
	}

	_, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})

	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if ok := errors.As(err, &notFoundErr); ok {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// DeleteTable deletes a DynamoDB table
func (m *Manager) DeleteTable(tableName string) error {
	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table deletion: %w", err)
	}

	_, err = client.DeleteTable(ctx, &dynamodb.DeleteTableInput{
		TableName: aws.String(tableName),
	})

	if err != nil {
		return fmt.Errorf("failed to delete table %s: %w", tableName, err)
	}

	// Wait for table to be deleted
	waiter := dynamodb.NewTableNotExistsWaiter(client)
	return waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	}, 5*time.Minute)
}

// DescribeTable returns table description
func (m *Manager) DescribeTable(model any) (*types.TableDescription, error) {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return nil, fmt.Errorf("failed to get model metadata: %w", err)
	}

	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return nil, fmt.Errorf("failed to get client for table description: %w", err)
	}

	output, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(metadata.TableName),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to describe table %s: %w", metadata.TableName, err)
	}

	return output.Table, nil
}

// UpdateTable updates table configuration (throughput, indexes, etc.)
func (m *Manager) UpdateTable(model any, opts ...TableOption) error {
	metadata, err := m.registry.GetMetadata(model)
	if err != nil {
		return fmt.Errorf("failed to get model metadata: %w", err)
	}

	// Get current table description
	current, err := m.DescribeTable(model)
	if err != nil {
		return err
	}

	// Build update input
	input := &dynamodb.UpdateTableInput{
		TableName: aws.String(metadata.TableName),
	}

	// Apply options to determine what to update
	createInput := &dynamodb.CreateTableInput{}
	for _, opt := range opts {
		opt(createInput)
	}

	// Update billing mode if changed
	if createInput.BillingMode != "" && createInput.BillingMode != current.BillingModeSummary.BillingMode {
		input.BillingMode = createInput.BillingMode

		// If switching to provisioned, set throughput
		if createInput.BillingMode == types.BillingModeProvisioned && createInput.ProvisionedThroughput != nil {
			input.ProvisionedThroughput = createInput.ProvisionedThroughput
		}
	}

	// Update streams if changed
	if createInput.StreamSpecification != nil {
		input.StreamSpecification = createInput.StreamSpecification
	}

	// Update SSE if changed
	if createInput.SSESpecification != nil {
		input.SSESpecification = createInput.SSESpecification
	}

	// Handle GSI updates
	gsiUpdates, err := m.calculateGSIUpdates(metadata, current)
	if err != nil {
		return fmt.Errorf("failed to calculate GSI updates: %w", err)
	}

	// Apply GSI updates (DynamoDB allows only one GSI operation per UpdateTable call)
	if len(gsiUpdates.ToCreate) > 0 || len(gsiUpdates.ToDelete) > 0 {
		// If there are multiple GSI changes, we'll need to make multiple UpdateTable calls
		// For now, we'll return an error with instructions
		totalChanges := len(gsiUpdates.ToCreate) + len(gsiUpdates.ToDelete)
		if totalChanges > 1 {
			return fmt.Errorf("multiple GSI changes detected (%d creates, %d deletes). DynamoDB allows only one GSI operation per UpdateTable call. Please use AutoMigrate for complex schema changes",
				len(gsiUpdates.ToCreate), len(gsiUpdates.ToDelete))
		}

		// Apply single GSI create
		if len(gsiUpdates.ToCreate) == 1 {
			input.GlobalSecondaryIndexUpdates = []types.GlobalSecondaryIndexUpdate{
				{
					Create: &types.CreateGlobalSecondaryIndexAction{
						IndexName:             gsiUpdates.ToCreate[0].IndexName,
						KeySchema:             gsiUpdates.ToCreate[0].KeySchema,
						Projection:            gsiUpdates.ToCreate[0].Projection,
						ProvisionedThroughput: gsiUpdates.ToCreate[0].ProvisionedThroughput,
					},
				},
			}
		}

		// Apply single GSI delete
		if len(gsiUpdates.ToDelete) == 1 {
			input.GlobalSecondaryIndexUpdates = []types.GlobalSecondaryIndexUpdate{
				{
					Delete: &types.DeleteGlobalSecondaryIndexAction{
						IndexName: aws.String(gsiUpdates.ToDelete[0]),
					},
				},
			}
		}
	}

	ctx := context.Background()
	client, err := m.session.Client()
	if err != nil {
		return fmt.Errorf("failed to get client for table update: %w", err)
	}

	_, err = client.UpdateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to update table %s: %w", metadata.TableName, err)
	}

	// Wait for update to complete
	return m.waitForTableActive(metadata.TableName)
}

// GSIUpdatePlan contains GSIs to create and delete
type GSIUpdatePlan struct {
	ToCreate []types.GlobalSecondaryIndex
	ToDelete []string
}

// calculateGSIUpdates compares current GSIs with desired GSIs and returns update plan
func (m *Manager) calculateGSIUpdates(metadata *model.Metadata, current *types.TableDescription) (*GSIUpdatePlan, error) {
	plan := &GSIUpdatePlan{
		ToCreate: []types.GlobalSecondaryIndex{},
		ToDelete: []string{},
	}

	// Build desired GSIs from metadata
	desiredGSIs, _ := m.buildIndexes(metadata)

	// Create map of current GSIs
	currentGSIMap := make(map[string]*types.GlobalSecondaryIndexDescription)
	if current.GlobalSecondaryIndexes != nil {
		for _, gsi := range current.GlobalSecondaryIndexes {
			if gsi.IndexName != nil {
				currentGSIMap[*gsi.IndexName] = &gsi
			}
		}
	}

	// Create map of desired GSIs
	desiredGSIMap := make(map[string]*types.GlobalSecondaryIndex)
	for i := range desiredGSIs {
		gsi := &desiredGSIs[i]
		if gsi.IndexName != nil {
			desiredGSIMap[*gsi.IndexName] = gsi
		}
	}

	// Find GSIs to create (in desired but not in current)
	for name, desiredGSI := range desiredGSIMap {
		if _, exists := currentGSIMap[name]; !exists {
			// Set default provisioned throughput if billing mode is provisioned
			if current.BillingModeSummary != nil &&
				current.BillingModeSummary.BillingMode == types.BillingModeProvisioned &&
				desiredGSI.ProvisionedThroughput == nil {
				desiredGSI.ProvisionedThroughput = &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				}
			}
			plan.ToCreate = append(plan.ToCreate, *desiredGSI)
		}
	}

	// Find GSIs to delete (in current but not in desired)
	for name := range currentGSIMap {
		if _, exists := desiredGSIMap[name]; !exists {
			plan.ToDelete = append(plan.ToDelete, name)
		}
	}

	// Note: We don't check for GSI modifications because DynamoDB doesn't support
	// modifying GSIs in place. Users would need to delete and recreate.

	return plan, nil
}

// WithGSICreate creates a TableOption for adding a new GSI
func WithGSICreate(indexName string, partitionKey string, sortKey string, projectionType types.ProjectionType) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		// This is a marker option - actual GSI creation is handled in UpdateTable
		// by comparing model metadata with current table state
	}
}

// WithGSIDelete creates a TableOption for deleting a GSI
func WithGSIDelete(indexName string) TableOption {
	return func(input *dynamodb.CreateTableInput) {
		// This is a marker option - actual GSI deletion is handled in UpdateTable
		// by comparing model metadata with current table state
	}
}

// BatchUpdateTable performs multiple table updates that require separate API calls
// This is useful for multiple GSI changes since DynamoDB only allows one GSI operation per UpdateTable call
func (m *Manager) BatchUpdateTable(model any, updates []TableOption) error {
	for i, update := range updates {
		if err := m.UpdateTable(model, update); err != nil {
			return fmt.Errorf("batch update failed at step %d: %w", i+1, err)
		}

		// Wait a bit between updates to avoid throttling
		if i < len(updates)-1 {
			time.Sleep(2 * time.Second)
		}
	}
	return nil
}
