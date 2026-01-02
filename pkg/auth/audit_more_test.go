package auth

import (
	"context"
	"errors"
	"testing"

	storageinterfaces "github.com/equaltoai/lesser/pkg/storage/interfaces"
	"github.com/equaltoai/lesser/pkg/storage/repositories"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type auditProviderStub struct {
	audit *repositories.AuditRepository
}

func (a auditProviderStub) Account() *repositories.AccountRepository               { return nil }
func (a auditProviderStub) Actor() storageinterfaces.ActorRepository               { return nil }
func (a auditProviderStub) Activity() storageinterfaces.ActivityRepository         { return nil }
func (a auditProviderStub) Notification() storageinterfaces.NotificationRepository { return nil }
func (a auditProviderStub) Recovery() *repositories.RecoveryRepository             { return nil }
func (a auditProviderStub) Audit() *repositories.AuditRepository                   { return a.audit }

func TestNewAuditLogger_ConfigNil_AndGetAuditRepoBranches(t *testing.T) {
	t.Parallel()

	al := NewAuditLogger(nil, zap.NewNop(), nil)
	require.NotNil(t, al)
	require.NotNil(t, al.config)
	require.Nil(t, al.auditRepo)

	require.Nil(t, getAuditRepo(nil))
	require.Nil(t, getAuditRepo(auditProviderStub{audit: nil}))
	require.NotNil(t, getAuditRepo(auditProviderStub{audit: &repositories.AuditRepository{}}))
}

func TestAuditLogger_LogEvent_FallbackToStructuredLogging_AndAlerts(t *testing.T) {
	t.Parallel()

	al := &AuditLogger{
		logger: zap.NewNop(),
		config: &AuditConfig{
			Enabled:                  true,
			StoreToDB:                true,  // fallback path when auditRepo nil
			StoreToFile:              false, // keep deterministic
			StoreToSIEM:              false,
			HashIPAddresses:          false,
			RedactSensitive:          false,
			AlertOnAnomalousLocation: true,
		},
	}

	// High risk score alert.
	require.NoError(t, al.LogEvent(context.Background(), &AuditEvent{
		EventType:  AuditLoginFailed,
		Username:   "alice",
		IPAddress:  "192.0.2.10",
		Success:    false,
		RiskScore:  80,
		Metadata:   map[string]interface{}{"k": "v"},
		DeviceName: "device",
	}))

	// Brute-force alert + critical severity auto-selection.
	require.NoError(t, al.LogEvent(context.Background(), &AuditEvent{
		EventType: AuditBruteForceDetected,
		Username:  "alice",
		IPAddress: "192.0.2.10",
		Success:   false,
		Metadata:  map[string]interface{}{"k": "v"},
	}))

	// Anomalous location alert.
	require.NoError(t, al.LogEvent(context.Background(), &AuditEvent{
		EventType: AuditAnomalousLocation,
		Username:  "alice",
		IPAddress: "192.0.2.10",
		Success:   false,
		Country:   "US",
		City:      "New York",
		Metadata:  map[string]interface{}{"k": "v"},
	}))
}

func TestAuditLogger_LogOAuthToken_LogWebAuthn_AndQueryErrors(t *testing.T) {
	t.Parallel()

	repo := newInMemoryAuditRepo()
	al := &AuditLogger{
		auditRepo: repo,
		logger:    zap.NewNop(),
		config: &AuditConfig{
			Enabled:     true,
			StoreToDB:   true,
			StoreToFile: false,
			StoreToSIEM: false,
		},
	}

	al.LogOAuthToken(context.Background(), "client-1", "alice", "192.0.2.10", AuditOAuthTokenFailed, []string{"read"}, false, errors.New("boom"))
	require.Equal(t, "boom", repo.lastStore.failureReason)

	al.LogWebAuthn(context.Background(), "alice", "192.0.2.10", "ua", AuditWebAuthnLoginFailed, "cred-1", false, errors.New("nope"))
	require.Equal(t, "nope", repo.lastStore.failureReason)

	_, err := (&AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: true}}).GetIPAuditLogs(context.Background(), "192.0.2.10", 10)
	require.ErrorIs(t, err, ErrAuditRepositoryUnavailable)
	_, err = (&AuditLogger{logger: zap.NewNop(), config: &AuditConfig{Enabled: true}}).GetSessionAuditLogs(context.Background(), "sid-1")
	require.ErrorIs(t, err, ErrAuditRepositoryUnavailable)
}

func TestAuditLogger_logEventBestEffort_DoesNotPanicOnLogEventErrors(t *testing.T) {
	t.Parallel()

	al := &AuditLogger{
		logger: zap.NewNop(),
		config: &AuditConfig{
			Enabled:         true,
			StoreToDB:       false,
			StoreToFile:     true,
			StoreToSIEM:     false,
			RedactSensitive: false,
		},
	}

	al.logEventBestEffort(context.Background(), &AuditEvent{
		EventType: AuditLoginSuccess,
		Metadata:  map[string]interface{}{"bad": make(chan int)},
		Success:   true,
	})
}
