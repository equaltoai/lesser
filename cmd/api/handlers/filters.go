package handlers

import (
	"errors"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	storageModels "github.com/equaltoai/lesser/pkg/storage/models"
	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
	"go.uber.org/zap"
)

var errFilterKeywordNotFound = errors.New("filter keyword not found")

// HandleGetFiltersLift handles GET /api/v2/filters
func (h *Handler) HandleGetFiltersLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get all filters for the user
	filters, err := h.repos.Moderation().GetFiltersForUser(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get filters", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert to Mastodon API format
	result := make([]*mastodon.Filter, 0, len(filters))
	for _, filter := range filters {
		// Get keywords and statuses for each filter
		keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context(), filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context(), filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)
		result = append(result, mastodonFilter)
	}

	return okJSON(result)
}

// HandleGetFilterLift handles GET /api/v2/filters/:id
func (h *Handler) HandleGetFilterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get keywords and statuses
	keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context(), filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context(), filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)
	return okJSON(mastodonFilter)
}

// HandleCreateFilterLift handles POST /api/v2/filters
func (h *Handler) HandleCreateFilterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse request
	var params apimodels.CreateFilterRequest
	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		return common.RespondInvalidRequest(ctx)
	}

	// Validate filter parameters using comprehensive validation
	contextParams := make([]interface{}, 0, len(params.Context))
	for _, c := range params.Context {
		contextParams = append(contextParams, c)
	}

	filterParams := map[string]interface{}{
		"title":         params.Title,
		"context":       contextParams,
		"filter_action": params.FilterAction,
	}
	if params.ExpiresIn != nil {
		filterParams["expires_in"] = float64(*params.ExpiresIn)
	}
	if err := common.ValidateFilterParams(filterParams); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}

	// Validate and set default filter action if not provided
	if err := common.ValidateFilterAction(params.FilterAction); err != nil {
		return common.RespondBadRequest(ctx, err.Error())
	}
	if params.FilterAction == "" {
		params.FilterAction = "warn"
	}

	// Create the filter
	filter := h.buildFilterFromParams(username, &params)

	// Save filter to storage
	if err := h.saveFilter(ctx, filter); err != nil {
		h.logger.Error("failed to create filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Add keywords if provided
	keywords := h.addFilterKeywords(ctx, filter.ID, params.KeywordsAttributes)

	// Convert to Mastodon API format
	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, []*storage.FilterStatus{})
	return okJSON(mastodonFilter)
}

// buildFilterFromParams builds a Filter object from request parameters
func (h *Handler) buildFilterFromParams(username string, params *apimodels.CreateFilterRequest) *storage.Filter {
	filter := &storage.Filter{
		Username:      username,
		Title:         params.Title,
		Context:       params.Context,
		FilterAction:  params.FilterAction,
		Severity:      h.validateSeverity(params.Severity),
		MatchMode:     h.validateMatchMode(params.MatchMode),
		CaseSensitive: params.CaseSensitive,
	}

	// Set expiration if provided
	if params.ExpiresIn != nil && *params.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(*params.ExpiresIn) * time.Second)
		filter.ExpiresAt = &expiresAt
	}

	return filter
}

// validateSeverity validates and normalizes filter severity
func (h *Handler) validateSeverity(severity string) string {
	switch severity {
	case "low", "medium", "high":
		return severity
	default:
		return "medium" // Default severity
	}
}

// validateMatchMode validates and normalizes match mode
func (h *Handler) validateMatchMode(matchMode string) string {
	switch matchMode {
	case "keyword", "regex", "semantic", "exact":
		return matchMode
	default:
		return "keyword" // Default match mode
	}
}

// saveFilter saves the filter to storage
func (h *Handler) saveFilter(ctx *apptheory.Context, filter *storage.Filter) error {
	return h.repos.Moderation().CreateFilter(ctx.Context(), filter)
}

// addFilterKeywords adds keywords to a filter
func (h *Handler) addFilterKeywords(ctx *apptheory.Context, filterID string, keywordsAttributes []apimodels.FilterKeywordAttribute) []*storage.FilterKeyword {
	keywords := make([]*storage.FilterKeyword, 0)

	if err := common.ValidateSliceNotEmpty("keywordsAttributes", keywordsAttributes); err != nil {
		return keywords
	}

	for _, kwAttr := range keywordsAttributes {
		kw := h.extractFilterKeyword(kwAttr)
		if kw == nil {
			continue
		}

		if err := h.repos.Moderation().AddFilterKeyword(ctx.Context(), filterID, kw); err != nil {
			h.logger.Error("failed to add filter keyword", zap.Error(err))
			// Continue with other keywords
		} else {
			keywords = append(keywords, kw)
		}
	}

	return keywords
}

// extractFilterKeyword extracts a keyword from attributes map
func (h *Handler) extractFilterKeyword(kwAttr apimodels.FilterKeywordAttribute) *storage.FilterKeyword {
	keyword := kwAttr.Keyword
	if common.ValidateFilterKeyword(keyword) != nil {
		return nil
	}

	wholeWord := kwAttr.WholeWord

	isRegex := false
	if kwAttr.IsRegex != nil {
		isRegex = *kwAttr.IsRegex
	}

	matchWeight := 1.0 // Default weight
	if kwAttr.MatchWeight != nil && *kwAttr.MatchWeight >= 0.0 && *kwAttr.MatchWeight <= 1.0 {
		matchWeight = *kwAttr.MatchWeight
	}

	contextTypes := append([]string(nil), kwAttr.ContextTypes...)

	return &storage.FilterKeyword{
		Keyword:      keyword,
		WholeWord:    wholeWord,
		IsRegex:      isRegex,
		MatchWeight:  matchWeight,
		ContextTypes: contextTypes,
	}
}

// HandleUpdateFilterLift handles PUT /api/v2/filters/:id
func (h *Handler) HandleUpdateFilterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Parse request parameters
	params, err := h.parseFilterUpdateParams(ctx)
	if err != nil {
		return common.RespondValidationError(ctx, err)
	}

	if err := h.validateKeywordUpdates(ctx, filterID, params); err != nil {
		if errors.Is(err, errFilterKeywordNotFound) {
			return h.respondNotFound(ctx, "filter keyword")
		}
		h.logger.Error("failed to verify filter keyword ownership", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Build filter updates
	updates := h.buildFilterUpdates(params)

	// Update the filter
	if err := h.repos.Moderation().UpdateFilter(ctx.Context(), filterID, updates); err != nil {
		h.logger.Error("failed to update filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Handle keyword updates
	if err := h.handleKeywordUpdates(ctx, filterID, params); err != nil {
		if errors.Is(err, errFilterKeywordNotFound) {
			return h.respondNotFound(ctx, "filter keyword")
		}
		h.logger.Error("failed to update filter keywords", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return updated filter
	return h.returnUpdatedFilter(ctx, filterID)
}

// Helper functions for HandleUpdateFilterLift

// parseFilterUpdateParams parses request parameters for filter updates
func (h *Handler) parseFilterUpdateParams(ctx *apptheory.Context) (map[string]any, error) {
	var params map[string]any

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		if err2 := common.ParseRequestBody(ctx.Request.Body, &params); err2 != nil {
			return nil, err
		}
	}

	return params, nil
}

// buildFilterUpdates builds the updates map from parameters
func (h *Handler) buildFilterUpdates(params map[string]any) map[string]any {
	updates := make(map[string]any)

	if title, ok := params["title"].(string); ok {
		updates["title"] = title
	}

	if context, ok := params["context"].([]any); ok {
		contextStrings := make([]string, 0, len(context))
		for _, c := range context {
			if str, ok := c.(string); ok {
				contextStrings = append(contextStrings, str)
			}
		}
		updates["context"] = contextStrings
	}

	if filterAction, ok := params["filter_action"].(string); ok {
		updates["filter_action"] = filterAction
	}

	if expiresIn, ok := params["expires_in"].(float64); ok {
		expiresAt := time.Now().Add(time.Duration(int(expiresIn)) * time.Second)
		updates["expires_at"] = &expiresAt
	}

	return updates
}

func (h *Handler) validateKeywordUpdates(ctx *apptheory.Context, filterID string, params map[string]any) error {
	keywordsAttrs, ok := params["keywords_attributes"].([]any)
	if !ok {
		return nil
	}

	for _, kwAttr := range keywordsAttrs {
		kwMap, ok := kwAttr.(map[string]any)
		if !ok {
			continue
		}
		keywordID, hasID := kwMap["id"].(string)
		if !hasID || keywordID == "" {
			continue
		}
		if err := h.ensureFilterKeywordBelongsToFilter(ctx, filterID, keywordID); err != nil {
			return err
		}
	}

	return nil
}

// handleKeywordUpdates handles keyword updates for a filter
func (h *Handler) handleKeywordUpdates(ctx *apptheory.Context, filterID string, params map[string]any) error {
	keywordsAttrs, ok := params["keywords_attributes"].([]any)
	if !ok {
		return nil
	}

	for _, kwAttr := range keywordsAttrs {
		kwMap, ok := kwAttr.(map[string]any)
		if !ok {
			continue
		}

		if err := h.processKeywordUpdate(ctx, filterID, kwMap); err != nil {
			return err
		}
	}

	return nil
}

// processKeywordUpdate processes a single keyword update
func (h *Handler) processKeywordUpdate(ctx *apptheory.Context, filterID string, kwMap map[string]any) error {
	if id, hasID := kwMap["id"].(string); hasID {
		// Update or delete existing keyword
		if destroy, ok := kwMap["_destroy"].(bool); ok && destroy {
			return h.deleteFilterKeyword(ctx, filterID, id)
		}
		return h.updateFilterKeyword(ctx, filterID, id, kwMap)
	}

	// Create new keyword
	h.createFilterKeyword(ctx, filterID, kwMap)

	return nil
}

// deleteFilterKeyword deletes a filter keyword
func (h *Handler) deleteFilterKeyword(ctx *apptheory.Context, filterID string, keywordID string) error {
	if err := h.ensureFilterKeywordBelongsToFilter(ctx, filterID, keywordID); err != nil {
		return err
	}
	if err := h.repos.Moderation().DeleteFilterKeyword(ctx.Context(), keywordID); err != nil {
		h.logger.Error("failed to delete filter keyword", zap.String("keyword_id", keywordID), zap.Error(err))
		return err
	}
	return nil
}

// updateFilterKeyword updates a filter keyword
func (h *Handler) updateFilterKeyword(ctx *apptheory.Context, filterID string, keywordID string, kwMap map[string]any) error {
	if err := h.ensureFilterKeywordBelongsToFilter(ctx, filterID, keywordID); err != nil {
		return err
	}
	kwUpdates := make(map[string]any)
	if keyword, ok := kwMap["keyword"].(string); ok {
		kwUpdates["keyword"] = keyword
	}
	if wholeWord, ok := kwMap["whole_word"].(bool); ok {
		kwUpdates["whole_word"] = wholeWord
	}
	if err := h.repos.Moderation().UpdateFilterKeyword(ctx.Context(), keywordID, kwUpdates); err != nil {
		h.logger.Error("failed to update filter keyword", zap.String("keyword_id", keywordID), zap.Error(err))
		return err
	}
	return nil
}

// createFilterKeyword creates a new filter keyword
func (h *Handler) createFilterKeyword(ctx *apptheory.Context, filterID string, kwMap map[string]any) {
	keyword, ok := kwMap["keyword"].(string)
	if !ok || common.ValidateFilterKeyword(keyword) != nil {
		return
	}

	wholeWord := false
	if ww, ok := kwMap["whole_word"].(bool); ok {
		wholeWord = ww
	}

	kw := &storage.FilterKeyword{
		Keyword:   keyword,
		WholeWord: wholeWord,
	}

	if err := h.repos.Moderation().AddFilterKeyword(ctx.Context(), filterID, kw); err != nil {
		h.logger.Error("failed to add filter keyword", zap.Error(err))
	}
}

func (h *Handler) ensureFilterKeywordBelongsToFilter(ctx *apptheory.Context, filterID string, keywordID string) error {
	keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context(), filterID)
	if err != nil {
		return err
	}

	for _, keyword := range keywords {
		if keyword == nil {
			continue
		}
		if keyword.ID == keywordID && keyword.FilterID == filterID {
			return nil
		}
	}

	return errFilterKeywordNotFound
}

// returnUpdatedFilter returns the updated filter with keywords and statuses
func (h *Handler) returnUpdatedFilter(ctx *apptheory.Context, filterID string) (*apptheory.Response, error) {
	updatedFilter, _ := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	keywords, _ := h.repos.Moderation().GetFilterKeywords(ctx.Context(), filterID)
	statuses, _ := h.repos.Moderation().GetFilterStatuses(ctx.Context(), filterID)

	mastodonFilter := h.converter.ConvertFilterToMastodon(updatedFilter, keywords, statuses)
	return okJSON(mastodonFilter)
}

// HandleDeleteFilterLift handles DELETE /api/v2/filters/:id
func (h *Handler) HandleDeleteFilterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the filter (this should cascade delete keywords and statuses)
	if err := h.repos.Moderation().DeleteFilter(ctx.Context(), filterID); err != nil {
		h.logger.Error("failed to delete filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return okJSON(apimodels.EmptyObject{})
}

// HandleGetFilterKeywordsLift handles GET /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleGetFilterKeywordsLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get keywords
	keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter keywords", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert to Mastodon API format
	result := make([]mastodon.FilterKeyword, 0, len(keywords))
	for _, keyword := range keywords {
		result = append(result, mastodon.FilterKeyword{
			ID:        keyword.ID,
			Keyword:   keyword.Keyword,
			WholeWord: keyword.WholeWord,
		})
	}

	return okJSON(result)
}

// HandleGetFilterStatusesLift handles GET /api/v2/filters/:filter_id/statuses
func (h *Handler) HandleGetFilterStatusesLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get statuses
	statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter statuses", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert to Mastodon API format
	result := make([]mastodon.FilterStatus, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, mastodon.FilterStatus{
			ID:       status.ID,
			StatusID: status.StatusID,
		})
	}

	return okJSON(result)
}

// HandleAddFilterKeywordLift handles POST /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleAddFilterKeywordLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Parse request body
	var params apimodels.AddFilterKeywordRequest

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		if err2 := common.ParseRequestBody(ctx.Request.Body, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	if err := common.ValidateFilterKeyword(params.Keyword); err != nil {
		return h.respondUnprocessableEntity(ctx, err.Error())
	}

	// Create the keyword
	keyword := &storage.FilterKeyword{
		Keyword:   params.Keyword,
		WholeWord: params.WholeWord,
	}

	if err := h.repos.Moderation().AddFilterKeyword(ctx.Context(), filterID, keyword); err != nil {
		h.logger.Error("failed to add filter keyword", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return the created keyword
	result := mastodon.FilterKeyword{
		ID:        keyword.ID,
		Keyword:   keyword.Keyword,
		WholeWord: keyword.WholeWord,
	}

	return okJSON(result)
}

// HandleDeleteFilterKeywordLift handles DELETE /api/v2/filters/:filter_id/keywords/:keyword_id
func (h *Handler) HandleDeleteFilterKeywordLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	keywordID := ctx.Param("keyword_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	if err := common.ValidateKeywordParamID(keywordID); err != nil {
		return common.RespondValidationError(ctx, err)
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the keyword only after proving it belongs to the owned filter.
	if err := h.ensureFilterKeywordBelongsToFilter(ctx, filterID, keywordID); err != nil {
		if errors.Is(err, errFilterKeywordNotFound) {
			return h.respondNotFound(ctx, "filter keyword")
		}
		h.logger.Error("failed to verify filter keyword ownership", zap.String("filter_id", filterID), zap.String("keyword_id", keywordID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}
	if err := h.repos.Moderation().DeleteFilterKeyword(ctx.Context(), keywordID); err != nil {
		h.logger.Error("failed to delete filter keyword", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return okJSON(apimodels.EmptyObject{})
}

// HandleAddFilterStatusLift handles POST /api/v2/filters/:filter_id/statuses
func (h *Handler) HandleAddFilterStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Parse request body
	var params apimodels.AddFilterStatusRequest

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		if err2 := common.ParseRequestBody(ctx.Request.Body, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	if err := common.ValidateRequiredParam("statusID", params.StatusID); err != nil {
		return h.respondUnprocessableEntity(ctx, "status_id can't be blank")
	}

	// Create the filter status
	filterStatus := &storage.FilterStatus{
		StatusID: params.StatusID,
	}

	if err := h.repos.Moderation().AddFilterStatus(ctx.Context(), filterID, filterStatus); err != nil {
		h.logger.Error("failed to add filter status", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return the created filter status
	result := mastodon.FilterStatus{
		ID:       filterStatus.ID,
		StatusID: filterStatus.StatusID,
	}

	return okJSON(result)
}

// HandleDeleteFilterStatusLift handles DELETE /api/v2/filters/:filter_id/statuses/:status_id
func (h *Handler) HandleDeleteFilterStatusLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	filterID := ctx.Param("filter_id")
	statusID := ctx.Param("status_id")
	if err := common.ValidateRequiredParam("filterID", filterID); err != nil {
		return common.RespondBadRequest(ctx, "missing filter id")
	}
	if err := common.ValidateRequiredParam("statusID", statusID); err != nil {
		return common.RespondBadRequest(ctx, "missing status id")
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context(), filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the filter status
	if err := h.repos.Moderation().DeleteFilterStatus(ctx.Context(), statusID); err != nil {
		h.logger.Error("failed to delete filter status", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return okJSON(apimodels.EmptyObject{})
}

// HandleTestFilterLift handles POST /api/v2/filters/test
// Tests filter rules against provided content
func (h *Handler) HandleTestFilterLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if isInsufficientScopeError(err) {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse request body
	var params apimodels.TestFilterRequest

	if err := common.ParseRequestWithFallback(ctx, &params); err != nil {
		if err2 := common.ParseRequestBody(ctx.Request.Body, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	if err := common.ValidateRequiredParam("content", params.Content); err != nil {
		return h.respondUnprocessableEntity(ctx, "content can't be blank")
	}

	// Get user's filters
	storageFilters, err := h.repos.Moderation().GetFiltersForUser(ctx.Context(), username)
	if err != nil {
		h.logger.Error("failed to get user filters", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert storage.Filter to models.Filter for the filter engine
	modelFilters := make([]*storageModels.Filter, len(storageFilters))
	for i, sf := range storageFilters {
		modelFilters[i] = &storageModels.Filter{
			ID:            sf.ID,
			Username:      sf.Username,
			Title:         sf.Title,
			Context:       sf.Context,
			FilterAction:  sf.FilterAction,
			Severity:      sf.Severity,
			MatchMode:     sf.MatchMode,
			CaseSensitive: sf.CaseSensitive,
			ExpiresAt:     sf.ExpiresAt,
			CreatedAt:     sf.CreatedAt,
			UpdatedAt:     sf.UpdatedAt,
		}
	}

	// Test content against filters using advanced filter engine
	filterEngine := moderation.NewAdvancedFilterEngine(h.logger)

	contentCtx := &moderation.ContentContext{
		Type:       "test", // Special context for testing
		Timestamp:  time.Now(),
		IsReply:    false,
		HasMedia:   false,
		Language:   "en",
		Visibility: "public",
	}

	results, err := filterEngine.EvaluateContent(ctx.Context(), params.Content, modelFilters, contentCtx)
	if err != nil {
		h.logger.Error("failed to evaluate content", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return filter test results
	return okJSON(apimodels.FilterTestResponse{
		Content:      params.Content,
		TotalFilters: len(storageFilters),
		MatchedCount: len(results),
		Results:      results,
	})
}
