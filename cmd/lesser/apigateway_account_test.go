package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	apigwtypes "github.com/aws/aws-sdk-go-v2/service/apigateway/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/require"
)

func TestAPIGatewayClientFactories(t *testing.T) {
	require.NotNil(t, newAPIGatewayClientFn(aws.Config{}))
	require.NotNil(t, newIAMClientFn(aws.Config{}))
}

type stubAPIGatewayAccountClient struct {
	getAccountFn func(context.Context, *apigateway.GetAccountInput, ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error)
	updateFn     func(context.Context, *apigateway.UpdateAccountInput, ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error)
}

func (s stubAPIGatewayAccountClient) GetAccount(ctx context.Context, in *apigateway.GetAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
	return s.getAccountFn(ctx, in, optFns...)
}

func (s stubAPIGatewayAccountClient) UpdateAccount(ctx context.Context, in *apigateway.UpdateAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error) {
	if s.updateFn == nil {
		panic("unexpected UpdateAccount call")
	}
	return s.updateFn(ctx, in, optFns...)
}

type stubIAMRoleClient struct {
	getRoleFn          func(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error)
	createRoleFn       func(context.Context, *iam.CreateRoleInput, ...func(*iam.Options)) (*iam.CreateRoleOutput, error)
	attachRolePolicyFn func(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error)
}

func (s stubIAMRoleClient) GetRole(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	if s.getRoleFn == nil {
		panic("unexpected GetRole call")
	}
	return s.getRoleFn(ctx, in, optFns...)
}
func (s stubIAMRoleClient) CreateRole(ctx context.Context, in *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	if s.createRoleFn == nil {
		panic("unexpected CreateRole call")
	}
	return s.createRoleFn(ctx, in, optFns...)
}
func (s stubIAMRoleClient) AttachRolePolicy(ctx context.Context, in *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	if s.attachRolePolicyFn == nil {
		panic("unexpected AttachRolePolicy call")
	}
	return s.attachRolePolicyFn(ctx, in, optFns...)
}

func TestEnsureAPIGatewayCloudWatchLogsRole_ExistingARN(t *testing.T) {
	originalAPIGateway := newAPIGatewayClientFn
	originalIAM := newIAMClientFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = originalAPIGateway
		newIAMClientFn = originalIAM
	})

	newAPIGatewayClientFn = func(cfg aws.Config) apiGatewayAccountAPI {
		return stubAPIGatewayAccountClient{
			getAccountFn: func(context.Context, *apigateway.GetAccountInput, ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
				return &apigateway.GetAccountOutput{
					CloudwatchRoleArn: aws.String("arn:aws:iam::123456789012:role/existing"),
				}, nil
			},
		}
	}
	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{}
	}

	require.NoError(t, ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{}))
}

func TestEnsureAPIGatewayCloudWatchLogsRole_GetAccountError(t *testing.T) {
	originalAPIGateway := newAPIGatewayClientFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = originalAPIGateway
	})

	newAPIGatewayClientFn = func(cfg aws.Config) apiGatewayAccountAPI {
		return stubAPIGatewayAccountClient{
			getAccountFn: func(context.Context, *apigateway.GetAccountInput, ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
				return nil, errors.New("boom")
			},
		}
	}

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "apigateway:GetAccount")
}

func TestEnsureAPIGatewayCloudWatchLogsRole_SetsCloudWatchRoleARN(t *testing.T) {
	originalAPIGateway := newAPIGatewayClientFn
	originalIAM := newIAMClientFn
	originalSleepFn := sleepFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = originalAPIGateway
		newIAMClientFn = originalIAM
		sleepFn = originalSleepFn
	})

	var patchOps []apigwtypes.PatchOperation
	var updateCalls int
	newAPIGatewayClientFn = func(cfg aws.Config) apiGatewayAccountAPI {
		return stubAPIGatewayAccountClient{
			getAccountFn: func(context.Context, *apigateway.GetAccountInput, ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
				return &apigateway.GetAccountOutput{}, nil
			},
			updateFn: func(ctx context.Context, in *apigateway.UpdateAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error) {
				updateCalls++
				patchOps = append([]apigwtypes.PatchOperation(nil), in.PatchOperations...)
				return &apigateway.UpdateAccountOutput{}, nil
			},
		}
	}

	var attachedPolicy string
	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(context.Context, *iam.GetRoleInput, ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return &iam.GetRoleOutput{
					Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123456789012:role/existing")},
				}, nil
			},
			attachRolePolicyFn: func(ctx context.Context, in *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
				attachedPolicy = aws.ToString(in.PolicyArn)
				return &iam.AttachRolePolicyOutput{}, nil
			},
		}
	}

	sleepFn = func(duration time.Duration) {}

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.NoError(t, err)
	require.Equal(t, 1, updateCalls)
	require.Len(t, patchOps, 1)
	require.Equal(t, apigwtypes.OpReplace, patchOps[0].Op)
	require.Equal(t, "/cloudwatchRoleArn", aws.ToString(patchOps[0].Path))
	require.Equal(t, "arn:aws:iam::123456789012:role/existing", aws.ToString(patchOps[0].Value))
	require.Equal(t, "arn:aws:iam::aws:policy/service-role/AmazonAPIGatewayPushToCloudWatchLogs", attachedPolicy)
}

func TestEnsureAPIGatewayCloudWatchLogsRole_UpdateAccountRetriesExhausted(t *testing.T) {
	originalAPIGateway := newAPIGatewayClientFn
	originalIAM := newIAMClientFn
	originalSleepFn := sleepFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = originalAPIGateway
		newIAMClientFn = originalIAM
		sleepFn = originalSleepFn
	})

	var updateCalls int
	var slept []time.Duration
	newAPIGatewayClientFn = func(cfg aws.Config) apiGatewayAccountAPI {
		return stubAPIGatewayAccountClient{
			getAccountFn: func(context.Context, *apigateway.GetAccountInput, ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
				return &apigateway.GetAccountOutput{}, nil
			},
			updateFn: func(ctx context.Context, in *apigateway.UpdateAccountInput, optFns ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error) {
				updateCalls++
				return nil, errors.New("still denied")
			},
		}
	}
	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return &iam.GetRoleOutput{
					Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123456789012:role/existing")},
				}, nil
			},
			attachRolePolicyFn: func(ctx context.Context, in *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
				return &iam.AttachRolePolicyOutput{}, nil
			},
		}
	}
	sleepFn = func(duration time.Duration) {
		slept = append(slept, duration)
	}

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "apigateway:UpdateAccount")
	require.Equal(t, 6, updateCalls)
	require.Equal(t, []time.Duration{2 * time.Second, 4 * time.Second, 6 * time.Second, 8 * time.Second, 10 * time.Second, 12 * time.Second}, slept)
}

func TestEnsureIAMRoleForAPIGatewayLogs_CreatesRoleWhenMissing(t *testing.T) {
	originalIAM := newIAMClientFn
	t.Cleanup(func() {
		newIAMClientFn = originalIAM
	})

	var createdAssumeRolePolicy string
	var attachedRoleName string
	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return nil, &iamtypes.NoSuchEntityException{}
			},
			createRoleFn: func(ctx context.Context, in *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
				createdAssumeRolePolicy = aws.ToString(in.AssumeRolePolicyDocument)
				return &iam.CreateRoleOutput{
					Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123456789012:role/created")},
				}, nil
			},
			attachRolePolicyFn: func(ctx context.Context, in *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
				attachedRoleName = aws.ToString(in.RoleName)
				return &iam.AttachRolePolicyOutput{}, nil
			},
		}
	}

	roleARN, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{
		Region: "us-east-1",
	})
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123456789012:role/created", roleARN)
	require.Equal(t, apiGatewayLogsRoleName, attachedRoleName)
	require.Contains(t, createdAssumeRolePolicy, "apigateway.amazonaws.com")
	require.Contains(t, createdAssumeRolePolicy, "sts:AssumeRole")
}

func TestEnsureIAMRoleForAPIGatewayLogs_GetRoleUnexpectedError(t *testing.T) {
	originalIAM := newIAMClientFn
	t.Cleanup(func() {
		newIAMClientFn = originalIAM
	})

	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return nil, errors.New("boom")
			},
		}
	}

	_, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), `iam:GetRole "lesser-apigateway-cloudwatch-logs"`)
}

func TestEnsureIAMRoleForAPIGatewayLogs_ExistingRoleAttachPolicyError(t *testing.T) {
	originalIAM := newIAMClientFn
	t.Cleanup(func() {
		newIAMClientFn = originalIAM
	})

	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return &iam.GetRoleOutput{
					Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123456789012:role/existing")},
				}, nil
			},
			attachRolePolicyFn: func(ctx context.Context, in *iam.AttachRolePolicyInput, optFns ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
				return nil, errors.New("attach failed")
			},
		}
	}

	_, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "attach failed")
}

func TestEnsureIAMRoleForAPIGatewayLogs_CreateRoleError(t *testing.T) {
	originalIAM := newIAMClientFn
	t.Cleanup(func() {
		newIAMClientFn = originalIAM
	})

	newIAMClientFn = func(cfg aws.Config) iamRoleAPI {
		return stubIAMRoleClient{
			getRoleFn: func(ctx context.Context, in *iam.GetRoleInput, optFns ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
				return nil, &iamtypes.NoSuchEntityException{}
			},
			createRoleFn: func(ctx context.Context, in *iam.CreateRoleInput, optFns ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
				return nil, errors.New("create failed")
			},
		}
	}

	_, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "create failed")
}

func TestEnsureRoleHasAPIGatewayLogsPolicy_Error(t *testing.T) {
	client := stubIAMRoleClient{
		attachRolePolicyFn: func(context.Context, *iam.AttachRolePolicyInput, ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
			return nil, errors.New("denied")
		},
	}

	err := ensureRoleHasAPIGatewayLogsPolicy(context.Background(), client)
	require.Error(t, err)
	require.True(t, strings.Contains(err.Error(), "iam:AttachRolePolicy") && strings.Contains(err.Error(), "denied"))
}
