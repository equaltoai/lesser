package graph

import (
	"context"
	"errors"
	"strings"

	"github.com/equaltoai/lesser/graph/model"
	"github.com/equaltoai/lesser/pkg/activitypub"
	"github.com/equaltoai/lesser/pkg/activitypubutil"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/services/notes"
	"github.com/equaltoai/lesser/pkg/storage/interfaces"
	storagemodels "github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/equaltoai/lesser/pkg/translation"
	"go.uber.org/zap"
)

type statusUsersFetcher func(ctx context.Context, service *notes.Service, statusID string, pagination interfaces.PaginationOptions) (*notes.UsersResult, error)

func (r *queryResolver) resolveStatusActorListPage(ctx context.Context, id string, first *int, after *model.Cursor, label string, fetch statusUsersFetcher) (*model.ActorListPage, error) {
	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	service := r.Registry.Notes()
	if service == nil {
		return nil, errors.New("notes service is not available")
	}
	if fetch == nil {
		return nil, errors.New("fetch function is not available")
	}

	limit := clampLimit(first)
	cursor := cursorToString(after)

	result, err := fetch(ctx, service, statusID, interfaces.PaginationOptions{Limit: limit, Cursor: cursor})
	if err != nil {
		r.Logger.Error("Failed to list status users",
			zap.String("kind", label),
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to list status users"), err)
	}

	actors := make([]*activitypub.Actor, 0, len(result.Users))
	for _, account := range result.Users {
		actor := r.convertAccountToActor(account)
		if actor != nil {
			actors = append(actors, actor)
		}
	}

	nextCursor := ""
	totalCount := len(actors)
	if result.Pagination != nil {
		nextCursor = result.Pagination.NextCursor
		if result.Pagination.Total >= 0 {
			totalCount = int(result.Pagination.Total)
		}
	}

	return &model.ActorListPage{
		Actors:     actors,
		NextCursor: stringToCursor(nextCursor),
		TotalCount: totalCount,
	}, nil
}

// StatusFavouritedBy returns actors who favourited a status.
func (r *queryResolver) StatusFavouritedBy(ctx context.Context, id string, first *int, after *model.Cursor) (*model.ActorListPage, error) {
	return r.resolveStatusActorListPage(ctx, id, first, after, "favourited_by", func(ctx context.Context, service *notes.Service, statusID string, pagination interfaces.PaginationOptions) (*notes.UsersResult, error) {
		return service.GetLikers(ctx, &notes.GetLikersQuery{
			StatusID:   statusID,
			Pagination: pagination,
		})
	})
}

// StatusRebloggedBy returns actors who reblogged a status.
func (r *queryResolver) StatusRebloggedBy(ctx context.Context, id string, first *int, after *model.Cursor) (*model.ActorListPage, error) {
	return r.resolveStatusActorListPage(ctx, id, first, after, "reblogged_by", func(ctx context.Context, service *notes.Service, statusID string, pagination interfaces.PaginationOptions) (*notes.UsersResult, error) {
		return service.GetRebloggers(ctx, &notes.GetRebloggersQuery{
			StatusID:   statusID,
			Pagination: pagination,
		})
	})
}

// StatusHistory returns the edit history for a status.
func (r *queryResolver) StatusHistory(ctx context.Context, id string, limit *int) ([]*model.StatusEdit, error) {
	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	service := r.Registry.Notes()
	if service == nil {
		return nil, errors.New("notes service is not available")
	}

	status, err := service.GetNote(ctx, statusID)
	if err != nil || status == nil {
		r.Logger.Error("Failed to get status for history",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get status for history"), err)
	}

	accountActor := r.resolveStatusAuthorActor(ctx, status)

	currentCreatedAt := status.PublishedAt
	if !status.UpdatedAt.IsZero() {
		currentCreatedAt = status.UpdatedAt
	}

	currentSpoiler := ""
	if status.Note != nil && status.Note.Get() != nil {
		currentSpoiler = status.Note.Get().Summary
	}

	edits := []*model.StatusEdit{
		{
			Content:     status.Content,
			SpoilerText: optionalString(currentSpoiler),
			Sensitive:   status.Sensitive,
			CreatedAt:   model.Time(currentCreatedAt),
			Account:     accountActor,
		},
	}

	historyLimit := 100
	if limit != nil && *limit > 0 {
		if *limit > 100 {
			historyLimit = 100
		} else {
			historyLimit = *limit
		}
	}

	historyResult, err := service.GetUpdateHistory(ctx, &notes.GetUpdateHistoryQuery{
		StatusID: statusID,
		Limit:    historyLimit,
	})
	if err != nil {
		r.Logger.Error("Failed to get status update history",
			zap.String("status_id", statusID),
			zap.Error(err))
		// Return current edit even if history retrieval fails.
		return edits, nil
	}

	for _, history := range historyResult.History {
		if history == nil {
			continue
		}

		content := history.Content
		spoilerText := history.Summary
		sensitive := false

		if len(history.PreviousState) > 0 {
			content = getStringFromMapWithFallback(history.PreviousState, "content", content)
			spoilerText = getStringFromMapWithFallback(history.PreviousState, "summary", spoilerText)
			sensitive = getBoolFromMap(history.PreviousState, "sensitive", sensitive)
		}

		edits = append(edits, &model.StatusEdit{
			Content:     content,
			SpoilerText: optionalString(spoilerText),
			Sensitive:   sensitive,
			CreatedAt:   model.Time(history.UpdatedAt),
			Account:     accountActor,
		})
	}

	return edits, nil
}

// LinkTimeline returns statuses that contain a given link.
func (r *queryResolver) LinkTimeline(ctx context.Context, url string, first *int, after *model.Cursor) (*model.ObjectConnection, error) {
	linkURL := strings.TrimSpace(url)
	if err := common.ValidateRequiredParam("url", linkURL); err != nil {
		return nil, err
	}

	if r.Storage == nil || r.Storage.Status() == nil {
		return nil, ErrStatusRepositoryUnavailable
	}

	limit := 20
	if first != nil && *first > 0 {
		if *first > 100 {
			limit = 100
		} else {
			limit = *first
		}
	}

	opts := interfaces.PaginationOptions{
		Limit:  limit,
		Cursor: cursorToString(after),
	}

	results, err := r.Storage.Status().SearchStatuses(ctx, linkURL, opts)
	if err != nil {
		r.Logger.Error("Failed to search statuses for link timeline",
			zap.String("url", linkURL),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to search statuses for link timeline"), err)
	}

	edges := make([]*model.ObjectEdge, 0, len(results.Items))
	for _, status := range results.Items {
		if status == nil {
			continue
		}
		if !statusContainsLink(status, linkURL) {
			continue
		}

		edges = append(edges, &model.ObjectEdge{
			Node:   r.convertStatusToObject(ctx, status),
			Cursor: model.Cursor(status.StatusID),
		})
	}

	var startCursor, endCursor *model.Cursor
	if err := common.ValidateSliceNotEmpty("edges", edges); err == nil {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.ObjectConnection{
		Edges: edges,
		PageInfo: &model.PageInfo{
			HasNextPage:     results.HasMore,
			HasPreviousPage: after != nil,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(edges),
	}, nil
}

// TranslateStatus translates the content of a status.
func (r *queryResolver) TranslateStatus(ctx context.Context, id string, targetLanguage *string) (*model.TranslationResult, error) {
	username, err := r.requireAuth(ctx)
	if err != nil {
		return nil, err
	}

	if r.Config == nil || !r.Config.TranslationEnabled {
		return nil, errors.New("translation service is not enabled")
	}

	statusID := strings.TrimSpace(id)
	if err := common.ValidateRequiredParam("id", statusID); err != nil {
		return nil, err
	}

	notesService := r.Registry.Notes()
	if notesService == nil {
		return nil, errors.New("notes service is not available")
	}

	status, err := notesService.GetNote(ctx, statusID)
	if err != nil || status == nil {
		r.Logger.Error("Failed to get status for translation",
			zap.String("status_id", statusID),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to get status for translation"), err)
	}

	content := strings.TrimSpace(status.Content)
	if err := common.ValidateRequiredParam("content", content); err != nil {
		return nil, errors.New("status has no content to translate")
	}

	spoilerText := ""
	if status.Note != nil && status.Note.Get() != nil {
		spoilerText = status.Note.Get().Summary
	}

	sourceLang := strings.TrimSpace(status.Language)
	targetLang := r.resolveTargetLanguage(ctx, username, targetLanguage)
	if err := common.ValidateLanguageCode(targetLang); err != nil {
		return nil, errors.Join(errors.New("invalid target language"), err)
	}

	translationSvc, err := translation.NewService(ctx, r.Config, r.Storage, r.Logger, true)
	if err != nil {
		r.Logger.Error("Failed to initialize translation service", zap.Error(err))
		return nil, errors.Join(errors.New("failed to initialize translation service"), err)
	}

	translatedContent, detectedLang, err := translationSvc.TranslateHTML(ctx, content, sourceLang, targetLang)
	if err != nil {
		r.Logger.Error("Failed to translate status content",
			zap.String("status_id", statusID),
			zap.String("target_language", targetLang),
			zap.Error(err))
		return nil, errors.Join(errors.New("failed to translate status content"), err)
	}

	var translatedSpoiler *string
	if spoilerText != "" {
		if translated, _, tErr := translationSvc.TranslateHTML(ctx, spoilerText, sourceLang, targetLang); tErr == nil {
			translatedSpoiler = optionalString(translated)
		}
	}

	return &model.TranslationResult{
		Content:          translatedContent,
		SpoilerText:      translatedSpoiler,
		DetectedLanguage: detectedLang,
		Provider:         "AWS Translate",
	}, nil
}

func (r *queryResolver) resolveTargetLanguage(ctx context.Context, username string, explicit *string) string {
	if explicit != nil {
		lang := strings.TrimSpace(*explicit)
		if lang != "" {
			return lang
		}
	}

	service := r.Registry.Accounts()
	if service == nil {
		return "en"
	}

	result, err := service.GetPreferences(ctx, &accounts.GetPreferencesQuery{
		Username: username,
	})
	if err != nil || result == nil || result.Preferences == nil {
		return "en"
	}

	if lang, ok := result.Preferences["language"].(string); ok {
		lang = strings.TrimSpace(lang)
		if lang != "" {
			return lang
		}
	}

	return "en"
}

func (r *queryResolver) resolveStatusAuthorActor(ctx context.Context, status *storagemodels.Status) *activitypub.Actor {
	username := strings.TrimSpace(status.AuthorUsername)
	if username == "" {
		username = strings.TrimSpace(extractUsernameFromActorIdentifier(status.AuthorID))
	}

	if username != "" && r.Registry != nil && r.Registry.Accounts() != nil {
		account, err := r.Registry.Accounts().GetAccount(ctx, username)
		if err == nil && account != nil {
			actor := r.convertAccountToActor(account)
			if actor != nil {
				return actor
			}
		}
	}

	baseURL := ""
	if r.Config != nil {
		baseURL = r.Config.BaseURL()
	}

	fallback := username
	if fallback == "" {
		fallback = "unknown"
	}
	return activitypubutil.BuildLocalActor(fallback, baseURL, nil, nil)
}

func optionalString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func getStringFromMapWithFallback(m map[string]any, key string, fallback string) string {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(string); ok {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return fallback
}

func getBoolFromMap(m map[string]any, key string, fallback bool) bool {
	if m == nil {
		return fallback
	}
	if v, ok := m[key].(bool); ok {
		return v
	}
	return fallback
}

func statusContainsLink(status *storagemodels.Status, linkURL string) bool {
	if status == nil {
		return false
	}

	linkURL = strings.TrimSpace(linkURL)
	if linkURL == "" {
		return false
	}

	for _, existing := range status.URLs {
		if strings.EqualFold(strings.TrimSpace(existing), linkURL) {
			return true
		}
	}

	return strings.Contains(status.Content, linkURL)
}
