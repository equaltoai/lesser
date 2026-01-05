package cms

import (
	"context"
	neturl "net/url"
	"strings"

	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/dynamorm/pkg/core"
	dynamormerrors "github.com/pay-theory/dynamorm/pkg/errors"
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
	return strings.TrimSpace(parsed.Host)
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

func cmsDeleteArticleSlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSArticleSlugIndexPK(slug))
}

func cmsEnsureCategorySlugIndex(ctx context.Context, db core.DB, slug string, categoryID string) (bool, error) {
	return cmsEnsureSlugIndex(ctx, db, models.CMSCategorySlugIndexPK(slug), slug, categoryID, "category slug")
}

func cmsDeleteCategorySlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSCategorySlugIndexPK(slug))
}

func cmsEnsurePublicationSlugIndex(ctx context.Context, db core.DB, slug string, publicationID string) (bool, error) {
	return cmsEnsureSlugIndex(ctx, db, models.CMSPublicationSlugIndexPK(slug), slug, publicationID, "publication slug")
}

func cmsDeletePublicationSlugIndex(ctx context.Context, db core.DB, slug string) {
	cmsDeleteSlugIndex(ctx, db, models.CMSPublicationSlugIndexPK(slug))
}
