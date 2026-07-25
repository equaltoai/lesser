package common

import (
	"fmt"
	"net/http"
	"strings"

	apptheory "github.com/theory-cloud/apptheory/v2/runtime"
)

const (
	// BearerErrorInvalidToken is the canonical API error for missing, invalid, expired, or otherwise refreshable
	// bearer-token failures on authenticated `/api/*` routes.
	BearerErrorInvalidToken = "invalid_token"

	// BearerErrorInsufficientScope is the canonical API error for authenticated-but-not-authorized bearer requests.
	BearerErrorInsufficientScope = "insufficient_scope"

	// BearerErrorSlowDown is the canonical auth-path rate-limit error for OAuth/MCP clients.
	BearerErrorSlowDown = "slow_down"

	bearerRealm = "lesser"
)

// BearerAuthErrorResponse is the canonical machine-readable body for bearer-token failures on `/api/*` routes.
type BearerAuthErrorResponse struct {
	Error       string `json:"error"`
	Description string `json:"error_description,omitempty"`
	Scope       string `json:"scope,omitempty"`
}

func isBearerAPIAuthPath(ctx *apptheory.Context) bool {
	if ctx == nil {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(ctx.Request.Path), "/api/")
}

// RespondBearerMissingAuth returns the canonical protected-API response for missing bearer authentication.
func RespondBearerMissingAuth(_ *apptheory.Context) (*apptheory.Response, error) {
	return respondBearerAuthError(http.StatusUnauthorized, BearerAuthErrorResponse{
		Error:       BearerErrorInvalidToken,
		Description: "authentication required",
	}, bearerChallengeHeader(BearerErrorInvalidToken, "authentication required", ""))
}

// RespondBearerInvalidToken returns the canonical protected-API response for invalid or expired bearer tokens.
func RespondBearerInvalidToken(_ *apptheory.Context, description ...string) (*apptheory.Response, error) {
	desc := "invalid token"
	if len(description) > 0 && strings.TrimSpace(description[0]) != "" {
		desc = strings.TrimSpace(description[0])
	}
	return respondBearerAuthError(http.StatusUnauthorized, BearerAuthErrorResponse{
		Error:       BearerErrorInvalidToken,
		Description: desc,
	}, bearerChallengeHeader(BearerErrorInvalidToken, desc, ""))
}

// RespondBearerExpiredToken returns the canonical protected-API response for bearer tokens that are known expired.
func RespondBearerExpiredToken(_ *apptheory.Context, description ...string) (*apptheory.Response, error) {
	desc := "token expired"
	if len(description) > 0 && strings.TrimSpace(description[0]) != "" {
		desc = strings.TrimSpace(description[0])
	}
	return respondBearerAuthError(http.StatusUnauthorized, BearerAuthErrorResponse{
		Error:       BearerErrorInvalidToken,
		Description: desc,
	}, bearerChallengeHeader(BearerErrorInvalidToken, desc, ""))
}

// RespondBearerInsufficientScope returns the canonical protected-API response for authenticated requests lacking scope.
func RespondBearerInsufficientScope(_ *apptheory.Context, requiredScope ...string) (*apptheory.Response, error) {
	scope := ""
	desc := "insufficient scope"
	if len(requiredScope) > 0 && strings.TrimSpace(requiredScope[0]) != "" {
		scope = strings.TrimSpace(requiredScope[0])
		desc = fmt.Sprintf("insufficient scope: requires %s", scope)
	}
	return respondBearerAuthError(http.StatusForbidden, BearerAuthErrorResponse{
		Error:       BearerErrorInsufficientScope,
		Description: desc,
		Scope:       scope,
	}, bearerChallengeHeader(BearerErrorInsufficientScope, desc, scope))
}

// RespondBearerRateLimited returns the canonical auth-path rate-limit response for machine clients.
func RespondBearerRateLimited(_ *apptheory.Context, description string) (*apptheory.Response, error) {
	desc := strings.TrimSpace(description)
	if desc == "" {
		desc = "rate limit exceeded"
	}
	return respondBearerAuthError(http.StatusTooManyRequests, BearerAuthErrorResponse{
		Error:       BearerErrorSlowDown,
		Description: desc,
	}, "")
}

func respondBearerAuthError(status int, payload BearerAuthErrorResponse, challenge string) (*apptheory.Response, error) {
	resp, err := apptheory.JSON(status, payload)
	if err != nil {
		return nil, err
	}
	if challenge != "" {
		if resp.Headers == nil {
			resp.Headers = map[string][]string{}
		}
		resp.Headers["www-authenticate"] = []string{challenge}
	}
	return resp, nil
}

func bearerChallengeHeader(code, description, scope string) string {
	parts := []string{fmt.Sprintf("Bearer realm=%q", bearerRealm)}
	if strings.TrimSpace(code) != "" {
		parts = append(parts, fmt.Sprintf("error=%q", bearerChallengeEscape(code)))
	}
	if strings.TrimSpace(description) != "" {
		parts = append(parts, fmt.Sprintf("error_description=%q", bearerChallengeEscape(description)))
	}
	if strings.TrimSpace(scope) != "" {
		parts = append(parts, fmt.Sprintf("scope=%q", bearerChallengeEscape(scope)))
	}
	return strings.Join(parts, ", ")
}

func bearerChallengeEscape(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(strings.TrimSpace(value))
}
