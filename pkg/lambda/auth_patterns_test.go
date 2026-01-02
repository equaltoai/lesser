package lambda

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/equaltoai/lesser/pkg/common"
	"github.com/equaltoai/lesser/pkg/config"
	apperrors "github.com/equaltoai/lesser/pkg/errors"
	"github.com/golang-jwt/jwt/v5"
	liftPkg "github.com/pay-theory/lift/pkg/lift"
	"github.com/pay-theory/lift/pkg/lift/adapters"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newAuthTestContext(headers map[string]string) *liftPkg.Context {
	req := liftPkg.NewRequest(nil)
	req.Method = "GET"
	req.Path = "/resource"
	req.Headers = headers
	return liftPkg.NewContext(context.Background(), req)
}

func signTestTokenHMAC(t *testing.T, secret string, claims *auth.Claims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(t, err)
	return signed
}

func TestStandardAuthPattern_AuthenticateRequest_AllowsAnonymousWithoutToken(t *testing.T) {
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config:      &config.Config{JWTSecret: "secret"},
		Logger:      zap.NewNop(),
		Repos:       struct{}{},
		ServiceName: "test",
	})

	ctx := newAuthTestContext(nil)
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{AllowAnonymous: true})
	require.NoError(t, err)
	require.Nil(t, claims)
}

func TestStandardAuthPattern_AuthenticateRequest_RequiresToken(t *testing.T) {
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
	})

	ctx := newAuthTestContext(nil)
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{AllowAnonymous: false})
	require.Nil(t, claims)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeUnauthorized, appErr.Code)
	require.Equal(t, 401, appErr.HTTPStatusCode)
}

func TestStandardAuthPattern_AuthenticateRequest_InvalidToken(t *testing.T) {
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: "secret"},
		Logger: zap.NewNop(),
	})

	ctx := newAuthTestContext(map[string]string{
		"Authorization": "Bearer not-a-jwt",
	})
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{})
	require.Nil(t, claims)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeUnauthorized, appErr.Code)
	require.Equal(t, 401, appErr.HTTPStatusCode)
	require.NotNil(t, appErr.InternalError)
}

func TestStandardAuthPattern_AuthenticateRequest_InsufficientScope(t *testing.T) {
	secret := "secret"
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: secret},
		Logger: zap.NewNop(),
	})

	token := signTestTokenHMAC(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
		Scopes:           []string{auth.ScopeRead},
	})

	ctx := newAuthTestContext(map[string]string{
		"authorization": "Bearer " + token, // exercise lowercase header lookup
	})
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{RequiredScopes: []string{auth.ScopeWrite}})
	require.Nil(t, claims)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeForbidden, appErr.Code)
	require.Equal(t, 403, appErr.HTTPStatusCode)
}

func TestStandardAuthPattern_AuthenticateRequest_AdminRequired(t *testing.T) {
	secret := "secret"
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: secret},
		Logger: zap.NewNop(),
	})

	token := signTestTokenHMAC(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
		Scopes:           []string{auth.ScopeRead, auth.ScopeWrite},
	})

	ctx := newAuthTestContext(map[string]string{
		"Authorization": "Bearer " + token,
	})
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{RequireAdmin: true})
	require.Nil(t, claims)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeForbidden, appErr.Code)
	require.Equal(t, 403, appErr.HTTPStatusCode)
}

func TestStandardAuthPattern_AuthenticateRequest_SetsContextClaims(t *testing.T) {
	secret := "secret"
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: secret},
		Logger: zap.NewNop(),
	})

	token := signTestTokenHMAC(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
		Scopes:           []string{auth.ScopeRead, auth.ScopeWrite},
	})

	ctx := newAuthTestContext(map[string]string{
		"Authorization": "Bearer " + token,
	})
	claims, err := sap.AuthenticateRequest(ctx, AuthConfig{RequiredScopes: []string{auth.ScopeRead}})
	require.NoError(t, err)
	require.NotNil(t, claims)
	require.Equal(t, "alice", claims.Username)

	require.Equal(t, "alice", ctx.Get("user"))
	require.Equal(t, claims, ctx.Get("claims"))
}

func TestStandardAuthPattern_AuthenticateWithUsernameMatch_Mismatch(t *testing.T) {
	secret := "secret"
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: secret},
		Logger: zap.NewNop(),
	})

	token := signTestTokenHMAC(t, secret, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
		Scopes:           []string{auth.ScopeRead},
	})

	ctx := newAuthTestContext(map[string]string{
		"Authorization": "Bearer " + token,
	})
	claims, err := sap.AuthenticateWithUsernameMatch(ctx, "bob", AuthConfig{RequiredScopes: []string{auth.ScopeRead}})
	require.Nil(t, claims)

	appErr, ok := apperrors.AsAppError(err)
	require.True(t, ok)
	require.Equal(t, apperrors.CodeForbidden, appErr.Code)
	require.Equal(t, 403, appErr.HTTPStatusCode)
}

func TestStandardAuthPattern_validateJWTToken_UnexpectedSigningMethod(t *testing.T) {
	secret := "secret"
	sap := NewStandardAuthPattern(&common.LambdaContext{
		Config: &config.Config{JWTSecret: secret},
		Logger: zap.NewNop(),
	})

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, &auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
		Username:         "alice",
	})
	signed, err := token.SignedString(key)
	require.NoError(t, err)

	_, err = sap.validateJWTToken(signed)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unexpected signing method")
}

func TestHTTPSignatureAuth_ValidateHTTPSignature_EarlyErrors(t *testing.T) {
	hsa := NewHTTPSignatureAuth(nil, zap.NewNop())
	ctx := newAuthTestContext(nil)

	require.Error(t, hsa.ValidateHTTPSignature(ctx, []byte("{}")))

	ctx = newAuthTestContext(map[string]string{
		"Signature": `headers="(request-target) host date",signature="xyz"`,
	})
	require.Error(t, hsa.ValidateHTTPSignature(ctx, []byte("{}")))
}

type fakeSignatureVerifier struct {
	sawActorURL string
	sawMethod   string
	sawURL      string
	sawHeaders  http.Header
}

func (f *fakeSignatureVerifier) VerifySignature(_ context.Context, req *http.Request, actorURL string) error {
	f.sawActorURL = actorURL
	if req != nil {
		f.sawMethod = req.Method
		f.sawURL = req.URL.String()
		f.sawHeaders = req.Header.Clone()
	}
	return nil
}

func TestHTTPSignatureAuth_ValidateHTTPSignature_Success_CreatesHTTPRequest(t *testing.T) {
	verifier := &fakeSignatureVerifier{}
	hsa := NewHTTPSignatureAuth(verifier, zap.NewNop())

	adapterReq := &adapters.Request{
		Method: "POST",
		Path:   "/inbox",
		Headers: map[string]string{
			"Signature":         `keyId="https://example.com/users/alice#main-key",headers="(request-target) host date",signature="xyz"`,
			"X-Forwarded-Proto": "http",
			"Host":              "example.com",
			"Date":              "Wed, 21 Oct 2015 07:28:00 GMT",
			"Digest":            "SHA-256=abc",
			"Content-Type":      "application/activity+json",
		},
		QueryParams: map[string]string{
			"a": "b",
		},
	}

	req := liftPkg.NewRequest(adapterReq)
	ctx := liftPkg.NewContext(context.Background(), req)

	require.NoError(t, hsa.ValidateHTTPSignature(ctx, []byte(`{"ok":true}`)))
	require.Equal(t, "https://example.com/users/alice", verifier.sawActorURL)
	require.Equal(t, "POST", verifier.sawMethod)
	require.Contains(t, verifier.sawURL, "http://example.com/inbox")
	require.Contains(t, verifier.sawURL, "a=b")
	require.Contains(t, verifier.sawHeaders.Get("Content-Type"), "application/activity+json")
}

func TestHTTPSignatureAuth_CreateHTTPSignatureMiddleware_MissingBody(t *testing.T) {
	hsa := NewHTTPSignatureAuth(&fakeSignatureVerifier{}, zap.NewNop())
	mw := hsa.CreateHTTPSignatureMiddleware()

	req := liftPkg.NewRequest(nil)
	req.Method = "POST"
	req.Path = "/inbox"
	req.Headers = map[string]string{"Signature": `keyId="https://example.com/users/alice#main-key",signature="xyz"`}
	req.Body = nil
	ctx := liftPkg.NewContext(context.Background(), req)

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.False(t, nextCalled)
	require.Equal(t, 400, ctx.Response.StatusCode)
}

func TestHTTPSignatureAuth_CreateHTTPSignatureMiddleware_InvalidSignature(t *testing.T) {
	hsa := NewHTTPSignatureAuth(&fakeSignatureVerifier{}, zap.NewNop())
	mw := hsa.CreateHTTPSignatureMiddleware()

	req := liftPkg.NewRequest(nil)
	req.Method = "POST"
	req.Path = "/inbox"
	req.Headers = map[string]string{"Host": "example.com"}
	req.Body = []byte(`{"ok":true}`)
	ctx := liftPkg.NewContext(context.Background(), req)

	nextCalled := false
	handler := mw(liftPkg.HandlerFunc(func(*liftPkg.Context) error {
		nextCalled = true
		return nil
	}))

	require.NoError(t, handler.Handle(ctx))
	require.False(t, nextCalled)
	require.Equal(t, 401, ctx.Response.StatusCode)
}
