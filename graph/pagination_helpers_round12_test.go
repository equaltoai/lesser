package graph

import (
	"math"
	"testing"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/stretchr/testify/require"
)

func TestRound12Pagination_ParseArgs(t *testing.T) {
	first := 10
	last := 5
	_, err := ParsePaginationArgs(&first, nil, &last, nil)
	require.ErrorIs(t, err, ErrPaginationMixedParams)

	neg := -1
	_, err = ParsePaginationArgs(&neg, nil, nil, nil)
	require.ErrorIs(t, err, ErrFirstMustBePositive)

	big := 1000
	opts, err := ParsePaginationArgs(&big, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, opts.First)
	require.Equal(t, 100, *opts.First)

	opts, err = ParsePaginationArgs(nil, nil, nil, nil)
	require.NoError(t, err)
	require.NotNil(t, opts.First)
	require.Equal(t, 20, *opts.First)

	negLast := -2
	_, err = ParsePaginationArgs(nil, nil, &negLast, nil)
	require.ErrorIs(t, err, ErrLastMustBePositive)

	bigLast := 1000
	opts, err = ParsePaginationArgs(nil, nil, &bigLast, nil)
	require.NoError(t, err)
	require.NotNil(t, opts.Last)
	require.Equal(t, 100, *opts.Last)
}

func TestRound12Pagination_CursorEncodeDecode(t *testing.T) {
	require.Equal(t, model.Cursor(""), EncodeGraphQLCursor(nil))

	bad := EncodeGraphQLCursor(&CursorData{ID: "x", Timestamp: time.Now(), Score: math.NaN()})
	require.Equal(t, model.Cursor(""), bad)

	data := &CursorData{
		ID:        "id-1",
		Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Score:     42.0,
	}
	cursor := EncodeGraphQLCursor(data)
	require.NotEmpty(t, cursor)

	decoded, err := DecodeGraphQLCursor(cursor)
	require.NoError(t, err)
	require.NotNil(t, decoded)
	require.Equal(t, data.ID, decoded.ID)
	require.True(t, decoded.Timestamp.Equal(data.Timestamp))
	require.Equal(t, data.Score, decoded.Score)

	decoded, err = DecodeGraphQLCursor("")
	require.NoError(t, err)
	require.Nil(t, decoded)

	_, err = DecodeGraphQLCursor("not-base64")
	require.Error(t, err)

	_, err = DecodeGraphQLCursor(model.Cursor("bm90LWpzb24=")) // "not-json"
	require.Error(t, err)
}

func TestRound12Pagination_BuildPageInfo(t *testing.T) {
	empty := BuildPageInfo(nil, true, false, func(v interface{}) model.Cursor { return v.(model.Cursor) })
	require.NotNil(t, empty)
	require.True(t, empty.HasPreviousPage)
	require.False(t, empty.HasNextPage)
	require.Nil(t, empty.StartCursor)
	require.Nil(t, empty.EndCursor)

	edges := []interface{}{model.Cursor("a"), model.Cursor("b")}
	info := BuildPageInfo(edges, false, true, func(v interface{}) model.Cursor { return v.(model.Cursor) })
	require.NotNil(t, info.StartCursor)
	require.NotNil(t, info.EndCursor)
	require.Equal(t, model.Cursor("a"), *info.StartCursor)
	require.Equal(t, model.Cursor("b"), *info.EndCursor)
}

func TestRound12Pagination_DynamormConversionAndResults(t *testing.T) {
	first := 2
	after := model.Cursor("not-base64")
	opts, err := ParsePaginationArgs(&first, &after, nil, nil)
	require.NoError(t, err)
	_, err = ConvertToDynamORMPagination(opts)
	require.Error(t, err)

	validAfter := EncodeGraphQLCursor(&CursorData{
		ID:        "id",
		Timestamp: time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC),
		Score:     1.0,
	})
	opts, err = ParsePaginationArgs(&first, &validAfter, nil, nil)
	require.NoError(t, err)
	dynamormOpts, err := ConvertToDynamORMPagination(opts)
	require.NoError(t, err)
	require.Equal(t, 3, dynamormOpts.Limit)

	results := []int{1, 2, 3}
	paged, hasPrev, hasNext, err := ApplyPaginationToResults(results, opts, func(v int) string { return "" }, func(int) time.Time { return time.Time{} }, func(int) float64 { return 0 })
	require.NoError(t, err)
	require.True(t, hasPrev)
	require.True(t, hasNext)
	require.Equal(t, []int{1, 2}, paged)

	last := 2
	before := model.Cursor("x")
	backOpts, err := ParsePaginationArgs(nil, nil, &last, &before)
	require.NoError(t, err)
	backPaged, backHasPrev, backHasNext, err := ApplyPaginationToResults([]int{1, 2, 3}, backOpts, func(v int) string { return "" }, func(int) time.Time { return time.Time{} }, func(int) float64 { return 0 })
	require.NoError(t, err)
	require.True(t, backHasPrev)
	require.True(t, backHasNext)
	require.Equal(t, []int{3, 2}, backPaged)
}

type round12EdgeItem struct {
	id  string
	t   time.Time
	f64 float64
}

func TestRound12Pagination_CreateEdges(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	items := []round12EdgeItem{{id: "a", t: now, f64: 1.0}}

	_, err := CreateObjectEdges(items, func(round12EdgeItem) (*model.Object, error) { return nil, ErrObjectNotFound }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t }, func(i round12EdgeItem) float64 { return i.f64 })
	require.Error(t, err)

	edges, err := CreateObjectEdges(items, func(round12EdgeItem) (*model.Object, error) { return &model.Object{ID: "o"}, nil }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t }, func(i round12EdgeItem) float64 { return i.f64 })
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.NotEmpty(t, edges[0].Cursor)
	require.NotNil(t, edges[0].Node)

	_, err = CreateNotificationEdges(items, func(round12EdgeItem) (*model.Notification, error) { return nil, ErrObjectNotFound }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t })
	require.Error(t, err)

	notifEdges, err := CreateNotificationEdges(items, func(round12EdgeItem) (*model.Notification, error) { return &model.Notification{ID: "n"}, nil }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t })
	require.NoError(t, err)
	require.Len(t, notifEdges, 1)

	_, err = CreateHashtagEdges(items, func(round12EdgeItem) (*model.Hashtag, error) { return nil, ErrObjectNotFound }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t })
	require.Error(t, err)
	_, err = CreateSeveredRelationshipEdges(items, func(round12EdgeItem) (*model.SeveredRelationship, error) { return nil, ErrObjectNotFound }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t })
	require.Error(t, err)
	_, err = CreateAffectedRelationshipEdges(items, func(round12EdgeItem) (*model.AffectedRelationship, error) { return nil, ErrObjectNotFound }, func(i round12EdgeItem) string { return i.id }, func(i round12EdgeItem) time.Time { return i.t })
	require.Error(t, err)
}
