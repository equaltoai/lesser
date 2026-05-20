package cms

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage/models"
)

func cmsNormalizeAttributionActorID(value string) string {
	return strings.TrimSpace(value)
}

func cmsDefaultLocalAttributionActorID(domain string, username string) string {
	username = strings.TrimSpace(username)
	domain = strings.TrimSpace(domain)
	if username == "" || domain == "" {
		return ""
	}
	return common.GenerateActorID(domain, username)
}

func cmsNormalizeDraftAttribution(draft *models.Draft) {
	if draft == nil {
		return
	}
	draft.GeneratedBy = cmsNormalizeAttributionActorID(draft.GeneratedBy)
	draft.ReviewedBy = cmsNormalizeAttributionActorID(draft.ReviewedBy)
}

func cmsNormalizeArticleAttribution(article *models.Article, existing *models.Article) {
	if article == nil {
		return
	}

	article.GeneratedBy = cmsNormalizeAttributionActorID(article.GeneratedBy)
	article.ReviewedBy = cmsNormalizeAttributionActorID(article.ReviewedBy)
	article.PublishedBy = cmsNormalizeAttributionActorID(article.PublishedBy)

	if existing != nil {
		if article.GeneratedBy == "" {
			article.GeneratedBy = cmsNormalizeAttributionActorID(existing.GeneratedBy)
		}
		if article.ReviewedBy == "" {
			article.ReviewedBy = cmsNormalizeAttributionActorID(existing.ReviewedBy)
		}
		if article.PublishedBy == "" {
			article.PublishedBy = cmsNormalizeAttributionActorID(existing.PublishedBy)
		}
	}

	if article.PublishedBy == "" {
		article.PublishedBy = cmsNormalizeAttributionActorID(article.AttributedTo)
	}
}

func cmsApplyDraftAttributionToArticle(article *models.Article, draft *models.Draft, domain string, publisherUsername string, preserveExisting bool) {
	if article == nil || draft == nil {
		return
	}

	cmsNormalizeDraftAttribution(draft)
	if draft.GeneratedBy != "" || !preserveExisting {
		article.GeneratedBy = draft.GeneratedBy
	}
	if draft.ReviewedBy != "" || !preserveExisting {
		article.ReviewedBy = draft.ReviewedBy
	}

	publishedBy := cmsDefaultLocalAttributionActorID(domain, publisherUsername)
	if publishedBy != "" {
		article.PublishedBy = publishedBy
	}
}
