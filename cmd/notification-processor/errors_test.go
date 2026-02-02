package main

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestNotificationProcessorErrors(t *testing.T) {
	internal := stderrors.New("internal")

	cases := []struct {
		name         string
		err          error
		wantCode     apperrors.ErrorCode
		wantCategory apperrors.ErrorCategory
		wantMetaKey  string
		wantMetaVal  any
		wantWrapped  error
	}{
		{
			name:         "ErrNotificationBudgetExceeded",
			err:          ErrNotificationBudgetExceeded(),
			wantCode:     apperrors.CodeRateLimited,
			wantCategory: apperrors.CategoryValidation,
		},
		{
			name:         "ErrSNSClientNotInitialized",
			err:          ErrSNSClientNotInitialized(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrAPIGatewayClientNotInitialized",
			err:          ErrAPIGatewayClientNotInitialized(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrSQSClientNotInitialized",
			err:          ErrSQSClientNotInitialized(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrPushTopicNotConfigured",
			err:          ErrPushTopicNotConfigured(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrRetryQueueNotConfigured",
			err:          ErrRetryQueueNotConfigured(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrSQSConfigurationIncomplete",
			err:          ErrSQSConfigurationIncomplete(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrUnsupportedDeliveryChannel",
			err:          ErrUnsupportedDeliveryChannel("fax"),
			wantCode:     apperrors.CodeInvalidInput,
			wantCategory: apperrors.CategoryValidation,
			wantMetaKey:  "channel",
			wantMetaVal:  "fax",
		},
		{
			name:         "ErrDeliveryChannelFailed",
			err:          ErrDeliveryChannelFailed(),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
		},
		{
			name:         "ErrUnmarshalDeliveryRequest",
			err:          ErrUnmarshalDeliveryRequest(internal),
			wantCode:     apperrors.CodeInvalidFormat,
			wantCategory: apperrors.CategoryValidation,
			wantWrapped:  internal,
		},
		{
			name:         "ErrGetNotification",
			err:          ErrGetNotification(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrMarshalPushPayload",
			err:          ErrMarshalPushPayload(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrSendPushNotification",
			err:          ErrSendPushNotification(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrGetWebSocketConnections",
			err:          ErrGetWebSocketConnections(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrDeliverWebSocketMessage",
			err:          ErrDeliverWebSocketMessage(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrMarshalWebSocketMessage",
			err:          ErrMarshalWebSocketMessage(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrPublishToSNS",
			err:          ErrPublishToSNS(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrMarshalScheduledRequest",
			err:          ErrMarshalScheduledRequest(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrRequeueNotification",
			err:          ErrRequeueNotification(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrMarshalRetryRequest",
			err:          ErrMarshalRetryRequest(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrScheduleRetry",
			err:          ErrScheduleRetry(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.err)

			var appErr *apperrors.AppError
			require.True(t, stderrors.As(tc.err, &appErr))
			require.Equal(t, tc.wantCode, appErr.Code)
			require.Equal(t, tc.wantCategory, appErr.Category)

			if tc.wantMetaKey != "" {
				require.Contains(t, appErr.Metadata, tc.wantMetaKey)
				require.Equal(t, tc.wantMetaVal, appErr.Metadata[tc.wantMetaKey])
			}

			if tc.wantWrapped != nil {
				require.ErrorIs(t, tc.err, tc.wantWrapped)
			}
		})
	}
}
