package federation

import (
	"context"
	"errors"
	"testing"

	"github.com/equaltoai/lesser/pkg/monitoring"
	"github.com/equaltoai/lesser/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type hooksTrackerStub struct {
	trackDeliveryErr error
	trackInboundErr  error
	updateMetaErr    error

	attempts []*DeliveryAttempt
	inbound  []*InboundActivity
	metadata []*storage.InstanceMetadata

	analysis           *RelationshipAnalysis
	analysisErr        error
	recommendations    []*FederationRecommendation
	recommendationsErr error
}

func (h *hooksTrackerStub) TrackDeliveryAttempt(_ context.Context, attempt *DeliveryAttempt) error {
	h.attempts = append(h.attempts, attempt)
	return h.trackDeliveryErr
}

func (h *hooksTrackerStub) TrackInboundActivity(_ context.Context, activity *InboundActivity) error {
	h.inbound = append(h.inbound, activity)
	return h.trackInboundErr
}

func (h *hooksTrackerStub) UpdateInstanceMetadata(_ context.Context, metadata *storage.InstanceMetadata) error {
	h.metadata = append(h.metadata, metadata)
	return h.updateMetaErr
}

func (h *hooksTrackerStub) AnalyzeRelationshipStrength(_ context.Context, _ string, _ string) (*RelationshipAnalysis, error) {
	return h.analysis, h.analysisErr
}

func (h *hooksTrackerStub) GenerateRecommendations(_ context.Context, _ string) ([]*FederationRecommendation, error) {
	return h.recommendations, h.recommendationsErr
}

type hooksMonitorStub struct {
	calls []struct {
		domain  string
		op      string
		latency float64
		success bool
	}

	err error
}

func (m *hooksMonitorStub) RecordFederationPerformance(_ context.Context, domain string, operation string, latencyMs float64, success bool) error {
	m.calls = append(m.calls, struct {
		domain  string
		op      string
		latency float64
		success bool
	}{domain: domain, op: operation, latency: latencyMs, success: success})
	return m.err
}

func TestFederationHooks_OnOutboxDelivery_TracksAndMonitors(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	ctx := context.Background()
	delivery := &OutboxDelivery{
		SourceDomain:   "local.example",
		TargetDomain:   "remote.example",
		ActivityType:   "Create",
		Success:        true,
		ResponseTimeMs: 123,
	}

	require.NoError(t, hooks.OnOutboxDelivery(ctx, delivery))
	require.Len(t, tracker.attempts, 1)
	assert.Equal(t, "local.example", tracker.attempts[0].SourceDomain)
	assert.Equal(t, "remote.example", tracker.attempts[0].TargetDomain)
	assert.Equal(t, "Create", tracker.attempts[0].ActivityType)
	assert.True(t, tracker.attempts[0].Success)

	require.Len(t, monitor.calls, 1)
	assert.Equal(t, "remote.example", monitor.calls[0].domain)
	assert.Equal(t, "outbox_delivery", monitor.calls[0].op)
	assert.Equal(t, float64(123), monitor.calls[0].latency)
	assert.True(t, monitor.calls[0].success)
}

func TestFederationHooks_OnOutboxDelivery_TrackerErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{trackDeliveryErr: errors.New("boom")}
	monitor := &hooksMonitorStub{}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnOutboxDelivery(context.Background(), &OutboxDelivery{
		SourceDomain: "local.example",
		TargetDomain: "remote.example",
		ActivityType: "Create",
	}))
	assert.Empty(t, monitor.calls)
}

func TestFederationHooks_OnOutboxDelivery_MonitorErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{err: errors.New("boom")}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnOutboxDelivery(context.Background(), &OutboxDelivery{
		SourceDomain:   "local.example",
		TargetDomain:   "remote.example",
		ActivityType:   "Create",
		Success:        true,
		ResponseTimeMs: 1,
	}))
	assert.Len(t, monitor.calls, 1)
}

func TestFederationHooks_OnInboxReceive_TracksAndMonitors(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	ctx := context.Background()
	inbox := &InboxActivity{
		SourceDomain: "remote.example",
		TargetDomain: "local.example",
		ActivityType: "Create",
	}

	require.NoError(t, hooks.OnInboxReceive(ctx, inbox))
	require.Len(t, tracker.inbound, 1)
	assert.Equal(t, "remote.example", tracker.inbound[0].SourceDomain)
	assert.Equal(t, "local.example", tracker.inbound[0].TargetDomain)

	require.Len(t, monitor.calls, 1)
	assert.Equal(t, "remote.example", monitor.calls[0].domain)
	assert.Equal(t, "inbox_receive", monitor.calls[0].op)
	assert.Equal(t, float64(0), monitor.calls[0].latency)
	assert.True(t, monitor.calls[0].success)
}

func TestFederationHooks_OnInboxReceive_TrackerErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{trackInboundErr: errors.New("boom")}
	monitor := &hooksMonitorStub{}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnInboxReceive(context.Background(), &InboxActivity{
		SourceDomain: "remote.example",
		TargetDomain: "local.example",
		ActivityType: "Create",
	}))
	assert.Empty(t, monitor.calls)
}

func TestFederationHooks_OnInboxReceive_MonitorErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{err: errors.New("boom")}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnInboxReceive(context.Background(), &InboxActivity{
		SourceDomain: "remote.example",
		TargetDomain: "local.example",
		ActivityType: "Create",
	}))
	assert.Len(t, monitor.calls, 1)
}

func TestFederationHooks_OnInstanceDiscovery_UpdatesMetadata(t *testing.T) {
	tracker := &hooksTrackerStub{}
	hooks := &FederationHooks{
		tracker: tracker,
	}

	inst := &InstanceDiscovery{
		Domain:      "remote.example",
		DisplayName: "Remote",
		Description: "Desc",
		Software:    "soft",
		Version:     "1.0",
		UserCount:   10,
		StatusCount: 20,
	}

	require.NoError(t, hooks.OnInstanceDiscovery(context.Background(), inst))
	require.Len(t, tracker.metadata, 1)
	assert.Equal(t, "remote.example", tracker.metadata[0].Domain)
	assert.Equal(t, "Remote", tracker.metadata[0].DisplayName)
	assert.Equal(t, int64(10), tracker.metadata[0].UserCount)
}

func TestFederationHooks_OnConnectionError_TracksAndMonitors(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{}
	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	errEvent := &ConnectionError{
		SourceDomain: "local.example",
		TargetDomain: "remote.example",
		ActivityType: "Create",
		TimeoutMs:    500,
	}

	require.NoError(t, hooks.OnConnectionError(context.Background(), errEvent))
	require.Len(t, tracker.attempts, 1)
	assert.False(t, tracker.attempts[0].Success)
	assert.Equal(t, float64(500), tracker.attempts[0].ResponseTimeMs)

	require.Len(t, monitor.calls, 1)
	assert.Equal(t, "remote.example", monitor.calls[0].domain)
	assert.Equal(t, "connection_error", monitor.calls[0].op)
	assert.Equal(t, float64(500), monitor.calls[0].latency)
	assert.False(t, monitor.calls[0].success)
}

func TestFederationHooks_OnConnectionError_TrackerErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{trackDeliveryErr: errors.New("boom")}
	monitor := &hooksMonitorStub{}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnConnectionError(context.Background(), &ConnectionError{
		SourceDomain: "local.example",
		TargetDomain: "remote.example",
		ActivityType: "Create",
		TimeoutMs:    1,
	}))
	assert.Empty(t, monitor.calls)
}

func TestFederationHooks_OnConnectionError_MonitorErrorIsSwallowed(t *testing.T) {
	tracker := &hooksTrackerStub{}
	monitor := &hooksMonitorStub{err: errors.New("boom")}

	hooks := &FederationHooks{
		tracker: tracker,
		monitor: monitor,
	}

	require.NoError(t, hooks.OnConnectionError(context.Background(), &ConnectionError{
		SourceDomain: "local.example",
		TargetDomain: "remote.example",
		ActivityType: "Create",
		TimeoutMs:    1,
	}))
	assert.Len(t, monitor.calls, 1)
}

func TestFederationHooks_GettersDelegate(t *testing.T) {
	wantAnalysis := &RelationshipAnalysis{SourceDomain: "a", TargetDomain: "b"}
	wantRecs := []*FederationRecommendation{{
		Type:         "performance",
		Priority:     "high",
		TargetDomain: "x",
		Description:  "desc",
		Action:       "action",
	}}

	tracker := &hooksTrackerStub{
		analysis:        wantAnalysis,
		recommendations: wantRecs,
	}

	hooks := &FederationHooks{tracker: tracker}

	gotAnalysis, err := hooks.GetRelationshipAnalysis(context.Background(), "a", "b")
	require.NoError(t, err)
	assert.Equal(t, wantAnalysis, gotAnalysis)

	gotRecs, err := hooks.GetFederationRecommendations(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, wantRecs, gotRecs)
}

func TestNewFederationHooks_Smoke(t *testing.T) {
	// Smoke test the constructor without exercising the tracker internals.
	logger := zap.NewNop()
	var pm *monitoring.PerformanceMonitor
	hooks := NewFederationHooks(nil, pm, nil, logger)
	require.NotNil(t, hooks)
}
