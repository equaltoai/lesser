package graph

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/services/cms"
	"github.com/equaltoai/lesser/pkg/services/media"
	"github.com/equaltoai/lesser/pkg/storage/core"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

func (r *Resolver) cmsStorage() core.RepositoryStorage {
	if r == nil {
		return nil
	}
	if !r.cmsLongFormEnabled() {
		return nil
	}
	if r.Storage != nil {
		return r.Storage
	}
	if r.Registry != nil {
		return r.Registry.GetStorage()
	}
	return nil
}

func (r *Resolver) convertCMSDraft(ctx context.Context, draft *models.Draft) *model.Draft {
	if draft == nil {
		return nil
	}

	author := r.resolveActorByID(ctx, draft.AuthorID)
	contentType := cmsObjectTypeFromStorage(draft.ContentType)
	contentFormat := cmsContentFormatFromStorage(draft.ContentFormat)
	status := cmsDraftStatusFromStorage(draft.Status)

	var scheduledAt *model.Time
	if draft.ScheduledAt != nil && !draft.ScheduledAt.IsZero() {
		t := model.Time(*draft.ScheduledAt)
		scheduledAt = &t
	}

	var objectID *string
	if draft.ObjectID != nil && strings.TrimSpace(*draft.ObjectID) != "" {
		v := strings.TrimSpace(*draft.ObjectID)
		objectID = &v
	}

	bindings := r.resolveCMSEditorialMediaBindings(ctx, draft)
	return &model.Draft{
		ID:              draft.ID,
		AuthorID:        strings.TrimSpace(draft.AuthorID),
		Author:          author,
		ContentType:     contentType,
		Title:           cmsOptionalString(strings.TrimSpace(draft.Title)),
		Slug:            cmsOptionalString(strings.TrimSpace(draft.Slug)),
		Content:         draft.Content,
		ContentFormat:   contentFormat,
		Status:          status,
		ScheduledAt:     scheduledAt,
		ObjectID:        objectID,
		GeneratedBy:     r.resolveActorByID(ctx, draft.GeneratedBy),
		ReviewedBy:      r.resolveActorByID(ctx, draft.ReviewedBy),
		ActedBy:         r.resolveActorByID(ctx, draft.ActedBy),
		ReviewVerdict:   cmsDraftReviewVerdict(draft.ReviewStatus),
		EditorialMedia:  r.convertCMSEditorialMediaBindings(ctx, bindings, false),
		ContentHash:     cms.DraftReviewContentHashWithMedia(draft, cmsDraftMediaDigests(bindings)),
		Revision:        draft.AutosaveVersion,
		AutosaveVersion: draft.AutosaveVersion,
		LastSavedAt:     model.Time(draft.LastSavedAt),
		CreatedAt:       model.Time(draft.CreatedAt),
		UpdatedAt:       model.Time(draft.UpdatedAt),
	}
}

// cmsDraftMediaDigests derives the canonical content-digest map for a draft's
// resolved media bindings, mirroring the service-level review hash binding.
func cmsDraftMediaDigests(bindings []cms.DraftEditorialMediaBinding) map[string]string {
	digests := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if binding.Media != nil && strings.TrimSpace(binding.Media.ContentHash) != "" {
			digests[binding.Usage.MediaID] = binding.Media.ContentHash
		}
	}
	return digests
}

func cmsDraftReviewVerdict(value string) *model.DraftReviewVerdict {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(model.DraftReviewVerdictApproved):
		verdict := model.DraftReviewVerdictApproved
		return &verdict
	case string(model.DraftReviewVerdictChangesRequested):
		verdict := model.DraftReviewVerdictChangesRequested
		return &verdict
	default:
		return nil
	}
}

func (r *Resolver) convertCMSDraftPreview(
	ctx context.Context,
	draft *models.Draft,
	bindings []cms.DraftEditorialMediaBinding,
	rendered cmsrender.RenderedArticleContent,
	renderErr error,
	includeAccessUrls bool,
) (*model.DraftPreview, error) {
	if draft == nil {
		return nil, nil
	}

	sourceFormat := strings.TrimSpace(draft.ContentFormat)
	sourceBytes := len(draft.Content)
	renderedBytes := 0
	var renderedHTML *string
	errorsList := []string{}

	if renderErr == nil {
		if strings.TrimSpace(rendered.SourceFormat) != "" {
			sourceFormat = rendered.SourceFormat
		}
		sourceBytes = rendered.SourceBytes
		renderedBytes = rendered.RenderedBytes
		html := rendered.HTML
		renderedHTML = &html
	} else {
		errorsList = append(errorsList, renderErr.Error())
	}

	if sourceFormat == "" {
		sourceFormat = cmsrender.FormatMarkdown
	}

	// URL minting is scoped: ordinary preview reads do not mint a short-lived
	// S3 URL per bound asset; only callers that explicitly request
	// includeAccessUrls (or use the exact-asset draftEditorialMediaAccess lane)
	// pay the mint cost.
	var editorialMedia []*model.EditorialMediaUsage
	if includeAccessUrls {
		editorialMedia = r.convertCMSEditorialMediaBindingsWithAccess(ctx, bindings)
	} else {
		editorialMedia = r.convertCMSEditorialMediaBindings(ctx, bindings, false)
	}
	return &model.DraftPreview{
		DraftID:        draft.ID,
		Success:        renderErr == nil,
		RenderedHTML:   renderedHTML,
		SourceFormat:   sourceFormat,
		SourceBytes:    sourceBytes,
		RenderedBytes:  renderedBytes,
		Errors:         errorsList,
		EditorialMedia: editorialMedia,
	}, nil
}

func (r *Resolver) resolveCMSEditorialMediaBindings(ctx context.Context, draft *models.Draft) []cms.DraftEditorialMediaBinding {
	if draft == nil {
		return nil
	}
	bindings := make([]cms.DraftEditorialMediaBinding, 0, len(draft.EditorialMedia))
	store := r.cmsStorage()
	for _, usage := range draft.EditorialMedia {
		binding := cms.DraftEditorialMediaBinding{Usage: usage}
		if store != nil && store.Media() != nil {
			media, err := store.Media().GetMedia(ctx, usage.MediaID)
			if err == nil && media != nil && strings.EqualFold(strings.TrimSpace(media.UserID), strings.TrimSpace(draft.AuthorID)) {
				binding.Media = media
			}
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func (r *Resolver) convertCMSEditorialMediaBindings(
	ctx context.Context,
	bindings []cms.DraftEditorialMediaBinding,
	includeAccess bool,
) []*model.EditorialMediaUsage {
	out := make([]*model.EditorialMediaUsage, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, r.convertCMSEditorialMediaBinding(ctx, binding, includeAccess, nil))
	}
	return out
}

func (r *Resolver) convertCMSEditorialMediaBindingsWithAccess(
	ctx context.Context,
	bindings []cms.DraftEditorialMediaBinding,
) []*model.EditorialMediaUsage {
	out := make([]*model.EditorialMediaUsage, 0, len(bindings))
	for _, binding := range bindings {
		var access *media.EditorialAccess
		if binding.Media != nil && binding.Media.IsInternalEditorial() {
			issued, err := r.Registry.Media().IssueEditorialAccess(ctx, binding.Media.MediaID)
			if err != nil {
				if r.Logger != nil {
					r.Logger.Warn("failed to issue editorial media access",
						zap.String("media_id", binding.Media.MediaID),
						zap.Error(err))
				}
			} else {
				access = issued
			}
		}
		out = append(out, r.convertCMSEditorialMediaBinding(ctx, binding, true, access))
	}
	return out
}

func (r *Resolver) convertCMSEditorialMediaBinding(
	ctx context.Context,
	binding cms.DraftEditorialMediaBinding,
	includeAccess bool,
	access *media.EditorialAccess,
) *model.EditorialMediaUsage {
	usage := binding.Usage
	out := &model.EditorialMediaUsage{
		MediaID:        usage.MediaID,
		Role:           model.EditorialMediaRole(strings.ToUpper(string(usage.Role))),
		InlinePosition: usage.InlinePosition,
		Caption:        cmsOptionalString(usage.Caption),
		CreditLine:     cmsOptionalString(usage.CreditLine),
		AltText:        cmsOptionalString(usage.AltText),
		Focus:          cmsOptionalString(usage.Focus),
		State:          model.EditorialMediaStateMissing,
	}
	mediaRecord := binding.Media
	if mediaRecord == nil {
		return out
	}
	out.Width = cmsOptionalPositiveInt(mediaRecord.Width)
	out.Height = cmsOptionalPositiveInt(mediaRecord.Height)
	out.MimeType = cmsOptionalString(mediaRecord.ContentType)
	out.ContentHash = cmsOptionalString(mediaRecord.ContentHash)
	out.EffectiveAltText = cmsOptionalString(usage.AltText)
	if out.EffectiveAltText == nil {
		out.EffectiveAltText = cmsOptionalString(mediaRecord.Description)
	}
	if lifecycleReason := cmsEditorialLifecycleReason(mediaRecord); lifecycleReason != "" {
		switch lifecycleReason {
		case cms.DraftReviewMediaReasonWithdrawn:
			out.State = model.EditorialMediaStateWithdrawn
		case cms.DraftReviewMediaReasonSuperseded:
			out.State = model.EditorialMediaStateSuperseded
		case cms.DraftReviewMediaReasonUnavailable:
			out.State = model.EditorialMediaStateUnavailable
		default:
			out.State = model.EditorialMediaStateRejected
		}
	} else {
		switch {
		case !mediaRecord.IsInternalEditorial(), mediaRecord.Provenance == nil,
			mediaRecord.Provenance.ContentIntegrity != mediaRecord.ContentHash, mediaRecord.IsFailed():
			out.State = model.EditorialMediaStateRejected
		case mediaRecord.IsReady():
			out.State = model.EditorialMediaStateReady
		default:
			out.State = model.EditorialMediaStateProcessing
		}
	}
	out.PublishedURL = cmsOptionalString(mediaRecord.PublishedURL)
	if mediaRecord.PublishedAt != nil {
		publishedAt := model.Time(*mediaRecord.PublishedAt)
		out.PublishedAt = &publishedAt
	}
	out.Provenance = r.convertCMSEditorialMediaProvenance(ctx, mediaRecord.Provenance)
	if includeAccess && access != nil {
		out.AccessURL = cmsOptionalString(access.URL)
		expiresAt := model.Time(access.ExpiresAt)
		out.AccessExpiresAt = &expiresAt
	}
	return out
}

// cmsEditorialLifecycleReason maps an internal asset's explicit editorial
// lifecycle onto the shared blocking-reason vocabulary; the empty/available
// lifecycle is servable.
func cmsEditorialLifecycleReason(media *models.Media) string {
	if media == nil {
		return ""
	}
	switch models.EditorialLifecycle(strings.ToLower(strings.TrimSpace(string(media.EditorialState)))) {
	case "", models.EditorialLifecycleAvailable:
		return ""
	case models.EditorialLifecycleWithdrawn:
		return cms.DraftReviewMediaReasonWithdrawn
	case models.EditorialLifecycleSuperseded:
		return cms.DraftReviewMediaReasonSuperseded
	default:
		return cms.DraftReviewMediaReasonUnavailable
	}
}

func (r *Resolver) convertCMSEditorialMediaProvenance(ctx context.Context, provenance *models.MediaProvenance) *model.EditorialMediaProvenance {
	if provenance == nil {
		return nil
	}
	out := &model.EditorialMediaProvenance{
		Origin:             model.EditorialMediaOrigin(strings.ToUpper(string(provenance.Origin))),
		Tool:               cmsOptionalString(provenance.Tool),
		ResponsibleActorID: provenance.ResponsibleActor,
		ResponsibleActor:   r.resolveActorByID(ctx, provenance.ResponsibleActor),
		SourceReferences:   append([]string(nil), provenance.SourceReferences...),
		RightsLicenseNotes: cmsOptionalString(provenance.RightsLicenseNotes),
		RecordedAt:         model.Time(provenance.RecordedAt),
		ContentIntegrity:   provenance.ContentIntegrity,
	}
	if provenance.CreatedAt != nil {
		value := model.Time(*provenance.CreatedAt)
		out.CreatedAt = &value
	}
	if provenance.UpdatedAt != nil {
		value := model.Time(*provenance.UpdatedAt)
		out.UpdatedAt = &value
	}
	return out
}

func cmsOptionalPositiveInt(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func (r *Resolver) convertCMSRevision(ctx context.Context, revision *models.Revision) *model.Revision {
	if revision == nil {
		return nil
	}

	id := strings.TrimSpace(revision.ID)
	if id == "" {
		id = fmt.Sprintf("%s#%d", revision.ObjectID, revision.Version)
	}

	changedBy := r.resolveActorByID(ctx, revision.ChangedBy)
	changeType := cmsChangeTypeFromStorage(revision.ChangeType)

	var metadataJSON *string
	if strings.TrimSpace(revision.MetadataJSON) != "" {
		v := revision.MetadataJSON
		metadataJSON = &v
	}

	var changeSummary *string
	if strings.TrimSpace(revision.ChangeSummary) != "" {
		v := revision.ChangeSummary
		changeSummary = &v
	}

	return &model.Revision{
		ID:            id,
		ObjectID:      revision.ObjectID,
		Version:       revision.Version,
		Content:       revision.Content,
		MetadataJSON:  metadataJSON,
		ChangedBy:     changedBy,
		ChangeSummary: changeSummary,
		ChangeType:    changeType,
		GeneratedBy:   r.resolveActorByID(ctx, revision.GeneratedBy),
		ReviewedBy:    r.resolveActorByID(ctx, revision.ReviewedBy),
		PublishedBy:   r.resolveActorByID(ctx, revision.PublishedBy),
		CreatedAt:     model.Time(revision.CreatedAt),
	}
}

func (r *Resolver) convertCMSSeries(ctx context.Context, series *models.Series) *model.Series {
	if series == nil {
		return nil
	}

	author := r.resolveActorByID(ctx, series.AuthorID)
	description := cmsOptionalString(strings.TrimSpace(series.Description))
	cover := cmsOptionalString(strings.TrimSpace(series.CoverImage))

	return &model.Series{
		ID:            cmsSeriesGraphQLID(series.AuthorID, series.ID),
		Author:        author,
		Title:         series.Title,
		Description:   description,
		Slug:          series.Slug,
		CoverImageURL: cover,
		IsComplete:    series.IsComplete,
		ArticleCount:  series.ArticleCount,
		CreatedAt:     model.Time(series.CreatedAt),
		UpdatedAt:     model.Time(series.UpdatedAt),
	}
}

func (r *Resolver) convertCMSCategory(ctx context.Context, category *models.Category, includeRelations bool) *model.Category {
	if category == nil {
		return nil
	}

	description := cmsOptionalString(strings.TrimSpace(category.Description))
	color := cmsOptionalString(strings.TrimSpace(category.Color))

	result := &model.Category{
		ID:           category.ID,
		Name:         category.Name,
		Slug:         category.Slug,
		Description:  description,
		ArticleCount: category.ArticleCount,
		Order:        category.Order,
		Color:        color,
		CreatedAt:    model.Time(category.CreatedAt),
		UpdatedAt:    model.Time(category.UpdatedAt),
		Children:     []*model.Category{},
	}

	if !includeRelations {
		return result
	}

	store := r.cmsStorage()
	if store == nil || store.Category() == nil {
		return result
	}

	if category.ParentID != nil && strings.TrimSpace(*category.ParentID) != "" {
		parent, err := store.Category().GetCategory(ctx, strings.TrimSpace(*category.ParentID))
		if err == nil {
			result.Parent = r.convertCMSCategory(ctx, parent, false)
		}
	}

	children, err := store.Category().ListCategories(ctx, &category.ID, 500)
	if err == nil && len(children) > 0 {
		result.Children = make([]*model.Category, 0, len(children))
		for _, child := range children {
			result.Children = append(result.Children, r.convertCMSCategory(ctx, child, false))
		}
	}

	return result
}

func (r *Resolver) convertCMSArticle(ctx context.Context, article *models.Article, includeRelations bool) *model.Article {
	if article == nil {
		return nil
	}

	wordCount := article.WordCount
	if wordCount == 0 {
		wordCount = cmsEstimateWordCount(article.Content)
	}

	readingMinutes := article.ReadingTimeMinutes
	if readingMinutes == 0 {
		readingMinutes = cmsEstimateReadingMinutes(wordCount)
	}

	toc := make([]*model.TOCEntry, 0, len(article.TableOfContents))
	for _, entry := range article.TableOfContents {
		toc = append(toc, &model.TOCEntry{
			ID:    entry.ID,
			Level: entry.Level,
			Text:  entry.Text,
		})
	}

	slug := strings.TrimSpace(article.Slug)
	if slug == "" {
		slug = cmsExtractSlugFromURL(article.ID)
	}

	var editorNotes *string
	var reviewStatus *string
	if r.canViewCMSPrivateAttribution(ctx, article.AttributedTo) {
		editorNotes = cmsOptionalString(strings.TrimSpace(article.EditorNotes))
		reviewStatus = cmsOptionalString(strings.TrimSpace(article.ReviewStatus))
	}

	result := &model.Article{
		ID:       article.ID,
		Slug:     slug,
		AuthorID: strings.TrimSpace(article.AttributedTo),
		Author:   r.resolveActorByID(ctx, article.AttributedTo),

		Title:    article.Name,
		Subtitle: cmsOptionalString(strings.TrimSpace(article.Subtitle)),
		Excerpt:  cmsOptionalString(strings.TrimSpace(article.Excerpt)),

		Content:          article.Content,
		ContentFormat:    cmsContentFormatFromStorage(article.ContentFormat),
		RawContentFormat: article.ContentFormat,

		FeaturedImage:   r.convertMediaToGraphQL(article.FeaturedImage),
		TableOfContents: toc,

		ReadingTimeMinutes: readingMinutes,
		WordCount:          wordCount,

		SeriesOrder: article.SeriesOrder,
		Categories:  []*model.Category{},

		SEOTitle:       cmsOptionalString(strings.TrimSpace(article.SEOTitle)),
		SEODescription: cmsOptionalString(strings.TrimSpace(article.SEODescription)),
		CanonicalURL:   cmsOptionalString(strings.TrimSpace(article.CanonicalURL)),
		OGImage:        cmsOptionalString(strings.TrimSpace(article.OGImage)),

		EditorNotes:  editorNotes,
		ReviewStatus: reviewStatus,

		// CSR-049: generatedBy / reviewedBy / publishedBy / actedBy are private CMS
		// workflow attribution actors distinct from the public Author (attributedTo)
		// byline. Only resolve them when the viewer is authenticated and is either
		// the article author or an instance admin. Public viewers see nil for these fields.
		GeneratedBy: r.resolveCMSPrivateAttributionActor(ctx, article.AttributedTo, article.GeneratedBy),
		ReviewedBy:  r.resolveCMSPrivateAttributionActor(ctx, article.AttributedTo, article.ReviewedBy),
		PublishedBy: r.resolveCMSPrivateAttributionActor(ctx, article.AttributedTo, article.PublishedBy),
		ActedBy:     r.resolveCMSPrivateAttributionActor(ctx, article.AttributedTo, article.ActedBy),

		PublishedAt: model.Time(article.Published),
		CreatedAt:   model.Time(article.CreatedAt),
		UpdatedAt:   model.Time(article.UpdatedAt),
	}

	if !includeRelations {
		return result
	}

	store := r.cmsStorage()
	if store == nil {
		return result
	}

	if article.SeriesID != nil && strings.TrimSpace(*article.SeriesID) != "" && store.Series() != nil {
		authorID, seriesID, ok := parseSeriesGraphQLID(strings.TrimSpace(*article.SeriesID))
		if ok {
			series, err := store.Series().GetSeries(ctx, authorID, seriesID)
			if err == nil {
				result.Series = r.convertCMSSeries(ctx, series)
			}
		}
	}

	if len(article.CategoryIDs) > 0 && store.Category() != nil {
		for _, categoryID := range article.CategoryIDs {
			categoryID = strings.TrimSpace(categoryID)
			if categoryID == "" {
				continue
			}
			category, err := store.Category().GetCategory(ctx, categoryID)
			if err != nil {
				continue
			}
			result.Categories = append(result.Categories, r.convertCMSCategory(ctx, category, false))
		}
	}

	return result
}

func (r *Resolver) convertCMSPublication(ctx context.Context, publication *models.Publication, includeMembers bool) *model.Publication {
	if publication == nil {
		return nil
	}

	actorID := strings.TrimSpace(publication.ActorID)
	if actorID == "" {
		actorID = publication.ID
	}

	result := &model.Publication{
		ID:           publication.ID,
		Name:         publication.Name,
		Tagline:      cmsOptionalString(strings.TrimSpace(publication.Tagline)),
		Description:  cmsOptionalString(strings.TrimSpace(publication.Description)),
		Slug:         publication.Slug,
		LogoURL:      cmsOptionalString(strings.TrimSpace(publication.LogoURL)),
		BannerURL:    cmsOptionalString(strings.TrimSpace(publication.BannerURL)),
		CustomDomain: cmsOptionalString(strings.TrimSpace(publication.CustomDomain)),
		Actor:        r.resolveActorByID(ctx, actorID),
		Members:      []*model.PublicationMember{},
		CreatedAt:    model.Time(publication.CreatedAt),
		UpdatedAt:    model.Time(publication.UpdatedAt),
	}

	if !includeMembers {
		return result
	}

	if r.Registry == nil {
		return result
	}

	pubSvc := r.Registry.Publications()
	if pubSvc == nil {
		return result
	}

	members, err := pubSvc.ListMembers(ctx, publication.ID)
	if err != nil {
		return result
	}

	result.Members = make([]*model.PublicationMember, 0, len(members))
	for _, member := range members {
		result.Members = append(result.Members, r.convertCMSPublicationMember(ctx, member))
	}

	return result
}

func (r *Resolver) convertCMSPublicationMember(ctx context.Context, member *models.PublicationMember) *model.PublicationMember {
	if member == nil {
		return nil
	}

	userID := strings.TrimSpace(member.UserID)
	if userID == "" {
		userID = member.DisplayName
	}

	actor := r.resolveActorByID(ctx, userID)
	if actor == nil {
		actor = r.resolveActorByID(ctx, member.UserID)
	}

	displayName := cmsOptionalString(strings.TrimSpace(member.DisplayName))
	bio := cmsOptionalString(strings.TrimSpace(member.Bio))

	return &model.PublicationMember{
		User:        actor,
		Role:        cmsPublicationRoleFromStorage(member.Role),
		DisplayName: displayName,
		Bio:         bio,
		JoinedAt:    model.Time(member.JoinedAt),
	}
}

func cmsOptionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func cmsSeriesGraphQLID(authorID, seriesID string) string {
	authorID = strings.TrimSpace(authorID)
	seriesID = strings.TrimSpace(seriesID)
	if authorID == "" || seriesID == "" {
		return seriesID
	}
	return fmt.Sprintf("%s|%s", authorID, seriesID)
}

func parseSeriesGraphQLID(value string) (authorID string, seriesID string, ok bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", false
	}

	parts := strings.SplitN(value, "|", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	authorID = strings.TrimSpace(parts[0])
	seriesID = strings.TrimSpace(parts[1])
	if authorID == "" || seriesID == "" {
		return "", "", false
	}

	return authorID, seriesID, true
}

func (r *Resolver) ensureAuthorCanWriteCMS(ctx context.Context, username string, attributedTo string) error {
	if username == "" {
		return ErrAuthenticationRequired
	}

	if r.isAdmin(ctx, username) {
		return nil
	}

	expected := cmsLocalActorID(r.getDomain(), username)
	if strings.EqualFold(strings.TrimSpace(attributedTo), expected) {
		return nil
	}

	if actorHost := cmsHostFromID(attributedTo); actorHost != "" {
		if !strings.EqualFold(actorHost, r.getDomain()) {
			return errors.New("insufficient privileges for CMS write")
		}
	}

	if strings.EqualFold(cmsNormalizeUsername(attributedTo), username) {
		return nil
	}

	return errors.New("insufficient privileges for CMS write")
}

// resolveCMSPrivateAttributionActor gates CMS workflow attribution actor resolution
// (generatedBy, reviewedBy, publishedBy) behind an authorization check. These fields
// are private workflow metadata distinct from the public Author (attributedTo) byline.
// Only the article author or an instance admin may view them; public viewers get nil.
//
// CSR-049: Public GraphQL articles leak CMS attribution actors.
func (r *Resolver) resolveCMSPrivateAttributionActor(ctx context.Context, attributedTo string, actorID string) *activitypub.Actor {
	actorID = strings.TrimSpace(actorID)
	if actorID == "" {
		return nil
	}
	if !r.canViewCMSPrivateAttribution(ctx, attributedTo) {
		return nil
	}
	return r.resolveActorByID(ctx, actorID)
}

// canViewCMSPrivateAttribution returns true when the current viewer may see
// CMS workflow attribution actors (generatedBy, reviewedBy, publishedBy).
func (r *Resolver) canViewCMSPrivateAttribution(ctx context.Context, attributedTo string) bool {
	attributedTo = strings.TrimSpace(attributedTo)
	if attributedTo == "" {
		return false
	}

	claims := optionalGraphAuthClaims(ctx)
	if claims == nil || strings.TrimSpace(claims.Username) == "" {
		return false
	}

	// Instance admins can view all CMS attribution.
	if r.isAdmin(ctx, claims.Username) {
		return true
	}

	// The article's attributed author can view their own CMS attribution.
	expected := cmsLocalActorID(r.getDomain(), claims.Username)
	if strings.EqualFold(attributedTo, expected) {
		return true
	}
	return strings.EqualFold(cmsNormalizeUsername(attributedTo), claims.Username)
}

func (r *Resolver) buildCMSDraftReview(
	ctx context.Context,
	draft *models.Draft,
	grant *models.DraftReviewGrant,
	verdicts []*models.DraftReviewVerdict,
	includeMediaAccess bool,
) (*model.DraftReview, error) {
	if draft == nil {
		return nil, nil
	}
	service := r.Registry.Drafts()
	if service == nil {
		return nil, errors.New("draft service is not available")
	}
	rendered, renderErr := cms.RenderDraftPreview(draft)
	var renderedHTML *string
	renderErrors := make([]string, 0, 1)
	if renderErr != nil {
		renderErrors = append(renderErrors, renderErr.Error())
	} else {
		renderedHTML = &rendered.HTML
	}
	state, err := service.DraftReviewState(ctx, draft.AuthorID, draft.ID, draft)
	if err != nil {
		return nil, err
	}
	var scheduledAt *model.Time
	if draft.ScheduledAt != nil {
		t := model.Time(*draft.ScheduledAt)
		scheduledAt = &t
	}
	var reviewGrant *model.DraftReviewGrant
	if grant != nil {
		reviewGrant = r.convertCMSDraftReviewGrant(ctx, grant)
	}
	viewer := strings.TrimSpace(getUsernameFromContext(ctx))
	mediaBindings := r.resolveCMSEditorialMediaBindings(ctx, draft)
	editorialMedia := r.convertCMSEditorialMediaBindings(ctx, mediaBindings, false)
	if includeMediaAccess {
		editorialMedia = r.convertCMSEditorialMediaBindingsWithAccess(ctx, mediaBindings)
	}
	grants := make([]*model.DraftReviewGrant, 0, len(state.Grants))
	activeReviewerIDs := make([]string, 0, len(state.Grants))
	viewerIsOwner := strings.EqualFold(viewer, draft.AuthorID)
	for _, item := range state.Grants {
		if item == nil || (!viewerIsOwner && !strings.EqualFold(item.Reviewer, viewer)) {
			continue
		}
		grants = append(grants, r.convertCMSDraftReviewGrant(ctx, item))
		if item.IsActive(time.Now().UTC()) {
			activeReviewerIDs = append(activeReviewerIDs, item.Reviewer)
		}
	}
	grantCount := state.GrantCount
	grantsTruncated := state.GrantsTruncated
	if !viewerIsOwner {
		grantCount = len(grants)
		grantsTruncated = false
	}
	out := make([]*model.DraftReviewVerdictRecord, 0, len(verdicts))
	for _, v := range verdicts {
		if v == nil {
			continue
		}
		current := state.CurrentVerdicts[v.Reviewer]
		isCurrent := current != nil && current.RecordedAt.Equal(v.RecordedAt) && current.ContentHash == v.ContentHash
		out = append(out, &model.DraftReviewVerdictRecord{
			Verdict:     model.DraftReviewVerdict(v.Verdict),
			Notes:       cmsOptionalString(v.Notes),
			ContentHash: cmsOptionalString(v.ContentHash),
			ReviewerID:  v.Reviewer,
			Reviewer:    r.resolveActorByID(ctx, v.Reviewer),
			RecordedAt:  model.Time(v.RecordedAt),
			Current:     isCurrent,
			Stale:       !isCurrent,
		})
	}
	return &model.DraftReview{
		DraftID: draft.ID, OwnerID: draft.AuthorID, Title: cmsOptionalString(draft.Title), Slug: cmsOptionalString(draft.Slug),
		Content: draft.Content, RenderedHTML: renderedHTML, RenderErrors: renderErrors, ContentFormat: cmsContentFormatFromStorage(draft.ContentFormat),
		Status: cmsDraftStatusFromStorage(draft.Status), ScheduledAt: scheduledAt, UpdatedAt: model.Time(draft.UpdatedAt),
		CreatedAt: model.Time(draft.CreatedAt), GeneratedBy: r.resolveActorByID(ctx, draft.GeneratedBy),
		ReviewedBy: r.resolveActorByID(ctx, draft.ReviewedBy), ReviewStatus: cmsOptionalString(draft.ReviewStatus),
		EditorNotes: cmsOptionalString(draft.EditorNotes), ContentHash: state.ContentHash, Revision: draft.AutosaveVersion,
		EditorialMedia:    editorialMedia,
		ActiveReviewerIds: activeReviewerIDs, PublishEligible: state.PublishEligible,
		PublishBlockingReasons: state.BlockingReasons, ReviewersApproved: state.ReviewersApproved,
		PrincipalApprovalRequired: state.PrincipalApprovalRequired, PrincipalApproved: state.PrincipalApproved,
		GrantCount: grantCount, GrantsTruncated: grantsTruncated,
		Grants: grants, Grant: reviewGrant, Verdicts: out,
		PublishEligibility: &model.DraftPublishEligibility{
			Eligible: state.PublishEligible, BlockingReasons: state.BlockingReasons, ReviewersApproved: state.ReviewersApproved,
			PrincipalApprovalRequired: state.PrincipalApprovalRequired, PrincipalApproved: state.PrincipalApproved,
		},
	}, nil
}

func (r *Resolver) convertCMSDraftReviewGrant(ctx context.Context, grant *models.DraftReviewGrant) *model.DraftReviewGrant {
	if grant == nil {
		return nil
	}
	status := model.DraftReviewGrantStatusActive
	var revokedAt *model.Time
	var expiresAt *model.Time
	if grant.RevokedAt != nil {
		status = model.DraftReviewGrantStatusRevoked
		value := model.Time(*grant.RevokedAt)
		revokedAt = &value
	} else if grant.Expired(time.Now().UTC()) {
		status = model.DraftReviewGrantStatusExpired
	}
	if grant.ExpiresAt != nil {
		value := model.Time(*grant.ExpiresAt)
		expiresAt = &value
	}
	return &model.DraftReviewGrant{
		ReviewerID: grant.Reviewer, Reviewer: r.resolveActorByID(ctx, grant.Reviewer), GrantedAt: model.Time(grant.GrantedAt),
		Status: status, RevokedAt: revokedAt, ExpiresAt: expiresAt,
	}
}

// convertCMSUploadGrant maps a storage upload grant onto its inspectable
// GraphQL surface. The status query recomputes EXPIRED at read time from the
// bounded expiry; presignedURL is populated only while the grant is minted.
func (r *Resolver) convertCMSUploadGrant(grant *models.UploadGrant, presignedURL string) *model.UploadGrant {
	if grant == nil {
		return nil
	}
	now := time.Now().UTC()
	status := model.UploadGrantStatusMinted
	switch {
	case grant.IsUsed():
		status = model.UploadGrantStatusUsed
	case grant.IsFailedDigest():
		status = model.UploadGrantStatusFailedDigest
	case grant.Expired(now):
		status = model.UploadGrantStatusExpired
	}
	out := &model.UploadGrant{
		ID:             grant.GrantID,
		OwnerID:        grant.Owner,
		ContentType:    grant.ContentType,
		MaxSizeBytes:   int(grant.MaxSizeBytes),
		DeclaredSha256: grant.ContentSHA256,
		Status:         status,
		GrantedAt:      model.Time(grant.GrantedAt),
		ExpiresAt:      model.Time(grant.ExpiresAt),
	}
	if presignedURL != "" {
		out.PresignedURL = &presignedURL
	}
	if grant.MediaID != "" {
		mediaID := grant.MediaID
		out.MediaID = &mediaID
	}
	if grant.UsedAt != nil {
		usedAt := model.Time(*grant.UsedAt)
		out.UsedAt = &usedAt
	}
	if grant.FailureReason != "" {
		reason := grant.FailureReason
		out.FailureReason = &reason
	}
	return out
}

// convertCMSUploadGrantMedia maps the admitted internal editorial media record
// onto the finalize result surface (media ID for M1 draft binding, verified
// content hash, and the M0 processing status).
func (r *Resolver) convertCMSUploadGrantMedia(media *models.Media) *model.UploadGrantMedia {
	if media == nil {
		return nil
	}
	return &model.UploadGrantMedia{
		MediaID:     media.MediaID,
		ContentType: media.ContentType,
		Size:        int(media.FileSize),
		ContentHash: media.ContentHash,
		Status:      media.Status,
		Visibility:  string(media.Visibility),
	}
}
