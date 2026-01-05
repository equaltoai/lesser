package cms

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type fakeSeriesRepo struct {
	createErr error
	updateErr error
	deleteErr error
	getErr    error

	series map[string]*models.Series
}

func seriesKey(authorID, seriesID string) string { return authorID + ":" + seriesID }

func (f *fakeSeriesRepo) CreateSeries(_ context.Context, series *models.Series) error {
	if f.createErr != nil {
		return f.createErr
	}
	if f.series == nil {
		f.series = map[string]*models.Series{}
	}
	f.series[seriesKey(series.AuthorID, series.ID)] = series
	return nil
}

func (f *fakeSeriesRepo) GetSeries(_ context.Context, authorID, seriesID string) (*models.Series, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	s, ok := f.series[seriesKey(authorID, seriesID)]
	if !ok {
		return nil, errors.New("not found")
	}
	return s, nil
}

func (f *fakeSeriesRepo) Update(_ context.Context, series *models.Series) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	if f.series == nil {
		f.series = map[string]*models.Series{}
	}
	f.series[seriesKey(series.AuthorID, series.ID)] = series
	return nil
}

func (f *fakeSeriesRepo) Delete(_ context.Context, _ string, _ string) error {
	return f.deleteErr
}

func (f *fakeSeriesRepo) ListSeriesByAuthor(_ context.Context, authorID string, _ int) ([]*models.Series, error) {
	out := make([]*models.Series, 0)
	for _, s := range f.series {
		if s.AuthorID == authorID {
			out = append(out, s)
		}
	}
	return out, nil
}

type fakeArticleSeriesRepo struct {
	articles  map[string]*models.Article
	getErrIDs map[string]error
	updateErr error
}

func (f *fakeArticleSeriesRepo) GetArticle(_ context.Context, articleID string) (*models.Article, error) {
	if err, ok := f.getErrIDs[articleID]; ok {
		return nil, err
	}
	a, ok := f.articles[articleID]
	if !ok {
		return nil, errors.New("not found")
	}
	return a, nil
}

func (f *fakeArticleSeriesRepo) UpdateArticle(_ context.Context, article *models.Article) error {
	if f.updateErr != nil {
		return f.updateErr
	}
	f.articles[article.ID] = article
	return nil
}

func TestSeriesService_Round25_Mainline(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	seriesRepo := &fakeSeriesRepo{series: map[string]*models.Series{}}
	articleRepo := &fakeArticleSeriesRepo{
		articles: map[string]*models.Article{
			"a1": {Object: models.Object{ID: "a1", Published: time.Now()}, Slug: "s"},
			"a2": {Object: models.Object{ID: "a2", Published: time.Now()}, Slug: "s", SeriesID: ptr("series-1")},
		},
		getErrIDs: map[string]error{},
	}
	svc := NewSeriesService(seriesRepo, articleRepo, zap.NewNop())

	series := &models.Series{ID: "series-1", AuthorID: "author", Title: "t"}
	err := svc.CreateSeries(ctx, series)
	require.NoError(t, err)
	assert.False(t, series.CreatedAt.IsZero())
	assert.False(t, series.UpdatedAt.IsZero())

	got, err := svc.GetSeries(ctx, "author", "series-1")
	require.NoError(t, err)
	require.NotNil(t, got)

	before := got.UpdatedAt
	time.Sleep(1 * time.Millisecond)
	got.Title = "updated"
	err = svc.UpdateSeries(ctx, got)
	require.NoError(t, err)
	assert.True(t, got.UpdatedAt.After(before))

	list, err := svc.ListSeriesByAuthor(ctx, "author", 10)
	require.NoError(t, err)
	assert.NotEmpty(t, list)

	err = svc.AddArticleToSeries(ctx, "a1", "series-1", 3)
	require.NoError(t, err)
	updated := articleRepo.articles["a1"]
	require.NotNil(t, updated.SeriesID)
	assert.Equal(t, "series-1", *updated.SeriesID)
	require.NotNil(t, updated.SeriesOrder)
	assert.Equal(t, 3, *updated.SeriesOrder)

	err = svc.RemoveArticleFromSeries(ctx, "a1")
	require.NoError(t, err)
	updated = articleRepo.articles["a1"]
	assert.Nil(t, updated.SeriesID)
	assert.Nil(t, updated.SeriesOrder)

	t.Run("reorder skips get errors but fails wrong series", func(t *testing.T) {
		articleRepo.getErrIDs["missing"] = errors.New("boom")
		err := svc.ReorderArticles(ctx, "series-1", map[string]int{
			"missing": 1,
			"a1":      2,
		})
		require.Error(t, err, "a1 is not in series-1 after removal")
		articleRepo.getErrIDs = map[string]error{}
	})

	t.Run("reorder returns update errors", func(t *testing.T) {
		// put article back in series
		err := svc.AddArticleToSeries(ctx, "a1", "series-1", 1)
		require.NoError(t, err)

		articleRepo.updateErr = errors.New("update failed")
		err = svc.ReorderArticles(ctx, "series-1", map[string]int{"a1": 2})
		require.Error(t, err)
		articleRepo.updateErr = nil
	})
}

func ptr(v string) *string { return &v }
