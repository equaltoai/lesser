package handlers

import (
	"net/http"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	frameworkoauth "github.com/theory-cloud/apptheory/v3/runtime/oauth"
)

// HandleOAuthAuthorizationServerMetadataLift serves RFC 8414 authorization server metadata.
func (h *Handler) HandleOAuthAuthorizationServerMetadataLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	baseURL := h.cfg.BaseURL()
	deviceFlowEnabled := oauthDeviceFlowEnabled(h.cfg)
	metadataOptions := []frameworkoauth.AuthorizationServerMetadataOption{
		frameworkoauth.WithRevocationEndpoint(baseURL + "/oauth/revoke"),
	}
	if deviceFlowEnabled {
		metadataOptions = append(metadataOptions,
			frameworkoauth.WithDeviceAuthorizationEndpoint(baseURL+"/oauth/device/code"))
	}
	metadata, err := frameworkoauth.NewAuthorizationServerMetadata(baseURL, metadataOptions...)
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{
			"error": "server_error",
		})
	}

	// AppTheory owns the RFC 8414 wire shape and conventional endpoint layout.
	// Lesser supplies its scope catalog and actual token authentication methods;
	// the legacy /oauth/* routes remain additive compatibility aliases.
	metadata.ScopesSupported = auth.CanonicalOAuthScopes()
	metadata.GrantTypesSupported = []string{auth.GrantTypeAuthorizationCode, auth.GrantTypeRefreshToken}
	if deviceFlowEnabled {
		metadata.GrantTypesSupported = append(metadata.GrantTypesSupported, oauthDeviceCodeGrantType)
	}
	metadata.TokenEndpointAuthMethodsSupported = []string{"client_secret_basic", "client_secret_post", "none"}
	// Lesser access tokens are currently symmetric JWTs, so there is no public
	// signing-key set to advertise. JWKSURI is optional in RFC 8414.
	metadata.JWKSURI = ""

	resp, err := frameworkoauth.AuthorizationServerMetadataHandler(metadata)(ctx)
	if err != nil {
		return nil, err
	}
	setHeader(resp, "Cache-Control", "public, max-age=300")
	return resp, nil
}

// HandleOAuthProtectedResourceMetadataLift serves the actor-scoped MCP RFC
// 9728 document using AppTheory's wire primitive.
func (h *Handler) HandleOAuthProtectedResourceMetadataLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	username := ctx.Param("username")
	if err := common.ValidateUsernameParamID(username); err != nil {
		return common.RespondValidationError(ctx, err)
	}
	bundle := auth.BuildPublicMCPAccessBundle(h.cfg.BaseURL(), username)
	metadata, err := frameworkoauth.NewProtectedResourceMetadata(bundle.MCPURL, []string{h.cfg.BaseURL()})
	if err != nil {
		return apptheory.JSON(http.StatusInternalServerError, map[string]string{"error": "server_error"})
	}
	metadata.ScopesSupported = auth.CanonicalOAuthScopes()
	metadata.BearerMethodsSupported = []string{"header"}
	return frameworkoauth.ProtectedResourceMetadataHandler(metadata)(ctx)
}

func oauthDeviceFlowEnabled(cfg *config.Config) bool {
	return cfg != nil && cfg.AllowDeviceFlow
}
