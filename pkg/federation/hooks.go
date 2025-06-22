package federation

import (
	"context"
	"time"

	"github.com/aron23/lesser/pkg/monitoring"
	"github.com/aron23/lesser/pkg/storage"
)

// FederationHooks provides hooks into federation activities for tracking
type FederationHooks struct {
	tracker *RelationshipTracker
	monitor *monitoring.PerformanceMonitor
}

// NewFederationHooks creates a new federation hooks instance
func NewFederationHooks(store storage.Storage, monitor *monitoring.PerformanceMonitor) *FederationHooks {
	return &FederationHooks{
		tracker: NewRelationshipTracker(store),
		monitor: monitor,
	}
}

// OnOutboxDelivery is called when an activity is delivered to another instance
func (fh *FederationHooks) OnOutboxDelivery(ctx context.Context, delivery *OutboxDelivery) error {
	// Track the delivery attempt
	attempt := &DeliveryAttempt{
		SourceDomain:   delivery.SourceDomain,
		TargetDomain:   delivery.TargetDomain,
		ActivityType:   delivery.ActivityType,
		Success:        delivery.Success,
		ResponseTimeMs: delivery.ResponseTimeMs,
		Timestamp:      time.Now(),
	}

	// Track relationship data
	if err := fh.tracker.TrackDeliveryAttempt(ctx, attempt); err != nil {
		// Log error but don't fail the delivery
		return nil
	}

	// Record performance metrics if monitor is available
	if fh.monitor != nil {
		fh.monitor.RecordFederationPerformance(
			ctx,
			delivery.TargetDomain,
			"outbox_delivery",
			delivery.ResponseTimeMs,
			delivery.Success,
		)
	}

	return nil
}

// OnInboxReceive is called when an activity is received from another instance
func (fh *FederationHooks) OnInboxReceive(ctx context.Context, activity *InboxActivity) error {
	// Track the inbound activity
	inbound := &InboundActivity{
		SourceDomain: activity.SourceDomain,
		TargetDomain: activity.TargetDomain,
		ActivityType: activity.ActivityType,
		Timestamp:    time.Now(),
	}

	// Track relationship data
	if err := fh.tracker.TrackInboundActivity(ctx, inbound); err != nil {
		// Log error but don't fail the processing
		return nil
	}

	// Record performance metrics
	if fh.monitor != nil {
		fh.monitor.RecordFederationPerformance(
			ctx,
			activity.SourceDomain,
			"inbox_receive",
			0, // No response time for inbound
			true,
		)
	}

	return nil
}

// OnInstanceDiscovery is called when a new instance is discovered
func (fh *FederationHooks) OnInstanceDiscovery(ctx context.Context, instance *InstanceDiscovery) error {
	// Update instance metadata
	metadata := &storage.InstanceMetadata{
		Domain:      instance.Domain,
		DisplayName: instance.DisplayName,
		Description: instance.Description,
		Software:    instance.Software,
		Version:     instance.Version,
		UserCount:   instance.UserCount,
		StatusCount: instance.StatusCount,
		LastUpdated: time.Now(),
	}

	// Store or update the instance metadata
	return fh.tracker.storage.UpdateInstanceMetadata(ctx, metadata)
}

// OnConnectionError is called when there's an error connecting to an instance
func (fh *FederationHooks) OnConnectionError(ctx context.Context, connError *ConnectionError) error {
	// Track failed delivery attempt
	attempt := &DeliveryAttempt{
		SourceDomain:   connError.SourceDomain,
		TargetDomain:   connError.TargetDomain,
		ActivityType:   connError.ActivityType,
		Success:        false,
		ResponseTimeMs: connError.TimeoutMs,
		Timestamp:      time.Now(),
	}

	// Track the failed attempt
	if err := fh.tracker.TrackDeliveryAttempt(ctx, attempt); err != nil {
		return nil
	}

	// Record error metrics
	if fh.monitor != nil {
		fh.monitor.RecordFederationPerformance(
			ctx,
			connError.TargetDomain,
			"connection_error",
			connError.TimeoutMs,
			false,
		)
	}

	return nil
}

// GetRelationshipAnalysis provides analysis for a specific relationship
func (fh *FederationHooks) GetRelationshipAnalysis(ctx context.Context, sourceDomain, targetDomain string) (*RelationshipAnalysis, error) {
	return fh.tracker.AnalyzeRelationshipStrength(ctx, sourceDomain, targetDomain)
}

// GetFederationRecommendations provides recommendations for improving federation
func (fh *FederationHooks) GetFederationRecommendations(ctx context.Context, domain string) ([]*FederationRecommendation, error) {
	return fh.tracker.GenerateRecommendations(ctx, domain)
}

// Hook types for federation events

type OutboxDelivery struct {
	SourceDomain   string
	TargetDomain   string
	ActivityType   string
	ActivityID     string
	Success        bool
	ResponseTimeMs float64
	HTTPStatus     int
	ErrorMessage   string
}

type InboxActivity struct {
	SourceDomain string
	TargetDomain string
	ActivityType string
	ActivityID   string
	Valid        bool
	ProcessedAt  time.Time
}

type InstanceDiscovery struct {
	Domain      string
	DisplayName string
	Description string
	Software    string
	Version     string
	UserCount   int64
	StatusCount int64
	DiscoveredAt time.Time
}

type ConnectionError struct {
	SourceDomain string
	TargetDomain string
	ActivityType string
	ErrorType    string // timeout/dns/refused/invalid_cert/etc
	ErrorMessage string
	TimeoutMs    float64
	OccurredAt   time.Time
}