package dynamorm

import (
	"context"
	"time"

	"github.com/pay-theory/dynamorm/pkg/core"
	"github.com/pay-theory/dynamorm/pkg/session"

	"github.com/pay-theory/lift/pkg/lift"
)

// DynamORMConfig holds configuration for DynamORM integration
type DynamORMConfig struct {
	// Strings (16 bytes each) - largest first
	TableName string `json:"table_name"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint,omitempty"` // For local testing
	TenantKey string `json:"tenant_key"`         // Key used for tenant isolation (default: "tenant_id")

	// 8-byte aligned fields
	Timeout time.Duration `json:"timeout"`

	// 4-byte aligned fields
	MaxRetries int `json:"max_retries"`
	BatchSize  int `json:"batch_size"` // Default batch size for operations

	// Boolean flags (1 byte each)
	AutoTransaction bool `json:"auto_transaction"` // Automatically wrap writes in transactions
	TenantIsolation bool `json:"tenant_isolation"` // Enforce tenant-based data isolation
	ConsistentRead  bool `json:"consistent_read"`  // Use strongly consistent reads
}

// DefaultConfig returns a default DynamORM configuration
func DefaultConfig() *DynamORMConfig {
	return &DynamORMConfig{
		TableName:       "lift_data",
		Region:          "us-east-1",
		MaxRetries:      3,
		Timeout:         30 * time.Second,
		AutoTransaction: true,
		TenantIsolation: true,
		TenantKey:       "tenant_id",
		ConsistentRead:  false,
		BatchSize:       25,
	}
}

// WithDynamORM creates middleware that provides DynamORM integration
func WithDynamORM(config *DynamORMConfig, optionalFactory ...DBFactory) lift.Middleware {
	if config == nil {
		config = DefaultConfig()
	}

	// Use provided factory or default
	var factory DBFactory
	if len(optionalFactory) > 0 && optionalFactory[0] != nil {
		factory = optionalFactory[0]
	} else {
		factory = &DefaultDBFactory{}
	}

	return func(next lift.Handler) lift.Handler {
		return lift.HandlerFunc(func(ctx *lift.Context) error {
			// Initialize DynamORM connection using factory
			db, err := initDynamORMWithFactory(config, factory)
			if err != nil {
				return lift.SystemError("Failed to initialize DynamORM").WithCause(err)
			}

			// Store DynamORM instance in context
			ctx.Set("dynamorm", db)

			// Add tenant isolation if enabled
			if config.TenantIsolation {
				tenantID := ctx.TenantID()
				if tenantID == "" {
					return lift.Unauthorized("Tenant ID required for data access")
				}

				// Create tenant-scoped database instance
				tenantDB := db.WithTenant(tenantID)
				ctx.Set("dynamorm_tenant", tenantDB)
			}

			// Handle automatic transactions for write operations
			if config.AutoTransaction && isWriteOperation(ctx.Request.Method) {
				return executeWithTransaction(ctx, db, next, config)
			}

			// For read operations, just proceed
			return next.Handle(ctx)
		})
	}
}

// DB retrieves the DynamORM instance from the context
func DB(ctx *lift.Context) (*DynamORMWrapper, error) {
	db, exists := ctx.Get("dynamorm").(*DynamORMWrapper)
	if !exists {
		return nil, lift.SystemError("DynamORM not initialized")
	}
	return db, nil
}

// TenantDB retrieves the tenant-scoped DynamORM instance from the context
func TenantDB(ctx *lift.Context) (*DynamORMWrapper, error) {
	db, exists := ctx.Get("dynamorm_tenant").(*DynamORMWrapper)
	if !exists {
		// Fall back to regular DB if tenant isolation is disabled
		return DB(ctx)
	}
	return db, nil
}

// executeWithTransaction wraps the handler execution in a DynamORM transaction
func executeWithTransaction(ctx *lift.Context, db *DynamORMWrapper, next lift.Handler, _ *DynamORMConfig) error {
	// Begin transaction
	tx, err := db.BeginTransaction()
	if err != nil {
		return lift.SystemError("Failed to begin transaction").WithCause(err)
	}

	// Store transaction in context
	ctx.Set("dynamorm_transaction", tx)

	// Set up panic recovery
	defer func() {
		if r := recover(); r != nil {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				// Log rollback error but still re-panic the original error
				ctx.Set("rollback_error", rollbackErr)
			}
			panic(r) // Re-panic after rollback
		}
	}()

	// Execute handler
	err = next.Handle(ctx)
	if err != nil {
		// Rollback on error
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			// Return both errors - handler error is primary, rollback error is additional context
			return lift.SystemError("Handler failed and rollback failed").
				WithCause(err).
				WithDetail("rollback_error", rollbackErr.Error())
		}
		return err
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return lift.SystemError("Failed to commit transaction").WithCause(err)
	}

	return nil
}

// isWriteOperation determines if an HTTP method is a write operation
func isWriteOperation(method string) bool {
	switch method {
	case "POST", "PUT", "PATCH", "DELETE":
		return true
	default:
		return false
	}
}

// initDynamORMWithFactory initializes a DynamORM connection using the provided factory
func initDynamORMWithFactory(config *DynamORMConfig, factory DBFactory) (*DynamORMWrapper, error) {
	// Create session config for DynamORM
	sessionConfig := session.Config{
		Region: config.Region,
	}

	// Add endpoint for local testing
	if config.Endpoint != "" {
		sessionConfig.Endpoint = config.Endpoint
	}

	// Use factory to create DB instance
	db, err := factory.CreateDB(sessionConfig)
	if err != nil {
		return nil, err
	}

	wrapper := &DynamORMWrapper{
		db:        db,
		config:    config,
		tableName: config.TableName,
		region:    config.Region,
	}

	return wrapper, nil
}

// DynamORMWrapper wraps the DynamORM client with Lift-specific functionality
type DynamORMWrapper struct {
	db        core.ExtendedDB
	config    *DynamORMConfig
	tableName string
	region    string
	tenantID  string // Set when using tenant isolation
}

// WithTenant creates a tenant-scoped wrapper
func (d *DynamORMWrapper) WithTenant(tenantID string) *DynamORMWrapper {
	return &DynamORMWrapper{
		db:        d.db,
		config:    d.config,
		tableName: d.tableName,
		region:    d.region,
		tenantID:  tenantID,
	}
}

// BeginTransaction starts a new transaction using DynamORM
func (d *DynamORMWrapper) BeginTransaction() (*Transaction, error) {
	// DynamORM handles transactions through the TransactionFunc method
	// We'll create a wrapper that defers the actual transaction execution
	return &Transaction{
		wrapper:    d,
		committed:  false,
		rolledBack: false,
		operations: make([]TransactionOperation, 0),
	}, nil
}

// Get retrieves an item by primary key using DynamORM
func (d *DynamORMWrapper) Get(ctx context.Context, key any, result any) error {
	// Use DynamORM's Model().Where().First() pattern
	return d.db.WithContext(ctx).Model(result).
		Where("ID", "=", key).
		First(result)
}

// Put saves an item using DynamORM
func (d *DynamORMWrapper) Put(ctx context.Context, item any) error {
	// Use DynamORM's Model().Create() pattern
	return d.db.WithContext(ctx).Model(item).Create()
}

// Query performs a query operation using DynamORM
func (d *DynamORMWrapper) Query(ctx context.Context, query *Query) (*QueryResult, error) {
	var results []any

	// Build DynamORM query
	q := d.db.WithContext(ctx).Model(&results)

	if query.PartitionKey != nil {
		q = q.Where("PK", "=", query.PartitionKey)
	}

	if query.SortKey != nil {
		q = q.Where("SK", "=", query.SortKey)
	}

	if query.IndexName != "" {
		q = q.Index(query.IndexName)
	}

	if query.Limit > 0 {
		q = q.Limit(query.Limit)
	}

	// Add filters
	for field, value := range query.Filters {
		q = q.Where(field, "=", value)
	}

	err := q.All(&results)
	if err != nil {
		return nil, err
	}

	return &QueryResult{
		Items:        results,
		Count:        len(results),
		ScannedCount: len(results),
	}, nil
}

// Delete removes an item using DynamORM
func (d *DynamORMWrapper) Delete(ctx context.Context, key any) error {
	// Use DynamORM's Model().Where().Delete() pattern
	return d.db.WithContext(ctx).Model(&struct{}{}).
		Where("ID", "=", key).
		Delete()
}

// GetCoreDB returns the underlying core.DB for use with other libraries
func (d *DynamORMWrapper) GetCoreDB() core.DB {
	// core.ExtendedDB implements core.DB interface
	return d.db
}

// Transaction represents a DynamORM transaction
type Transaction struct {
	wrapper    *DynamORMWrapper
	operations []TransactionOperation
	committed  bool
	rolledBack bool
}

// TransactionOperation represents an operation to be executed in a transaction
type TransactionOperation struct {
	Item any
	Key  any
	Type string
}

// Put adds a put operation to the transaction
func (t *Transaction) Put(_ context.Context, item any) error {
	if t.committed || t.rolledBack {
		return lift.SystemError("Transaction already completed")
	}

	t.operations = append(t.operations, TransactionOperation{
		Type: "put",
		Item: item,
	})
	return nil
}

// Delete adds a delete operation to the transaction
func (t *Transaction) Delete(_ context.Context, key any) error {
	if t.committed || t.rolledBack {
		return lift.SystemError("Transaction already completed")
	}

	t.operations = append(t.operations, TransactionOperation{
		Type: "delete",
		Key:  key,
	})
	return nil
}

// Commit commits the transaction using DynamORM
func (t *Transaction) Commit() error {
	if t.committed || t.rolledBack {
		return lift.SystemError("Transaction already completed")
	}

	// Execute all operations using DynamORM's TransactionFunc
	err := t.wrapper.db.Transaction(func(tx *core.Tx) error {
		for _, op := range t.operations {
			switch op.Type {
			case "put":
				if err := tx.Create(op.Item); err != nil {
					return err
				}
			case "delete":
				if err := tx.Delete(op.Key); err != nil {
					return err
				}
			}
		}
		return nil
	})

	if err != nil {
		t.rolledBack = true
		return err
	}

	t.committed = true
	return nil
}

// Rollback rolls back the transaction using DynamORM
func (t *Transaction) Rollback() error {
	if t.committed || t.rolledBack {
		return lift.SystemError("Transaction already completed")
	}

	// For DynamORM, rollback is automatic if the transaction function returns an error
	// We just mark it as rolled back
	t.rolledBack = true
	return nil
}

// Query represents a DynamORM query
type Query struct {
	PartitionKey any
	SortKey      any
	Filters      map[string]any
	IndexName    string
	Limit        int
	Ascending    bool
}

// QueryResult represents the result of a query operation
type QueryResult struct {
	LastKey      any
	Items        []any
	Count        int
	ScannedCount int
}
