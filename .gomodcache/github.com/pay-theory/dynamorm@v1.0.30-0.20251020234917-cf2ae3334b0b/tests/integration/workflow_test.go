package integration

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/transaction"
	"github.com/pay-theory/dynamorm/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// User model for testing
type User struct {
	ID        string `dynamorm:"pk"`
	Name      string
	Email     string `dynamorm:"index:email-index,pk"`
	Balance   float64
	Status    string
	Version   int       `dynamorm:"version"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

// Product model for testing with composite key
type Product struct {
	ProductID  string `dynamorm:"pk"`
	CategoryID string `dynamorm:"sk"`
	Name       string
	Price      float64
	Stock      int
	LastSold   time.Time `dynamorm:"lsi:lsi-last-sold,sk"`
}

func TestCompleteWorkflow(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	t.Run("CreateTables", func(t *testing.T) {
		// Create user table with custom options
		testCtx.CreateTableIfNotExists(t, &User{})

		// Create product table
		testCtx.CreateTableIfNotExists(t, &Product{})

		// Verify tables exist
		desc, err := testCtx.DB.DescribeTable(&User{})
		assert.NoError(t, err)
		assert.Equal(t, types.TableStatusActive, desc.(*types.TableDescription).TableStatus)

		desc, err = testCtx.DB.DescribeTable(&Product{})
		assert.NoError(t, err)
		assert.Equal(t, types.TableStatusActive, desc.(*types.TableDescription).TableStatus)
	})

	t.Run("BasicCRUDOperations", func(t *testing.T) {
		// Create a user
		user := &User{
			ID:      "user-1",
			Name:    "Alice",
			Email:   "alice@example.com",
			Balance: 100.0,
			Status:  "active",
		}

		err := testCtx.DB.Model(user).Create()
		require.NoError(t, err)

		// Read the user
		var fetchedUser User
		err = testCtx.DB.Model(&User{ID: "user-1"}).First(&fetchedUser)
		require.NoError(t, err)
		assert.Equal(t, "Alice", fetchedUser.Name)
		assert.Equal(t, float64(100), fetchedUser.Balance)

		// Update the user
		fetchedUser.Balance = 125.0
		err = testCtx.DB.Model(&fetchedUser).Update()
		require.NoError(t, err)

		// Verify update
		var updatedUser User
		err = testCtx.DB.Model(&User{ID: "user-1"}).First(&updatedUser)
		require.NoError(t, err)
		assert.Equal(t, float64(125), updatedUser.Balance)
		assert.Equal(t, 1, updatedUser.Version) // Version should be incremented

		// Create products
		products := []Product{
			{ProductID: "prod-1", CategoryID: "electronics", Name: "Laptop", Price: 999.99, Stock: 10},
			{ProductID: "prod-2", CategoryID: "electronics", Name: "Mouse", Price: 29.99, Stock: 100},
			{ProductID: "prod-3", CategoryID: "books", Name: "Go Programming", Price: 39.99, Stock: 50},
		}

		for _, p := range products {
			err = testCtx.DB.Model(&p).Create()
			require.NoError(t, err)
		}

		// Query products by category
		var electronics []Product
		err = testCtx.DB.Model(&Product{CategoryID: "electronics"}).
			Where("CategoryID", "=", "electronics").
			All(&electronics)
		require.NoError(t, err)
		assert.Len(t, electronics, 2)
	})

	t.Run("TransactionSupport", func(t *testing.T) {
		// Create two users for fund transfer
		user1 := &User{ID: "tx-user-1", Name: "Bob", Email: "bob@example.com", Balance: 200.0}
		user2 := &User{ID: "tx-user-2", Name: "Charlie", Email: "charlie@example.com", Balance: 50.0}

		err := testCtx.DB.Model(user1).Create()
		require.NoError(t, err)
		err = testCtx.DB.Model(user2).Create()
		require.NoError(t, err)

		// Perform atomic fund transfer
		transferAmount := 25.0
		err = testCtx.DB.TransactionFunc(func(tx any) error {
			txTyped := tx.(*transaction.Transaction)
			// Fetch current balances
			var u1, u2 User
			err := testCtx.DB.Model(&User{ID: "tx-user-1"}).First(&u1)
			if err != nil {
				return err
			}
			err = testCtx.DB.Model(&User{ID: "tx-user-2"}).First(&u2)
			if err != nil {
				return err
			}

			// Update balances
			u1.Balance -= transferAmount
			u2.Balance += transferAmount

			// Add updates to transaction
			if err := txTyped.Update(&u1); err != nil {
				return err
			}
			return txTyped.Update(&u2)
		})
		require.NoError(t, err)

		// Verify balances after transaction
		var afterUser1, afterUser2 User
		err = testCtx.DB.Model(&User{ID: "tx-user-1"}).First(&afterUser1)
		require.NoError(t, err)
		err = testCtx.DB.Model(&User{ID: "tx-user-2"}).First(&afterUser2)
		require.NoError(t, err)

		assert.Equal(t, 175.0, afterUser1.Balance)
		assert.Equal(t, 75.0, afterUser2.Balance)
	})

	t.Run("TransactionWithNewItems", func(t *testing.T) {
		// Get the product before the transaction
		var product Product
		err := testCtx.DB.Model(&Product{ProductID: "prod-1", CategoryID: "electronics"}).First(&product)
		require.NoError(t, err)

		// Create order and update inventory atomically
		err = testCtx.DB.TransactionFunc(func(tx any) error {
			txTyped := tx.(*transaction.Transaction)
			// Create a new order
			order := &User{
				ID:      "order-1",
				Name:    "Order for prod-1",
				Email:   "order@example.com",
				Balance: 999.99,
				Status:  "pending",
			}
			if err := txTyped.Create(order); err != nil {
				return err
			}

			// Update product stock
			product.Stock -= 1
			product.LastSold = time.Now()

			return txTyped.Update(&product)
		})
		require.NoError(t, err)

		// Verify order was created
		var order User
		err = testCtx.DB.Model(&User{ID: "order-1"}).First(&order)
		require.NoError(t, err)
		assert.Equal(t, "pending", order.Status)

		// Verify stock was updated
		var updatedProduct Product
		err = testCtx.DB.Model(&Product{ProductID: "prod-1", CategoryID: "electronics"}).First(&updatedProduct)
		require.NoError(t, err)
		assert.Equal(t, 9, updatedProduct.Stock)
	})

	t.Run("ConditionalTransactionFailure", func(t *testing.T) {
		// Try to create a user that already exists
		err := testCtx.DB.TransactionFunc(func(tx any) error {
			txTyped := tx.(*transaction.Transaction)
			duplicate := &User{
				ID:   "user-1",
				Name: "Duplicate User",
			}
			return txTyped.Create(duplicate)
		})
		// Should fail due to conditional check
		assert.Error(t, err)
	})

	t.Run("QueryWithIndex", func(t *testing.T) {
		// Query by email using GSI
		var userByEmail User
		err := testCtx.DB.Model(&User{}).
			Index("email-index").
			Where("Email", "=", "alice@example.com").
			First(&userByEmail)
		require.NoError(t, err)
		assert.Equal(t, "Alice", userByEmail.Name)
	})

	t.Run("AutoMigrate", func(t *testing.T) {
		// Test AutoMigrate with existing tables
		err := testCtx.DB.AutoMigrate(&User{}, &Product{})
		assert.NoError(t, err) // Should not error on existing tables
	})

	t.Run("TableUpdateOptions", func(t *testing.T) {
		// This would normally update table settings, but DynamoDB Local
		// may have limitations on certain updates
		// Commenting out for local testing but this shows the API

		// err = testCtx.DB.UpdateTable(&User{},
		//     schema.WithStreamSpecification(types.StreamSpecification{
		//         StreamEnabled:  aws.Bool(true),
		//         StreamViewType: types.StreamViewTypeNewAndOldImages,
		//     }),
		// )
		// assert.NoError(t, err)
	})

	// Cleanup
	t.Cleanup(func() {
		_ = testCtx.DB.DeleteTable(&User{})
		_ = testCtx.DB.DeleteTable(&Product{})
	})
}

func TestEnsureTable(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	// EnsureTable should create if not exists
	err := testCtx.DB.EnsureTable(&User{})
	require.NoError(t, err)

	// Second call should not error
	err = testCtx.DB.EnsureTable(&User{})
	require.NoError(t, err)

	// Verify table exists
	desc, err := testCtx.DB.DescribeTable(&User{})
	assert.NoError(t, err)
	assert.NotNil(t, desc)

	// Cleanup
	_ = testCtx.DB.DeleteTable(&User{})
}

func TestBatchOperationsWithTransaction(t *testing.T) {
	tests.RequireDynamoDBLocal(t)

	// Initialize test context with automatic cleanup
	testCtx := InitTestDB(t)

	// Ensure table exists
	_ = testCtx.DB.DeleteTable(&User{})
	err := testCtx.DB.CreateTable(&User{})
	require.NoError(t, err)

	// Create multiple users in a transaction
	err = testCtx.DB.TransactionFunc(func(tx any) error {
		txTyped := tx.(*transaction.Transaction)
		users := []User{
			{ID: "batch-1", Name: "User 1", Email: "user1@example.com", Balance: 100},
			{ID: "batch-2", Name: "User 2", Email: "user2@example.com", Balance: 200},
			{ID: "batch-3", Name: "User 3", Email: "user3@example.com", Balance: 300},
			{ID: "batch-4", Name: "User 4", Email: "user4@example.com", Balance: 400},
			{ID: "batch-5", Name: "User 5", Email: "user5@example.com", Balance: 500},
		}

		for _, u := range users {
			if err := txTyped.Create(&u); err != nil {
				return err
			}
		}
		return nil
	})
	require.NoError(t, err)

	// Verify all users were created
	var allUsers []User
	err = testCtx.DB.Model(&User{}).Scan(&allUsers)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allUsers), 5)

	// Cleanup
	_ = testCtx.DB.DeleteTable(&User{})
}
