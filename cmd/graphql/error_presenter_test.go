package main

import (
	"context"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestGraphQLErrorPresenter_AttachesExtensionsForAppError(t *testing.T) {
	appErr := apperrors.NotFound("thing")

	got := graphQLErrorPresenter(context.Background(), appErr)
	require.NotNil(t, got)
	require.Equal(t, appErr.Message, got.Message)

	code, ok := got.Extensions["code"]
	require.True(t, ok)
	require.Equal(t, string(appErr.Code), code.(string))

	status, ok := got.Extensions["http_status"]
	require.True(t, ok)
	require.Equal(t, appErr.HTTPStatusCode, status.(int))
}
