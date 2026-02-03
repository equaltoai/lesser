package naming

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeAppName(t *testing.T) {
	got, err := NormalizeAppName(" My-App ")
	require.NoError(t, err)
	require.Equal(t, "my-app", got)

	_, err = NormalizeAppName("")
	require.Error(t, err)

	_, err = NormalizeAppName("bad_app")
	require.Error(t, err)
}

func TestStageForEnvironment(t *testing.T) {
	require.Equal(t, StageDev, StageForEnvironment(""))
	require.Equal(t, StageDev, StageForEnvironment("development"))
	require.Equal(t, StageStaging, StageForEnvironment("test"))
	require.Equal(t, StageLive, StageForEnvironment("production"))
	require.Equal(t, Stage("qa"), StageForEnvironment("qa"))
}

func TestDomains(t *testing.T) {
	require.Equal(t, "example.com", StageDomain(StageLive, "Example.com."))
	require.Equal(t, "dev.example.com", StageDomain(StageDev, "Example.com"))
	require.Equal(t, "api.dev.example.com", ServiceDomain("api", StageDev, "example.com"))
	require.Equal(t, "", ServiceDomain("", StageDev, "example.com"))
	require.Equal(t, "", StageDomain(StageDev, ""))
}

func TestS3BucketName_IsSanitized(t *testing.T) {
	bucket := S3BucketName("my-app", StageDev, "Media Uploads", "123456789012", "us-east-1")
	require.NotEmpty(t, bucket)
	require.Equal(t, bucket, regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).FindString(bucket))
}

func TestNameHelpers(t *testing.T) {
	require.Equal(t, DefaultAppName, appOrDefault(""))
	require.Equal(t, "my-app", appOrDefault("My-App"))
	require.Panics(t, func() { _ = appOrDefault("bad_app") })

	require.True(t, IsLiveEnvironment("prod"))
	require.False(t, IsLiveEnvironment("staging"))

	require.True(t, IsLiveStage(StageLive))
	require.False(t, IsLiveStage(StageStaging))

	require.Equal(t, "my-app-shared", SharedStackName("my-app"))
	require.Equal(t, "my-app-dev", StageStackName("my-app", StageDev))
	require.Equal(t, "my-app-shared-logs", SharedResourceName("my-app", "logs"))
	require.Equal(t, "my-app-live-api", StageResourceName("my-app", StageLive, "api"))
	require.Equal(t, "lesser-live-api", ResourceNameWithApp("", "api", "production"))
	require.Equal(t, "lesser-live-api", ResourceName("api", "production"))
}

func TestSanitizeS3BucketName_Branches(t *testing.T) {
	require.Equal(t, "bucket", sanitizeS3BucketName(""))
	require.Equal(t, "a-bucket", sanitizeS3BucketName("A"))
	require.Equal(t, "bucket", sanitizeS3BucketName("!!!"))
	require.Equal(t, "a-b", sanitizeS3BucketName("!!!A@@@B"))

	long := strings.Repeat("a", 80)
	sanitized := sanitizeS3BucketName(long)
	require.Len(t, sanitized, 63)
	require.Equal(t, sanitized, regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`).FindString(sanitized))
}
