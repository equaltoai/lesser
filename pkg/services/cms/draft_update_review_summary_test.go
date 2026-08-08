package cms

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDraftContentWritesAdvanceVersionAndClearReviewSummary(t *testing.T) {
	svc, repo := newReviewService(t)
	ctx := context.Background()
	draft, err := repo.GetDraft(ctx, "owner", "d1")
	require.NoError(t, err)
	draft.ReviewedBy = "reviewer"
	draft.ReviewStatus = DraftReviewApproved
	draft.EditorNotes = "approved"

	require.NoError(t, svc.UpdateDraft(ctx, "owner", draft))
	require.Equal(t, 1, draft.AutosaveVersion)
	require.Empty(t, draft.ReviewedBy)
	require.Empty(t, draft.ReviewStatus)
	require.Empty(t, draft.EditorNotes)

	draft.ReviewedBy = "reviewer"
	draft.ReviewStatus = DraftReviewChangesRequested
	draft.EditorNotes = "revise"
	require.NoError(t, svc.Autosave(ctx, "owner", draft))
	require.Equal(t, 2, draft.AutosaveVersion)
	require.Empty(t, draft.ReviewedBy)
	require.Empty(t, draft.ReviewStatus)
	require.Empty(t, draft.EditorNotes)
}
