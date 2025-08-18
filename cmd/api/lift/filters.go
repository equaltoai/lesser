package lift

import (
	"time"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/moderation"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/pay-theory/lift/pkg/lift"
	"go.uber.org/zap"
)

// HandleGetFiltersLift handles GET /api/v2/filters
func (h *Handler) HandleGetFiltersLift(ctx *lift.Context) error {
	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get all filters for the user
	filters, err := h.repos.Moderation().GetFiltersForUser(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get filters", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert to Mastodon API format
	result := make([]*mastodon.Filter, 0, len(filters))
	for _, filter := range filters {
		// Get keywords and statuses for each filter
		keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context, filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context, filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)
		result = append(result, mastodonFilter)
	}

	return ctx.Status(200).JSON(result)
}

// HandleGetFilterLift handles GET /api/v2/filters/:id
func (h *Handler) HandleGetFilterLift(ctx *lift.Context) error {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get keywords and statuses
	keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context, filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context, filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)
	return ctx.Status(200).JSON(mastodonFilter)
}

// HandleCreateFilterLift handles POST /api/v2/filters
func (h *Handler) HandleCreateFilterLift(ctx *lift.Context) error {
	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse request
	var params createFilterParams
	if err := ctx.ParseRequest(&params); err != nil {
		return common.RespondInvalidRequest(ctx)
	}

	// Validate filter parameters using comprehensive validation
	filterParams := map[string]interface{}{
		"title":         params.Title,
		"context":       params.Context,
		"filter_action": params.FilterAction,
		"expires_in":    params.ExpiresIn,
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
		return err
	}

	// Add keywords if provided
	keywords := h.addFilterKeywords(ctx, filter.ID, params.KeywordsAttributes)

	// Convert to Mastodon API format
	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, []*storage.FilterStatus{})
	return ctx.Status(200).JSON(mastodonFilter)
}

// createFilterParams holds parameters for filter creation
type createFilterParams struct {
	Title              string           `json:"title"`
	Context            []string         `json:"context"`
	FilterAction       string           `json:"filter_action"`
	Severity           string           `json:"severity"`       // New: Filter severity
	MatchMode          string           `json:"match_mode"`     // New: Matching mode
	CaseSensitive      bool             `json:"case_sensitive"` // New: Case-sensitive matching
	ExpiresIn          *int             `json:"expires_in"`
	KeywordsAttributes []map[string]any `json:"keywords_attributes"`
}

// parseCreateFilterRequest parses and validates the filter creation request
func (h *Handler) parseCreateFilterRequest(ctx *lift.Context) (*createFilterParams, error) {
	var params createFilterParams

	// Parse request body
	if err := h.parseFilterRequestBody(ctx, &params); err != nil {
		return nil, err
	}

	// Validate required fields
	if err := h.validateFilterRequiredFields(ctx, &params); err != nil {
		return nil, err
	}

	// Context validation is already handled in validateFilterRequiredFields

	// Validate and set filter action
	if err := h.validateFilterAction(ctx, &params); err != nil {
		return nil, err
	}

	return &params, nil
}

// parseFilterRequestBody parses the request body
func (h *Handler) parseFilterRequestBody(ctx *lift.Context, params *createFilterParams) error {
	if err := ctx.ParseRequest(params); err != nil {
		// Fallback to common.ParseRequestBody for test environments
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}
	return nil
}

// validateFilterRequiredFields validates required fields
func (h *Handler) validateFilterRequiredFields(ctx *lift.Context, params *createFilterParams) error {
	if err := common.ValidateFilterTitle(params.Title); err != nil {
		return h.respondUnprocessableEntity(ctx, err.Error())
	}
	if err := common.ValidateFilterContext(params.Context); err != nil {
		return h.respondUnprocessableEntity(ctx, err.Error())
	}
	return nil
}

// validateFilterAction validates and sets default filter action
func (h *Handler) validateFilterAction(ctx *lift.Context, params *createFilterParams) error {
	// Set default if not provided
	if err := common.ValidateRequiredParam("filterAction", params.FilterAction); err != nil {
		params.FilterAction = "warn"
	}

	// Validate filter action
	if err := common.ValidateFilterAction(params.FilterAction); err != nil {
		return h.respondUnprocessableEntity(ctx, err.Error())
	}

	return nil
}

// buildFilterFromParams builds a Filter object from request parameters
func (h *Handler) buildFilterFromParams(username string, params *createFilterParams) *storage.Filter {
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
func (h *Handler) saveFilter(ctx *lift.Context, filter *storage.Filter) error {
	if err := h.repos.Moderation().CreateFilter(ctx.Context, filter); err != nil {
		h.logger.Error("failed to create filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}
	return nil
}

// addFilterKeywords adds keywords to a filter
func (h *Handler) addFilterKeywords(ctx *lift.Context, filterID string, keywordsAttributes []map[string]any) []*storage.FilterKeyword {
	keywords := make([]*storage.FilterKeyword, 0)

	if err := common.ValidateSliceNotEmpty("keywordsAttributes", keywordsAttributes); err != nil {
		return keywords
	}

	for _, kwAttr := range keywordsAttributes {
		kw := h.extractFilterKeyword(kwAttr)
		if kw == nil {
			continue
		}

		if err := h.repos.Moderation().AddFilterKeyword(ctx.Context, filterID, kw); err != nil {
			h.logger.Error("failed to add filter keyword", zap.Error(err))
			// Continue with other keywords
		} else {
			keywords = append(keywords, kw)
		}
	}

	return keywords
}

// extractFilterKeyword extracts a keyword from attributes map
func (h *Handler) extractFilterKeyword(kwAttr map[string]any) *storage.FilterKeyword {
	keyword, ok := kwAttr["keyword"].(string)
	if !ok || common.ValidateFilterKeyword(keyword) != nil {
		return nil
	}

	wholeWord := false
	if ww, ok := kwAttr["whole_word"].(bool); ok {
		wholeWord = ww
	}

	isRegex := false
	if ir, ok := kwAttr["is_regex"].(bool); ok {
		isRegex = ir
	}

	matchWeight := 1.0 // Default weight
	if mw, ok := kwAttr["match_weight"].(float64); ok && mw >= 0.0 && mw <= 1.0 {
		matchWeight = mw
	}

	var contextTypes []string
	if ct, ok := kwAttr["context_types"].([]interface{}); ok {
		for _, c := range ct {
			if contextStr, ok := c.(string); ok {
				contextTypes = append(contextTypes, contextStr)
			}
		}
	}

	return &storage.FilterKeyword{
		Keyword:      keyword,
		WholeWord:    wholeWord,
		IsRegex:      isRegex,
		MatchWeight:  matchWeight,
		ContextTypes: contextTypes,
	}
}

// HandleUpdateFilterLift handles PUT /api/v2/filters/:id
func (h *Handler) HandleUpdateFilterLift(ctx *lift.Context) error {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
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
		return err
	}

	// Build filter updates
	updates := h.buildFilterUpdates(params)

	// Update the filter
	if err := h.repos.Moderation().UpdateFilter(ctx.Context, filterID, updates); err != nil {
		h.logger.Error("failed to update filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Handle keyword updates
	h.handleKeywordUpdates(ctx, filterID, params)

	// Return updated filter
	return h.returnUpdatedFilter(ctx, filterID)
}

// Helper functions for HandleUpdateFilterLift

// parseFilterUpdateParams parses request parameters for filter updates
func (h *Handler) parseFilterUpdateParams(ctx *lift.Context) (map[string]any, error) {
	var params map[string]any

	if err := ctx.ParseRequest(&params); err != nil {
		// Fallback to common.ParseRequestBody for test environments
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
			return nil, common.RespondValidationError(ctx, err)
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

// handleKeywordUpdates handles keyword updates for a filter
func (h *Handler) handleKeywordUpdates(ctx *lift.Context, filterID string, params map[string]any) {
	keywordsAttrs, ok := params["keywords_attributes"].([]any)
	if !ok {
		return
	}

	for _, kwAttr := range keywordsAttrs {
		kwMap, ok := kwAttr.(map[string]any)
		if !ok {
			continue
		}

		h.processKeywordUpdate(ctx, filterID, kwMap)
	}
}

// processKeywordUpdate processes a single keyword update
func (h *Handler) processKeywordUpdate(ctx *lift.Context, filterID string, kwMap map[string]any) {
	if id, hasID := kwMap["id"].(string); hasID {
		// Update or delete existing keyword
		if destroy, ok := kwMap["_destroy"].(bool); ok && destroy {
			h.deleteFilterKeyword(ctx, id)
		} else {
			h.updateFilterKeyword(ctx, id, kwMap)
		}
	} else {
		// Create new keyword
		h.createFilterKeyword(ctx, filterID, kwMap)
	}
}

// deleteFilterKeyword deletes a filter keyword
func (h *Handler) deleteFilterKeyword(ctx *lift.Context, keywordID string) {
	if err := h.repos.Moderation().DeleteFilterKeyword(ctx.Context, keywordID); err != nil {
		h.logger.Error("failed to delete filter keyword", zap.String("keyword_id", keywordID), zap.Error(err))
	}
}

// updateFilterKeyword updates a filter keyword
func (h *Handler) updateFilterKeyword(ctx *lift.Context, keywordID string, kwMap map[string]any) {
	kwUpdates := make(map[string]any)
	if keyword, ok := kwMap["keyword"].(string); ok {
		kwUpdates["keyword"] = keyword
	}
	if wholeWord, ok := kwMap["whole_word"].(bool); ok {
		kwUpdates["whole_word"] = wholeWord
	}
	if err := h.repos.Moderation().UpdateFilterKeyword(ctx.Context, keywordID, kwUpdates); err != nil {
		h.logger.Error("failed to update filter keyword", zap.String("keyword_id", keywordID), zap.Error(err))
	}
}

// createFilterKeyword creates a new filter keyword
func (h *Handler) createFilterKeyword(ctx *lift.Context, filterID string, kwMap map[string]any) {
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

	if err := h.repos.Moderation().AddFilterKeyword(ctx.Context, filterID, kw); err != nil {
		h.logger.Error("failed to add filter keyword", zap.Error(err))
	}
}

// returnUpdatedFilter returns the updated filter with keywords and statuses
func (h *Handler) returnUpdatedFilter(ctx *lift.Context, filterID string) error {
	updatedFilter, _ := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	keywords, _ := h.repos.Moderation().GetFilterKeywords(ctx.Context, filterID)
	statuses, _ := h.repos.Moderation().GetFilterStatuses(ctx.Context, filterID)

	mastodonFilter := h.converter.ConvertFilterToMastodon(updatedFilter, keywords, statuses)
	return ctx.Status(200).JSON(mastodonFilter)
}

// HandleDeleteFilterLift handles DELETE /api/v2/filters/:id
func (h *Handler) HandleDeleteFilterLift(ctx *lift.Context) error {
	filterID := ctx.Param("id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the filter (this should cascade delete keywords and statuses)
	if err := h.repos.Moderation().DeleteFilter(ctx.Context, filterID); err != nil {
		h.logger.Error("failed to delete filter", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return ctx.Status(200).JSON(map[string]any{})
}

// HandleGetFilterKeywordsLift handles GET /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleGetFilterKeywordsLift(ctx *lift.Context) error {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get keywords
	keywords, err := h.repos.Moderation().GetFilterKeywords(ctx.Context, filterID)
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

	return ctx.Status(200).JSON(result)
}

// HandleGetFilterStatusesLift handles GET /api/v2/filters/:filter_id/statuses
func (h *Handler) HandleGetFilterStatusesLift(ctx *lift.Context) error {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Get statuses
	statuses, err := h.repos.Moderation().GetFilterStatuses(ctx.Context, filterID)
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

	return ctx.Status(200).JSON(result)
}

// HandleAddFilterKeywordLift handles POST /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleAddFilterKeywordLift(ctx *lift.Context) error {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Get the filter to verify ownership
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Parse request body
	var params struct {
		Keyword   string `json:"keyword"`
		WholeWord bool   `json:"whole_word"`
	}

	if err := ctx.ParseRequest(&params); err != nil {
		// Fallback to common.ParseRequestBody for test environments
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
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

	if err := h.repos.Moderation().AddFilterKeyword(ctx.Context, filterID, keyword); err != nil {
		h.logger.Error("failed to add filter keyword", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return the created keyword
	result := mastodon.FilterKeyword{
		ID:        keyword.ID,
		Keyword:   keyword.Keyword,
		WholeWord: keyword.WholeWord,
	}

	return ctx.Status(200).JSON(result)
}

// HandleDeleteFilterKeywordLift handles DELETE /api/v2/filters/:filter_id/keywords/:keyword_id
func (h *Handler) HandleDeleteFilterKeywordLift(ctx *lift.Context) error {
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
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the keyword
	if err := h.repos.Moderation().DeleteFilterKeyword(ctx.Context, keywordID); err != nil {
		h.logger.Error("failed to delete filter keyword", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return ctx.Status(200).JSON(map[string]string{})
}

// HandleAddFilterStatusLift handles POST /api/v2/filters/:filter_id/statuses
func (h *Handler) HandleAddFilterStatusLift(ctx *lift.Context) error {
	filterID := ctx.Param("filter_id")
	if err := common.ValidateFilterParamID(filterID); err != nil {
		return h.respondBadRequest(ctx, err.Error())
	}

	// Authenticate user with write:filters scope
	username, err := h.authenticateUser(ctx, []string{"write:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Parse request body
	var params struct {
		StatusID string `json:"status_id"`
	}

	if err := ctx.ParseRequest(&params); err != nil {
		// Fallback to common.ParseRequestBody for test environments
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
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

	if err := h.repos.Moderation().AddFilterStatus(ctx.Context, filterID, filterStatus); err != nil {
		h.logger.Error("failed to add filter status", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return the created filter status
	result := mastodon.FilterStatus{
		ID:       filterStatus.ID,
		StatusID: filterStatus.StatusID,
	}

	return ctx.Status(200).JSON(result)
}

// HandleDeleteFilterStatusLift handles DELETE /api/v2/filters/:filter_id/statuses/:status_id
func (h *Handler) HandleDeleteFilterStatusLift(ctx *lift.Context) error {
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
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Verify the filter belongs to the user
	filter, err := h.repos.Moderation().GetFilter(ctx.Context, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	if filter == nil || filter.Username != username {
		return h.respondNotFound(ctx, "filter")
	}

	// Delete the filter status
	if err := h.repos.Moderation().DeleteFilterStatus(ctx.Context, statusID); err != nil {
		h.logger.Error("failed to delete filter status", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	return ctx.Status(200).JSON(map[string]string{})
}

// HandleTestFilterLift handles POST /api/v2/filters/test
// Tests filter rules against provided content
func (h *Handler) HandleTestFilterLift(ctx *lift.Context) error {
	// Authenticate user with read:filters scope
	username, err := h.authenticateUser(ctx, []string{"read:filters"})
	if err != nil {
		if err.Error() == "insufficient scope" {
			return h.respondInsufficientScope(ctx)
		}
		return h.respondUnauthorized(ctx)
	}

	// Parse request body
	var params struct {
		Content string   `json:"content"`
		Context []string `json:"context"`
	}

	if err := ctx.ParseRequest(&params); err != nil {
		// Fallback to common.ParseRequestBody for test environments
		bodyBytes := ctx.Request.Body
		if err := common.ValidateSliceNotEmpty("bodyBytes", bodyBytes); err != nil && ctx.Request != nil && ctx.Request.Request != nil {
			bodyBytes = ctx.Request.Request.Body
		}
		if err2 := common.ParseRequestBody(bodyBytes, &params); err2 != nil {
			return common.RespondValidationError(ctx, err)
		}
	}

	if err := common.ValidateRequiredParam("content", params.Content); err != nil {
		return h.respondUnprocessableEntity(ctx, "content can't be blank")
	}

	// Get user's filters
	storageFilters, err := h.repos.Moderation().GetFiltersForUser(ctx.Context, username)
	if err != nil {
		h.logger.Error("failed to get user filters", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Convert storage.Filter to models.Filter for the filter engine
	modelFilters := make([]*models.Filter, len(storageFilters))
	for i, sf := range storageFilters {
		modelFilters[i] = &models.Filter{
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

	results, err := filterEngine.EvaluateContent(ctx.Context, params.Content, modelFilters, contentCtx)
	if err != nil {
		h.logger.Error("failed to evaluate content", zap.Error(err))
		return h.respondInternalError(ctx, "internal server error")
	}

	// Return filter test results
	response := map[string]any{
		"content":       params.Content,
		"total_filters": len(storageFilters),
		"matched_count": len(results),
		"results":       results,
	}

	return ctx.Status(200).JSON(response)
}
