package lift

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// DynamoDBConnectionStore implements ConnectionStore using DynamoDB
type DynamoDBConnectionStore struct {
	client    *dynamodb.Client
	tableName string
	ttlHours  int
}

// DynamoDBConnectionStoreConfig configures the DynamoDB connection store
type DynamoDBConnectionStoreConfig struct {
	TableName string
	Region    string
	TTLHours  int // Hours until connection records expire (default: 24)
}

// NewDynamoDBConnectionStore creates a new DynamoDB-backed connection store
func NewDynamoDBConnectionStore(ctx context.Context, config DynamoDBConnectionStoreConfig) (*DynamoDBConnectionStore, error) {
	if config.TableName == "" {
		return nil, fmt.Errorf("table name is required")
	}

	if config.TTLHours <= 0 {
		config.TTLHours = 24 // Default to 24 hours
	}

	// Load AWS configuration
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(config.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create DynamoDB client
	client := dynamodb.NewFromConfig(cfg)

	return &DynamoDBConnectionStore{
		client:    client,
		tableName: config.TableName,
		ttlHours:  config.TTLHours,
	}, nil
}

// DynamoDBConnection represents a connection record in DynamoDB
// Memory optimized: 168 → 160 bytes (8 bytes saved)
type DynamoDBConnection struct {
	Metadata  map[string]any `` // map = 24 bytes (largest first)
	PK        string         `` // Primary key: "CONNECTION#<connectionId>" (strings = 16 bytes each)
	SK        string         `` // Sort key: "CONNECTION"
	GSI1PK    string         `` // GSI1 primary key: "USER#<userId>"
	GSI1SK    string         `` // GSI1 sort key: "CONNECTION#<connectionId>"
	GSI2PK    string         `` // GSI2 primary key: "TENANT#<tenantId>"
	GSI2SK    string         `` // GSI2 sort key: "CONNECTION#<connectionId>"
	ID        string         ``
	UserID    string         ``
	TenantID  string         ``
	CreatedAt string         ``
	TTL       int64          `` // int64 = 8 bytes (last)
}

// Save stores a connection in DynamoDB
func (s *DynamoDBConnectionStore) Save(ctx context.Context, conn *Connection) error {
	if conn.ID == "" {
		return fmt.Errorf("connection ID is required")
	}

	// Create DynamoDB record
	dbConn := DynamoDBConnection{
		PK:        fmt.Sprintf("CONNECTION#%s", conn.ID),
		SK:        "CONNECTION",
		ID:        conn.ID,
		UserID:    conn.UserID,
		TenantID:  conn.TenantID,
		CreatedAt: conn.CreatedAt,
		TTL:       time.Now().Add(time.Duration(s.ttlHours) * time.Hour).Unix(),
		Metadata:  conn.Metadata,
	}

	// Set GSI keys if user/tenant IDs are present
	if conn.UserID != "" {
		dbConn.GSI1PK = fmt.Sprintf("USER#%s", conn.UserID)
		dbConn.GSI1SK = fmt.Sprintf("CONNECTION#%s", conn.ID)
	}
	if conn.TenantID != "" {
		dbConn.GSI2PK = fmt.Sprintf("TENANT#%s", conn.TenantID)
		dbConn.GSI2SK = fmt.Sprintf("CONNECTION#%s", conn.ID)
	}

	// Marshal to DynamoDB attribute values
	item, err := attributevalue.MarshalMap(dbConn)
	if err != nil {
		return fmt.Errorf("failed to marshal connection: %w", err)
	}

	// Put item in DynamoDB
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(s.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("failed to save connection: %w", err)
	}

	// Atomically increment the connection counter
	if err := s.incrementConnectionCounter(ctx); err != nil {
		// Log the error but don't fail the connection save
		// The counter is for monitoring, not critical functionality
		fmt.Printf("Warning: failed to increment connection counter: %v\n", err)
	}

	return nil
}

// Get retrieves a connection by ID
func (s *DynamoDBConnectionStore) Get(ctx context.Context, connectionID string) (*Connection, error) {
	if connectionID == "" {
		return nil, fmt.Errorf("connection ID is required")
	}

	// Get item from DynamoDB
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONNECTION#%s", connectionID)},
			"sk": &types.AttributeValueMemberS{Value: "CONNECTION"},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get connection: %w", err)
	}

	if result.Item == nil {
		return nil, nil // Not found
	}

	// Unmarshal from DynamoDB
	var dbConn DynamoDBConnection
	if err := attributevalue.UnmarshalMap(result.Item, &dbConn); err != nil {
		return nil, fmt.Errorf("failed to unmarshal connection: %w", err)
	}

	// Convert to Connection
	return &Connection{
		ID:        dbConn.ID,
		UserID:    dbConn.UserID,
		TenantID:  dbConn.TenantID,
		CreatedAt: dbConn.CreatedAt,
		Metadata:  dbConn.Metadata,
	}, nil
}

// Delete removes a connection by ID
func (s *DynamoDBConnectionStore) Delete(ctx context.Context, connectionID string) error {
	if connectionID == "" {
		return fmt.Errorf("connection ID is required")
	}

	// Delete item from DynamoDB
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: fmt.Sprintf("CONNECTION#%s", connectionID)},
			"sk": &types.AttributeValueMemberS{Value: "CONNECTION"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete connection: %w", err)
	}

	// Atomically decrement the connection counter
	if err := s.decrementConnectionCounter(ctx); err != nil {
		// The counter is for monitoring, not critical functionality
		// Silently ignore counter errors
		_ = err
	}

	return nil
}

// ListByUser retrieves all connections for a user
func (s *DynamoDBConnectionStore) ListByUser(ctx context.Context, userID string) ([]*Connection, error) {
	if userID == "" {
		return nil, fmt.Errorf("user ID is required")
	}

	return s.queryConnectionsByIndex(ctx, "gsi1", "gsi1pk", fmt.Sprintf("USER#%s", userID), "failed to query connections by user")
}

// ListByTenant retrieves all connections for a tenant
func (s *DynamoDBConnectionStore) ListByTenant(ctx context.Context, tenantID string) ([]*Connection, error) {
	if tenantID == "" {
		return nil, fmt.Errorf("tenant ID is required")
	}

	return s.queryConnectionsByIndex(ctx, "gsi2", "gsi2pk", fmt.Sprintf("TENANT#%s", tenantID), "failed to query connections by tenant")
}

// queryConnectionsByIndex is a helper function to query connections by a GSI
func (s *DynamoDBConnectionStore) queryConnectionsByIndex(ctx context.Context, indexName, keyAttribute, keyValue, errorMessage string) ([]*Connection, error) {
	result, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(s.tableName),
		IndexName:              aws.String(indexName),
		KeyConditionExpression: aws.String(fmt.Sprintf("%s = :pk", keyAttribute)),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: keyValue},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("%s: %w", errorMessage, err)
	}

	// Convert results
	connections := make([]*Connection, 0, len(result.Items))
	for _, item := range result.Items {
		var dbConn DynamoDBConnection
		if err := attributevalue.UnmarshalMap(item, &dbConn); err != nil {
			continue // Skip invalid items
		}

		connections = append(connections, &Connection{
			ID:        dbConn.ID,
			UserID:    dbConn.UserID,
			TenantID:  dbConn.TenantID,
			CreatedAt: dbConn.CreatedAt,
			Metadata:  dbConn.Metadata,
		})
	}

	return connections, nil
}

// CountActive returns the number of active connections using efficient counter pattern
func (s *DynamoDBConnectionStore) CountActive(ctx context.Context) (int64, error) {
	// Use DynamoDB counter item for efficient connection counting
	counterKey := "CONNECTION_COUNTER"

	// Get the counter item
	result, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: counterKey},
			"sk": &types.AttributeValueMemberS{Value: "COUNTER"},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get connection counter: %w", err)
	}

	// If counter doesn't exist, return 0
	if result.Item == nil {
		return 0, nil
	}

	// Extract the count value
	if countAttr, exists := result.Item["count"]; exists {
		if countNum, ok := countAttr.(*types.AttributeValueMemberN); ok {
			count := int64(0)
			if _, err := fmt.Sscanf(countNum.Value, "%d", &count); err != nil {
				return 0, fmt.Errorf("failed to parse connection count: %w", err)
			}

			// Ensure count is never negative
			if count < 0 {
				count = 0
			}

			return count, nil
		}
	}

	return 0, fmt.Errorf("invalid counter format in DynamoDB")
}

// CreateTable creates the DynamoDB table with proper indexes
func (s *DynamoDBConnectionStore) CreateTable(ctx context.Context) error {
	_, err := s.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(s.tableName),
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("pk"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("sk"),
				KeyType:       types.KeyTypeRange,
			},
		},
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("pk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("sk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("gsi1pk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("gsi1sk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("gsi2pk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("gsi2sk"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{
			{
				IndexName: aws.String("gsi1"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("gsi1pk"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("gsi1sk"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
			{
				IndexName: aws.String("gsi2"),
				KeySchema: []types.KeySchemaElement{
					{
						AttributeName: aws.String("gsi2pk"),
						KeyType:       types.KeyTypeHash,
					},
					{
						AttributeName: aws.String("gsi2sk"),
						KeyType:       types.KeyTypeRange,
					},
				},
				Projection: &types.Projection{
					ProjectionType: types.ProjectionTypeAll,
				},
				ProvisionedThroughput: &types.ProvisionedThroughput{
					ReadCapacityUnits:  aws.Int64(5),
					WriteCapacityUnits: aws.Int64(5),
				},
			},
		},
		BillingMode: types.BillingModePayPerRequest,
		StreamSpecification: &types.StreamSpecification{
			StreamEnabled:  aws.Bool(true),
			StreamViewType: types.StreamViewTypeNewAndOldImages,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	// Wait for table to be active
	waiter := dynamodb.NewTableExistsWaiter(s.client)
	if waitErr := waiter.Wait(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(s.tableName),
	}, 5*time.Minute); waitErr != nil {
		return fmt.Errorf("failed waiting for table to be active: %w", waitErr)
	}

	// Enable TTL
	_, err = s.client.UpdateTimeToLive(ctx, &dynamodb.UpdateTimeToLiveInput{
		TableName: aws.String(s.tableName),
		TimeToLiveSpecification: &types.TimeToLiveSpecification{
			AttributeName: aws.String("ttl"),
			Enabled:       aws.Bool(true),
		},
	})
	if err != nil {
		// TTL update might fail if already enabled, silently ignore
		_ = err
	}

	return nil
}

// incrementConnectionCounter atomically increments the connection counter
func (s *DynamoDBConnectionStore) incrementConnectionCounter(ctx context.Context) error {
	return s.updateConnectionCounter(ctx, 1)
}

// decrementConnectionCounter atomically decrements the connection counter
func (s *DynamoDBConnectionStore) decrementConnectionCounter(ctx context.Context) error {
	return s.updateConnectionCounter(ctx, -1)
}

// updateConnectionCounter atomically updates the connection counter by the specified delta
func (s *DynamoDBConnectionStore) updateConnectionCounter(ctx context.Context, delta int64) error {
	counterKey := "CONNECTION_COUNTER"

	// Use atomic ADD operation to update the counter
	_, err := s.client.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(s.tableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: counterKey},
			"sk": &types.AttributeValueMemberS{Value: "COUNTER"},
		},
		UpdateExpression: aws.String("ADD #count :delta"),
		ExpressionAttributeNames: map[string]string{
			"#count": "count",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":delta": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", delta)},
		},
		ReturnValues: types.ReturnValueNone,
	})

	if err != nil {
		return fmt.Errorf("failed to update connection counter: %w", err)
	}

	return nil
}
