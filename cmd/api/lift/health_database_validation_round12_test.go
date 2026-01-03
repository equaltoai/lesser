package lift

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHealthChecker_checkDatabase_FallbackValidationPaths(t *testing.T) {
	checker := NewHealthChecker(zap.NewNop(), nil)
	ctx := context.Background()

	t.Run("missing required env vars marks database unhealthy", func(t *testing.T) {
		t.Setenv("DOMAIN_NAME", "")
		t.Setenv("AWS_REGION", "")
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("PRIVATE_KEY_SECRET", "")
		t.Setenv("JWT_SECRET", "")
		t.Setenv("JWT_SECRET_ARN", "")

		checks := make(map[string]CheckResult)
		checker.checkDatabase(ctx, checks)

		require.Contains(t, checks, "database")
		require.Equal(t, HealthStatusUnhealthy, checks["database"].Status)
		require.Equal(t, "Database configuration validation failed", checks["database"].Message)
		require.NotEmpty(t, checks["database"].Error)
	})

	t.Run("present env vars but nil repos marks database degraded", func(t *testing.T) {
		t.Setenv("DOMAIN_NAME", "example.com")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("DYNAMODB_TABLE", "test-table")
		t.Setenv("PRIVATE_KEY_SECRET", "secret")
		t.Setenv("JWT_SECRET", "jwt-secret")
		t.Setenv("JWT_SECRET_ARN", "")

		checks := make(map[string]CheckResult)
		checker.checkDatabase(ctx, checks)

		require.Contains(t, checks, "database")
		require.Equal(t, HealthStatusDegraded, checks["database"].Status)
		require.Equal(t, "Database configuration valid but repository interface not available", checks["database"].Message)
		require.Empty(t, checks["database"].Error)
	})
}

func TestHealthChecker_checkS3Storage_BucketMissingAndAWSConfigError(t *testing.T) {
	checker := NewHealthChecker(zap.NewNop(), nil)

	t.Run("no bucket configured is degraded", func(t *testing.T) {
		t.Setenv("S3_BUCKET_NAME", "")
		t.Setenv("S3_BUCKET", "")
		t.Setenv("S3_MEDIA_BUCKET", "")
		t.Setenv("MEDIA_BUCKET_NAME", "")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusDegraded, checks["s3_storage"].Status)
	})

	t.Run("aws config load error is unhealthy", func(t *testing.T) {
		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("S3_BUCKET", "")
		t.Setenv("S3_MEDIA_BUCKET", "")
		t.Setenv("MEDIA_BUCKET_NAME", "")

		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_PROFILE", "missing-profile")
		t.Setenv("AWS_SDK_LOAD_CONFIG", "1")
		t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/missing-config")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusUnhealthy, checks["s3_storage"].Status)
		require.NotEmpty(t, checks["s3_storage"].Error)
	})
}
