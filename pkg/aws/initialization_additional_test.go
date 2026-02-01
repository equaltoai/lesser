package awsinit

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestServiceConfigPresets_CoverAllPresets(t *testing.T) {
	t.Parallel()

	media := MediaServiceConfig()
	require.True(t, media.RequiresMediaConvert)
	require.True(t, media.RequiresSQS)
	require.Equal(t, "media", media.ServiceName)

	fed := FederationServiceConfig()
	require.True(t, fed.RequiresSQS)
	require.Equal(t, "federation", fed.ServiceName)

	ai := AIServiceConfig()
	require.True(t, ai.RequiresComprehend)
	require.True(t, ai.RequiresS3)
	require.Equal(t, "ai", ai.ServiceName)
}

func TestInitializeWithConfig_InitializesRequestedClients(t *testing.T) {
	t.Parallel()

	cfg := aws.Config{
		Region:      "us-east-1",
		Credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("id", "secret", "")),
	}

	services := InitializeWithConfig(cfg, ServiceConfig{
		ServiceName:            "all",
		RequiresS3:             true,
		RequiresSQS:            true,
		RequiresCloudWatch:     true,
		RequiresSecretsManager: true,
		RequiresComprehend:     true,
		RequiresMediaConvert:   true,
		RequiresSNS:            true,
		RequiresLambda:         true,
	}, zap.NewNop())

	require.NotNil(t, services.S3)
	require.NotNil(t, services.SQS)
	require.NotNil(t, services.CloudWatch)
	require.NotNil(t, services.SecretsManager)
	require.NotNil(t, services.Comprehend)
	require.NotNil(t, services.MediaConvert)
	require.NotNil(t, services.SNS)
	require.NotNil(t, services.Lambda)
}

func TestInitializeServices_SuccessAndMustInitializeServices(t *testing.T) {
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_ACCESS_KEY_ID", "id")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	logger := zap.NewNop()

	services, err := InitializeServices(context.Background(), ServiceConfig{
		ServiceName:            "test",
		Region:                 "us-east-1",
		RequestTimeout:         0,
		RetryMaxAttempts:       0,
		RequiresS3:             true,
		RequiresSQS:            true,
		RequiresCloudWatch:     true,
		RequiresSecretsManager: true,
		RequiresComprehend:     true,
		RequiresMediaConvert:   true,
		RequiresSNS:            true,
		RequiresLambda:         true,
	}, logger)
	require.NoError(t, err)
	require.NotNil(t, services)
	require.NotNil(t, services.S3)
	require.NotNil(t, services.SQS)
	require.NotNil(t, services.CloudWatch)
	require.NotNil(t, services.SecretsManager)
	require.NotNil(t, services.Comprehend)
	require.NotNil(t, services.MediaConvert)
	require.NotNil(t, services.SNS)
	require.NotNil(t, services.Lambda)

	services = MustInitializeServices(context.Background(), ServiceConfig{
		ServiceName: "test",
		Region:      "us-east-1",
	}, logger)
	require.NotNil(t, services)
}
