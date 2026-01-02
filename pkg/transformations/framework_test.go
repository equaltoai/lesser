package transformations

import (
	"context"
	stdErrors "errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/errors"
	"github.com/stretchr/testify/require"
)

func TestBaseTransformer_Transform_ErrorsWhenMissingFunc(t *testing.T) {
	bt := &BaseTransformer[string, string]{}

	_, err := bt.Transform(context.Background(), "x")
	require.Error(t, err)

	appErr, ok := errors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, errors.CodeInternal, appErr.Code)
}

func TestBaseTransformer_TransformList_EmptyInputReturnsEmptySlice(t *testing.T) {
	bt := &BaseTransformer[int, int]{
		TransformFunc: func(_ context.Context, v int) (int, error) { return v * 2, nil },
	}

	out, err := bt.TransformList(context.Background(), nil)
	require.NoError(t, err)
	require.Empty(t, out)
}

func TestBaseTransformer_TransformList_StopsOnTransformError(t *testing.T) {
	bt := &BaseTransformer[int, int]{
		TransformFunc: func(_ context.Context, v int) (int, error) {
			if v == 2 {
				return 0, stdErrors.New("boom")
			}
			return v, nil
		},
	}

	out, err := bt.TransformList(context.Background(), []int{1, 2, 3})
	require.Nil(t, out)
	require.Error(t, err)
}

func TestTransformTimestamp(t *testing.T) {
	require.Equal(t, "", TransformTimestamp(time.Time{}, time.RFC3339))

	ts := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	require.Equal(t, ts.Format(time.RFC3339), TransformTimestamp(ts, ""))
}

func TestTransformIDList(t *testing.T) {
	require.Empty(t, TransformIDList(nil, "id_"))
	require.Equal(t, []string{"id_1", "id_2"}, TransformIDList([]string{"1", "2"}, "id_"))
	require.Equal(t, []string{"id_1"}, TransformIDList([]string{"id_1"}, "id_"))
}

func TestTransformPaginationInfo(t *testing.T) {
	out := TransformPaginationInfo(PaginationInfo{
		MaxID: "max",
		Limit: 20,
	})
	require.Equal(t, "max", out["max_id"])
	require.Equal(t, 20, out["limit"])
	_, hasOffset := out["offset"]
	require.False(t, hasOffset)
}

func TestTransformErrorResponse(t *testing.T) {
	out := TransformErrorResponse(nil)
	require.Equal(t, "unknown error", out["error"])

	err := stdErrors.New("bad")
	out = TransformErrorResponse(err)
	require.Equal(t, "bad", out["error"])
	require.Contains(t, out["error_type"], "error")
	require.NotEmpty(t, out["timestamp"])
}
