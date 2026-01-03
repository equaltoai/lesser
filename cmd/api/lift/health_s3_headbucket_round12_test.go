package lift

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHealthChecker_checkS3Storage_HeadBucketBranches_Round12(t *testing.T) {
	checker := NewHealthChecker(zap.NewNop(), nil)

	t.Run("head bucket success marks storage healthy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(server.Close)

		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
		t.Setenv("AWS_MAX_ATTEMPTS", "1")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusHealthy, checks["s3_storage"].Status)
	})

	t.Run("head bucket error marks storage unhealthy", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		t.Cleanup(server.Close)

		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_ENDPOINT_URL_S3", server.URL)
		t.Setenv("AWS_MAX_ATTEMPTS", "1")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusUnhealthy, checks["s3_storage"].Status)
		require.NotEmpty(t, checks["s3_storage"].Error)
	})
}

