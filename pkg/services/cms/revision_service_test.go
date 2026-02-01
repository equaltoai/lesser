package cms

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	dynamormcore "github.com/theory-cloud/tabletheory/pkg/core"
	"go.uber.org/zap"
)

type memRevisionRepo struct {
	byObject map[string]map[int]*models.Revision
	created  []*models.Revision
}

func newMemRevisionRepo() *memRevisionRepo {
	return &memRevisionRepo{
		byObject: map[string]map[int]*models.Revision{},
	}
}

func (r *memRevisionRepo) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	objectRevs := r.byObject[objectID]
	if len(objectRevs) == 0 {
		return []*models.Revision{}, nil
	}

	versions := make([]int, 0, len(objectRevs))
	for version := range objectRevs {
		versions = append(versions, version)
	}
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			if versions[j] > versions[i] {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

	out := make([]*models.Revision, 0, len(versions))
	for _, version := range versions {
		out = append(out, cloneRevision(objectRevs[version]))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memRevisionRepo) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	objectRevs := r.byObject[objectID]
	if objectRevs == nil {
		return nil, apperrors.NotFound("revision")
	}
	rev, ok := objectRevs[version]
	if !ok {
		return nil, apperrors.NotFound("revision")
	}
	return cloneRevision(rev), nil
}

func (r *memRevisionRepo) CreateRevision(ctx context.Context, revision *models.Revision) error {
	if revision == nil {
		return apperrors.ValidationFailedWithField("revision")
	}
	if r.byObject[revision.ObjectID] == nil {
		r.byObject[revision.ObjectID] = map[int]*models.Revision{}
	}
	r.byObject[revision.ObjectID][revision.Version] = cloneRevision(revision)
	r.created = append(r.created, cloneRevision(revision))
	return nil
}

func (r *memRevisionRepo) Delete(ctx context.Context, pk, sk string) error {
	return nil
}

func cloneRevision(rev *models.Revision) *models.Revision {
	if rev == nil {
		return nil
	}
	cp := *rev
	return &cp
}

type memArticleRepo struct {
	byID    map[string]*models.Article
	updated []*models.Article
}

func newMemArticleRepo() *memArticleRepo {
	return &memArticleRepo{
		byID: map[string]*models.Article{},
	}
}

func (r *memArticleRepo) GetDB() dynamormcore.DB { return nil }

func (r *memArticleRepo) GetArticle(ctx context.Context, id string) (*models.Article, error) {
	article, ok := r.byID[id]
	if !ok {
		return nil, apperrors.NotFound("article")
	}
	return cloneArticle(article), nil
}

func (r *memArticleRepo) UpdateArticle(ctx context.Context, article *models.Article) error {
	if article == nil {
		return apperrors.ValidationFailedWithField("article")
	}
	r.byID[article.ID] = cloneArticle(article)
	r.updated = append(r.updated, cloneArticle(article))
	return nil
}

type memCMSIndexWriter struct {
	created []*models.CMSArticleIndex
	deleted []*models.CMSArticleIndex
}

func (w *memCMSIndexWriter) Create(ctx context.Context, entry *models.CMSArticleIndex) error {
	w.created = append(w.created, cloneCMSIndex(entry))
	return nil
}

func (w *memCMSIndexWriter) Delete(ctx context.Context, entry *models.CMSArticleIndex) error {
	w.deleted = append(w.deleted, cloneCMSIndex(entry))
	return nil
}

func cloneCMSIndex(entry *models.CMSArticleIndex) *models.CMSArticleIndex {
	if entry == nil {
		return nil
	}
	cp := *entry
	return &cp
}

func TestRevisionServiceBuildRevisionMetadataIncludesCMSFields(t *testing.T) {
	t.Parallel()

	svc := &RevisionService{}
	seriesID := "alice|series-1"
	seriesOrder := 2
	article := &models.Article{
		Object: models.Object{
			ID:           "https://example.com/articles/hello",
			Type:         activitypub.ArticleType,
			Name:         "Hello",
			Summary:      "Summary",
			AttributedTo: "https://example.com/users/alice",
		},
		Subtitle:      "Sub",
		Excerpt:       "Excerpt",
		ContentFormat: "markdown",
		TableOfContents: []models.TOCEntry{
			{ID: "h1", Level: 1, Text: "Hello"},
		},
		ReadingTimeMinutes: 1,
		WordCount:          10,
		SeriesID:           &seriesID,
		SeriesOrder:        &seriesOrder,
		CategoryIDs:        []string{"cat-1", "cat-2"},
		SEOTitle:           "SEO",
		SEODescription:     "SEO desc",
		CanonicalURL:       "https://canonical.example.com/hello",
		OGImage:            "https://cdn.example.com/og.png",
		EditorNotes:        "note",
		ReviewStatus:       "reviewed",
		FeaturedImage: &models.Media{
			MediaID:     "media-1",
			ContentType: "image/png",
			CDNUrl:      "https://cdn.example.com/media.png",
			Description: "alt",
			Blurhash:    "hash",
			Width:       10,
			Height:      20,
			FileSize:    42,
		},
	}

	raw, err := svc.buildRevisionMetadata(article)
	require.NoError(t, err)

	var meta articleRevisionMetadata
	require.NoError(t, json.Unmarshal([]byte(raw), &meta))
	require.Equal(t, article.Name, meta.Name)
	require.Equal(t, article.Summary, meta.Summary)
	require.Equal(t, article.AttributedTo, meta.AttributedTo)
	require.Equal(t, article.Subtitle, meta.Subtitle)
	require.Equal(t, article.Excerpt, meta.Excerpt)
	require.Equal(t, article.ContentFormat, meta.ContentFormat)
	require.Equal(t, article.SeriesID, meta.SeriesID)
	require.Equal(t, article.SeriesOrder, meta.SeriesOrder)
	require.Equal(t, article.CategoryIDs, meta.CategoryIDs)
	require.Equal(t, article.SEOTitle, meta.SEOTitle)
	require.Equal(t, article.SEODescription, meta.SEODescription)
	require.Equal(t, article.CanonicalURL, meta.CanonicalURL)
	require.Equal(t, article.OGImage, meta.OGImage)
	require.Equal(t, article.EditorNotes, meta.EditorNotes)
	require.Equal(t, article.ReviewStatus, meta.ReviewStatus)
	require.NotNil(t, meta.FeaturedImage)
	require.Equal(t, "media-1", meta.FeaturedImage.ID)
}

func TestRevisionServiceRestoreRevisionAppliesMetadataAndWritesAuditRevisions(t *testing.T) {
	t.Parallel()

	const objectID = "https://example.com/articles/hello-world"
	published := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	articleRepo := newMemArticleRepo()
	articleRepo.byID[objectID] = &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			Name:         "Before",
			Summary:      "Before summary",
			AttributedTo: "https://example.com/users/alice",
			Content:      "# Before\n\nOld content.",
			Published:    published,
			Updated:      published,
		},
		ContentFormat: "markdown",
		SeriesID:      ptrString("alice|series-old"),
		CategoryIDs:   []string{"cat-old"},
	}

	meta := articleRevisionMetadata{
		Name:          "After",
		Summary:       "After summary",
		AttributedTo:  "https://example.com/users/alice",
		ContentFormat: "markdown",
		SeriesID:      ptrString("alice|series-new"),
		CategoryIDs:   []string{"cat-new"},
		FeaturedImage: &articleRevisionMedia{
			ID:          "media-1",
			ContentType: "image/png",
			CDNURL:      "https://cdn.example.com/m.png",
			Description: "alt",
		},
	}
	metaJSON, err := json.Marshal(meta)
	require.NoError(t, err)

	revisionRepo := newMemRevisionRepo()
	revisionRepo.byObject[objectID] = map[int]*models.Revision{
		1: {
			ObjectID:     objectID,
			Version:      1,
			Content:      "# After\n\nNew content.",
			ContentHash:  "hash",
			MetadataJSON: string(metaJSON),
			ChangeType:   revisionChangeTypeUpdate,
			CreatedAt:    time.Now(),
		},
	}

	indexWriter := &memCMSIndexWriter{}
	svc := &RevisionService{
		revisionRepo:          revisionRepo,
		articleRepo:           articleRepo,
		articleIndexWriter:    indexWriter,
		maxRevisionsPerObject: 0,
		logger:                zap.NewNop(),
	}

	restored, err := svc.RestoreRevision(context.Background(), objectID, 1)
	require.NoError(t, err)
	require.NotNil(t, restored)
	require.Equal(t, "# After\n\nNew content.", restored.Content)
	require.Equal(t, "After", restored.Name)
	require.Equal(t, "After summary", restored.Summary)
	require.Equal(t, "https://example.com/users/alice", restored.AttributedTo)
	require.Equal(t, "markdown", restored.ContentFormat)
	require.Equal(t, []string{"cat-new"}, restored.CategoryIDs)
	require.NotNil(t, restored.SeriesID)
	require.Equal(t, "alice|series-new", *restored.SeriesID)
	require.NotNil(t, restored.FeaturedImage)
	require.Equal(t, "media-1", restored.FeaturedImage.MediaID)

	require.GreaterOrEqual(t, len(indexWriter.created), 1)
	require.GreaterOrEqual(t, len(indexWriter.deleted), 1)

	require.Len(t, revisionRepo.created, 2)
	require.Equal(t, revisionChangeTypeUpdate, revisionRepo.created[0].ChangeType)
	require.Contains(t, revisionRepo.created[0].ChangeSummary, "backup before restore")
	require.Equal(t, "# Before\n\nOld content.", revisionRepo.created[0].Content)

	require.Equal(t, revisionChangeTypeRestore, revisionRepo.created[1].ChangeType)
	require.Contains(t, revisionRepo.created[1].ChangeSummary, "restored from version")
	require.Equal(t, "# After\n\nNew content.", revisionRepo.created[1].Content)
}

func ptrString(value string) *string {
	return &value
}
