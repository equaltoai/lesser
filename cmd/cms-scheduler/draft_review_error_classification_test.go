package main

import (
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/stretchr/testify/require"
)

func TestScheduledPublishFailureReasonUsesApprovalSentinels(t *testing.T) {
	require.Equal(t, scheduledReviewBlocked, scheduledPublishFailureReason(cms.ErrDraftReviewApprovalRequired))
	require.Equal(t, scheduledReviewBlocked, scheduledPublishFailureReason(cms.ErrDraftReviewPrincipalApprovalRequired))
	require.Equal(t, scheduledPublishFailed, scheduledPublishFailureReason(cms.ErrInstancePrincipalUnavailable))
	require.Equal(t, scheduledPublishFailed, scheduledPublishFailureReason(cms.ErrInstancePrincipalNotConfigured))
	require.Equal(t, scheduledPublishFailed, scheduledPublishFailureReason(errors.New("instance principal is unavailable")))
}
