package cms

import (
	"context"
	stdErrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/common"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	draftStatusDraft      = "draft"
	draftStatusScheduled  = "scheduled"
	draftStatusPublishing = "publishing"
	draftStatusPublished  = "published"
	draftStatusFailed     = "failed"
)

// PublishFailureReason values recorded by markDraftFailed on the interactive
// publish path. They are deliberately distinct from the scheduler's scheduled
// publish reason strings; both surfaces are alarmable by reason text.
const (
	draftPublishFailureApproval = "draft publish failed: approval required"
	draftPublishFailureMedia    = "draft publish failed: bound media unavailable"
	draftPublishFailureStorage  = "draft publish failed: storage unavailable"
	draftPublishFailureGeneric  = "draft publish failed"
)

type draftRepository interface {
	CreateDraft(ctx context.Context, draft *models.Draft) error
	UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error
	UpdateDraftEditorialMedia(ctx context.Context, authorID string, draft *models.Draft) error
	GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error)
	DeleteDraft(ctx context.Context, authorID, draftID string) error
}

type articleDraftPublisher interface {
	GetArticle(ctx context.Context, articleID string) (*models.Article, error)
	GetArticleBySlug(ctx context.Context, slug string) (*models.Article, error)
	CreateArticle(ctx context.Context, article *models.Article) error
	UpdateArticle(ctx context.Context, article *models.Article) error
}

type editorialMediaRepository interface {
	GetMedia(ctx context.Context, mediaID string) (*models.Media, error)
}

// EditorialPublishedMedia is the durable public serving minted for one bound
// asset at the publish transition. The URL serves the exact approved original
// bytes without expiring presignatures or temporary generator URLs.
type EditorialPublishedMedia struct {
	MediaID     string
	ContentHash string
	ContentType string
	FileSize    int64
	Width       int
	Height      int
	URL         string
	S3Key       string
	PublishedAt time.Time
}

// editorialPublishMinter transitions one internal editorial asset to durable
// public serving of its exact approved bytes. The media service implements it;
// the CMS service verifies the minted bytes match the approved revision digest.
// UnpublishEditorialMedia best-effort removes that serving when a publish fails
// before the article is committed.
type editorialPublishMinter interface {
	PublishEditorialMedia(ctx context.Context, mediaID string) (*EditorialPublishedMedia, error)
	UnpublishEditorialMedia(ctx context.Context, mediaID string) error
}

// DraftService handles business logic for drafts
type DraftService struct {
	draftRepo              draftRepository
	articleService         articleDraftPublisher
	domain                 string
	scheduling             bool
	logger                 *zap.Logger
	principalUsername      func(context.Context) (string, error)
	mediaRepo              editorialMediaRepository
	editorialPublishMinter editorialPublishMinter
}

// SetEditorialMediaRepository wires the media lookup used to enforce asset
// ownership and internal-state invariants at the CMS service boundary.
func (s *DraftService) SetEditorialMediaRepository(repo editorialMediaRepository) {
	s.mediaRepo = repo
}

// SetEditorialPublishMinter wires the durable published-serving transition used
// at the publish gate. Without it, drafts with bound media cannot publish.
func (s *DraftService) SetEditorialPublishMinter(minter editorialPublishMinter) {
	s.editorialPublishMinter = minter
}

// NewDraftService creates a new DraftService
func NewDraftService(draftRepo draftRepository, articleService *ArticleService, domain string, schedulingEnabled bool, logger *zap.Logger) *DraftService {
	return &DraftService{
		draftRepo:      draftRepo,
		articleService: articleService,
		domain:         strings.TrimSpace(domain),
		scheduling:     schedulingEnabled,
		logger:         logger,
	}
}

// SetPrincipalUsernameProvider supplies the instance's designated operator account.
// Generated drafts fail closed when this provider is absent or cannot identify one.
func (s *DraftService) SetPrincipalUsernameProvider(provider func(context.Context) (string, error)) {
	s.principalUsername = provider
}

// CreateDraft creates a new draft
func (s *DraftService) CreateDraft(ctx context.Context, draft *models.Draft) error {
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "create_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)

	s.logger.Info("creating draft", zap.String("title", draft.Title))

	if draft.CreatedAt.IsZero() {
		draft.CreatedAt = time.Now()
	}
	draft.UpdatedAt = time.Now()
	draft.LastSavedAt = time.Now()

	return s.draftRepo.CreateDraft(ctx, draft)
}

func validateDraftWriteAuthor(authorID string, draft *models.Draft) error {
	authorID = strings.TrimSpace(authorID)
	if authorID == "" {
		return stdErrors.New("authorID is required")
	}
	if draft == nil {
		return stdErrors.New("draft is required")
	}
	draftAuthor := strings.TrimSpace(draft.AuthorID)
	if draftAuthor == "" {
		return stdErrors.New("draft author is required")
	}
	if draftAuthor != authorID {
		return stdErrors.New("draft does not belong to author")
	}
	draft.AuthorID = draftAuthor
	return nil
}

func validateArticleDraftRenderable(draft *models.Draft) error {
	if draft == nil {
		return stdErrors.New("draft is required")
	}
	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return nil
	}
	rendered, err := cmsrender.RenderArticleContent(draft.Content, draft.ContentFormat)
	if err != nil {
		return err
	}
	draft.ContentFormat = rendered.SourceFormat
	return nil
}

// UpdateDraft updates an existing draft
func (s *DraftService) UpdateDraft(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "update_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)
	invalidateDraftReviewSummary(draft)
	now := time.Now()
	draft.UpdatedAt = now
	draft.LastSavedAt = now
	draft.AutosaveVersion++
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

// Autosave updates the draft content without changing its primary status
func (s *DraftService) Autosave(ctx context.Context, authorID string, draft *models.Draft) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	if err := validateArticleDraftRenderable(draft); err != nil {
		logCMSDraftRenderFailure(s.logger, "autosave_draft", draft, err)
		return err
	}
	cmsNormalizeDraftAttribution(draft)
	invalidateDraftReviewSummary(draft)
	s.logger.Debug("autosaving draft", zap.String("id", draft.ID))
	now := time.Now()
	draft.LastSavedAt = now
	draft.UpdatedAt = now
	draft.AutosaveVersion++
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

func invalidateDraftReviewSummary(draft *models.Draft) {
	if draft == nil {
		return
	}
	draft.ReviewedBy = ""
	draft.ReviewStatus = ""
	draft.EditorNotes = ""
}

// GetDraft retrieves a draft
func (s *DraftService) GetDraft(ctx context.Context, authorID, draftID string) (*models.Draft, error) {
	return s.draftRepo.GetDraft(ctx, authorID, draftID)
}

// SetEditorialMedia replaces the complete ordered media binding for a draft.
// Media changes deliberately do not extend the review content hash in M1; M2
// owns byte-bound approval and publish-gate semantics.
func (s *DraftService) SetEditorialMedia(ctx context.Context, authorID, draftID string, usages []models.DraftMediaUsage) (*models.Draft, error) {
	authorID = strings.TrimSpace(authorID)
	draftID = strings.TrimSpace(draftID)
	if s.mediaRepo == nil {
		return nil, stdErrors.New("editorial media repository is unavailable")
	}
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return nil, err
	}
	normalized, err := models.NormalizeDraftMediaUsages(usages)
	if err != nil {
		return nil, err
	}
	for _, usage := range normalized {
		media, getErr := s.mediaRepo.GetMedia(ctx, usage.MediaID)
		if getErr != nil {
			return nil, getErr
		}
		if media == nil || !strings.EqualFold(strings.TrimSpace(media.UserID), authorID) {
			return nil, stdErrors.New("editorial media does not belong to draft author")
		}
		if !media.IsInternalEditorial() || media.Provenance == nil || media.Provenance.ContentIntegrity != media.ContentHash {
			return nil, stdErrors.New("editorial media is not an integrity-bound internal asset")
		}
	}
	draft.EditorialMedia = normalized
	// M1 gives the association and content independent field-scoped write lanes:
	// content writers cannot write EditorialMedia, and this association writer
	// cannot write content from its stale draft snapshot. Media changes do not
	// change the content revision, review summary, or current content hash; M2
	// owns binding media bytes into those approval and publication invariants.
	draft.UpdatedAt = time.Now().UTC()
	if err := s.draftRepo.UpdateDraftEditorialMedia(ctx, authorID, draft); err != nil {
		return nil, err
	}
	return draft, nil
}

// PreviewDraft renders a draft through the same Article publication renderer used for ActivityPub and public HTML.
func (s *DraftService) PreviewDraft(ctx context.Context, authorID, draftID string) (cmsrender.RenderedArticleContent, error) {
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return cmsrender.RenderedArticleContent{}, err
	}
	rendered, err := RenderDraftPreview(draft)
	if err != nil {
		logCMSDraftRenderFailure(s.logger, "preview_draft", draft, err)
		return cmsrender.RenderedArticleContent{}, err
	}
	return rendered, nil
}

// RenderDraftPreview renders draft source through the canonical Article renderer.
func RenderDraftPreview(draft *models.Draft) (cmsrender.RenderedArticleContent, error) {
	if draft == nil {
		return cmsrender.RenderedArticleContent{}, stdErrors.New("draft is required")
	}
	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return cmsrender.RenderedArticleContent{}, stdErrors.New("only article drafts can be previewed")
	}
	rendered, err := cmsrender.RenderArticleContent(draft.Content, draft.ContentFormat)
	if err != nil {
		return cmsrender.RenderedArticleContent{}, err
	}
	draft.ContentFormat = rendered.SourceFormat
	return rendered, nil
}

// DeleteDraft deletes a draft
func (s *DraftService) DeleteDraft(ctx context.Context, authorID, draftID string) error {
	authorID = strings.TrimSpace(authorID)
	draftID = strings.TrimSpace(draftID)
	if authorID == "" {
		return stdErrors.New("authorID is required")
	}
	if draftID == "" {
		return stdErrors.New("draftID is required")
	}
	if repo, ok := s.draftRepo.(draftReviewRepository); ok && repo != nil {
		grants, err := repo.ListDraftReviewGrants(ctx, authorID, draftID)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		for _, grant := range grants {
			if grant == nil || grant.RevokedAt != nil {
				continue
			}
			grant.RevokedAt = &now
			if err := repo.RevokeDraftReviewGrant(ctx, grant); err != nil {
				return err
			}
		}
	}
	return s.draftRepo.DeleteDraft(ctx, authorID, draftID)
}

// ScheduleDraft schedules a draft for publishing
func (s *DraftService) ScheduleDraft(ctx context.Context, authorID, draftID string, scheduledAt time.Time) error {
	if !s.scheduling {
		return stdErrors.New("scheduled publishing is disabled")
	}

	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return err
	}

	approved, principalApproved, approval, approvalErr := s.draftReviewGateApprovals(ctx, authorID, draftID, draft)
	if approvalErr != nil {
		return approvalErr
	}
	if !approved {
		return ErrDraftReviewApprovalRequired
	}
	if !principalApproved {
		return ErrDraftReviewPrincipalApprovalRequired
	}
	// Bound media must serve the exact approved bytes before the draft is
	// scheduled. A withdrawal changes EditorialState, not the content digest,
	// so a hash-current approval would otherwise sail through scheduling and
	// the scheduler would later burn attempts and fail the draft with no
	// media reason. Fail closed here with the explicit media reason.
	if err := requireBoundMediaReady(approval); err != nil {
		return err
	}
	draft.ScheduledAt = &scheduledAt
	draft.Status = draftStatusScheduled
	draft.PublishFailureReason = ""
	draft.UpdatedAt = time.Now()
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

// PublishDraft converts a draft into an article
func (s *DraftService) PublishDraft(ctx context.Context, authorID, draftID string) (*models.Article, error) {
	return s.PublishDraftWithAttribution(ctx, authorID, draftID, "")
}

// PublishDraftWithAttribution converts a draft into an article, recording
// actedBy (a local actor URI) on the resulting article when the publish is
// performed by a caller acting under an active share grant. An empty actedBy
// preserves the draft's own attribution.
func (s *DraftService) PublishDraftWithAttribution(ctx context.Context, authorID, draftID, actedBy string) (*models.Article, error) {
	s.logger.Info("publishing draft", zap.String("draft_id", draftID))

	domain := s.domain
	if domain == "" {
		return nil, stdErrors.New("domain is required to publish drafts")
	}

	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return nil, err
	}

	if !strings.EqualFold(strings.TrimSpace(draft.ContentType), activitypub.ArticleType) {
		return nil, stdErrors.New("only article drafts can be published")
	}
	approved, principalApproved, approval, approvalErr := s.draftReviewGateApprovals(ctx, authorID, draftID, draft)
	if approvalErr != nil {
		return nil, approvalErr
	}
	if !approved {
		return nil, ErrDraftReviewApprovalRequired
	}
	if !principalApproved {
		return nil, ErrDraftReviewPrincipalApprovalRequired
	}
	// Required bound media must serve the exact approved bytes: missing,
	// not-ready, withdrawn, superseded, and unavailable assets block publish
	// with an explicit reason until the draft is re-reviewed and re-authorized.
	if err := requireBoundMediaReady(approval); err != nil {
		return nil, err
	}

	if s.isPublishedDraftCleanup(draft) {
		return s.cleanupPublishedDraft(ctx, authorID, draftID, domain, draft)
	}

	objectID, slug, err := s.resolveArticleDraftTarget(domain, draft)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	if err := s.transitionDraftToPublishing(ctx, authorID, draft, now); err != nil {
		return nil, err
	}

	// Minting runs only after the draft is committed to the publishing
	// transition, so a failed transition cannot leave minted state behind. A
	// mint failure rolls back the batch and marks the draft failed; a later
	// article-write failure rolls the batch back the same way. Both rollbacks
	// are best-effort: a compensating delete that fails is logged at error
	// level and left for ReconcileOrphanedPublishedMedia, so a failed publish
	// should not leave an orphaned IsPublished record or world-readable bytes
	// but the invariant is reconciled rather than guaranteed.
	mints, err := s.mintDraftBoundMedia(ctx, draft, approval.mediaDigests)
	if err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		return nil, err
	}

	actedBy = cmsNormalizeAttributionActorID(actedBy)
	if draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != "" {
		return s.publishDraftUpdateExistingArticle(ctx, authorID, draftID, domain, objectID, slug, draft, now, actedBy, mints)
	}

	return s.publishDraftCreateNewArticle(ctx, authorID, draftID, domain, objectID, slug, draft, now, actedBy, mints)
}

// requireBoundMediaReady blocks publication when any required bound asset cannot
// serve the exact approved bytes. The reasons were derived from the same media
// resolution instant that produced the approval hash.
func requireBoundMediaReady(approval *draftReviewApprovalState) error {
	if approval == nil || len(approval.mediaReasons) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s", ErrDraftReviewMediaRequired, strings.Join(approval.mediaReasons, ", "))
}

// mintDraftBoundMedia transitions every bound asset to durable published
// serving, verifying that the exact bytes minted match the digest bound into
// the approved revision hash. A mismatch (the media record changed between
// approval resolution and mint) fails closed. If any asset in the batch fails,
// the previously minted assets are rolled back best-effort so a failed publish
// should not leave an orphaned IsPublished record or world-readable bytes
// behind; a rollback that itself fails is logged at error level inside the
// media service and reconciled by ReconcileOrphanedPublishedMedia.
func (s *DraftService) mintDraftBoundMedia(ctx context.Context, draft *models.Draft, approvedDigests map[string]string) (map[string]EditorialPublishedMedia, error) {
	if len(draft.EditorialMedia) == 0 {
		return nil, nil
	}
	if s.editorialPublishMinter == nil {
		return nil, stdErrors.New("editorial media publish transition is unavailable")
	}
	mints := make(map[string]EditorialPublishedMedia, len(draft.EditorialMedia))
	mintedIDs := make([]string, 0, len(draft.EditorialMedia))
	for _, usage := range draft.EditorialMedia {
		mint, err := s.editorialPublishMinter.PublishEditorialMedia(ctx, usage.MediaID)
		if err != nil {
			s.rollbackDraftMints(ctx, mintedIDs)
			return nil, err
		}
		if mint == nil {
			s.rollbackDraftMints(ctx, mintedIDs)
			return nil, fmt.Errorf("%w: media %q returned no published serving", ErrDraftReviewMediaRequired, usage.MediaID)
		}
		expected := approvedDigests[usage.MediaID]
		if strings.TrimSpace(expected) == "" || strings.TrimSpace(mint.ContentHash) != expected {
			s.rollbackDraftMints(ctx, mintedIDs)
			return nil, fmt.Errorf("%w: media %q bytes changed after approval", ErrDraftReviewMediaRequired, usage.MediaID)
		}
		mints[usage.MediaID] = *mint
		mintedIDs = append(mintedIDs, usage.MediaID)
	}
	return mints, nil
}

// rollbackDraftMints best-effort removes durable published serving minted for
// the assets of a publish that failed before the article was committed. The
// deterministic published keys make the compensating deletes idempotent; a
// rollback failure is logged and never surfaced over the original publish error.
func (s *DraftService) rollbackDraftMints(ctx context.Context, mintedIDs []string) {
	for i := len(mintedIDs) - 1; i >= 0; i-- {
		if err := s.editorialPublishMinter.UnpublishEditorialMedia(ctx, mintedIDs[i]); err != nil {
			s.logger.Warn("failed to roll back editorial published serving",
				zap.String("media_id", mintedIDs[i]), zap.Error(err))
		}
	}
}

// mintedMediaIDs returns the media IDs of a mint batch for compensating rollback.
func mintedMediaIDs(mints map[string]EditorialPublishedMedia) []string {
	ids := make([]string, 0, len(mints))
	for id := range mints {
		ids = append(ids, id)
	}
	return ids
}

func (s *DraftService) isPublishedDraftCleanup(draft *models.Draft) bool {
	if draft == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(draft.Status), draftStatusPublished) {
		return false
	}
	return draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != ""
}

func (s *DraftService) resolveArticleDraftTarget(domain string, draft *models.Draft) (objectID string, slug string, err error) {
	objectID = ""
	if draft.ObjectID != nil {
		objectID = strings.TrimSpace(*draft.ObjectID)
	}

	slug = common.Slugify(draft.Slug)
	if slug == "" {
		slug = common.Slugify(draft.Title)
	}

	if slug == "" {
		return "", "", stdErrors.New("draft slug or title is required to publish")
	}

	if objectID == "" {
		objectID = common.GenerateObjectID(domain, "articles", slug)
	}

	if !strings.HasPrefix(objectID, "https://"+domain+"/") && !strings.HasPrefix(objectID, "http://"+domain+"/") {
		return "", "", stdErrors.New("draft objectId must be a local id")
	}

	if !strings.HasPrefix(objectID, "https://"+domain+"/articles/") &&
		!strings.HasPrefix(objectID, "http://"+domain+"/articles/") &&
		!strings.HasPrefix(objectID, "https://"+domain+"/objects/") &&
		!strings.HasPrefix(objectID, "http://"+domain+"/objects/") {
		return "", "", stdErrors.New("draft objectId must be a local article id")
	}

	return objectID, slug, nil
}

func (s *DraftService) cleanupPublishedDraft(ctx context.Context, authorID, draftID, domain string, draft *models.Draft) (*models.Article, error) {
	objectID := strings.TrimSpace(*draft.ObjectID)
	article, err := s.articleService.GetArticle(ctx, objectID)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(article.AttributedTo) != common.GenerateActorID(domain, authorID) {
		return nil, stdErrors.New("draft does not have permission to access this article")
	}

	if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
		s.logger.Warn("failed to delete published draft during cleanup", zap.Error(err))
	}

	return article, nil
}

func (s *DraftService) transitionDraftToPublishing(ctx context.Context, authorID string, draft *models.Draft, now time.Time) error {
	if err := validateDraftWriteAuthor(authorID, draft); err != nil {
		return err
	}
	draft.Status = draftStatusPublishing
	draft.ScheduledAt = nil
	draft.UpdatedAt = now
	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}

func (s *DraftService) publishDraftUpdateExistingArticle(ctx context.Context, authorID, draftID, domain, objectID, slug string, draft *models.Draft, now time.Time, actedBy string, mints map[string]EditorialPublishedMedia) (*models.Article, error) {
	article, err := s.articleService.GetArticle(ctx, objectID)
	if err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		s.rollbackDraftMints(ctx, mintedMediaIDs(mints))
		return nil, err
	}

	if strings.TrimSpace(article.AttributedTo) != common.GenerateActorID(domain, authorID) {
		err := stdErrors.New("draft does not have permission to update this article")
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		s.rollbackDraftMints(ctx, mintedMediaIDs(mints))
		return nil, err
	}

	article = cmsCloneArticleForDraftMutation(article)
	if title := strings.TrimSpace(draft.Title); title != "" {
		article.Name = title
	}
	if slug != "" {
		article.Slug = slug
	}
	article.Content = draft.Content
	if format := strings.TrimSpace(draft.ContentFormat); format != "" {
		article.ContentFormat = format
	}
	cmsApplyDraftAttributionToArticle(article, draft, domain, authorID, true)
	applyPublishedDraftMedia(article, draft, mints)
	if actedBy != "" {
		article.ActedBy = actedBy
	}
	article.UpdatedAt = now
	article.Updated = now

	if err := s.articleService.UpdateArticle(ctx, article); err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		s.rollbackDraftMints(ctx, mintedMediaIDs(mints))
		return nil, err
	}

	s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
	return article, nil
}

// applyPublishedDraftMedia wires the hero binding into Article.featuredImage and
// the social card binding into Article.ogImage using the durable published
// serving minted from the exact approved bytes. Inline positions remain
// structured (M1 contract); their assets are durably served at the media-record
// level so any future reference resolves to the approved bytes.
func applyPublishedDraftMedia(article *models.Article, draft *models.Draft, mints map[string]EditorialPublishedMedia) {
	if article == nil || draft == nil {
		return
	}
	for _, usage := range draft.EditorialMedia {
		mint, ok := mints[usage.MediaID]
		if !ok {
			continue
		}
		switch usage.Role {
		case models.EditorialMediaRoleHero:
			article.FeaturedImage = cmsFeaturedImageSnapshot(draft.AuthorID, usage, mint)
		case models.EditorialMediaRoleSocialCard:
			if url := strings.TrimSpace(mint.URL); url != "" {
				article.OGImage = url
			}
		}
	}
}

// cmsFeaturedImageSnapshot builds the published article's featured-image
// reference from the durable published serving. It is a serving snapshot, not
// the internal record: it carries the minted public URL and never re-enters the
// internal-media contract.
func cmsFeaturedImageSnapshot(owner string, usage models.DraftMediaUsage, mint EditorialPublishedMedia) *models.Media {
	now := time.Now().UTC()
	snapshot := &models.Media{
		MediaID:       mint.MediaID,
		Version:       "original",
		UserID:        strings.TrimSpace(owner),
		ContentType:   mint.ContentType,
		FileSize:      mint.FileSize,
		ContentHash:   mint.ContentHash,
		S3Key:         mint.S3Key,
		CDNUrl:        mint.URL,
		Width:         mint.Width,
		Height:        mint.Height,
		Visibility:    models.MediaVisibilityPublic,
		Status:        "ready",
		UploadedAt:    now,
		CreatedAt:     now,
		UpdatedAt:     now,
		Description:   usage.AltText,
		MediaCategory: models.DetermineMediaCategory(mint.ContentType),
	}
	if snapshot.MediaCategory == models.MediaCategoryUnknown {
		snapshot.MediaCategory = models.MediaCategoryImage
	}
	return snapshot
}

func cmsCloneArticleForDraftMutation(article *models.Article) *models.Article {
	if article == nil {
		return nil
	}
	cp := *article
	cp.To = append([]string{}, article.To...)
	cp.CC = append([]string{}, article.CC...)
	cp.BTo = append([]string{}, article.BTo...)
	cp.BCC = append([]string{}, article.BCC...)
	cp.CategoryIDs = append([]string{}, article.CategoryIDs...)
	cp.TableOfContents = append([]models.TOCEntry{}, article.TableOfContents...)
	if article.InReplyTo != nil {
		v := *article.InReplyTo
		cp.InReplyTo = &v
	}
	if article.SeriesID != nil {
		v := *article.SeriesID
		cp.SeriesID = &v
	}
	if article.SeriesOrder != nil {
		v := *article.SeriesOrder
		cp.SeriesOrder = &v
	}
	if article.FeaturedImage != nil {
		v := *article.FeaturedImage
		cp.FeaturedImage = &v
	}
	return &cp
}

func (s *DraftService) publishDraftCreateNewArticle(ctx context.Context, authorID, draftID, domain, objectID, slug string, draft *models.Draft, now time.Time, actedBy string, mints map[string]EditorialPublishedMedia) (*models.Article, error) {
	article := &models.Article{
		Object: models.Object{
			ID:           objectID,
			Type:         activitypub.ArticleType,
			Name:         draft.Title,
			Content:      draft.Content,
			AttributedTo: common.GenerateActorID(domain, authorID),
			Published:    now,
			Updated:      now,
			CreatedAt:    now,
		},
		Slug:          slug,
		ContentFormat: draft.ContentFormat,
		UpdatedAt:     now,
	}
	cmsApplyDraftAttributionToArticle(article, draft, domain, authorID, false)
	applyPublishedDraftMedia(article, draft, mints)
	if actedBy != "" {
		article.ActedBy = actedBy
	}

	if err := s.articleService.CreateArticle(ctx, article); err != nil {
		s.markDraftFailed(ctx, authorID, draft, draftID, err)
		s.rollbackDraftMints(ctx, mintedMediaIDs(mints))
		return nil, err
	}

	s.deleteDraftAfterPublish(ctx, draft, authorID, draftID, objectID)
	return article, nil
}

func (s *DraftService) deleteDraftAfterPublish(ctx context.Context, draft *models.Draft, authorID, draftID, objectID string) {
	if err := s.draftRepo.DeleteDraft(ctx, authorID, draftID); err != nil {
		s.logger.Warn("failed to delete draft after publish", zap.Error(err))
		draft.Status = draftStatusPublished
		draft.ObjectID = &objectID
		draft.UpdatedAt = time.Now()
		_ = s.draftRepo.UpdateDraft(ctx, authorID, draft)
	}
}

// classifyDraftPublishFailureReason buckets a failed interactive publish into a
// reason operators can alarm on: approval (review gate), media (a required
// bound asset cannot serve the exact approved bytes), or storage (durable
// write/read failure). The scheduler's scheduled-path classification is
// intentionally unchanged.
func classifyDraftPublishFailureReason(err error) string {
	if err == nil {
		return draftPublishFailureGeneric
	}
	if stdErrors.Is(err, ErrDraftReviewApprovalRequired) ||
		stdErrors.Is(err, ErrDraftReviewPrincipalApprovalRequired) {
		return draftPublishFailureApproval
	}
	if stdErrors.Is(err, ErrDraftReviewMediaRequired) || apperrors.HasCategory(err, apperrors.CategoryMedia) {
		return draftPublishFailureMedia
	}
	if apperrors.HasCategory(err, apperrors.CategoryStorage) {
		return draftPublishFailureStorage
	}
	return draftPublishFailureGeneric
}

func (s *DraftService) markDraftFailed(ctx context.Context, authorID string, draft *models.Draft, draftID string, err error) {
	s.logger.Warn("draft publish failed", zap.String("draft_id", draftID), zap.Error(err))
	draft.Status = draftStatusFailed
	draft.PublishFailureReason = classifyDraftPublishFailureReason(err)
	draft.UpdatedAt = time.Now()
	if updateErr := s.draftRepo.UpdateDraft(ctx, authorID, draft); updateErr != nil {
		// The failed status is best-effort: if the marker write itself fails,
		// the failure is logged and the original publish error still surfaces.
		s.logger.Warn("failed to mark draft failed",
			zap.String("draft_id", draftID),
			zap.String("author_id", authorID),
			zap.Error(updateErr))
	}
}

// CancelScheduledDraft cancels a scheduled draft publish.
func (s *DraftService) CancelScheduledDraft(ctx context.Context, authorID, draftID string) error {
	draft, err := s.draftRepo.GetDraft(ctx, authorID, draftID)
	if err != nil {
		return err
	}

	draft.ScheduledAt = nil
	draft.Status = draftStatusDraft
	draft.UpdatedAt = time.Now()

	return s.draftRepo.UpdateDraft(ctx, authorID, draft)
}
