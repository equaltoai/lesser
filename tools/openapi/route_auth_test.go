package main

import (
	"go/ast"
	"go/parser"
	"slices"
	"testing"
)

func TestApplyOperationOverridesMakesQuoteStubsNotImplemented(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		route            routeDef
		initialResponses []string
		wantResponses    []string
		wantDescription  string
	}{
		{
			name:             "create quote",
			route:            routeDef{Method: methodPOST, Path: "/api/v1/statuses/{id}/quote"},
			initialResponses: []string{"200", "400", "401", "403", "404", "422", "429", "500"},
			wantResponses:    []string{"400", "401", "403", "429", "501"},
			wantDescription:  "Quote creation is not implemented; target IDs are not looked up.",
		},
		{
			name:             "list quotes",
			route:            routeDef{Method: methodGET, Path: "/api/v1/statuses/{id}/quotes"},
			initialResponses: []string{"200", "400", "404", "500"},
			wantResponses:    []string{"400", "501"},
			wantDescription:  "Quote listing is not implemented; target IDs and quote counts are not looked up.",
		},
		{
			name:             "get quote permissions",
			route:            routeDef{Method: methodGET, Path: "/api/v1/accounts/{id}/quote_permissions"},
			initialResponses: []string{"200", "400", "401", "403", "404", "500"},
			wantResponses:    []string{"400", "401", "403", "501"},
			wantDescription:  "Quote permission reads are not implemented and no settings are retrieved.",
		},
		{
			name:             "update quote permissions",
			route:            routeDef{Method: methodPUT, Path: "/api/v1/accounts/quote_permissions"},
			initialResponses: []string{"200", "400", "401", "403", "422", "500"},
			wantResponses:    []string{"400", "401", "403", "501"},
			wantDescription:  "Quote permission updates are not implemented and no settings are persisted.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			op := &operation{Responses: make(map[string]response, len(tt.initialResponses))}
			for _, status := range tt.initialResponses {
				op.Responses[status] = response{}
			}
			applyOperationOverrides(op, tt.route)

			gotResponses := make([]string, 0, len(op.Responses))
			for status := range op.Responses {
				gotResponses = append(gotResponses, status)
			}
			slices.Sort(gotResponses)
			if !slices.Equal(gotResponses, tt.wantResponses) {
				t.Fatalf("responses = %v, want only reachable %v", gotResponses, tt.wantResponses)
			}
			if got := op.Responses["501"].Description; got != tt.wantDescription {
				t.Fatalf("501 description = %q", got)
			}
		})
	}
}

func TestExtractAPIRouteMetaInfersRouteMiddlewareAuth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		expr            string
		wantMethod      string
		wantPath        string
		wantHandler     string
		wantAuth        authMode
		wantRateLimited bool
	}{
		{
			name:        "require read route guard is bearer required",
			expr:        `app.Get("/api/v1/exports", apiHandler.HandleListExportsLift, requireRead)`,
			wantMethod:  methodGET,
			wantPath:    "/api/v1/exports",
			wantHandler: "HandleListExportsLift",
			wantAuth:    authModeBearerRequired,
		},
		{
			name: "ratelimit wrapped require auth route guard is bearer required",
			expr: `app.Post("/api/v1/statuses/{id}/translate", ratelimit.ApplyRateLimit(
				apiHandler.HandleTranslateStatusLift,
				20, time.Hour, logger), requireAuth)`,
			wantMethod:      methodPOST,
			wantPath:        "/api/v1/statuses/{id}/translate",
			wantHandler:     "HandleTranslateStatusLift",
			wantAuth:        authModeBearerRequired,
			wantRateLimited: true,
		},
		{
			name: "optional auth route guard is bearer optional",
			expr: `app.Get("/api/v1/skills", ratelimit.ApplyRateLimit(
				apiHandler.HandleListSkillsLift,
				30, 5*time.Minute, logger), optionalAuth)`,
			wantMethod:      methodGET,
			wantPath:        "/api/v1/skills",
			wantHandler:     "HandleListSkillsLift",
			wantAuth:        authModeBearerOptional,
			wantRateLimited: true,
		},
		{
			name:        "public route remains public",
			expr:        `app.Get("/api/v1/announcements", apiHandler.HandleGetAnnouncementsLift)`,
			wantMethod:  methodGET,
			wantPath:    "/api/v1/announcements",
			wantHandler: "HandleGetAnnouncementsLift",
			wantAuth:    authModePublic,
		},
		{
			name:        "app handle route guard is bearer required",
			expr:        `app.Handle("POST", "/api/v1/reputation/export", apiHandler.HandleExportReputationLift, requireAuth)`,
			wantMethod:  methodPOST,
			wantPath:    "/api/v1/reputation/export",
			wantHandler: "HandleExportReputationLift",
			wantAuth:    authModeBearerRequired,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			meta := parseAPIRouteMetaForTest(t, tc.expr)
			if meta.Method != tc.wantMethod {
				t.Fatalf("method = %q, want %q", meta.Method, tc.wantMethod)
			}
			if meta.Path != tc.wantPath {
				t.Fatalf("path = %q, want %q", meta.Path, tc.wantPath)
			}
			if meta.Handler != tc.wantHandler {
				t.Fatalf("handler = %q, want %q", meta.Handler, tc.wantHandler)
			}
			if meta.RouteAuth != tc.wantAuth {
				t.Fatalf("route auth = %q, want %q", meta.RouteAuth, tc.wantAuth)
			}
			if meta.RateLimited != tc.wantRateLimited {
				t.Fatalf("rate limited = %v, want %v", meta.RateLimited, tc.wantRateLimited)
			}
		})
	}
}

func TestResolveContractAuthModeUsesPublicSurfaceFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		method      string
		path        string
		lambda      string
		routeGuard  authMode
		handlerAuth authMode
		want        authMode
	}{
		{
			name:   "guardless api route outside public surface is bearer required",
			method: methodGET,
			path:   "/api/v1/notifications",
			lambda: lambdaAPI,
			want:   authModeBearerRequired,
		},
		{
			name:       "public route with optional guard is bearer optional",
			method:     methodGET,
			path:       "/api/v1/accounts/search",
			lambda:     lambdaAPI,
			routeGuard: authModeBearerOptional,
			want:       authModeBearerOptional,
		},
		{
			name:       "public route with required guard remains bearer required",
			method:     methodGET,
			path:       "/api/v1/search/statuses",
			lambda:     lambdaAPI,
			routeGuard: authModeBearerRequired,
			want:       authModeBearerRequired,
		},
		{
			name:   "normal public route remains public",
			method: methodGET,
			path:   "/api/v1/custom_emojis",
			lambda: lambdaAPI,
			want:   authModePublic,
		},
		{
			name:        "public route with optional handler remains bearer optional",
			method:      methodGET,
			path:        "/api/v1/announcements",
			lambda:      lambdaAPI,
			handlerAuth: authModeBearerOptional,
			want:        authModeBearerOptional,
		},
		{
			name:       "app registration preserves public contract despite optional guard",
			method:     methodPOST,
			path:       "/api/v1/apps",
			lambda:     lambdaAPI,
			routeGuard: authModeBearerOptional,
			want:       authModePublic,
		},
		{
			name:   "setup admin uses setup bearer override",
			method: methodPOST,
			path:   "/setup/admin",
			lambda: lambdaAPI,
			want:   authModeSetupBearer,
		},
		{
			name:   "setup finalize preserves oauth bearer override",
			method: methodPOST,
			path:   "/setup/finalize",
			lambda: lambdaAPI,
			want:   authModeBearerRequired,
		},
		{
			name:   "internal notification delivery is bearer required",
			method: methodPOST,
			path:   "/api/v1/notifications/deliver",
			lambda: lambdaAPI,
			want:   authModeBearerRequired,
		},
		{
			name:   "soul binding write uses dedicated integration bearer",
			method: methodPOST,
			path:   "/api/v1/souls/bindings",
			lambda: lambdaAPI,
			want:   authModeSoulBinding,
		},
		{
			name:   "soul binding read uses dedicated integration bearer",
			method: methodGET,
			path:   "/api/v1/souls/bindings/{agentId}",
			lambda: lambdaAPI,
			want:   authModeSoulBinding,
		},
		{
			name:   "streaming health remains public",
			method: methodGET,
			path:   "/api/v1/streaming/health",
			lambda: lambdaSSE,
			want:   authModePublic,
		},
		{
			name:   "streaming user endpoint remains bearer required",
			method: methodGET,
			path:   "/api/v1/streaming/user",
			lambda: lambdaSSE,
			want:   authModeBearerRequired,
		},
		{
			name:   "graphql remains bearer optional",
			method: methodPOST,
			path:   "/api/graphql",
			lambda: lambdaGraphQL,
			want:   authModeBearerOptional,
		},
		{
			name:   "activitypub actor route remains public",
			method: methodGET,
			path:   "/users/{username}",
			lambda: "actor",
			want:   authModePublic,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := resolveContractAuthMode(tc.method, tc.path, tc.lambda, tc.routeGuard, tc.handlerAuth)
			if got != tc.want {
				t.Fatalf("resolveContractAuthMode(%s %s, %s, %s, %s) = %q, want %q",
					tc.method, tc.path, tc.lambda, tc.routeGuard, tc.handlerAuth, got, tc.want)
			}
		})
	}
}

func parseAPIRouteMetaForTest(t *testing.T, src string) apiRouteMeta {
	t.Helper()

	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parse route expression: %v", err)
	}
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		t.Fatalf("parsed expression is %T, want *ast.CallExpr", expr)
	}
	meta, ok := extractAPIRouteMeta(call)
	if !ok {
		t.Fatalf("extractAPIRouteMeta(%s) returned false", src)
	}
	return meta
}
