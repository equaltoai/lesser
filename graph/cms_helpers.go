package graph

import (
	neturl "net/url"
	"regexp"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/transformations"
)

var (
	cmsHTMLTags = regexp.MustCompile(`<[^>]*>`)
)

const (
	cmsDraftStatusDraft      = "draft"
	cmsDraftStatusScheduled  = "scheduled"
	cmsDraftStatusPublishing = "publishing"
	cmsDraftStatusPublished  = "published"
	cmsDraftStatusFailed     = jobStatusFailed

	cmsPublicationRoleOwner       = "owner"
	cmsPublicationRoleEditor      = "editor"
	cmsPublicationRoleWriter      = "writer"
	cmsPublicationRoleContributor = "contributor"
)

func cmsSlugify(value string) string {
	return common.Slugify(value)
}

func cmsArticleID(domain, slug string) string {
	return common.GenerateObjectID(domain, "articles", slug)
}

func cmsArticleObjectID(domain, objectID string) string {
	return common.GenerateObjectID(domain, "objects", objectID)
}

func cmsCategoryID(domain, slug string) string {
	return common.GenerateObjectID(domain, "categories", slug)
}

func cmsCategoryObjectID(domain, objectID string) string {
	return common.GenerateObjectID(domain, "categories", objectID)
}

func cmsPublicationID(domain, slug string) string {
	return common.GenerateObjectID(domain, "publications", slug)
}

func cmsPublicationObjectID(domain, objectID string) string {
	return common.GenerateObjectID(domain, "publications", objectID)
}

func cmsExtractSlugFromURL(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}

	parsed, err := neturl.Parse(id)
	if err == nil && parsed.Path != "" {
		path := strings.Trim(parsed.Path, "/")
		if path == "" {
			return ""
		}
		parts := strings.Split(path, "/")
		return parts[len(parts)-1]
	}

	if strings.Contains(id, "/") {
		parts := strings.Split(strings.Trim(id, "/"), "/")
		return parts[len(parts)-1]
	}

	return id
}

func cmsNormalizeUsername(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	if strings.Contains(value, "://") {
		return transformations.ExtractUsernameFromActorID(value)
	}

	return strings.TrimPrefix(value, "@")
}

func cmsLocalActorID(domain, username string) string {
	return common.GenerateActorID(domain, username)
}

func cmsContentFormatToStorage(format model.ContentFormat) string {
	switch format {
	case model.ContentFormatMarkdown:
		return "markdown"
	case model.ContentFormatHTML:
		return "html"
	default:
		return strings.ToLower(string(format))
	}
}

func cmsContentFormatFromStorage(value string) model.ContentFormat {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "markdown", "md":
		return model.ContentFormatMarkdown
	case "html":
		return model.ContentFormatHTML
	default:
		return model.ContentFormatMarkdown
	}
}

func cmsDraftStatusFromStorage(value string) model.DraftStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case cmsDraftStatusScheduled:
		return model.DraftStatusScheduled
	case cmsDraftStatusPublishing:
		return model.DraftStatusPublishing
	case cmsDraftStatusPublished:
		return model.DraftStatusPublished
	case cmsDraftStatusFailed:
		return model.DraftStatusFailed
	default:
		return model.DraftStatusDraft
	}
}

func cmsDraftStatusToStorage(value model.DraftStatus) string {
	switch value {
	case model.DraftStatusScheduled:
		return cmsDraftStatusScheduled
	case model.DraftStatusPublishing:
		return cmsDraftStatusPublishing
	case model.DraftStatusPublished:
		return cmsDraftStatusPublished
	case model.DraftStatusFailed:
		return cmsDraftStatusFailed
	default:
		return cmsDraftStatusDraft
	}
}

func cmsChangeTypeFromStorage(value string) model.ChangeType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "create":
		return model.ChangeTypeCreate
	case "restore":
		return model.ChangeTypeRestore
	default:
		return model.ChangeTypeUpdate
	}
}

func cmsPublicationRoleToStorage(role model.PublicationRole) string {
	switch role {
	case model.PublicationRoleOwner:
		return cmsPublicationRoleOwner
	case model.PublicationRoleEditor:
		return cmsPublicationRoleEditor
	case model.PublicationRoleWriter:
		return cmsPublicationRoleWriter
	case model.PublicationRoleContributor:
		return cmsPublicationRoleContributor
	default:
		return strings.ToLower(string(role))
	}
}

func cmsPublicationRoleFromStorage(role string) model.PublicationRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case cmsPublicationRoleOwner:
		return model.PublicationRoleOwner
	case cmsPublicationRoleEditor:
		return model.PublicationRoleEditor
	case cmsPublicationRoleWriter:
		return model.PublicationRoleWriter
	case cmsPublicationRoleContributor:
		return model.PublicationRoleContributor
	default:
		return model.PublicationRoleContributor
	}
}

func cmsObjectTypeToStorage(value model.ObjectType) string {
	switch value {
	case model.ObjectTypeArticle:
		return activitypub.ArticleType
	case model.ObjectTypeNote:
		return activitypub.NoteType
	case model.ObjectTypeImage:
		return activitypub.ImageType
	case model.ObjectTypeVideo:
		return activitypub.VideoType
	case model.ObjectTypeQuestion:
		return "Question"
	case model.ObjectTypeEvent:
		return "Event"
	case model.ObjectTypePage:
		return "Page"
	default:
		return activitypub.ArticleType
	}
}

func cmsObjectTypeFromStorage(value string) model.ObjectType {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ContentTypeNote:
		return model.ObjectTypeNote
	case ContentTypeImage:
		return model.ObjectTypeImage
	case ContentTypeVideo:
		return model.ObjectTypeVideo
	case "question":
		return model.ObjectTypeQuestion
	case ContentTypeEvent:
		return model.ObjectTypeEvent
	case "page":
		return model.ObjectTypePage
	default:
		return model.ObjectTypeArticle
	}
}

func cmsEstimateWordCount(content string) int {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0
	}

	stripped := cmsHTMLTags.ReplaceAllString(content, " ")
	return len(strings.Fields(stripped))
}

func cmsEstimateReadingMinutes(wordCount int) int {
	if wordCount <= 0 {
		return 0
	}

	const wordsPerMinute = 200
	minutes := (wordCount + (wordsPerMinute - 1)) / wordsPerMinute
	if minutes < 1 {
		return 1
	}
	return minutes
}
