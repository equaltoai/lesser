package disaster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

const defaultDRTestFrequency = 24 * time.Hour

var (
	errInvalidTestingFrequency = errors.New("disaster: testing schedule frequency must be greater than zero")
	errInvalidNotifyLeadTime   = errors.New("disaster: notify_before must be non-negative and less than testing frequency")
	errInvalidHealthInterval   = errors.New("disaster: health check interval must be greater than zero when enabled")
)

// DisasterRecoveryManager manages disaster recovery operations
type DisasterRecoveryManager struct {
	healthMonitor   *HealthMonitor
	dataSync        *DataSynchronizer
	notificationMgr *NotificationManager
	primaryRegion   string
	currentState    DRState
	backupRegions   []string
	failoverHistory []FailoverEvent
	metrics         DRMetrics
	config          DRConfig
	mu              sync.RWMutex
}

// DRConfig holds disaster recovery configuration
type DRConfig struct {
	Environment      string                `json:"environment"`
	FailoverStrategy FailoverStrategyType  `json:"failover_strategy"`
	PrimaryRegion    string                `json:"primary_region"`
	ApplicationName  string                `json:"application_name"`
	DataReplication  DataReplicationConfig `json:"data_replication"`
	TestingSchedule  TestingScheduleConfig `json:"testing_schedule"`
	BackupRegions    []string              `json:"backup_regions"`
	Notifications    NotificationConfig    `json:"notifications"`
	HealthCheck      HealthCheckConfig     `json:"health_check"`
	BackupRetention  BackupRetentionConfig `json:"backup_retention"`
	RPO              time.Duration         `json:"rpo"`
	RTO              time.Duration         `json:"rto"`
	AutoFailover     bool                  `json:"auto_failover"`
	AutoFailback     bool                  `json:"auto_failback"`
}

// FailoverStrategyType defines failover strategies
type FailoverStrategyType string

const (
	FailoverHotStandby   FailoverStrategyType = "hot_standby"
	FailoverWarmStandby  FailoverStrategyType = "warm_standby"
	FailoverColdStandby  FailoverStrategyType = "cold_standby"
	FailoverActiveActive FailoverStrategyType = "active_active"
)

// DRState represents the current disaster recovery state
type DRState struct {
	LastFailover    time.Time               `json:"last_failover"`
	LastFailback    time.Time               `json:"last_failback"`
	LastHealthCheck time.Time               `json:"last_health_check"`
	RegionHealth    map[string]RegionHealth `json:"region_health"`
	ActiveRegion    string                  `json:"active_region"`
	FailoverReason  string                  `json:"failover_reason,omitempty"`
	Status          DRStatus                `json:"status"`
	StandbyRegions  []string                `json:"standby_regions"`
	DataSyncStatus  DataSyncStatus          `json:"data_sync_status"`
}

// DRStatus represents disaster recovery status
type DRStatus string

const (
	DRStatusNormal      DRStatus = "normal"
	DRStatusDegraded    DRStatus = "degraded"
	DRStatusFailover    DRStatus = "failover"
	DRStatusFailback    DRStatus = "failback"
	DRStatusMaintenance DRStatus = "maintenance"
	DRStatusTesting     DRStatus = "testing"
)

// RegionHealth represents the health of a region
type RegionHealth struct {
	LastCheck           time.Time     `json:"last_check"`
	Region              string        `json:"region"`
	LastError           string        `json:"last_error,omitempty"`
	Status              HealthStatus  `json:"status"`
	ResponseTime        time.Duration `json:"response_time"`
	ErrorRate           float64       `json:"error_rate"`
	Availability        float64       `json:"availability"`
	ConsecutiveFailures int           `json:"consecutive_failures"`
}

// HealthStatus represents health status
type HealthStatus string

const (
	HealthStatusHealthy   HealthStatus = "healthy"
	HealthStatusDegraded  HealthStatus = "degraded"
	HealthStatusUnhealthy HealthStatus = "unhealthy"
	HealthStatusUnknown   HealthStatus = "unknown"
)

// DataSyncStatus represents data synchronization status
type DataSyncStatus struct {
	LastSync       time.Time       `json:"last_sync"`
	TablesInSync   map[string]bool `json:"tables_in_sync"`
	BucketsInSync  map[string]bool `json:"buckets_in_sync"`
	Status         SyncStatus      `json:"status"`
	SyncErrors     []SyncError     `json:"sync_errors"`
	ReplicationLag time.Duration   `json:"replication_lag"`
}

// SyncStatus represents synchronization status
type SyncStatus string

const (
	SyncStatusInSync    SyncStatus = "in_sync"
	SyncStatusSyncing   SyncStatus = "syncing"
	SyncStatusOutOfSync SyncStatus = "out_of_sync"
	SyncStatusError     SyncStatus = "error"
)

// SyncError represents a synchronization error
type SyncError struct {
	Timestamp time.Time `json:"timestamp"`
	Resource  string    `json:"resource"`
	Error     string    `json:"error"`
	Retries   int       `json:"retries"`
}

// FailoverEvent represents a failover event
type FailoverEvent struct {
	Timestamp    time.Time       `json:"timestamp"`
	RollbackPlan *RollbackPlan   `json:"rollback_plan,omitempty"`
	ID           string          `json:"id"`
	FromRegion   string          `json:"from_region"`
	ToRegion     string          `json:"to_region"`
	Reason       string          `json:"reason"`
	Type         FailoverType    `json:"type"`
	Trigger      FailoverTrigger `json:"trigger"`
	Status       FailoverStatus  `json:"status"`
	Steps        []FailoverStep  `json:"steps"`
	Impact       FailoverImpact  `json:"impact"`
	Duration     time.Duration   `json:"duration"`
}

// FailoverType represents the type of failover
type FailoverType string

const (
	FailoverTypeAutomatic FailoverType = "automatic"
	FailoverTypeManual    FailoverType = "manual"
	FailoverTypeTesting   FailoverType = "testing"
)

// FailoverTrigger represents what triggered the failover
type FailoverTrigger string

const (
	TriggerHealthCheck    FailoverTrigger = "health_check"
	TriggerManualRequest  FailoverTrigger = "manual_request"
	TriggerScheduledTest  FailoverTrigger = "scheduled_test"
	TriggerDataCorruption FailoverTrigger = "data_corruption"
	TriggerRegionOutage   FailoverTrigger = "region_outage"
)

// FailoverStatus represents failover status
type FailoverStatus string

const (
	FailoverStatusInProgress FailoverStatus = "in_progress"
	FailoverStatusCompleted  FailoverStatus = "completed"
	FailoverStatusFailed     FailoverStatus = "failed"
	FailoverStatusRolledBack FailoverStatus = "rolled_back"
)

// FailoverStep represents a step in the failover process
type FailoverStep struct {
	StartTime   time.Time     `json:"start_time"`
	EndTime     time.Time     `json:"end_time"`
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Status      StepStatus    `json:"status"`
	Error       string        `json:"error,omitempty"`
	Duration    time.Duration `json:"duration"`
	Retries     int           `json:"retries"`
}

// StepStatus represents step status
type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusFailed     StepStatus = "failed"
	StepStatusSkipped    StepStatus = "skipped"
)

// RollbackPlan represents a rollback plan
type RollbackPlan struct {
	ID          string         `json:"id"`
	Steps       []FailoverStep `json:"steps"`
	Conditions  []string       `json:"conditions"`
	TimeLimit   time.Duration  `json:"time_limit"`
	AutoExecute bool           `json:"auto_execute"`
}

// FailoverImpact represents the impact of a failover
type FailoverImpact struct {
	DowntimeDuration     time.Duration `json:"downtime_duration"`
	DataLoss             time.Duration `json:"data_loss"`
	AffectedUsers        int64         `json:"affected_users"`
	AffectedTransactions int64         `json:"affected_transactions"`
	EstimatedCost        float64       `json:"estimated_cost"`
}

// DRMetrics holds disaster recovery metrics
type DRMetrics struct {
	LastUpdated         time.Time     `json:"last_updated"`
	AverageFailoverTime time.Duration `json:"average_failover_time"`
	AverageDowntime     time.Duration `json:"average_downtime"`
	MTTR                time.Duration `json:"mttr"` // Mean Time To Recovery
	MTBF                time.Duration `json:"mtbf"` // Mean Time Between Failures
	TotalFailovers      int64         `json:"total_failovers"`
	SuccessfulFailovers int64         `json:"successful_failovers"`
	FailedFailovers     int64         `json:"failed_failovers"`
	Availability        float64       `json:"availability"`
}

// HealthCheckConfig defines health check configuration
type HealthCheckConfig struct {
	Endpoints         []HealthEndpoint    `json:"endpoints"`
	CustomChecks      []CustomHealthCheck `json:"custom_checks"`
	Interval          time.Duration       `json:"interval"`
	Timeout           time.Duration       `json:"timeout"`
	FailureThreshold  int                 `json:"failure_threshold"`
	RecoveryThreshold int                 `json:"recovery_threshold"`
	Enabled           bool                `json:"enabled"`
}

// HealthEndpoint defines a health check endpoint
type HealthEndpoint struct {
	Headers        map[string]string `json:"headers"`
	Name           string            `json:"name"`
	URL            string            `json:"url"`
	Method         string            `json:"method"`
	ExpectedBody   string            `json:"expected_body,omitempty"`
	Timeout        time.Duration     `json:"timeout"`
	ExpectedStatus int               `json:"expected_status"`
	Critical       bool              `json:"critical"`
}

// CustomHealthCheck defines a custom health check
type CustomHealthCheck struct {
	Config   map[string]any `json:"config"`
	Name     string         `json:"name"`
	Type     string         `json:"type"`
	Critical bool           `json:"critical"`
}

// DataReplicationConfig defines data replication configuration
type DataReplicationConfig struct {
	Strategy          ReplicationStrategy         `json:"strategy"`
	Tables            []TableReplicationConfig    `json:"tables"`
	S3Buckets         []S3ReplicationConfig       `json:"s3_buckets"`
	Databases         []DatabaseReplicationConfig `json:"databases"`
	MaxReplicationLag time.Duration               `json:"max_replication_lag"`
	Enabled           bool                        `json:"enabled"`
}

// ReplicationStrategy defines replication strategy
type ReplicationStrategy string

const (
	ReplicationAsynchronous ReplicationStrategy = "asynchronous"
	ReplicationSynchronous  ReplicationStrategy = "synchronous"
	ReplicationSemiSync     ReplicationStrategy = "semi_synchronous"
)

// TableReplicationConfig defines table replication
type TableReplicationConfig struct {
	TableName     string   `json:"table_name"`
	SourceRegion  string   `json:"source_region"`
	TargetRegions []string `json:"target_regions"`
	GlobalTables  bool     `json:"global_tables"`
	StreamEnabled bool     `json:"stream_enabled"`
	BackupEnabled bool     `json:"backup_enabled"`
}

// S3ReplicationConfig defines S3 replication
type S3ReplicationConfig struct {
	BucketName       string              `json:"bucket_name"`
	SourceRegion     string              `json:"source_region"`
	TargetRegions    []string            `json:"target_regions"`
	ReplicationRules []S3ReplicationRule `json:"replication_rules"`
}

// S3ReplicationRule defines S3 replication rule
type S3ReplicationRule struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Prefix   string `json:"prefix,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

// DatabaseReplicationConfig defines database replication
type DatabaseReplicationConfig struct {
	DatabaseName      string   `json:"database_name"`
	Type              string   `json:"type"` // RDS, Aurora, etc.
	SourceRegion      string   `json:"source_region"`
	TargetRegions     []string `json:"target_regions"`
	ReadReplicas      bool     `json:"read_replicas"`
	CrossRegionBackup bool     `json:"cross_region_backup"`
}

// NotificationConfig defines notification configuration
type NotificationConfig struct {
	Templates  map[string]string     `json:"templates"`
	Channels   []NotificationChannel `json:"channels"`
	Escalation EscalationConfig      `json:"escalation"`
	Enabled    bool                  `json:"enabled"`
}

// NotificationChannel defines a notification channel
type NotificationChannel struct {
	Type     string         `json:"type"` // email, sms, slack, pagerduty
	Config   map[string]any `json:"config"`
	Events   []string       `json:"events"`
	Severity []string       `json:"severity"`
}

// EscalationConfig defines escalation configuration
type EscalationConfig struct {
	Levels  []EscalationLevel `json:"levels"`
	Timeout time.Duration     `json:"timeout"`
	Enabled bool              `json:"enabled"`
}

// EscalationLevel defines an escalation level
type EscalationLevel struct {
	Recipients []string      `json:"recipients"`
	Channels   []string      `json:"channels"`
	Level      int           `json:"level"`
	Delay      time.Duration `json:"delay"`
}

// BackupRetentionConfig defines backup retention configuration
type BackupRetentionConfig struct {
	DailyRetention   int  `json:"daily_retention"`
	WeeklyRetention  int  `json:"weekly_retention"`
	MonthlyRetention int  `json:"monthly_retention"`
	YearlyRetention  int  `json:"yearly_retention"`
	Enabled          bool `json:"enabled"`
	CrossRegion      bool `json:"cross_region"`
	Encryption       bool `json:"encryption"`
}

// TestingScheduleConfig defines DR testing schedule
type TestingScheduleConfig struct {
	MaintenanceWindow string        `json:"maintenance_window"`
	TestTypes         []string      `json:"test_types"`
	Frequency         time.Duration `json:"frequency"`
	NotifyBefore      time.Duration `json:"notify_before"`
	Enabled           bool          `json:"enabled"`
}

// NewDisasterRecoveryManager creates a new disaster recovery manager
func NewDisasterRecoveryManager(config DRConfig) *DisasterRecoveryManager {
	config = normalizeDRConfig(config)

	drm := &DisasterRecoveryManager{
		config:          config,
		primaryRegion:   config.PrimaryRegion,
		backupRegions:   config.BackupRegions,
		failoverHistory: make([]FailoverEvent, 0),
		currentState: DRState{
			Status:         DRStatusNormal,
			ActiveRegion:   config.PrimaryRegion,
			StandbyRegions: config.BackupRegions,
			RegionHealth:   make(map[string]RegionHealth),
		},
		metrics: DRMetrics{
			LastUpdated: time.Now(),
		},
	}

	// Initialize health monitor
	drm.healthMonitor = NewHealthMonitor(config.HealthCheck)

	// Initialize data synchronizer
	drm.dataSync = NewDataSynchronizer(config.DataReplication)

	// Initialize notification manager
	drm.notificationMgr = NewNotificationManager(config.Notifications)

	return drm
}

func normalizeDRConfig(config DRConfig) DRConfig {
	if config.HealthCheck.Enabled && config.HealthCheck.Interval <= 0 {
		config.HealthCheck.Interval = defaultHealthCheckInterval
	}

	if config.TestingSchedule.Enabled && config.TestingSchedule.Frequency <= 0 {
		config.TestingSchedule.Frequency = defaultDRTestFrequency
	}

	return config
}

// StartMonitoring starts disaster recovery monitoring
func (drm *DisasterRecoveryManager) StartMonitoring(ctx context.Context) error {
	if err := drm.validateConfig(); err != nil {
		return err
	}

	drm.mu.Lock()
	defer drm.mu.Unlock()

	// Start health monitoring
	go drm.healthMonitor.Start(ctx, drm.handleHealthEvent)

	// Start data synchronization monitoring
	go drm.dataSync.StartMonitoring(ctx, drm.handleSyncEvent)

	// Start periodic DR testing if enabled
	if drm.config.TestingSchedule.Enabled {
		go drm.startPeriodicTesting(ctx)
	}

	return nil
}

func (drm *DisasterRecoveryManager) validateConfig() error {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	if drm.config.HealthCheck.Enabled && drm.config.HealthCheck.Interval <= 0 {
		return errInvalidHealthInterval
	}

	if drm.config.TestingSchedule.Enabled {
		if drm.config.TestingSchedule.Frequency <= 0 {
			return errInvalidTestingFrequency
		}
		if drm.config.TestingSchedule.NotifyBefore < 0 || drm.config.TestingSchedule.NotifyBefore >= drm.config.TestingSchedule.Frequency {
			return errInvalidNotifyLeadTime
		}
	}

	return nil
}

// TriggerFailover triggers a manual failover
func (drm *DisasterRecoveryManager) TriggerFailover(ctx context.Context, targetRegion, reason string) (*FailoverEvent, error) {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	if drm.currentState.Status == DRStatusFailover {
		return nil, fmt.Errorf("failover already in progress")
	}

	event := &FailoverEvent{
		ID:         fmt.Sprintf("failover-%d", time.Now().Unix()),
		Timestamp:  time.Now(),
		Type:       FailoverTypeManual,
		FromRegion: drm.currentState.ActiveRegion,
		ToRegion:   targetRegion,
		Reason:     reason,
		Trigger:    TriggerManualRequest,
		Status:     FailoverStatusInProgress,
		Steps:      drm.generateFailoverSteps(targetRegion),
	}

	return drm.executeFailover(ctx, event)
}

// executeFailover executes a failover event
func (drm *DisasterRecoveryManager) executeFailover(ctx context.Context, event *FailoverEvent) (*FailoverEvent, error) {
	startTime := time.Now()

	// Update state
	drm.currentState.Status = DRStatusFailover
	drm.currentState.FailoverReason = event.Reason

	// Notify stakeholders
	if err := drm.notificationMgr.SendNotification(ctx, "failover_started", map[string]any{
		"event":       event,
		"from_region": event.FromRegion,
		"to_region":   event.ToRegion,
		"reason":      event.Reason,
	}); err != nil {
		log.Printf("Failed to send failover started notification: %v", err)
	}

	// Execute failover steps
	for i, step := range event.Steps {
		stepStartTime := time.Now()
		event.Steps[i].Status = StepStatusInProgress
		event.Steps[i].StartTime = stepStartTime

		if err := drm.executeFailoverStep(ctx, &event.Steps[i]); err != nil {
			event.Steps[i].Status = StepStatusFailed
			event.Steps[i].Error = err.Error()
			event.Steps[i].EndTime = time.Now()
			event.Steps[i].Duration = time.Since(stepStartTime)

			// Handle step failure
			if drm.shouldRollback(event, &event.Steps[i]) {
				return drm.rollbackFailover(ctx, event)
			}

			event.Status = FailoverStatusFailed
			event.Duration = time.Since(startTime)
			drm.failoverHistory = append(drm.failoverHistory, *event)
			drm.updateMetrics(event)

			return event, fmt.Errorf("failover step %s failed: %w", step.Name, err)
		}

		event.Steps[i].Status = StepStatusCompleted
		event.Steps[i].EndTime = time.Now()
		event.Steps[i].Duration = time.Since(stepStartTime)
	}

	// Update state on successful failover
	drm.currentState.Status = DRStatusNormal
	drm.currentState.ActiveRegion = event.ToRegion
	drm.currentState.LastFailover = time.Now()
	drm.currentState.FailoverReason = ""

	// Update standby regions
	newStandbyRegions := make([]string, 0)
	for _, region := range drm.backupRegions {
		if region != event.ToRegion {
			newStandbyRegions = append(newStandbyRegions, region)
		}
	}
	newStandbyRegions = append(newStandbyRegions, event.FromRegion)
	drm.currentState.StandbyRegions = newStandbyRegions

	event.Status = FailoverStatusCompleted
	event.Duration = time.Since(startTime)

	// Calculate impact
	event.Impact = drm.calculateFailoverImpact(event)

	// Add to history
	drm.failoverHistory = append(drm.failoverHistory, *event)

	// Update metrics
	drm.updateMetrics(event)

	// Notify completion
	if err := drm.notificationMgr.SendNotification(ctx, "failover_completed", map[string]any{
		"event":    event,
		"duration": event.Duration,
		"impact":   event.Impact,
	}); err != nil {
		log.Printf("Failed to send failover completed notification: %v", err)
	}

	return event, nil
}

// generateFailoverSteps generates the steps for a failover
func (drm *DisasterRecoveryManager) generateFailoverSteps(targetRegion string) []FailoverStep {
	steps := []FailoverStep{
		{
			ID:          "validate_target",
			Name:        "Validate Target Region",
			Description: fmt.Sprintf("Validate that %s is ready for failover", targetRegion),
			Status:      StepStatusPending,
		},
		{
			ID:          "stop_traffic",
			Name:        "Stop Traffic to Primary",
			Description: "Stop routing traffic to the primary region",
			Status:      StepStatusPending,
		},
		{
			ID:          "sync_data",
			Name:        "Synchronize Data",
			Description: "Ensure all data is synchronized to the target region",
			Status:      StepStatusPending,
		},
		{
			ID:          "activate_standby",
			Name:        "Activate Standby Region",
			Description: fmt.Sprintf("Activate services in %s", targetRegion),
			Status:      StepStatusPending,
		},
		{
			ID:          "update_dns",
			Name:        "Update DNS Records",
			Description: "Update DNS to point to the new active region",
			Status:      StepStatusPending,
		},
		{
			ID:          "start_traffic",
			Name:        "Start Traffic to New Primary",
			Description: "Start routing traffic to the new primary region",
			Status:      StepStatusPending,
		},
		{
			ID:          "verify_health",
			Name:        "Verify Health",
			Description: "Verify that the new primary region is healthy",
			Status:      StepStatusPending,
		},
	}

	return steps
}

// executeFailoverStep executes a single failover step
func (drm *DisasterRecoveryManager) executeFailoverStep(ctx context.Context, step *FailoverStep) error {
	switch step.ID {
	case "validate_target":
		return drm.validateTargetRegion(ctx, step)
	case "stop_traffic":
		return drm.stopTrafficToPrimary(ctx, step)
	case "sync_data":
		return drm.synchronizeData(ctx, step)
	case "activate_standby":
		return drm.activateStandbyRegion(ctx, step)
	case "update_dns":
		return drm.updateDNSRecords(ctx, step)
	case "start_traffic":
		return drm.startTrafficToNewPrimary(ctx, step)
	case "verify_health":
		return drm.verifyHealth(ctx, step)
	default:
		return fmt.Errorf("unknown failover step: %s", step.ID)
	}
}

// validateTargetRegion validates the target region
func (drm *DisasterRecoveryManager) validateTargetRegion(_ context.Context, _ *FailoverStep) error {
	// Implementation would validate that the target region is ready
	time.Sleep(2 * time.Second) // Simulate validation time
	return nil
}

// stopTrafficToPrimary stops traffic to the primary region
func (drm *DisasterRecoveryManager) stopTrafficToPrimary(_ context.Context, _ *FailoverStep) error {
	// Implementation would stop traffic routing
	time.Sleep(1 * time.Second) // Simulate traffic stop time
	return nil
}

// synchronizeData synchronizes data to the target region
func (drm *DisasterRecoveryManager) synchronizeData(ctx context.Context, _ *FailoverStep) error {
	// Implementation would ensure data synchronization
	return drm.dataSync.ForceSynchronization(ctx)
}

// activateStandbyRegion activates the standby region
func (drm *DisasterRecoveryManager) activateStandbyRegion(_ context.Context, _ *FailoverStep) error {
	// Implementation would activate services in the standby region
	time.Sleep(5 * time.Second) // Simulate activation time
	return nil
}

// updateDNSRecords updates DNS records
func (drm *DisasterRecoveryManager) updateDNSRecords(_ context.Context, _ *FailoverStep) error {
	// Implementation would update DNS records
	time.Sleep(3 * time.Second) // Simulate DNS update time
	return nil
}

// startTrafficToNewPrimary starts traffic to the new primary
func (drm *DisasterRecoveryManager) startTrafficToNewPrimary(_ context.Context, _ *FailoverStep) error {
	// Implementation would start traffic routing
	time.Sleep(1 * time.Second) // Simulate traffic start time
	return nil
}

// verifyHealth verifies the health of the new primary
func (drm *DisasterRecoveryManager) verifyHealth(ctx context.Context, _ *FailoverStep) error {
	// Implementation would verify health
	return drm.healthMonitor.VerifyRegionHealth(ctx, drm.currentState.ActiveRegion)
}

// shouldRollback determines if a rollback should be performed
func (drm *DisasterRecoveryManager) shouldRollback(_ *FailoverEvent, failedStep *FailoverStep) bool {
	// Critical steps that should trigger rollback
	criticalSteps := map[string]bool{
		"activate_standby": true,
		"update_dns":       true,
		"start_traffic":    true,
	}

	return criticalSteps[failedStep.ID]
}

// rollbackFailover rolls back a failed failover
func (drm *DisasterRecoveryManager) rollbackFailover(ctx context.Context, event *FailoverEvent) (*FailoverEvent, error) {
	// Implementation would rollback the failover
	event.Status = FailoverStatusRolledBack
	drm.currentState.Status = DRStatusNormal

	if err := drm.notificationMgr.SendNotification(ctx, "failover_rolled_back", map[string]any{
		"event": event,
	}); err != nil {
		// Log notification error but don't fail rollback operation
		log.Printf("Failed to send failover rollback notification: %v", err)
	}

	return event, nil
}

// calculateFailoverImpact calculates the impact of a failover
func (drm *DisasterRecoveryManager) calculateFailoverImpact(event *FailoverEvent) FailoverImpact {
	return FailoverImpact{
		DowntimeDuration:     event.Duration,
		DataLoss:             0,     // Assuming no data loss in successful failover
		AffectedUsers:        1000,  // Example value
		AffectedTransactions: 500,   // Example value
		EstimatedCost:        100.0, // Example value
	}
}

// updateMetrics updates DR metrics
func (drm *DisasterRecoveryManager) updateMetrics(event *FailoverEvent) {
	drm.metrics.TotalFailovers++

	if event.Status == FailoverStatusCompleted {
		drm.metrics.SuccessfulFailovers++
	} else {
		drm.metrics.FailedFailovers++
	}

	// Update average failover time
	if drm.metrics.TotalFailovers > 0 {
		totalTime := drm.metrics.AverageFailoverTime * time.Duration(drm.metrics.TotalFailovers-1)
		drm.metrics.AverageFailoverTime = (totalTime + event.Duration) / time.Duration(drm.metrics.TotalFailovers)
	} else {
		drm.metrics.AverageFailoverTime = event.Duration
	}

	drm.metrics.LastUpdated = time.Now()
}

// handleHealthEvent handles health monitoring events
func (drm *DisasterRecoveryManager) handleHealthEvent(ctx context.Context, event HealthEvent) {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	// Update region health
	drm.currentState.RegionHealth[event.Region] = RegionHealth{
		Region:              event.Region,
		Status:              event.Status,
		LastCheck:           event.Timestamp,
		ResponseTime:        event.ResponseTime,
		ErrorRate:           event.ErrorRate,
		Availability:        event.Availability,
		ConsecutiveFailures: event.ConsecutiveFailures,
		LastError:           event.Error,
	}

	// Check if automatic failover should be triggered
	if drm.config.AutoFailover && event.Region == drm.currentState.ActiveRegion {
		if event.Status == HealthStatusUnhealthy && event.ConsecutiveFailures >= drm.config.HealthCheck.FailureThreshold {
			// Trigger automatic failover
			targetRegion := drm.selectBestBackupRegion()
			if targetRegion != "" {
				go func() {
					_, err := drm.TriggerFailover(ctx, targetRegion, fmt.Sprintf("Automatic failover due to health check failure: %s", event.Error))
					if err != nil {
						if notifyErr := drm.notificationMgr.SendNotification(ctx, "automatic_failover_failed", map[string]any{
							"error":       err.Error(),
							"from_region": event.Region,
							"to_region":   targetRegion,
						}); notifyErr != nil {
							// Log notification error but continue
							log.Printf("Failed to send recovery test notification: %v", notifyErr)
						}
					}
				}()
			}
		}
	}
}

// handleSyncEvent handles data synchronization events
func (drm *DisasterRecoveryManager) handleSyncEvent(ctx context.Context, event SyncEvent) {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	// Update data sync status
	drm.currentState.DataSyncStatus = DataSyncStatus{
		Status:         event.Status,
		LastSync:       event.Timestamp,
		ReplicationLag: event.ReplicationLag,
		SyncErrors:     event.Errors,
		TablesInSync:   event.TablesInSync,
		BucketsInSync:  event.BucketsInSync,
	}

	// Check if replication lag exceeds RPO
	if event.ReplicationLag > drm.config.RPO {
		if err := drm.notificationMgr.SendNotification(ctx, "rpo_violation", map[string]any{
			"rpo":             drm.config.RPO,
			"replication_lag": event.ReplicationLag,
		}); err != nil {
			// Log notification error but continue
			log.Printf("Failed to send notification: %v", err)
		}
	}
}

// selectBestBackupRegion selects the best backup region for failover
func (drm *DisasterRecoveryManager) selectBestBackupRegion() string {
	var bestRegion string
	var bestScore float64

	for _, region := range drm.currentState.StandbyRegions {
		health, exists := drm.currentState.RegionHealth[region]
		if !exists || health.Status != HealthStatusHealthy {
			continue
		}

		// Calculate score based on availability and response time
		score := health.Availability - (float64(health.ResponseTime.Milliseconds()) / 1000.0)

		if score > bestScore {
			bestScore = score
			bestRegion = region
		}
	}

	return bestRegion
}

// startPeriodicTesting starts periodic DR testing
func (drm *DisasterRecoveryManager) startPeriodicTesting(ctx context.Context) {
	freq := drm.getTestingFrequency()
	if freq <= 0 {
		return
	}

	ticker := time.NewTicker(freq)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			drm.performDRTest(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (drm *DisasterRecoveryManager) getTestingFrequency() time.Duration {
	drm.mu.RLock()
	defer drm.mu.RUnlock()
	return drm.config.TestingSchedule.Frequency
}

// performDRTest performs a disaster recovery test
func (drm *DisasterRecoveryManager) performDRTest(ctx context.Context) {
	drm.mu.RLock()
	notifyBefore := drm.config.TestingSchedule.NotifyBefore
	activeRegion := drm.currentState.ActiveRegion
	testRegion := drm.selectBestBackupRegion()
	drm.mu.RUnlock()

	scheduledTime := time.Now().Add(notifyBefore)

	if err := drm.notificationMgr.SendNotification(ctx, "dr_test_starting", map[string]any{
		"scheduled_time": scheduledTime,
	}); err != nil {
		// Log notification error but continue with test
		log.Printf("Failed to send DR test starting notification: %v", err)
	}

	if notifyBefore > 0 {
		timer := time.NewTimer(notifyBefore)
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
			return
		}
	}

	// Perform test failover
	if testRegion != "" {
		event := &FailoverEvent{
			ID:         fmt.Sprintf("test-failover-%d", time.Now().Unix()),
			Timestamp:  time.Now(),
			Type:       FailoverTypeTesting,
			FromRegion: activeRegion,
			ToRegion:   testRegion,
			Reason:     "Scheduled DR test",
			Trigger:    TriggerScheduledTest,
			Status:     FailoverStatusInProgress,
			Steps:      drm.generateFailoverSteps(testRegion),
		}

		// Execute test (non-destructive)
		drm.executeTestFailover(ctx, event)
	}
}

// executeTestFailover executes a test failover (non-destructive)
func (drm *DisasterRecoveryManager) executeTestFailover(ctx context.Context, event *FailoverEvent) {
	// Implementation would perform a non-destructive test
	// This would validate the failover process without actually switching traffic

	event.Status = FailoverStatusCompleted
	event.Duration = 5 * time.Minute // Example test duration

	drm.mu.Lock()
	drm.failoverHistory = append(drm.failoverHistory, *event)
	drm.mu.Unlock()

	if err := drm.notificationMgr.SendNotification(ctx, "dr_test_completed", map[string]any{
		"event":   event,
		"success": event.Status == FailoverStatusCompleted,
	}); err != nil {
		// Log notification error but continue
		log.Printf("Failed to send DR test completed notification: %v", err)
	}
}

// GetCurrentState returns the current DR state
func (drm *DisasterRecoveryManager) GetCurrentState() DRState {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	return drm.currentState
}

// GetFailoverHistory returns the failover history
func (drm *DisasterRecoveryManager) GetFailoverHistory() []FailoverEvent {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	history := make([]FailoverEvent, len(drm.failoverHistory))
	copy(history, drm.failoverHistory)
	return history
}

// GetMetrics returns DR metrics
func (drm *DisasterRecoveryManager) GetMetrics() DRMetrics {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	return drm.metrics
}

// ExportConfiguration exports the DR configuration
func (drm *DisasterRecoveryManager) ExportConfiguration() ([]byte, error) {
	drm.mu.RLock()
	defer drm.mu.RUnlock()

	return json.MarshalIndent(drm.config, "", "  ")
}

// ImportConfiguration imports DR configuration
func (drm *DisasterRecoveryManager) ImportConfiguration(data []byte) error {
	drm.mu.Lock()
	defer drm.mu.Unlock()

	var config DRConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	drm.config = config
	return nil
}
