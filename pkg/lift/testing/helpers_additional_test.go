package testing

import (
	"strings"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/auth"
	"github.com/pay-theory/lift/pkg/lift"
	lifttesting "github.com/pay-theory/lift/pkg/testing"
	"github.com/stretchr/testify/require"
)

func TestCreateTestClaims_SetsExpectedScopes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		userType string
		wantAny  []string
	}{
		{name: "admin", userType: "admin", wantAny: []string{"admin:read", "admin:write"}},
		{name: "moderator", userType: "moderator", wantAny: []string{"admin:read"}},
		{name: "standard", userType: "standard", wantAny: []string{"read", "write"}},
		{name: "read-only", userType: "read-only", wantAny: []string{"read"}},
		{name: "bot", userType: "bot", wantAny: []string{"read", "write"}},
		{name: "unknown defaults", userType: "???", wantAny: []string{"read", "write"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := CreateTestClaims("alice", tt.userType)
			require.Equal(t, "alice", claims.Username)
			require.Equal(t, "test-client", claims.ClientID)
			require.True(t, strings.HasPrefix(claims.SessionID, "test-session-alice"))
			require.True(t, strings.HasPrefix(claims.DeviceID, "test-device-alice"))

			for _, scope := range tt.wantAny {
				require.Contains(t, claims.Scopes, scope)
			}
		})
	}
}

func TestCreateCustomClaims_UsesProvidedScopes(t *testing.T) {
	t.Parallel()

	claims := CreateCustomClaims("bob", []string{"x", "y"})
	require.Equal(t, "bob", claims.Username)
	require.Equal(t, []string{"x", "y"}, claims.Scopes)
}

func TestAuthenticatedAndTenantContexts_SetExpectedValues(t *testing.T) {
	t.Parallel()

	ctx := NewAuthenticatedContext("GET", "/path", "alice", "admin")
	require.IsType(t, &auth.EnhancedClaims{}, ctx.Get("claims"))
	require.Equal(t, "alice", ctx.Get("username"))
	require.NotEmpty(t, ctx.Get("session_id"))
	require.NotEmpty(t, ctx.Get("device_id"))

	tenantCtx := NewTenantContext("GET", "/path", "alice", "admin", "tenant-123")
	require.Equal(t, "tenant-123", tenantCtx.Get("tenant_id"))
	require.Equal(t, "tenant-123", tenantCtx.Header("X-Tenant-ID"))
}

func TestNewTestContextBuilders_InitializeRequestFields(t *testing.T) {
	t.Parallel()

	ctx := NewTestContextWithBody("POST", "/x", map[string]string{"k": "v"})
	require.Equal(t, "POST", ctx.Request.Method)
	require.Equal(t, "/x", ctx.Request.Path)
	require.Contains(t, string(ctx.Request.Body), `"k":"v"`)
	require.Equal(t, "application/json", ctx.Request.Headers["Content-Type"])

	ctx = NewTestContextWithHeaders("GET", "/y", map[string]string{"X-Test": "1"})
	require.Equal(t, "GET", ctx.Request.Method)
	require.Equal(t, "1", ctx.Request.Headers["X-Test"])
}

func TestNewTestContextWithBody_EncodeErrorProducesEmptyBody(t *testing.T) {
	t.Parallel()

	ctx := NewTestContextWithBody("POST", "/x", make(chan int))
	require.Equal(t, 0, len(ctx.Request.Body))
	require.Equal(t, "", ctx.Request.Headers["Content-Type"])
}

func TestTokenGenerators_ReturnExpectedFormats(t *testing.T) {
	t.Parallel()

	token := GenerateTestToken("alice", "admin")
	require.True(t, strings.HasPrefix(token, "test-token-alice-admin-"))

	expired := GenerateExpiredToken("alice")
	require.Equal(t, "expired-token-alice", expired)

	admin := GenerateAdminToken("root")
	require.True(t, strings.HasPrefix(admin, "test-token-root-admin-"))
}

func TestEventBuilders_CreateExpectedShapes(t *testing.T) {
	t.Parallel()

	sqs := CreateSQSEvent(`{"hello":"world"}`)
	require.Contains(t, sqs, "Records")

	stream := CreateDynamoDBStreamEvent("INSERT", map[string]interface{}{"foo": "bar"})
	require.Contains(t, stream, "Records")

	api := CreateAPIGatewayEvent("GET", "/x", "", nil)
	require.Equal(t, "GET", api["httpMethod"])
	require.Equal(t, "/x", api["path"])
}

func TestMeasureExecutionTime_ReturnsDuration(t *testing.T) {
	t.Parallel()

	d := MeasureExecutionTime(func() {
		for i := 0; i < 1000; i++ {
		}
	})
	require.GreaterOrEqual(t, d, time.Duration(0))
}

func TestTestUserFactories_CreateExpectedDefaults(t *testing.T) {
	t.Parallel()

	user := NewTestUser("alice")
	require.Equal(t, "user-alice", user.ID)
	require.Equal(t, "alice", user.Username)
	require.NotEmpty(t, user.Name)

	req := NewCreateUserRequest("bob")
	require.Equal(t, "bob", req.Username)
	require.NotEmpty(t, req.Name)
}

func TestTestEnvironment_DataAndTeardown(t *testing.T) {
	app := NewTestEnvironment()
	require.NotNil(t, app.App)
	require.NotNil(t, app.TestData)

	app.SetTestData("k", "v")
	require.Equal(t, "v", app.GetTestData("k"))

	called := false
	app.WithTeardown(func() { called = true })
	app.Cleanup()
	require.True(t, called)
}

func TestAssertHelpers_DoNotPanic(t *testing.T) {
	resp := &TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 200, Body: `{"k":"v"}`}}

	AssertSuccessResponse(t, resp)
	AssertJSONResponse(t, resp, nil)
	AssertErrorResponse(t, &TestResponse{TestResponse: &lifttesting.TestResponse{StatusCode: 400, Body: `{"error":"bad"}`}}, 400, "bad")
}

func TestAuthAssertionHelpers_CoverAllMethods(t *testing.T) {
	app := NewTestApp()

	register := func(method string) {
		_ = app.App().Handle(method, "/auth", func(ctx *lift.Context) error {
			if ctx.Header("Authorization") == "" {
				return ctx.Status(401).JSON(map[string]string{"error": "unauthorized"})
			}
			return ctx.Status(403).JSON(map[string]string{"error": "forbidden"})
		})
	}

	register(methodGET)
	register(methodPOST)
	register(methodPUT)
	register(methodDELETE)

	AssertAuthenticationRequired(t, app, methodGET, "/auth")
	AssertAuthenticationRequired(t, app, methodPOST, "/auth")
	AssertAuthenticationRequired(t, app, methodPUT, "/auth")
	AssertAuthenticationRequired(t, app, methodDELETE, "/auth")

	AssertScopeRequired(t, app, methodGET, "/auth", "admin:write")
	AssertScopeRequired(t, app, methodPOST, "/auth", "admin:write")
	AssertScopeRequired(t, app, methodPUT, "/auth", "admin:write")
	AssertScopeRequired(t, app, methodDELETE, "/auth", "admin:write")
}

func TestAssertExecutionTime_PassesWhenUnderBudget(t *testing.T) {
	t.Parallel()

	AssertExecutionTime(t, func() { _ = 1 + 1 }, 1*time.Second)
}
