package main

import (
	stderrors "errors"
	"testing"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestModerationProcessorErrors(t *testing.T) {
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
			name:         "ErrInvalidReviewPKFormat",
			err:          ErrInvalidReviewPKFormat("pk"),
			wantCode:     apperrors.CodeInvalidFormat,
			wantCategory: apperrors.CategoryValidation,
			wantMetaKey:  "pk",
			wantMetaVal:  "pk",
		},
		{
			name:         "ErrInvalidReviewSKFormat",
			err:          ErrInvalidReviewSKFormat("sk"),
			wantCode:     apperrors.CodeInvalidFormat,
			wantCategory: apperrors.CategoryValidation,
			wantMetaKey:  "sk",
			wantMetaVal:  "sk",
		},
		{
			name:         "ErrNotReviewRecord",
			err:          ErrNotReviewRecord(),
			wantCode:     apperrors.CodeInvalidInput,
			wantCategory: apperrors.CategoryValidation,
		},
		{
			name:         "ErrNotEventRecord",
			err:          ErrNotEventRecord(),
			wantCode:     apperrors.CodeInvalidInput,
			wantCategory: apperrors.CategoryValidation,
		},
		{
			name:         "ErrNotDecisionRecord",
			err:          ErrNotDecisionRecord(),
			wantCode:     apperrors.CodeInvalidInput,
			wantCategory: apperrors.CategoryValidation,
		},
		{
			name:         "ErrUnknownActionType",
			err:          ErrUnknownActionType("mystery"),
			wantCode:     apperrors.CodeInvalidInput,
			wantCategory: apperrors.CategoryValidation,
			wantMetaKey:  "action_type",
			wantMetaVal:  "mystery",
		},
		{
			name:         "ErrFailedToRetrieveModerators",
			err:          ErrFailedToRetrieveModerators(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrNoAdminsAvailableForFallback",
			err:          ErrNoAdminsAvailableForFallback(),
			wantCode:     apperrors.CodeNotFound,
			wantCategory: apperrors.CategoryValidation,
		},
		{
			name:         "ErrEnforcementFailed",
			err:          ErrEnforcementFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrContentRemovalFailed",
			err:          ErrContentRemovalFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrTimelineFilteringFailed",
			err:          ErrTimelineFilteringFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToExtractReview",
			err:          ErrFailedToExtractReview(internal),
			wantCode:     apperrors.CodeEventProcessingFailed,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToExtractEvent",
			err:          ErrFailedToExtractEvent(internal),
			wantCode:     apperrors.CodeEventProcessingFailed,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToExtractDecision",
			err:          ErrFailedToExtractDecision(internal),
			wantCode:     apperrors.CodeEventProcessingFailed,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToGetAvailableModerators",
			err:          ErrFailedToGetAvailableModerators(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToGetAdminList",
			err:          ErrFailedToGetAdminList(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToAddAutomaticReview",
			err:          ErrFailedToAddAutomaticReview(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToProcessAutomaticReview",
			err:          ErrFailedToProcessAutomaticReview(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrUserUpdateFailed",
			err:          ErrUserUpdateFailed(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrTimelineFilteringOp",
			err:          ErrTimelineFilteringOp(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrSearchOperationFailed",
			err:          ErrSearchOperationFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFederationOpFailed",
			err:          ErrFederationOpFailed(internal),
			wantCode:     apperrors.CodeDeliveryFailed,
			wantCategory: apperrors.CategoryFederation,
			wantWrapped:  internal,
		},
		{
			name:         "ErrObjectDeletionFailed",
			err:          ErrObjectDeletionFailed(internal),
			wantCode:     apperrors.CodeQueryFailed,
			wantCategory: apperrors.CategoryStorage,
			wantWrapped:  internal,
		},
		{
			name:         "ErrTimelineRemovalFailed",
			err:          ErrTimelineRemovalFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrSearchRemovalFailed",
			err:          ErrSearchRemovalFailed(internal),
			wantCode:     apperrors.CodeInternal,
			wantCategory: apperrors.CategoryLambda,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFederationDeletionFailed",
			err:          ErrFederationDeletionFailed(internal),
			wantCode:     apperrors.CodeDeliveryFailed,
			wantCategory: apperrors.CategoryFederation,
			wantWrapped:  internal,
		},
		{
			name:         "ErrFailedToProcessRecords",
			err:          ErrFailedToProcessRecords(internal),
			wantCode:     apperrors.CodeSQSProcessingFailed,
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
