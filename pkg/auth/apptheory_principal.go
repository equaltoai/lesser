package auth

import (
	"strings"

	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/logging"
	"github.com/golang-jwt/jwt/v5"
	apptheory "github.com/theory-cloud/apptheory/runtime"
	"go.uber.org/zap"
)

const (
	principalClaimsRawKey           = "lesser.raw_claims"
	principalResolutionAttemptedKey = "lesser.auth_resolution_attempted"
)

type principalClaimsValidator func(string) (*Claims, error)

type principalTokenFamily string

const (
	principalTokenFamilyUnknown principalTokenFamily = "unknown"
	principalTokenFamilySession principalTokenFamily = "session"
	principalTokenFamilyOAuth   principalTokenFamily = "oauth"
)

type appTheoryPrincipalResolver struct {
	validate    principalClaimsValidator
	logger      *zap.Logger
	serviceName string
}

func newAppTheoryPrincipalResolver(validate principalClaimsValidator, logger *zap.Logger, serviceName string) *appTheoryPrincipalResolver {
	return &appTheoryPrincipalResolver{
		validate:    validate,
		logger:      logger,
		serviceName: strings.TrimSpace(serviceName),
	}
}

// NewAppTheoryPrincipalHookFromAuthService builds an AppTheory principal hook from AuthService validation.
func NewAppTheoryPrincipalHookFromAuthService(authService *AuthService, logger *zap.Logger, serviceName string) apptheory.PrincipalAuthHook {
	if authService == nil {
		return nil
	}

	resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
		return authService.ValidateAccessToken(token)
	}, logger, serviceName)

	return resolver.Resolve
}

// NewAppTheoryPrincipalHookFromOAuthService builds an AppTheory principal hook from OAuthService validation.
func NewAppTheoryPrincipalHookFromOAuthService(oauthService *OAuthService, logger *zap.Logger, serviceName string) apptheory.PrincipalAuthHook {
	if oauthService == nil {
		return nil
	}

	resolver := newAppTheoryPrincipalResolver(oauthService.ValidateAccessToken, logger, serviceName)
	return resolver.Resolve
}

// NewAppTheoryPrincipalHookFromAuthAndOAuthServices builds an AppTheory principal hook that preserves
// native session validation while also accepting OAuth-issued access tokens on the same API surface.
func NewAppTheoryPrincipalHookFromAuthAndOAuthServices(authService *AuthService, oauthService *OAuthService, logger *zap.Logger, serviceName string) apptheory.PrincipalAuthHook {
	resolver := newCompositeAppTheoryPrincipalResolver(authService, oauthService, logger, serviceName)
	if resolver == nil {
		return nil
	}

	return resolver.Resolve
}

// CreatePrincipalContextBridgeFromAuthService mirrors AuthService-backed principal data into the legacy request context.
func CreatePrincipalContextBridgeFromAuthService(authService *AuthService, logger *zap.Logger, serviceName string) apptheory.Middleware {
	if authService == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	resolver := newAppTheoryPrincipalResolver(func(token string) (*Claims, error) {
		return authService.ValidateAccessToken(token)
	}, logger, serviceName)

	return createPrincipalContextBridge(resolver)
}

// CreatePrincipalContextBridgeFromOAuthService mirrors OAuthService-backed principal data into the legacy request context.
func CreatePrincipalContextBridgeFromOAuthService(oauthService *OAuthService, logger *zap.Logger, serviceName string) apptheory.Middleware {
	if oauthService == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	resolver := newAppTheoryPrincipalResolver(oauthService.ValidateAccessToken, logger, serviceName)
	return createPrincipalContextBridge(resolver)
}

// CreatePrincipalContextBridgeFromAuthAndOAuthServices mirrors principal data for both native
// session-backed tokens and OAuth-issued tokens into the legacy request context.
func CreatePrincipalContextBridgeFromAuthAndOAuthServices(authService *AuthService, oauthService *OAuthService, logger *zap.Logger, serviceName string) apptheory.Middleware {
	resolver := newCompositeAppTheoryPrincipalResolver(authService, oauthService, logger, serviceName)
	if resolver == nil {
		return func(next apptheory.Handler) apptheory.Handler { return next }
	}

	return createPrincipalContextBridge(resolver)
}

func newCompositeAppTheoryPrincipalResolver(authService *AuthService, oauthService *OAuthService, logger *zap.Logger, serviceName string) *appTheoryPrincipalResolver {
	validate := newCompositePrincipalClaimsValidator(authService, oauthService)
	if validate == nil {
		return nil
	}

	return newAppTheoryPrincipalResolver(validate, logger, serviceName)
}

func newCompositePrincipalClaimsValidator(authService *AuthService, oauthService *OAuthService) principalClaimsValidator {
	if authService == nil && oauthService == nil {
		return nil
	}

	return func(token string) (*Claims, error) {
		switch inferPrincipalTokenFamily(token) {
		case principalTokenFamilyOAuth:
			if oauthService == nil {
				return nil, ErrInvalidToken
			}
			return oauthService.ValidateAccessToken(token)
		case principalTokenFamilySession:
			if authService == nil {
				return nil, ErrInvalidToken
			}
			return authService.ValidateAccessToken(token)
		default:
			return nil, ErrInvalidToken
		}
	}
}

func inferPrincipalTokenFamily(token string) principalTokenFamily {
	token = strings.TrimSpace(token)
	if token == "" {
		return principalTokenFamilyUnknown
	}

	claims := &Claims{}
	parser := jwt.NewParser()
	if _, _, err := parser.ParseUnverified(token, claims); err != nil {
		return principalTokenFamilyUnknown
	}

	// API route auth deliberately distinguishes Lesser's two current bearer families:
	// OAuthService-issued access tokens always carry a JWT ID (`jti`) because revocation is keyed by JTI,
	// while AuthService's short-lived native session tokens intentionally omit `jti` and remain anchored
	// to backing session rows. If either family changes, revisit this classifier and the route-matrix tests.
	if strings.TrimSpace(claims.ID) != "" {
		return principalTokenFamilyOAuth
	}

	return principalTokenFamilySession
}

func createPrincipalContextBridge(resolver *appTheoryPrincipalResolver) apptheory.Middleware {
	return func(next apptheory.Handler) apptheory.Handler {
		return func(ctx *apptheory.Context) (*apptheory.Response, error) {
			if ctx == nil {
				return next(ctx)
			}

			if ctx.AuthPrincipal == nil {
				if claims := ClaimsFromAppTheoryContext(ctx); claims != nil {
					setPrincipalContext(ctx, PrincipalFromClaims(claims))
				}
			}

			if ctx.AuthPrincipal == nil && !principalResolutionAttempted(ctx) && resolver != nil {
				principal, err := resolver.Resolve(ctx)
				if err != nil {
					return nil, err
				}
				setPrincipalContext(ctx, principal)
			}

			populateLegacyAuthContext(ctx)
			return next(ctx)
		}
	}
}

func (r *appTheoryPrincipalResolver) Resolve(ctx *apptheory.Context) (*apptheory.AuthPrincipal, error) {
	markPrincipalResolutionAttempted(ctx)
	if ctx == nil || r == nil || r.validate == nil {
		return nil, nil
	}

	authHeader := common.ExtractAuthHeader(ctx)
	if authHeader == "" {
		return nil, nil
	}

	token, err := ExtractBearerToken(authHeader)
	if err != nil || strings.TrimSpace(token) == "" {
		r.logDebug(ctx, "principal authentication skipped - invalid bearer token format", zap.Error(err))
		return nil, nil
	}

	claims, err := r.validate(token)
	if err != nil || claims == nil {
		r.logWarn(ctx, "principal authentication failed - header present but validation failed", zap.Error(err))
		return nil, nil
	}

	return PrincipalFromClaims(claims), nil
}

func (r *appTheoryPrincipalResolver) logDebug(ctx *apptheory.Context, message string, fields ...zap.Field) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Debug(message, r.logFields(ctx, fields...)...)
}

func (r *appTheoryPrincipalResolver) logWarn(ctx *apptheory.Context, message string, fields ...zap.Field) {
	if r == nil || r.logger == nil {
		return
	}
	r.logger.Warn(message, r.logFields(ctx, fields...)...)
}

func (r *appTheoryPrincipalResolver) logFields(ctx *apptheory.Context, fields ...zap.Field) []zap.Field {
	out := make([]zap.Field, 0, len(fields)+3)
	if serviceName := strings.TrimSpace(r.serviceName); serviceName != "" {
		out = append(out, zap.String("service", serviceName))
	}
	if ctx != nil {
		out = append(out, zap.String("path", logging.SanitizeLogPath(ctx.Request.Path)))
		out = append(out, zap.Bool("has_auth_header", common.ExtractAuthHeader(ctx) != ""))
	}
	out = append(out, fields...)
	return out
}

// PrincipalFromClaims converts Lesser OAuth claims into an AppTheory auth principal.
func PrincipalFromClaims(claims *Claims) *apptheory.AuthPrincipal {
	if claims == nil {
		return nil
	}

	return &apptheory.AuthPrincipal{
		Identity: principalIdentityFromClaims(claims),
		Scopes:   append([]string(nil), claims.Scopes...),
		Claims: map[string]any{
			principalClaimsRawKey: claims,
			"username":            claims.GetUsername(),
			"client_id":           claims.ClientID,
			"client_class":        claims.ClientClass,
			"session_id":          claims.SessionID,
			"agent_session_id":    claims.AgentSessionID,
			"is_agent":            claims.IsAgent,
			"agent_type":          claims.AgentType,
			"delegated_by":        claims.DelegatedBy,
			"subject":             strings.TrimSpace(claims.Subject),
		},
	}
}

// ClaimsFromAppTheoryContext returns Lesser OAuth claims from either AppTheory principal state or legacy context keys.
func ClaimsFromAppTheoryContext(ctx *apptheory.Context) *Claims {
	if ctx == nil {
		return nil
	}

	if ctx.AuthPrincipal == nil {
		return claimsFromLegacyValue(ctx.Get("claims"))
	}

	if raw, ok := ctx.AuthPrincipal.Claims[principalClaimsRawKey].(*Claims); ok && raw != nil {
		return raw
	}

	if claims := claimsFromPrincipal(ctx.AuthPrincipal); claims != nil {
		return claims
	}

	return claimsFromLegacyValue(ctx.Get("claims"))
}

// UsernameFromAppTheoryContext returns the authenticated username from AppTheory principal state or legacy context keys.
func UsernameFromAppTheoryContext(ctx *apptheory.Context) string {
	if claims := ClaimsFromAppTheoryContext(ctx); claims != nil {
		return claims.GetUsername()
	}

	if ctx == nil {
		return ""
	}

	if username, ok := ctx.Get("username").(string); ok && strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}

	if ctx.AuthPrincipal == nil {
		return ""
	}

	if username, ok := ctx.AuthPrincipal.Claims["username"].(string); ok && strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}

	return strings.TrimSpace(ctx.AuthIdentity)
}

// LegacyAuthContextFromAppTheoryContext synthesizes the legacy common.AuthContext from AppTheory principal state.
func LegacyAuthContextFromAppTheoryContext(ctx *apptheory.Context) *common.AuthContext {
	username := UsernameFromAppTheoryContext(ctx)
	claims := ClaimsFromAppTheoryContext(ctx)
	if username == "" && claims == nil {
		return &common.AuthContext{}
	}

	return &common.AuthContext{
		Username: username,
		Claims:   claims,
	}
}

// IsAppTheoryContextAuthenticated reports whether the request has authenticated AppTheory principal state.
func IsAppTheoryContextAuthenticated(ctx *apptheory.Context) bool {
	if ctx == nil {
		return false
	}

	if ctx.AuthPrincipal != nil && strings.TrimSpace(ctx.AuthIdentity) != "" {
		return true
	}

	if authenticated, ok := ctx.Get("is_authenticated").(bool); ok {
		return authenticated
	}

	return UsernameFromAppTheoryContext(ctx) != ""
}

func populateLegacyAuthContext(ctx *apptheory.Context) {
	if ctx == nil {
		return
	}

	if ctx.AuthPrincipal == nil {
		ctx.Set("is_authenticated", false)
		return
	}

	if strings.TrimSpace(ctx.AuthIdentity) == "" {
		ctx.AuthIdentity = strings.TrimSpace(ctx.AuthPrincipal.Identity)
	}

	claims := ClaimsFromAppTheoryContext(ctx)
	if claims == nil {
		claims = claimsFromPrincipal(ctx.AuthPrincipal)
	}

	username := UsernameFromAppTheoryContext(ctx)
	if username == "" {
		username = strings.TrimSpace(ctx.AuthIdentity)
	}

	if claims != nil {
		ctx.Set("claims", claims)
	}
	ctx.Set("auth_context", &common.AuthContext{
		Username: username,
		Claims:   claims,
	})
	ctx.Set("username", username)
	ctx.Set("is_authenticated", username != "")
}

func principalIdentityFromClaims(claims *Claims) string {
	if claims == nil {
		return ""
	}

	if username := claims.GetUsername(); username != "" {
		return username
	}

	if subject := strings.TrimSpace(claims.Subject); subject != "" {
		return subject
	}

	return strings.TrimSpace(claims.ClientID)
}

func claimsFromLegacyValue(value any) *Claims {
	switch typed := value.(type) {
	case *Claims:
		return typed
	default:
		return nil
	}
}

func claimsFromPrincipal(principal *apptheory.AuthPrincipal) *Claims {
	if principal == nil {
		return nil
	}

	claims := &Claims{
		Username: identityFromPrincipal(principal),
		Scopes:   append([]string(nil), principal.Scopes...),
	}

	if clientID, ok := principal.Claims["client_id"].(string); ok {
		claims.ClientID = clientID
	}
	if clientClass, ok := principal.Claims["client_class"].(string); ok {
		claims.ClientClass = clientClass
	}
	if sessionID, ok := principal.Claims["session_id"].(string); ok {
		claims.SessionID = sessionID
	}
	if agentSessionID, ok := principal.Claims["agent_session_id"].(string); ok {
		claims.AgentSessionID = agentSessionID
	}
	if isAgent, ok := principal.Claims["is_agent"].(bool); ok {
		claims.IsAgent = isAgent
	}
	if agentType, ok := principal.Claims["agent_type"].(string); ok {
		claims.AgentType = agentType
	}
	if delegatedBy, ok := principal.Claims["delegated_by"].(string); ok {
		claims.DelegatedBy = delegatedBy
	}
	if subject, ok := principal.Claims["subject"].(string); ok {
		claims.Subject = subject
	}

	return claims
}

func identityFromPrincipal(principal *apptheory.AuthPrincipal) string {
	if principal == nil {
		return ""
	}

	if username, ok := principal.Claims["username"].(string); ok && strings.TrimSpace(username) != "" {
		return strings.TrimSpace(username)
	}

	return strings.TrimSpace(principal.Identity)
}

func setPrincipalContext(ctx *apptheory.Context, principal *apptheory.AuthPrincipal) {
	if ctx == nil || principal == nil {
		return
	}

	ctx.AuthPrincipal = principal
	ctx.AuthIdentity = strings.TrimSpace(principal.Identity)
}

func markPrincipalResolutionAttempted(ctx *apptheory.Context) {
	if ctx == nil {
		return
	}
	ctx.Set(principalResolutionAttemptedKey, true)
}

func principalResolutionAttempted(ctx *apptheory.Context) bool {
	if ctx == nil {
		return false
	}
	attempted, _ := ctx.Get(principalResolutionAttemptedKey).(bool)
	return attempted
}
