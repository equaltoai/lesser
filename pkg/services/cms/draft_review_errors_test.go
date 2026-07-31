package cms

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestDraftReviewPrincipalResolutionUsesSentinels(t *testing.T) {
	svc := NewDraftService(newReviewMemRepo(), nil, "example.test", true, zap.NewNop())

	_, err := svc.instancePrincipal(context.Background())
	require.ErrorIs(t, err, ErrInstancePrincipalUnavailable)
	require.NotErrorIs(t, err, ErrDraftReviewPrincipalApprovalRequired)

	providerErr := fmt.Errorf("ssm unavailable")
	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "", providerErr })
	_, err = svc.instancePrincipal(context.Background())
	require.ErrorIs(t, err, ErrInstancePrincipalUnavailable)
	require.ErrorIs(t, err, providerErr)
	require.NotErrorIs(t, err, ErrDraftReviewPrincipalApprovalRequired)

	svc.SetPrincipalUsernameProvider(func(context.Context) (string, error) { return "   ", nil })
	_, err = svc.instancePrincipal(context.Background())
	require.ErrorIs(t, err, ErrInstancePrincipalNotConfigured)
	require.NotErrorIs(t, err, ErrDraftReviewPrincipalApprovalRequired)
}
