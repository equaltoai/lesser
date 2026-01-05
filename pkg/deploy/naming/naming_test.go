package naming

import (
	"regexp"
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
}

func TestS3BucketName_IsSanitized(t *testing.T) {
	bucket := S3BucketName("my-app", StageDev, "Media Uploads", "123456789012", "us-east-1")
	require.NotEmpty(t, bucket)
	require.Equal(t, bucket, regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`).FindString(bucket))
}
