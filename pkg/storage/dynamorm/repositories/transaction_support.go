package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/cost"
	"github.com/aron23/lesser/pkg/storage/dynamorm"
	"github.com/pay-theory/dynamorm/pkg/core"
	"go.uber.org/zap"
)

// TransactionManager provides high-level transaction management for repositories
type TransactionManager struct {
	db      core.DB
	logger  *zap.Logger
	tracker *cost.Tracker
}

// NewTransactionManager creates a new transaction manager
func NewTransactionManager(db core.DB, logger *zap.Logger, tracker *cost.Tracker) *TransactionManager {
	return &TransactionManager{
		db:      db,
		logger:  logger,
		tracker: tracker,
	}
}

// TransactionContext holds context for a transaction
type TransactionContext struct {
	tx            *core.Tx
	operationsCnt int
	startTime     time.Time
	logger        *zap.Logger
	tracker       *cost.Tracker
}

// ExecuteTransaction executes a function within a transaction
func (tm *TransactionManager) ExecuteTransaction(ctx context.Context, fn func(*TransactionContext) error) error {
	startTime := time.Now()
	
	// Track initial costs
	var initialCost *cost.OperationCost
	if tm.tracker != nil {
		initialCost = tm.tracker.CalculateCost()
	}

	err := tm.db.Transaction(func(tx *core.Tx) error {
		txCtx := &TransactionContext{
			tx:            tx,
			operationsCnt: 0,
			startTime:     startTime,
			logger:        tm.logger,
			tracker:       tm.tracker,
		}

		// Execute the transaction function
		return fn(txCtx)
	})

	// Track transaction cost
	if tm.tracker != nil && err == nil {
		finalCost := tm.tracker.CalculateCost()
		consumedWrites := finalCost.DynamoDBWrites - initialCost.DynamoDBWrites
		tm.tracker.TrackDynamoWrite(int(consumedWrites))
	}

	// Log transaction result
	if tm.logger != nil {
		tm.logger.Info("transaction_executed",
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
	}

	return err
}

// TransactionalRepository wraps a base repository with transaction support
type TransactionalRepository struct {
	*dynamorm.BaseRepository
	tm *TransactionManager
}

// NewTransactionalRepository creates a repository with transaction support
func NewTransactionalRepository(db core.DB, tableName string, logger *zap.Logger, tracker *cost.Tracker) *TransactionalRepository {
	return &TransactionalRepository{
		BaseRepository: dynamorm.NewBaseRepository(db, tableName),
		tm:             NewTransactionManager(db, logger, tracker),
	}
}

// WithTransaction returns a transaction manager for this repository
func (r *TransactionalRepository) WithTransaction() *TransactionManager {
	return r.tm
}

// Transaction operations for the context

// Put adds a Put operation to the transaction
func (tc *TransactionContext) Put(item any) error {
	tc.operationsCnt++
	// Use DynamORM transaction put - this is a placeholder for actual implementation
	// In practice, you'd need to integrate with DynamORM's transaction API
	return fmt.Errorf("transaction put not yet implemented in DynamORM - placeholder")
}

// Delete adds a Delete operation to the transaction
func (tc *TransactionContext) Delete(item any) error {
	tc.operationsCnt++
	// Use DynamORM transaction delete - this is a placeholder
	return fmt.Errorf("transaction delete not yet implemented in DynamORM - placeholder")
}

// Update adds an Update operation to the transaction
func (tc *TransactionContext) Update(item any) error {
	tc.operationsCnt++
	// Use DynamORM transaction update - this is a placeholder
	return fmt.Errorf("transaction update not yet implemented in DynamORM - placeholder")
}

// ConditionCheck adds a condition check to the transaction
func (tc *TransactionContext) ConditionCheck(item any, condition string, values ...any) error {
	tc.operationsCnt++
	// Use DynamORM transaction condition check - this is a placeholder
	return fmt.Errorf("transaction condition check not yet implemented in DynamORM - placeholder")
}

// GetOperationCount returns the number of operations in this transaction
func (tc *TransactionContext) GetOperationCount() int {
	return tc.operationsCnt
}

// Repository-specific transactional operations

// FollowUserTransactional implements a follow operation with multiple updates
func (r *TransactionalRepository) FollowUserTransactional(ctx context.Context, followerID, followeeID string) error {
	return r.tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		// This is a conceptual example - actual implementation would depend on 
		// DynamORM's transaction API when it becomes available
		
		// 1. Create follow relationship
		follow := map[string]any{
			"PK":         fmt.Sprintf("USER#%s", followerID),
			"SK":         fmt.Sprintf("FOLLOWS#%s", followeeID),
			"FollowerID": followerID,
			"FolloweeID": followeeID,
			"CreatedAt":  time.Now(),
		}
		if err := txCtx.Put(follow); err != nil {
			return fmt.Errorf("failed to create follow relationship: %w", err)
		}

		// 2. Update follower count (conditional)
		followeeUpdate := map[string]any{
			"PK": fmt.Sprintf("USER#%s", followeeID),
			"SK": fmt.Sprintf("USER#%s", followeeID),
		}
		if err := txCtx.Update(followeeUpdate); err != nil {
			return fmt.Errorf("failed to update follower count: %w", err)
		}

		// 3. Update following count
		followerUpdate := map[string]any{
			"PK": fmt.Sprintf("USER#%s", followerID),
			"SK": fmt.Sprintf("USER#%s", followerID),
		}
		if err := txCtx.Update(followerUpdate); err != nil {
			return fmt.Errorf("failed to update following count: %w", err)
		}

		// 4. Add notification
		notification := map[string]any{
			"PK":           fmt.Sprintf("USER#%s", followeeID),
			"SK":           fmt.Sprintf("NOTIF#%s#%s", time.Now().Format("20060102150405"), followerID),
			"Type":         "follow",
			"ActorID":      followerID,
			"CreatedAt":    time.Now(),
			"IsRead":       false,
		}
		if err := txCtx.Put(notification); err != nil {
			return fmt.Errorf("failed to create notification: %w", err)
		}

		return nil
	})
}

// CreateStatusWithChecksTransactional creates a status with validation checks
func (r *TransactionalRepository) CreateStatusWithChecksTransactional(ctx context.Context, status map[string]any) error {
	return r.tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		userID := status["UserID"].(string)
		
		// 1. Check user exists and is not suspended
		userCheck := map[string]any{
			"PK": fmt.Sprintf("USER#%s", userID),
			"SK": fmt.Sprintf("USER#%s", userID),
		}
		if err := txCtx.ConditionCheck(userCheck, "attribute_exists(PK) AND Suspended = :false", false); err != nil {
			return fmt.Errorf("user validation failed: %w", err)
		}

		// 2. Check rate limits
		rateLimitCheck := map[string]any{
			"PK": fmt.Sprintf("RATE_LIMIT#%s", userID),
			"SK": fmt.Sprintf("POSTS#%s", time.Now().Format("2006-01-02")),
		}
		if err := txCtx.ConditionCheck(rateLimitCheck, "PostCount < :limit", 300); err != nil {
			return fmt.Errorf("rate limit exceeded: %w", err)
		}

		// 3. Create status
		if err := txCtx.Put(status); err != nil {
			return fmt.Errorf("failed to create status: %w", err)
		}

		// 4. Update rate limit
		rateLimitUpdate := map[string]any{
			"PK": fmt.Sprintf("RATE_LIMIT#%s", userID),
			"SK": fmt.Sprintf("POSTS#%s", time.Now().Format("2006-01-02")),
		}
		if err := txCtx.Update(rateLimitUpdate); err != nil {
			return fmt.Errorf("failed to update rate limit: %w", err)
		}

		return nil
	})
}

// TransferOwnershipTransactional transfers ownership of resources atomically
func (r *TransactionalRepository) TransferOwnershipTransactional(ctx context.Context, fromUserID, toUserID string, resourceIDs []string) error {
	return r.tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		// Validate both users exist
		fromUserCheck := map[string]any{
			"PK": fmt.Sprintf("USER#%s", fromUserID),
			"SK": fmt.Sprintf("USER#%s", fromUserID),
		}
		if err := txCtx.ConditionCheck(fromUserCheck, "attribute_exists(PK)", nil); err != nil {
			return fmt.Errorf("source user not found: %w", err)
		}

		toUserCheck := map[string]any{
			"PK": fmt.Sprintf("USER#%s", toUserID),
			"SK": fmt.Sprintf("USER#%s", toUserID),
		}
		if err := txCtx.ConditionCheck(toUserCheck, "attribute_exists(PK)", nil); err != nil {
			return fmt.Errorf("target user not found: %w", err)
		}

		// Transfer each resource
		for _, resourceID := range resourceIDs {
			resource := map[string]any{
				"PK":     fmt.Sprintf("STATUS#%s", resourceID),
				"SK":     fmt.Sprintf("STATUS#%s", resourceID),
				"UserID": toUserID,
				"UpdatedAt": time.Now(),
			}
			if err := txCtx.Update(resource); err != nil {
				return fmt.Errorf("failed to transfer resource %s: %w", resourceID, err)
			}
		}

		// Log transfer
		transferLog := map[string]any{
			"PK":          "TRANSFER_LOG",
			"SK":          fmt.Sprintf("TRANSFER#%s", time.Now().Format("20060102150405")),
			"FromUserID":  fromUserID,
			"ToUserID":    toUserID,
			"ResourceIDs": resourceIDs,
			"Timestamp":   time.Now(),
		}
		if err := txCtx.Put(transferLog); err != nil {
			return fmt.Errorf("failed to log transfer: %w", err)
		}

		return nil
	})
}

// TransactionConfig holds configuration for transaction operations
type TransactionConfig struct {
	MaxRetries      int
	BackoffDuration time.Duration
	Logger          *zap.Logger
	CostTracker     *cost.Tracker
}

// DefaultTransactionConfig returns default transaction configuration
func DefaultTransactionConfig() TransactionConfig {
	return TransactionConfig{
		MaxRetries:      3,
		BackoffDuration: 100 * time.Millisecond,
		Logger:          nil,
		CostTracker:     nil,
	}
}

// ExecuteWithRetry executes a transaction with retry logic
func (tm *TransactionManager) ExecuteWithRetry(ctx context.Context, config TransactionConfig, fn func(*TransactionContext) error) error {
	var lastErr error
	
	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			backoff := config.BackoffDuration * time.Duration(1<<uint(attempt-1))
			if config.Logger != nil {
				config.Logger.Info("retrying_transaction",
					zap.Int("attempt", attempt),
					zap.Duration("backoff", backoff),
				)
			}
			
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		err := tm.ExecuteTransaction(ctx, fn)
		if err == nil {
			if config.Logger != nil && attempt > 0 {
				config.Logger.Info("transaction_succeeded_after_retry",
					zap.Int("attempts", attempt+1),
				)
			}
			return nil
		}

		lastErr = err
		
		// Check if error is retryable
		if !isRetryableError(err) {
			break
		}
	}

	if config.Logger != nil {
		config.Logger.Error("transaction_failed_after_retries",
			zap.Int("max_attempts", config.MaxRetries+1),
			zap.Error(lastErr),
		)
	}

	return fmt.Errorf("transaction failed after %d attempts: %w", config.MaxRetries+1, lastErr)
}

// isRetryableError determines if an error is retryable
func isRetryableError(err error) bool {
	// This is a simplified implementation - in practice, you'd check for specific
	// DynamoDB error types like ConditionalCheckFailedException, ThrottlingException, etc.
	if err == nil {
		return false
	}
	
	errorStr := err.Error()
	// Check for common retryable patterns
	retryablePatterns := []string{
		"temporary error",
		"throttle",
		"timeout",
		"connection",
		"retry",
	}
	
	for _, pattern := range retryablePatterns {
		if strings.Contains(strings.ToLower(errorStr), pattern) {
			return true
		}
	}
	
	return false
}

// Utility functions for common transaction patterns

// ConditionalCreate creates an item only if it doesn't exist
func ConditionalCreate(txCtx *TransactionContext, item any, key map[string]any) error {
	// Check item doesn't exist
	if err := txCtx.ConditionCheck(key, "attribute_not_exists(PK)", nil); err != nil {
		return fmt.Errorf("item already exists: %w", err)
	}
	
	// Create item
	return txCtx.Put(item)
}

// ConditionalUpdate updates an item only if it exists and meets conditions
func ConditionalUpdate(txCtx *TransactionContext, item any, key map[string]any, condition string, values ...any) error {
	// Check condition
	if err := txCtx.ConditionCheck(key, condition, values...); err != nil {
		return fmt.Errorf("condition check failed: %w", err)
	}
	
	// Update item
	return txCtx.Update(item)
}

// ConditionalDelete deletes an item only if it exists and meets conditions
func ConditionalDelete(txCtx *TransactionContext, item any, key map[string]any, condition string, values ...any) error {
	// Check condition
	if err := txCtx.ConditionCheck(key, condition, values...); err != nil {
		return fmt.Errorf("condition check failed: %w", err)
	}
	
	// Delete item
	return txCtx.Delete(item)
}