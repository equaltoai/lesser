package main

import (
	"go/ast"
	"go/parser"
	"testing"
)

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
