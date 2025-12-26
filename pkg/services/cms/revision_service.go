package cms

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
	"go.uber.org/zap"
)

const (
	revisionChangeTypeCreate  = "create"
	revisionChangeTypeUpdate  = "update"
	revisionChangeTypeRestore = "restore"
)

type articleRevisionMedia struct {
	ID          string `json:"id,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	CDNURL      string `json:"cdnUrl,omitempty"`
	Description string `json:"description,omitempty"`
	Blurhash    string `json:"blurhash,omitempty"`
	Width       int    `json:"width,omitempty"`
	Height      int    `json:"height,omitempty"`
	FileSize    int64  `json:"fileSize,omitempty"`
}

type articleRevisionMetadata struct {
	Name          string                `json:"name,omitempty"`
	Summary       string                `json:"summary,omitempty"`
	AttributedTo  string                `json:"attributedTo,omitempty"`
	Subtitle      string                `json:"subtitle,omitempty"`
	Excerpt       string                `json:"excerpt,omitempty"`
	ContentFormat string                `json:"contentFormat,omitempty"`
	FeaturedImage *articleRevisionMedia `json:"featuredImage,omitempty"`

	TableOfContents    []models.TOCEntry `json:"tableOfContents,omitempty"`
	ReadingTimeMinutes int               `json:"readingTimeMinutes,omitempty"`
	WordCount          int               `json:"wordCount,omitempty"`

	SeriesID    *string  `json:"seriesId,omitempty"`
	SeriesOrder *int     `json:"seriesOrder,omitempty"`
	CategoryIDs []string `json:"categoryIds,omitempty"`

	SEOTitle       string `json:"seoTitle,omitempty"`
	SEODescription string `json:"seoDescription,omitempty"`
	CanonicalURL   string `json:"canonicalUrl,omitempty"`
	OGImage        string `json:"ogImage,omitempty"`

	EditorNotes  string `json:"editorNotes,omitempty"`
	ReviewStatus string `json:"reviewStatus,omitempty"`
}

// RevisionService handles business logic for content revisions
type RevisionService struct {
	revisionRepo          *repositories.RevisionRepository
	articleRepo           *repositories.ArticleRepository
	seriesRepo            *repositories.SeriesRepository
	categoryRepo          *repositories.CategoryRepository
	maxRevisionsPerObject int
	logger                *zap.Logger
}

// NewRevisionService creates a new RevisionService
func NewRevisionService(
	revisionRepo *repositories.RevisionRepository,
	articleRepo *repositories.ArticleRepository,
	seriesRepo *repositories.SeriesRepository,
	categoryRepo *repositories.CategoryRepository,
	maxRevisionsPerObject int,
	logger *zap.Logger,
) *RevisionService {
	return &RevisionService{
		revisionRepo:          revisionRepo,
		articleRepo:           articleRepo,
		seriesRepo:            seriesRepo,
		categoryRepo:          categoryRepo,
		maxRevisionsPerObject: maxRevisionsPerObject,
		logger:                logger,
	}
}

// CreateRevision creates a new revision from an article state
func (s *RevisionService) CreateRevision(ctx context.Context, article *models.Article) (*models.Revision, error) {
	return s.createRevision(ctx, article, revisionChangeTypeUpdate, "")
}

func (s *RevisionService) createRevision(ctx context.Context, article *models.Article, changeType string, changeSummary string) (*models.Revision, error) {
	if article == nil {
		return nil, fmt.Errorf("article is required")
	}

	s.logger.Info("creating revision", zap.String("article_id", article.ID), zap.String("change_type", changeType))

	// Determine next version number
	// Ideally this should be transactional or based on a counter, but for now we count existing revisions
	revisions, err := s.revisionRepo.ListRevisions(ctx, article.ID, 1)
	nextVersion := 1
	if err == nil && len(revisions) > 0 {
		nextVersion = revisions[0].Version + 1
	}

	metadata, err := s.buildRevisionMetadata(article)
	if err != nil {
		return nil, err
	}

	contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(article.Content)))

	changedBy := strings.TrimSpace(article.AttributedTo)
	if claims, ok := ctx.Value(common.ContextKeyClaims).(common.Claims); ok && claims != nil {
		if username := strings.TrimSpace(claims.GetUsername()); username != "" {
			changedBy = username
		}
	}

	revision := &models.Revision{
		ObjectID:     article.ID,
		Version:      nextVersion,
		Content:      article.Content,
		ContentHash:  contentHash,
		MetadataJSON: metadata,
		ChangedBy:    changedBy,
		ChangeType:   revisionChangeTypeUpdate,
		CreatedAt:    time.Now(),
	}
	if normalized := strings.ToLower(strings.TrimSpace(changeType)); normalized != "" {
		revision.ChangeType = normalized
	}
	revision.ID = fmt.Sprintf("%s#%08d", revision.ObjectID, revision.Version)
	if strings.TrimSpace(changeSummary) != "" {
		revision.ChangeSummary = strings.TrimSpace(changeSummary)
	}

	if err := s.revisionRepo.CreateRevision(ctx, revision); err != nil {
		s.logger.Error("failed to create revision", zap.Error(err))
		return nil, err
	}

	s.trimRevisionsBestEffort(ctx, article.ID)

	return revision, nil
}

func (s *RevisionService) trimRevisionsBestEffort(ctx context.Context, objectID string) {
	maxRevisions := s.maxRevisionsPerObject
	if maxRevisions <= 0 {
		return
	}

	// Keep this bounded to avoid unbounded work on write paths.
	const maxDeletesPerCall = 25
	pk := fmt.Sprintf("OBJECT#%s#REVISION", objectID)

	for i := 0; i < maxDeletesPerCall; i++ {
		revisions, err := s.revisionRepo.ListRevisions(ctx, objectID, maxRevisions+1)
		if err != nil {
			s.logger.Warn("failed to enforce revision retention", zap.String("object_id", objectID), zap.Error(err))
			return
		}
		if len(revisions) <= maxRevisions {
			return
		}

		oldest := revisions[len(revisions)-1]
		if oldest == nil {
			return
		}

		if err := s.revisionRepo.Delete(ctx, pk, oldest.SK); err != nil {
			s.logger.Warn("failed to delete old revision",
				zap.String("object_id", objectID),
				zap.String("sk", oldest.SK),
				zap.Error(err))
			return
		}
	}
}

func (s *RevisionService) buildRevisionMetadata(article *models.Article) (string, error) {
	metadata := articleRevisionMetadata{
		Name:               article.Name,
		Summary:            article.Summary,
		AttributedTo:       article.AttributedTo,
		Subtitle:           article.Subtitle,
		Excerpt:            article.Excerpt,
		ContentFormat:      article.ContentFormat,
		TableOfContents:    append([]models.TOCEntry{}, article.TableOfContents...),
		ReadingTimeMinutes: article.ReadingTimeMinutes,
		WordCount:          article.WordCount,
		SeriesID:           article.SeriesID,
		SeriesOrder:        article.SeriesOrder,
		CategoryIDs:        append([]string{}, article.CategoryIDs...),
		SEOTitle:           article.SEOTitle,
		SEODescription:     article.SEODescription,
		CanonicalURL:       article.CanonicalURL,
		OGImage:            article.OGImage,
		EditorNotes:        article.EditorNotes,
		ReviewStatus:       article.ReviewStatus,
	}

	if article.FeaturedImage != nil {
		metadata.FeaturedImage = &articleRevisionMedia{
			ID:          article.FeaturedImage.MediaID,
			ContentType: article.FeaturedImage.ContentType,
			CDNURL:      article.FeaturedImage.CDNUrl,
			Description: article.FeaturedImage.Description,
			Blurhash:    article.FeaturedImage.Blurhash,
			Width:       article.FeaturedImage.Width,
			Height:      article.FeaturedImage.Height,
			FileSize:    article.FeaturedImage.FileSize,
		}
	}

	out, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// ListRevisions lists revisions for a given article
func (s *RevisionService) ListRevisions(ctx context.Context, objectID string, limit int) ([]*models.Revision, error) {
	return s.revisionRepo.ListRevisions(ctx, objectID, limit)
}

// GetRevision retrieves a specific revision
func (s *RevisionService) GetRevision(ctx context.Context, objectID string, version int) (*models.Revision, error) {
	return s.revisionRepo.GetRevision(ctx, objectID, version)
}

// RestoreRevision reverts an article to a specific revision
func (s *RevisionService) RestoreRevision(ctx context.Context, objectID string, version int) (*models.Article, error) {
	s.logger.Info("restoring revision", zap.String("object_id", objectID), zap.Int("version", version))

	// Get the revision
	revision, err := s.revisionRepo.GetRevision(ctx, objectID, version)
	if err != nil {
		return nil, err
	}

	// Get the current article
	article, err := s.articleRepo.GetArticle(ctx, objectID)
	if err != nil {
		return nil, err
	}

	before := *article
	before.CategoryIDs = append([]string{}, article.CategoryIDs...)

	// Record a revision of the CURRENT state before overwriting (safety net).
	if _, err := s.createRevision(ctx, article, revisionChangeTypeUpdate, fmt.Sprintf("backup before restore from version %d", version)); err != nil {
		s.logger.Warn("failed to record pre-restore backup revision", zap.Error(err))
	}

	// Update article with revision content
	article.Content = revision.Content

	// Restore metadata from JSON if available
	if revision.MetadataJSON != "" {
		var metadata articleRevisionMetadata
		if err := json.Unmarshal([]byte(revision.MetadataJSON), &metadata); err == nil {
			if strings.TrimSpace(metadata.Name) != "" {
				article.Name = metadata.Name
			}
			article.Summary = metadata.Summary
			if strings.TrimSpace(metadata.AttributedTo) != "" {
				article.AttributedTo = metadata.AttributedTo
			}
			article.Subtitle = metadata.Subtitle
			article.Excerpt = metadata.Excerpt
			article.ContentFormat = metadata.ContentFormat
			article.TableOfContents = append([]models.TOCEntry{}, metadata.TableOfContents...)
			article.ReadingTimeMinutes = metadata.ReadingTimeMinutes
			article.WordCount = metadata.WordCount
			article.SeriesID = metadata.SeriesID
			article.SeriesOrder = metadata.SeriesOrder
			article.CategoryIDs = append([]string{}, metadata.CategoryIDs...)
			article.SEOTitle = metadata.SEOTitle
			article.SEODescription = metadata.SEODescription
			article.CanonicalURL = metadata.CanonicalURL
			article.OGImage = metadata.OGImage
			article.EditorNotes = metadata.EditorNotes
			article.ReviewStatus = metadata.ReviewStatus

			if metadata.FeaturedImage == nil || strings.TrimSpace(metadata.FeaturedImage.ID) == "" {
				article.FeaturedImage = nil
			} else {
				article.FeaturedImage = &models.Media{
					MediaID:     metadata.FeaturedImage.ID,
					ContentType: metadata.FeaturedImage.ContentType,
					CDNUrl:      metadata.FeaturedImage.CDNURL,
					Description: metadata.FeaturedImage.Description,
					Blurhash:    metadata.FeaturedImage.Blurhash,
					Width:       metadata.FeaturedImage.Width,
					Height:      metadata.FeaturedImage.Height,
					FileSize:    metadata.FeaturedImage.FileSize,
				}
			}
		}
	}

	// Recompute derived enrichment fields for consistency (TOC, word count, reading time).
	enrichArticleContent(article)

	article.Updated = time.Now()
	article.UpdatedAt = time.Now()

	if err := s.articleRepo.UpdateArticle(ctx, article); err != nil {
		s.logger.Error("failed to update article for restore", zap.Error(err))
		return nil, err
	}

	db := s.articleRepo.GetDB()
	for _, entry := range cmsArticleIndexEntries(article) {
		if entry == nil {
			continue
		}
		if err := db.WithContext(ctx).Model(entry).Create(); err != nil {
			s.logger.Error("failed to upsert CMS article indexes on restore", zap.Error(err), zap.String("article_id", article.ID))
			return nil, err
		}
	}
	for _, entry := range cmsArticleIndexEntriesForRemovedGroups(&before, article) {
		if entry == nil {
			continue
		}
		if err := db.WithContext(ctx).Model(entry).Delete(); err != nil && !dynamormerrors.IsNotFound(err) {
			s.logger.Warn("failed to delete removed CMS article index entry on restore", zap.Error(err), zap.String("pk", entry.PK), zap.String("sk", entry.SK))
		}
	}
	cmsUpdateArticleCountsBestEffort(ctx, s.seriesRepo, s.categoryRepo, &before, article, s.logger)

	// Record a revision for the restored state (audit trail). This should not block the restore.
	if _, err := s.createRevision(ctx, article, revisionChangeTypeRestore, fmt.Sprintf("restored from version %d", version)); err != nil {
		s.logger.Warn("failed to record restored revision", zap.Error(err))
	}

	return article, nil
}
