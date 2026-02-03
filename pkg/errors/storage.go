package errors // nolint:revive // Legacy package name; import with an alias when also using stdlib errors.

// Storage domain errors
// Consolidates errors from pkg/storage/errors.go, database-related errors, and repository operations

// NewStorageError creates a new storage error with the specified error code and message.
func NewStorageError(code ErrorCode, message string) *AppError {
	return NewAppError(code, CategoryStorage, message)
}

// NewStorageInternalError creates a storage error with internal details wrapped from an underlying error.
func NewStorageInternalError(code ErrorCode, message string, internal error) *AppError {
	return WrapError(internal, code, CategoryStorage, message)
}

// ItemNotFound creates an error indicating an item was not found.
func ItemNotFound(itemType string) *AppError {
	return NewStorageError(CodeNotFound, "Item not found").
		WithMetadata("item_type", itemType)
}

// ItemNotFoundWithID creates an error indicating an item with the specified ID was not found.
func ItemNotFoundWithID(itemType, id string) *AppError {
	return NewStorageError(CodeNotFound, "Item not found").
		WithMetadata("item_type", itemType).
		WithMetadata("id", id)
}

// ItemAlreadyExists creates an error indicating an item already exists.
func ItemAlreadyExists(itemType string) *AppError {
	return NewStorageError(CodeAlreadyExists, "Item already exists").
		WithMetadata("item_type", itemType)
}

// ItemAlreadyExistsWithID creates an error indicating an item with the specified ID already exists.
func ItemAlreadyExistsWithID(itemType, id string) *AppError {
	return NewStorageError(CodeAlreadyExists, "Item already exists").
		WithMetadata("item_type", itemType).
		WithMetadata("id", id)
}

// DatabaseConnectionFailed creates an error indicating database connection failed.
func DatabaseConnectionFailed(err error) *AppError {
	return NewStorageInternalError(CodeDatabaseConnection, "Database connection failed", err).AsRetryable()
}

// DatabaseTimeout creates an error indicating a database operation timed out.
func DatabaseTimeout(operation string) *AppError {
	return NewStorageError(CodeTimeout, "Database operation timed out").
		WithMetadata("operation", operation).AsRetryable()
}

// DatabaseUnavailable creates an error indicating the database service is unavailable.
func DatabaseUnavailable(err error) *AppError {
	return NewStorageInternalError(CodeExternalServiceUnavailable, "Database service unavailable", err).AsRetryable()
}

// QueryFailed creates an error indicating a database query failed.
func QueryFailed(query string, err error) *AppError {
	return NewStorageInternalError(CodeQueryFailed, "Database query failed", err).
		WithMetadata("query_type", query)
}

// QueryInvalid creates an error indicating a database query is invalid.
func QueryInvalid(query string, reason string) *AppError {
	return NewStorageError(CodeBadRequest, "Invalid database query").
		WithMetadata("query_type", query).
		WithMetadata("reason", reason)
}

// TransactionFailed creates an error indicating a database transaction failed.
func TransactionFailed(err error) *AppError {
	return NewStorageInternalError(CodeTransactionFailed, "Database transaction failed", err).AsRetryable()
}

// TransactionConflict creates an error indicating a transaction conflict was detected.
func TransactionConflict(resource string) *AppError {
	return NewStorageError(CodeConcurrencyError, "Transaction conflict detected").
		WithMetadata("resource", resource).AsRetryable()
}

// IndexError creates an error indicating a database index error occurred.
func IndexError(indexName string, err error) *AppError {
	return NewStorageInternalError(CodeIndexError, "Database index error", err).
		WithMetadata("index", indexName)
}

// ConstraintViolated creates an error indicating a database constraint was violated.
func ConstraintViolated(constraint string, err error) *AppError {
	return NewStorageInternalError(CodeConstraintViolated, "Database constraint violated", err).
		WithMetadata("constraint", constraint)
}

// UniqueConstraintViolated creates an error indicating a unique constraint was violated.
func UniqueConstraintViolated(field string) *AppError {
	return NewStorageError(CodeAlreadyExists, "Duplicate value violates uniqueness").
		WithMetadata("field", field)
}

// DynamoDBProvisionedThroughputExceeded creates an error indicating DynamoDB capacity was exceeded.
func DynamoDBProvisionedThroughputExceeded() *AppError {
	return NewStorageError(CodeRateLimited, "Database capacity exceeded").AsRetryable()
}

// DynamoDBItemTooLarge creates an error indicating a DynamoDB item exceeds maximum size.
func DynamoDBItemTooLarge() *AppError {
	return NewStorageError(CodeContentTooLarge, "Item exceeds maximum size")
}

// DynamoDBConditionalCheckFailed creates an error indicating a DynamoDB conditional check failed.
func DynamoDBConditionalCheckFailed(condition string) *AppError {
	return NewStorageError(CodeConflict, "Conditional check failed").
		WithMetadata("condition", condition)
}

// CreateFailed creates an error indicating item creation failed.
func CreateFailed(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to create item", err).
		WithMetadata("item_type", itemType).AsRetryable()
}

// UpdateFailed creates an error indicating item update failed.
func UpdateFailed(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to update item", err).
		WithMetadata("item_type", itemType).AsRetryable()
}

// DeleteFailed creates an error indicating item deletion failed.
func DeleteFailed(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to delete item", err).
		WithMetadata("item_type", itemType).AsRetryable()
}

// GetFailed creates an error indicating item retrieval failed.
func GetFailed(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to retrieve item", err).
		WithMetadata("item_type", itemType).AsRetryable()
}

// ListFailed creates an error indicating item listing failed.
func ListFailed(itemType string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Failed to list items", err).
		WithMetadata("item_type", itemType).AsRetryable()
}

// QueryByFieldFailed creates an error indicating querying by field failed.
func QueryByFieldFailed(itemType, field string, err error) *AppError {
	return NewStorageInternalError(CodeQueryFailed, "Failed to query by field", err).
		WithMetadata("item_type", itemType).
		WithMetadata("field", field).AsRetryable()
}

// BatchOperationFailed creates an error indicating a batch operation failed.
func BatchOperationFailed(operation string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Batch operation failed", err).
		WithMetadata("operation", operation).AsRetryable()
}

// BatchSizeExceeded creates an error indicating batch size exceeds maximum.
func BatchSizeExceeded(size, maxSize int) *AppError {
	return NewStorageError(CodeBadRequest, "Batch size exceeds maximum").
		WithMetadata("size", size).
		WithMetadata("max_size", maxSize)
}

// BatchPartialFailure creates an error indicating a batch operation partially failed.
func BatchPartialFailure(successCount, failureCount int) *AppError {
	return NewStorageError(CodeInternal, "Batch operation partially failed").
		WithMetadata("success_count", successCount).
		WithMetadata("failure_count", failureCount)
}

// InvalidInput creates an error indicating invalid input data.
func InvalidInput(field, reason string) *AppError {
	return NewStorageError(CodeInvalidInput, "Invalid input data").
		WithMetadata("field", field).
		WithMetadata("reason", reason)
}

// StorageRequiredFieldMissing creates an error indicating a required field is missing.
func StorageRequiredFieldMissing(field string) *AppError {
	return NewStorageError(CodeRequiredFieldMissing, "Required field is missing").
		WithMetadata("field", field)
}

// StorageFieldTooLong creates an error indicating a field exceeds maximum length.
func StorageFieldTooLong(field string, maxLength int) *AppError {
	return NewStorageError(CodeFieldTooLong, "Field exceeds maximum length").
		WithMetadata("field", field).
		WithMetadata("max_length", maxLength)
}

// StorageFieldTooShort creates an error indicating a field is below minimum length.
func StorageFieldTooShort(field string, minLength int) *AppError {
	return NewStorageError(CodeFieldTooShort, "Field is below minimum length").
		WithMetadata("field", field).
		WithMetadata("min_length", minLength)
}

// DataIntegrityViolated creates an error indicating a data integrity violation.
func DataIntegrityViolated(reason string) *AppError {
	return NewStorageError(CodeConstraintViolated, "Data integrity violation").
		WithMetadata("reason", reason)
}

// StorageQuotaExceeded creates an error indicating storage quota was exceeded.
func StorageQuotaExceeded(userID string, quota int64) *AppError {
	return NewStorageError(CodeStorageQuotaExceeded, "Storage quota exceeded").
		WithMetadata("user_id", userID).
		WithMetadata("quota", quota)
}

// FileSizeExceeded creates an error indicating file size exceeds limit.
func FileSizeExceeded(size, maxSize int64) *AppError {
	return NewStorageError(CodeContentTooLarge, "File size exceeds limit").
		WithMetadata("size", size).
		WithMetadata("max_size", maxSize)
}

// TooManyItems creates an error indicating too many items exist.
func TooManyItems(count, maxCount int) *AppError {
	return NewStorageError(CodeQuotaExceeded, "Too many items").
		WithMetadata("count", count).
		WithMetadata("max_count", maxCount)
}

// Specific entity errors commonly used across repositories

// StorageUserNotFound creates an error indicating a user was not found.
func StorageUserNotFound(username string) *AppError {
	return ItemNotFoundWithID("user", username)
}

// UserAlreadyExists creates an error indicating a user already exists.
func UserAlreadyExists(username string) *AppError {
	return ItemAlreadyExistsWithID("user", username)
}

// StorageActorNotFound creates an error indicating an actor was not found.
func StorageActorNotFound(username string) *AppError {
	return ItemNotFoundWithID("actor", username)
}

// ActorAlreadyExists creates an error indicating an actor already exists.
func ActorAlreadyExists(username string) *AppError {
	return ItemAlreadyExistsWithID("actor", username)
}

// StatusNotFound creates an error indicating a status was not found.
func StatusNotFound(statusID string) *AppError {
	return ItemNotFoundWithID("status", statusID)
}

// ObjectNotFound creates an error indicating an object was not found.
func ObjectNotFound(objectID string) *AppError {
	return ItemNotFoundWithID("object", objectID)
}

// ActivityNotFound creates an error indicating an activity was not found.
func ActivityNotFound(activityID string) *AppError {
	return ItemNotFoundWithID("activity", activityID)
}

// RelationshipNotFound creates an error indicating a relationship was not found.
func RelationshipNotFound() *AppError {
	return ItemNotFound("relationship")
}

// RelationshipAlreadyExists creates an error indicating a relationship already exists.
func RelationshipAlreadyExists() *AppError {
	return ItemAlreadyExists("relationship")
}

// ListNotFound creates an error indicating a list was not found.
func ListNotFound(listID string) *AppError {
	return ItemNotFoundWithID("list", listID)
}

// ListMemberNotFound creates an error indicating a list member was not found.
func ListMemberNotFound() *AppError {
	return ItemNotFound("list_member")
}

// ListMemberAlreadyExists creates an error indicating a list member already exists.
func ListMemberAlreadyExists() *AppError {
	return ItemAlreadyExists("list_member")
}

// StorageSessionNotFound creates an error indicating a session was not found.
func StorageSessionNotFound(sessionID string) *AppError {
	return ItemNotFoundWithID("session", sessionID)
}

// TokenNotFound creates an error indicating a token was not found.
func TokenNotFound() *AppError {
	return ItemNotFound("token")
}

// RefreshTokenNotFound creates an error indicating a refresh token was not found.
func RefreshTokenNotFound() *AppError {
	return ItemNotFound("refresh_token")
}

// PatternNotFound creates an error indicating a moderation pattern was not found.
func PatternNotFound() *AppError {
	return ItemNotFound("moderation_pattern")
}

// PatternCacheNotFound creates an error indicating a pattern cache was not found.
func PatternCacheNotFound() *AppError {
	return ItemNotFound("pattern_cache")
}

// MigrationFailed creates an error indicating database migration failed.
func MigrationFailed(version string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Database migration failed", err).
		WithMetadata("version", version)
}

// BackupFailed creates an error indicating database backup failed.
func BackupFailed(err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Database backup failed", err)
}

// MaintenanceRequired creates an error indicating database maintenance is required.
func MaintenanceRequired(reason string) *AppError {
	return NewStorageError(CodeExternalServiceUnavailable, "Database maintenance required").
		WithMetadata("reason", reason)
}

// CostTrackingFailed creates an error indicating cost tracking failed.
func CostTrackingFailed(operation string, err error) *AppError {
	return NewStorageInternalError(CodeInternal, "Cost tracking failed", err).
		WithMetadata("operation", operation)
}

// CostLimitExceeded creates an error indicating cost limit was exceeded.
func CostLimitExceeded(operation string, cost float64) *AppError {
	return NewStorageError(CodeQuotaExceeded, "Cost limit exceeded").
		WithMetadata("operation", operation).
		WithMetadata("cost", cost)
}
