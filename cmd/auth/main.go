package main

import (
	"context"
	"encoding/base64"
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
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
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
	store, err = storageDB.New()
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
	ActionURL           string
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

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Get the path, removing the stage prefix if present
	path := request.RawPath
	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	// Log request for debugging
	logger.Info("OAuth request",
		zap.String("raw_path", request.RawPath),
		zap.String("path", path),
		zap.String("method", request.RequestContext.HTTP.Method))

	// Route based on path
	switch path {
	case "/oauth/authorize":
		return handleAuthorize(ctx, request)
	case "/oauth/token":
		return handleToken(ctx, request)
	case "/oauth/revoke":
		return handleRevoke(ctx, request)
	case "/oauth/.well-known/oauth-authorization-server":
		return handleDiscovery(ctx, request)
	default:
		return common.NotFound(errors.New("unknown OAuth endpoint")), nil
	}
}

func handleAuthorize(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Log raw request details for debugging
	logger.Info("raw authorize request details",
		zap.Any("query_params", request.QueryStringParameters),
		zap.String("raw_query_string", request.RawQueryString),
		zap.String("raw_path", request.RawPath))

	// Parse request parameters
	var req AuthorizeRequest
	var loginErr string

	if request.RequestContext.HTTP.Method == http.MethodGet {
		req = AuthorizeRequest{
			ResponseType:        request.QueryStringParameters["response_type"],
			ClientID:            request.QueryStringParameters["client_id"],
			RedirectURI:         request.QueryStringParameters["redirect_uri"],
			State:               request.QueryStringParameters["state"],
			CodeChallenge:       request.QueryStringParameters["code_challenge"],
			CodeChallengeMethod: request.QueryStringParameters["code_challenge_method"],
			Scope:               request.QueryStringParameters["scope"],
		}

		logger.Info("parsed OAuth authorize request",
			zap.String("response_type", req.ResponseType),
			zap.String("client_id", req.ClientID),
			zap.String("redirect_uri", req.RedirectURI),
			zap.String("state", req.State),
			zap.String("code_challenge", req.CodeChallenge),
			zap.String("scope", req.Scope))
	} else if request.RequestContext.HTTP.Method == http.MethodPost {
		// Check if this is a login form submission
		contentType := request.Headers["content-type"]
		if contentType == "" {
			contentType = request.Headers["Content-Type"]
		}

		// Decode body if base64 encoded
		body := request.Body
		if request.IsBase64Encoded {
			decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
			if err != nil {
				logger.Error("failed to decode base64 body", zap.Error(err))
				return common.BadRequest(fmt.Errorf("failed to decode body: %w", err)), nil
			}
			body = string(decodedBytes)
		}

		logger.Info("processing POST request",
			zap.String("content_type", contentType),
			zap.Bool("is_base64", request.IsBase64Encoded),
			zap.String("body", body))

		if strings.Contains(contentType, "application/x-www-form-urlencoded") {
			// Parse form data
			values, err := url.ParseQuery(body)
			if err != nil {
				logger.Error("failed to parse form data", zap.Error(err))
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
				logger.Info("login successful, completing authorization",
					zap.String("username", user.Username),
					zap.String("client_id", req.ClientID))
				return completeAuthorization(ctx, req, user.Username)
			}
		} else {
			// JSON request
			if err := json.Unmarshal([]byte(body), &req); err != nil {
				return common.BadRequest(err), nil
			}
		}
	} else {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Validate request
	if req.ResponseType != "code" {
		logger.Warn("unsupported response type", zap.String("response_type", req.ResponseType))
		return returnOAuthError("unsupported_response_type", "Only authorization code flow is supported", req.RedirectURI, req.State), nil
	}

	if req.ClientID == "" {
		logger.Warn("missing client_id")
		return common.BadRequest(errors.New("missing client_id")), nil
	}

	if req.RedirectURI == "" {
		logger.Warn("missing redirect_uri")
		return common.BadRequest(errors.New("missing redirect_uri")), nil
	}

	// Validate client and redirect URI
	logger.Info("validating client and redirect URI",
		zap.String("client_id", req.ClientID),
		zap.String("redirect_uri", req.RedirectURI))

	if err := oauthSvc.ValidateRedirectURI(ctx, req.ClientID, req.RedirectURI); err != nil {
		logger.Warn("client validation failed",
			zap.String("client_id", req.ClientID),
			zap.String("redirect_uri", req.RedirectURI),
			zap.Error(err))
		return common.BadRequest(err), nil
	}

	// PKCE is optional but recommended
	if req.CodeChallenge == "" {
		logger.Info("PKCE not used for this request")
	} else {
		logger.Info("PKCE enabled", zap.String("code_challenge_method", req.CodeChallengeMethod))
	}

	// Parse scopes
	scopes := []string{auth.ScopeRead, auth.ScopeWrite} // Default scopes
	if req.Scope != "" {
		scopes = strings.Split(req.Scope, " ")
		if err := auth.ValidateScopes(scopes); err != nil {
			logger.Warn("invalid scopes", zap.String("scopes", req.Scope), zap.Error(err))
			return returnOAuthError("invalid_scope", "Invalid scopes requested", req.RedirectURI, req.State), nil
		}
	}

	// Return login page
	logger.Info("rendering login page",
		zap.String("client_id", req.ClientID),
		zap.String("redirect_uri", req.RedirectURI))
	return renderLoginPage(req, loginErr), nil
}

// validateUserCredentials checks username and password
func validateUserCredentials(ctx context.Context, username, password string) (*storage.User, error) {
	logger.Info("validating user credentials", zap.String("username", username))

	if username == "" || password == "" {
		return nil, errors.New("username and password are required")
	}

	// Try to get user by username first
	logger.Info("attempting to get user by username", zap.String("username", username))
	user, err := store.GetUser(ctx, username)
	if err != nil {
		logger.Info("username lookup failed, trying email", zap.Error(err))
		// Try by email
		user, err = store.GetUserByEmail(ctx, username)
		if err != nil {
			logger.Warn("email lookup also failed", zap.Error(err))
			return nil, errors.New("invalid credentials")
		}
	}

	logger.Info("user found, verifying password", zap.String("username", user.Username))

	// Verify password
	if err := auth.VerifyPassword(password, user.PasswordHash); err != nil {
		logger.Warn("password verification failed", zap.Error(err))
		return nil, errors.New("invalid credentials")
	}

	logger.Info("password verified successfully")

	return user, nil
}

// completeAuthorization completes the OAuth flow after successful login
func completeAuthorization(ctx context.Context, req AuthorizeRequest, username string) (*events.APIGatewayV2HTTPResponse, error) {
	logger.Info("starting authorization completion",
		zap.String("username", username),
		zap.String("client_id", req.ClientID))

	// Parse scopes
	scopes := []string{auth.ScopeRead, auth.ScopeWrite} // Default scopes
	if req.Scope != "" {
		scopes = strings.Split(req.Scope, " ")
	}

	// Generate authorization code
	logger.Info("generating authorization code")
	code, err := oauthSvc.GenerateAuthorizationCode()
	if err != nil {
		logger.Error("failed to generate authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	logger.Info("authorization code generated", zap.String("code", code))

	// Store authorization code
	authCode := &storage.AuthorizationCode{
		Code:          code,
		ClientID:      req.ClientID,
		Username:      username,
		CodeChallenge: req.CodeChallenge,
		ExpiresAt:     time.Now().Add(auth.AuthCodeDuration),
		Scopes:        scopes,
	}

	logger.Info("storing authorization code in DynamoDB")
	if err := store.CreateAuthorizationCode(ctx, authCode); err != nil {
		logger.Error("failed to store authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	logger.Info("authorization code stored successfully")

	// Build redirect URL
	redirectURL, _ := url.Parse(req.RedirectURI)
	q := redirectURL.Query()
	q.Set("code", code)
	if req.State != "" {
		q.Set("state", req.State)
	}
	redirectURL.RawQuery = q.Encode()

	logger.Info("redirecting with authorization code",
		zap.String("redirect_url", redirectURL.String()))

	// Return redirect response
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusFound,
		Headers: map[string]string{
			"Location":                     redirectURL.String(),
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
	}, nil
}

// renderLoginPage returns an HTML login page
func renderLoginPage(req AuthorizeRequest, errorMsg string) *events.APIGatewayV2HTTPResponse {
	// Build the form action URL with query parameters
	actionURL := "/oauth/authorize"

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
        <form method="POST" action="{{.ActionURL}}">
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
		ActionURL:           actionURL,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		logger.Error("failed to execute login template", zap.Error(err))
		return common.InternalServerError(err)
	}

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "text/html; charset=utf-8",
		},
		Body: buf.String(),
	}
}

func handleToken(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	logger.Info("token endpoint called",
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.Any("headers", request.Headers),
		zap.String("body", request.Body))

	// Only accept POST
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Parse request
	var req TokenRequest
	contentType := request.Headers["content-type"]
	logger.Info("parsing token request",
		zap.String("content_type", contentType),
		zap.Bool("is_json", strings.Contains(contentType, "application/json")))

	if strings.Contains(contentType, "application/json") {
		if err := json.Unmarshal([]byte(request.Body), &req); err != nil {
			logger.Error("failed to parse JSON", zap.Error(err))
			return returnTokenError("invalid_request", "Invalid JSON"), nil
		}
	} else if strings.Contains(contentType, "multipart/form-data") {
		// Handle multipart/form-data (used by Ivory)
		body := request.Body
		if request.IsBase64Encoded {
			decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
			if err != nil {
				logger.Error("failed to decode base64 body", zap.Error(err))
				return returnTokenError("invalid_request", "Failed to decode body"), nil
			}
			body = string(decodedBytes)
		}

		// Parse multipart form
		boundary := ""
		if idx := strings.Index(contentType, "boundary="); idx != -1 {
			boundary = contentType[idx+9:]
			// Remove quotes if present
			boundary = strings.Trim(boundary, "\"")
		}

		if boundary == "" {
			logger.Error("missing boundary in multipart form")
			return returnTokenError("invalid_request", "Invalid multipart form"), nil
		}

		// Simple multipart parser
		parts := strings.Split(body, "--"+boundary)
		formData := make(map[string]string)

		for _, part := range parts {
			if strings.TrimSpace(part) == "" || strings.HasPrefix(part, "--") {
				continue
			}

			// Split headers and content
			sections := strings.SplitN(part, "\r\n\r\n", 2)
			if len(sections) != 2 {
				continue
			}

			// Extract field name
			headers := sections[0]
			content := strings.TrimSpace(sections[1])

			if strings.Contains(headers, `name="`) {
				start := strings.Index(headers, `name="`) + 6
				end := strings.Index(headers[start:], `"`)
				if end > 0 {
					fieldName := headers[start : start+end]
					formData[fieldName] = content
				}
			}
		}

		// Map form data to TokenRequest
		req = TokenRequest{
			GrantType:    formData["grant_type"],
			Code:         formData["code"],
			RedirectURI:  formData["redirect_uri"],
			ClientID:     formData["client_id"],
			ClientSecret: formData["client_secret"],
			CodeVerifier: formData["code_verifier"],
			RefreshToken: formData["refresh_token"],
			Scope:        formData["scope"],
		}

		logger.Info("parsed multipart form data",
			zap.Any("form_data", formData))
	} else {
		// Parse form data (application/x-www-form-urlencoded)
		body := request.Body
		if request.IsBase64Encoded {
			decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
			if err != nil {
				logger.Error("failed to decode base64 body", zap.Error(err))
				return returnTokenError("invalid_request", "Failed to decode body"), nil
			}
			body = string(decodedBytes)
		}

		values, err := url.ParseQuery(body)
		if err != nil {
			logger.Error("failed to parse form data", zap.Error(err))
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

	logger.Info("parsed token request",
		zap.String("grant_type", req.GrantType),
		zap.String("client_id", req.ClientID),
		zap.String("code", req.Code),
		zap.String("redirect_uri", req.RedirectURI),
		zap.Bool("has_client_secret", req.ClientSecret != ""))

	// Validate client
	logger.Info("validating client credentials")
	if err := oauthSvc.ValidateClient(ctx, req.ClientID, req.ClientSecret); err != nil {
		logger.Error("client validation failed", zap.Error(err))
		return returnTokenError("invalid_client", "Invalid client credentials"), nil
	}
	logger.Info("client validation successful")

	switch req.GrantType {
	case auth.GrantTypeAuthorizationCode:
		logger.Info("handling authorization code grant")
		return handleAuthorizationCodeGrant(ctx, req)
	case auth.GrantTypeRefreshToken:
		logger.Info("handling refresh token grant")
		return handleRefreshTokenGrant(ctx, req)
	case "client_credentials":
		logger.Info("handling client credentials grant")
		return handleClientCredentialsGrant(ctx, req)
	default:
		logger.Warn("unsupported grant type", zap.String("grant_type", req.GrantType))
		return returnTokenError("unsupported_grant_type", "Grant type not supported"), nil
	}
}

func handleAuthorizationCodeGrant(ctx context.Context, req TokenRequest) (*events.APIGatewayV2HTTPResponse, error) {
	logger.Info("starting authorization code grant",
		zap.String("code", req.Code),
		zap.String("client_id", req.ClientID))

	if req.Code == "" {
		logger.Warn("missing authorization code")
		return returnTokenError("invalid_request", "Missing authorization code"), nil
	}

	// Get authorization code
	logger.Info("retrieving authorization code from store")
	authCode, err := store.GetAuthorizationCode(ctx, req.Code)
	if err != nil {
		if common.IsNotFound(err) {
			logger.Warn("authorization code not found", zap.String("code", req.Code))
			return returnTokenError("invalid_grant", "Invalid or expired authorization code"), nil
		}
		logger.Error("failed to get authorization code", zap.Error(err))
		return common.InternalServerError(err), nil
	}
	logger.Info("authorization code retrieved",
		zap.String("username", authCode.Username),
		zap.String("client_id", authCode.ClientID))

	// Add detailed logging about the authorization code
	logger.Info("authorization code details",
		zap.String("code", authCode.Code),
		zap.String("username", authCode.Username),
		zap.String("client_id", authCode.ClientID),
		zap.String("code_challenge", authCode.CodeChallenge),
		zap.Time("expires_at", authCode.ExpiresAt),
		zap.Int("scopes_count", len(authCode.Scopes)))

	// Check if authCode is nil (defensive programming)
	if authCode == nil {
		logger.Error("authorization code is nil after retrieval")
		return returnTokenError("invalid_grant", "Invalid authorization code"), nil
	}

	logger.Info("checking client ID match",
		zap.String("auth_code_client_id", authCode.ClientID),
		zap.String("request_client_id", req.ClientID))

	// Verify client ID matches
	if authCode.ClientID != req.ClientID {
		logger.Warn("client ID mismatch",
			zap.String("expected", authCode.ClientID),
			zap.String("received", req.ClientID))
		return returnTokenError("invalid_grant", "Client ID mismatch"), nil
	}

	logger.Info("client ID match verified")

	// Verify PKCE if it was used
	if authCode.CodeChallenge != "" {
		logger.Info("PKCE verification required",
			zap.String("code_challenge", authCode.CodeChallenge))
		if req.CodeVerifier == "" {
			logger.Warn("missing PKCE code_verifier")
			return returnTokenError("invalid_request", "PKCE code_verifier is required"), nil
		}
		if err := oauthSvc.VerifyCodeChallenge(authCode.CodeChallenge, req.CodeVerifier, "S256"); err != nil {
			logger.Error("PKCE verification failed", zap.Error(err))
			return returnTokenError("invalid_grant", "Invalid PKCE code_verifier"), nil
		}
		logger.Info("PKCE verification successful")
	} else {
		logger.Info("PKCE not used for this authorization code")
	}

	// Generate tokens
	logger.Info("generating tokens",
		zap.String("username", authCode.Username),
		zap.String("client_id", authCode.ClientID),
		zap.Int("scopes_count", len(authCode.Scopes)))

	accessToken, refreshToken, err := oauthSvc.GenerateTokens(authCode.Username, authCode.ClientID, authCode.Scopes)
	if err != nil {
		logger.Error("failed to generate tokens", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	logger.Info("tokens generated successfully",
		zap.Int("access_token_length", len(accessToken)),
		zap.Int("refresh_token_length", len(refreshToken)))

	// Store refresh token
	logger.Info("storing refresh token")
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
	logger.Info("refresh token stored successfully")

	// Delete used authorization code
	logger.Info("deleting used authorization code")
	if err := store.DeleteAuthorizationCode(ctx, req.Code); err != nil {
		logger.Warn("failed to delete authorization code", zap.Error(err))
	} else {
		logger.Info("authorization code deleted successfully")
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

	logger.Info("returning token response",
		zap.Int("status", http.StatusOK),
		zap.String("token_type", resp.TokenType),
		zap.Int("expires_in", resp.ExpiresIn),
		zap.String("scope", resp.Scope))

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Cache-Control":                "no-store",
			"Pragma":                       "no-cache",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}, nil
}

func handleRefreshTokenGrant(ctx context.Context, req TokenRequest) (*events.APIGatewayV2HTTPResponse, error) {
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
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Cache-Control":                "no-store",
			"Pragma":                       "no-cache",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}, nil
}

func handleClientCredentialsGrant(ctx context.Context, req TokenRequest) (*events.APIGatewayV2HTTPResponse, error) {
	logger.Info("processing client credentials grant",
		zap.String("client_id", req.ClientID),
		zap.String("scope", req.Scope))

	// Client credentials grant doesn't use authorization codes
	// It directly exchanges client credentials for an access token

	// Parse requested scopes
	scopes := []string{auth.ScopeRead} // Default minimal scope
	if req.Scope != "" {
		requestedScopes := strings.Split(req.Scope, " ")
		// Validate scopes
		if err := auth.ValidateScopes(requestedScopes); err == nil {
			scopes = requestedScopes
		}
	}

	// Generate a client token (not associated with a user)
	// For client credentials, we use the client_id as the "username" in the token
	accessToken, _, err := oauthSvc.GenerateTokens(req.ClientID, req.ClientID, scopes)
	if err != nil {
		logger.Error("failed to generate client token", zap.Error(err))
		return common.InternalServerError(err), nil
	}

	// Return token response (no refresh token for client credentials)
	resp := TokenResponse{
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresIn:   int(auth.AccessTokenDuration.Seconds()),
		Scope:       strings.Join(scopes, " "),
	}

	body, _ := json.Marshal(resp)

	logger.Info("client credentials token issued",
		zap.String("client_id", req.ClientID),
		zap.String("scope", resp.Scope))

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Cache-Control":                "no-store",
			"Pragma":                       "no-cache",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}, nil
}

func handleRevoke(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	logger.Info("revoke endpoint called",
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.Any("headers", request.Headers))

	// Only accept POST
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Parse request
	contentType := request.Headers["content-type"]
	if contentType == "" {
		contentType = request.Headers["Content-Type"]
	}

	var token string
	var tokenTypeHint string
	var clientID string
	var clientSecret string

	// Parse form data
	body := request.Body
	if request.IsBase64Encoded {
		decodedBytes, err := base64.StdEncoding.DecodeString(request.Body)
		if err != nil {
			logger.Error("failed to decode base64 body", zap.Error(err))
			return common.BadRequest(err), nil
		}
		body = string(decodedBytes)
	}

	values, err := url.ParseQuery(body)
	if err != nil {
		logger.Error("failed to parse form data", zap.Error(err))
		return common.BadRequest(err), nil
	}

	token = values.Get("token")
	tokenTypeHint = values.Get("token_type_hint")
	clientID = values.Get("client_id")
	clientSecret = values.Get("client_secret")

	// Validate request
	if token == "" {
		logger.Warn("missing token parameter")
		return common.BadRequest(errors.New("missing token parameter")), nil
	}

	// Validate client credentials if provided
	if clientID != "" {
		if err := oauthSvc.ValidateClient(ctx, clientID, clientSecret); err != nil {
			logger.Error("client validation failed", zap.Error(err))
			// Per RFC 7009, invalid client credentials should still return 200 OK
			// to prevent token scanning attacks
			return &events.APIGatewayV2HTTPResponse{
				StatusCode: http.StatusOK,
				Headers: map[string]string{
					"Content-Type":                 "application/json",
					"Cache-Control":                "no-store",
					"Pragma":                       "no-cache",
					"Access-Control-Allow-Origin":  "*",
					"Access-Control-Allow-Headers": "Content-Type, Authorization",
				},
				Body: "{}",
			}, nil
		}
	}

	// Try to revoke based on token type hint
	revoked := false

	if tokenTypeHint == "" || tokenTypeHint == "refresh_token" {
		// Try to revoke as refresh token first
		err := store.DeleteRefreshToken(ctx, token)
		if err == nil {
			logger.Info("refresh token revoked", zap.String("token", token[:10]+"..."))
			revoked = true
		} else if !common.IsNotFound(err) {
			logger.Error("failed to delete refresh token", zap.Error(err))
		}
	}

	if !revoked && (tokenTypeHint == "" || tokenTypeHint == "access_token") {
		// Try to revoke as access token
		// Since we're using JWTs for access tokens, we can't truly revoke them
		// but we can add them to a blacklist if needed
		// For now, we'll just validate it's a valid JWT
		claims, err := oauthSvc.ValidateAccessToken(token)
		if err == nil {
			logger.Info("access token validated for revocation",
				zap.String("username", claims.Username),
				zap.String("client_id", claims.ClientID))
			// In a production system, you might want to add this to a blacklist
			revoked = true
		}
	}

	// Per RFC 7009, always return 200 OK regardless of whether the token was found
	// This prevents token scanning attacks
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Cache-Control":                "no-store",
			"Pragma":                       "no-cache",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: "{}",
	}, nil
}

func handleDiscovery(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	// Only accept GET
	if request.RequestContext.HTTP.Method != http.MethodGet {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Return OAuth discovery document
	discovery := map[string]interface{}{
		"issuer":                                cfg.BaseURL(),
		"authorization_endpoint":                cfg.BaseURL() + "/oauth/authorize",
		"token_endpoint":                        cfg.BaseURL() + "/oauth/token",
		"revocation_endpoint":                   cfg.BaseURL() + "/oauth/revoke",
		"response_types_supported":              []string{"code"},
		"grant_types_supported":                 []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported":      []string{"S256", "plain"},
		"token_endpoint_auth_methods_supported": []string{"client_secret_post", "client_secret_basic"},
		"scopes_supported":                      []string{"read", "write", "follow", "push"},
	}

	body, _ := json.Marshal(discovery)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}, nil
}

func returnOAuthError(error, description, redirectURI, state string) *events.APIGatewayV2HTTPResponse {
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

		return &events.APIGatewayV2HTTPResponse{
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

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: string(body),
	}
}

func returnTokenError(error, description string) *events.APIGatewayV2HTTPResponse {
	errResp := ErrorResponse{
		Error:            error,
		ErrorDescription: description,
	}
	body, _ := json.Marshal(errResp)

	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusBadRequest,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Cache-Control":                "no-store",
			"Pragma":                       "no-cache",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}
}

// methodNotAllowed returns a 405 Method Not Allowed response
func methodNotAllowed(method string) *events.APIGatewayV2HTTPResponse {
	return common.ErrorResponseWithCode(
		http.StatusMethodNotAllowed,
		"METHOD_NOT_ALLOWED",
		fmt.Errorf("method %s not allowed", method),
	)
}

func main() {
	lambda.Start(handler)
}
