package services

import (
	"context"
	"errors"
	"strings"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

// draftStatusFailed mirrors the CMS package's failed-status constant. A failed
// draft is the terminal state markDraftFailed writes after a publish error; its
// bound media mints may have survived a best-effort rollback.
const draftStatusFailed = "failed"

// orphanReconcilePageSize bounds each failed-draft enumeration page.
const orphanReconcilePageSize = 100

// orphanDraftEnumerator pages drafts in one status by GSI4SK cursor values.
type orphanDraftEnumerator interface {
	ListDraftsByStatusPaginated(ctx context.Context, status string, limit int, cursor string) ([]*models.Draft, string, error)
}

// orphanMediaLookup resolves one media record to its published-serving state.
type orphanMediaLookup interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}

// orphanArticleLookup resolves a draft's publish target article.
type orphanArticleLookup interface {
	GetArticle(ctx context.Context, articleID string) (*models.Article, error)
}

// cmsOrphanedPublishedMintSource enumerates durable published mints whose
// owning draft is terminally failed and no live article references them. It is
// the registry's wiring for media.Service.ReconcileOrphanedPublishedMedia.
type cmsOrphanedPublishedMintSource struct {
	drafts   orphanDraftEnumerator
	media    orphanMediaLookup
	articles orphanArticleLookup
	logger   *zap.Logger
}

// ListOrphanedPublishedMintIDs returns the deduplicated media IDs minted by
// failed drafts that are still published and unreferenced by any live article.
// Candidates whose reference state cannot be verified are skipped (fail
// closed): an orphan left behind stays alarmable and can be retried, while a
// live published asset is never unpublished.
func (src *cmsOrphanedPublishedMintSource) ListOrphanedPublishedMintIDs(ctx context.Context) ([]string, error) {
	cursor := ""
	seen := map[string]struct{}{}
	var orphaned []string
	for {
		drafts, next, err := src.drafts.ListDraftsByStatusPaginated(ctx, draftStatusFailed, orphanReconcilePageSize, cursor)
		if err != nil {
			return nil, err
		}
		for _, draft := range drafts {
			orphaned = append(orphaned, src.classifyDraftMints(ctx, draft, seen)...)
		}
		cursor = next
		if cursor == "" {
			break
		}
	}
	return orphaned, nil
}

// classifyDraftMints reports which of one failed draft's bound media mints are
// orphaned, marking each media ID as seen so a mint shared across drafts is
// reported once.
func (src *cmsOrphanedPublishedMintSource) classifyDraftMints(ctx context.Context, draft *models.Draft, seen map[string]struct{}) []string {
	if draft == nil || len(draft.EditorialMedia) == 0 {
		return nil
	}
	var orphaned []string
	for _, usage := range draft.EditorialMedia {
		mediaID := strings.TrimSpace(usage.MediaID)
		if mediaID == "" {
			continue
		}
		if _, dup := seen[mediaID]; dup {
			continue
		}
		seen[mediaID] = struct{}{}
		if src.isOrphanedMint(ctx, draft, mediaID) {
			orphaned = append(orphaned, mediaID)
		}
	}
	return orphaned
}

// isOrphanedMint reports whether one bound media mint is orphaned: still
// published and unreferenced by a live article. Unverifiable candidates fail
// closed (treated as live) so a live published asset is never unpublished.
func (src *cmsOrphanedPublishedMintSource) isOrphanedMint(ctx context.Context, draft *models.Draft, mediaID string) bool {
	media, err := src.media.GetMedia(ctx, mediaID)
	if err != nil {
		src.logger.Warn("orphan reconciliation skipped unverifiable media",
			zap.String("media_id", mediaID), zap.Error(err))
		return false
	}
	if media == nil || !media.IsPublished() {
		// Nothing minted (or already cleared): nothing to reconcile.
		return false
	}
	live, err := src.referencesLiveArticle(ctx, draft, mediaID, media)
	if err != nil {
		src.logger.Warn("orphan reconciliation skipped unverifiable article reference",
			zap.String("media_id", mediaID), zap.Error(err))
		return false
	}
	return !live
}

// referencesLiveArticle reports whether the draft's publish target article
// still references the minted asset by ID (featured image snapshot) or by its
// deterministic published URL (og image or inline content). Drafts without a
// publish target (the create-new-article path) never have a live reference.
func (src *cmsOrphanedPublishedMintSource) referencesLiveArticle(ctx context.Context, draft *models.Draft, mediaID string, media *models.Media) (bool, error) {
	objectID := ""
	if draft.ObjectID != nil {
		objectID = strings.TrimSpace(*draft.ObjectID)
	}
	if objectID == "" {
		return false, nil
	}
	article, err := src.articles.GetArticle(ctx, objectID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) || apperrors.HasCode(err, apperrors.CodeNotFound) {
			return false, nil
		}
		return false, err
	}
	if article == nil {
		return false, nil
	}
	if article.FeaturedImage != nil && strings.TrimSpace(article.FeaturedImage.MediaID) == mediaID {
		return true, nil
	}
	publishedURL := strings.TrimSpace(media.PublishedURL)
	if publishedURL == "" {
		return false, nil
	}
	return strings.Contains(article.OGImage, publishedURL) || strings.Contains(article.Content, publishedURL), nil
}

// ReconcileOrphanedPublishedMedia re-runs the best-effort unpublish for every
// durable published mint whose owning draft is terminally failed and no live
// article references it. It wires the media service's reconciliation to the
// CMS draft and article surfaces so the capability is exercisable (manually or
// from a future maintenance cron) without touching the publish hot path.
func (r *Registry) ReconcileOrphanedPublishedMedia(ctx context.Context) error {
	service := r.Media()
	if service == nil {
		return errors.New("media service is unavailable")
	}
	if r.storage == nil {
		return errors.New("storage is unavailable")
	}
	drafts := r.storage.Draft()
	if drafts == nil {
		return errors.New("draft repository is unavailable")
	}
	mediaRepo := r.storage.Media()
	if mediaRepo == nil {
		return errors.New("media repository is unavailable")
	}
	articles := r.Articles()
	if articles == nil {
		return errors.New("article service is unavailable")
	}
	logger := r.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	service.SetOrphanPublishedMintSource(&cmsOrphanedPublishedMintSource{
		drafts:   drafts,
		media:    mediaRepo,
		articles: articles,
		logger:   logger,
	})
	return service.ReconcileOrphanedPublishedMedia(ctx)
}

var (
	_ orphanDraftEnumerator = draftOrphanEnumerator(nil)
	_ orphanArticleLookup   = (*cms.ArticleService)(nil)
)

type draftOrphanEnumerator func(context.Context, string, int, string) ([]*models.Draft, string, error)

func (f draftOrphanEnumerator) ListDraftsByStatusPaginated(ctx context.Context, status string, limit int, cursor string) ([]*models.Draft, string, error) {
	return f(ctx, status, limit, cursor)
}
