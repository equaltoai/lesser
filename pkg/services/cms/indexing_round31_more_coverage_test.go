package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type seriesCountCall struct {
	authorID string
	seriesID string
	delta    int
}

type fakeSeriesCountUpdater struct {
	calls []seriesCountCall
	err   error
}

func (f *fakeSeriesCountUpdater) UpdateArticleCount(ctx context.Context, authorID string, seriesID string, delta int) error {
	f.calls = append(f.calls, seriesCountCall{authorID: authorID, seriesID: seriesID, delta: delta})
	return f.err
}

type categoryCountCall struct {
	categoryID string
	delta      int
}

type fakeCategoryCountUpdater struct {
	calls []categoryCountCall
	err   error
}

func (f *fakeCategoryCountUpdater) UpdateArticleCount(ctx context.Context, categoryID string, delta int) error {
	f.calls = append(f.calls, categoryCountCall{categoryID: categoryID, delta: delta})
	return f.err
}

func TestCMSArticleIndexEntries_ReturnsNilForNilOrMissingID(t *testing.T) {
	t.Parallel()

	require.Nil(t, cmsArticleIndexEntries(nil))
	require.Nil(t, cmsArticleIndexEntries(&models.Article{}))
}

func TestCMSArticleIndexEntries_BuildsEntriesAndDedupesCategories(t *testing.T) {
	t.Parallel()

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	seriesID := "alice|series-1"
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		SeriesID:    &seriesID,
		CategoryIDs: []string{"cat-1", "cat-1", " ", "cat-2", "cat-2"},
	}

	entries := cmsArticleIndexEntries(article)
	require.Len(t, entries, 4) // author + series + 2 unique categories

	expectedSK := models.CMSArticleIndexSK(published, article.ID)
	seenPK := map[string]struct{}{}
	for _, entry := range entries {
		require.NotNil(t, entry)
		require.Equal(t, expectedSK, entry.SK)
		require.Equal(t, article.ID, entry.ArticleID)
		require.False(t, entry.CreatedAt.IsZero())
		seenPK[entry.PK] = struct{}{}
	}

	require.Contains(t, seenPK, models.CMSArticleIndexPKForAuthor(article.AttributedTo))
	require.Contains(t, seenPK, models.CMSArticleIndexPKForSeries(seriesID))
	require.Contains(t, seenPK, models.CMSArticleIndexPKForCategory("cat-1"))
	require.Contains(t, seenPK, models.CMSArticleIndexPKForCategory("cat-2"))
}

func TestCMSArticleIndexEntriesForRemovedGroups_ReturnsBeforeEntriesWhenAfterNil(t *testing.T) {
	t.Parallel()

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	seriesID := "alice|series-1"
	before := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		SeriesID:    &seriesID,
		CategoryIDs: []string{"cat-1"},
	}

	removed := cmsArticleIndexEntriesForRemovedGroups(before, nil)
	require.NotEmpty(t, removed)
	require.Len(t, removed, len(cmsArticleIndexEntries(before)))
}

func TestCMSArticleIndexEntriesForRemovedGroups_DetectsRemovedGroups(t *testing.T) {
	t.Parallel()

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	seriesBefore := "alice|series-old"
	seriesAfter := "alice|series-new"

	before := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		SeriesID:    &seriesBefore,
		CategoryIDs: []string{"cat-1", "cat-2"},
	}

	after := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/bob",
			Published:    published,
		},
		SeriesID:    &seriesAfter,
		CategoryIDs: []string{"cat-2", "cat-3"},
	}

	removed := cmsArticleIndexEntriesForRemovedGroups(before, after)
	require.Len(t, removed, 3)

	expectedSK := models.CMSArticleIndexSK(published, before.ID)
	seenPK := map[string]struct{}{}
	for _, entry := range removed {
		require.NotNil(t, entry)
		require.Equal(t, expectedSK, entry.SK)
		seenPK[entry.PK] = struct{}{}
	}

	require.Contains(t, seenPK, models.CMSArticleIndexPKForAuthor(before.AttributedTo))
	require.Contains(t, seenPK, models.CMSArticleIndexPKForSeries(seriesBefore))
	require.Contains(t, seenPK, models.CMSArticleIndexPKForCategory("cat-1"))
}

func TestCMSParseSeriesGraphQLID(t *testing.T) {
	t.Parallel()

	authorID, seriesID, ok := cmsParseSeriesGraphQLID("")
	require.False(t, ok)
	require.Empty(t, authorID)
	require.Empty(t, seriesID)

	authorID, seriesID, ok = cmsParseSeriesGraphQLID("alice")
	require.False(t, ok)

	authorID, seriesID, ok = cmsParseSeriesGraphQLID("alice|")
	require.False(t, ok)

	authorID, seriesID, ok = cmsParseSeriesGraphQLID("|series")
	require.False(t, ok)

	authorID, seriesID, ok = cmsParseSeriesGraphQLID(" alice | series-1 ")
	require.True(t, ok)
	require.Equal(t, "alice", authorID)
	require.Equal(t, "series-1", seriesID)
}

func TestCMSUpdateArticleCountsBestEffort_UpdatesSeriesAndCategoryCounts(t *testing.T) {
	t.Parallel()

	series := &fakeSeriesCountUpdater{err: errors.New("series update failed")}
	categories := &fakeCategoryCountUpdater{err: errors.New("category update failed")}

	beforeSeries := "alice|series-old"
	afterSeries := "alice|series-new"

	before := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
		},
		SeriesID:    &beforeSeries,
		CategoryIDs: []string{"cat-1", "cat-2"},
	}
	after := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
		},
		SeriesID:    &afterSeries,
		CategoryIDs: []string{"cat-2", "cat-3"},
	}

	cmsUpdateArticleCountsBestEffort(context.Background(), series, categories, before, after, zap.NewNop())

	require.Equal(t, []seriesCountCall{
		{authorID: "alice", seriesID: "series-old", delta: -1},
		{authorID: "alice", seriesID: "series-new", delta: 1},
	}, series.calls)

	seenCategory := map[categoryCountCall]struct{}{}
	for _, call := range categories.calls {
		seenCategory[call] = struct{}{}
	}
	require.Contains(t, seenCategory, categoryCountCall{categoryID: "cat-1", delta: -1})
	require.Contains(t, seenCategory, categoryCountCall{categoryID: "cat-3", delta: 1})
	require.Len(t, categories.calls, 2)
}

func TestCMSUpdateArticleCountsBestEffort_SkipsInvalidSeriesIDs(t *testing.T) {
	t.Parallel()

	series := &fakeSeriesCountUpdater{}
	categories := &fakeCategoryCountUpdater{}

	invalidSeries := "not-a-series-id"
	before := &models.Article{SeriesID: &invalidSeries, CategoryIDs: []string{"cat-1"}}
	after := &models.Article{SeriesID: &invalidSeries, CategoryIDs: []string{"cat-1"}}

	cmsUpdateArticleCountsBestEffort(context.Background(), series, categories, before, after, zap.NewNop())

	require.Empty(t, series.calls)
	require.Empty(t, categories.calls)
}
