package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"strings"

	"github.com/equaltoai/lesser/cmd/api/models"
	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/storage"
	apptheory "github.com/theory-cloud/apptheory/v3/runtime"
	"go.uber.org/zap"
)

const (
	oauthRegistrationSourceDynamic = "dynamic"
	oauthRegistrationSourceManual  = "manual"
)

type dynamicRegistrationNormalizedRequest struct {
	clientName              string
	redirectURIs            []string
	scopes                  []string
	grantTypes              []string
	responseTypes           []string
	tokenEndpointAuthMethod string
	clientClass             string
	clientURI               string
	logoURI                 string
	contacts                []string
	tosURI                  string
	policyURI               string
	softwareID              string
	softwareVersion         string
	confidential            bool
	ownerID                 string
}

// HandleOAuthDynamicClientRegistrationLift serves RFC 7591 dynamic client registration.
func (h *Handler) HandleOAuthDynamicClientRegistrationLift(ctx *apptheory.Context) (*apptheory.Response, error) {
	req, resp, err := h.parseOAuthDynamicClientRegistrationRequest(ctx)
	if resp != nil || err != nil {
		return resp, err
	}

	normalized, resp, err := h.normalizeDynamicClientRegistration(ctx, req)
	if resp != nil || err != nil {
		return resp, err
	}

	client := &storage.OAuthClient{
		Name:               normalized.clientName,
		Website:            normalized.clientURI,
		ClientURI:          normalized.clientURI,
		LogoURI:            normalized.logoURI,
		Contacts:           normalized.contacts,
		TosURI:             normalized.tosURI,
		PolicyURI:          normalized.policyURI,
		SoftwareID:         normalized.softwareID,
		SoftwareVersion:    normalized.softwareVersion,
		RedirectURIs:       normalized.redirectURIs,
		GrantTypes:         normalized.grantTypes,
		ResponseTypes:      normalized.responseTypes,
		Scopes:             normalized.scopes,
		ClientClass:        normalized.clientClass,
		OwnerID:            normalized.ownerID,
		RegistrationSource: oauthRegistrationSourceDynamic,
		Confidential:       normalized.confidential,
	}

	if err := h.repos.Account().CreateOAuthClient(ctx.Context(), client); err != nil {
		h.logger.Error("failed to create dynamically registered OAuth client", zap.Error(err))
		return h.oauthDynamicRegistrationError(http.StatusInternalServerError, "server_error", "failed to register oauth client")
	}

	respBody := models.OAuthDynamicClientRegistrationResponse{
		ClientID:                client.ClientID,
		ClientIDIssuedAt:        client.CreatedAt.UTC().Unix(),
		ClientSecretExpiresAt:   0,
		ClientName:              client.Name,
		RedirectURIs:            append([]string(nil), client.RedirectURIs...),
		GrantTypes:              append([]string(nil), client.GrantTypes...),
		ResponseTypes:           append([]string(nil), client.ResponseTypes...),
		TokenEndpointAuthMethod: normalized.tokenEndpointAuthMethod,
		Scope:                   strings.Join(client.Scopes, " "),
		ClientURI:               client.ClientURI,
		LogoURI:                 client.LogoURI,
		Contacts:                append([]string(nil), client.Contacts...),
		TosURI:                  client.TosURI,
		PolicyURI:               client.PolicyURI,
		SoftwareID:              client.SoftwareID,
		SoftwareVersion:         client.SoftwareVersion,
		ClientClass:             client.ClientClass,
		RegistrationSource:      client.RegistrationSource,
	}
	if normalized.confidential {
		respBody.ClientSecret = client.ClientSecret
	}

	return apptheory.JSON(http.StatusCreated, respBody)
}

func (h *Handler) parseOAuthDynamicClientRegistrationRequest(ctx *apptheory.Context) (*models.OAuthDynamicClientRegistrationRequest, *apptheory.Response, error) {
	contentType := strings.ToLower(strings.TrimSpace(headerValue(ctx, "Content-Type")))
	if contentType == "" {
		contentType = strings.ToLower(strings.TrimSpace(headerValue(ctx, "content-type")))
	}
	if !strings.Contains(contentType, "application/json") {
		resp, err := h.oauthDynamicRegistrationError(http.StatusUnsupportedMediaType, "invalid_client_metadata", "dynamic registration requires application/json")
		return nil, resp, err
	}
	if err := common.ValidateSliceNotEmpty("request_body", ctx.Request.Body); err != nil {
		resp, err := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "registration request body is required")
		return nil, resp, err
	}

	var req models.OAuthDynamicClientRegistrationRequest
	decoder := json.NewDecoder(bytes.NewReader(ctx.Request.Body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "unsupported or malformed client metadata")
		return nil, resp, respErr
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "unsupported or malformed client metadata")
		return nil, resp, respErr
	}

	return &req, nil, nil
}

func (h *Handler) normalizeDynamicClientRegistration(ctx *apptheory.Context, req *models.OAuthDynamicClientRegistrationRequest) (*dynamicRegistrationNormalizedRequest, *apptheory.Response, error) {
	if req == nil {
		resp, err := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "registration request body is required")
		return nil, resp, err
	}

	clientName := strings.TrimSpace(req.ClientName)
	if err := common.ValidateRequiredParam("client_name", clientName); err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "client_name is required")
		return nil, resp, respErr
	}
	if err := common.ValidateApplicationName(clientName); err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "client_name is invalid")
		return nil, resp, respErr
	}

	redirectURIs, err := normalizeDynamicRegistrationRedirectURIs(req.RedirectURIs)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_redirect_uri", err.Error())
		return nil, resp, respErr
	}

	clientURI := strings.TrimSpace(req.ClientURI)
	if clientURI != "" {
		if err := validateDynamicRegistrationClientURI(clientURI); err != nil {
			resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return nil, resp, respErr
		}
	}

	logoURI := strings.TrimSpace(req.LogoURI)
	if logoURI != "" {
		if err := validateDynamicRegistrationMetadataURI("logo_uri", logoURI); err != nil {
			resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return nil, resp, respErr
		}
	}

	tosURI := strings.TrimSpace(req.TosURI)
	if tosURI != "" {
		if err := validateDynamicRegistrationMetadataURI("tos_uri", tosURI); err != nil {
			resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return nil, resp, respErr
		}
	}

	policyURI := strings.TrimSpace(req.PolicyURI)
	if policyURI != "" {
		if err := validateDynamicRegistrationMetadataURI("policy_uri", policyURI); err != nil {
			resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
			return nil, resp, respErr
		}
	}

	contacts, err := normalizeDynamicRegistrationContacts(req.Contacts)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	tokenEndpointAuthMethod, _, err := normalizeOAuthTokenEndpointAuthMethod(req.TokenEndpointAuthMethod, req.ClientClass)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	clientClass, err := normalizeDynamicRegistrationClientClass(req.ClientClass, tokenEndpointAuthMethod)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	tokenEndpointAuthMethod, confidential, err := normalizeOAuthTokenEndpointAuthMethod(req.TokenEndpointAuthMethod, clientClass)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	scopes := h.parseScopes(req.Scope)
	if err := auth.ValidatePublicOAuthScopes(scopes); err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", "requested scopes are invalid")
		return nil, resp, respErr
	}

	grantTypes, err := normalizeDynamicRegistrationGrantTypes(req.GrantTypes, clientClass, tokenEndpointAuthMethod, h.cfg != nil && h.cfg.AllowDeviceFlow)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	responseTypes, err := normalizeDynamicRegistrationResponseTypes(req.ResponseTypes)
	if err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	if err := validateDynamicRegistrationApplicationType(req.ApplicationType); err != nil {
		resp, respErr := h.oauthDynamicRegistrationError(http.StatusBadRequest, "invalid_client_metadata", err.Error())
		return nil, resp, respErr
	}

	ownerID := h.getOptionalAuthenticatedUser(ctx)

	return &dynamicRegistrationNormalizedRequest{
		clientName:              clientName,
		redirectURIs:            redirectURIs,
		scopes:                  scopes,
		grantTypes:              grantTypes,
		responseTypes:           responseTypes,
		tokenEndpointAuthMethod: tokenEndpointAuthMethod,
		clientClass:             clientClass,
		clientURI:               clientURI,
		logoURI:                 logoURI,
		contacts:                contacts,
		tosURI:                  tosURI,
		policyURI:               policyURI,
		softwareID:              strings.TrimSpace(req.SoftwareID),
		softwareVersion:         strings.TrimSpace(req.SoftwareVersion),
		confidential:            confidential,
		ownerID:                 ownerID,
	}, nil, nil
}

func normalizeDynamicRegistrationRedirectURIs(redirectURIs []string) ([]string, error) {
	if len(redirectURIs) == 0 {
		return nil, errors.New("redirect_uris must contain at least one redirect URI")
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(redirectURIs))
	for _, redirectURI := range redirectURIs {
		redirectURI = strings.TrimSpace(redirectURI)
		if err := validateDynamicRegistrationRedirectURI(redirectURI); err != nil {
			return nil, err
		}
		if _, ok := seen[redirectURI]; ok {
			continue
		}
		seen[redirectURI] = struct{}{}
		out = append(out, redirectURI)
	}

	return out, nil
}

func validateDynamicRegistrationRedirectURI(redirectURI string) error {
	if err := common.ValidateRequiredParam("redirect_uri", redirectURI); err != nil {
		return errors.New("redirect_uris contain an empty value")
	}
	if redirectURI == "urn:ietf:wg:oauth:2.0:oob" {
		return errors.New("urn:ietf:wg:oauth:2.0:oob is not allowed for dynamic registration")
	}

	parsed, err := url.Parse(redirectURI)
	if err != nil || parsed == nil || parsed.Scheme == "" {
		return errors.New("redirect_uris contain an invalid absolute URI")
	}

	switch strings.ToLower(parsed.Scheme) {
	case schemeHTTPS:
		if parsed.Host == "" {
			return errors.New("https redirect_uris must include a host")
		}
		return nil
	case schemeHTTP:
		host := parsed.Hostname()
		if !isLoopbackRedirectHost(host) {
			return errors.New("http redirect_uris must use a loopback host")
		}
		return nil
	default:
		// Native-app custom schemes are allowed as long as the URI is absolute.
		return nil
	}
}

func isLoopbackRedirectHost(host string) bool {
	host = strings.TrimSpace(host)
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateDynamicRegistrationClientURI(clientURI string) error {
	parsed, err := url.Parse(strings.TrimSpace(clientURI))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("client_uri must be an absolute http or https URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return errors.New("client_uri must use http or https")
	}
}

func validateDynamicRegistrationMetadataURI(fieldName, value string) error {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed == nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New(fieldName + " must be an absolute http or https URL")
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
		return nil
	default:
		return errors.New(fieldName + " must use http or https")
	}
}

func normalizeDynamicRegistrationContacts(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(requested))
	for _, contact := range requested {
		contact = strings.TrimSpace(contact)
		if contact == "" {
			continue
		}
		parsed, err := mail.ParseAddress(contact)
		if err != nil || parsed.Address == "" {
			return nil, errors.New("contacts must contain valid email addresses")
		}
		normalized := strings.ToLower(strings.TrimSpace(parsed.Address))
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out, nil
}

func normalizeDynamicRegistrationClientClass(value, tokenEndpointAuthMethod string) (string, error) {
	if strings.TrimSpace(value) == "" {
		if strings.EqualFold(strings.TrimSpace(tokenEndpointAuthMethod), oauthTokenEndpointAuthMethodNone) {
			return auth.ClientClassCLI, nil
		}
		return auth.ClientClassWeb, nil
	}
	return normalizePublicOAuthClientClass(value)
}

func normalizeDynamicRegistrationGrantTypes(requested []string, clientClass, tokenEndpointAuthMethod string, allowDeviceFlow bool) ([]string, error) {
	defaultGrants := dynamicRegistrationDefaultGrantTypes(clientClass, tokenEndpointAuthMethod, allowDeviceFlow)
	if len(requested) == 0 {
		return defaultGrants, nil
	}

	requestedValue := strings.Join(requested, " ")
	normalized, err := normalizeOAuthClientGrantTypes(requestedValue, clientClass, allowDeviceFlow)
	if err != nil {
		return nil, err
	}
	if !grantTypeSubsetAllowed(defaultGrants, normalized) {
		return nil, errors.New("grant_types exceed Lesser's dynamic registration policy")
	}
	return normalized, nil
}

func grantTypeSubsetAllowed(allowed, requested []string) bool {
	if len(requested) == 0 {
		return true
	}

	allowedSet := make(map[string]struct{}, len(allowed))
	for _, item := range allowed {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		allowedSet[item] = struct{}{}
	}

	for _, item := range requested {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" {
			continue
		}
		if _, ok := allowedSet[item]; !ok {
			return false
		}
	}

	return true
}

func dynamicRegistrationDefaultGrantTypes(clientClass, _ string, allowDeviceFlow bool) []string {
	switch clientClass {
	case auth.ClientClassCLI:
		grants := []string{
			auth.GrantTypeAuthorizationCode,
			auth.GrantTypeRefreshToken,
		}
		if allowDeviceFlow {
			grants = append(grants, oauthDeviceCodeGrantType)
		}
		return grants
	default:
		return []string{
			auth.GrantTypeAuthorizationCode,
			auth.GrantTypeRefreshToken,
		}
	}
}

func validateDynamicRegistrationApplicationType(value string) error {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "native" || value == "web" {
		return nil
	}
	return errors.New("application_type must be native or web")
}

func normalizeDynamicRegistrationResponseTypes(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return []string{"code"}, nil
	}

	seen := map[string]struct{}{}
	out := make([]string, 0, len(requested))
	for _, responseType := range requested {
		responseType = strings.ToLower(strings.TrimSpace(responseType))
		if responseType == "" {
			continue
		}
		if responseType != "code" {
			return nil, errors.New("response_types must only contain code")
		}
		if _, ok := seen[responseType]; ok {
			continue
		}
		seen[responseType] = struct{}{}
		out = append(out, responseType)
	}
	if len(out) == 0 {
		return []string{"code"}, nil
	}
	return out, nil
}

func (h *Handler) oauthDynamicRegistrationError(status int, errorCode, errorDescription string) (*apptheory.Response, error) {
	return apptheory.JSON(status, map[string]string{
		"error":             errorCode,
		"error_description": errorDescription,
	})
}
