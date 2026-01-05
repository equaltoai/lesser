package config

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubSecretsManagerGetter struct {
	output *secretsmanager.GetSecretValueOutput
	err    error
}

func (s stubSecretsManagerGetter) GetSecretValue(_ context.Context, _ *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	return s.output, s.err
}

func TestConfig_URLHelpers(t *testing.T) {
	cfg := &Config{Domain: "example.com"}
	assert.Equal(t, "https://example.com", cfg.BaseURL())
	assert.Equal(t, "https://example.com/users/alice", cfg.ActorURL("alice"))
	assert.Equal(t, "https://example.com/objects/123", cfg.ObjectURL("objects", "123"))

	cfg.Domain = "localhost"
	assert.Equal(t, "http://localhost", cfg.BaseURL())
}

func TestConfig_ModeAndCMSHelpers(t *testing.T) {
	assert.Equal(t, InstanceModeHybrid, (*Config)(nil).EffectiveInstanceMode())

	cfg := &Config{InstanceMode: InstanceModeSocial}
	assert.False(t, cfg.CMSEnabled())

	cfg = &Config{
		InstanceMode:                  InstanceModeHybrid,
		CMSLongFormPublishingEnabled:  true,
		CMSDraftSystemEnabled:         true,
		CMSRevisionHistoryEnabled:     true,
		CMSScheduledPublishingEnabled: true,
		CMSSeriesEnabled:              true,
		CMSCategoriesEnabled:          true,
	}
	assert.True(t, cfg.CMSEnabled())
	assert.True(t, cfg.CMSLongFormEnabled())
	assert.True(t, cfg.CMSDraftsEnabled())
	assert.True(t, cfg.CMSRevisionsEnabled())
	assert.True(t, cfg.CMSSchedulingEnabled())
	assert.True(t, cfg.CMSSeriesAllowed())
	assert.True(t, cfg.CMSCategoriesAllowed())
}

func TestResetForTests(t *testing.T) {
	SetupTestEnvironment(t)
	c1 := Get()
	require.NotNil(t, c1)

	ResetForTests()
	c2 := Get()
	require.NotNil(t, c2)
	assert.NotSame(t, c1, c2)
}

func TestEnvParsingHelpers(t *testing.T) {
	t.Setenv("INT_KEY", "not-an-int")
	assert.Equal(t, 5, getEnvAsIntOrDefault("INT_KEY", 5))
	t.Setenv("INT_KEY", "42")
	assert.Equal(t, 42, getEnvAsIntOrDefault("INT_KEY", 5))

	t.Setenv("I64_KEY", "not-an-int")
	assert.Equal(t, int64(7), getEnvAsInt64OrDefault("I64_KEY", 7))
	t.Setenv("I64_KEY", "99")
	assert.Equal(t, int64(99), getEnvAsInt64OrDefault("I64_KEY", 7))

	t.Setenv("BOOL_KEY", "yes")
	assert.True(t, getEnvAsBoolOrDefault("BOOL_KEY", false))
	t.Setenv("BOOL_KEY", "no")
	assert.False(t, getEnvAsBoolOrDefault("BOOL_KEY", true))

	t.Setenv("SLICE_KEY", "a, b, ,c")
	assert.Equal(t, []string{"a", "b", "c"}, getEnvAsStringSliceOrDefault("SLICE_KEY", []string{"x"}))
	t.Setenv("SLICE_KEY", " , ")
	assert.Equal(t, []string{"x"}, getEnvAsStringSliceOrDefault("SLICE_KEY", []string{"x"}))

	t.Setenv("DUR_KEY", "not-a-duration")
	assert.Equal(t, time.Hour, getEnvAsDurationOrDefault("DUR_KEY", time.Hour))
	t.Setenv("DUR_KEY", "2s")
	assert.Equal(t, 2*time.Second, getEnvAsDurationOrDefault("DUR_KEY", time.Hour))

	t.Setenv("U32_KEY", "not-a-number")
	assert.Equal(t, uint32(10), getEnvAsUint32OrDefault("U32_KEY", 10))
	t.Setenv("U32_KEY", "12")
	assert.Equal(t, uint32(12), getEnvAsUint32OrDefault("U32_KEY", 10))

	t.Setenv("U8_KEY", "not-a-number")
	assert.Equal(t, uint8(3), getEnvAsUint8OrDefault("U8_KEY", 3))
	t.Setenv("U8_KEY", "7")
	assert.Equal(t, uint8(7), getEnvAsUint8OrDefault("U8_KEY", 3))
}

func TestEnvironmentResolutionHelpers(t *testing.T) {
	assert.Equal(t, "x", firstNonEmpty(" ", "x", "y"))

	t.Setenv("ENVIRONMENT", "")
	t.Setenv("STAGE", "prod")
	env, envRaw, stage := resolveEnvironmentAndStage()
	assert.Equal(t, "prod", env)
	assert.Equal(t, "", envRaw)
	assert.Equal(t, "prod", stage)
}

func TestParseInstanceMode(t *testing.T) {
	assert.Equal(t, InstanceModeSocial, parseInstanceMode("social"))
	assert.Equal(t, InstanceModeCMS, parseInstanceMode("cms"))
	assert.Equal(t, InstanceModeHybrid, parseInstanceMode(""))
	assert.Equal(t, InstanceModeHybrid, parseInstanceMode("unknown"))
}

func TestIsRunningTests_CanBeForcedFalse(t *testing.T) {
	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })

	os.Args = []string{"app"}
	assert.False(t, isRunningTests())

	os.Args = []string{"app", "-test.v"}
	assert.True(t, isRunningTests())
}

func TestResolveDynamoTableName(t *testing.T) {
	t.Run("explicit env wins", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "explicit")
		assert.Equal(t, "explicit", resolveDynamoTableName())
	})

	t.Run("derived from stage", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "Prod")
		assert.Equal(t, "lesser-prod", resolveDynamoTableName())
	})

	t.Run("uses test fallback when running tests", func(t *testing.T) {
		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		assert.Equal(t, "test-table", resolveDynamoTableName())
	})

	t.Run("panics when missing and not running tests", func(t *testing.T) {
		origArgs := os.Args
		t.Cleanup(func() { os.Args = origArgs })
		os.Args = []string{"app"}

		t.Setenv("DYNAMODB_TABLE", "")
		t.Setenv("DYNAMO_TABLE_NAME", "")
		t.Setenv("ENVIRONMENT", "")
		t.Setenv("STAGE", "")

		require.Panics(t, func() { _ = resolveDynamoTableName() })
	})
}

func TestGetterHelpers(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")
	t.Setenv("STAGE", "")
	assert.Equal(t, "development", GetEnvironment())

	t.Setenv("ENVIRONMENT", "production")
	t.Setenv("STAGE", "")
	assert.Equal(t, "production", GetEnvironment())

	t.Setenv("S3_BUCKET_NAME", "bucket-1")
	assert.Equal(t, "bucket-1", GetS3Bucket())

	t.Setenv("PRIVATE_KEY_SECRET", "secret-name")
	t.Setenv("PRIVATE_KEY_SECRET_ARN", "arn:aws:secretsmanager:...")
	assert.Equal(t, "secret-name", GetPrivateKeySecret())

	t.Setenv("PRIVATE_KEY_SECRET", "")
	t.Setenv("PRIVATE_KEY_SECRET_ARN", "arn:aws:secretsmanager:...")
	assert.Equal(t, "arn:aws:secretsmanager:...", GetPrivateKeySecret())

	t.Setenv("DOMAIN_NAME", "example.com")
	assert.Equal(t, "example.com", GetDomainName())

	t.Setenv("DYNAMODB_TABLE", "main-table")
	assert.Equal(t, "main-table", GetDynamoTableName())
	assert.Equal(t, "main-table", GetMainTableName())

	t.Setenv("STREAM_EVENTS_TABLE_NAME", "stream-events")
	assert.Equal(t, "stream-events", GetStreamEventsTableName())
}

func TestMustGetJWTSecret_SecretsManagerPath(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
		ResetForTests()
	})

	os.Args = []string{"app"}
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:jwt")

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	secretString := `{"secret":"from-secrets-manager"}`
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{
			output: &secretsmanager.GetSecretValueOutput{SecretString: &secretString},
		}
	}

	assert.Equal(t, "from-secrets-manager", mustGetJWTSecret())
}

func TestMustGetJWTSecret_PlaintextAndTestDummy(t *testing.T) {
	ResetForTests()
	t.Cleanup(ResetForTests)

	t.Setenv("JWT_SECRET", "plaintext")
	assert.Equal(t, "plaintext", mustGetJWTSecret())

	ResetForTests()
	t.Setenv("JWT_SECRET", "")
	assert.Equal(t, "dummy", mustGetJWTSecret())
}

func TestMustGetJWTSecret_PanicsOnFetchError(t *testing.T) {
	ResetForTests()

	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	origArgs := os.Args
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
		os.Args = origArgs
		ResetForTests()
	})

	os.Args = []string{"app"}
	t.Setenv("JWT_SECRET", "")
	t.Setenv("JWT_SECRET_ARN", "arn:aws:secretsmanager:us-east-1:123:secret:jwt")

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{err: errors.New("boom")}
	}

	require.Panics(t, func() { _ = mustGetJWTSecret() })
}

func TestFetchSecretValue_ErrorPaths(t *testing.T) {
	origLoad := loadDefaultAWSConfig
	origNew := newSecretsManagerValueGetter
	t.Cleanup(func() {
		loadDefaultAWSConfig = origLoad
		newSecretsManagerValueGetter = origNew
	})

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{}, errors.New("no config")
	}
	_, err := fetchSecretValue("arn")
	require.Error(t, err)

	loadDefaultAWSConfig = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
		return aws.Config{Region: "us-east-1"}, nil
	}
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{output: &secretsmanager.GetSecretValueOutput{}}
	}
	_, err = fetchSecretValue("arn")
	require.Error(t, err)

	invalidJSON := "{"
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{output: &secretsmanager.GetSecretValueOutput{SecretString: &invalidJSON}}
	}
	_, err = fetchSecretValue("arn")
	require.Error(t, err)

	missingKey := `{"not_secret":"x"}`
	newSecretsManagerValueGetter = func(_ aws.Config) secretsManagerValueGetter {
		return stubSecretsManagerGetter{output: &secretsmanager.GetSecretValueOutput{SecretString: &missingKey}}
	}
	_, err = fetchSecretValue("arn")
	require.Error(t, err)
}
