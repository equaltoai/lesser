package cms

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/equaltoai/lesser/pkg/cmsrender"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"go.uber.org/zap"
)

const (
	cmsMetricNamespace                 = "Lesser/CMS"
	cmsMetricArticleRenderFailure      = "ArticleRenderFailure"
	cmsMetricArticleFederationAttempt  = "ArticleFederationAttempt"
	cmsMetricArticleFederationSuccess  = "ArticleFederationSuccess"
	cmsMetricArticleFederationFailure  = "ArticleFederationFailure"
	cmsMetricStatusAttempt             = "attempt"
	cmsMetricStatusSuccess             = "success"
	cmsMetricStatusFailure             = "failure"
	cmsMetricUnknownStage              = "unknown"
	cmsLogArticleRenderFailure         = "cms_article_render_failure"
	cmsLogDraftRenderFailure           = "cms_draft_render_failure"
	cmsLogArticleFederationAttempt     = "cms_article_federation_attempt"
	cmsLogArticleFederationOutcome     = "cms_article_federation_outcome"
	cmsFederationFailureStageTransform = "transform"
	cmsFederationFailureStageActor     = "actor_lookup"
	cmsFederationFailureStageDelivery  = "delivery"
)

func cmsLogger(logger *zap.Logger) *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

func cmsMetricStage() string {
	for _, key := range []string{"STAGE", "ENVIRONMENT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return cmsMetricUnknownStage
}

func cmsMetricFields(metricName string, operation string, status string) []zap.Field {
	stage := cmsMetricStage()
	return []zap.Field{
		zap.Any("_aws", map[string]interface{}{
			"Timestamp": time.Now().UnixMilli(),
			"CloudWatchMetrics": []map[string]interface{}{
				{
					"Namespace": cmsMetricNamespace,
					"Dimensions": [][]string{
						{"Stage", "Status"},
						{"Stage", "Operation", "Status"},
					},
					"Metrics": []map[string]string{
						{"Name": metricName, "Unit": "Count"},
					},
				},
			},
		}),
		zap.String("Stage", stage),
		zap.String("Operation", strings.TrimSpace(operation)),
		zap.String("Status", strings.TrimSpace(status)),
		zap.Float64(metricName, 1),
	}
}

func logCMSArticleRenderFailure(logger *zap.Logger, operation string, article *models.Article, err error) {
	fields := cmsMetricFields(cmsMetricArticleRenderFailure, operation, cmsMetricStatusFailure)
	fields = append(fields, cmsArticleFields(article)...)
	fields = append(fields, renderErrorFields(err)...)
	cmsLogger(logger).Error(cmsLogArticleRenderFailure, fields...)
}

func logCMSDraftRenderFailure(logger *zap.Logger, operation string, draft *models.Draft, err error) {
	fields := cmsMetricFields(cmsMetricArticleRenderFailure, operation, cmsMetricStatusFailure)
	fields = append(fields, cmsDraftFields(draft)...)
	fields = append(fields, renderErrorFields(err)...)
	cmsLogger(logger).Error(cmsLogDraftRenderFailure, fields...)
}

func logCMSArticleFederationAttempt(logger *zap.Logger, operation string, activityType string, article *models.Article) {
	fields := cmsMetricFields(cmsMetricArticleFederationAttempt, operation, cmsMetricStatusAttempt)
	fields = append(fields, cmsArticleFields(article)...)
	fields = append(fields, zap.String("activity_type", strings.TrimSpace(activityType)))
	cmsLogger(logger).Info(cmsLogArticleFederationAttempt, fields...)
}

func logCMSArticleFederationSuccess(logger *zap.Logger, operation string, activityType string, activityID string, article *models.Article) {
	fields := cmsMetricFields(cmsMetricArticleFederationSuccess, operation, cmsMetricStatusSuccess)
	fields = append(fields, cmsArticleFields(article)...)
	fields = append(fields,
		zap.String("activity_type", strings.TrimSpace(activityType)),
		zap.String("activity_id", strings.TrimSpace(activityID)),
	)
	cmsLogger(logger).Info(cmsLogArticleFederationOutcome, fields...)
}

func logCMSArticleFederationFailure(
	logger *zap.Logger,
	operation string,
	failureStage string,
	activityType string,
	activityID string,
	article *models.Article,
	err error,
) {
	fields := cmsMetricFields(cmsMetricArticleFederationFailure, operation, cmsMetricStatusFailure)
	fields = append(fields, cmsArticleFields(article)...)
	fields = append(fields,
		zap.String("failure_stage", strings.TrimSpace(failureStage)),
		zap.String("activity_type", strings.TrimSpace(activityType)),
		zap.String("activity_id", strings.TrimSpace(activityID)),
		zap.Error(err),
	)
	cmsLogger(logger).Error(cmsLogArticleFederationOutcome, fields...)
}

func cmsArticleFields(article *models.Article) []zap.Field {
	if article == nil {
		return []zap.Field{
			zap.String("article_id", ""),
			zap.String("slug", ""),
			zap.String("content_format", ""),
			zap.Int("source_bytes", 0),
		}
	}
	return []zap.Field{
		zap.String("article_id", cmsArticleID(article)),
		zap.String("slug", strings.TrimSpace(article.Slug)),
		zap.String("content_format", strings.TrimSpace(article.ContentFormat)),
		zap.Int("source_bytes", len(article.Content)),
	}
}

func cmsArticleID(article *models.Article) string {
	if article == nil {
		return ""
	}
	return strings.TrimSpace(article.ID)
}

func cmsDraftFields(draft *models.Draft) []zap.Field {
	if draft == nil {
		return []zap.Field{
			zap.String("draft_id", ""),
			zap.String("content_type", ""),
			zap.String("content_format", ""),
			zap.Int("source_bytes", 0),
		}
	}
	return []zap.Field{
		zap.String("draft_id", strings.TrimSpace(draft.ID)),
		zap.String("content_type", strings.TrimSpace(draft.ContentType)),
		zap.String("content_format", strings.TrimSpace(draft.ContentFormat)),
		zap.Int("source_bytes", len(draft.Content)),
	}
}

func renderErrorFields(err error) []zap.Field {
	return []zap.Field{
		zap.String("error_kind", renderErrorKind(err)),
		zap.Error(err),
	}
}

func renderErrorKind(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, cmsrender.ErrUnsupportedContentFormat):
		return "unsupported_content_format"
	case errors.Is(err, cmsrender.ErrArticleContentTooLarge):
		return "source_too_large"
	case errors.Is(err, cmsrender.ErrArticleRenderedContentTooLarge):
		return "rendered_too_large"
	case strings.Contains(strings.ToLower(err.Error()), "valid utf-8"):
		return "invalid_utf8"
	default:
		return "render_error"
	}
}
