package main

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/apigateway"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/stretchr/testify/require"
)

type fakeAPIGatewayClient struct {
	cloudwatchRoleArn *string
	getErr            error

	updateCalls int
	failUpdates int
	updateErr   error
}

func (f *fakeAPIGatewayClient) GetAccount(_ context.Context, _ *apigateway.GetAccountInput, _ ...func(*apigateway.Options)) (*apigateway.GetAccountOutput, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return &apigateway.GetAccountOutput{CloudwatchRoleArn: f.cloudwatchRoleArn}, nil
}

func (f *fakeAPIGatewayClient) UpdateAccount(_ context.Context, _ *apigateway.UpdateAccountInput, _ ...func(*apigateway.Options)) (*apigateway.UpdateAccountOutput, error) {
	f.updateCalls++
	if f.updateCalls <= f.failUpdates {
		if f.updateErr != nil {
			return nil, f.updateErr
		}
		return nil, stdErrors.New("update failed")
	}
	return &apigateway.UpdateAccountOutput{}, nil
}

type fakeIAMClient struct {
	getRoleOut    *iam.GetRoleOutput
	getRoleErr    error
	createRoleOut *iam.CreateRoleOutput
	createRoleErr error
	attachErr     error
}

func (f *fakeIAMClient) GetRole(_ context.Context, _ *iam.GetRoleInput, _ ...func(*iam.Options)) (*iam.GetRoleOutput, error) {
	if f.getRoleErr != nil {
		return nil, f.getRoleErr
	}
	return f.getRoleOut, nil
}

func (f *fakeIAMClient) CreateRole(_ context.Context, _ *iam.CreateRoleInput, _ ...func(*iam.Options)) (*iam.CreateRoleOutput, error) {
	if f.createRoleErr != nil {
		return nil, f.createRoleErr
	}
	return f.createRoleOut, nil
}

func (f *fakeIAMClient) AttachRolePolicy(_ context.Context, _ *iam.AttachRolePolicyInput, _ ...func(*iam.Options)) (*iam.AttachRolePolicyOutput, error) {
	if f.attachErr != nil {
		return nil, f.attachErr
	}
	return &iam.AttachRolePolicyOutput{}, nil
}

func TestEnsureAPIGatewayCloudWatchLogsRole_AlreadySet(t *testing.T) {
	prevNew := newAPIGatewayClientFn
	t.Cleanup(func() { newAPIGatewayClientFn = prevNew })

	fakeAPIGW := &fakeAPIGatewayClient{cloudwatchRoleArn: aws.String("arn:aws:iam::123:role/existing")}
	newAPIGatewayClientFn = func(_ aws.Config) apiGatewayAccountAPI { return fakeAPIGW }

	require.NoError(t, ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{}))
	require.Equal(t, 0, fakeAPIGW.updateCalls)
}

func TestEnsureAPIGatewayCloudWatchLogsRole_SetsRoleWithRetries(t *testing.T) {
	prevNewAPIGW := newAPIGatewayClientFn
	prevNewIAM := newIAMClientFn
	prevSleep := sleepFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = prevNewAPIGW
		newIAMClientFn = prevNewIAM
		sleepFn = prevSleep
	})

	sleepFn = func(time.Duration) {}

	fakeAPIGW := &fakeAPIGatewayClient{
		cloudwatchRoleArn: aws.String(""),
		failUpdates:       2,
	}
	newAPIGatewayClientFn = func(_ aws.Config) apiGatewayAccountAPI { return fakeAPIGW }

	fakeIAM := &fakeIAMClient{
		getRoleOut: &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123:role/apigw")}},
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM }

	require.NoError(t, ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{}))
	require.GreaterOrEqual(t, fakeAPIGW.updateCalls, 3)
}

func TestEnsureAPIGatewayCloudWatchLogsRole_GetAccountError(t *testing.T) {
	prevNew := newAPIGatewayClientFn
	t.Cleanup(func() { newAPIGatewayClientFn = prevNew })

	fakeAPIGW := &fakeAPIGatewayClient{getErr: errSentinel}
	newAPIGatewayClientFn = func(_ aws.Config) apiGatewayAccountAPI { return fakeAPIGW }

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "apigateway:GetAccount")
}

func TestEnsureAPIGatewayCloudWatchLogsRole_UpdateRetriesExhausted(t *testing.T) {
	prevNewAPIGW := newAPIGatewayClientFn
	prevNewIAM := newIAMClientFn
	prevSleep := sleepFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = prevNewAPIGW
		newIAMClientFn = prevNewIAM
		sleepFn = prevSleep
	})

	sleepFn = func(time.Duration) {}

	fakeAPIGW := &fakeAPIGatewayClient{
		cloudwatchRoleArn: aws.String(""),
		failUpdates:       10,
		updateErr:         errSentinel,
	}
	newAPIGatewayClientFn = func(_ aws.Config) apiGatewayAccountAPI { return fakeAPIGW }

	fakeIAM := &fakeIAMClient{
		getRoleOut: &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123:role/apigw")}},
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM }

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "apigateway:UpdateAccount")
	require.Equal(t, 6, fakeAPIGW.updateCalls)
}

func TestEnsureAPIGatewayCloudWatchLogsRole_PropagatesIAMRoleErrors(t *testing.T) {
	prevNewAPIGW := newAPIGatewayClientFn
	prevNewIAM := newIAMClientFn
	t.Cleanup(func() {
		newAPIGatewayClientFn = prevNewAPIGW
		newIAMClientFn = prevNewIAM
	})

	fakeAPIGW := &fakeAPIGatewayClient{cloudwatchRoleArn: aws.String("")}
	newAPIGatewayClientFn = func(_ aws.Config) apiGatewayAccountAPI { return fakeAPIGW }

	fakeIAM := &fakeIAMClient{getRoleErr: stdErrors.New("boom")}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM }

	err := ensureAPIGatewayCloudWatchLogsRole(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "iam:GetRole")
	require.Equal(t, 0, fakeAPIGW.updateCalls)
}

func TestEnsureIAMRoleForAPIGatewayLogs_CreateRoleFlowAndFailures(t *testing.T) {
	prevNewIAM := newIAMClientFn
	t.Cleanup(func() { newIAMClientFn = prevNewIAM })

	// Not found -> create -> success.
	fakeIAM := &fakeIAMClient{
		getRoleErr:    &iamtypes.NoSuchEntityException{},
		createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123:role/new")}},
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM }

	arn, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.NoError(t, err)
	require.Equal(t, "arn:aws:iam::123:role/new", arn)

	// Create returns empty ARN -> error.
	fakeIAM2 := &fakeIAMClient{
		getRoleErr:    &iamtypes.NoSuchEntityException{},
		createRoleOut: &iam.CreateRoleOutput{Role: &iamtypes.Role{Arn: aws.String("")}},
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM2 }
	_, err = ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)

	// Existing role, but attaching policy fails.
	fakeIAM3 := &fakeIAMClient{
		getRoleOut: &iam.GetRoleOutput{Role: &iamtypes.Role{Arn: aws.String("arn:aws:iam::123:role/existing")}},
		attachErr:  stdErrors.New("attach failed"),
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM3 }
	_, err = ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)
}

func TestEnsureIAMRoleForAPIGatewayLogs_UnexpectedErrors(t *testing.T) {
	prevNewIAM := newIAMClientFn
	t.Cleanup(func() { newIAMClientFn = prevNewIAM })

	fakeIAM := &fakeIAMClient{
		getRoleErr: stdErrors.New("boom"),
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM }
	_, err := ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "iam:GetRole")

	fakeIAM2 := &fakeIAMClient{
		getRoleErr:    &iamtypes.NoSuchEntityException{},
		createRoleErr: errSentinel,
	}
	newIAMClientFn = func(_ aws.Config) iamRoleAPI { return fakeIAM2 }
	_, err = ensureIAMRoleForAPIGatewayLogs(context.Background(), aws.Config{})
	require.ErrorIs(t, err, errSentinel)
	require.Contains(t, err.Error(), "iam:CreateRole")
}
