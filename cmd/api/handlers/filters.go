package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/mastodon"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/aws/aws-lambda-go/events"
	"go.uber.org/zap"
)

// HandleGetFilters handles GET /api/v2/filters
func (h *Handler) HandleGetFilters(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:filters scope
	if !claims.HasScope("read:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get all filters for the user
	filters, err := h.store.GetFiltersForUser(ctx, claims.Username)
	if err != nil {
		h.logger.Error("failed to get filters", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Convert to Mastodon API format
	result := make([]*mastodon.Filter, 0, len(filters))
	for _, filter := range filters {
		// Get keywords and statuses for each filter
		keywords, err := h.store.GetFilterKeywords(ctx, filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		statuses, err := h.store.GetFilterStatuses(ctx, filter.ID)
		if err != nil {
			h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
			continue
		}

		mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)
		result = append(result, mastodonFilter)
	}

	body, _ := json.Marshal(result)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleGetFilter handles GET /api/v2/filters/:id
func (h *Handler) HandleGetFilter(ctx context.Context, request events.APIGatewayV2HTTPRequest, filterID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:filters scope
	if !claims.HasScope("read:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the filter
	filter, err := h.store.GetFilter(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if filter == nil || filter.Username != claims.Username {
		return common.NotFound(fmt.Errorf("filter not found")), nil
	}

	// Get keywords and statuses
	keywords, err := h.store.GetFilterKeywords(ctx, filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter keywords", zap.String("filter_id", filter.ID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	statuses, err := h.store.GetFilterStatuses(ctx, filter.ID)
	if err != nil {
		h.logger.Error("failed to get filter statuses", zap.String("filter_id", filter.ID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, statuses)

	body, _ := json.Marshal(mastodonFilter)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleCreateFilter handles POST /api/v2/filters
func (h *Handler) HandleCreateFilter(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:filters scope
	if !claims.HasScope("write:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Parse request body
	var params struct {
		Title              string           `json:"title"`
		Context            []string         `json:"context"`
		FilterAction       string           `json:"filter_action"`
		ExpiresIn          *int             `json:"expires_in"`
		KeywordsAttributes []map[string]any `json:"keywords_attributes"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &params); err != nil {
		return common.BadRequest(err), nil
	}

	// Validate required fields
	if params.Title == "" {
		return common.UnprocessableEntity(fmt.Errorf("title can't be blank")), nil
	}
	if len(params.Context) == 0 {
		return common.UnprocessableEntity(fmt.Errorf("context can't be blank")), nil
	}

	// Validate context values
	validContexts := map[string]bool{
		"home":          true,
		"notifications": true,
		"public":        true,
		"thread":        true,
		"account":       true,
	}
	for _, ctx := range params.Context {
		if !validContexts[ctx] {
			return common.UnprocessableEntity(fmt.Errorf("invalid context supplied")), nil
		}
	}

	// Set default filter action if not provided
	if params.FilterAction == "" {
		params.FilterAction = "warn"
	}

	// Validate filter action
	if params.FilterAction != "warn" && params.FilterAction != "hide" && params.FilterAction != "blur" {
		return common.UnprocessableEntity(fmt.Errorf("invalid filter_action")), nil
	}

	// Create the filter
	filter := &storage.Filter{
		Username:     claims.Username,
		Title:        params.Title,
		Context:      params.Context,
		FilterAction: params.FilterAction,
	}

	// Set expiration if provided
	if params.ExpiresIn != nil && *params.ExpiresIn > 0 {
		expiresAt := time.Now().Add(time.Duration(*params.ExpiresIn) * time.Second)
		filter.ExpiresAt = &expiresAt
	}

	if err := h.store.CreateFilter(ctx, filter); err != nil {
		h.logger.Error("failed to create filter", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Add keywords if provided
	keywords := make([]*storage.FilterKeyword, 0)
	if len(params.KeywordsAttributes) > 0 {
		for _, kwAttr := range params.KeywordsAttributes {
			keyword, ok := kwAttr["keyword"].(string)
			if !ok || keyword == "" {
				continue
			}

			wholeWord := false
			if ww, ok := kwAttr["whole_word"].(bool); ok {
				wholeWord = ww
			}

			kw := &storage.FilterKeyword{
				Keyword:   keyword,
				WholeWord: wholeWord,
			}

			if err := h.store.AddFilterKeyword(ctx, filter.ID, kw); err != nil {
				h.logger.Error("failed to add filter keyword", zap.Error(err))
				// Continue with other keywords
			} else {
				keywords = append(keywords, kw)
			}
		}
	}

	// Convert to Mastodon API format
	mastodonFilter := h.converter.ConvertFilterToMastodon(filter, keywords, []*storage.FilterStatus{})

	body, _ := json.Marshal(mastodonFilter)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleUpdateFilter handles PUT /api/v2/filters/:id
func (h *Handler) HandleUpdateFilter(ctx context.Context, request events.APIGatewayV2HTTPRequest, filterID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:filters scope
	if !claims.HasScope("write:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the existing filter
	filter, err := h.store.GetFilter(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if filter == nil || filter.Username != claims.Username {
		return common.NotFound(fmt.Errorf("filter not found")), nil
	}

	// Parse request body
	var params map[string]any
	if err := common.ParseRequestBody([]byte(request.Body), &params); err != nil {
		return common.BadRequest(err), nil
	}

	// Build updates map
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

	// Update the filter
	if err := h.store.UpdateFilter(ctx, filterID, updates); err != nil {
		h.logger.Error("failed to update filter", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Handle keyword updates if provided
	if keywordsAttrs, ok := params["keywords_attributes"].([]any); ok {
		for _, kwAttr := range keywordsAttrs {
			kwMap, ok := kwAttr.(map[string]any)
			if !ok {
				continue
			}

			// Check if this is an update, create, or delete
			if id, hasID := kwMap["id"].(string); hasID {
				if destroy, ok := kwMap["_destroy"].(bool); ok && destroy {
					// Delete the keyword
					if err := h.store.DeleteFilterKeyword(ctx, id); err != nil {
						h.logger.Error("failed to delete filter keyword", zap.String("keyword_id", id), zap.Error(err))
					}
				} else {
					// Update the keyword
					kwUpdates := make(map[string]any)
					if keyword, ok := kwMap["keyword"].(string); ok {
						kwUpdates["keyword"] = keyword
					}
					if wholeWord, ok := kwMap["whole_word"].(bool); ok {
						kwUpdates["whole_word"] = wholeWord
					}
					if err := h.store.UpdateFilterKeyword(ctx, id, kwUpdates); err != nil {
						h.logger.Error("failed to update filter keyword", zap.String("keyword_id", id), zap.Error(err))
					}
				}
			} else {
				// Create new keyword
				keyword, ok := kwMap["keyword"].(string)
				if !ok || keyword == "" {
					continue
				}

				wholeWord := false
				if ww, ok := kwMap["whole_word"].(bool); ok {
					wholeWord = ww
				}

				kw := &storage.FilterKeyword{
					Keyword:   keyword,
					WholeWord: wholeWord,
				}

				if err := h.store.AddFilterKeyword(ctx, filterID, kw); err != nil {
					h.logger.Error("failed to add filter keyword", zap.Error(err))
				}
			}
		}
	}

	// Get updated filter with keywords and statuses
	updatedFilter, _ := h.store.GetFilter(ctx, filterID)
	keywords, _ := h.store.GetFilterKeywords(ctx, filterID)
	statuses, _ := h.store.GetFilterStatuses(ctx, filterID)

	mastodonFilter := h.converter.ConvertFilterToMastodon(updatedFilter, keywords, statuses)

	body, _ := json.Marshal(mastodonFilter)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleDeleteFilter handles DELETE /api/v2/filters/:id
func (h *Handler) HandleDeleteFilter(ctx context.Context, request events.APIGatewayV2HTTPRequest, filterID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:filters scope
	if !claims.HasScope("write:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the filter to verify ownership
	filter, err := h.store.GetFilter(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if filter == nil || filter.Username != claims.Username {
		return common.NotFound(fmt.Errorf("filter not found")), nil
	}

	// Delete the filter (this should cascade delete keywords and statuses)
	if err := h.store.DeleteFilter(ctx, filterID); err != nil {
		h.logger.Error("failed to delete filter", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: "{}",
	}, nil
}

// HandleGetFilterKeywords handles GET /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleGetFilterKeywords(ctx context.Context, request events.APIGatewayV2HTTPRequest, filterID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check read:filters scope
	if !claims.HasScope("read:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the filter to verify ownership
	filter, err := h.store.GetFilter(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if filter == nil || filter.Username != claims.Username {
		return common.NotFound(fmt.Errorf("filter not found")), nil
	}

	// Get keywords
	keywords, err := h.store.GetFilterKeywords(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter keywords", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
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

	body, _ := json.Marshal(result)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

// HandleAddFilterKeyword handles POST /api/v2/filters/:filter_id/keywords
func (h *Handler) HandleAddFilterKeyword(ctx context.Context, request events.APIGatewayV2HTTPRequest, filterID string) (*events.APIGatewayV2HTTPResponse, error) {
	// Extract token from Authorization header
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Validate token and get claims
	oauthSvc := auth.NewOAuthService(h.cfg.JWTSecret, h.store)
	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(err), nil
	}

	// Check write:filters scope
	if !claims.HasScope("write:filters") {
		return common.Forbidden(fmt.Errorf("insufficient scope")), nil
	}

	// Get the filter to verify ownership
	filter, err := h.store.GetFilter(ctx, filterID)
	if err != nil {
		h.logger.Error("failed to get filter", zap.String("filter_id", filterID), zap.Error(err))
		return common.InternalServerError(err), nil
	}

	if filter == nil || filter.Username != claims.Username {
		return common.NotFound(fmt.Errorf("filter not found")), nil
	}

	// Parse request body
	var params struct {
		Keyword   string `json:"keyword"`
		WholeWord bool   `json:"whole_word"`
	}

	if err := common.ParseRequestBody([]byte(request.Body), &params); err != nil {
		return common.BadRequest(err), nil
	}

	if params.Keyword == "" {
		return common.UnprocessableEntity(fmt.Errorf("keyword can't be blank")), nil
	}

	// Create the keyword
	keyword := &storage.FilterKeyword{
		Keyword:   params.Keyword,
		WholeWord: params.WholeWord,
	}

	if err := h.store.AddFilterKeyword(ctx, filterID, keyword); err != nil {
		h.logger.Error("failed to add filter keyword", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return the created keyword
	result := mastodon.FilterKeyword{
		ID:        keyword.ID,
		Keyword:   keyword.Keyword,
		WholeWord: keyword.WholeWord,
	}

	body, _ := json.Marshal(result)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}
