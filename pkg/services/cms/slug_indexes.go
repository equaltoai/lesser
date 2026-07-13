package cms

import (
	"context"
	neturl "net/url"
	"strings"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/theory-cloud/tabletheory/v2/pkg/core"
	dynamormerrors "github.com/theory-cloud/tabletheory/v2/pkg/errors"
)

func cmsHostFromURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parsed, err := neturl.Parse(value)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parsed.Host))
}

func cmsNormalizeTenant(tenant string) string {
	return strings.ToLower(strings.TrimSpace(tenant))
}

func cmsTenantFromID(value string) string {
	return cmsNormalizeTenant(cmsHostFromURL(value))
}

func cmsArticleSlugFromCanonicalID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}

	parsed, err := neturl.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Host) == "" {
		return "", false
	}

	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) != 2 || parts[0] != "articles" || strings.TrimSpace(parts[1]) == "" {
		return "", false
	}

	return strings.TrimSpace(parts[1]), true
}

func cmsEnsureSlugIndex(ctx context.Context, db core.DB, pk string, slug string, targetID string, itemType string) (bool, error) {
	idx := &models.CMSSlugIndex{
		PK:       pk,
		Slug:     slug,
		TargetID: targetID,
	}
	if err := idx.UpdateKeys(); err != nil {
		return false, err
	}

	err := db.WithContext(ctx).Model(idx).IfNotExists().Create()
	if err == nil {
		return true, nil
	}
	if !dynamormerrors.IsConditionFailed(err) {
		return false, err
	}

	var existing models.CMSSlugIndex
	getErr := db.WithContext(ctx).Model(&models.CMSSlugIndex{}).
		Where("PK", "=", idx.PK).
		Where("SK", "=", idx.SK).
		First(&existing)
	if getErr != nil {
		return false, err
	}

	if strings.EqualFold(strings.TrimSpace(existing.TargetID), strings.TrimSpace(targetID)) {
		return false, nil
	}

	return false, apperrors.ItemAlreadyExistsWithID(itemType, strings.TrimSpace(slug))
}

func cmsDeleteSlugIndex(ctx context.Context, db core.DB, pk string) {
	pk = strings.TrimSpace(pk)
	if pk == "" {
		return
	}

	entry := &models.CMSSlugIndex{
		PK: pk,
		SK: models.CMSSlugIndexSK(),
	}
	_ = db.WithContext(ctx).Model(entry).Delete()
}

func cmsEnsureArticleSlugIndex(ctx context.Context, db core.DB, slug string, articleID string) (bool, error) {
	return cmsEnsureSlugIndex(ctx, db, models.CMSArticleSlugIndexPK(slug), slug, articleID, "article slug")
}

func cmsEnsureArticleSlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string, articleID string) (bool, error) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		return cmsEnsureArticleSlugIndex(ctx, db, slug, articleID)
	}
	return cmsEnsureSlugIndex(ctx, db, models.CMSTenantArticleSlugIndexPK(tenant, slug), slug, articleID, "article slug")
}

func cmsDeleteArticleSlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSArticleSlugIndexPK(slug))
}

func cmsDeleteArticleSlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		cmsDeleteArticleSlugIndex(ctx, db, slug)
		return
	}
	cmsDeleteSlugIndex(ctx, db, models.CMSTenantArticleSlugIndexPK(tenant, slug))
}

func cmsEnsureCategorySlugIndex(ctx context.Context, db core.DB, slug string, categoryID string) (bool, error) {
	return cmsEnsureSlugIndex(ctx, db, models.CMSCategorySlugIndexPK(slug), slug, categoryID, "category slug")
}

func cmsEnsureCategorySlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string, categoryID string) (bool, error) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		return cmsEnsureCategorySlugIndex(ctx, db, slug, categoryID)
	}
	return cmsEnsureSlugIndex(ctx, db, models.CMSTenantCategorySlugIndexPK(tenant, slug), slug, categoryID, "category slug")
}

func cmsDeleteCategorySlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSCategorySlugIndexPK(slug))
}

func cmsDeleteCategorySlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		cmsDeleteCategorySlugIndex(ctx, db, slug)
		return
	}
	cmsDeleteSlugIndex(ctx, db, models.CMSTenantCategorySlugIndexPK(tenant, slug))
}

func cmsEnsurePublicationSlugIndex(ctx context.Context, db core.DB, slug string, publicationID string) (bool, error) {
	return cmsEnsureSlugIndex(ctx, db, models.CMSPublicationSlugIndexPK(slug), slug, publicationID, "publication slug")
}

func cmsEnsurePublicationSlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string, publicationID string) (bool, error) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		return cmsEnsurePublicationSlugIndex(ctx, db, slug, publicationID)
	}
	return cmsEnsureSlugIndex(ctx, db, models.CMSTenantPublicationSlugIndexPK(tenant, slug), slug, publicationID, "publication slug")
}

func cmsDeletePublicationSlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSPublicationSlugIndexPK(slug))
}

func cmsDeletePublicationSlugIndexForTenant(ctx context.Context, db core.DB, tenant string, slug string) {
	tenant = cmsNormalizeTenant(tenant)
	if tenant == "" {
		cmsDeletePublicationSlugIndex(ctx, db, slug)
		return
	}
	cmsDeleteSlugIndex(ctx, db, models.CMSTenantPublicationSlugIndexPK(tenant, slug))
}
