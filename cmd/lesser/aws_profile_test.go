package main

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/stretchr/testify/require"
)

func TestLoadAWSConfigFromProfile_RequiresProfile(t *testing.T) {
	_, err := loadAWSConfigFromProfile(context.Background(), "   ")
	require.Error(t, err)
}

func TestAWSCLIConfigureGet_SuccessAndErrorFormatting(t *testing.T) {
	prev := execCommandCombinedOutputFn
	t.Cleanup(func() { execCommandCombinedOutputFn = prev })

	t.Run("success", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, name string, args ...string) ([]byte, error) {
			require.Equal(t, "aws", name)
			require.Equal(t, []string{"configure", "get", "region", "--profile", "p"}, args)
			return []byte("us-east-1\n"), nil
		}

		region, err := awsCLIConfigureGet(context.Background(), "p", "region")
		require.NoError(t, err)
		require.Equal(t, "us-east-1", region)
	})

	t.Run("error includes output", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("bad output"), errors.New("exit 1")
		}

		_, err := awsCLIConfigureGet(context.Background(), "p", "region")
		require.Error(t, err)
		require.Contains(t, err.Error(), "aws configure get region --profile p")
		require.Contains(t, err.Error(), "bad output")
	})
}

func TestAWSCLIExportCredentials_ParsesAndValidates(t *testing.T) {
	prev := execCommandCombinedOutputFn
	t.Cleanup(func() { execCommandCombinedOutputFn = prev })

	t.Run("bad json", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("not json"), nil
		}

		_, err := awsCLIExportCredentials(context.Background(), "p")
		require.Error(t, err)
		require.Contains(t, err.Error(), "parse aws export-credentials output")
	})

	t.Run("empty keys", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"Version":1,"AccessKeyId":"","SecretAccessKey":"","SessionToken":""}`), nil
		}

		_, err := awsCLIExportCredentials(context.Background(), "p")
		require.Error(t, err)
		require.Contains(t, err.Error(), "returned empty keys")
	})

	t.Run("success", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte(`{"Version":1,"AccessKeyId":"AKIA","SecretAccessKey":"SECRET","SessionToken":"TOKEN"}`), nil
		}

		creds, err := awsCLIExportCredentials(context.Background(), "p")
		require.NoError(t, err)
		require.Equal(t, "AKIA", creds.AccessKeyID)
		require.Equal(t, "SECRET", creds.SecretAccessKey)
	})
}

func TestLoadAWSConfigFromProfile_WiresAWSCLIAndLoadDefaultConfig(t *testing.T) {
	prevExec := execCommandCombinedOutputFn
	prevLoad := awsLoadDefaultConfigFn
	t.Cleanup(func() {
		execCommandCombinedOutputFn = prevExec
		awsLoadDefaultConfigFn = prevLoad
	})

	t.Run("missing region", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			require.Equal(t, []string{"configure", "get", "region", "--profile", "p"}, args)
			return []byte(" \n"), nil
		}

		_, err := loadAWSConfigFromProfile(context.Background(), "p")
		require.Error(t, err)
		require.Contains(t, err.Error(), "has no default region configured")
	})

	t.Run("load default config error", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			switch {
			case len(args) >= 3 && args[0] == "configure" && args[1] == "get" && args[2] == "region":
				return []byte("us-east-1\n"), nil
			case len(args) >= 2 && args[0] == "configure" && args[1] == "export-credentials":
				return []byte(`{"Version":1,"AccessKeyId":"AKIA","SecretAccessKey":"SECRET","SessionToken":"TOKEN"}`), nil
			default:
				return nil, errors.New("unexpected aws cli args")
			}
		}

		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("load failed")
		}

		_, err := loadAWSConfigFromProfile(context.Background(), "p")
		require.Error(t, err)
		require.Contains(t, err.Error(), "load AWS config")
	})

	t.Run("success", func(t *testing.T) {
		execCommandCombinedOutputFn = func(_ context.Context, _ string, args ...string) ([]byte, error) {
			switch {
			case len(args) >= 3 && args[0] == "configure" && args[1] == "get" && args[2] == "region":
				return []byte("us-east-1\n"), nil
			case len(args) >= 2 && args[0] == "configure" && args[1] == "export-credentials":
				return []byte(`{"Version":1,"AccessKeyId":"AKIA","SecretAccessKey":"SECRET","SessionToken":"TOKEN"}`), nil
			default:
				return nil, errors.New("unexpected aws cli args")
			}
		}

		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "us-east-1"}, nil
		}

		cfg, err := loadAWSConfigFromProfile(context.Background(), "p")
		require.NoError(t, err)
		require.Equal(t, "us-east-1", cfg.Region)
	})
}

func TestLoadAWSConfigForCLI_ChoosesProfileOrAmbient(t *testing.T) {
	prevProfile := loadAWSConfigFromProfileFn
	prevLoad := awsLoadDefaultConfigFn
	t.Cleanup(func() {
		loadAWSConfigFromProfileFn = prevProfile
		awsLoadDefaultConfigFn = prevLoad
	})

	t.Run("uses explicit profile", func(t *testing.T) {
		loadAWSConfigFromProfileFn = func(_ context.Context, profile string) (aws.Config, error) {
			require.Equal(t, "explicit", profile)
			return aws.Config{Region: "us-west-2"}, nil
		}
		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("should not call ambient")
		}

		cfg, profile, err := loadAWSConfigForCLI(context.Background(), "explicit")
		require.NoError(t, err)
		require.Equal(t, "explicit", profile)
		require.Equal(t, "us-west-2", cfg.Region)
	})

	t.Run("uses env profile when explicit missing", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "from-env")
		loadAWSConfigFromProfileFn = func(_ context.Context, profile string) (aws.Config, error) {
			require.Equal(t, "from-env", profile)
			return aws.Config{Region: "us-east-2"}, nil
		}
		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{}, errors.New("should not call ambient")
		}

		cfg, profile, err := loadAWSConfigForCLI(context.Background(), "")
		require.NoError(t, err)
		require.Equal(t, "from-env", profile)
		require.Equal(t, "us-east-2", cfg.Region)
	})

	t.Run("uses ambient when no profile", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "")
		loadAWSConfigFromProfileFn = func(_ context.Context, _ string) (aws.Config, error) {
			return aws.Config{}, errors.New("should not call profile")
		}
		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: "us-east-1"}, nil
		}

		cfg, profile, err := loadAWSConfigForCLI(context.Background(), "")
		require.NoError(t, err)
		require.Empty(t, profile)
		require.Equal(t, "us-east-1", cfg.Region)
	})

	t.Run("ambient requires region", func(t *testing.T) {
		t.Setenv("AWS_PROFILE", "")
		loadAWSConfigFromProfileFn = func(_ context.Context, _ string) (aws.Config, error) {
			return aws.Config{}, errors.New("should not call profile")
		}
		awsLoadDefaultConfigFn = func(_ context.Context, _ ...func(*awsconfig.LoadOptions) error) (aws.Config, error) {
			return aws.Config{Region: ""}, nil
		}

		_, _, err := loadAWSConfigForCLI(context.Background(), "")
		require.Error(t, err)
		require.Contains(t, err.Error(), "AWS region is required")
	})
}
