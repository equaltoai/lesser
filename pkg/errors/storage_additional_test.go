package errors

import (
	stdErrors "errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageConstructors_AdditionalCoverage(t *testing.T) {
	underlying := stdErrors.New("boom")

	tests := []struct {
		name           string
		create         func() *AppError
		wantCode       ErrorCode
		wantRetryable  bool
		wantMetaKeys   []string
		expectUnwrapIs bool
	}{
		{
			name:           "DatabaseConnectionFailed",
			create:         func() *AppError { return DatabaseConnectionFailed(underlying) },
			wantCode:       CodeDatabaseConnection,
			wantRetryable:  true,
			expectUnwrapIs: true,
		},
		{
			name:           "DatabaseUnavailable",
			create:         func() *AppError { return DatabaseUnavailable(underlying) },
			wantCode:       CodeExternalServiceUnavailable,
			wantRetryable:  true,
			expectUnwrapIs: true,
		},
		{
			name:           "QueryFailed",
			create:         func() *AppError { return QueryFailed("gsi1", underlying) },
			wantCode:       CodeQueryFailed,
			wantMetaKeys:   []string{"query_type"},
			expectUnwrapIs: true,
		},
		{
			name:         "QueryInvalid",
			create:       func() *AppError { return QueryInvalid("scan", "bad filter") },
			wantCode:     CodeBadRequest,
			wantMetaKeys: []string{"query_type", "reason"},
		},
		{
			name:           "TransactionFailed",
			create:         func() *AppError { return TransactionFailed(underlying) },
			wantCode:       CodeTransactionFailed,
			wantRetryable:  true,
			expectUnwrapIs: true,
		},
		{
			name:          "TransactionConflict",
			create:        func() *AppError { return TransactionConflict("object#1") },
			wantCode:      CodeConcurrencyError,
			wantRetryable: true,
			wantMetaKeys:  []string{"resource"},
		},
		{
			name:           "IndexError",
			create:         func() *AppError { return IndexError("gsi1", underlying) },
			wantCode:       CodeIndexError,
			wantMetaKeys:   []string{"index"},
			expectUnwrapIs: true,
		},
		{
			name:           "ConstraintViolated",
			create:         func() *AppError { return ConstraintViolated("constraint", underlying) },
			wantCode:       CodeConstraintViolated,
			wantMetaKeys:   []string{"constraint"},
			expectUnwrapIs: true,
		},
		{
			name:          "CreateFailed",
			create:        func() *AppError { return CreateFailed("user", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type"},
		},
		{
			name:          "UpdateFailed",
			create:        func() *AppError { return UpdateFailed("user", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type"},
		},
		{
			name:          "DeleteFailed",
			create:        func() *AppError { return DeleteFailed("user", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type"},
		},
		{
			name:          "GetFailed",
			create:        func() *AppError { return GetFailed("user", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type"},
		},
		{
			name:          "ListFailed",
			create:        func() *AppError { return ListFailed("user", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type"},
		},
		{
			name:          "QueryByFieldFailed",
			create:        func() *AppError { return QueryByFieldFailed("user", "username", underlying) },
			wantCode:      CodeQueryFailed,
			wantRetryable: true,
			wantMetaKeys:  []string{"item_type", "field"},
		},
		{
			name:          "BatchOperationFailed",
			create:        func() *AppError { return BatchOperationFailed("write", underlying) },
			wantCode:      CodeInternal,
			wantRetryable: true,
			wantMetaKeys:  []string{"operation"},
		},
		{
			name:         "BatchPartialFailure",
			create:       func() *AppError { return BatchPartialFailure(1, 2) },
			wantCode:     CodeInternal,
			wantMetaKeys: []string{"success_count", "failure_count"},
		},
		{
			name:         "InvalidInput",
			create:       func() *AppError { return InvalidInput("field", "bad") },
			wantCode:     CodeInvalidInput,
			wantMetaKeys: []string{"field", "reason"},
		},
		{
			name:         "StorageRequiredFieldMissing",
			create:       func() *AppError { return StorageRequiredFieldMissing("field") },
			wantCode:     CodeRequiredFieldMissing,
			wantMetaKeys: []string{"field"},
		},
		{
			name:         "StorageFieldTooLong",
			create:       func() *AppError { return StorageFieldTooLong("field", 10) },
			wantCode:     CodeFieldTooLong,
			wantMetaKeys: []string{"field", "max_length"},
		},
		{
			name:         "StorageFieldTooShort",
			create:       func() *AppError { return StorageFieldTooShort("field", 2) },
			wantCode:     CodeFieldTooShort,
			wantMetaKeys: []string{"field", "min_length"},
		},
		{
			name:         "DataIntegrityViolated",
			create:       func() *AppError { return DataIntegrityViolated("bad data") },
			wantCode:     CodeConstraintViolated,
			wantMetaKeys: []string{"reason"},
		},
		{
			name:         "StorageQuotaExceeded",
			create:       func() *AppError { return StorageQuotaExceeded("u1", 5) },
			wantCode:     CodeStorageQuotaExceeded,
			wantMetaKeys: []string{"user_id", "quota"},
		},
		{
			name:         "FileSizeExceeded",
			create:       func() *AppError { return FileSizeExceeded(10, 5) },
			wantCode:     CodeContentTooLarge,
			wantMetaKeys: []string{"size", "max_size"},
		},
		{
			name:         "TooManyItems",
			create:       func() *AppError { return TooManyItems(10, 5) },
			wantCode:     CodeQuotaExceeded,
			wantMetaKeys: []string{"count", "max_count"},
		},
		{
			name:         "MaintenanceRequired",
			create:       func() *AppError { return MaintenanceRequired("planned") },
			wantCode:     CodeExternalServiceUnavailable,
			wantMetaKeys: []string{"reason"},
		},
		{
			name:         "MigrationFailed",
			create:       func() *AppError { return MigrationFailed("v1", underlying) },
			wantCode:     CodeInternal,
			wantMetaKeys: []string{"version"},
		},
		{
			name:     "StorageUserNotFound",
			create:   func() *AppError { return StorageUserNotFound("alice") },
			wantCode: CodeNotFound,
			wantMetaKeys: []string{
				"item_type",
				"id",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.create()
			require.NotNil(t, err)
			assert.Equal(t, CategoryStorage, err.Category)
			assert.Equal(t, tc.wantCode, err.Code)
			for _, key := range tc.wantMetaKeys {
				assert.Contains(t, err.Metadata, key)
			}
			assert.Equal(t, tc.wantRetryable, err.Retryable)
			if tc.expectUnwrapIs {
				assert.ErrorIs(t, err, underlying)
			}
		})
	}
}
