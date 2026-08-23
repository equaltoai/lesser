package services

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
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

// orphanReconcileAuthorScanPageSize bounds each page of the cross-article
// reference scan over the owning author's live articles.
const orphanReconcileAuthorScanPageSize = 100

// orphanReconcileAuthorScanMaxPages bounds the cross-article reference scan so
// reconciliation stays bounded even for an author with a very large article
// set. A candidate whose reference state cannot be verified within the budget
// fails closed (treated as referenced) and is never unpublished.
const orphanReconcileAuthorScanMaxPages = 10

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

// orphanArticleEnumerator pages an author's live articles (CMS author index) so
// the cross-article reference check can reach an asset's serving even when the
// failed draft was a create-path publish with no target article.
type orphanArticleEnumerator interface {
	ListArticlesByAuthorPaginated(ctx context.Context, authorActorID string, limit int, cursor string) ([]*models.Article, string, error)
}

// cmsOrphanedPublishedMintSource enumerates durable published mints whose
// owning draft is terminally failed and no live article references them. It is
// the registry's wiring for media.Service.ReconcileOrphanedPublishedMedia.
type cmsOrphanedPublishedMintSource struct {
	drafts          orphanDraftEnumerator
	media           orphanMediaLookup
	articles        orphanArticleLookup
	articlesByAuthor orphanArticleEnumerator
	domain          string
	logger          *zap.Logger
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

// referencesLiveArticle reports whether any live article references the minted
// asset by ID (featured image snapshot) or by its deterministic published URL
// (og image or inline content). The check covers the failed draft's own publish
// target (the update-existing-article path) and, when an article enumerator and
// domain are wired, the owning author's live article set: only the owning
// author's drafts can bind the asset, so the first successful publish that
// lives on the asset is reachable through that author's article index even when
// the failed draft was a create-path publish with no target article. The ACTUAL
// guarantee is that an asset referenced by the failed draft's own target
// article or by any live article of the owning author is never unpublished;
// candidates whose reference state cannot be verified (enumerator or domain
// unwired, scan budget exceeded, lookup error) fail closed and are treated as
// referenced so a live published asset is never unpublished.
func (src *cmsOrphanedPublishedMintSource) referencesLiveArticle(ctx context.Context, draft *models.Draft, mediaID string, media *models.Media) (bool, error) {
	publishedURL := strings.TrimSpace(media.PublishedURL)
	// Fast path: the update-existing-article draft's own target article. A
	// missing target falls through to the cross-article scan below.
	if draft != nil && draft.ObjectID != nil {
		objectID := strings.TrimSpace(*draft.ObjectID)
		if objectID != "" {
			article, err := src.articles.GetArticle(ctx, objectID)
			if err != nil {
				if errors.Is(err, storage.ErrNotFound) || apperrors.HasCode(err, apperrors.CodeNotFound) {
					// Target article gone: nothing referenced there.
				} else {
					return false, err
				}
			} else if article != nil && articleReferencesMedia(article, mediaID, publishedURL) {
				return true, nil
			}
		}
	}
	// Cross-article check: a bounded scan of the owning author's live articles.
	// Without the enumerator or the domain the reference state cannot be
	// verified, so fail closed rather than risk unpublishing a live asset.
	if src.articlesByAuthor == nil || strings.TrimSpace(src.domain) == "" {
		return true, nil
	}
	actorID := common.GenerateActorID(strings.TrimSpace(src.domain), strings.TrimSpace(draft.AuthorID))
	if actorID == "" {
		return true, nil
	}
	cursor := ""
	for page := 0; page < orphanReconcileAuthorScanMaxPages; page++ {
		articles, next, err := src.articlesByAuthor.ListArticlesByAuthorPaginated(ctx, actorID, orphanReconcileAuthorScanPageSize, cursor)
		if err != nil {
			return false, err
		}
		for _, article := range articles {
			if articleReferencesMedia(article, mediaID, publishedURL) {
				return true, nil
			}
		}
		cursor = next
		if cursor == "" {
			break
		}
	}
	if cursor != "" {
		// The author's article set exceeds the scan budget: the reference state
		// cannot be verified within the bound. Fail closed.
		return true, nil
	}
	return false, nil
}

// articleReferencesMedia reports whether one article references a minted asset
// by its featured-image snapshot ID or by its durable published URL (og image
// or inline content).
func articleReferencesMedia(article *models.Article, mediaID, publishedURL string) bool {
	if article == nil {
		return false
	}
	if article.FeaturedImage != nil && strings.TrimSpace(article.FeaturedImage.MediaID) == mediaID {
		return true
	}
	if publishedURL == "" {
		return false
	}
	return strings.Contains(article.OGImage, publishedURL) || strings.Contains(article.Content, publishedURL)
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
	articleRepo := r.storage.Article()
	if articleRepo == nil {
		return errors.New("article repository is unavailable")
	}
	logger := r.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	service.SetOrphanPublishedMintSource(&cmsOrphanedPublishedMintSource{
		drafts:           drafts,
		media:            mediaRepo,
		articles:         articles,
		articlesByAuthor: articleRepo,
		domain:           r.getCMSDomainName(),
		logger:           logger,
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
