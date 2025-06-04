package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/storage"
	"github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg      *config.Config
	store    storage.Storage
	logger   *zap.Logger
	oauthSvc *auth.OAuthService
)

func init() {
	cfg = config.Get()
	logger = common.Logger()

	var err error
	store, err = dynamodb.New()
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	// Initialize OAuth service
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// Use a default for development - should be set via environment variable in production
		jwtSecret = "development-secret-change-me"
		logger.Warn("JWT_SECRET not set, using development default")
	}
	oauthSvc = auth.NewOAuthService(jwtSecret, store)

	// Create default client for development if it doesn't exist
	ctx := context.Background()
	defaultClientID := "lesser-web"
	if _, err := store.GetOAuthClient(ctx, defaultClientID); err != nil {
		// Create default client
		defaultClient := &storage.OAuthClient{
			ClientID:     defaultClientID,
			ClientSecret: "development-client-secret",
			Name:         "Lesser Web Client",
			RedirectURIs: []string{cfg.BaseURL() + "/auth/callback"},
		}
		if err := store.CreateOAuthClient(ctx, defaultClient); err != nil {
			logger.Warn("failed to create default OAuth client", zap.Error(err))
		}
	}
}

// AuthorizeRequest represents the authorization endpoint request
type AuthorizeRequest struct {
	ResponseType        string `json:"response_type"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Scope               string `json:"scope"`
}

// TokenRequest represents the token endpoint request
type TokenRequest struct {
	GrantType    string `json:"grant_type"`
	Code         string `json:"code"`
	RedirectURI  string `json:"redirect_uri"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	CodeVerifier string `json:"code_verifier"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

// TokenResponse represents the token endpoint response
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// ErrorResponse represents an OAuth error response
type ErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// LoginPageData represents data for the login page template
type LoginPageData struct {
	ClientID            string
	RedirectURI         string
	State               string
	CodeChallenge       string
	CodeChallengeMethod string
	Scope               string
	ResponseType        string
	Error               string
}

// LoginRequest represents a login form submission
type LoginRequest struct {
	Username            string `json:"username"`
	Password            string `json:"password"`
	ClientID            string `json:"client_id"`
	RedirectURI         string `json:"redirect_uri"`
	State               string `json:"state"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	Scope               string `json:"scope"`
	ResponseType        string `json:"response_type"`
}

func handler(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Route based on path
	switch request.Path {
	case "/oauth/authorize":
		return handleAuthorize(ctx, request)
	case "/oauth/token":
		return handleToken(ctx, request)
	case "/oauth/.well-known/oauth-authorization-server":
		return handleDiscovery(ctx, request)
	default:
		return common.NotFound(errors.New("unknown OAuth endpoint")), nil
	}
}

func handleAuthorize(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Parse request parameters
	var req AuthorizeRequest
	var loginErr string

	if request.HTTPMethod == http.MethodGet {
		req = AuthorizeRequest{
			ResponseType:        request.QueryStringParameters["response_type"],
			ClientID:            request.QueryStringParameters["client_id"],
			RedirectURI:         request.QueryStringParameters["redirect_uri"],
			State:               request.QueryStringParameters["state"],
			CodeChallenge:       request.QueryStringParameters["code_challenge"],
			CodeChallengeMethod: request.QueryStringParameters["code_challenge_method"],
			Scope:               request.QueryStringParameters["scope"],
		}
	} else if request.HTTPMethod == http.MethodPost {
		// Check if this is a login form submission
		contentType := request.Headers["content-type"]
		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			// Parse form data
			values, err := url.ParseQuery(request.Body)
			if err != nil {
				return common.BadRequest(err), nil
			}

			// Handle login
			username := values.Get("username")
			password := values.Get("password")

			// Reconstruct OAuth request from form fields
			req = AuthorizeRequest{
				ResponseType:        values.Get("response_type"),
				ClientID:            values.Get("client_id"),
				RedirectURI:         values.Get("redirect_uri"),
				State:               values.Get("state"),
				CodeChallenge:       values.Get("code_challenge"),
				CodeChallengeMethod: values.Get("code_challenge_method"),
				Scope:               values.Get("scope"),
			}

			// Validate credentials
			user, err := validateUserCredentials(ctx, username, password)
			if err != nil {
				logger.Warn("login failed", zap.String("username", username), zap.Error(err))
				loginErr = "Invalid username or password"
			} else if user.Suspended {
				loginErr = "Account is suspended"
			} else if !user.Approved {
				loginErr = "Account is pending approval"
			} else {
				// Login successful, continue with OAuth flow
				return completeAuthorization(ctx, req, user.Username)
			}
		} else {
			// JSON request
			if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
				return common.BadRequest(err), nil
			}
		}
	} else {
		return methodNotAllowed(request.HTTPMethod), nil
	}

	// Validate request
	if req.ResponseType != "code" {
		return returnOAuthError("unsupported_response_type", "Only authorization code flow is supported", req.RedirectURI, req.State), nil
	}

	if req.ClientID == "" {
		return common.BadRequest(errors.New("missing client_id")), nil
	}

	if req.RedirectURI == "" {
		return common.BadRequest(errors.New("missing redirect_uri")), nil
	}

	// Validate client and redirect URI
	if err := oauthSvc.ValidateRedirectURI(ctx, req.ClientID, req.RedirectURI); err != nil {
		return common.BadRequest(err), nil
	}

	// PKCE is required
	if req.CodeChallenge == "" {
		return returnOAuthError("invalid_request", "PKCE code_challenge is required", req.RedirectURI, req.State), nil
	}

	// Parse scopes
	scopes := []string{auth.ScopeRead, auth.ScopeWrite} // Default scopes
	if req.Scope != "" {
		scopes = strings.Split(req.Scope, " ")
		if err := auth.ValidateScopes(scopes); err != nil {
			return returnOAuthError("invalid_scope", "Invalid scopes requested", req.RedirectURI, req.State), nil
		}
	}

	// Return login page
	return renderLoginPage(req, loginErr), nil
}

// validateUserCredentials checks username and password
func validateUserCredentials(ctx context.Context, username, password string) (*storage.User, error) {
	if username == "" || password == "" {
		return nil, errors.New("username and password are required")
	}

	// Try to get user by username first
	user, err := store.GetUser(ctx, username)
	if err != nil {
		// Try by email
		user, err = store.GetUserByEmail(ctx, username)
		if err != nil {
			return nil, errors.New("invalid credentials")
		}
	}

	// Verify password
	if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
		return nil, errors.New("invalid credentials")
	}

	return user, nil
}

// completeAuthorization completes the OAuth flow after successful login
func completeAuthorization(ctx context.Context, req AuthorizeRequest, username string) (events.APIGatewayProxyResponse, error) {
	// Parse scopes
	scopes := []string{auth.ScopeRead, auth.ScopeWrite} // Default scopes
	if req.Scope != "" {
		scopes = strings.Split(req.Scope, " ")
	}

	// Generate authorization code
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		logger.Error("failed to generate authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Store authorization code
	authCode := &storage.AuthorizationCode{
		Code:          code,
		ClientID:      req.ClientID,
		Username:      username,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     time.Now().Add(auth.AuthCodeDuration),
		Scopes:        scopes,
	}

	if err := store.CreateAuthorizationCode(ctx, authCode); err != nil {
		logger.Error("failed to store authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Build redirect URL
	redirectURL, _ := url.Parse(req.RedirectURI)
	q := redirectURL.Query()
	q.Set("code", code)
	if req.State != "" {
		q.Set("state", req.State)
	}
	redirectURL.RawQuery = q.Encode()

	// Return redirect response
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusFound,
		Headers: map[string]string{
			"Location": redirectURL.String(),
		},
	}, nil
}

// renderLoginPage returns an HTML login page
func renderLoginPage(req AuthorizeRequest, errorMsg string) events.APIGatewayProxyResponse {
	loginHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - Lesser</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
            display: flex;
            justify-content: center;
            align-items: center;
            min-height: 100vh;
            margin: 0;
            background-color: #f5f5f5;
        }
        .login-container {
            background: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            width: 100%;
            max-width: 400px;
        }
        h1 {
            text-align: center;
            color: #333;
            margin-bottom: 2rem;
        }
        .form-group {
            margin-bottom: 1rem;
        }
        label {
            display: block;
            margin-bottom: 0.5rem;
            color: #555;
            font-weight: 500;
        }
        input {
            width: 100%;
            padding: 0.75rem;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 1rem;
            box-sizing: border-box;
        }
        input:focus {
            outline: none;
            border-color: #4CAF50;
        }
        button {
            width: 100%;
            padding: 0.75rem;
            background-color: #4CAF50;
            color: white;
            border: none;
            border-radius: 4px;
            font-size: 1rem;
            font-weight: 500;
            cursor: pointer;
            margin-top: 1rem;
        }
        button:hover {
            background-color: #45a049;
        }
        .error {
            background-color: #fee;
            color: #c33;
            padding: 0.75rem;
            border-radius: 4px;
            margin-bottom: 1rem;
            text-align: center;
        }
        .info {
            color: #666;
            font-size: 0.875rem;
            text-align: center;
            margin-top: 1rem;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>Sign in to Lesser</h1>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        <form method="POST" action="/oauth/authorize">
            <input type="hidden" name="response_type" value="{{.ResponseType}}">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            
            <div class="form-group">
                <label for="username">Username or Email</label>
                <input type="text" id="username" name="username" required autofocus>
            </div>
            
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <button type="submit">Sign In</button>
        </form>
        
        <p class="info">
            Don't have an account? <a href="/register">Sign up</a>
        </p>
    </div>
</body>
</html>`

	// Parse and execute template
	tmpl, err := template.New("login").Parse(loginHTML)
	if err != nil {
		logger.Error("failed to parse login template", zap.Error(err))
		return common.InternalServerError(err)
	}

	data := LoginPageData{
		ClientID:            req.ClientID,
		RedirectURI:         req.RedirectURI,
		State:               req.State,
		CodeChallenge:       req.CodeChallenge,
		CodeChallengeMethod: req.CodeChallengeMethod,
		Scope:               req.Scope,
		ResponseType:        req.ResponseType,
		Error:               errorMsg,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		logger.Error("failed to execute login template", zap.Error(err))
		return common.InternalServerError(err)
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "text/html; charset=utf-8",
		},
		Body: buf.String(),
	}
}

func handleToken(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Only accept POST
	if request.HTTPMethod != http.MethodPost {
		return methodNotAllowed(request.HTTPMethod), nil
	}

	// Parse request
	var req TokenRequest
	contentType := request.Headers["content-type"]
	if strings.Contains(contentType, "application/json") {
		if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
			return returnTokenError("invalid_request", "Invalid JSON"), nil
		}
	} else {
		// Parse form data
		values, err := url.ParseQuery(request.Body)
		if err != nil {
			return returnTokenError("invalid_request", "Invalid form data"), nil
		}
		req = TokenRequest{
			GrantType:    values.Get("grant_type"),
			Code:         values.Get("code"),
			RedirectURI:  values.Get("redirect_uri"),
			ClientID:     values.Get("client_id"),
			ClientSecret: values.Get("client_secret"),
			CodeVerifier: values.Get("code_verifier"),
			RefreshToken: values.Get("refresh_token"),
			Scope:        values.Get("scope"),
		}
	}

	// Validate client
	if err := oauthSvc.ValidateClient(ctx, req.ClientID, req.ClientSecret); err != nil {
		return returnTokenError("invalid_client", "Invalid client credentials"), nil
	}

	switch req.GrantType {
	case auth.GrantTypeAuthorizationCode:
		return handleAuthorizationCodeGrant(ctx, req)
	case auth.GrantTypeRefreshToken:
		return handleRefreshTokenGrant(ctx, req)
	default:
		return returnTokenError("unsupported_grant_type", "Grant type not supported"), nil
	}
}

func handleAuthorizationCodeGrant(ctx context.Context, req TokenRequest) (events.APIGatewayProxyResponse, error) {
	if req.Code == "" {
		return returnTokenError("invalid_request", "Missing authorization code"), nil
	}

	if req.CodeVerifier == "" {
		return returnTokenError("invalid_request", "Missing PKCE code_verifier"), nil
	}

	// Get authorization code
	authCode, err := store.GetAuthorizationCode(ctx, req.Code)
	if err != nil {
		if common.IsNotFound(err) {
			return returnTokenError("invalid_grant", "Invalid or expired authorization code"), nil
		}
		logger.Error("failed to get authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Verify client ID matches
	if authCode.ClientID != req.ClientID {
		return returnTokenError("invalid_grant", "Client ID mismatch"), nil
	}

	// Verify PKCE
	if err := oauthSvc.VerifyCodeChallenge(authCode.CodeChallenge, req.CodeVerifier, "S256"); err != nil {
		return returnTokenError("invalid_grant", "Invalid PKCE code_verifier"), nil
	}

	// Generate tokens
	accessToken, refreshToken, err := oauthSvc.GenerateTokens(authCode.Username, authCode.ClientID, authCode.Scopes)
	if err != nil {
		logger.Error("failed to generate tokens", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Store refresh token
	refreshTokenData := &storage.RefreshToken{
		Token:     refreshToken,
		ClientID:  authCode.ClientID,
		Username:  authCode.Username,
		ExpiresAt: time.Now().Add(auth.RefreshTokenDuration),
		Scopes:    authCode.Scopes,
	}

	if err := store.CreateRefreshToken(ctx, refreshTokenData); err != nil {
		logger.Error("failed to store refresh token", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Delete used authorization code
	if err := store.DeleteAuthorizationCode(ctx, req.Code); err != nil {
		logger.Warn("failed to delete authorization code", zap.Error(err))
	}

	// Return tokens
	resp := TokenResponse{
		AccessToken:  accessToken,
		TokenType:    "Bearer",
		ExpiresIn:    int(auth.AccessTokenDuration.Seconds()),
		RefreshToken: refreshToken,
		Scope:        strings.Join(authCode.Scopes, " "),
	}

	body, _ := json.Marshal(resp)
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Cache-Control": "no-store",
			"Pragma":        "no-cache",
		},
		Body: string(body),
	}, nil
}

func handleRefreshTokenGrant(ctx context.Context, req TokenRequest) (events.APIGatewayProxyResponse, error) {
	if req.RefreshToken == "" {
		return returnTokenError("invalid_request", "Missing refresh token"), nil
	}

	// Get refresh token
	refreshToken, err := store.GetRefreshToken(ctx, req.RefreshToken)
	if err != nil {
		if common.IsNotFound(err) {
			return returnTokenError("invalid_grant", "Invalid or expired refresh token"), nil
		}
		logger.Error("failed to get refresh token", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Verify client ID matches
	if refreshToken.ClientID != req.ClientID {
		return returnTokenError("invalid_grant", "Client ID mismatch"), nil
	}

	// Generate new access token
	accessToken, _, err := oauthSvc.GenerateTokens(refreshToken.Username, refreshToken.ClientID, refreshToken.Scopes)
	if err != nil {
		logger.Error("failed to generate access token", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return new access token
	resp := TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(auth.AccessTokenDuration.Seconds()),
		Scope:       strings.Join(refreshToken.Scopes, " "),
	}

	body, _ := json.Marshal(resp)
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Cache-Control": "no-store",
			"Pragma":        "no-cache",
		},
		Body: string(body),
	}, nil
}

func handleDiscovery(ctx context.Context, request events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	// Only accept GET
	if request.HTTPMethod != http.MethodGet {
		return methodNotAllowed(request.HTTPMethod), nil
	}

	// Return OAuth discovery document
	discovery := map[string]interface{}{
		"issuer":                                cfg.BaseURL(),
		"authorization_endpoint":                cfg.BaseURL() + "/oauth/authorize",
		"token_endpoint":                        cfg.BaseURL() + "/oauth/token",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"scopes_supported":                      []string{"read", "write"},
	}

	body, _ := json.Marshal(discovery)
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

func returnOAuthError(error, description, redirectURI, state string) events.APIGatewayProxyResponse {
	// If we have a redirect URI, redirect with error
	if redirectURI != "" {
		redirectURL, _ := url.Parse(redirectURI)
		q := redirectURL.Query()
		q.Set("error", error)
		if description != "" {
			q.Set("error_description", description)
		}
		if state != "" {
			q.Set("state", state)
		}
		redirectURL.RawQuery = q.Encode()

		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusFound,
			Headers: map[string]string{
				"Location": redirectURL.String(),
			},
		}
	}

	// Otherwise return JSON error
	errResp := ErrorResponse{
		Error:            error,
		ErrorDescription: description,
	}
	body, _ := json.Marshal(errResp)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func returnTokenError(error, description string) events.APIGatewayProxyResponse {
	errResp := ErrorResponse{
		Error:            error,
		ErrorDescription: description,
	}
	body, _ := json.Marshal(errResp)

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			"Content-Type":  "application/json",
			"Cache-Control": "no-store",
			"Pragma":        "no-cache",
		},
		Body: string(body),
	}
}

// methodNotAllowed returns a 405 Method Not Allowed response
func methodNotAllowed(method string) events.APIGatewayProxyResponse {
	return common.ErrorResponseWithCode(
		http.StatusMethodNotAllowed,
		"METHOD_NOT_ALLOWED",
		fmt.Errorf("method %s not allowed", method),
	)
}

func main() {
	lambda.Start(handler)
}
