package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	apimodels "github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	"github.com/equaltoai/lesser/pkg/security/authz"
	"github.com/equaltoai/lesser/pkg/services/accounts"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

type authorizeRequest struct {
	responseType        string
	clientID            string
	redirectURI         string
	resource            string
	scope               string
	state               string
	codeChallenge       string
	codeChallengeMethod string
}

type authorizeFlow struct {
	request           *authorizeRequest
	oauthSvc          *auth.OAuthService
	client            *storage.OAuthClient
	principalUsername string
	username          string
	scopes            []string
}

const oauthDeviceCodeGrantType = "urn:ietf:params:oauth:grant-type:device_code"

const (
	oauthGrantTypeAuthorizationCode = "authorization_code"
	oauthGrantTypeRefreshToken      = "refresh_token"
	oauthGrantTypeClientCredentials = "client_credentials"

	oauthTokenTypeBearer       = "Bearer"
	oauthTokenExpiresInSeconds = 3600

	oauthInstanceSurfacePtah = "ptah"
	oauthInstanceSurfaceBa   = "ba"

	oauthResourceSegmentMCP      = "mcp"
	oauthResourceSegmentInstance = "instance"

	standardRefreshRetryGrace = 30 * time.Second
)

var errOAuthInvalidTarget = errors.New("invalid_target")

type oauthAuthorizeTargetError struct {
	code        string
	description string
}

func (e *oauthAuthorizeTargetError) Error() string {
	if e == nil {
		return ""
	}
	return e.description
}

func (h *Handler) isOAuthAuthorizeUIMode(ctx *apptheory.Context) bool {
	if strings.EqualFold(queryValue(ctx, "mode"), "ui") {
		return true
	}

	accept := headerValue(ctx, "Accept")
	if accept == "" {
		accept = headerValue(ctx, "accept")
	}

	return strings.Contains(accept, "application/json")
}

func (h *Handler) writeOAuthAuthorizeRedirect(ctx *apptheory.Context, nextURL string) (*apptheory.Response, error) {
	if h.isOAuthAuthorizeUIMode(ctx) {
		return okJSON(apimodels.OAuthAuthorizeResponse{NextURL: nextURL})
	}

	resp := &apptheory.Response{Status: http.StatusFound}
	setHeader(resp, "Location", nextURL)
	return resp, nil
}

// HandleOAuthAuthorizeLift handles the OAuth authorization endpoint using native Lift patterns
// GET /oauth/authorize
func (h *Handler) HandleOAuthAuthorizeLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	flow, resp, err := h.initializeAuthorizeFlow(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	resp, err = h.ensureConsentForFlow(ctx, flow)
	if resp != nil || err != nil {
		return resp, err
	}

	return h.completeAuthorizationFlow(ctx, flow)
}

func (h *Handler) initializeAuthorizeFlow(ctx *apptheory.Context) (*authorizeFlow, *apptheory.Response, error) {
	req, resp, err := h.extractAuthorizeRequest(ctx)
	if resp != nil || err != nil {
		return nil, resp, err
	}

	oauthSvc, err := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	if err != nil {
		h.logger.Error("failed to initialize OAuth service", zap.Error(err))
		resp, respErr := h.oauthErrorLift(ctx, "server_error", "OAuth service unavailable", req.redirectURI, req.state)
		return nil, resp, respErr
	}

	flow := &authorizeFlow{
		request:  req,
		oauthSvc: oauthSvc,
	}

	if err := flow.oauthSvc.ValidateRedirectURI(ctx.Context(), req.clientID, req.redirectURI); err != nil {
		h.logger.Error("invalid redirect URI",
			zap.String("client_id", req.clientID),
			zap.String("redirect_uri", req.redirectURI),
			zap.Error(err))
		resp, respErr := h.oauthErrorLift(ctx, "invalid_request", "Invalid redirect_uri", "", req.state)
		return nil, resp, respErr
	}

	client, err := h.repos.Account().GetOAuthClient(ctx.Context(), req.clientID)
	if err != nil || client == nil {
		resp, respErr := h.oauthErrorLift(ctx, "invalid_client", "Invalid client", req.redirectURI, req.state)
		return nil, resp, respErr
	}
	flow.client = client

	if oauthClientRequiresPKCE(client) {
		if err := requireOAuthPKCE(req.codeChallenge, req.codeChallengeMethod); err != nil {
			resp, respErr := h.oauthErrorLift(ctx, "invalid_request", err.Error(), req.redirectURI, req.state)
			return nil, resp, respErr
		}
	}
	if strings.TrimSpace(req.resource) == "" && oauthAuthorizeRequiresResource(client) {
		resp, respErr := h.oauthErrorLift(ctx, "invalid_target", "resource is required for remote MCP authorization", req.redirectURI, req.state)
		return nil, resp, respErr
	}

	principalUsername, resp, err := h.resolveAuthorizeUser(ctx, req)
	if resp != nil || err != nil {
		return nil, resp, err
	}
	flow.principalUsername = principalUsername
	flow.username = principalUsername

	if resp, err := h.bindAuthorizeTarget(ctx, flow); resp != nil || err != nil {
		return nil, resp, err
	}

	scopes, err := h.normalizeAuthorizeScopesForFlow(ctx.Context(), flow)
	if err != nil {
		resp, respErr := h.oauthErrorLift(ctx, "invalid_scope", err.Error(), req.redirectURI, req.state)
		return nil, resp, respErr
	}
	flow.scopes = scopes

	return flow, nil, nil
}

func (h *Handler) extractAuthorizeRequest(ctx *apptheory.Context) (*authorizeRequest, *apptheory.Response, error) {
	req := &authorizeRequest{
		responseType:        queryValue(ctx, "response_type"),
		clientID:            queryValue(ctx, "client_id"),
		redirectURI:         queryValue(ctx, "redirect_uri"),
		resource:            queryValue(ctx, "resource"),
		scope:               queryValue(ctx, "scope"),
		state:               queryValue(ctx, "state"),
		codeChallenge:       queryValue(ctx, "code_challenge"),
		codeChallengeMethod: queryValue(ctx, "code_challenge_method"),
	}

	if req.responseType != "code" {
		resp, err := h.oauthErrorLift(ctx, "unsupported_response_type", "Only 'code' response type is supported", req.redirectURI, req.state)
		return nil, resp, err
	}

	if req.clientID == "" || req.redirectURI == "" {
		resp, err := h.redirectMissingAuthorizeParams(ctx, req)
		return nil, resp, err
	}

	resource, err := normalizeOAuthResourceIndicator(req.resource)
	if err != nil {
		resp, respErr := h.oauthErrorLift(ctx, "invalid_target", err.Error(), req.redirectURI, req.state)
		return nil, resp, respErr
	}
	req.resource = resource

	return req, nil, nil
}

func (h *Handler) redirectMissingAuthorizeParams(ctx *apptheory.Context, req *authorizeRequest) (*apptheory.Response, error) {
	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)
	errorMessage := "Invalid authorization request - missing required parameters. Please restart the authorization flow from your application."

	errorParams := url.Values{}
	errorParams.Set("error", errorMessage)
	if req.clientID != "" {
		errorParams.Set("client_id", req.clientID)
	}
	if req.redirectURI != "" {
		errorParams.Set("redirect_uri", req.redirectURI)
	}
	if req.state != "" {
		errorParams.Set("state", req.state)
	}
	if req.scope != "" {
		errorParams.Set("scope", req.scope)
	}
	if req.resource != "" {
		errorParams.Set("resource", req.resource)
	}
	if req.responseType != "" {
		errorParams.Set("response_type", req.responseType)
	}

	errorURL := fmt.Sprintf("%s/oauth/authorize?%s", authUIBaseURL, errorParams.Encode())
	return h.writeOAuthAuthorizeRedirect(ctx, errorURL)
}

func (h *Handler) resolveAuthorizeUser(ctx *apptheory.Context, req *authorizeRequest) (string, *apptheory.Response, error) {
	username := h.getUserFromSessionLift(ctx)

	if err := common.ValidateRequiredParam("username", username); err != nil {
		resp, respErr := h.redirectUserToLogin(ctx, req)
		return "", resp, respErr
	}

	return username, nil, nil
}

func (h *Handler) redirectUserToLogin(ctx *apptheory.Context, req *authorizeRequest) (*apptheory.Response, error) {
	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)
	authRequest := map[string]string{
		"client_id":             req.clientID,
		"redirect_uri":          req.redirectURI,
		"resource":              req.resource,
		"scope":                 req.scope,
		"state":                 req.state,
		"response_type":         req.responseType,
		"code_challenge":        req.codeChallenge,
		"code_challenge_method": req.codeChallengeMethod,
	}

	payload, _ := json.Marshal(authRequest)
	loginURL := fmt.Sprintf("%s/login?return_to=%s&auth_request=%s",
		authUIBaseURL,
		url.QueryEscape("/oauth/authorize"),
		url.QueryEscape(string(payload)))
	return h.writeOAuthAuthorizeRedirect(ctx, loginURL)
}

func (h *Handler) normalizeAuthorizeScopes(scope string) ([]string, error) {
	scopes, err := parseAuthorizeScopes(scope)
	if err != nil {
		return nil, err
	}

	if err := auth.ValidatePublicOAuthScopes(scopes); err != nil {
		return nil, errors.New("one or more requested scopes are invalid")
	}

	return scopes, nil
}

func parseAuthorizeScopes(scope string) ([]string, error) {
	scopes := auth.DefaultScopes()
	if strings.TrimSpace(scope) != "" {
		scopes = splitOAuthSpaceDelimited(scope)
	}

	scopesStr := strings.Join(scopes, " ")
	if err := common.ValidateApplicationScopes(scopesStr); err != nil {
		return nil, fmt.Errorf("invalid scopes: %v", err)
	}

	return scopes, nil
}

// normalizeAuthorizeScopesForFlow preserves the public scope policy while allowing the
// owner-bound internal operator client to request admin authority for the instance plane.
// The exception is deliberately tied to the exact instance resource and the checks in
// oauthOperatorClientCanAuthorizeInstanceResource; public clients never enter this path.
func (h *Handler) normalizeAuthorizeScopesForFlow(ctx context.Context, flow *authorizeFlow) ([]string, error) {
	if flow == nil || flow.request == nil || flow.client == nil {
		return nil, errors.New("one or more requested scopes are invalid")
	}

	scopes, err := parseAuthorizeScopes(flow.request.scope)
	if err != nil {
		return nil, err
	}
	if err := auth.ValidatePublicOAuthScopes(scopes); err == nil {
		return scopes, nil
	}

	if !oauthResourceTargetsInstancePlane(flow.request.resource) ||
		!strings.EqualFold(strings.TrimSpace(flow.client.ClientClass), auth.ClientClassOperator) ||
		!h.oauthOperatorClientCanAuthorizeInstanceResource(ctx, flow.client, flow.principalUsername) ||
		auth.ValidateScopes(scopes) != nil ||
		!auth.ScopeSetAllows(flow.client.Scopes, scopes) {
		return nil, errors.New("one or more requested scopes are invalid")
	}

	return scopes, nil
}

func normalizeOAuthResourceIndicator(resource string) (string, error) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		return "", nil
	}

	parsed, err := url.Parse(resource)
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" {
		return "", errors.New("resource must be an absolute https URI without fragment")
	}
	if !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("resource must use https")
	}
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return "", errors.New("resource must not include a query string")
	}
	if parsed.Fragment != "" {
		return "", errors.New("resource must not include a fragment")
	}

	return resource, nil
}

func (h *Handler) bindAuthorizeTarget(ctx *apptheory.Context, flow *authorizeFlow) (*apptheory.Response, error) {
	if flow == nil || flow.request == nil {
		return nil, nil
	}

	req := flow.request
	if strings.TrimSpace(req.resource) == "" {
		if oauthAuthorizeRequiresResource(flow.client) {
			return h.oauthErrorLift(ctx, "invalid_target", "resource is required for remote MCP authorization", req.redirectURI, req.state)
		}
		return nil, nil
	}

	if oauthResourceTargetsInstancePlane(req.resource) {
		targetUsername, canonicalResource, err := h.resolveAuthorizeTargetInstanceFromResource(ctx.Context(), req.resource, flow.principalUsername)
		if err != nil {
			var targetErr *oauthAuthorizeTargetError
			if errors.As(err, &targetErr) {
				return h.oauthErrorLift(ctx, targetErr.code, targetErr.description, req.redirectURI, req.state)
			}

			h.logger.Error("failed to resolve OAuth instance resource target", zap.Error(err))
			return h.oauthErrorLift(ctx, "server_error", "Failed to resolve requested resource", req.redirectURI, req.state)
		}
		if !oauthClientCanAuthorizeInstanceResource(flow.client) {
			return h.oauthErrorLift(ctx, "access_denied", "instance-plane MCP requires an account-holder OAuth client", req.redirectURI, req.state)
		}
		if !h.oauthOperatorClientCanAuthorizeInstanceResource(ctx.Context(), flow.client, flow.principalUsername) {
			return h.oauthErrorLift(ctx, "access_denied", "instance-plane MCP operator access requires the owning local admin", req.redirectURI, req.state)
		}

		// Instance-plane resources belong to the account holder, not to an actor.
		// Keeping the token subject on the principal prevents Ptah/Ba authorization
		// from becoming an actor-scoped Ka session.
		flow.username = targetUsername
		flow.request.resource = canonicalResource
		return nil, nil
	}

	targetUsername, canonicalResource, err := h.resolveAuthorizeTargetActorFromResource(ctx.Context(), req.resource, flow.principalUsername)
	if err == nil {
		flow.username = targetUsername
		flow.request.resource = canonicalResource
		return nil, nil
	}

	var targetErr *oauthAuthorizeTargetError
	if errors.As(err, &targetErr) {
		return h.oauthErrorLift(ctx, targetErr.code, targetErr.description, req.redirectURI, req.state)
	}

	h.logger.Error("failed to resolve OAuth resource target", zap.Error(err))
	return h.oauthErrorLift(ctx, "server_error", "Failed to resolve requested resource", req.redirectURI, req.state)
}

func (h *Handler) resolveAuthorizeTargetActorFromResource(ctx context.Context, resource, principalUsername string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || parsed == nil {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must be the actor-scoped MCP URL for this server",
		}
	}
	if parsed.ForceQuery || strings.TrimSpace(parsed.RawQuery) != "" || strings.TrimSpace(parsed.Fragment) != "" {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must be the actor-scoped MCP URL for this server",
		}
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 2 || segments[0] != oauthResourceSegmentMCP || strings.TrimSpace(segments[1]) == "" {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must be the actor-scoped MCP URL for this server",
		}
	}

	actorUsername := strings.TrimSpace(segments[1])
	if err := common.ValidateUsernameParamID(actorUsername); err != nil {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must reference a valid local MCP actor",
		}
	}

	actorUser, err := h.repos.Account().GetUser(ctx, actorUsername)
	if err != nil || actorUser == nil || !actorUser.IsAgent {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must reference an existing local MCP actor",
		}
	}
	// The agent is always the token subject. The authorizing human — the owner on
	// the owner path, an active share grantee on the grantee path — is carried
	// separately in the authorization code's PrincipalUsername and surfaces as
	// DelegatedBy at token issuance. The grant read is the uncached,
	// strongly-consistent direct base-table check from the agentshare service and is
	// consulted only for non-owners, so the owner path stays unchanged.
	if !h.agentOwnedByPrincipal(actorUser, principalUsername) {
		grantActive, grantErr := h.agentShareGrantActive(ctx, actorUsername, principalUsername)
		if grantErr != nil {
			return "", "", fmt.Errorf("agent share grant check failed: %w", grantErr)
		}
		if !grantActive {
			return "", "", &oauthAuthorizeTargetError{
				code:        "access_denied",
				description: "principal is not authorized for requested MCP resource",
			}
		}
	}

	canonicalResource := auth.BuildPublicMCPAccessBundle(h.cfg.BaseURL(), actorUsername).MCPURL
	canonicalURL, canonicalErr := url.Parse(canonicalResource)
	if canonicalErr != nil || canonicalURL == nil {
		return "", "", errors.New("authorization server resource URL is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, canonicalURL.Scheme) ||
		!strings.EqualFold(parsed.Host, canonicalURL.Host) ||
		parsed.EscapedPath() != canonicalURL.EscapedPath() {
		return "", "", &oauthAuthorizeTargetError{
			code:        "invalid_target",
			description: "resource must be the actor-scoped MCP URL for this server",
		}
	}

	return actorUsername, canonicalResource, nil
}

func oauthResourceTargetsInstancePlane(resource string) bool {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || parsed == nil {
		return false
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(segments) > 0 && segments[0] == oauthResourceSegmentInstance
}

// oauthResourceTargetsActorMCP reports whether the resource is an actor-scoped MCP
// URL (/mcp/{actor}). These tokens always carry an agent subject; the authorizing
// human rides in DelegatedBy and must survive refresh with the grant re-validated.
func oauthResourceTargetsActorMCP(resource string) bool {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || parsed == nil {
		return false
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	return len(segments) == 2 && segments[0] == oauthResourceSegmentMCP && strings.TrimSpace(segments[1]) != ""
}

func oauthClientCanAuthorizeInstanceResource(client *storage.OAuthClient) bool {
	if client == nil {
		return false
	}

	// Agent clients are reserved for actor-scoped Ka resources. In particular,
	// an agent client must not mint an account-holder token for Ptah or Ba.
	return !strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassAgent) &&
		strings.TrimSpace(client.AgentUsername) == ""
}

// oauthOperatorClientCanAuthorizeInstanceResource applies the owner/admin boundary
// for Lesser's internally provisioned operator client. Public client classes remain
// governed by the existing account-holder check above and do not receive admin scope.
func (h *Handler) oauthOperatorClientCanAuthorizeInstanceResource(ctx context.Context, client *storage.OAuthClient, principalUsername string) bool {
	authorized, err := h.oauthOperatorClientCanAuthorizeInstanceResourceStatus(ctx, client, principalUsername)
	return err == nil && authorized
}

func (h *Handler) oauthOperatorClientCanAuthorizeInstanceResourceStatus(ctx context.Context, client *storage.OAuthClient, principalUsername string) (bool, error) {
	if client == nil || !strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassOperator) {
		return true, nil
	}

	ownerID := strings.TrimSpace(client.OwnerID)
	principalUsername = strings.TrimSpace(principalUsername)
	if ownerID == "" || principalUsername == "" || !strings.EqualFold(ownerID, principalUsername) || h == nil || h.repos == nil || h.repos.Account() == nil {
		return false, nil
	}

	return h.oauthInstanceOperatorPrincipalStatus(ctx, principalUsername)
}

// oauthInstanceOperatorPrincipal reports whether the current local account is an
// active administrator who can receive instance-plane operator authority. This
// is intentionally evaluated against the account row at issuance/refresh time;
// the public OAuth client's class is not itself an authority grant.
func (h *Handler) oauthInstanceOperatorPrincipal(ctx context.Context, principalUsername string) bool {
	authorized, err := h.oauthInstanceOperatorPrincipalStatus(ctx, principalUsername)
	return err == nil && authorized
}

func (h *Handler) oauthInstanceOperatorPrincipalStatus(ctx context.Context, principalUsername string) (bool, error) {
	principalUsername = strings.TrimSpace(principalUsername)
	if h == nil || h.repos == nil || h.repos.Account() == nil || principalUsername == "" {
		return false, nil
	}

	principal, err := h.repos.Account().GetUser(ctx, principalUsername)
	if err != nil {
		return false, fmt.Errorf("load instance-plane principal: %w", err)
	}
	if principal == nil {
		return false, nil
	}

	return !principal.IsAgent && principal.Approved && !principal.Suspended && !principal.Silenced && authz.IsAdmin(principal.Role), nil
}

func (h *Handler) resolveAuthorizeTargetInstanceFromResource(ctx context.Context, resource, principalUsername string) (string, string, error) {
	parsed, err := url.Parse(strings.TrimSpace(resource))
	if err != nil || parsed == nil || parsed.User != nil {
		return "", "", oauthInvalidInstanceResourceTarget()
	}
	if parsed.ForceQuery || strings.TrimSpace(parsed.RawQuery) != "" || strings.TrimSpace(parsed.Fragment) != "" {
		return "", "", oauthInvalidInstanceResourceTarget()
	}

	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(segments) != 3 || segments[0] != oauthResourceSegmentInstance || segments[2] != oauthResourceSegmentMCP {
		return "", "", oauthInvalidInstanceResourceTarget()
	}

	surface := strings.TrimSpace(segments[1])
	if surface != oauthInstanceSurfacePtah && surface != oauthInstanceSurfaceBa {
		return "", "", oauthInvalidInstanceResourceTarget()
	}

	canonicalResource, err := h.canonicalOAuthInstanceResource(surface)
	if err != nil {
		return "", "", err
	}
	canonicalURL, err := url.Parse(canonicalResource)
	if err != nil || canonicalURL == nil {
		return "", "", errors.New("authorization server instance resource URL is invalid")
	}
	if !strings.EqualFold(parsed.Scheme, canonicalURL.Scheme) ||
		!strings.EqualFold(parsed.Host, canonicalURL.Host) ||
		parsed.EscapedPath() != canonicalURL.EscapedPath() {
		return "", "", oauthInvalidInstanceResourceTarget()
	}

	principalUsername = strings.TrimSpace(principalUsername)
	principal, err := h.repos.Account().GetUser(ctx, principalUsername)
	if err != nil {
		return "", "", fmt.Errorf("load instance-plane resource owner: %w", err)
	}
	if principal == nil || principal.IsAgent {
		return "", "", &oauthAuthorizeTargetError{
			code:        "access_denied",
			description: "instance-plane MCP requires an account-holder principal",
		}
	}

	return principalUsername, canonicalResource, nil
}

func oauthInvalidInstanceResourceTarget() error {
	return &oauthAuthorizeTargetError{
		code:        "invalid_target",
		description: "resource must be the instance-plane MCP URL for this server",
	}
}

func (h *Handler) canonicalOAuthInstanceResource(surface string) (string, error) {
	if h == nil || h.cfg == nil {
		return "", errors.New("authorization server instance resource URL is unavailable")
	}

	// BuildPublicMCPAccessBundle is the existing source of truth for the
	// public API hostname (including the api.<stageDomain> transformation).
	// Only the path differs for the Body-owned instance plane.
	actorBundle := auth.BuildPublicMCPAccessBundle(h.cfg.BaseURL(), "instance-resource")
	if actorBundle.MCPURL == "" {
		return "", errors.New("authorization server instance resource URL is invalid")
	}
	parsed, err := url.Parse(actorBundle.MCPURL)
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("authorization server instance resource URL is invalid")
	}

	parsed.Path = "/instance/" + surface + "/mcp"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (h *Handler) validateOAuthInstanceResourceOwner(ctx context.Context, resource, principalUsername, targetUsername string) error {
	principalUsername = strings.TrimSpace(principalUsername)
	targetUsername = strings.TrimSpace(targetUsername)
	if principalUsername == "" || targetUsername == "" || principalUsername != targetUsername {
		return errOAuthInvalidTarget
	}

	resolvedUsername, canonicalResource, err := h.resolveAuthorizeTargetInstanceFromResource(ctx, resource, principalUsername)
	if err != nil {
		var targetErr *oauthAuthorizeTargetError
		if errors.As(err, &targetErr) {
			return errOAuthInvalidTarget
		}
		return err
	}
	if resolvedUsername != targetUsername || canonicalResource != strings.TrimSpace(resource) {
		return errOAuthInvalidTarget
	}

	return nil
}

func (h *Handler) validateOAuthInstanceAuthorizationCodeTarget(ctx context.Context, client *storage.OAuthClient, authCode *storage.AuthorizationCode) error {
	if authCode == nil || !oauthClientCanAuthorizeInstanceResource(client) {
		return errOAuthInvalidTarget
	}
	principalUsername := oauthAuthorizationCodePrincipalUsername(authCode)
	authorized, err := h.oauthOperatorClientCanAuthorizeInstanceResourceStatus(ctx, client, principalUsername)
	if err != nil {
		return err
	}
	if !authorized {
		return errOAuthInvalidTarget
	}

	return h.validateOAuthInstanceResourceOwner(ctx, authCode.Resource, principalUsername, authCode.Username)
}

func (h *Handler) validateOAuthInstanceRefreshTokenTarget(ctx context.Context, client *storage.OAuthClient, token *storage.RefreshToken) error {
	if token == nil || !oauthClientCanAuthorizeInstanceResource(client) || strings.EqualFold(strings.TrimSpace(token.ClientClass), auth.ClientClassAgent) {
		return errOAuthInvalidTarget
	}

	username := strings.TrimSpace(token.Username)
	clientAuthorized, err := h.oauthOperatorClientCanAuthorizeInstanceResourceStatus(ctx, client, username)
	if err != nil {
		return err
	}
	if !clientAuthorized {
		return errOAuthInvalidTarget
	}
	if strings.EqualFold(strings.TrimSpace(token.ClientClass), auth.ClientClassOperator) {
		operator, operatorErr := h.oauthInstanceOperatorPrincipalStatus(ctx, username)
		if operatorErr != nil {
			return operatorErr
		}
		if !operator {
			return errOAuthInvalidTarget
		}
	}
	return h.validateOAuthInstanceResourceOwner(ctx, token.Resource, username, username)
}

func oauthAuthorizationCodePrincipalUsername(authCode *storage.AuthorizationCode) string {
	if authCode == nil {
		return ""
	}
	if principalUsername := strings.TrimSpace(authCode.PrincipalUsername); principalUsername != "" {
		return principalUsername
	}
	return strings.TrimSpace(authCode.Username)
}

func (h *Handler) ensureConsentForFlow(ctx *apptheory.Context, flow *authorizeFlow) (*apptheory.Response, error) {
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth authorization")
		return h.oauthErrorLift(ctx, "server_error", "Service unavailable", flow.request.redirectURI, flow.request.state)
	}

	if h.hasUserConsentedToApp(ctx.Context(), flow.principalUsername, flow.request.clientID, flow.request.resource, flow.scopes) {
		return nil, nil
	}

	authState := &storage.OAuthState{
		State:               flow.request.state,
		ClientID:            flow.request.clientID,
		Username:            flow.username,
		PrincipalUsername:   flow.principalUsername,
		Scopes:              flow.scopes,
		RedirectURI:         flow.request.redirectURI,
		Resource:            flow.request.resource,
		CodeChallenge:       flow.request.codeChallenge,
		CodeChallengeMethod: flow.request.codeChallengeMethod,
		ExpiresAt:           time.Now().Add(10 * time.Minute),
	}

	if _, err := h.registry.Accounts().StoreOAuthState(ctx.Context(), &accounts.StoreOAuthStateCommand{
		State:      authState.State,
		OAuthState: authState,
	}); err != nil {
		h.logger.Error("failed to save OAuth state", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to save authorization state", flow.request.redirectURI, flow.request.state)
	}

	h.logger.Info("redirecting to consent UI",
		zap.String("username", flow.username),
		zap.String("client_id", flow.request.clientID))

	return h.redirectToConsentUI(ctx, authState)
}

func (h *Handler) completeAuthorizationFlow(ctx *apptheory.Context, flow *authorizeFlow) (*apptheory.Response, error) {
	code, err := flow.oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		h.logger.Error("failed to generate authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to generate authorization code", flow.request.redirectURI, flow.request.state)
	}

	authCode := &storage.AuthorizationCode{
		Code:              code,
		ClientID:          flow.request.clientID,
		RedirectURI:       flow.request.redirectURI,
		Resource:          flow.request.resource,
		Username:          flow.username,
		PrincipalUsername: flow.principalUsername,
		CodeChallenge:     flow.request.codeChallenge,
		ExpiresAt:         time.Now().Add(10 * time.Minute),
		Scopes:            flow.scopes,
	}

	if _, err := h.registry.Accounts().CreateAuthorizationCode(ctx.Context(), &accounts.CreateAuthorizationCodeCommand{
		AuthCode: authCode,
	}); err != nil {
		h.logger.Error("failed to store authorization code", zap.Error(err))
		return h.oauthErrorLift(ctx, "server_error", "Failed to store authorization code", flow.request.redirectURI, flow.request.state)
	}

	redirectURL, _ := url.Parse(flow.request.redirectURI)
	query := redirectURL.Query()
	query.Set("code", code)
	if flow.request.state != "" {
		query.Set("state", flow.request.state)
	}
	redirectURL.RawQuery = query.Encode()
	return h.writeOAuthAuthorizeRedirect(ctx, redirectURL.String())
}

// Helper methods for Lift implementation

// oauthErrorLift handles OAuth errors in Lift style
func (h *Handler) oauthErrorLift(ctx *apptheory.Context, errorCode, errorDescription, redirectURI, state string) (*apptheory.Response, error) {
	// If no redirect URI, return JSON error
	if err := common.ValidateRequiredParam("redirect_uri", redirectURI); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             errorCode,
			"error_description": errorDescription,
		})
	}

	// Build redirect URL with error parameters
	u, err := url.Parse(redirectURI)
	if err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid redirect_uri",
		})
	}

	q := u.Query()
	q.Set("error", errorCode)
	q.Set("error_description", errorDescription)
	if state != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return h.writeOAuthAuthorizeRedirect(ctx, u.String())
}

// getUserFromSessionLift extracts the username from the session using Lift patterns
func (h *Handler) getUserFromSessionLift(ctx *apptheory.Context) string {
	return h.getOptionalAuthenticatedUser(ctx)
}

// redirectToConsentUI redirects to the hosted OAuth consent UI
func (h *Handler) redirectToConsentUI(ctx *apptheory.Context, authState *storage.OAuthState) (*apptheory.Response, error) {
	// Check if registry is initialized
	if h.registry == nil {
		h.logger.Error("service registry not initialized for OAuth consent")
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Service unavailable",
		})
	}

	// Get app details to pass to consent UI
	result, err := h.registry.Accounts().GetOAuthApp(ctx.Context(), &accounts.GetOAuthAppQuery{
		ClientID: authState.ClientID,
	})
	if err != nil {
		h.logger.Error("failed to get OAuth app", zap.Error(err))
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "client not found",
		})
	}
	app := result.App

	authUIBaseURL := fmt.Sprintf("https://%s/auth", h.cfg.Domain)

	// Build consent URL with all necessary parameters.
	consentURL := fmt.Sprintf("%s/consent?state=%s&client_id=%s&client_name=%s&client_url=%s&scopes=%s&redirect_uri=%s",
		authUIBaseURL,
		url.QueryEscape(authState.State),
		url.QueryEscape(authState.ClientID),
		url.QueryEscape(app.Name),
		url.QueryEscape(app.Website),
		url.QueryEscape(strings.Join(authState.Scopes, " ")),
		url.QueryEscape(authState.RedirectURI))
	if strings.TrimSpace(authState.Resource) != "" {
		consentURL += "&resource=" + url.QueryEscape(authState.Resource)
	}
	return h.writeOAuthAuthorizeRedirect(ctx, consentURL)
}

// hasUserConsentedToApp checks if user has consented to the app with required scopes
func (h *Handler) hasUserConsentedToApp(ctx context.Context, username, clientID, resource string, scopes []string) bool {
	result, err := h.registry.Accounts().GetUserAppConsent(ctx, &accounts.GetUserAppConsentQuery{
		Username: username,
		ClientID: clientID,
		Resource: resource,
	})
	if err != nil || result == nil || result.Consent == nil {
		return false
	}
	consent := result.Consent

	// Check if all requested scopes are granted
	grantedMap := make(map[string]bool)
	for _, s := range consent.Scopes {
		grantedMap[s] = true
	}

	for _, s := range scopes {
		if !grantedMap[s] {
			return false
		}
	}

	return true
}

type oauthTokenRequest struct {
	grantType    string
	code         string
	redirectURI  string
	clientID     string
	clientSecret string
	resource     string
	codeVerifier string
	refreshToken string
	deviceCode   string
	scope        string
}

func parseOAuthTokenRequest(ctx *apptheory.Context) (*oauthTokenRequest, *apptheory.Response, error) {
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Empty request body",
		})
		return nil, resp, respErr
	}

	contentType := strings.ToLower(strings.TrimSpace(headerValue(ctx, "Content-Type")))
	if contentType == "" {
		contentType = strings.ToLower(strings.TrimSpace(headerValue(ctx, "content-type")))
	}
	if !strings.HasPrefix(contentType, common.ContentTypeForm) {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "token endpoint requires application/x-www-form-urlencoded",
		})
		return nil, resp, respErr
	}

	params, err := common.ParseFormURLEncoded(string(ctx.Request.Body))
	if err != nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": "Unable to parse form data",
		})
		return nil, resp, respErr
	}

	clientID := params["client_id"]
	clientSecret := params["client_secret"]
	basicClientID, basicClientSecret, usedBasicAuth, basicErr := parseOAuthTokenBasicClientCredentials(ctx)
	if basicErr != nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
		return nil, resp, respErr
	}
	if usedBasicAuth {
		clientID = basicClientID
		clientSecret = basicClientSecret
	}

	resource, err := normalizeOAuthResourceIndicator(params["resource"])
	if err != nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_target",
			"error_description": err.Error(),
		})
		return nil, resp, respErr
	}

	return &oauthTokenRequest{
		grantType:    params["grant_type"],
		code:         params["code"],
		redirectURI:  params["redirect_uri"],
		clientID:     clientID,
		clientSecret: clientSecret,
		resource:     resource,
		codeVerifier: params["code_verifier"],
		refreshToken: params["refresh_token"],
		deviceCode:   params["device_code"],
		scope:        params["scope"],
	}, nil, nil
}

func parseOAuthTokenBasicClientCredentials(ctx *apptheory.Context) (clientID, clientSecret string, used bool, err error) {
	authHeader := strings.TrimSpace(headerValue(ctx, "Authorization"))
	if authHeader == "" {
		authHeader = strings.TrimSpace(headerValue(ctx, "authorization"))
	}
	if authHeader == "" {
		return "", "", false, nil
	}
	if !strings.HasPrefix(strings.ToLower(authHeader), "basic ") {
		return "", "", false, nil
	}

	encoded := strings.TrimSpace(authHeader[len("Basic "):])
	decoded, decodeErr := base64.StdEncoding.DecodeString(encoded)
	if decodeErr != nil {
		return "", "", true, decodeErr
	}

	clientID, clientSecret, found := strings.Cut(string(decoded), ":")
	if !found || strings.TrimSpace(clientID) == "" {
		return "", "", true, errors.New("invalid basic auth credentials")
	}

	return clientID, clientSecret, true, nil
}

// HandleOAuthTokenLift handles the OAuth token endpoint using native Lift patterns
// POST /oauth/token
func (h *Handler) HandleOAuthTokenLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	req, resp, err := parseOAuthTokenRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	// Initialize OAuth service
	oauthSvc, err := createOAuthService(h.cfg.JWTSecret, h.cfg, h.repos, h.logger)
	if err != nil {
		h.logger.Error("failed to initialize OAuth service", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, apimodels.OAuthErrorResponse{
			Error:            "server_error",
			ErrorDescription: "OAuth service unavailable",
		})
	}

	switch req.grantType {
	case oauthGrantTypeAuthorizationCode:
		return h.handleOAuthAuthorizationCodeGrant(ctx.Context(), oauthSvc, req)
	case oauthGrantTypeRefreshToken:
		return h.handleOAuthRefreshTokenGrant(ctx, oauthSvc, req)
	case oauthDeviceCodeGrantType:
		return h.handleOAuthDeviceCodeGrant(ctx.Context(), oauthSvc, req)
	default:
		description := "Only authorization_code and refresh_token grant types are supported"
		if oauthDeviceFlowEnabled(h.cfg) {
			description = "Only authorization_code, refresh_token, and device_code grant types are supported"
		}
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "unsupported_grant_type",
			"error_description": description,
		})
	}
}

func (h *Handler) handleOAuthAuthorizationCodeGrant(ctx context.Context, oauthSvc *auth.OAuthService, req *oauthTokenRequest) (*apptheory.Response, error) {
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"code":         req.code,
		"client_id":    req.clientID,
		"redirect_uri": req.redirectURI,
	}); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": err.Error(),
		})
	}

	accessToken, refreshTokenOut, grantedScopes, err := h.exchangeAuthorizationCode(ctx, oauthSvc, req.code, req.clientID, req.redirectURI, req.codeVerifier, req.clientSecret, req.resource)
	if err != nil {
		h.logger.Error("failed to exchange authorization code", zap.Error(err))
		switch {
		case errors.Is(err, auth.ErrOAuthTemporarilyUnavailable):
			return oauthTemporarilyUnavailable("Authorization code exchange is temporarily unavailable")
		case errors.Is(err, errOAuthInvalidTarget):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_target",
				"error_description": "resource must match the original authorization request",
			})
		case errors.Is(err, auth.ErrInvalidGrant):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "Invalid authorization code or expired",
			})
		case errors.Is(err, auth.ErrInvalidClient):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_client",
				"error_description": "Invalid client credentials",
			})
		case errors.Is(err, auth.ErrUnauthorizedClient):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "unauthorized_client",
				"error_description": "This client is not allowed to use authorization_code",
			})
		case errors.Is(err, auth.ErrInvalidCodeChallenge):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "PKCE verification failed",
			})
		default:
			return apptheory.JSON(http.StatusInternalServerError, map[string]string{
				"error":             "server_error",
				"error_description": "Authorization code exchange failed",
			})
		}
	}

	return okJSON(apimodels.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    oauthTokenTypeBearer,
		Scope:        strings.Join(grantedScopes, " "),
		CreatedAt:    time.Now().Unix(),
		ExpiresIn:    oauthTokenExpiresInSeconds,
		RefreshToken: refreshTokenOut,
	})
}

func (h *Handler) handleOAuthRefreshTokenGrant(ctx *apptheory.Context, oauthSvc *auth.OAuthService, req *oauthTokenRequest) (*apptheory.Response, error) {
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"refresh_token": req.refreshToken,
		"client_id":     req.clientID,
	}); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": err.Error(),
		})
	}

	userAgent, ipAddress := h.getDeviceInfo(ctx)
	accessToken, newRefreshToken, grantedScopes, err := h.exchangeRefreshToken(ctx.Context(), oauthSvc, req.refreshToken, req.clientID, req.clientSecret, req.resource, ipAddress, userAgent)
	if err != nil {
		h.logger.Error("failed to refresh tokens", zap.Error(err))
		switch {
		case errors.Is(err, auth.ErrOAuthTemporarilyUnavailable):
			return oauthTemporarilyUnavailable("Refresh token exchange is temporarily unavailable")
		case errors.Is(err, auth.ErrInvalidToken):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "Invalid or expired refresh token",
			})
		case errors.Is(err, auth.ErrInvalidClient):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_client",
				"error_description": "Invalid client credentials",
			})
		case errors.Is(err, auth.ErrUnauthorizedClient):
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "unauthorized_client",
				"error_description": "This client is not allowed to use refresh_token",
			})
		default:
			return apptheory.JSON(http.StatusInternalServerError, map[string]string{
				"error":             "server_error",
				"error_description": "Refresh token exchange failed",
			})
		}
	}

	return okJSON(apimodels.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    oauthTokenTypeBearer,
		Scope:        strings.Join(grantedScopes, " "),
		CreatedAt:    time.Now().Unix(),
		ExpiresIn:    oauthTokenExpiresInSeconds,
		RefreshToken: newRefreshToken,
	})
}

func oauthTemporarilyUnavailable(description string) (*apptheory.Response, error) {
	resp, err := apptheory.JSON(http.StatusServiceUnavailable, map[string]string{
		"error":             "temporarily_unavailable",
		"error_description": description,
	})
	if resp != nil {
		setHeader(resp, "Retry-After", "1")
	}
	return resp, err
}

func (h *Handler) handleOAuthDeviceCodeGrant(ctx context.Context, oauthSvc *auth.OAuthService, req *oauthTokenRequest) (*apptheory.Response, error) {
	if h.cfg == nil || !h.cfg.AllowDeviceFlow {
		return apptheory.JSON(http.StatusForbidden, map[string]string{
			"error":             "access_denied",
			"error_description": "device authorization is disabled",
		})
	}

	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"device_code": req.deviceCode,
		"client_id":   req.clientID,
	}); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_request",
			"error_description": err.Error(),
		})
	}

	client, err := h.repos.Account().GetOAuthClient(ctx, strings.TrimSpace(req.clientID))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return oauthTemporarilyUnavailable("Device authorization is temporarily unavailable")
		}
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
	}
	if client == nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
	}
	resp, err := validateOAuthDeviceGrantClientSecret(ctx, oauthSvc, client, req.clientSecret)
	if resp != nil || err != nil {
		return resp, err
	}
	if !oauthClientSupportsGrantType(client, oauthDeviceCodeGrantType) {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "unauthorized_client",
			"error_description": "device_code is not allowed for this client",
		})
	}

	session, resp, err := h.loadOAuthDeviceSessionForTokenGrant(ctx, req.deviceCode, req.clientID)
	if resp != nil || err != nil {
		return resp, err
	}

	now := time.Now().UTC()
	resp, err = h.enforceOAuthDeviceSessionPolling(ctx, session, now)
	if resp != nil || err != nil {
		return resp, err
	}

	return h.oauthDeviceSessionTokenResponse(ctx, oauthSvc, session, req.clientID, now)
}

//nolint:unused // Kept for direct helper coverage and future optional-secret OAuth endpoints.
func (h *Handler) validateOAuthClientSecretIfProvided(ctx context.Context, oauthSvc *auth.OAuthService, clientID, clientSecret string) (*apptheory.Response, error) {
	if clientSecret == "" {
		return nil, nil
	}

	if err := oauthSvc.ValidateClient(ctx, clientID, clientSecret); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
	}

	return nil, nil
}

func validateOAuthDeviceGrantClientSecret(ctx context.Context, oauthSvc *auth.OAuthService, client *storage.OAuthClient, clientSecret string) (*apptheory.Response, error) {
	if err := validateRefreshGrantClientSecret(ctx, oauthSvc, client, clientSecret); err != nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_client",
			"error_description": "Invalid client credentials",
		})
	}
	return nil, nil
}

func oauthClientSupportsGrantType(client *storage.OAuthClient, grantType string) bool {
	if client == nil {
		return false
	}
	normalizedGrantType := strings.ToLower(strings.TrimSpace(grantType))
	clientClass := strings.ToLower(strings.TrimSpace(client.ClientClass))
	if normalizedGrantType == oauthGrantTypeClientCredentials {
		return false
	}
	if normalizedGrantType == oauthDeviceCodeGrantType && clientClass == auth.ClientClassAgent {
		return false
	}
	if len(client.GrantTypes) == 0 {
		switch normalizedGrantType {
		case oauthGrantTypeAuthorizationCode, oauthGrantTypeRefreshToken:
			return true
		case oauthDeviceCodeGrantType:
			return clientClass == auth.ClientClassCLI
		default:
			return false
		}
	}
	for _, allowedGrant := range client.GrantTypes {
		if strings.EqualFold(strings.TrimSpace(allowedGrant), grantType) {
			return true
		}
	}
	return false
}

func (h *Handler) loadOAuthDeviceSessionForTokenGrant(ctx context.Context, deviceCode, clientID string) (*storage.OAuthDeviceSession, *apptheory.Response, error) {
	session, err := h.repos.Account().GetOAuthDeviceSession(ctx, oauthDeviceCodeHash(deviceCode))
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			resp, respErr := oauthTemporarilyUnavailable("Device authorization is temporarily unavailable")
			return nil, resp, respErr
		}
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid device_code",
		})
		return nil, resp, respErr
	}
	if session == nil {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid device_code",
		})
		return nil, resp, respErr
	}

	if session.ClientID != clientID {
		resp, respErr := apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid device_code",
		})
		return nil, resp, respErr
	}

	return session, nil, nil
}

func (h *Handler) enforceOAuthDeviceSessionPolling(ctx context.Context, session *storage.OAuthDeviceSession, now time.Time) (*apptheory.Response, error) {
	if session == nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid device_code",
		})
	}

	if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "expired_token",
			"error_description": "The device_code has expired",
		})
	}

	intervalSeconds := session.IntervalSeconds
	if intervalSeconds <= 0 {
		intervalSeconds = oauthDevicePollIntervalSeconds
	}

	if !session.LastPolledAt.IsZero() {
		nextAllowed := session.LastPolledAt.Add(time.Duration(intervalSeconds) * time.Second)
		if now.Before(nextAllowed) {
			session.PollCount++
			if updateErr := h.repos.Account().UpdateOAuthDeviceSession(ctx, session); updateErr != nil {
				h.logger.Warn("failed to update oauth device session poll metadata", zap.Error(updateErr))
				return oauthTemporarilyUnavailable("Device authorization is temporarily unavailable")
			}

			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "slow_down",
				"error_description": "Polling too frequently",
			})
		}
	}

	session.PollCount++
	session.LastPolledAt = now
	if updateErr := h.repos.Account().UpdateOAuthDeviceSession(ctx, session); updateErr != nil {
		h.logger.Warn("failed to update oauth device session poll metadata", zap.Error(updateErr))
		return oauthTemporarilyUnavailable("Device authorization is temporarily unavailable")
	}

	return nil, nil
}

func (h *Handler) oauthDeviceSessionTokenResponse(ctx context.Context, oauthSvc *auth.OAuthService, session *storage.OAuthDeviceSession, clientID string, now time.Time) (*apptheory.Response, error) {
	status := strings.ToLower(strings.TrimSpace(session.Status))
	switch status {
	case oauthDeviceSessionStatusPending:
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "authorization_pending",
			"error_description": "Authorization is pending",
		})
	case oauthDeviceSessionStatusDenied:
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "access_denied",
			"error_description": "Access denied",
		})
	case oauthDeviceSessionStatusConsumed:
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Invalid device_code",
		})
	case oauthDeviceSessionStatusApproved:
		return h.oauthDeviceSessionApprovedTokenResponse(ctx, oauthSvc, session, clientID, now)
	default:
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Device authorization is in an invalid state",
		})
	}
}

func (h *Handler) oauthDeviceSessionApprovedTokenResponse(ctx context.Context, oauthSvc *auth.OAuthService, session *storage.OAuthDeviceSession, clientID string, now time.Time) (*apptheory.Response, error) {
	username := strings.TrimSpace(session.ApprovedUsername)
	if username == "" {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Device authorization is in an invalid state",
		})
	}

	client, err := h.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return oauthTemporarilyUnavailable("Device authorization is temporarily unavailable")
		}
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Device authorization is no longer valid",
		})
	}
	if client == nil {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Device authorization is no longer valid",
		})
	}

	tokenUsername, clientClass, sessionID, accessTTL, err := h.oauthDeviceApprovedTokenContext(ctx, client, username)
	if err != nil {
		return h.oauthDeviceApprovedTokenContextErrorResponse(err)
	}

	accessToken, refreshTokenOut, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContext(
		ctx,
		tokenUsername,
		clientID,
		"",
		session.Scopes,
		accessTTL,
		clientClass,
		sessionID,
	)
	if err != nil {
		h.logger.Error("failed to generate tokens for device flow", zap.Error(err))
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error":             "server_error",
			"error_description": "Failed to generate tokens",
		})
	}

	oauthRefreshToken := buildAuthorizationCodeRefreshToken(now, refreshTokenOut, clientID, client, &storage.AuthorizationCode{
		Username: tokenUsername,
		Scopes:   session.Scopes,
	}, clientClass, sessionID, accessTTL)
	if err := h.repos.Account().ConsumeOAuthDeviceSessionAndCreateRefreshToken(ctx, session, oauthRefreshToken, now); err != nil {
		h.logger.Error("failed to atomically consume device session and store refresh token", zap.Error(err))
		if errors.Is(err, storage.ErrNotFound) {
			return apptheory.JSON(http.StatusBadRequest, map[string]string{
				"error":             "invalid_grant",
				"error_description": "Device authorization is no longer valid",
			})
		}
		return oauthTemporarilyUnavailable("Device token issuance is temporarily unavailable")
	}

	return okJSON(apimodels.OAuthTokenResponse{
		AccessToken:  accessToken,
		TokenType:    oauthTokenTypeBearer,
		Scope:        strings.Join(session.Scopes, " "),
		CreatedAt:    now.Unix(),
		ExpiresIn:    int(accessTTL.Seconds()),
		RefreshToken: refreshTokenOut,
	})
}

func (h *Handler) oauthDeviceApprovedTokenContext(_ context.Context, client *storage.OAuthClient, approvedUsername string) (string, string, string, time.Duration, error) {
	clientClass := strings.ToLower(strings.TrimSpace(client.ClientClass))
	if clientClass == "" {
		clientClass = auth.ClientClassCLI
	}
	if clientClass == auth.ClientClassAgent {
		return "", "", "", 0, auth.ErrInvalidGrant
	}

	tokenUsername := strings.TrimSpace(approvedUsername)
	sessionID := ""
	accessTTL := auth.AccessTokenDuration
	if clientClass == auth.ClientClassCLI {
		sessionID = common.GenerateSessionIDULID()
	}

	return tokenUsername, clientClass, sessionID, accessTTL, nil
}

func (h *Handler) oauthDeviceApprovedTokenContextErrorResponse(err error) (*apptheory.Response, error) {
	if errors.Is(err, auth.ErrInvalidGrant) {
		return apptheory.JSON(http.StatusBadRequest, map[string]string{
			"error":             "invalid_grant",
			"error_description": "Device authorization is no longer valid",
		})
	}

	return apptheory.JSON(http.StatusInternalServerError, map[string]string{
		"error":             "server_error",
		"error_description": "Device authorization is in an invalid state",
	})
}

// exchangeAuthorizationCode exchanges an authorization code for access and refresh tokens
func (h *Handler) exchangeAuthorizationCode(ctx context.Context, oauthSvc *auth.OAuthService, code, clientID, redirectURI, codeVerifier, clientSecret, requestedResource string) (string, string, []string, error) {
	code = strings.TrimSpace(code)
	clientID = strings.TrimSpace(clientID)
	redirectURI = strings.TrimSpace(redirectURI)
	clientSecret = strings.TrimSpace(clientSecret)
	requestedResource = strings.TrimSpace(requestedResource)

	client, err := h.validateAuthorizationCodeExchangeClient(ctx, oauthSvc, clientID, redirectURI, clientSecret, code)
	if err != nil {
		return "", "", nil, err
	}

	authCode, err := h.loadAndValidateAuthorizationCodeForExchange(ctx, oauthSvc, code, clientID, redirectURI, codeVerifier)
	if err != nil {
		return "", "", nil, err
	}
	if err := validateOAuthAuthorizationCodeScopes(client, authCode); err != nil {
		return "", "", nil, err
	}
	storedResource := strings.TrimSpace(authCode.Resource)
	if storedResource != "" && requestedResource == "" {
		// Some public MCP clients bind the resource at authorization time but omit it
		// during the token exchange. Preserve the original code-bound resource instead
		// of rejecting an otherwise valid authorization-code flow.
		requestedResource = storedResource
	}
	if storedResource != requestedResource {
		return "", "", nil, errOAuthInvalidTarget
	}
	if oauthResourceTargetsInstancePlane(storedResource) {
		if err := h.validateOAuthInstanceAuthorizationCodeTarget(ctx, client, authCode); err != nil {
			return "", "", nil, err
		}
	}

	_, sessionID, accessTTL, err := authorizationCodeExchangeTokenContext(h.cfg, client, authCode)
	if err != nil {
		return "", "", nil, err
	}
	clientClass, err := h.oauthAuthorizationCodeClientClass(ctx, client, authCode)
	if err != nil {
		return "", "", nil, err
	}

	// Generate tokens
	accessToken, refreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContextAndAudienceAndDelegatedBy(ctx, authCode.Username, clientID, "", authCode.Scopes, accessTTL, clientClass, sessionID, authCode.Resource, authCode.PrincipalUsername)
	if err != nil {
		return "", "", nil, errors.Join(failedToGenerateTokens(), err)
	}

	now := time.Now().UTC()
	oauthRefreshToken := buildAuthorizationCodeRefreshToken(now, refreshToken, clientID, client, authCode, clientClass, sessionID, accessTTL)

	if err := h.repos.Account().ConsumeAuthorizationCodeAndCreateRefreshToken(ctx, code, oauthRefreshToken); err != nil {
		h.logger.Error("failed to atomically consume authorization code and store refresh token", zap.Error(err))
		if errors.Is(err, storage.ErrNotFound) {
			return "", "", nil, auth.ErrInvalidGrant
		}
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}

	return accessToken, refreshToken, authCode.Scopes, nil
}

func (h *Handler) validateAuthorizationCodeExchangeClient(ctx context.Context, oauthSvc *auth.OAuthService, clientID, redirectURI, clientSecret, code string) (*storage.OAuthClient, error) {
	client, err := h.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, auth.ErrInvalidClient
		}
		return nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	if client == nil {
		return nil, auth.ErrInvalidClient
	}
	if !oauthClientSupportsGrantType(client, oauthGrantTypeAuthorizationCode) {
		return nil, auth.ErrUnauthorizedClient
	}
	if err := validateAuthorizationCodeExchangeRedirect(client, clientID, redirectURI, code); err != nil {
		return nil, err
	}
	if err := validateAuthorizationCodeExchangeClientSecret(ctx, oauthSvc, client, clientSecret); err != nil {
		return nil, err
	}
	return client, nil
}

func validateAuthorizationCodeExchangeRedirect(client *storage.OAuthClient, clientID, redirectURI, code string) error {
	if err := common.ValidateMultipleRequiredParams(map[string]string{
		"clientID":    clientID,
		"redirectURI": redirectURI,
		"authCode":    code,
	}); err != nil {
		return auth.ErrInvalidRequest
	}
	for _, registeredURI := range client.RedirectURIs {
		if auth.RedirectURIsMatch(client, registeredURI, redirectURI) {
			return nil
		}
	}
	return auth.ErrInvalidRequest
}

func validateAuthorizationCodeExchangeClientSecret(ctx context.Context, oauthSvc *auth.OAuthService, client *storage.OAuthClient, clientSecret string) error {
	if client.Confidential {
		if clientSecret == "" {
			return auth.ErrInvalidClient
		}
		return oauthSvc.ValidateClientRecord(ctx, client, clientSecret)
	}
	if clientSecret == "" {
		return nil
	}
	return oauthSvc.ValidateClientRecord(ctx, client, clientSecret)
}

func (h *Handler) loadAndValidateAuthorizationCodeForExchange(ctx context.Context, oauthSvc *auth.OAuthService, code, clientID, redirectURI, codeVerifier string) (*storage.AuthorizationCode, error) {
	authCode, err := h.repos.Account().GetAuthorizationCode(ctx, code)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, auth.ErrInvalidGrant
		}
		return nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	if authCode.ClientID != clientID {
		return nil, auth.ErrInvalidGrant
	}
	if strings.TrimSpace(authCode.RedirectURI) == "" || authCode.RedirectURI != redirectURI {
		return nil, auth.ErrInvalidGrant
	}
	// PKCE verification: when the authorization code carries a challenge,
	// verify the proof against the supplied verifier. The authorize endpoint
	// now enforces PKCE for all public clients; this exchange check is
	// defense-in-depth and handles pre-existing authorization codes that
	// may have been issued before PKCE enforcement (no stored challenge).
	if authCode.CodeChallenge != "" {
		if codeVerifier == "" {
			return nil, auth.ErrInvalidGrant
		}
		if err := oauthSvc.VerifyCodeChallenge(authCode.CodeChallenge, codeVerifier, "S256"); err != nil {
			return nil, err
		}
	} else if codeVerifier != "" {
		// Legacy authorization code without a PKCE challenge stored; the
		// exchange supplies a verifier but the code was not issued for PKCE.
		// Reject as a grant-level mismatch (invalid_grant).
		return nil, auth.ErrInvalidGrant
	}
	return authCode, nil
}

func validateOAuthAuthorizationCodeScopes(client *storage.OAuthClient, authCode *storage.AuthorizationCode) error {
	if client == nil || authCode == nil {
		return auth.ErrInvalidGrant
	}
	if err := auth.ValidateScopes(authCode.Scopes); err != nil {
		return auth.ErrInvalidScope
	}
	if !strings.EqualFold(strings.TrimSpace(client.ClientClass), auth.ClientClassOperator) {
		if err := auth.ValidatePublicOAuthScopes(authCode.Scopes); err != nil {
			return auth.ErrInvalidScope
		}
	} else if !oauthResourceTargetsInstancePlane(authCode.Resource) && oauthScopesContainAdmin(authCode.Scopes) {
		return auth.ErrInvalidScope
	}
	if len(client.Scopes) > 0 && !auth.ScopeSetAllows(client.Scopes, authCode.Scopes) {
		return auth.ErrInvalidScope
	}
	return nil
}

func oauthScopesContainAdmin(scopes []string) bool {
	for _, scope := range scopes {
		scope = strings.ToLower(strings.TrimSpace(scope))
		if scope == auth.ScopeAdmin || strings.HasPrefix(scope, auth.ScopeAdmin+":") {
			return true
		}
	}
	return false
}

func oauthClientRequiresPKCE(client *storage.OAuthClient) bool {
	// All public (non-confidential) clients must use PKCE, regardless of
	// registration source. The previous implementation only required PKCE
	// for dynamically-registered clients, which left manually-registered
	// public clients without proof-of-possession protection.
	return client != nil && !client.Confidential
}

func oauthAuthorizeRequiresResource(client *storage.OAuthClient) bool {
	// Only dynamic clients require explicit MCP resource binding.
	return client != nil &&
		!client.Confidential &&
		strings.EqualFold(strings.TrimSpace(client.RegistrationSource), oauthRegistrationSourceDynamic)
}

func requireOAuthPKCE(codeChallenge, codeChallengeMethod string) error {
	codeChallenge = strings.TrimSpace(codeChallenge)
	codeChallengeMethod = strings.TrimSpace(codeChallengeMethod)
	if codeChallenge == "" {
		return errors.New("public clients must use PKCE")
	}
	if !strings.EqualFold(codeChallengeMethod, "S256") {
		return errors.New("public clients must use code_challenge_method=S256")
	}
	return nil
}

func authorizationCodeExchangeTokenContext(cfg *config.Config, client *storage.OAuthClient, authCode *storage.AuthorizationCode) (string, string, time.Duration, error) {
	clientClass := strings.ToLower(strings.TrimSpace(client.ClientClass))
	sessionID := ""
	accessTTL := auth.AccessTokenDuration
	if clientClass == auth.ClientClassCLI || clientClass == auth.ClientClassAgent {
		sessionID = common.GenerateSessionIDULID()
	}
	if clientClass != auth.ClientClassAgent {
		return clientClass, sessionID, accessTTL, nil
	}
	if strings.TrimSpace(authCode.Username) == "" {
		return "", "", 0, auth.ErrInvalidGrant
	}
	if strings.TrimSpace(authCode.Resource) == "" {
		return "", "", 0, auth.ErrInvalidGrant
	}
	return clientClass, sessionID, auth.AgentAccessTokenTTL(cfg), nil
}

// oauthAuthorizationCodeClientClass derives the authority marker for the token
// being issued. A dynamically registered client remains cli/web in storage and
// receives operator authority only for an exact instance-plane resource when the
// authorization-code principal is still an active local administrator. The
// marker is therefore bound to this token audience, not to the public client.
func (h *Handler) oauthAuthorizationCodeClientClass(ctx context.Context, client *storage.OAuthClient, authCode *storage.AuthorizationCode) (string, error) {
	if client == nil || authCode == nil {
		return "", auth.ErrInvalidGrant
	}

	// Derive the class from the persisted client record rather than trusting the
	// caller-supplied context. The operator marker is an issuance decision, so a
	// stale or inconsistent intermediate value must not elevate a generic client.
	clientClass := strings.ToLower(strings.TrimSpace(client.ClientClass))
	if !oauthResourceTargetsInstancePlane(authCode.Resource) {
		if clientClass == auth.ClientClassOperator {
			// The internally provisioned operator client may still be used for a
			// non-instance OAuth resource, but it must not export its operator
			// marker outside the instance plane.
			return auth.ClientClassWeb, nil
		}
		return clientClass, nil
	}

	if err := h.validateOAuthInstanceAuthorizationCodeTarget(ctx, client, authCode); err != nil {
		return "", err
	}
	if clientClass != auth.ClientClassCLI && clientClass != auth.ClientClassWeb && clientClass != auth.ClientClassOperator {
		return clientClass, nil
	}

	principalUsername := oauthAuthorizationCodePrincipalUsername(authCode)
	if h.oauthInstanceOperatorPrincipal(ctx, principalUsername) {
		return auth.ClientClassOperator, nil
	}
	if clientClass == auth.ClientClassOperator {
		return "", errOAuthInvalidTarget
	}
	return clientClass, nil
}

func buildAuthorizationCodeRefreshToken(now time.Time, refreshToken, clientID string, client *storage.OAuthClient, authCode *storage.AuthorizationCode, clientClass, sessionID string, accessTTL time.Duration) *storage.RefreshToken {
	oauthRefreshToken := &storage.RefreshToken{
		Token:             refreshToken,
		Username:          authCode.Username,
		PrincipalUsername: authCode.PrincipalUsername,
		ClientID:          clientID,
		Resource:          authCode.Resource,
		Scopes:            authCode.Scopes,
		CreatedAt:         now,
		ExpiresAt:         now.Add(auth.RefreshTokenDuration),
		ClientClass:       clientClass,
		SessionID:         sessionID,
		LastAuthSuccessAt: now,
		FamilyID:          common.GenerateSessionIDULID(),
		Generation:        1,
		Current:           true,
	}
	if !auth.IsAgentRuntimeClientID(clientID) {
		return oauthRefreshToken
	}

	refreshIdleTTL := time.Duration(0)
	refreshAbsoluteTTL := time.Duration(0)
	if clientID == delegatedAgentClientID && accessTTL > 0 {
		refreshIdleTTL = accessTTL
		refreshAbsoluteTTL = accessTTL
	}
	idleExpiry, absoluteExpiry := auth.AgentRuntimeRefreshExpiries(now, refreshIdleTTL, refreshAbsoluteTTL)

	oauthRefreshToken.ExpiresAt = idleExpiry
	labelPrimary := ""
	if client != nil {
		labelPrimary = client.Name
	}
	oauthRefreshToken.DeviceLabel = auth.CoalesceAgentRuntimeLabel(labelPrimary, "")
	oauthRefreshToken.LastUsedAt = now
	oauthRefreshToken.IdleExpiresAt = idleExpiry
	oauthRefreshToken.AbsoluteExpiresAt = absoluteExpiry
	oauthRefreshToken.SessionCreatedAt = now
	oauthRefreshToken.AccessTTLSeconds = int(accessTTL.Seconds())
	return oauthRefreshToken
}

func (h *Handler) validateRefreshGrantClient(ctx context.Context, oauthSvc *auth.OAuthService, refreshToken, clientID, clientSecret string) (*storage.OAuthClient, error) {
	client, err := h.repos.Account().GetOAuthClient(ctx, clientID)
	if err != nil {
		if !errors.Is(err, storage.ErrNotFound) {
			return nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
		}
		client = nil
	}
	if client == nil {
		if auth.IsAgentRuntimeClientID(clientID) {
			h.noteAgentRuntimeRefreshFailure(ctx, refreshToken, clientID, "invalid_client", "Invalid client credentials")
		}
		return nil, auth.ErrInvalidClient
	}
	if !oauthClientSupportsGrantType(client, oauthGrantTypeRefreshToken) {
		if auth.IsAgentRuntimeClientID(clientID) {
			h.noteAgentRuntimeRefreshFailure(ctx, refreshToken, clientID, "unauthorized_client", "refresh_token is not allowed for this client")
		}
		return nil, auth.ErrUnauthorizedClient
	}
	if err := validateRefreshGrantClientSecret(ctx, oauthSvc, client, clientSecret); err != nil {
		if auth.IsAgentRuntimeClientID(clientID) {
			h.noteAgentRuntimeRefreshFailure(ctx, refreshToken, clientID, "invalid_client", "Invalid client credentials")
		}
		return nil, err
	}
	return client, nil
}

func validateRefreshGrantClientSecret(ctx context.Context, oauthSvc *auth.OAuthService, client *storage.OAuthClient, clientSecret string) error {
	if client.Confidential && clientSecret == "" {
		return auth.ErrInvalidClient
	}
	if clientSecret == "" {
		return nil
	}
	return oauthSvc.ValidateClientRecord(ctx, client, clientSecret)
}

func refreshGrantClientClass(client *storage.OAuthClient, storedToken *storage.RefreshToken) string {
	if storedToken != nil {
		if clientClass := strings.TrimSpace(storedToken.ClientClass); clientClass != "" {
			return strings.ToLower(clientClass)
		}
	}
	if client != nil {
		return strings.ToLower(strings.TrimSpace(client.ClientClass))
	}
	return ""
}

func refreshTokenCarriesRuntimeDecisionState(token *storage.RefreshToken) bool {
	if token == nil {
		return false
	}
	return strings.TrimSpace(token.FamilyID) != "" ||
		token.Generation > 0 ||
		token.Current ||
		strings.TrimSpace(token.DeviceLabel) != "" ||
		!token.LastUsedAt.IsZero() ||
		!token.IdleExpiresAt.IsZero() ||
		!token.AbsoluteExpiresAt.IsZero() ||
		!token.SessionCreatedAt.IsZero() ||
		!token.ReuseDetectedAt.IsZero() ||
		strings.TrimSpace(token.ReuseDetectedFromIP) != "" ||
		strings.TrimSpace(token.ReuseDetectedFromUA) != ""
}

func validateAgentRefreshGrantState(storedToken *storage.RefreshToken) error {
	if storedToken == nil {
		return auth.ErrInvalidToken
	}
	if storedToken.Revoked || !storedToken.RevokedAt.IsZero() {
		return auth.ErrInvalidToken
	}
	if refreshTokenCarriesRuntimeDecisionState(storedToken) && !storedToken.Current {
		return auth.ErrInvalidToken
	}
	return nil
}

func agentRefreshGrantLifetimes(now time.Time, cfg *config.Config, storedToken *storage.RefreshToken) (time.Duration, time.Time, error) {
	if err := validateAgentRefreshGrantState(storedToken); err != nil {
		return 0, time.Time{}, err
	}

	refreshExpiry := storedToken.ExpiresAt
	if refreshExpiry.IsZero() || !refreshExpiry.After(now) {
		return 0, time.Time{}, auth.ErrInvalidToken
	}

	accessTTL := auth.AgentAccessTokenTTL(cfg)
	if storedToken.AccessTTLSeconds > 0 {
		storedAccessTTL := time.Duration(storedToken.AccessTTLSeconds) * time.Second
		if storedAccessTTL > 0 && storedAccessTTL < accessTTL {
			accessTTL = storedAccessTTL
		}
	}

	for _, expiry := range []time.Time{storedToken.IdleExpiresAt, storedToken.AbsoluteExpiresAt} {
		if expiry.IsZero() {
			continue
		}
		remaining := expiry.Sub(now)
		if remaining <= 0 {
			return 0, time.Time{}, auth.ErrInvalidToken
		}
		if expiry.Before(refreshExpiry) {
			refreshExpiry = expiry
		}
		if remaining < accessTTL {
			accessTTL = remaining
		}
	}

	if remaining := refreshExpiry.Sub(now); remaining < accessTTL {
		accessTTL = remaining
	}
	if accessTTL <= 0 {
		return 0, time.Time{}, auth.ErrInvalidToken
	}

	return accessTTL, refreshExpiry, nil
}

func standardRefreshGrantLifetimes(now time.Time, cfg *config.Config, client *storage.OAuthClient, storedToken *storage.RefreshToken) (time.Duration, time.Time, string, error) {
	clientClass := refreshGrantClientClass(client, storedToken)
	if clientClass == auth.ClientClassAgent {
		accessTTL, refreshExpiry, err := agentRefreshGrantLifetimes(now, cfg, storedToken)
		return accessTTL, refreshExpiry, clientClass, err
	}

	return auth.AccessTokenDuration, now.Add(auth.RefreshTokenDuration), clientClass, nil
}

// exchangeRefreshToken exchanges a refresh token for new access and refresh tokens
func (h *Handler) exchangeRefreshToken(ctx context.Context, oauthSvc *auth.OAuthService, refreshToken, clientID, clientSecret, requestedResource, ipAddress, userAgent string) (string, string, []string, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	clientID = strings.TrimSpace(clientID)
	clientSecret = strings.TrimSpace(clientSecret)

	if auth.IsAgentRuntimeClientID(clientID) {
		return h.exchangeAgentRuntimeRefreshToken(ctx, oauthSvc, refreshToken, clientID, ipAddress, userAgent)
	}

	client, err := h.validateRefreshGrantClient(ctx, oauthSvc, refreshToken, clientID, clientSecret)
	if err != nil {
		return "", "", nil, err
	}

	// Get refresh token from storage
	storedToken, err := h.repos.Account().GetRefreshToken(ctx, refreshToken)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return "", "", nil, auth.ErrInvalidToken
		}
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, failedToValidateRefreshToken(), err)
	}

	now := time.Now().UTC()
	terminal := func(reason string) (string, string, []string, error) {
		h.revokeStandardRefreshFamily(ctx, storedToken, reason, ipAddress, userAgent)
		return "", "", nil, auth.ErrInvalidToken
	}

	// Validate refresh token belongs to the client before considering grace.
	if storedToken.ClientID != clientID {
		return terminal("refresh_cross_client_replay")
	}
	if storedToken.Revoked || !storedToken.RevokedAt.IsZero() {
		if standardRefreshRetryEligible(storedToken, now) {
			return h.redeemStandardRefreshRetry(ctx, oauthSvc, client, storedToken, requestedResource, ipAddress, userAgent, now)
		}
		// Same-client staleness, including a later presentation after a rescue,
		// is not evidence of credential theft. Only a CAS-losing concurrent
		// retry redemption below is treated as redeemed-reuse replay.
		return "", "", nil, auth.ErrInvalidToken
	}
	reason, authorityErr := h.standardRefreshAuthorityRevocationReason(ctx, client, storedToken, requestedResource, now)
	if authorityErr != nil {
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, authorityErr)
	}
	if reason != "" {
		return "", "", nil, auth.ErrInvalidToken
	}

	accessTTL, refreshExpiry, clientClass, err := standardRefreshGrantLifetimes(now, h.cfg, client, storedToken)
	if err != nil {
		return "", "", nil, auth.ErrInvalidToken
	}

	// The authorizing human must survive a refresh. For actor-scoped MCP tokens the
	// subject is the agent, so DelegatedBy has to be re-derived from the stored
	// principal (owner or grantee) rather than silently reverting to the owner, and
	// a non-owner principal's share grant is re-validated so revocation takes effect
	// on the next refresh.
	delegatedByOverride := strings.TrimSpace(storedToken.PrincipalUsername)
	if oauthResourceTargetsActorMCP(storedToken.Resource) {
		if delegatedByOverride == "" {
			// Legacy actor-scoped refresh token issued before the authorizing
			// principal was persisted. Fail closed: re-deriving DelegatedBy as the
			// owner would silently re-attribute a grantee's token to the owner.
			return "", "", nil, auth.ErrInvalidToken
		}
		authorized, authErr := h.agentRefreshGrantAuthorized(ctx, storedToken.Username, delegatedByOverride)
		if authErr != nil {
			return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, authErr)
		}
		if !authorized {
			return terminal("refresh_share_grant_revoked")
		}
	}

	accessToken, newRefreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContextAndAudienceAndDelegatedBy(ctx, storedToken.Username, clientID, "", storedToken.Scopes, accessTTL, clientClass, storedToken.SessionID, storedToken.Resource, delegatedByOverride)
	if err != nil {
		return "", "", nil, errors.Join(failedToGenerateNewTokens(), err)
	}

	// Create new refresh token record
	newOAuthRefreshToken := &storage.RefreshToken{
		Token:               newRefreshToken,
		Username:            storedToken.Username,
		PrincipalUsername:   delegatedByOverride,
		ClientID:            clientID,
		Resource:            storedToken.Resource,
		Scopes:              storedToken.Scopes,
		CreatedAt:           now,
		ExpiresAt:           refreshExpiry,
		ClientClass:         clientClass,
		SessionID:           storedToken.SessionID,
		LastAuthFailureCode: storedToken.LastAuthFailureCode,
		LastAuthFailureAt:   storedToken.LastAuthFailureAt,
		LastAuthFailureMsg:  storedToken.LastAuthFailureMsg,
		LastAuthSuccessAt:   now,
		FamilyID:            storedToken.FamilyID,
		Generation:          storedToken.Generation + 1,
		Current:             true,
		DeviceLabel:         storedToken.DeviceLabel,
		IdleExpiresAt:       storedToken.IdleExpiresAt,
		AbsoluteExpiresAt:   storedToken.AbsoluteExpiresAt,
		SessionCreatedAt:    storedToken.SessionCreatedAt,
	}
	if strings.TrimSpace(newOAuthRefreshToken.FamilyID) == "" || storedToken.Generation < 1 {
		newOAuthRefreshToken.FamilyID = common.GenerateSessionIDULID()
		newOAuthRefreshToken.Generation = 2
	}
	if clientClass == auth.ClientClassAgent && storedToken.AccessTTLSeconds > 0 {
		newOAuthRefreshToken.AccessTTLSeconds = storedToken.AccessTTLSeconds
	}
	if !storedToken.LastUsedAt.IsZero() {
		newOAuthRefreshToken.LastUsedAt = now
	}

	if err := h.repos.Account().RotateRefreshToken(ctx, storedToken, newOAuthRefreshToken, now); err != nil {
		h.logger.Warn("refresh token rotation transaction failed", zap.Error(err))
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, failedToStoreNewRefreshToken(), err)
	}

	return accessToken, newRefreshToken, storedToken.Scopes, nil
}

func (h *Handler) standardRefreshAuthorityRevocationReason(
	ctx context.Context,
	client *storage.OAuthClient,
	token *storage.RefreshToken,
	requestedResource string,
	now time.Time,
) (string, error) {
	// A refresh resource parameter is optional, but when supplied it must be the
	// exact resource bound at the original grant. A mismatch is authoritative.
	if requestedResource != "" && requestedResource != strings.TrimSpace(token.Resource) {
		return "refresh_resource_mismatch", nil
	}
	if strings.TrimSpace(token.FamilyID) != "" && !token.Current {
		return "refresh_stale_generation", nil
	}
	if strings.EqualFold(strings.TrimSpace(token.ClientClass), auth.ClientClassOperator) && !oauthResourceTargetsInstancePlane(token.Resource) {
		return "refresh_resource_mismatch", nil
	}
	if oauthResourceTargetsInstancePlane(token.Resource) {
		if err := h.validateOAuthInstanceRefreshTokenTarget(ctx, client, token); err != nil {
			if errors.Is(err, errOAuthInvalidTarget) {
				return "refresh_resource_authority_revoked", nil
			}
			return "", err
		}
	}
	if !token.ExpiresAt.After(now) {
		return "refresh_token_expired", nil
	}
	return "", nil
}

func standardRefreshRetryEligible(token *storage.RefreshToken, now time.Time) bool {
	if token == nil || (token.RevokedReason != "rotated" && token.RevokedReason != "retry_rescued") ||
		token.RevokedAt.IsZero() || !token.RetryRedeemedAt.IsZero() {
		return false
	}
	age := now.Sub(token.RevokedAt)
	return age >= 0 && age <= standardRefreshRetryGrace
}

func standardRefreshRetryReplacement(stale *storage.RefreshToken, family []storage.RefreshToken, now time.Time) (*storage.RefreshToken, bool) {
	if stale == nil {
		return nil, true
	}
	successorObserved := false
	for i := range family {
		candidate := &family[i]
		if candidate.FamilyID == stale.FamilyID && candidate.Generation > stale.Generation {
			successorObserved = true
		}
		if candidate.FamilyID != stale.FamilyID || candidate.Generation != stale.Generation+1 ||
			(strings.TrimSpace(stale.ReplacedByHash) != "" && storage.RefreshTokenReplacementHash(candidate.Token) != stale.ReplacedByHash) ||
			candidate.ClientID != stale.ClientID || candidate.Username != stale.Username ||
			candidate.PrincipalUsername != stale.PrincipalUsername || candidate.Resource != stale.Resource ||
			!slices.Equal(candidate.Scopes, stale.Scopes) || candidate.Revoked || !candidate.Current ||
			!candidate.ExpiresAt.After(now) {
			continue
		}
		return candidate, true
	}
	return nil, successorObserved
}

func (h *Handler) validateStandardRefreshRetryTarget(
	ctx context.Context,
	client *storage.OAuthClient,
	active *storage.RefreshToken,
	requestedResource string,
) error {
	if requestedResource != "" && requestedResource != strings.TrimSpace(active.Resource) {
		return auth.ErrInvalidToken
	}
	if strings.EqualFold(strings.TrimSpace(active.ClientClass), auth.ClientClassOperator) && !oauthResourceTargetsInstancePlane(active.Resource) {
		return auth.ErrInvalidToken
	}
	if !oauthResourceTargetsInstancePlane(active.Resource) {
		return nil
	}
	if err := h.validateOAuthInstanceRefreshTokenTarget(ctx, client, active); err != nil {
		if errors.Is(err, errOAuthInvalidTarget) {
			return auth.ErrInvalidToken
		}
		return errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	return nil
}

func (h *Handler) redeemStandardRefreshRetry(
	ctx context.Context,
	oauthSvc *auth.OAuthService,
	client *storage.OAuthClient,
	stale *storage.RefreshToken,
	requestedResource, ipAddress, userAgent string,
	now time.Time,
) (string, string, []string, error) {
	family, err := h.repos.Account().ListRefreshTokensByFamily(ctx, stale.FamilyID)
	if err != nil {
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}
	active, authoritative := standardRefreshRetryReplacement(stale, family, now)
	if active == nil {
		if authoritative {
			return "", "", nil, auth.ErrInvalidToken
		}
		// The family index is eventually consistent. During the bounded rescue
		// window, absence is not authoritative evidence that rotation failed.
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, errors.New("refresh successor is not yet visible"))
	}
	if err := h.validateStandardRefreshRetryTarget(ctx, client, active, requestedResource); err != nil {
		return "", "", nil, err
	}

	accessTTL, refreshExpiry, clientClass, err := standardRefreshGrantLifetimes(now, h.cfg, client, active)
	if err != nil {
		return "", "", nil, auth.ErrInvalidToken
	}

	delegatedByOverride := strings.TrimSpace(active.PrincipalUsername)
	if oauthResourceTargetsActorMCP(active.Resource) {
		if delegatedByOverride == "" {
			return "", "", nil, auth.ErrInvalidToken
		}
		authorized, authErr := h.agentRefreshGrantAuthorized(ctx, active.Username, delegatedByOverride)
		if authErr != nil {
			return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, authErr)
		}
		if !authorized {
			return "", "", nil, auth.ErrInvalidToken
		}
	}

	accessToken, newRefreshToken, err := oauthSvc.GenerateTokensWithAccessTokenTTLAndClientContextAndAudienceAndDelegatedBy(
		ctx, active.Username, active.ClientID, "", active.Scopes, accessTTL, clientClass,
		active.SessionID, active.Resource, delegatedByOverride,
	)
	if err != nil {
		return "", "", nil, errors.Join(failedToGenerateNewTokens(), err)
	}

	next := &storage.RefreshToken{
		Token:               newRefreshToken,
		Username:            active.Username,
		PrincipalUsername:   delegatedByOverride,
		ClientID:            active.ClientID,
		Resource:            active.Resource,
		Scopes:              active.Scopes,
		CreatedAt:           now,
		ExpiresAt:           refreshExpiry,
		ClientClass:         clientClass,
		SessionID:           active.SessionID,
		FamilyID:            active.FamilyID,
		Generation:          active.Generation + 1,
		Current:             true,
		AccessTTLSeconds:    active.AccessTTLSeconds,
		DeviceLabel:         active.DeviceLabel,
		IdleExpiresAt:       active.IdleExpiresAt,
		AbsoluteExpiresAt:   active.AbsoluteExpiresAt,
		SessionCreatedAt:    active.SessionCreatedAt,
		LastAuthFailureCode: active.LastAuthFailureCode,
		LastAuthFailureAt:   active.LastAuthFailureAt,
		LastAuthFailureMsg:  active.LastAuthFailureMsg,
		LastAuthSuccessAt:   now,
	}
	if !active.LastUsedAt.IsZero() {
		next.LastUsedAt = now
	}
	if err := h.repos.Account().RedeemRefreshTokenRetry(ctx, stale, active, next, now); err != nil {
		latest, readErr := h.repos.Account().GetRefreshToken(ctx, stale.Token)
		if readErr == nil && latest != nil && !latest.RetryRedeemedAt.IsZero() {
			h.revokeStandardRefreshFamily(ctx, latest, "refresh_retry_replayed", ipAddress, userAgent)
			return "", "", nil, auth.ErrInvalidToken
		}
		if readErr != nil && !errors.Is(readErr, storage.ErrNotFound) {
			return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, readErr)
		}
		return "", "", nil, errors.Join(auth.ErrOAuthTemporarilyUnavailable, err)
	}

	return accessToken, newRefreshToken, active.Scopes, nil
}

func (h *Handler) revokeStandardRefreshFamily(ctx context.Context, token *storage.RefreshToken, reason, ipAddress, userAgent string) {
	if token == nil || strings.TrimSpace(token.FamilyID) == "" {
		return
	}
	if err := auth.RevokeAgentRuntimeFamily(ctx, h.repos, token, reason, ipAddress, userAgent); err != nil {
		h.logger.Error("failed to revoke standard refresh token family after reconciliation", zap.Error(err))
	}
}
