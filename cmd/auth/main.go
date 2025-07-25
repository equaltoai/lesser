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

	"github.com/aron23/lesser/pkg/activitypub"
	"github.com/aron23/lesser/pkg/auth"
	"github.com/aron23/lesser/pkg/common"
	"github.com/aron23/lesser/pkg/config"
	"github.com/aron23/lesser/pkg/federation"
	"github.com/aron23/lesser/pkg/storage"
	storageDB "github.com/aron23/lesser/pkg/storage/dynamodb"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/zap"
)

var (
	cfg         *config.Config
	store       storage.Storage
	logger      *zap.Logger
	oauthSvc    *auth.OAuthService
	webAuthnSvc *auth.WebAuthnService
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

	// Initialize WebAuthn service
	domain := cfg.Domain
	if domain == "" {
		domain = "lesser.host"
	}
	webAuthnSvc, err = auth.NewWebAuthnService(store, domain, "Lesser")
	if err != nil {
		logger.Error("failed to initialize WebAuthn service", zap.Error(err))
		// WebAuthn is optional, so we don't fail here
	}

	// Create default client for development if it doesn't exist
	ctx := context.Background()
	defaultClientID := "lesser-web"
	existingClient, err := store.GetOAuthClient(ctx, defaultClientID)
	if err != nil {
		logger.Info("creating default OAuth client",
			zap.String("client_id", defaultClientID),
			zap.String("redirect_uri", cfg.BaseURL()+"/auth/callback"))
		// Create default client
		defaultClient := &storage.OAuthClient{
			ClientID:     defaultClientID,
			ClientSecret: "development-client-secret",
			Name:         "Lesser Web Client",
			RedirectURIs: []string{cfg.BaseURL() + "/auth/callback"},
		}
		if err := store.CreateOAuthClient(ctx, defaultClient); err != nil {
			logger.Warn("failed to create default OAuth client", zap.Error(err))
		} else {
			logger.Info("created default OAuth client successfully")
		}
	} else {
		logger.Info("default OAuth client exists",
			zap.String("client_id", defaultClientID),
			zap.Strings("redirect_uris", existingClient.RedirectURIs))
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
	if path == "" {
		path = request.RequestContext.HTTP.Path
	}
	if request.RequestContext.Stage != "" && strings.HasPrefix(path, "/"+request.RequestContext.Stage) {
		path = strings.TrimPrefix(path, "/"+request.RequestContext.Stage)
	}

	// Log request for debugging
	logger.Info("OAuth request",
		zap.String("raw_path", request.RawPath),
		zap.String("path", path),
		zap.String("method", request.RequestContext.HTTP.Method),
		zap.String("stage", request.RequestContext.Stage),
		zap.Any("path_parameters", request.PathParameters),
		zap.String("request_context_path", request.RequestContext.HTTP.Path))

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
	case "/api/v1/auth/webauthn/login/begin", "/auth/webauthn/login/begin":
		return handleWebAuthnLoginBegin(ctx, request)
	case "/api/v1/auth/webauthn/login/finish", "/auth/webauthn/login/finish":
		return handleWebAuthnLoginFinish(ctx, request)
	case "/oauth/register":
		return handleRegister(ctx, request)
	case "/oauth/accounts", "/api/v1/accounts", "/accounts":
		return handleAccountCreation(ctx, request)
	case "/api/v1/auth/webauthn/register/begin", "/auth/webauthn/register/begin":
		return handleWebAuthnRegisterBegin(ctx, request)
	case "/api/v1/auth/webauthn/register/finish", "/auth/webauthn/register/finish":
		return handleWebAuthnRegisterFinish(ctx, request)
	default:
		logger.Error("unknown OAuth endpoint",
			zap.String("path", path),
			zap.String("raw_path", request.RawPath),
			zap.String("request_context_path", request.RequestContext.HTTP.Path))
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

	switch request.RequestContext.HTTP.Method {
	case http.MethodGet:
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
			zap.String("scope", req.Scope),
			zap.Any("all_params", request.QueryStringParameters))

		// Validate required parameters
		if req.RedirectURI == "" {
			loginErr = "Invalid request: redirect_uri is required"
			logger.Error("redirect_uri is missing from request",
				zap.Any("query_params", request.QueryStringParameters),
				zap.String("raw_query", request.RawQueryString))
		}
	case http.MethodPost:
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
			webauthnResponse := values.Get("webauthn_response")

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

			var user *storage.User

			// Check if this is a WebAuthn authentication
			if webauthnResponse != "" {
				logger.Info("handling WebAuthn authentication", zap.String("username", username))

				// Parse WebAuthn response
				var webauthnData struct {
					Challenge  string                 `json:"challenge"`
					Credential map[string]interface{} `json:"credential"`
				}
				if err := json.Unmarshal([]byte(webauthnResponse), &webauthnData); err != nil {
					logger.Error("failed to parse WebAuthn response", zap.Error(err))
					loginErr = "Invalid authentication response"
				} else {
					// Verify WebAuthn authentication
					var authErr error
					user, authErr = validateWebAuthnCredentials(ctx, username, webauthnData.Challenge, webauthnData.Credential)
					if authErr != nil {
						logger.Warn("WebAuthn authentication failed", zap.String("username", username), zap.Error(authErr))
						loginErr = "Authentication failed"
					}
				}
			} else if password != "" {
				// Fall back to password authentication
				var authErr error
				user, authErr = validateUserCredentials(ctx, username, password)
				if authErr != nil {
					logger.Warn("login failed", zap.String("username", username), zap.Error(authErr))
					loginErr = "Invalid username or password"
				}
			} else {
				loginErr = "Authentication required"
			}

			// Check user status
			if user != nil && loginErr == "" {
				if user.Suspended {
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
			}
		} else {
			// JSON request
			if err := common.ParseRequestBody([]byte(body), &req); err != nil {
				return common.BadRequest(err), nil
			}
		}
	default:
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

func validateWebAuthnCredentials(ctx context.Context, username string, challenge string, credential map[string]interface{}) (*storage.User, error) {
	if webAuthnSvc == nil {
		return nil, errors.New("WebAuthn service not available")
	}

	logger.Info("validating WebAuthn credentials", zap.String("username", username))

	// Get user from storage first to validate they exist
	user, err := store.GetUser(ctx, username)
	if err != nil {
		logger.Error("failed to get user", zap.Error(err))
		return nil, errors.New("invalid credentials")
	}

	// Convert credential map to JSON for the WebAuthn service
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		logger.Error("failed to marshal credential", zap.Error(err))
		return nil, errors.New("invalid credential format")
	}

	// Use the WebAuthn service to finish login verification
	_, err = webAuthnSvc.FinishLogin(ctx, username, challenge, credentialJSON)
	if err != nil {
		logger.Error("WebAuthn verification failed",
			zap.String("username", username),
			zap.Error(err))

		switch err {
		case auth.ErrChallengeNotFound:
			return nil, errors.New("invalid or expired challenge")
		case auth.ErrInvalidCredential:
			return nil, errors.New("invalid credential")
		case auth.ErrUserHasNoCredentials:
			return nil, errors.New("no passkeys registered for this account")
		default:
			return nil, errors.New("authentication failed")
		}
	}

	logger.Info("WebAuthn authentication successful",
		zap.String("username", username))

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
        button:disabled {
            background-color: #ccc;
            cursor: not-allowed;
        }
        .primary-button {
            background-color: #2196F3;
        }
        .primary-button:hover {
            background-color: #1976D2;
        }
        .secondary-button {
            background-color: #757575;
            font-size: 0.875rem;
            padding: 0.5rem;
        }
        .secondary-button:hover {
            background-color: #616161;
        }
        .error {
            background-color: #fee;
            color: #c33;
            padding: 0.75rem;
            border-radius: 4px;
            margin-bottom: 1rem;
            text-align: center;
        }
        .warning {
            background-color: #fff3cd;
            color: #856404;
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
        .webauthn-section {
            text-align: center;
            padding: 1.5rem 0;
            border-bottom: 1px solid #e0e0e0;
            margin-bottom: 1.5rem;
        }
        .webauthn-icon {
            width: 48px;
            height: 48px;
            margin: 0 auto 1rem;
            background-color: #e3f2fd;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .or-divider {
            text-align: center;
            margin: 1.5rem 0;
            color: #999;
            font-size: 0.875rem;
        }
        .hidden {
            display: none;
        }
        .loading {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #f3f3f3;
            border-top: 2px solid #333;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-right: 0.5rem;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="login-container">
        <h1>Sign in to Lesser</h1>
        {{if .Error}}
        <div class="error">{{.Error}}</div>
        {{end}}
        
        <!-- Hidden form to submit OAuth data -->
        <form id="oauthForm" method="POST" action="{{.ActionURL}}" style="display: none;">
            <input type="hidden" name="response_type" value="{{.ResponseType}}">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            <input type="hidden" name="username" id="hiddenUsername">
            <input type="hidden" name="webauthn_response" id="webauthnResponse">
        </form>
        
        <!-- WebAuthn Section (Primary) -->
        <div class="webauthn-section" id="webauthnSection">
            <div class="webauthn-icon">
                <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#2196F3" stroke-width="2">
                    <path d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"/>
                </svg>
            </div>
            
            <div class="form-group">
                <label for="webauthnUsername">Username</label>
                <input type="text" id="webauthnUsername" name="webauthnUsername" required autofocus>
            </div>
            
            <button type="button" id="webauthnButton" class="primary-button" onclick="loginWithPasskey()">
                Sign in with Passkey
            </button>
            
            <p style="margin-top: 0.5rem; font-size: 0.75rem; color: #666;">
                Secure, passwordless authentication
            </p>
        </div>
        
        <div class="or-divider">or use password</div>
        
        <!-- Password login (fallback) -->
        <form method="POST" action="{{.ActionURL}}" id="passwordForm">
            <input type="hidden" name="response_type" value="{{.ResponseType}}">
            <input type="hidden" name="client_id" value="{{.ClientID}}">
            <input type="hidden" name="redirect_uri" value="{{.RedirectURI}}">
            <input type="hidden" name="state" value="{{.State}}">
            <input type="hidden" name="code_challenge" value="{{.CodeChallenge}}">
            <input type="hidden" name="code_challenge_method" value="{{.CodeChallengeMethod}}">
            <input type="hidden" name="scope" value="{{.Scope}}">
            
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required>
            </div>
            
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            
            <button type="submit" class="secondary-button">Sign in with Password</button>
        </form>
        
        <div id="webauthnWarning" class="warning hidden" style="margin-top: 1rem;">
            WebAuthn is not supported in your browser. Please use a modern browser or sign in with password.
        </div>
        
        <p class="info">
            Don't have an account? <a href="/oauth/register" id="registerLink">Create one</a>
        </p>
    </div>
    
    <script>
        // Set up registration link with OAuth parameters
        document.addEventListener('DOMContentLoaded', function() {
            const registerLink = document.getElementById('registerLink');
            if (registerLink) {
                // Get current URL parameters
                const urlParams = new URLSearchParams(window.location.search);
                
                // Build registration URL with OAuth parameters
                const registerParams = new URLSearchParams();
                if (urlParams.get('client_id')) registerParams.append('client_id', urlParams.get('client_id'));
                if (urlParams.get('redirect_uri')) registerParams.append('redirect_uri', urlParams.get('redirect_uri'));
                if (urlParams.get('state')) registerParams.append('state', urlParams.get('state'));
                if (urlParams.get('scope')) registerParams.append('scope', urlParams.get('scope'));
                if (urlParams.get('response_type')) registerParams.append('response_type', urlParams.get('response_type'));
                if (urlParams.get('code_challenge')) registerParams.append('code_challenge', urlParams.get('code_challenge'));
                if (urlParams.get('code_challenge_method')) registerParams.append('code_challenge_method', urlParams.get('code_challenge_method'));
                
                if (registerParams.toString()) {
                    registerLink.href = '/oauth/register?' + registerParams.toString();
                }
            }
        });
        
        // Check WebAuthn support
        if (!window.PublicKeyCredential) {
            document.getElementById('webauthnSection').style.display = 'none';
            document.getElementById('webauthnWarning').classList.remove('hidden');
            document.querySelector('.or-divider').style.display = 'none';
        }
        
        // Sync username fields
        document.getElementById('webauthnUsername').addEventListener('input', function(e) {
            document.getElementById('username').value = e.target.value;
        });
        document.getElementById('username').addEventListener('input', function(e) {
            document.getElementById('webauthnUsername').value = e.target.value;
        });
        
        async function loginWithPasskey() {
            const username = document.getElementById('webauthnUsername').value;
            if (!username) {
                alert('Please enter your username');
                return;
            }
            
            const button = document.getElementById('webauthnButton');
            button.disabled = true;
            button.innerHTML = '<span class="loading"></span>Authenticating...';
            
            try {
                // Begin WebAuthn authentication
                console.log('Starting WebAuthn login for user:', username);
                const beginResponse = await fetch('/api/v1/auth/webauthn/login/begin', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ username })
                });
                
                console.log('Begin response status:', beginResponse.status);
                if (!beginResponse.ok) {
                    const error = await beginResponse.json();
                    console.error('Begin login error:', error);
                    throw new Error(error.error || 'Failed to start authentication');
                }
                
                const beginData = await beginResponse.json();
                console.log('Begin login data:', beginData);
                
                // Convert base64 to ArrayBuffer
                const publicKeyOptions = beginData.publicKey;
                
                // Check if challenge exists in publicKeyOptions
                if (!publicKeyOptions.challenge) {
                    console.error('Challenge missing from publicKeyOptions:', publicKeyOptions);
                    throw new Error('Invalid authentication data received from server');
                }
                
                publicKeyOptions.challenge = base64ToArrayBuffer(publicKeyOptions.challenge);
                
                if (publicKeyOptions.allowCredentials) {
                    publicKeyOptions.allowCredentials = publicKeyOptions.allowCredentials.map(cred => ({
                        ...cred,
                        id: base64ToArrayBuffer(cred.id)
                    }));
                }
                
                // Get credential
                const credential = await navigator.credentials.get({
                    publicKey: publicKeyOptions
                });
                
                // Prepare response
                const credentialResponse = {
                    id: credential.id,
                    rawId: arrayBufferToBase64(credential.rawId),
                    type: credential.type,
                    response: {
                        clientDataJSON: arrayBufferToBase64(credential.response.clientDataJSON),
                        authenticatorData: arrayBufferToBase64(credential.response.authenticatorData),
                        signature: arrayBufferToBase64(credential.response.signature),
                        userHandle: credential.response.userHandle ? 
                            arrayBufferToBase64(credential.response.userHandle) : null
                    }
                };
                
                // Submit OAuth form with WebAuthn response
                document.getElementById('hiddenUsername').value = username;
                document.getElementById('webauthnResponse').value = JSON.stringify({
                    challenge: beginData.challenge,
                    credential: credentialResponse
                });
                document.getElementById('oauthForm').submit();
                
            } catch (error) {
                console.error('WebAuthn error:', error);
                button.disabled = false;
                button.innerHTML = 'Sign in with Passkey';
                
                if (error.name === 'NotAllowedError') {
                    alert('Authentication was cancelled or timed out. Please try again.');
                } else {
                    alert('Authentication failed: ' + error.message);
                }
            }
        }
        
        function base64ToArrayBuffer(base64) {
            if (!base64) {
                throw new Error('base64ToArrayBuffer: input is null or undefined');
            }
            // Handle both base64 and base64url
            let base64String = base64.replace(/-/g, '+').replace(/_/g, '/');
            // Add padding if necessary
            while (base64String.length % 4) {
                base64String += '=';
            }
            const binaryString = window.atob(base64String);
            const bytes = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i);
            }
            return bytes.buffer;
        }
        
        function arrayBufferToBase64(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.byteLength; i++) {
                binary += String.fromCharCode(bytes[i]);
            }
            // Return base64url encoding (WebAuthn standard)
            return window.btoa(binary)
                .replace(/\+/g, '-')
                .replace(/\//g, '_')
                .replace(/=/g, '');
        }
    </script>
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
		if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
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

// handleRegister shows the registration page
func handleRegister(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodGet {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Extract OAuth parameters from query string
	params := request.QueryStringParameters
	oauthParams := OAuthParams{
		ClientID:            params["client_id"],
		RedirectURI:         params["redirect_uri"],
		State:               params["state"],
		Scope:               params["scope"],
		ResponseType:        params["response_type"],
		CodeChallenge:       params["code_challenge"],
		CodeChallengeMethod: params["code_challenge_method"],
	}

	return renderRegistrationPage("", oauthParams), nil
}

// OAuthParams holds OAuth parameters to preserve across registration
type OAuthParams struct {
	ClientID            string
	RedirectURI         string
	State               string
	Scope               string
	ResponseType        string
	CodeChallenge       string
	CodeChallengeMethod string
}

// renderRegistrationPage returns an HTML registration page
func renderRegistrationPage(errorMsg string, oauthParams OAuthParams) *events.APIGatewayV2HTTPResponse {
	// Template data
	type RegistrationPageData struct {
		Error       string
		OAuthParams OAuthParams
	}

	data := RegistrationPageData{
		Error:       errorMsg,
		OAuthParams: oauthParams,
	}

	registrationHTML := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Create Account - Lesser</title>
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
        .register-container {
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
            border-color: #2196F3;
        }
        button {
            width: 100%;
            padding: 0.75rem;
            background-color: #2196F3;
            color: white;
            border: none;
            border-radius: 4px;
            font-size: 1rem;
            font-weight: 500;
            cursor: pointer;
            margin-top: 1rem;
        }
        button:hover {
            background-color: #1976D2;
        }
        button:disabled {
            background-color: #ccc;
            cursor: not-allowed;
        }
        .error {
            background-color: #fee;
            color: #c33;
            padding: 0.75rem;
            border-radius: 4px;
            margin-bottom: 1rem;
            text-align: center;
        }
        .success {
            background-color: #e8f5e9;
            color: #2e7d32;
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
        .webauthn-icon {
            width: 64px;
            height: 64px;
            margin: 0 auto 1.5rem;
            background-color: #e3f2fd;
            border-radius: 50%;
            display: flex;
            align-items: center;
            justify-content: center;
        }
        .step {
            text-align: center;
            margin-bottom: 2rem;
        }
        .step-number {
            display: inline-block;
            width: 30px;
            height: 30px;
            background-color: #e0e0e0;
            color: #666;
            border-radius: 50%;
            line-height: 30px;
            font-weight: bold;
            margin-bottom: 0.5rem;
        }
        .step-number.active {
            background-color: #2196F3;
            color: white;
        }
        .step-number.completed {
            background-color: #4CAF50;
            color: white;
        }
        .hidden {
            display: none;
        }
        .loading {
            display: inline-block;
            width: 16px;
            height: 16px;
            border: 2px solid #f3f3f3;
            border-top: 2px solid #2196F3;
            border-radius: 50%;
            animation: spin 1s linear infinite;
            margin-right: 0.5rem;
        }
        @keyframes spin {
            0% { transform: rotate(0deg); }
            100% { transform: rotate(360deg); }
        }
    </style>
</head>
<body>
    <div class="register-container">
        <h1>Create Your Account</h1>
        
        <div id="errorMessage" class="error hidden"></div>
        <div id="successMessage" class="success hidden"></div>
        
        <!-- Hidden OAuth parameters -->
        <input type="hidden" id="oauth_client_id" value="{{.OAuthParams.ClientID}}">
        <input type="hidden" id="oauth_redirect_uri" value="{{.OAuthParams.RedirectURI}}">
        <input type="hidden" id="oauth_response_type" value="{{.OAuthParams.ResponseType}}">
        <input type="hidden" id="oauth_scope" value="{{.OAuthParams.Scope}}">
        <input type="hidden" id="oauth_state" value="{{.OAuthParams.State}}">
        <input type="hidden" id="oauth_code_challenge" value="{{.OAuthParams.CodeChallenge}}">
        <input type="hidden" id="oauth_code_challenge_method" value="{{.OAuthParams.CodeChallengeMethod}}">
        
        <!-- Step 1: Username -->
        <div id="step1" class="step-content">
            <div class="step">
                <div class="step-number active">1</div>
                <p>Choose your username</p>
            </div>
            
            <form id="usernameForm" onsubmit="createAccount(event)">
                <div class="form-group">
                    <label for="username">Username</label>
                    <input type="text" id="username" name="username" required autofocus
                           pattern="[a-zA-Z0-9_\\-]+" minlength="3" maxlength="30"
                           title="Username can only contain letters, numbers, underscore and hyphen">
                    <p style="margin-top: 0.5rem; font-size: 0.75rem; color: #666;">
                        Your handle will be @<span id="usernamePreview">username</span>@lesser.host
                    </p>
                </div>
                
                <button type="submit" id="createAccountBtn">
                    Create Account & Set Up Passkey
                </button>
            </form>
        </div>
        
        <!-- Step 2: WebAuthn Setup -->
        <div id="step2" class="step-content hidden">
            <div class="step">
                <div class="step-number completed">1</div>
                <div class="step-number active">2</div>
                <p>Set up your passkey</p>
            </div>
            
            <div class="webauthn-icon">
                <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#2196F3" stroke-width="2">
                    <path d="M12 11c0 3.517-1.009 6.799-2.753 9.571m-3.44-2.04l.054-.09A13.916 13.916 0 008 11a4 4 0 118 0c0 1.017-.07 2.019-.203 3m-2.118 6.844A21.88 21.88 0 0015.171 17m3.839 1.132c.645-2.266.99-4.659.99-7.132A8 8 0 008 4.07M3 15.364c.64-1.319 1-2.8 1-4.364 0-1.457.39-2.823 1.07-4"/>
                </svg>
            </div>
            
            <p style="text-align: center; margin-bottom: 1.5rem;">
                Follow your browser's prompts to create a passkey
            </p>
            
            <button onclick="setupPasskey()" id="setupPasskeyBtn">
                Set Up Passkey
            </button>
        </div>
        
        <p class="info">
            Already have an account? <a href="/oauth/authorize">Sign in</a>
        </p>
    </div>
    
    <script>
        let currentUsername = '';
        let accessToken = '';
        
        // Update username preview
        document.getElementById('username').addEventListener('input', function(e) {
            document.getElementById('usernamePreview').textContent = e.target.value || 'username';
        });
        
        function showError(message) {
            const errorDiv = document.getElementById('errorMessage');
            errorDiv.textContent = message;
            errorDiv.classList.remove('hidden');
            setTimeout(() => errorDiv.classList.add('hidden'), 5000);
        }
        
        function showSuccess(message) {
            const successDiv = document.getElementById('successMessage');
            successDiv.textContent = message;
            successDiv.classList.remove('hidden');
        }
        
        async function createAccount(e) {
            e.preventDefault();
            
            const username = document.getElementById('username').value;
            const button = document.getElementById('createAccountBtn');
            
            button.disabled = true;
            button.innerHTML = '<span class="loading"></span>Creating account...';
            
            try {
                // Create account
                const response = await fetch('/oauth/accounts', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        username: username,
                        locale: 'en',
                        agreement: true,
                        reason: 'OAuth registration'
                    })
                });
                
                if (!response.ok) {
                    const error = await response.json();
                    throw new Error(error.error || 'Failed to create account');
                }
                
                const accountData = await response.json();
                currentUsername = username;
                
                // Store the access token for WebAuthn registration
                if (!accountData.access_token) {
                    throw new Error('No access token received from server');
                }
                accessToken = accountData.access_token;
                
                // Move to step 2
                document.getElementById('step1').classList.add('hidden');
                document.getElementById('step2').classList.remove('hidden');
                showSuccess('Account created! Now set up your passkey.');
                
            } catch (error) {
                showError(error.message);
                button.disabled = false;
                button.innerHTML = 'Create Account & Set Up Passkey';
            }
        }
        
        async function setupPasskey() {
            const button = document.getElementById('setupPasskeyBtn');
            button.disabled = true;
            button.innerHTML = '<span class="loading"></span>Setting up passkey...';
            
            try {
                // Begin WebAuthn registration
                const beginResponse = await fetch('/api/v1/auth/webauthn/register/begin', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + accessToken
                    }
                });
                
                if (!beginResponse.ok) {
                    throw new Error('Failed to start passkey setup');
                }
                
                const beginData = await beginResponse.json();
                console.log('Begin registration data:', beginData);
                
                // Convert base64 to ArrayBuffer
                const publicKeyOptions = beginData.publicKey;
                
                // Check if challenge exists in publicKeyOptions
                if (!publicKeyOptions.challenge) {
                    console.error('Challenge missing from publicKeyOptions:', publicKeyOptions);
                    throw new Error('Invalid registration data received from server');
                }
                
                publicKeyOptions.challenge = base64ToArrayBuffer(publicKeyOptions.challenge);
                // The server sends user.id as base64, convert it to ArrayBuffer
                publicKeyOptions.user.id = base64ToArrayBuffer(publicKeyOptions.user.id);
                
                if (publicKeyOptions.excludeCredentials) {
                    publicKeyOptions.excludeCredentials = publicKeyOptions.excludeCredentials.map(cred => ({
                        ...cred,
                        id: base64ToArrayBuffer(cred.id)
                    }));
                }
                
                // Create credential
                const credential = await navigator.credentials.create({
                    publicKey: publicKeyOptions
                });
                
                // Prepare response
                const credentialResponse = {
                    id: credential.id,
                    rawId: arrayBufferToBase64(credential.rawId),
                    type: credential.type,
                    response: {
                        clientDataJSON: arrayBufferToBase64(credential.response.clientDataJSON),
                        attestationObject: arrayBufferToBase64(credential.response.attestationObject)
                    }
                };
                
                // Finish registration
                const finishResponse = await fetch('/api/v1/auth/webauthn/register/finish', {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Authorization': 'Bearer ' + accessToken
                    },
                    body: JSON.stringify({
                        challenge: beginData.challenge,
                        response: credentialResponse,
                        credential_name: 'Primary Passkey'
                    })
                });
                
                if (!finishResponse.ok) {
                    throw new Error('Failed to complete passkey setup');
                }
                
                showSuccess('Passkey created successfully! Redirecting to login...');
                
                // Redirect to login with OAuth parameters after 2 seconds
                setTimeout(() => {
                    const params = new URLSearchParams();
                    
                    // Get OAuth parameters from hidden fields or use defaults
                    const clientId = document.getElementById('oauth_client_id')?.value || 'lesser-web';
                    const redirectUri = document.getElementById('oauth_redirect_uri')?.value || window.location.origin + '/auth/callback';
                    const responseType = document.getElementById('oauth_response_type')?.value || 'code';
                    const scope = document.getElementById('oauth_scope')?.value || 'read write follow push';
                    const state = document.getElementById('oauth_state')?.value || Math.random().toString(36).substring(7);
                    const codeChallenge = document.getElementById('oauth_code_challenge')?.value || '';
                    const codeChallengeMethod = document.getElementById('oauth_code_challenge_method')?.value || '';
                    
                    params.append('response_type', responseType);
                    params.append('client_id', clientId);
                    params.append('redirect_uri', redirectUri);
                    params.append('scope', scope);
                    params.append('state', state);
                    
                    if (codeChallenge) {
                        params.append('code_challenge', codeChallenge);
                        params.append('code_challenge_method', codeChallengeMethod);
                    }
                    
                    window.location.href = '/oauth/authorize?' + params.toString();
                }, 2000);
                
            } catch (error) {
                showError(error.message);
                button.disabled = false;
                button.innerHTML = 'Set Up Passkey';
            }
        }
        
        function base64ToArrayBuffer(base64) {
            if (!base64) {
                throw new Error('base64ToArrayBuffer: input is null or undefined');
            }
            // Handle both base64 and base64url
            let base64String = base64.replace(/-/g, '+').replace(/_/g, '/');
            // Add padding if necessary
            while (base64String.length % 4) {
                base64String += '=';
            }
            const binaryString = window.atob(base64String);
            const bytes = new Uint8Array(binaryString.length);
            for (let i = 0; i < binaryString.length; i++) {
                bytes[i] = binaryString.charCodeAt(i);
            }
            return bytes.buffer;
        }
        
        function arrayBufferToBase64(buffer) {
            const bytes = new Uint8Array(buffer);
            let binary = '';
            for (let i = 0; i < bytes.byteLength; i++) {
                binary += String.fromCharCode(bytes[i]);
            }
            // Return base64url encoding (WebAuthn standard)
            return window.btoa(binary)
                .replace(/\+/g, '-')
                .replace(/\//g, '_')
                .replace(/=/g, '');
        }
        
        // Check WebAuthn support
        if (!window.PublicKeyCredential) {
            showError('WebAuthn is not supported in your browser. Please use a modern browser.');
            document.getElementById('createAccountBtn').disabled = true;
        }
    </script>
</body>
</html>`

	// Parse and execute template
	tmpl, err := template.New("registration").Parse(registrationHTML)
	if err != nil {
		logger.Error("failed to parse registration template", zap.Error(err))
		return common.InternalServerError(err)
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		logger.Error("failed to execute registration template", zap.Error(err))
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

// handleAccountCreation creates a new user account
func handleAccountCreation(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Parse request
	var req struct {
		Username  string `json:"username"`
		Locale    string `json:"locale"`
		Agreement bool   `json:"agreement"`
		Reason    string `json:"reason"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Validate username
	if req.Username == "" || len(req.Username) < 3 {
		return common.BadRequest(errors.New("username must be at least 3 characters")), nil
	}

	// Check if user already exists
	_, err := store.GetUser(ctx, req.Username)
	if err == nil {
		return common.BadRequest(errors.New("username already taken")), nil
	}

	// Create user
	user := &storage.User{
		Username:  req.Username,
		Approved:  true, // Auto-approve for now
		Suspended: false,
		Role:      "user",
		Locale:    req.Locale,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := store.CreateUser(ctx, user); err != nil {
		logger.Error("failed to create user", zap.Error(err))
		return common.InternalServerError(errors.New("failed to create account")), nil
	}

	// Create actor (ActivityPub profile)
	if err := createActorForUser(ctx, user.Username); err != nil {
		logger.Error("failed to create actor", zap.Error(err))
		// Don't fail the request, actor can be created later
	}

	// For OAuth registration flow, we need to return a temporary token
	// that allows WebAuthn registration
	tempToken, _, err := oauthSvc.GenerateTokens(user.Username, "oauth-register", []string{"write"})
	if err != nil {
		logger.Error("failed to generate temporary token", zap.Error(err))
		return common.InternalServerError(errors.New("account created but token generation failed")), nil
	}

	return common.OK(map[string]interface{}{
		"id":           user.Username,
		"username":     user.Username,
		"created_at":   user.CreatedAt,
		"access_token": tempToken, // Temporary token for WebAuthn setup
	}), nil
}

// createActorForUser creates an ActivityPub actor for a user
func createActorForUser(ctx context.Context, username string) error {
	// Generate RSA key pair
	privateKey, err := federation.GenerateRSAKeyPair(2048)
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	// Encode keys to PEM format
	publicKeyPEM, err := federation.EncodePublicKeyPEM(&privateKey.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to encode public key: %w", err)
	}

	privateKeyPEM, err := federation.EncodePrivateKeyPEM(privateKey)
	if err != nil {
		return fmt.Errorf("failed to encode private key: %w", err)
	}

	domain := cfg.Domain
	if domain == "" {
		domain = "lesser.host"
	}

	// Create actor
	actor := &activitypub.Actor{
		BaseObject: activitypub.BaseObject{
			Context: []interface{}{
				"https://www.w3.org/ns/activitystreams",
				"https://w3id.org/security/v1",
			},
			Type: "Person",
			ID:   fmt.Sprintf("https://%s/users/%s", domain, username),
		},
		Inbox:                     fmt.Sprintf("https://%s/users/%s/inbox", domain, username),
		Outbox:                    fmt.Sprintf("https://%s/users/%s/outbox", domain, username),
		Following:                 fmt.Sprintf("https://%s/users/%s/following", domain, username),
		Followers:                 fmt.Sprintf("https://%s/users/%s/followers", domain, username),
		Liked:                     fmt.Sprintf("https://%s/users/%s/liked", domain, username),
		PreferredUsername:         username,
		Name:                      username,
		Summary:                   "New Lesser user",
		URL:                       fmt.Sprintf("https://%s/@%s", domain, username),
		ManuallyApprovesFollowers: false,
		Discoverable:              true,
		PublicKey: &activitypub.PublicKey{
			ID:           fmt.Sprintf("https://%s/users/%s#main-key", domain, username),
			Owner:        fmt.Sprintf("https://%s/users/%s", domain, username),
			PublicKeyPem: string(publicKeyPEM),
		},
		Endpoints: &activitypub.Endpoints{
			SharedInbox: fmt.Sprintf("https://%s/inbox", domain),
		},
	}

	// Store actor using the storage interface
	if err := store.CreateActor(ctx, actor, string(privateKeyPEM)); err != nil {
		return fmt.Errorf("failed to store actor: %w", err)
	}

	return nil
}

// prepareWebAuthnResponse handles the common logic for preparing WebAuthn responses
// It extracts nested publicKey fields if necessary and returns a properly structured response
func prepareWebAuthnResponse(options interface{}, challenge string) (map[string]interface{}, error) {
	// The go-webauthn library might return nested structure
	// We need to structure the response properly for the client
	optionsJSON, err := json.Marshal(options)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal options: %w", err)
	}

	var optionsData map[string]interface{}
	if err := json.Unmarshal(optionsJSON, &optionsData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal options: %w", err)
	}

	// Check if it has a nested publicKey field
	var publicKeyData interface{}
	if pk, exists := optionsData["publicKey"]; exists {
		publicKeyData = pk
	} else {
		publicKeyData = optionsData
	}

	// Return the properly structured response
	return map[string]interface{}{
		"publicKey": publicKeyData,
		"challenge": challenge,
	}, nil
}

// handleWebAuthnLoginBegin starts the WebAuthn login process for OAuth flow
func handleWebAuthnLoginBegin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Parse request body
	var req struct {
		Username string `json:"username"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	if req.Username == "" {
		return common.BadRequest(errors.New("username required")), nil
	}

	if webAuthnSvc == nil {
		return common.InternalServerError(errors.New("WebAuthn service not available")), nil
	}

	// Begin WebAuthn login
	options, challenge, err := webAuthnSvc.BeginLogin(ctx, req.Username)
	if err != nil {
		logger.Error("failed to begin WebAuthn login",
			zap.String("username", req.Username),
			zap.Error(err))
		if err == auth.ErrUserHasNoCredentials {
			return common.BadRequest(errors.New("no passkeys registered for this user")), nil
		}
		return common.InternalServerError(errors.New("failed to begin login")), nil
	}

	logger.Info("WebAuthn login begin successful",
		zap.String("username", req.Username),
		zap.String("challenge", challenge))

	// Debug: Log the options structure
	optionsJSON, _ := json.Marshal(options)
	logger.Info("WebAuthn options structure",
		zap.String("options", string(optionsJSON)))

	// Prepare the response using the common helper
	response, err := prepareWebAuthnResponse(options, challenge)
	if err != nil {
		logger.Error("failed to prepare WebAuthn response", zap.Error(err))
		return common.InternalServerError(errors.New("failed to prepare response")), nil
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}, nil
}

// handleWebAuthnLoginFinish completes the WebAuthn login process for OAuth flow
func handleWebAuthnLoginFinish(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// This endpoint is not used in the OAuth flow
	// The OAuth flow handles WebAuthn completion in the main authorize endpoint
	return common.BadRequest(errors.New("use OAuth authorize endpoint for WebAuthn login")), nil
}

// handleWebAuthnRegisterBegin starts the WebAuthn registration process
func handleWebAuthnRegisterBegin(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Extract username from JWT token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(errors.New("authorization required")), nil
	}

	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(errors.New("invalid token")), nil
	}

	username := claims.Username
	if username == "" {
		return common.Unauthorized(errors.New("invalid token claims")), nil
	}

	// Begin WebAuthn registration
	options, challenge, err := webAuthnSvc.BeginRegistration(ctx, username)
	if err != nil {
		logger.Error("failed to begin WebAuthn registration", zap.Error(err))
		return common.InternalServerError(errors.New("failed to begin registration")), nil
	}

	// Prepare the response using the common helper
	response, err := prepareWebAuthnResponse(options, challenge)
	if err != nil {
		logger.Error("failed to prepare WebAuthn response", zap.Error(err))
		return common.InternalServerError(errors.New("failed to prepare response")), nil
	}

	body, _ := json.Marshal(response)
	return &events.APIGatewayV2HTTPResponse{
		StatusCode: http.StatusOK,
		Headers: map[string]string{
			"Content-Type":                 "application/json",
			"Access-Control-Allow-Origin":  "*",
			"Access-Control-Allow-Headers": "Content-Type, Authorization",
		},
		Body: string(body),
	}, nil
}

// handleWebAuthnRegisterFinish completes the WebAuthn registration process
func handleWebAuthnRegisterFinish(ctx context.Context, request events.APIGatewayV2HTTPRequest) (*events.APIGatewayV2HTTPResponse, error) {
	if request.RequestContext.HTTP.Method != http.MethodPost {
		return methodNotAllowed(request.RequestContext.HTTP.Method), nil
	}

	// Extract username from JWT token
	authHeader := request.Headers["Authorization"]
	if authHeader == "" {
		authHeader = request.Headers["authorization"]
	}

	token, err := auth.ExtractBearerToken(authHeader)
	if err != nil {
		return common.Unauthorized(errors.New("authorization required")), nil
	}

	claims, err := oauthSvc.ValidateAccessToken(token)
	if err != nil {
		return common.Unauthorized(errors.New("invalid token")), nil
	}

	username := claims.Username
	if username == "" {
		return common.Unauthorized(errors.New("invalid token claims")), nil
	}

	// Parse request body
	var req struct {
		Challenge      string          `json:"challenge"`
		Response       json.RawMessage `json:"response"`
		CredentialName string          `json:"credential_name"`
	}
	if err := common.ParseRequestBody([]byte(request.Body), &req); err != nil {
		return common.BadRequest(errors.New("invalid request body")), nil
	}

	// Finish registration
	err = webAuthnSvc.FinishRegistration(ctx, username, req.Challenge, req.Response, req.CredentialName)
	if err != nil {
		logger.Error("failed to finish WebAuthn registration", zap.Error(err))
		if err == auth.ErrChallengeNotFound {
			return common.BadRequest(errors.New("invalid or expired challenge")), nil
		}
		return common.InternalServerError(errors.New("failed to complete registration")), nil
	}

	return common.OK(map[string]string{
		"message": "passkey registered successfully",
	}), nil
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
