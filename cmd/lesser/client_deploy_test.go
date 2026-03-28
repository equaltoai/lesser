package main

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/cloudfront"
	"github.com/equaltoai/lesser/pkg/deploy/naming"
	"github.com/stretchr/testify/require"
)

func TestParseStageSelection(t *testing.T) {
	stages, err := parseStageSelection("dev")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev}, stages)

	stages, err = parseStageSelection("staging")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageStaging}, stages)

	stages, err = parseStageSelection("live")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageLive}, stages)

	stages, err = parseStageSelection("")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, stages)

	stages, err = parseStageSelection("both")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageLive}, stages)

	stages, err = parseStageSelection("all")
	require.NoError(t, err)
	require.Equal(t, []naming.Stage{naming.StageDev, naming.StageStaging, naming.StageLive}, stages)

	_, err = parseStageSelection("nope")
	require.Error(t, err)
}

func TestParseClientDeployArgs_RequiresFlags(t *testing.T) {
	_, err := parseClientDeployArgs(nil)
	require.Error(t, err)

	_, err = parseClientDeployArgs([]string{"--app", "app"})
	require.Error(t, err)
}

func TestRunClientDeploy_IsRetired(t *testing.T) {
	err := runClientDeploy([]string{
		"--app", "app",
		"--base-domain", "example.com",
		"--aws-profile", "profile",
		"--dist", "dist",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "retired")
	require.Contains(t, err.Error(), "lesser client install")
}

func TestInvalidateClientPaths_PropagatesErrors(t *testing.T) {
	previous := createCloudfrontInvalidationFn
	t.Cleanup(func() { createCloudfrontInvalidationFn = previous })

	createCloudfrontInvalidationFn = func(context.Context, *cloudfront.Client, *cloudfront.CreateInvalidationInput) (*cloudfront.CreateInvalidationOutput, error) {
		return nil, errSentinel
	}

	require.ErrorIs(t, invalidateClientPaths(context.Background(), &cloudfront.Client{}, "DIST"), errSentinel)
}
