package enterprise

import (
	"context"
	"fmt"
	"time"

	"github.com/pay-theory/lift/pkg/lift"
)

// ComplianceValidator provides validation capabilities for compliance frameworks
type ComplianceValidator struct {
	rules      map[string]ValidationRule
	processors map[string]ValidationProcessor
}

// ValidationRule defines a compliance validation rule
type ValidationRule struct {
	Parameters  map[string]any     `json:"parameters"`
	ID          string             `json:"id"`
	Framework   string             `json:"framework"`
	Category    string             `json:"category"`
	Description string             `json:"description"`
	Severity    ValidationSeverity `json:"severity"`
}

// ValidationProcessor processes validation rules
type ValidationProcessor interface {
	Process(ctx context.Context, app *lift.App, rule ValidationRule) (*ValidationResult, error)
}

// ValidationResult represents the result of a validation
type ValidationResult struct {
	Timestamp time.Time        `json:"timestamp"`
	Details   map[string]any   `json:"details"`
	RuleID    string           `json:"rule_id"`
	Status    ValidationStatus `json:"status"`
	Message   string           `json:"message"`
}

// ValidationStatus is already defined in types.go

// NewComplianceValidator creates a new compliance validator
func NewComplianceValidator() *ComplianceValidator {
	return &ComplianceValidator{
		rules:      make(map[string]ValidationRule),
		processors: make(map[string]ValidationProcessor),
	}
}

// ComplianceReporter generates compliance reports
type ComplianceReporter struct {
	templates map[string]ReportTemplate
	exporters map[string]ReportExporter
}

// ReportTemplate, ReportSection, SectionType, ReportExporter, and ExportFormat are already defined in types.go

// NewComplianceReporter creates a new compliance reporter
func NewComplianceReporter() *ComplianceReporter {
	return &ComplianceReporter{
		templates: make(map[string]ReportTemplate),
		exporters: make(map[string]ReportExporter),
	}
}

// ContinuousMonitor provides continuous compliance monitoring
type ContinuousMonitor struct {
	monitors  map[string]InfrastructureComplianceMonitor
	scheduler *MonitorScheduler
	alerter   *ComplianceAlerter
	metrics   *MonitoringMetrics
}

// InfrastructureComplianceMonitor defines a compliance monitor (renamed to avoid conflict)
type InfrastructureComplianceMonitor struct {
	Thresholds map[string]float64 `json:"thresholds"`
	ID         string             `json:"id"`
	Framework  string             `json:"framework"`
	Controls   []string           `json:"controls"`
	Actions    []MonitorAction    `json:"actions"`
	Frequency  time.Duration      `json:"frequency"`
	Enabled    bool               `json:"enabled"`
}

// MonitorAction defines an action to take when a monitor triggers
type MonitorAction struct {
	Parameters map[string]any `json:"parameters"`
	Type       ActionType     `json:"type"`
}

// ActionType defines the type of monitor action
type ActionType string

const (
	AlertAction     ActionType = "alert"
	EmailAction     ActionType = "email"
	WebhookAction   ActionType = "webhook"
	RemediateAction ActionType = "remediate"
)

// MonitorScheduler schedules compliance monitoring
type MonitorScheduler struct {
	jobs map[string]*ScheduledJob
}

// ScheduledJob represents a scheduled monitoring job
type ScheduledJob struct {
	NextRun   time.Time     `json:"next_run"`
	LastRun   time.Time     `json:"last_run"`
	ID        string        `json:"id"`
	Monitor   string        `json:"monitor"`
	Status    JobStatus     `json:"status"`
	Frequency time.Duration `json:"frequency"`
}

// JobStatus represents the status of a scheduled job
type JobStatus string

const (
	JobScheduled JobStatus = "scheduled"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
)

// ComplianceAlerter sends compliance alerts
type ComplianceAlerter struct {
	channels map[string]InfrastructureAlertChannel
	rules    map[string]AlertRule
}

// InfrastructureAlertChannel defines an alert channel (renamed to avoid conflict)
type InfrastructureAlertChannel interface {
	Send(ctx context.Context, alert *ComplianceAlert) error
}

// MonitoringMetrics tracks monitoring metrics
type MonitoringMetrics struct {
	LastUpdate         time.Time                    `json:"last_update"`
	MetricsByFramework map[string]*FrameworkMetrics `json:"metrics_by_framework"`
	TotalMonitors      int64                        `json:"total_monitors"`
	ActiveMonitors     int64                        `json:"active_monitors"`
	AlertsGenerated    int64                        `json:"alerts_generated"`
	ComplianceScore    float64                      `json:"compliance_score"`
}

// FrameworkMetrics tracks metrics for a specific framework
type FrameworkMetrics struct {
	LastAssessment  time.Time `json:"last_assessment"`
	Framework       string    `json:"framework"`
	TotalControls   int64     `json:"total_controls"`
	PassingControls int64     `json:"passing_controls"`
	FailingControls int64     `json:"failing_controls"`
	ComplianceScore float64   `json:"compliance_score"`
}

// NewContinuousMonitor creates a new continuous monitor
func NewContinuousMonitor() *ContinuousMonitor {
	return &ContinuousMonitor{
		monitors:  make(map[string]InfrastructureComplianceMonitor),
		scheduler: &MonitorScheduler{jobs: make(map[string]*ScheduledJob)},
		alerter: &ComplianceAlerter{
			channels: make(map[string]InfrastructureAlertChannel),
			rules:    make(map[string]AlertRule),
		},
		metrics: &MonitoringMetrics{
			MetricsByFramework: make(map[string]*FrameworkMetrics),
		},
	}
}

// EvidenceStore stores and manages compliance evidence
type EvidenceStore struct {
	storage    EvidenceStorage
	retention  *InfrastructureRetentionPolicy
	encryption *EvidenceEncryption
}

// EvidenceStorage defines evidence storage interface
type EvidenceStorage interface {
	Store(ctx context.Context, evidence *Evidence) error
	Retrieve(ctx context.Context, id string) (*Evidence, error)
	List(ctx context.Context, filter EvidenceFilter) ([]*Evidence, error)
	Delete(ctx context.Context, id string) error
}

// InfrastructureEvidenceIndexer provides evidence indexing and search (renamed to avoid conflict)
type InfrastructureEvidenceIndexer interface {
	Index(ctx context.Context, evidence *Evidence) error
	Search(ctx context.Context, query EvidenceQuery) ([]*Evidence, error)
	Update(ctx context.Context, evidence *Evidence) error
	Remove(ctx context.Context, id string) error
}

// EvidenceFilter defines filters for evidence queries
type EvidenceFilter struct {
	StartTime time.Time      `json:"start_time"`
	EndTime   time.Time      `json:"end_time"`
	Metadata  map[string]any `json:"metadata"`
	Framework string         `json:"framework"`
	ControlID string         `json:"control_id"`
	Type      EvidenceType   `json:"type"`
	Tags      []string       `json:"tags"`
}

// EvidenceQuery defines a search query for evidence
type EvidenceQuery struct {
	Text      string         `json:"text"`
	SortBy    string         `json:"sort_by"`
	SortOrder string         `json:"sort_order"`
	Filters   EvidenceFilter `json:"filters"`
	Limit     int            `json:"limit"`
	Offset    int            `json:"offset"`
}

// InfrastructureRetentionPolicy defines evidence retention policies (renamed to avoid conflict)
type InfrastructureRetentionPolicy struct {
	FrameworkPolicies map[string]time.Duration         `json:"framework_policies"`
	TypePolicies      map[EvidenceType]time.Duration   `json:"type_policies"`
	CustomPolicies    map[string]CustomRetentionPolicy `json:"custom_policies"`
	DefaultRetention  time.Duration                    `json:"default_retention"`
}

// CustomRetentionPolicy defines custom retention rules
type CustomRetentionPolicy struct {
	Conditions []RetentionCondition `json:"conditions"`
	Actions    []RetentionAction    `json:"actions"`
	Retention  time.Duration        `json:"retention"`
}

// RetentionCondition defines a condition for retention
type RetentionCondition struct {
	Value    any    `json:"value"`
	Field    string `json:"field"`
	Operator string `json:"operator"`
}

// RetentionAction defines an action for retention
type RetentionAction struct {
	Parameters map[string]any `json:"parameters"`
	Type       string         `json:"type"`
}

// EvidenceEncryption handles evidence encryption
type EvidenceEncryption struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"key_id"`
	Enabled   bool   `json:"enabled"`
}

// NewEvidenceStore creates a new evidence store
func NewEvidenceStore() *EvidenceStore {
	return &EvidenceStore{
		retention: &InfrastructureRetentionPolicy{
			DefaultRetention:  365 * 24 * time.Hour, // 1 year default
			FrameworkPolicies: make(map[string]time.Duration),
			TypePolicies:      make(map[EvidenceType]time.Duration),
			CustomPolicies:    make(map[string]CustomRetentionPolicy),
		},
		encryption: &EvidenceEncryption{
			Algorithm: "AES-256-GCM",
			Enabled:   true,
		},
	}
}

// StoreReport stores a compliance report as evidence
func (e *EvidenceStore) StoreReport(ctx context.Context, report *ComplianceReport) error {
	// Convert report to evidence
	evidence := &Evidence{
		Type:        TestResultEvidence,
		Description: fmt.Sprintf("Compliance report for %s", report.Framework),
		Timestamp:   report.StartTime,
		Location:    fmt.Sprintf("/evidence/reports/%s_%d", report.Framework, report.StartTime.Unix()),
		Hash:        fmt.Sprintf("report_%d", report.StartTime.Unix()),
		Metadata: map[string]any{
			"framework":      report.Framework,
			"audit_period":   report.AuditPeriod.String(),
			"overall_status": report.OverallStatus,
			"control_count":  len(report.Controls),
		},
	}

	if e.storage != nil {
		return e.storage.Store(ctx, evidence)
	}

	// Default implementation - would typically store to database or file system
	return nil
}

// InfrastructureTest represents an infrastructure test
type InfrastructureTest struct {
	Config   map[string]any     `json:"config"`
	ID       string             `json:"id"`
	Name     string             `json:"name"`
	Type     string             `json:"type"`
	Target   string             `json:"target"`
	Severity ValidationSeverity `json:"severity"`
	Timeout  time.Duration      `json:"timeout"`
	Retries  int                `json:"retries"`
}
