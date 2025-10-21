package schema

import (
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
	"github.com/pay-theory/dynamorm/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test models
type User struct {
	ID        string `dynamorm:"pk"`
	Email     string `dynamorm:"sk"`
	Name      string
	Age       int
	Balance   float64
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
	Version   int       `dynamorm:"version"`
}

type Product struct {
	ID          string `dynamorm:"pk"`
	CategoryID  string `dynamorm:"sk"`
	Name        string `dynamorm:"index:name-index,pk"`
	Price       float64
	StockLevel  int
	UpdatedTime time.Time `dynamorm:"lsi:updated-lsi,sk"`
}

type Order struct {
	OrderID    string    `dynamorm:"pk"`
	CustomerID string    `dynamorm:"index:customer-index,pk"`
	OrderDate  time.Time `dynamorm:"index:customer-index,sk"`
	Total      float64
	Status     string    `dynamorm:"index:status-index,pk"`
	UpdatedAt  time.Time `dynamorm:"index:status-index,sk"`
}

func TestCreateTable(t *testing.T) {
	// Skip if no test endpoint is set
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Create registry and register model
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)

	// Create schema manager
	manager := NewManager(sess, registry)

	t.Run("CreateSimpleTable", func(t *testing.T) {
		// Delete table if exists
		_ = manager.DeleteTable("Users")

		// Create table
		err := manager.CreateTable(&User{})
		assert.NoError(t, err)

		// Verify table exists
		exists, err := manager.TableExists("Users")
		assert.NoError(t, err)
		assert.True(t, exists)

		// Describe table
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, "Users", *desc.TableName)
		assert.Equal(t, types.TableStatusActive, desc.TableStatus)

		// Cleanup
		_ = manager.DeleteTable("Users")
	})

	t.Run("CreateTableWithGSI", func(t *testing.T) {
		// Register model with GSI
		err := registry.Register(&Order{})
		require.NoError(t, err)

		// Delete table if exists
		_ = manager.DeleteTable("Orders")

		// Create table
		err = manager.CreateTable(&Order{})
		assert.NoError(t, err)

		// Verify GSIs
		desc, err := manager.DescribeTable(&Order{})
		assert.NoError(t, err)
		assert.Len(t, desc.GlobalSecondaryIndexes, 2)

		// Check customer index
		var hasCustomerIndex, hasStatusIndex bool
		for _, gsi := range desc.GlobalSecondaryIndexes {
			if *gsi.IndexName == "customer-index" {
				hasCustomerIndex = true
				assert.Equal(t, "CustomerID", *gsi.KeySchema[0].AttributeName)
				assert.Equal(t, types.KeyTypeHash, gsi.KeySchema[0].KeyType)
				assert.Equal(t, "OrderDate", *gsi.KeySchema[1].AttributeName)
				assert.Equal(t, types.KeyTypeRange, gsi.KeySchema[1].KeyType)
			}
			if *gsi.IndexName == "status-index" {
				hasStatusIndex = true
			}
		}
		assert.True(t, hasCustomerIndex)
		assert.True(t, hasStatusIndex)

		// Cleanup
		_ = manager.DeleteTable("Orders")
	})

	t.Run("CreateTableWithLSI", func(t *testing.T) {
		// Register model with LSI
		err := registry.Register(&Product{})
		require.NoError(t, err)

		// Delete table if exists
		_ = manager.DeleteTable("Products")

		// Create table
		err = manager.CreateTable(&Product{})
		assert.NoError(t, err)

		// Verify LSI
		desc, err := manager.DescribeTable(&Product{})
		assert.NoError(t, err)
		assert.Len(t, desc.LocalSecondaryIndexes, 1)
		assert.Equal(t, "updated-lsi", *desc.LocalSecondaryIndexes[0].IndexName)

		// Cleanup
		_ = manager.DeleteTable("Products")
	})

	t.Run("CreateTableWithOptions", func(t *testing.T) {
		// Delete table if exists
		_ = manager.DeleteTable("Users")

		// Create table with provisioned throughput
		err := manager.CreateTable(&User{},
			WithBillingMode(types.BillingModeProvisioned),
			WithThroughput(5, 5),
		)
		assert.NoError(t, err)

		// Verify billing mode
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		if desc.BillingModeSummary != nil {
			assert.Equal(t, types.BillingModeProvisioned, desc.BillingModeSummary.BillingMode)
		}
		if desc.ProvisionedThroughput != nil {
			assert.Equal(t, int64(5), *desc.ProvisionedThroughput.ReadCapacityUnits)
			assert.Equal(t, int64(5), *desc.ProvisionedThroughput.WriteCapacityUnits)
		}

		// Cleanup
		_ = manager.DeleteTable("Users")
	})
}

func TestTableExists(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Create registry and manager
	registry := model.NewRegistry()
	manager := NewManager(sess, registry)

	t.Run("NonExistentTable", func(t *testing.T) {
		exists, err := manager.TableExists("NonExistentTable")
		assert.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("ExistingTable", func(t *testing.T) {
		// Create a table first
		err := registry.Register(&User{})
		require.NoError(t, err)

		_ = manager.DeleteTable("Users")
		err = manager.CreateTable(&User{})
		require.NoError(t, err)

		// Check existence
		exists, err := manager.TableExists("Users")
		assert.NoError(t, err)
		assert.True(t, exists)

		// Cleanup
		_ = manager.DeleteTable("Users")
	})
}

func TestUpdateTable(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}
	tests.RequireDynamoDBLocal(t)

	// Create test session
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Create registry and manager
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)
	manager := NewManager(sess, registry)

	t.Run("UpdateBillingMode", func(t *testing.T) {
		// Create table with on-demand billing
		_ = manager.DeleteTable("Users")
		err := manager.CreateTable(&User{})
		require.NoError(t, err)

		// Update to provisioned billing
		err = manager.UpdateTable(&User{},
			WithBillingMode(types.BillingModeProvisioned),
			WithThroughput(10, 10),
		)
		assert.NoError(t, err)

		// Verify update
		desc, err := manager.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, types.BillingModeProvisioned, desc.BillingModeSummary.BillingMode)

		// Cleanup
		_ = manager.DeleteTable("Users")
	})
}

func TestBuildAttributeDefinitions(t *testing.T) {
	registry := model.NewRegistry()
	manager := &Manager{registry: registry}

	t.Run("SimpleTable", func(t *testing.T) {
		err := registry.Register(&User{})
		require.NoError(t, err)

		metadata, err := registry.GetMetadata(&User{})
		require.NoError(t, err)

		attrs := manager.buildAttributeDefinitions(metadata)
		assert.Len(t, attrs, 2) // ID and Email (PK and SK)

		// Check that we have the right attributes
		attrMap := make(map[string]types.ScalarAttributeType)
		for _, attr := range attrs {
			attrMap[*attr.AttributeName] = attr.AttributeType
		}

		assert.Equal(t, types.ScalarAttributeTypeS, attrMap["ID"])
		assert.Equal(t, types.ScalarAttributeTypeS, attrMap["Email"])
	})

	t.Run("TableWithIndexes", func(t *testing.T) {
		err := registry.Register(&Order{})
		require.NoError(t, err)

		metadata, err := registry.GetMetadata(&Order{})
		require.NoError(t, err)

		attrs := manager.buildAttributeDefinitions(metadata)

		// Should have OrderID, CustomerID, OrderDate, Status, UpdatedAt
		attrMap := make(map[string]types.ScalarAttributeType)
		for _, attr := range attrs {
			attrMap[*attr.AttributeName] = attr.AttributeType
		}

		assert.Contains(t, attrMap, "OrderID")
		assert.Contains(t, attrMap, "CustomerID")
		assert.Contains(t, attrMap, "OrderDate")
		assert.Contains(t, attrMap, "Status")
		assert.Contains(t, attrMap, "UpdatedAt")
	})
}

func TestGetAttributeType(t *testing.T) {
	manager := &Manager{}

	tests := []struct {
		name     string
		kind     reflect.Kind
		expected types.ScalarAttributeType
	}{
		{"String", reflect.String, types.ScalarAttributeTypeS},
		{"Int", reflect.Int, types.ScalarAttributeTypeN},
		{"Int64", reflect.Int64, types.ScalarAttributeTypeN},
		{"Uint", reflect.Uint, types.ScalarAttributeTypeN},
		{"Slice", reflect.Slice, types.ScalarAttributeTypeB},
		{"Other", reflect.Bool, types.ScalarAttributeTypeS},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := manager.getAttributeType(tt.kind)
			assert.Equal(t, tt.expected, result)
		})
	}
}
