package graph

import (
	"testing"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestDraftReviewCursorTrimsAtResolverBoundary(t *testing.T) {
	blank := model.Cursor(" \t\n ")
	cursor := trimDraftReviewCursor(&blank)
	require.Empty(t, cursor)
	pageInfo := &model.PageInfo{HasPreviousPage: cursor != ""}
	require.False(t, pageInfo.HasPreviousPage, "a whitespace-only cursor must not report a previous page")

	after := model.Cursor("  TIME#cursor  ")
	cursor = trimDraftReviewCursor(&after)
	require.Equal(t, "TIME#cursor", cursor)
	pageInfo.HasPreviousPage = cursor != ""
	require.True(t, pageInfo.HasPreviousPage)
}
