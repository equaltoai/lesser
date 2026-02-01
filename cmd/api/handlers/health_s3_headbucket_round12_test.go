package handlers

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHealthChecker_checkS3Storage_HeadBucketBranches_Round12(t *testing.T) {
	checker := NewHealthChecker(zap.NewNop(), nil)

	t.Run("head bucket success marks storage healthy", func(t *testing.T) {
		checker.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}

		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_MAX_ATTEMPTS", "1")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusHealthy, checks["s3_storage"].Status)
	})

	t.Run("head bucket error marks storage unhealthy", func(t *testing.T) {
		checker.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Header:     make(http.Header),
				Body:       io.NopCloser(bytes.NewReader(nil)),
				Request:    req,
			}, nil
		})}

		t.Setenv("S3_BUCKET_NAME", "bucket")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("AWS_ACCESS_KEY_ID", "test")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
		t.Setenv("AWS_MAX_ATTEMPTS", "1")

		checks := make(map[string]CheckResult)
		checker.checkS3Storage(context.Background(), checks)

		require.Equal(t, HealthStatusUnhealthy, checks["s3_storage"].Status)
		require.NotEmpty(t, checks["s3_storage"].Error)
	})
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
