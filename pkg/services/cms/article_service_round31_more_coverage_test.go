package cms

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormerrors "github.com/theory-cloud/tabletheory/pkg/errors"
	"go.uber.org/zap"
)

func TestArticleService_GetArticle_ValidatesID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, _ := newCMSMockDB(t)
	repo := &fakeArticleRepo{db: db, articles: map[string]*models.Article{}}
	svc := NewArticleService(repo, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	_, err := svc.GetArticle(ctx, "")
	require.Error(t, err)
}

func TestArticleService_M3ArticleRenderValidationErrors(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	repo := &fakeArticleRepo{articles: map[string]*models.Article{}}
	svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	require.Error(t, validateArticleRenderable(nil))

	for _, tc := range []struct {
		name          string
		content       string
		contentFormat string
		wantErr       error
	}{
		{
			name:          "unsupported format",
			content:       "body",
			contentFormat: "asciidoc",
			wantErr:       cmsrender.ErrUnsupportedContentFormat,
		},
		{
			name:          "invalid utf8",
			content:       string([]byte{0xff}),
			contentFormat: "markdown",
			wantErr:       nil,
		},
		{
			name:          "rendered html too large",
			content:       strings.Repeat("&", cmsrender.MaxArticleSourceBytes/2),
			contentFormat: "markdown",
			wantErr:       cmsrender.ErrArticleRenderedContentTooLarge,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.CreateArticle(ctx, &models.Article{
				Object: models.Object{
					ID:      "https://example.com/articles/" + strings.ReplaceAll(tc.name, " ", "-"),
					Name:    tc.name,
					Content: tc.content,
				},
				Slug:          strings.ReplaceAll(tc.name, " ", "-"),
				ContentFormat: tc.contentFormat,
			})
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.EqualError(t, err, "article content must be valid UTF-8")
			}
		})
	}

	err := svc.UpdateArticle(ctx, &models.Article{
		Object: models.Object{
			ID:      "https://example.com/articles/update-rendered-too-large",
			Name:    "Update Rendered Too Large",
			Content: strings.Repeat("&", cmsrender.MaxArticleSourceBytes/2),
		},
		ContentFormat: "markdown",
	})
	require.ErrorIs(t, err, cmsrender.ErrArticleRenderedContentTooLarge)
	require.Empty(t, repo.articles)
}

func TestArticleService_UpdateArticle_AllowsBlankSlugAndSetsUpdatedFields(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	repo := &fakeArticleRepo{
		db:       db,
		articles: map[string]*models.Article{},
	}
	repo.articles["a1"] = &models.Article{
		Object: models.Object{
			ID:           "a1",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Content:      "# Before\n\ntext",
		},
		Slug: "old",
	}

	svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	article := &models.Article{
		Object: models.Object{
			ID:           "a1",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Content:      "# After\n\ntext",
		},
		Slug: "",
	}

	require.NoError(t, svc.UpdateArticle(ctx, article))
	require.False(t, article.UpdatedAt.IsZero())
	require.False(t, article.Updated.IsZero())
}

func TestArticleService_UpdateArticle_RejectsCanonicalPublishedSlugChange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	repo := &fakeArticleRepo{
		db:       db,
		articles: map[string]*models.Article{},
	}
	articleID := "https://example.com/articles/original-slug"
	repo.articles[articleID] = &models.Article{
		Object: models.Object{
			ID:           articleID,
			Type:         activitypub.ArticleType,
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Content:      "before",
		},
		Slug: "original-slug",
	}

	svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	err := svc.UpdateArticle(ctx, &models.Article{
		Object: models.Object{
			ID:           articleID,
			Type:         activitypub.ArticleType,
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Content:      "after",
		},
		Slug: "renamed-slug",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "immutable")

	err = svc.UpdateArticle(ctx, &models.Article{
		Object: models.Object{
			ID:           articleID,
			Type:         activitypub.ArticleType,
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Content:      "after",
		},
		Slug: "original-slug",
	})
	require.NoError(t, err)
}

func TestArticleService_DeleteArticle_ValidatesInput(t *testing.T) {
	t.Parallel()

	svc := NewArticleService(&fakeArticleRepo{}, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	require.Error(t, svc.DeleteArticle(context.Background(), nil))
	require.Error(t, svc.DeleteArticle(context.Background(), &models.Article{}))
}

func TestArticleService_deleteCMSArticleIndexes_LogsWarnOnUnexpectedDeleteError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Delete").Return(errors.New("delete failed")).Maybe()

	repo := &fakeArticleRepo{db: db, articles: map[string]*models.Article{}}
	svc := NewArticleService(repo, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	seriesID := "alice|series-1"
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    time.Now(),
		},
		SeriesID:    &seriesID,
		CategoryIDs: []string{"cat-1"},
	}

	svc.deleteCMSArticleIndexes(ctx, article)
}

func TestArticleService_deleteCMSArticleIndexesForRemovedGroups_IgnoresNotFound(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Delete").Return(dynamormerrors.ErrItemNotFound).Maybe()

	repo := &fakeArticleRepo{db: db, articles: map[string]*models.Article{}}
	svc := NewArticleService(repo, fakeActorRepo{}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	before := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		CategoryIDs: []string{"cat-1"},
	}
	after := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			AttributedTo: "https://example.com/users/alice",
			Published:    published,
		},
		CategoryIDs: []string{},
	}

	svc.deleteCMSArticleIndexesForRemovedGroups(ctx, before, after)
}

func TestArticleService_federateArticleWriteActivity_ReturnsWhenActorLookupFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db, q := newCMSMockDB(t)
	q.On("Create").Return(nil).Maybe()
	q.On("Delete").Return(nil).Maybe()

	repo := &fakeArticleRepo{db: db, articles: map[string]*models.Article{}}
	svc := NewArticleService(repo, fakeActorRepo{err: errors.New("no actor")}, nil, nil, nil, &fakeFederation{}, zap.NewNop())

	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/1",
			Name:         "title",
			Content:      "<p>hello</p>",
			Published:    time.Now(),
			AttributedTo: "https://example.com/users/alice",
			Type:         activitypub.ArticleType,
		},
		Slug: "slug",
	}

	svc.federateArticleWriteActivity(ctx, article, activitypub.CreateType, "create")
}
