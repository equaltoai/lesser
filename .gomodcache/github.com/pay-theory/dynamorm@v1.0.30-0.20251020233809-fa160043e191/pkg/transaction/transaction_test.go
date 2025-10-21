package transaction

import (
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/pay-theory/dynamorm/pkg/model"
	"github.com/pay-theory/dynamorm/pkg/session"
	pkgTypes "github.com/pay-theory/dynamorm/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test models
type User struct {
	ID        string `dynamorm:"pk"`
	Email     string
	Name      string
	Balance   float64
	Version   int       `dynamorm:"version"`
	CreatedAt time.Time `dynamorm:"created_at"`
	UpdatedAt time.Time `dynamorm:"updated_at"`
}

type Account struct {
	AccountID   string `dynamorm:"pk"`
	UserID      string `dynamorm:"sk"`
	AccountType string
	Balance     float64
	Version     int       `dynamorm:"version"`
	UpdatedAt   time.Time `dynamorm:"updated_at"`
}

type Order struct {
	OrderID    string `dynamorm:"pk"`
	CustomerID string
	Total      float64
	Status     string
	CreatedAt  time.Time `dynamorm:"created_at"`
}

func setupTest(t *testing.T) (*Transaction, *model.Registry) {
	// Skip if no test endpoint is set
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Create test session
	sess, err := session.NewSession(&session.Config{
		Region:   "us-east-1",
		Endpoint: "http://localhost:8000",
	})
	require.NoError(t, err)

	// Create registry and register models
	registry := model.NewRegistry()
	err = registry.Register(&User{})
	require.NoError(t, err)
	err = registry.Register(&Account{})
	require.NoError(t, err)
	err = registry.Register(&Order{})
	require.NoError(t, err)

	// Create converter
	converter := pkgTypes.NewConverter()

	// Create transaction
	tx := NewTransaction(sess, registry, converter)
	return tx, registry
}

func TestTransactionCreate(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddCreateToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Email:   "test@example.com",
			Name:    "Test User",
			Balance: 100.0,
		}

		err := tx.Create(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Put)
		assert.Equal(t, "Users", *writeItem.Put.TableName)
		assert.NotNil(t, writeItem.Put.ConditionExpression)
		assert.Contains(t, *writeItem.Put.ConditionExpression, "attribute_not_exists")
	})

	t.Run("MultipleCreates", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		user1 := &User{ID: "user-1", Name: "User 1"}
		user2 := &User{ID: "user-2", Name: "User 2"}
		order := &Order{OrderID: "order-1", CustomerID: "user-1", Total: 50.0}

		err := tx.Create(user1)
		assert.NoError(t, err)
		err = tx.Create(user2)
		assert.NoError(t, err)
		err = tx.Create(order)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 3)
	})
}

func TestTransactionUpdate(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddUpdateToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Email:   "updated@example.com",
			Name:    "Updated User",
			Balance: 150.0,
			Version: 1,
		}

		err := tx.Update(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Update)
		assert.Equal(t, "Users", *writeItem.Update.TableName)
		assert.NotNil(t, writeItem.Update.UpdateExpression)
		assert.Contains(t, *writeItem.Update.UpdateExpression, "SET")

		// Should have version condition
		assert.NotNil(t, writeItem.Update.ConditionExpression)
		assert.Contains(t, *writeItem.Update.ConditionExpression, "#ver = :currentVer")
	})

	t.Run("UpdateWithoutVersion", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		order := &Order{
			OrderID:    "order-1",
			CustomerID: "user-1",
			Status:     "SHIPPED",
		}

		err := tx.Update(order)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Should not have version condition
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Update)
		assert.Empty(t, writeItem.Update.ConditionExpression)
	})
}

func TestTransactionDelete(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddDeleteToTransaction", func(t *testing.T) {
		user := &User{
			ID:      "user-1",
			Version: 2,
		}

		err := tx.Delete(user)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Check the write item
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Delete)
		assert.Equal(t, "Users", *writeItem.Delete.TableName)

		// Should have version condition
		assert.NotNil(t, writeItem.Delete.ConditionExpression)
		assert.Contains(t, *writeItem.Delete.ConditionExpression, "#ver = :ver")
	})

	t.Run("DeleteWithoutVersion", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			writes:    make([]types.TransactWriteItem, 0),
		}

		order := &Order{
			OrderID: "order-1",
		}

		err := tx.Delete(order)
		assert.NoError(t, err)
		assert.Len(t, tx.writes, 1)

		// Should not have condition
		writeItem := tx.writes[0]
		assert.NotNil(t, writeItem.Delete)
		assert.Nil(t, writeItem.Delete.ConditionExpression)
	})
}

func TestTransactionGet(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("AddGetToTransaction", func(t *testing.T) {
		user := &User{ID: "user-1"}
		var result User

		err := tx.Get(user, &result)
		assert.NoError(t, err)
		assert.Len(t, tx.reads, 1)

		// Check the read item
		readItem := tx.reads[0]
		assert.NotNil(t, readItem.Get)
		assert.Equal(t, "Users", *readItem.Get.TableName)
		assert.Contains(t, readItem.Get.Key, "ID")
	})

	t.Run("MultipleGets", func(t *testing.T) {
		tx = &Transaction{
			session:   tx.session,
			registry:  tx.registry,
			converter: tx.converter,
			reads:     make([]types.TransactGetItem, 0),
		}

		user := &User{ID: "user-1"}
		order := &Order{OrderID: "order-1"}
		var userResult User
		var orderResult Order

		err := tx.Get(user, &userResult)
		assert.NoError(t, err)
		err = tx.Get(order, &orderResult)
		assert.NoError(t, err)

		assert.Len(t, tx.reads, 2)
	})
}

func TestTransactionMixed(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("MixedOperations", func(t *testing.T) {
		// Add various operations
		createUser := &User{ID: "user-new", Name: "New User", Balance: 100}
		updateUser := &User{ID: "user-1", Balance: 200, Version: 1}
		deleteOrder := &Order{OrderID: "order-old"}
		getUser := &User{ID: "user-2"}
		var getUserResult User

		err := tx.Create(createUser)
		assert.NoError(t, err)
		err = tx.Update(updateUser)
		assert.NoError(t, err)
		err = tx.Delete(deleteOrder)
		assert.NoError(t, err)
		err = tx.Get(getUser, &getUserResult)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 3)
		assert.Len(t, tx.reads, 1)
	})
}

func TestTransactionRollback(t *testing.T) {
	tx, _ := setupTest(t)

	t.Run("RollbackClearsOperations", func(t *testing.T) {
		// Add some operations
		user := &User{ID: "user-1", Name: "Test"}
		err := tx.Create(user)
		assert.NoError(t, err)

		var result User
		err = tx.Get(user, &result)
		assert.NoError(t, err)

		assert.Len(t, tx.writes, 1)
		assert.Len(t, tx.reads, 1)

		// Rollback
		err = tx.Rollback()
		assert.NoError(t, err)

		// Operations should be cleared
		assert.Nil(t, tx.writes)
		assert.Nil(t, tx.reads)
		assert.Nil(t, tx.results)
	})
}

func TestExtractPrimaryKey(t *testing.T) {
	tx, registry := setupTest(t)

	t.Run("SimpleKey", func(t *testing.T) {
		user := &User{ID: "user-1"}
		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		key, err := tx.extractPrimaryKey(user, metadata)
		assert.NoError(t, err)
		assert.Len(t, key, 1)
		assert.Contains(t, key, "ID")
	})

	t.Run("CompositeKey", func(t *testing.T) {
		account := &Account{
			AccountID: "acc-1",
			UserID:    "user-1",
		}
		metadata, err := registry.GetMetadata(account)
		require.NoError(t, err)

		key, err := tx.extractPrimaryKey(account, metadata)
		assert.NoError(t, err)
		assert.Len(t, key, 2)
		assert.Contains(t, key, "AccountID")
		assert.Contains(t, key, "UserID")
	})

	t.Run("MissingPartitionKey", func(t *testing.T) {
		user := &User{} // ID not set
		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		_, err = tx.extractPrimaryKey(user, metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "partition key")
	})

	t.Run("MissingSortKey", func(t *testing.T) {
		account := &Account{
			AccountID: "acc-1",
			// UserID not set
		}
		metadata, err := registry.GetMetadata(account)
		require.NoError(t, err)

		_, err = tx.extractPrimaryKey(account, metadata)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "sort key")
	})
}

func TestMarshalItem(t *testing.T) {
	tx, registry := setupTest(t)

	t.Run("FullItem", func(t *testing.T) {
		user := &User{
			ID:        "user-1",
			Email:     "test@example.com",
			Name:      "Test User",
			Balance:   100.50,
			Version:   1,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		item, err := tx.marshalItem(user, metadata)
		assert.NoError(t, err)
		assert.Contains(t, item, "ID")
		assert.Contains(t, item, "Email")
		assert.Contains(t, item, "Name")
		assert.Contains(t, item, "Balance")
		assert.Contains(t, item, "Version")
		assert.Contains(t, item, "CreatedAt")
		assert.Contains(t, item, "UpdatedAt")
	})

	t.Run("OmitEmpty", func(t *testing.T) {
		user := &User{
			ID:   "user-1",
			Name: "Test User",
			// Email and Balance are zero values
		}

		metadata, err := registry.GetMetadata(user)
		require.NoError(t, err)

		// Simulate omitempty on Email field
		if emailField, exists := metadata.Fields["Email"]; exists {
			emailField.OmitEmpty = true
		}

		item, err := tx.marshalItem(user, metadata)
		assert.NoError(t, err)
		assert.Contains(t, item, "ID")
		assert.Contains(t, item, "Name")
		// Balance should still be included (0 is valid)
		assert.Contains(t, item, "Balance")
	})
}
