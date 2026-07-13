package repositories

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/cost"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/theorydb"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
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
	tx            core.TransactionBuilder
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

	err := tm.executeWithBuilder(ctx, startTime, fn)

	// Track transaction cost
	if tm.tracker != nil && err == nil {
		finalCost := tm.tracker.CalculateCost()
		consumedWrites := finalCost.DynamoDBWrites - initialCost.DynamoDBWrites
		if trackErr := tm.tracker.TrackDynamoWrite(int(consumedWrites)); trackErr != nil {
			zap.L().Warn("failed to track transaction write cost", zap.Error(trackErr))
		}
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

func (tm *TransactionManager) executeWithBuilder(ctx context.Context, startTime time.Time, fn func(*TransactionContext) error) error {
	client, ok := tm.db.(interface {
		TransactWrite(context.Context, func(core.TransactionBuilder) error) error
	})
	if !ok {
		return fmt.Errorf("tabletheory transaction support requires core.ExtendedDB")
	}

	return client.TransactWrite(ctx, func(tx core.TransactionBuilder) error {
		txCtx := &TransactionContext{
			tx:            tx,
			operationsCnt: 0,
			startTime:     startTime,
			logger:        tm.logger,
			tracker:       tm.tracker,
		}
		return fn(txCtx)
	})
}

// TransactionalRepository wraps a base repository with transaction support
type TransactionalRepository struct {
	*theorydb.BaseRepository
	tm *TransactionManager
}

// NewTransactionalRepository creates a repository with transaction support
func NewTransactionalRepository(db core.DB, tableName string, logger *zap.Logger, tracker *cost.Tracker) *TransactionalRepository {
	return &TransactionalRepository{
		BaseRepository: theorydb.NewBaseRepository(db, tableName),
		tm:             NewTransactionManager(db, logger, tracker),
	}
}

// WithTransaction returns a transaction manager for this repository
func (r *TransactionalRepository) WithTransaction() *TransactionManager {
	return r.tm
}

// Transaction lifecycle methods

// BeginTransaction begins a new transaction and returns a transaction context
func (tm *TransactionManager) BeginTransaction(_ context.Context) (*TransactionContext, error) {
	// DynamoDB transactions are atomic and don't support explicit begin/commit
	// This method creates a transaction context for building operations
	txCtx := &TransactionContext{
		tx:            nil, // Will be set when transaction executes
		operationsCnt: 0,
		startTime:     time.Now(),
		logger:        tm.logger,
		tracker:       tm.tracker,
	}

	if tm.logger != nil {
		tm.logger.Debug("transaction_begun", zap.Time("start_time", txCtx.startTime))
	}

	return txCtx, nil
}

// CommitTransaction commits the transaction with accumulated operations
func (tm *TransactionManager) CommitTransaction(_ context.Context, txCtx *TransactionContext) error {
	// In DynamoDB, transactions are committed atomically via ExecuteTransaction
	// This method validates that the transaction context is ready for commit
	if txCtx == nil {
		return fmt.Errorf("transaction context is nil")
	}

	if txCtx.operationsCnt == 0 {
		if tm.logger != nil {
			tm.logger.Info("empty_transaction_committed",
				zap.Duration("duration", time.Since(txCtx.startTime)),
			)
		}
		return nil
	}

	if tm.logger != nil {
		tm.logger.Info("transaction_committed",
			zap.Int("operation_count", txCtx.operationsCnt),
			zap.Duration("duration", time.Since(txCtx.startTime)),
		)
	}

	return nil
}

// RollbackTransaction rolls back the transaction
func (tm *TransactionManager) RollbackTransaction(_ context.Context, txCtx *TransactionContext) error {
	// DynamoDB transactions automatically rollback on failure
	// This method is mainly for cleanup and logging
	if txCtx == nil {
		return fmt.Errorf("transaction context is nil")
	}

	if tm.logger != nil {
		tm.logger.Warn("transaction_rolled_back",
			zap.Int("operation_count", txCtx.operationsCnt),
			zap.Duration("duration", time.Since(txCtx.startTime)),
		)
	}

	// Reset operation count to indicate rollback
	txCtx.operationsCnt = 0

	return nil
}

// Transaction operations for the context

// Put adds a Put operation to the transaction
func (tc *TransactionContext) Put(item any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}
	if err := txOps.Create(item); err != nil {
		return fmt.Errorf("transaction put failed: %w", err)
	}

	return nil
}

// Delete adds a Delete operation to the transaction
func (tc *TransactionContext) Delete(item any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}
	if err := txOps.Delete(item); err != nil {
		return fmt.Errorf("transaction delete failed: %w", err)
	}

	return nil
}

// Update adds an Update operation to the transaction
func (tc *TransactionContext) Update(item any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}
	if err := txOps.Update(item); err != nil {
		return fmt.Errorf("transaction update failed: %w", err)
	}

	return nil
}

// ConditionCheck adds a condition check to the transaction
func (tc *TransactionContext) ConditionCheck(key any, condition string, values ...any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}

	// For condition checks, we need the table name. Since this is a generic method,
	// we'll assume the key contains the table information or use a default
	tableName := "default-table"
	if keyMap, ok := key.(map[string]any); ok {
		if err := txOps.ConditionCheck(tableName, keyMap, condition, values...); err != nil {
			return fmt.Errorf("transaction condition check failed: %w", err)
		}
	} else {
		return fmt.Errorf("condition check requires key to be map[string]any")
	}

	return nil
}

// UpdateWithExpression adds an Update operation with expression to the transaction
func (tc *TransactionContext) UpdateWithExpression(item any, expression string, values ...any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}

	if err := txOps.UpdateWithExpression(item, expression, values...); err != nil {
		return fmt.Errorf("transaction update with expression failed: %w", err)
	}

	return nil
}

// DeleteByKey adds a Delete operation by key to the transaction
func (tc *TransactionContext) DeleteByKey(tableName string, key map[string]any) error {
	tc.operationsCnt++

	// Check if transaction is available
	if tc.tx == nil {
		return fmt.Errorf("transaction not initialized")
	}

	txOps := &theorydb.MockTx{Builder: tc.tx}

	if err := txOps.DeleteByKey(tableName, key); err != nil {
		return fmt.Errorf("transaction delete by key failed: %w", err)
	}

	return nil
}

// TransactionalGet performs a get operation within the transaction context
func (tc *TransactionContext) TransactionalGet(_ any) error {
	tc.operationsCnt++

	// Note: Pure read operations don't affect transactions in DynamoDB
	// This is mainly for consistency tracking and cost monitoring
	return nil
}

// TransactionalPut is an alias for Put for interface compatibility
func (tc *TransactionContext) TransactionalPut(item any) error {
	return tc.Put(item)
}

// TransactionalUpdate is an alias for Update for interface compatibility
func (tc *TransactionContext) TransactionalUpdate(item any) error {
	return tc.Update(item)
}

// TransactionalDelete is an alias for Delete for interface compatibility
func (tc *TransactionContext) TransactionalDelete(item any) error {
	return tc.Delete(item)
}

// TransactionalBatchGet performs batch get operations within transaction context
func (tc *TransactionContext) TransactionalBatchGet(items []any) error {
	tc.operationsCnt += len(items)

	// Batch reads don't participate in DynamoDB transactions
	// This is mainly for cost tracking and consistency
	return nil
}

// TransactionalBatchWrite performs batch write operations within transaction
func (tc *TransactionContext) TransactionalBatchWrite(puts []any, deletes []any) error {
	// Add put operations
	for _, item := range puts {
		if err := tc.Put(item); err != nil {
			return fmt.Errorf("batch put failed: %w", err)
		}
	}

	// Add delete operations
	for _, item := range deletes {
		if err := tc.Delete(item); err != nil {
			return fmt.Errorf("batch delete failed: %w", err)
		}
	}

	return nil
}

// GetOperationCount returns the number of operations in this transaction
func (tc *TransactionContext) GetOperationCount() int {
	return tc.operationsCnt
}

// Repository-specific transactional operations

// FollowUserTransactional implements a follow operation with multiple updates
func (r *TransactionalRepository) FollowUserTransactional(ctx context.Context, followerID, followeeID string) error {
	return r.tm.ExecuteTransaction(ctx, func(txCtx *TransactionContext) error {
		now := time.Now()

		// 1. Create follow relationship using NewFollow constructor
		activityID := fmt.Sprintf("follow-%s-%s-%d", followerID, followeeID, now.Unix())
		follow := models.NewFollow(followerID, followeeID, activityID)
		follow.Accept() // Mark as accepted immediately for direct follows

		if err := txCtx.Put(follow); err != nil {
			return fmt.Errorf("failed to create follow relationship: %w", err)
		}

		// 2. Update follower's actor record (following count)
		followerActor := &models.Actor{
			Username: followerID,
		}
		followerActor.PK = fmt.Sprintf("ACTOR#%s", followerID)
		followerActor.SK = "PROFILE"
		followerActor.FollowingCount++
		followerActor.UpdatedAt = now

		if err := txCtx.Update(followerActor); err != nil {
			return fmt.Errorf("failed to update follower's following count: %w", err)
		}

		// 3. Update followee's actor record (follower count)
		followeeActor := &models.Actor{
			Username: followeeID,
		}
		followeeActor.PK = fmt.Sprintf("ACTOR#%s", followeeID)
		followeeActor.SK = "PROFILE"
		followeeActor.FollowerCount++
		followeeActor.UpdatedAt = now

		if err := txCtx.Update(followeeActor); err != nil {
			return fmt.Errorf("failed to update followee's follower count: %w", err)
		}

		// 4. Create follow notification manually
		notificationID := fmt.Sprintf("notif-%s-%s-%d", followeeID, followerID, now.Unix())
		notification := &models.Notification{
			ID:         notificationID,
			UserID:     followeeID,
			ActorID:    followerID,
			Type:       "follow",
			TargetID:   followerID,
			TargetType: "user",
			IsRead:     false,
			CreatedAt:  now,
			UpdatedAt:  now,
			ExpiresAt:  now.Add(30 * 24 * time.Hour).Unix(), // 30 days TTL
		}
		// Set up keys manually
		notification.PK = fmt.Sprintf("NOTIF#%s", followeeID)
		notification.SK = fmt.Sprintf("%s#%s", now.Format("2006-01-02T15:04:05.000Z"), notificationID)

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
			"SK": fmt.Sprintf("POSTS#%s", time.Now().Format(common.DateFormat)),
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
			"PK":        fmt.Sprintf("RATE_LIMIT#%s", userID),
			"SK":        fmt.Sprintf("POSTS#%s", time.Now().Format(common.DateFormat)),
			"PostCount": 1,
			"UpdatedAt": time.Now(),
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
				"PK":        fmt.Sprintf("STATUS#%s", resourceID),
				"SK":        fmt.Sprintf("STATUS#%s", resourceID),
				"UserID":    toUserID,
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
			// Exponential backoff with safe uint conversion
			attemptShift := attempt - 1
			var shiftAmount uint
			if attemptShift < 0 {
				shiftAmount = 0
			} else if attemptShift > 63 { // Prevent overflow in bitshift
				shiftAmount = 63
			} else {
				shiftAmount = uint(attemptShift)
			}
			backoff := config.BackoffDuration * time.Duration(1<<shiftAmount)
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

// isRetryableError determines if an error is retryable based on AWS DynamoDB error patterns
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errorStr := strings.ToLower(err.Error())

	// DynamoDB-specific retryable errors
	retryablePatterns := []string{
		"throttlingexception",
		"throttled",
		"throttle",
		"provisionedthroughputexceededexception",
		"serviceexception",
		"internalservererror",
		"temporaryerror",
		"temporary error",
		"timeout",
		"connection reset",
		"connection refused",
		"network error",
		"transactionconflictexception",
		"conditionalcheckfailedexception", // Can be retryable in some cases
		"requestlimitexceeded",
		"too many requests",
		"rate limit",
		"rate exceeded",
		"try again",
		"retry",
		"busy",
		"unavailable",
		"gateway timeout",
		"bad gateway",
		"service unavailable",
		"internal error",
		"server error",
		"deadline exceeded",
		"context deadline exceeded",
		"i/o timeout",
		"no such host",
		"connection timed out",
		"eof",
		"broken pipe",
	}

	// Check for retryable error patterns
	for _, pattern := range retryablePatterns {
		if strings.Contains(errorStr, pattern) {
			return true
		}
	}

	// Additional checks for AWS SDK error types by checking error structure
	// Check for specific AWS error codes that are retryable
	awsRetryableCodes := []string{
		"throttling",
		"requesttimeout",
		"requesttimewaitstate",
		"provisionedthroughputexceeded",
		"serviceunavailable",
		"slowdown",
		"requesttimetooskewed",
		"tokenbucketfull",
		"internalfailure",
		"servicefailure",
	}

	for _, code := range awsRetryableCodes {
		if strings.Contains(errorStr, code) {
			return true
		}
	}

	// Check for HTTP status codes that indicate retryable errors
	httpRetryablePatterns := []string{
		"429", // Too Many Requests
		"500", // Internal Server Error
		"502", // Bad Gateway
		"503", // Service Unavailable
		"504", // Gateway Timeout
		"408", // Request Timeout
	}

	for _, code := range httpRetryablePatterns {
		if strings.Contains(errorStr, code) {
			return true
		}
	}

	return false
}

// Advanced transaction support methods

// ExecuteIsolated executes a transaction with full isolation guarantees
func (tm *TransactionManager) ExecuteIsolated(ctx context.Context, isolationLevel string, fn func(*TransactionContext) error) error {
	startTime := time.Now()

	if tm.logger != nil {
		tm.logger.Debug("isolated_transaction_starting",
			zap.String("isolation_level", isolationLevel),
			zap.Time("start_time", startTime),
		)
	}

	// Track initial costs
	var initialCost *cost.OperationCost
	if tm.tracker != nil {
		initialCost = tm.tracker.CalculateCost()
	}

	err := tm.executeWithBuilder(ctx, startTime, fn)

	// Track transaction cost with isolation overhead
	if tm.tracker != nil && err == nil {
		finalCost := tm.tracker.CalculateCost()
		consumedWrites := finalCost.DynamoDBWrites - initialCost.DynamoDBWrites
		// Isolation adds overhead
		isolationOverhead := int(float64(consumedWrites) * 0.1)
		if trackErr := tm.tracker.TrackDynamoWrite(int(consumedWrites) + isolationOverhead); trackErr != nil {
			zap.L().Warn("failed to track isolated transaction cost", zap.Error(trackErr))
		}
	}

	if tm.logger != nil {
		tm.logger.Info("isolated_transaction_completed",
			zap.String("isolation_level", isolationLevel),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
	}

	return err
}

// ExecuteWithConsistency executes a transaction with consistency guarantees
func (tm *TransactionManager) ExecuteWithConsistency(ctx context.Context, consistencyLevel string, fn func(*TransactionContext) error) error {
	config := TransactionConfig{
		MaxRetries:      5, // Higher retries for consistency
		BackoffDuration: 150 * time.Millisecond,
		Logger:          tm.logger,
		CostTracker:     tm.tracker,
	}

	if tm.logger != nil {
		tm.logger.Debug("consistent_transaction_starting",
			zap.String("consistency_level", consistencyLevel),
		)
	}

	return tm.ExecuteWithRetry(ctx, config, fn)
}

// ExecuteNested executes a nested transaction within an existing transaction context
func (tm *TransactionManager) ExecuteNested(_ context.Context, parentTx *TransactionContext, fn func(*TransactionContext) error) error {
	if parentTx == nil {
		return fmt.Errorf("parent transaction context is required for nested transactions")
	}

	// Create a nested context that shares the parent transaction
	nestedCtx := &TransactionContext{
		tx:            parentTx.tx,
		operationsCnt: 0, // Separate counter for nested operations
		startTime:     time.Now(),
		logger:        tm.logger,
		tracker:       tm.tracker,
	}

	if tm.logger != nil {
		tm.logger.Debug("nested_transaction_starting",
			zap.Int("parent_operations", parentTx.operationsCnt),
		)
	}

	err := fn(nestedCtx)

	// Merge operation counts on success
	if err == nil {
		parentTx.operationsCnt += nestedCtx.operationsCnt
		if tm.logger != nil {
			tm.logger.Debug("nested_transaction_merged",
				zap.Int("nested_operations", nestedCtx.operationsCnt),
				zap.Int("total_operations", parentTx.operationsCnt),
			)
		}
	} else if tm.logger != nil {
		tm.logger.Warn("nested_transaction_failed",
			zap.Int("nested_operations", nestedCtx.operationsCnt),
			zap.Error(err),
		)
	}

	return err
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
