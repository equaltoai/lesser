package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/equaltoai/lesser/pkg/privacy"
	"github.com/equaltoai/lesser/pkg/storage/models"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type inMemoryAuditRepo struct {
	storeCalls int
	lastStore  struct {
		eventType     string
		severity      string
		username      string
		ipAddress     string
		userAgent     string
		deviceName    string
		sessionID     string
		requestID     string
		success       bool
		failureReason string
		metadata      map[string]interface{}
	}

	userLogs    map[string][]*models.AuthAuditLog
	ipLogs      map[string][]*models.AuthAuditLog
	sessionLogs map[string][]*models.AuthAuditLog
	security    map[string][]*models.AuthAuditLog

	storeErr error
}

func newInMemoryAuditRepo() *inMemoryAuditRepo {
	return &inMemoryAuditRepo{
		userLogs:    make(map[string][]*models.AuthAuditLog),
		ipLogs:      make(map[string][]*models.AuthAuditLog),
		sessionLogs: make(map[string][]*models.AuthAuditLog),
		security:    make(map[string][]*models.AuthAuditLog),
	}
}

func (r *inMemoryAuditRepo) StoreAuditEvent(_ context.Context, eventType, severity, username, _ string, ipAddress, userAgent, deviceName, sessionID, requestID string, success bool, failureReason string, metadata map[string]interface{}) error {
	r.storeCalls++
	r.lastStore.eventType = eventType
	r.lastStore.severity = severity
	r.lastStore.username = username
	r.lastStore.ipAddress = ipAddress
	r.lastStore.userAgent = userAgent
	r.lastStore.deviceName = deviceName
	r.lastStore.sessionID = sessionID
	r.lastStore.requestID = requestID
	r.lastStore.success = success
	r.lastStore.failureReason = failureReason
	r.lastStore.metadata = metadata
	return r.storeErr
}

func (r *inMemoryAuditRepo) GetUserAuditLogs(_ context.Context, username string, _ int, _, _ time.Time) ([]*models.AuthAuditLog, error) {
	return r.userLogs[username], nil
}

func (r *inMemoryAuditRepo) GetIPAuditLogs(_ context.Context, ipAddress string, _ int, _, _ time.Time) ([]*models.AuthAuditLog, error) {
	return r.ipLogs[ipAddress], nil
}

func (r *inMemoryAuditRepo) GetSessionAuditLogs(_ context.Context, sessionID string) ([]*models.AuthAuditLog, error) {
	return r.sessionLogs[sessionID], nil
}

func (r *inMemoryAuditRepo) GetSecurityEvents(_ context.Context, severity string, _, _ time.Time, _ int, _ string) ([]*models.AuthAuditLog, string, error) {
	return r.security[severity], "", nil
}

func TestAuditLogger_LogWebAuthn_LogSession_AndStoreFallbacks(t *testing.T) {
	t.Parallel()

	repo := newInMemoryAuditRepo()
	al := &AuditLogger{
		auditRepo: repo,
		logger:    zap.NewNop(),
		config: &AuditConfig{
			Enabled:         true,
			StoreToDB:       true,
			StoreToFile:     false,
			StoreToSIEM:     false,
			RedactSensitive: false,
		},
	}

	al.LogWebAuthn(context.Background(), "alice", "192.0.2.10", "ua", AuditWebAuthnLoginStarted, "cred-1", true, nil)
	require.Equal(t, 1, repo.storeCalls)
	require.Equal(t, string(AuditWebAuthnLoginStarted), repo.lastStore.eventType)
	require.Equal(t, "alice", repo.lastStore.username)
	require.Equal(t, "192.0.2.10", repo.lastStore.ipAddress)
	require.Equal(t, "ua", repo.lastStore.userAgent)
	require.True(t, repo.lastStore.success)
	require.Empty(t, repo.lastStore.failureReason)
	require.Equal(t, "cred-1", repo.lastStore.metadata["credential_id"])

	al.LogSession(context.Background(), "alice", "sid-1", "192.0.2.10", AuditSessionCreated)
	require.Equal(t, 2, repo.storeCalls)
	require.Equal(t, string(AuditSessionCreated), repo.lastStore.eventType)
	require.Equal(t, "sid-1", repo.lastStore.sessionID)
	require.Equal(t, "auth.session.created", repo.lastStore.metadata["session_operation"])

	// storeToFile error is best-effort (logged, not surfaced to callers).
	al.config.StoreToDB = false
	al.config.StoreToFile = true
	al.LogSession(context.Background(), "alice", "sid-2", "192.0.2.10", AuditSessionRefreshed)
}

func TestAuditLogger_SendToSIEM_AndMarshalFailures(t *testing.T) {
	t.Parallel()

	al := &AuditLogger{
		logger: zap.NewNop(),
		config: &AuditConfig{
			Enabled:     true,
			StoreToSIEM: true,
		},
	}

	// Missing endpoint is treated as a no-op.
	require.NoError(t, al.sendToSIEM(&AuditEvent{ID: "e"}))

	// Invalid endpoint with no host fails transmission.
	al.config.SIEMEndpoint = "http://"
	require.ErrorIs(t, al.sendToSIEM(&AuditEvent{ID: "e"}), ErrSIEMTransmission)

	// Invalid endpoint can also fail request creation.
	al.config.SIEMEndpoint = "http://[::1"
	require.ErrorIs(t, al.sendToSIEM(&AuditEvent{ID: "e"}), ErrSIEMRequestCreation)

	// Marshal failure surfaces ErrAuditEventMarshal.
	al.config.SIEMEndpoint = "https://example.invalid/siem"
	require.ErrorIs(t, al.sendToSIEM(&AuditEvent{Metadata: map[string]interface{}{"bad": make(chan int)}}), ErrAuditEventMarshal)

	// Non-2xx response returns ErrSIEMResponseError.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	al.config.SIEMEndpoint = server.URL
	require.ErrorIs(t, al.sendToSIEM(&AuditEvent{ID: "e"}), ErrSIEMResponseError)

	// Successful delivery uses auth header when configured.
	serverOK := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer api-key", r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(serverOK.Close)

	al.config.SIEMEndpoint = serverOK.URL
	al.config.SIEMAPIKey = "api-key"
	require.NoError(t, al.sendToSIEM(&AuditEvent{ID: "e"}))
}

func TestAuditLogger_PrivacyHashing_Redaction_AndQueryMethods(t *testing.T) {
	t.Parallel()

	masterKey, err := privacy.GenerateMasterKeyBase64()
	require.NoError(t, err)
	hasher, err := privacy.NewHasherFromMasterKey(masterKey)
	require.NoError(t, err)

	repo := newInMemoryAuditRepo()
	repo.userLogs["alice"] = []*models.AuthAuditLog{
		{
			ID:        "1",
			Timestamp: time.Now(),
			EventType: string(AuditLoginFailed),
			Severity:  string(SeverityError),
			Username:  "alice",
			IPAddress: "192.0.2.10",
			Metadata:  "{\"k\":\"v\"}",
		},
		{
			ID:        "2",
			Timestamp: time.Now(),
			EventType: string(AuditLoginFailed),
			Severity:  string(SeverityError),
			Username:  "alice",
			IPAddress: "192.0.2.10",
			Metadata:  "{bad json",
		},
	}
	repo.ipLogs["192.0.2.10"] = repo.userLogs["alice"]
	repo.sessionLogs["sid-1"] = repo.userLogs["alice"]
	repo.security[string(SeverityCritical)] = repo.userLogs["alice"][:1]

	al := &AuditLogger{
		auditRepo:     repo,
		logger:        zap.NewNop(),
		privacyHasher: hasher,
		config: &AuditConfig{
			Enabled:         true,
			StoreToDB:       true,
			RedactSensitive: true,
		},
	}

	event := &AuditEvent{
		EventType:  AuditLoginFailed,
		Username:   "alice",
		IPAddress:  "192.0.2.10",
		DeviceName: "Alice's Phone",
		Metadata: map[string]interface{}{
			"token":     "secret-token",
			"full_name": "Alice Example",
		},
		Success: false,
	}
	require.NoError(t, al.LogEvent(context.Background(), event))

	require.NotEqual(t, "alice", event.Username)
	require.NotEqual(t, "192.0.2.10", event.IPAddress)
	require.NotEqual(t, "Alice's Phone", event.DeviceName)
	require.Equal(t, "[REDACTED]", event.Metadata["token"])
	require.NotEqual(t, "Alice Example", event.Metadata["full_name"])
	require.Equal(t, len("alice"), event.Metadata["original_username_length"])

	events, err := al.GetUserAuditLogs(context.Background(), "alice", 10)
	require.NoError(t, err)
	require.Len(t, events, 2)

	events, err = al.GetIPAuditLogs(context.Background(), "192.0.2.10", 10)
	require.NoError(t, err)
	require.Len(t, events, 2)

	events, err = al.GetSessionAuditLogs(context.Background(), "sid-1")
	require.NoError(t, err)
	require.Len(t, events, 2)

	events, err = al.GetSecurityEvents(context.Background(), time.Now().Add(-time.Hour), time.Now(), []AuditSeverity{SeverityCritical})
	require.NoError(t, err)
	require.Len(t, events, 1)

	_, err = (&AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: true}}).GetUserAuditLogs(context.Background(), "alice", 10)
	require.ErrorIs(t, err, ErrAuditRepositoryUnavailable)

	// hashIPSecure and hashIPSimple helpers.
	noHasher := &AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: true}}
	require.Equal(t, "192.0.xxx.xxx", noHasher.hashIPSecure("192.0.2.10"))
	require.Equal(t, "xxx.xxx.xxx.xxx", hashIPSimple("not-an-ip"))

	// ensure convertToAuditEvents metadata parsing doesn't panic on invalid JSON.
	_, err = json.Marshal(events)
	require.NoError(t, err)
}

func TestAuditLogger_LogEvent_StoreToFileMarshalError_SurfaceError(t *testing.T) {
	t.Parallel()

	al := &AuditLogger{
		logger: zap.NewNop(),
		config: &AuditConfig{
			Enabled:      true,
			StoreToFile:  true,
			StoreToDB:    false,
			StoreToSIEM:  false,
			HashIPAddresses: false,
		},
	}

	err := al.LogEvent(context.Background(), &AuditEvent{
		EventType: AuditLoginSuccess,
		Metadata:  map[string]interface{}{"bad": make(chan int)},
		Success:   true,
	})
	require.ErrorIs(t, err, ErrAuditEventMarshal)
}
