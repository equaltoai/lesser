package integration

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BaseModel represents common fields for single-table design
type BaseModel struct {
	// Primary composite keys
	PK string `dynamorm:"pk"`
	SK string `dynamorm:"sk"`

	// Global Secondary Indexes
	GSI1PK string `dynamorm:"index:gsi1,pk"`
	GSI1SK string `dynamorm:"index:gsi1,sk"`
	GSI2PK string `dynamorm:"index:gsi2,pk"`
	GSI2SK string `dynamorm:"index:gsi2,sk"`

	// Common metadata fields
	Type      string    `dynamorm:"attr:type"`
	AccountID string    `dynamorm:"attr:account_id"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
	Version   int       `dynamorm:"version"`

	// Soft delete support
	Deleted   bool      `json:"deleted,omitempty" dynamorm:"attr:deleted"`
	DeletedAt time.Time `json:"-" dynamorm:"attr:deleted_at"`
}

// EmbeddedCustomer model with embedded BaseModel
type EmbeddedCustomer struct {
	BaseModel // Embedded struct

	// Customer-specific fields
	ID      string `json:"id" dynamorm:"attr:id"`
	Object  string `json:"object" dynamorm:"attr:object"`
	Created int64  `json:"created" dynamorm:"attr:created"`
	Email   string `json:"email" dynamorm:"attr:email"`
	Name    string `json:"name" dynamorm:"attr:name"`
}

func (c *EmbeddedCustomer) TableName() string {
	return "test-embedded-structs"
}

// EmbeddedProduct model also using embedded BaseModel
type EmbeddedProduct struct {
	BaseModel // Embedded struct

	// Product-specific fields
	ID          string  `json:"id" dynamorm:"attr:id"`
	Name        string  `json:"name" dynamorm:"attr:name"`
	Description string  `json:"description" dynamorm:"attr:description"`
	Price       float64 `json:"price" dynamorm:"attr:price"`
	Stock       int     `json:"stock" dynamorm:"attr:stock"`
}

func (p *EmbeddedProduct) TableName() string {
	return "test-embedded-structs"
}

func TestEmbeddedStructSupport(t *testing.T) {
	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	// Create table with automatic cleanup
	testCtx.CreateTableIfNotExists(t, &EmbeddedCustomer{})

	t.Run("CreateWithEmbeddedStruct", func(t *testing.T) {
		customer := &EmbeddedCustomer{
			BaseModel: BaseModel{
				PK:        "CUSTOMER#cus_test123",
				SK:        "METADATA",
				GSI1PK:    "ACCOUNT#acc_123",
				GSI1SK:    "CUSTOMER#cus_test123",
				GSI2PK:    "EMAIL#test@example.com",
				GSI2SK:    "CUSTOMER#cus_test123",
				Type:      "customer",
				AccountID: "acc_123",
			},
			ID:      "cus_test123",
			Object:  "customer",
			Created: time.Now().Unix(),
			Email:   "test@example.com",
			Name:    "Test Customer",
		}

		// Create the customer
		err := testCtx.DB.Model(customer).Create()
		require.NoError(t, err)

		// Verify UpdatedAt was set
		assert.NotZero(t, customer.UpdatedAt)

		// Read it back
		var retrieved EmbeddedCustomer
		err = testCtx.DB.Model(&EmbeddedCustomer{}).Where("PK", "=", "CUSTOMER#cus_test123").Where("SK", "=", "METADATA").First(&retrieved)
		require.NoError(t, err)

		// Verify all fields were saved correctly
		assert.Equal(t, customer.PK, retrieved.PK)
		assert.Equal(t, customer.SK, retrieved.SK)
		assert.Equal(t, customer.GSI1PK, retrieved.GSI1PK)
		assert.Equal(t, customer.GSI1SK, retrieved.GSI1SK)
		assert.Equal(t, customer.Type, retrieved.Type)
		assert.Equal(t, customer.AccountID, retrieved.AccountID)
		assert.Equal(t, customer.ID, retrieved.ID)
		assert.Equal(t, customer.Email, retrieved.Email)
		assert.Equal(t, customer.Name, retrieved.Name)
		assert.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("QueryByIndexWithEmbeddedStruct", func(t *testing.T) {
		// Create multiple customers for the same account
		for i := 1; i <= 3; i++ {
			customer := &EmbeddedCustomer{
				BaseModel: BaseModel{
					PK:        fmt.Sprintf("CUSTOMER#cus_test%d", i),
					SK:        "METADATA",
					GSI1PK:    "ACCOUNT#acc_456",
					GSI1SK:    fmt.Sprintf("CUSTOMER#cus_test%d", i),
					GSI2PK:    fmt.Sprintf("EMAIL#test%d@example.com", i),
					GSI2SK:    fmt.Sprintf("CUSTOMER#cus_test%d", i),
					Type:      "customer",
					AccountID: "acc_456",
				},
				ID:      fmt.Sprintf("cus_test%d", i),
				Object:  "customer",
				Created: time.Now().Unix(),
				Email:   fmt.Sprintf("test%d@example.com", i),
				Name:    fmt.Sprintf("Test Customer %d", i),
			}

			err := testCtx.DB.Model(customer).Create()
			require.NoError(t, err)
		}

		// Query by GSI1
		var customers []EmbeddedCustomer
		err := testCtx.DB.Model(&EmbeddedCustomer{}).
			Index("gsi1").
			Where("GSI1PK", "=", "ACCOUNT#acc_456").
			All(&customers)
		require.NoError(t, err)

		assert.Len(t, customers, 3)
		for _, c := range customers {
			assert.Equal(t, "ACCOUNT#acc_456", c.GSI1PK)
			assert.Equal(t, "acc_456", c.AccountID)
		}
	})

	t.Run("UpdateWithEmbeddedStruct", func(t *testing.T) {
		customer := &EmbeddedCustomer{
			BaseModel: BaseModel{
				PK:        "CUSTOMER#cus_update",
				SK:        "METADATA",
				GSI1PK:    "ACCOUNT#acc_789",
				GSI1SK:    "CUSTOMER#cus_update",
				GSI2PK:    "EMAIL#original@example.com",
				GSI2SK:    "CUSTOMER#cus_update",
				Type:      "customer",
				AccountID: "acc_789",
			},
			ID:    "cus_update",
			Email: "original@example.com",
			Name:  "Original Name",
		}

		// Create
		err := testCtx.DB.Model(customer).Create()
		require.NoError(t, err)

		originalUpdatedAt := customer.UpdatedAt

		// Wait a bit to ensure UpdatedAt changes
		time.Sleep(100 * time.Millisecond)

		// Update email
		customer.Email = "updated@example.com"
		customer.GSI2PK = "EMAIL#updated@example.com" // Update GSI key when email changes
		err = testCtx.DB.Model(customer).Update("Email", "GSI2PK")
		require.NoError(t, err)

		// Read back the updated customer to get the new UpdatedAt
		var updatedCustomer EmbeddedCustomer
		err = testCtx.DB.Model(&EmbeddedCustomer{}).Where("PK", "=", "CUSTOMER#cus_update").Where("SK", "=", "METADATA").First(&updatedCustomer)
		require.NoError(t, err)

		// Verify UpdatedAt changed
		assert.NotEqual(t, originalUpdatedAt, updatedCustomer.UpdatedAt)

		// Read back to verify
		var retrieved EmbeddedCustomer
		err = testCtx.DB.Model(&EmbeddedCustomer{}).Where("PK", "=", "CUSTOMER#cus_update").Where("SK", "=", "METADATA").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, "updated@example.com", retrieved.Email)
		assert.Equal(t, "Original Name", retrieved.Name)
	})

	t.Run("MultipleEmbeddedStructTypes", func(t *testing.T) {
		// Create a product using the same base model
		product := &EmbeddedProduct{
			BaseModel: BaseModel{
				PK:        "PRODUCT#prod_123",
				SK:        "METADATA",
				GSI1PK:    "ACCOUNT#acc_123",
				GSI1SK:    "PRODUCT#prod_123",
				GSI2PK:    "CATEGORY#electronics",
				GSI2SK:    "PRODUCT#prod_123",
				Type:      "product",
				AccountID: "acc_123",
			},
			ID:          "prod_123",
			Name:        "Test Product",
			Description: "A test product",
			Price:       99.99,
			Stock:       100,
		}

		err := testCtx.DB.Model(product).Create()
		require.NoError(t, err)

		// Verify it was created
		var retrieved EmbeddedProduct
		err = testCtx.DB.Model(&EmbeddedProduct{}).Where("PK", "=", "PRODUCT#prod_123").Where("SK", "=", "METADATA").First(&retrieved)
		require.NoError(t, err)

		assert.Equal(t, product.Name, retrieved.Name)
		assert.Equal(t, product.Price, retrieved.Price)
		assert.Equal(t, product.Stock, retrieved.Stock)
		assert.NotZero(t, retrieved.UpdatedAt)
	})

	t.Run("SoftDeleteWithEmbeddedStruct", func(t *testing.T) {
		customer := &EmbeddedCustomer{
			BaseModel: BaseModel{
				PK:        "CUSTOMER#cus_delete",
				SK:        "METADATA",
				GSI1PK:    "ACCOUNT#acc_999",
				GSI1SK:    "CUSTOMER#cus_delete",
				GSI2PK:    "EMAIL#delete@example.com",
				GSI2SK:    "CUSTOMER#cus_delete",
				Type:      "customer",
				AccountID: "acc_999",
			},
			ID:    "cus_delete",
			Email: "delete@example.com",
			Name:  "To Be Deleted",
		}

		// Create
		err := testCtx.DB.Model(customer).Create()
		require.NoError(t, err)

		// Soft delete
		customer.Deleted = true
		customer.DeletedAt = time.Now()
		err = testCtx.DB.Model(customer).Update("Deleted", "DeletedAt")
		require.NoError(t, err)

		// Read back
		var retrieved EmbeddedCustomer
		err = testCtx.DB.Model(&EmbeddedCustomer{}).Where("PK", "=", "CUSTOMER#cus_delete").Where("SK", "=", "METADATA").First(&retrieved)
		require.NoError(t, err)

		assert.True(t, retrieved.Deleted)
		assert.NotZero(t, retrieved.DeletedAt)
	})
}
