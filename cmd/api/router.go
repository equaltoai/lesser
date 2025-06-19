package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/aron23/lesser/cmd/api/handlers"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aws/aws-lambda-go/events"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// NewRouter creates a new chi router with all routes configured
func NewRouter(h *handlers.Handler, authMiddleware auth.Middleware, logger *zap.Logger) *chi.Mux {
	r := chi.NewRouter()

	// Global middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// Custom logging middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)

			defer func() {
				logger.Info("API request",
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Int("status", ww.Status()),
					zap.Duration("duration", time.Since(start)),
					zap.String("request_id", middleware.GetReqID(req.Context())))
			}()

			next.ServeHTTP(ww, req)
		})
	})

	// CORS middleware
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS, HEAD")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With, Accept, Accept-Encoding, Accept-Language, Date, Digest, Host, Signature, User-Agent, X-Forwarded-For, X-Forwarded-Proto, X-CSRF-Token")

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	})


	// Convert auth middleware to chi middleware
	authMiddlewareFunc := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create Lambda request for auth middleware
			lambdaReq := httpToLambdaRequest(r)

			// Validate auth
			claims, err := authMiddleware.RequireAuth(r.Context(), lambdaReq)
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add claims to context
			ctx := auth.WithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// Public routes (no auth required)
	r.Group(func(r chi.Router) {
		// OAuth endpoints
		r.Post("/apps", wrapHandler(h.HandleAppRegistration))
		r.Get("/oauth/authorize", wrapHandler(h.HandleOAuthAuthorize))
		r.Post("/oauth/token", wrapHandler(h.HandleOAuthToken))

		// Account registration
		r.Post("/accounts", wrapHandler(h.HandleRegistration))

		// Instance information
		r.Get("/instance", wrapHandler(h.HandleGetInstanceV1))
		r.Get("/instance/activity", wrapHandler(h.HandleGetInstanceActivity))
		r.Get("/instance/peers", wrapHandler(h.HandleGetInstancePeers))
		// r.Get("/instance/rules", wrapHandler(h.HandleGetInstanceRules))

		// Public timelines
		r.Get("/timelines/public", wrapHandler(h.HandlePublicTimeline))
		r.Get("/timelines/tag/{hashtag}", wrapHandlerWithParam(h.HandleHashtagTimeline, "hashtag"))

		// Webfinger and nodeinfo
		// TODO: Implement webfinger and nodeinfo handlers
		// r.Get("/.well-known/webfinger", wrapHandler(h.HandleWebFinger))
		// r.Get("/.well-known/nodeinfo", wrapHandler(h.HandleNodeInfo))

		// Custom emojis
		r.Get("/custom_emojis", wrapHandler(h.HandleGetCustomEmojis))
		
		// Streaming endpoints (SSE/WebSocket)
		r.Get("/streaming/{stream}", func(w http.ResponseWriter, r *http.Request) {
			streamType := chi.URLParam(r, "stream")
			lambdaReq := httpToLambdaRequest(r)
			resp, err := h.HandleSSEStream(r.Context(), lambdaReq, streamType)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeLambdaResponse(w, resp)
		})
		r.Get("/streaming", func(w http.ResponseWriter, r *http.Request) {
			lambdaReq := httpToLambdaRequest(r)
			resp, err := h.HandleSSEStream(r.Context(), lambdaReq)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			writeLambdaResponse(w, resp)
		})
	})

	// Authenticated routes (read-only)
	r.Group(func(r chi.Router) {
		r.Use(authMiddlewareFunc)

		// Account information
		r.Get("/accounts/verify_credentials", wrapHandler(h.HandleVerifyCredentials))
		r.Get("/accounts/relationships", wrapHandler(h.HandleGetRelationships))
		r.Get("/accounts/search", wrapHandler(h.HandleAccountSearch))
		r.Get("/accounts/lookup", wrapHandler(h.HandleAccountLookup))
		r.Get("/accounts/{id}", wrapHandlerWithParam(h.HandleGetAccount, "id"))
		r.Get("/accounts/{id}/statuses", wrapHandlerWithParam(h.HandleGetAccountStatuses, "id"))
		r.Get("/accounts/{id}/followers", wrapHandlerWithParam(h.HandleGetAccountFollowers, "id"))
		r.Get("/accounts/{id}/following", wrapHandlerWithParam(h.HandleGetAccountFollowing, "id"))

		// Timelines
		r.Get("/timelines/home", wrapHandler(h.HandleHomeTimeline))
		r.Get("/timelines/list/{id}", wrapHandlerWithParam(h.HandleListTimeline, "id"))

		// Statuses
		r.Get("/statuses/{id}", wrapHandlerWithParam(h.HandleGetStatus, "id"))
		r.Get("/statuses/{id}/context", wrapHandlerWithParam(h.HandleGetStatusContext, "id"))
		r.Get("/statuses/{id}/favourited_by", wrapHandlerWithParam(h.HandleGetStatusFavouritedBy, "id"))
		r.Get("/statuses/{id}/reblogged_by", wrapHandlerWithParam(h.HandleGetStatusRebloggedBy, "id"))

		// User collections
		r.Get("/bookmarks", wrapHandler(h.HandleGetBookmarks))
		r.Get("/favourites", wrapHandler(h.HandleGetFavourites))
		r.Get("/blocks", wrapHandler(h.HandleGetBlocks))
		r.Get("/mutes", wrapHandler(h.HandleGetMutedAccounts))
		r.Get("/domain_blocks", wrapHandler(h.HandleGetDomainBlocks))

		// Lists
		r.Get("/lists", wrapHandler(h.HandleGetLists))
		r.Get("/lists/{id}", wrapHandlerWithParam(h.HandleGetList, "id"))
		r.Get("/lists/{id}/accounts", wrapHandlerWithParam(h.HandleGetListAccounts, "id"))

		// Notifications
		r.Get("/notifications", wrapHandler(h.HandleGetNotifications))
		r.Get("/notifications/{id}", wrapHandlerWithParam(h.HandleGetNotification, "id"))

		// Search
		r.Get("/search", wrapHandler(h.HandleSearch))

		// Trends
		r.Get("/trends", wrapHandler(h.HandleGetTrends))
		r.Get("/trends/statuses", wrapHandler(h.HandleGetTrendingStatuses))
		r.Get("/trends/tags", wrapHandler(h.HandleGetTrendingTags))
		r.Get("/trends/links", wrapHandler(h.HandleGetTrendingLinks))
	})

	// Authenticated routes (write operations)
	r.Group(func(r chi.Router) {
		r.Use(authMiddlewareFunc)


		// Account updates
		r.Patch("/accounts/update_credentials", wrapHandler(h.HandleUpdateCredentials))

		// Status management
		r.Post("/statuses", wrapHandler(h.HandleCreateStatus))
		r.Delete("/statuses/{id}", wrapHandlerWithParam(h.HandleDeleteStatus, "id"))
		r.Put("/statuses/{id}", wrapHandlerWithParam(h.HandleUpdateStatus, "id"))

		// Status interactions
		r.Post("/statuses/{id}/favourite", wrapHandlerWithParam(h.HandleFavourite, "id"))
		r.Post("/statuses/{id}/unfavourite", wrapHandlerWithParam(h.HandleUnfavourite, "id"))
		r.Post("/statuses/{id}/reblog", wrapHandlerWithParam(h.HandleReblog, "id"))
		r.Post("/statuses/{id}/unreblog", wrapHandlerWithParam(h.HandleUnreblog, "id"))
		r.Post("/statuses/{id}/bookmark", wrapHandlerWithParam(h.HandleBookmark, "id"))
		r.Post("/statuses/{id}/unbookmark", wrapHandlerWithParam(h.HandleUnbookmark, "id"))
		r.Post("/statuses/{id}/mute", wrapHandlerWithParam(h.HandleMuteConversation, "id"))
		r.Post("/statuses/{id}/unmute", wrapHandlerWithParam(h.HandleUnmuteConversation, "id"))
		r.Post("/statuses/{id}/pin", wrapHandlerWithParam(h.HandlePinStatus, "id"))
		r.Post("/statuses/{id}/unpin", wrapHandlerWithParam(h.HandleUnpinStatus, "id"))

		// Account interactions
		r.Post("/accounts/{id}/follow", wrapHandlerWithParam(h.HandleFollow, "id"))
		r.Post("/accounts/{id}/unfollow", wrapHandlerWithParam(h.HandleUnfollow, "id"))
		r.Post("/accounts/{id}/block", wrapHandlerWithParam(h.HandleBlock, "id"))
		r.Post("/accounts/{id}/unblock", wrapHandlerWithParam(h.HandleUnblock, "id"))
		r.Post("/accounts/{id}/mute", wrapHandlerWithParam(h.HandleMuteAccount, "id"))
		r.Post("/accounts/{id}/unmute", wrapHandlerWithParam(h.HandleUnmuteAccount, "id"))
		r.Post("/accounts/{id}/pin", wrapHandlerWithParam(h.HandlePinAccount, "id"))
		r.Post("/accounts/{id}/unpin", wrapHandlerWithParam(h.HandleUnpinAccount, "id"))
		r.Post("/accounts/{id}/note", wrapHandlerWithParam(h.HandleSetAccountNote, "id"))
		r.Post("/accounts/{id}/remove_from_followers", wrapHandlerWithParam(h.HandleRemoveFromFollowers, "id"))

		// List management
		r.Post("/lists", wrapHandler(h.HandleCreateList))
		r.Put("/lists/{id}", wrapHandlerWithParam(h.HandleUpdateList, "id"))
		r.Delete("/lists/{id}", wrapHandlerWithParam(h.HandleDeleteList, "id"))
		r.Post("/lists/{id}/accounts", wrapHandlerWithParam(h.HandleAddAccountsToList, "id"))
		r.Delete("/lists/{id}/accounts", wrapHandlerWithParam(h.HandleRemoveAccountsFromList, "id"))

		// Media uploads
		r.Post("/media", wrapHandler(h.HandleMediaUpload))
		r.Put("/media/{id}", wrapHandler(h.HandleUpdateMedia))

		// Push subscriptions
		r.Post("/push/subscription", wrapHandler(h.HandleCreatePushSubscription))
		r.Get("/push/subscription", wrapHandler(h.HandleGetPushSubscription))
		r.Put("/push/subscription", wrapHandler(h.HandleUpdatePushSubscription))
		r.Delete("/push/subscription", wrapHandler(h.HandleDeletePushSubscription))

		// Domain blocks
		r.Post("/domain_blocks", wrapHandler(h.HandleCreateDomainBlock))
		r.Delete("/domain_blocks", wrapHandler(h.HandleDeleteDomainBlock))

		// Notification management
		r.Post("/notifications/clear", wrapHandler(h.HandleClearNotifications))
		r.Post("/notifications/{id}/dismiss", wrapHandlerWithParam(h.HandleDismissNotification, "id"))

		// Featured tags
		r.Get("/featured_tags", wrapHandler(h.HandleGetFeaturedTags))
		r.Post("/featured_tags", wrapHandler(h.HandleCreateFeaturedTag))
		r.Delete("/featured_tags/{id}", wrapHandlerWithParam(h.HandleDeleteFeaturedTag, "id"))
	})

	// Admin routes (require admin role)
	r.Group(func(r chi.Router) {
		r.Use(authMiddlewareFunc)
		r.Use(requireAdminMiddleware)

		r.Route("/admin", func(r chi.Router) {
			// Add admin routes here
		})
	})

	return r
}

// wrapHandler converts Lambda handler to http.Handler
func wrapHandler(fn func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Try to get the original Lambda request from context
		var lambdaReq events.APIGatewayV2HTTPRequest
		if origReq, ok := r.Context().Value(lambdaRequestKey).(events.APIGatewayV2HTTPRequest); ok {
			// Use the original Lambda request which has the correct body
			lambdaReq = origReq
		} else {
			// Fallback to converting from HTTP request (for tests)
			lambdaReq = httpToLambdaRequest(r)
		}

		// Call handler
		resp, err := fn(r.Context(), lambdaReq)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		writeLambdaResponse(w, resp)
	}
}

// wrapHandlerWithParam converts Lambda handler with parameter to http.Handler
func wrapHandlerWithParam(fn func(context.Context, events.APIGatewayV2HTTPRequest, string) (*events.APIGatewayV2HTTPResponse, error), param string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get parameter from chi context
		paramValue := chi.URLParam(r, param)

		// Try to get the original Lambda request from context
		var lambdaReq events.APIGatewayV2HTTPRequest
		if origReq, ok := r.Context().Value(lambdaRequestKey).(events.APIGatewayV2HTTPRequest); ok {
			// Use the original Lambda request which has the correct body
			lambdaReq = origReq
		} else {
			// Fallback to converting from HTTP request (for tests)
			lambdaReq = httpToLambdaRequest(r)
		}

		// Call handler
		resp, err := fn(r.Context(), lambdaReq, paramValue)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write response
		writeLambdaResponse(w, resp)
	}
}

// requireAdminMiddleware checks if user has admin role
func requireAdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := auth.GetClaims(r.Context())
		if !ok || claims == nil {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		// TODO: Add role check to claims when role support is added
		// For now, admin check would need to be done via username or a separate lookup
		next.ServeHTTP(w, r)
	})
}

// httpToLambdaRequest converts http.Request to Lambda request format
func httpToLambdaRequest(r *http.Request) events.APIGatewayV2HTTPRequest {
	headers := make(map[string]string)
	for k, v := range r.Header {
		headers[k] = v[0]
	}

	queryParams := make(map[string]string)
	for k, v := range r.URL.Query() {
		queryParams[k] = v[0]
	}

	// Read body
	body := ""
	isBase64Encoded := false
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err == nil {
			body = string(bodyBytes)
			// Reset the body so it can be read again by handlers if needed
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}
	}

	return events.APIGatewayV2HTTPRequest{
		Headers:               headers,
		QueryStringParameters: queryParams,
		Body:                  body,
		IsBase64Encoded:       isBase64Encoded,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method: r.Method,
				Path:   r.URL.Path,
			},
		},
	}
}

// writeLambdaResponse writes Lambda response to http.ResponseWriter
func writeLambdaResponse(w http.ResponseWriter, resp *events.APIGatewayV2HTTPResponse) {
	// Set headers
	for k, v := range resp.Headers {
		w.Header().Set(k, v)
	}

	// Set status code
	if resp.StatusCode != 0 {
		w.WriteHeader(resp.StatusCode)
	}

	// Write body
	if resp.Body != "" {
		w.Write([]byte(resp.Body))
	}
}

// Context key for storing the original Lambda request
type contextKey string

const lambdaRequestKey contextKey = "lambdaRequest"

// LambdaHandlerWithRouter creates a Lambda handler that uses the chi router
func LambdaHandlerWithRouter(router *chi.Mux) func(context.Context, events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	return func(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {

		// Create http.Request from Lambda request
		path := request.RequestContext.HTTP.Path

		// Remove stage prefix if present
		if request.RequestContext.Stage != "" && request.RequestContext.Stage != "$default" {
			stagePrefix := "/" + request.RequestContext.Stage
			path = strings.TrimPrefix(path, stagePrefix)
		}

		// Decode body if base64 encoded
		bodyReader := strings.NewReader(request.Body)
		actualBody := request.Body
		
		// Try to detect base64 even if flag not set (API Gateway bug)
		// Always try base64 decode if body looks like base64
		if request.Body != "" {
			// Try decoding regardless of flag
			decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
			if err == nil {
				// Successfully decoded, use the decoded body
				actualBody = string(decodedBytes)
				bodyReader = strings.NewReader(actualBody)
			}
		}

		// Store the original Lambda request in context with potentially decoded body
		lambdaReqWithDecodedBody := request
		lambdaReqWithDecodedBody.Body = actualBody
		ctx = context.WithValue(ctx, lambdaRequestKey, lambdaReqWithDecodedBody)

		// Create request
		httpReq, err := http.NewRequestWithContext(ctx, request.RequestContext.HTTP.Method, path, bodyReader)
		if err != nil {
			return common.InternalServerError(err), nil
		}

		// Set headers
		for k, v := range request.Headers {
			httpReq.Header.Set(k, v)
		}

		// Set query parameters
		q := httpReq.URL.Query()
		for k, v := range request.QueryStringParameters {
			q.Set(k, v)
		}
		httpReq.URL.RawQuery = q.Encode()

		// Create response recorder
		w := httptest.NewRecorder()

		// Serve request with router
		router.ServeHTTP(w, httpReq)

		// Convert response
		headers := make(map[string]string)
		for k := range w.Header() {
			headers[k] = w.Header().Get(k)
		}

		return &events.APIGatewayV2HTTPResponse{
			StatusCode: w.Code,
			Headers:    headers,
			Body:       w.Body.String(),
		}, nil
	}
}

