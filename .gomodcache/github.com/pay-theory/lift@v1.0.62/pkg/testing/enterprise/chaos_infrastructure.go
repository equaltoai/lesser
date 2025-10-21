package enterprise

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// NetworkFaultInjector injects network-related faults
type NetworkFaultInjector struct {
	config *NetworkFaultConfig
	active map[string]*ActiveFault
	mutex  sync.RWMutex
}

// NetworkFaultConfig configures network fault injection
type NetworkFaultConfig struct {
	Interface     string        `json:"interface"`
	DefaultDelay  time.Duration `json:"default_delay"`
	DefaultLoss   float64       `json:"default_loss"`
	DefaultJitter time.Duration `json:"default_jitter"`
	MaxBandwidth  int64         `json:"max_bandwidth"`
}

// ActiveFault represents an active fault injection
type ActiveFault struct {
	StartTime time.Time      `json:"start_time"`
	Config    map[string]any `json:"config"`
	Impact    *FaultImpact   `json:"impact"`
	ID        string         `json:"id"`
	Type      FaultType      `json:"type"`
	Duration  time.Duration  `json:"duration"`
}

// FaultImpact tracks the impact of fault injection
type FaultImpact struct {
	LastUpdated      time.Time      `json:"last_updated"`
	Metrics          map[string]any `json:"metrics"`
	AffectedRequests int64          `json:"affected_requests"`
	ErrorsIntroduced int64          `json:"errors_introduced"`
	LatencyAdded     time.Duration  `json:"latency_added"`
}

// ServiceFaultInjector injects service-level faults
type ServiceFaultInjector struct {
	config   *ServiceFaultConfig
	active   map[string]*ActiveFault
	handlers map[string]http.Handler
	mutex    sync.RWMutex
}

// ServiceFaultConfig configures service fault injection
type ServiceFaultConfig struct {
	ErrorRates      map[string]float64 `json:"error_rates"`
	LatencyProfiles map[string]any     `json:"latency_profiles"`
	ServiceName     string             `json:"service_name"`
	BaseURL         string             `json:"base_url"`
	DefaultTimeout  time.Duration      `json:"default_timeout"`
}

// ResourceFaultInjector injects resource-related faults
type ResourceFaultInjector struct {
	config *ResourceFaultConfig
	active map[string]*ActiveFault
	mutex  sync.RWMutex
}

// ResourceFaultConfig configures resource fault injection
type ResourceFaultConfig struct {
	CPULimits     map[string]float64 `json:"cpu_limits"`
	MemoryLimits  map[string]int64   `json:"memory_limits"`
	DiskLimits    map[string]int64   `json:"disk_limits"`
	NetworkLimits map[string]int64   `json:"network_limits"`
}

// ChaosGameDay represents a coordinated chaos engineering exercise
type ChaosGameDay struct {
	UpdatedAt    time.Time         `json:"updated_at"`
	CreatedAt    time.Time         `json:"created_at"`
	Results      *GameDayResults   `json:"results"`
	Schedule     *GameDaySchedule  `json:"schedule"`
	Metadata     map[string]any    `json:"metadata"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	ID           string            `json:"id"`
	Status       GameDayStatus     `json:"status"`
	Scenarios    []GameDayScenario `json:"scenarios"`
	Success      []SuccessCriteria `json:"success_criteria"`
	Objectives   []string          `json:"objectives"`
	Participants []Participant     `json:"participants"`
}

// GameDayScenario represents a scenario in a game day
type GameDayScenario struct {
	Metadata     map[string]any `json:"metadata"`
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Description  string         `json:"description"`
	Type         ScenarioType   `json:"type"`
	Experiments  []string       `json:"experiments"`
	Dependencies []string       `json:"dependencies"`
	Duration     time.Duration  `json:"duration"`
	Sequence     int            `json:"sequence"`
}

// ScenarioType defines types of game day scenarios
type ScenarioType string

const (
	DisasterRecoveryScenario ScenarioType = "disaster_recovery"
	SecurityIncidentScenario ScenarioType = "security_incident"
	PerformanceScenario      ScenarioType = "performance"
	ComplianceScenario       ScenarioType = "compliance"
	IntegrationScenario      ScenarioType = "integration"
)

// Participant represents a game day participant
type Participant struct {
	Metadata map[string]any  `json:"metadata"`
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Role     ParticipantRole `json:"role"`
	Team     string          `json:"team"`
	Contact  ContactInfo     `json:"contact"`
	Skills   []string        `json:"skills"`
}

// ParticipantRole defines participant roles
type ParticipantRole string

const (
	IncidentCommanderRole ParticipantRole = "incident_commander"
	TechnicalLeadRole     ParticipantRole = "technical_lead"
	ObserverRole          ParticipantRole = "observer"
	ParticipantRoleType   ParticipantRole = "participant"
	FacilitatorRole       ParticipantRole = "facilitator"
)

// ContactInfo represents contact information
type ContactInfo struct {
	Email  string `json:"email"`
	Phone  string `json:"phone"`
	Slack  string `json:"slack"`
	OnCall bool   `json:"on_call"`
}

// GameDaySchedule defines game day scheduling
type GameDaySchedule struct {
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	TimeZone   string        `json:"time_zone"`
	Breaks     []Break       `json:"breaks"`
	Milestones []Milestone   `json:"milestones"`
	Duration   time.Duration `json:"duration"`
}

// Break represents a scheduled break
type Break struct {
	StartTime time.Time     `json:"start_time"`
	Name      string        `json:"name"`
	Type      BreakType     `json:"type"`
	Duration  time.Duration `json:"duration"`
}

// BreakType defines types of breaks
type BreakType string

const (
	LunchBreak   BreakType = "lunch"
	CoffeeBreak  BreakType = "coffee"
	DebriefBreak BreakType = "debrief"
)

// Milestone represents a game day milestone
type Milestone struct {
	Name        string    `json:"name"`
	Time        time.Time `json:"time"`
	Description string    `json:"description"`
	Completed   bool      `json:"completed"`
}

// SuccessCriteria defines success criteria for game day
type SuccessCriteria struct {
	Target      any            `json:"target"`
	Actual      any            `json:"actual"`
	Metadata    map[string]any `json:"metadata"`
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        CriteriaType   `json:"type"`
	Weight      float64        `json:"weight"`
	Met         bool           `json:"met"`
}

// CriteriaType defines types of success criteria
type CriteriaType string

const (
	TimeCriteria        CriteriaType = "time"
	PerformanceCriteria CriteriaType = "performance"
	QualityCriteria     CriteriaType = "quality"
	LearningCriteria    CriteriaType = "learning"
)

// GameDayStatus defines game day status
type GameDayStatus string

const (
	PlannedGameDay    GameDayStatus = "planned"
	InProgressGameDay GameDayStatus = "in_progress"
	CompletedGameDay  GameDayStatus = "completed"
	CancelledGameDay  GameDayStatus = "canceled"
	PostponedGameDay  GameDayStatus = "postponed"
)

// GameDayResults contains game day results
type GameDayResults struct {
	StartTime      time.Time             `json:"start_time"`
	EndTime        time.Time             `json:"end_time"`
	Metrics        map[string]any        `json:"metrics"`
	Summary        string                `json:"summary"`
	Lessons        []Lesson              `json:"lessons"`
	ActionItems    []ActionItem          `json:"action_items"`
	Feedback       []ParticipantFeedback `json:"feedback"`
	Duration       time.Duration         `json:"duration"`
	ScenariosRun   int                   `json:"scenarios_run"`
	ExperimentsRun int                   `json:"experiments_run"`
	SuccessRate    float64               `json:"success_rate"`
}

// Lesson represents a lesson learned
type Lesson struct {
	Timestamp   time.Time      `json:"timestamp"`
	Metadata    map[string]any `json:"metadata"`
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Category    LessonCategory `json:"category"`
	Impact      LessonImpact   `json:"impact"`
	Source      string         `json:"source"`
}

// LessonCategory defines lesson categories
type LessonCategory string

const (
	TechnicalLesson     LessonCategory = "technical"
	ProcessLesson       LessonCategory = "process"
	CommunicationLesson LessonCategory = "communication"
	ToolingLesson       LessonCategory = "tooling"
)

// LessonImpact defines lesson impact
type LessonImpact string

const (
	HighImpact   LessonImpact = "high"
	MediumImpact LessonImpact = "medium"
	LowImpact    LessonImpact = "low"
)

// ActionItem represents an action item from game day
type ActionItem struct {
	DueDate     time.Time      `json:"due_date"`
	Metadata    map[string]any `json:"metadata"`
	ID          string         `json:"id"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Priority    ActionPriority `json:"priority"`
	Assignee    string         `json:"assignee"`
	Status      ActionStatus   `json:"status"`
	Category    ActionCategory `json:"category"`
}

// ActionPriority defines action item priority
type ActionPriority string

const (
	CriticalActionPriority ActionPriority = "critical"
	HighActionPriority     ActionPriority = "high"
	MediumActionPriority   ActionPriority = "medium"
	LowActionPriority      ActionPriority = "low"
)

// ActionStatus defines action item status
type ActionStatus string

const (
	OpenAction       ActionStatus = "open"
	InProgressAction ActionStatus = "in_progress"
	CompletedAction  ActionStatus = "completed"
	CancelledAction  ActionStatus = "canceled"
)

// ActionCategory defines action item category
type ActionCategory string

const (
	TechnicalAction     ActionCategory = "technical"
	ProcessAction       ActionCategory = "process"
	DocumentationAction ActionCategory = "documentation"
	TrainingAction      ActionCategory = "training"
)

// ParticipantFeedback represents feedback from participants
type ParticipantFeedback struct {
	Timestamp     time.Time      `json:"timestamp"`
	Metadata      map[string]any `json:"metadata"`
	ParticipantID string         `json:"participant_id"`
	Comments      string         `json:"comments"`
	Suggestions   []string       `json:"suggestions"`
	Rating        int            `json:"rating"`
	Anonymous     bool           `json:"anonymous"`
}

// ChaosEngineeringMetrics tracks chaos engineering metrics
type ChaosEngineeringMetrics struct {
	LastExperiment            time.Time      `json:"last_experiment"`
	LastUpdated               time.Time      `json:"last_updated"`
	Trends                    map[string]any `json:"trends"`
	ExperimentsRun            int            `json:"experiments_run"`
	ExperimentsSucceeded      int            `json:"experiments_succeeded"`
	ExperimentsFailed         int            `json:"experiments_failed"`
	SuccessRate               float64        `json:"success_rate"`
	AverageExperimentDuration time.Duration  `json:"average_experiment_duration"`
	FaultsInjected            int            `json:"faults_injected"`
	SystemsAffected           int            `json:"systems_affected"`
	ImprovementsFound         int            `json:"improvements_found"`
}

// BlastRadiusType defines types of blast radius
type BlastRadiusType string

const (
	InstanceBlastRadius BlastRadiusType = "instance"
	ServiceBlastRadius  BlastRadiusType = "service"
	RegionBlastRadius   BlastRadiusType = "region"
	ClusterBlastRadius  BlastRadiusType = "cluster"
)

// ConstraintType defines types of blast radius constraints
type ConstraintType string

const (
	TimeConstraint       ConstraintType = "time"
	PercentageConstraint ConstraintType = "percentage"
	CountConstraint      ConstraintType = "count"
	DependencyConstraint ConstraintType = "dependency"
)

// ChaosPolicy defines policies for chaos engineering
type ChaosPolicy struct {
	Scope       PolicyScope       `json:"scope"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]any    `json:"metadata"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Enforcement PolicyEnforcement `json:"enforcement"`
	Version     string            `json:"version"`
	Rules       []PolicyRule      `json:"rules"`
	Exceptions  []PolicyException `json:"exceptions"`
}

// PolicyRule defines a policy rule
type PolicyRule struct {
	Parameters map[string]any    `json:"parameters"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Type       PolicyRuleType    `json:"type"`
	Condition  string            `json:"condition"`
	Action     ChaosPolicyAction `json:"action"`
	Severity   PolicySeverity    `json:"severity"`
	Enabled    bool              `json:"enabled"`
}

// PolicyRuleType defines types of policy rules
type PolicyRuleType string

const (
	BlastRadiusRule PolicyRuleType = "blast_radius"
	TimeWindowRule  PolicyRuleType = "time_window"
	ApprovalRule    PolicyRuleType = "approval"
	SafetyRule      PolicyRuleType = "safety"
	ComplianceRule  PolicyRuleType = "compliance"
)

// ChaosPolicyAction defines policy actions
type ChaosPolicyAction string

const (
	AllowPolicyAction     ChaosPolicyAction = "allow"
	DenyPolicyAction      ChaosPolicyAction = "deny"
	RequireApprovalPolicy ChaosPolicyAction = "require_approval"
	LogPolicyAction       ChaosPolicyAction = "log"
	AlertPolicyAction     ChaosPolicyAction = "alert"
)

// PolicySeverity defines policy severity
type PolicySeverity string

const (
	InfoPolicySeverity     PolicySeverity = "info"
	WarningPolicySeverity  PolicySeverity = "warning"
	ErrorPolicySeverity    PolicySeverity = "error"
	CriticalPolicySeverity PolicySeverity = "critical"
)

// PolicyEnforcement defines policy enforcement
type PolicyEnforcement string

const (
	StrictEnforcementPolicy   PolicyEnforcement = "strict"
	LenientEnforcementPolicy  PolicyEnforcement = "lenient"
	AdvisoryEnforcementPolicy PolicyEnforcement = "advisory"
)

// PolicyScope defines policy scope
type PolicyScope struct {
	Filters  map[string]any `json:"filters"`
	Metadata map[string]any `json:"metadata"`
	Type     ChaosScopeType `json:"type"`
	Targets  []string       `json:"targets"`
}

// ChaosScopeType defines types of policy scope
type ChaosScopeType string

const (
	GlobalChaosScope      ChaosScopeType = "global"
	EnvironmentChaosScope ChaosScopeType = "environment"
	ServiceChaosScope     ChaosScopeType = "service"
	TeamChaosScope        ChaosScopeType = "team"
)

// PolicyException defines policy exceptions
type PolicyException struct {
	ExpiresAt  time.Time      `json:"expires_at"`
	Metadata   map[string]any `json:"metadata"`
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	Reason     string         `json:"reason"`
	Approver   string         `json:"approver"`
	Conditions []string       `json:"conditions"`
}

// Helper functions for fault injection status checking

// createFaultStatus creates a standard fault status response for an active fault
func createFaultStatus(activeFault *ActiveFault, impactKey string, impactValue any) FaultStatus {
	return FaultStatus{
		Active:    true,
		StartTime: activeFault.StartTime,
		Duration:  time.Since(activeFault.StartTime),
		Impact:    map[string]any{impactKey: impactValue},
		Metadata:  activeFault.Config,
	}
}

// checkFaultStatus checks if a fault is active and returns the appropriate status
func checkFaultStatus(active map[string]*ActiveFault, mutex *sync.RWMutex, faultID string, impactKey string, impactValueFunc func(*ActiveFault) any) (FaultStatus, error) {
	mutex.RLock()
	defer mutex.RUnlock()

	activeFault, exists := active[faultID]
	if !exists {
		return FaultStatus{Active: false}, nil
	}

	return createFaultStatus(activeFault, impactKey, impactValueFunc(activeFault)), nil
}

// Implementation methods for fault injectors

func NewNetworkFaultInjector(config *NetworkFaultConfig) *NetworkFaultInjector {
	return &NetworkFaultInjector{
		config: config,
		active: make(map[string]*ActiveFault),
	}
}

func (nfi *NetworkFaultInjector) Inject(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	nfi.mutex.Lock()
	defer nfi.mutex.Unlock()

	activeFault := &ActiveFault{
		ID:        fault.ID,
		Type:      fault.Type,
		StartTime: time.Now(),
		Duration:  fault.Duration,
		Config:    fault.Parameters,
		Impact: &FaultImpact{
			LastUpdated: time.Now(),
			Metrics:     make(map[string]any),
		},
	}

	// Simulate network fault injection based on type
	switch fault.Type {
	case LatencyFault:
		delay := nfi.config.DefaultDelay
		if d, ok := fault.Parameters["delay"].(time.Duration); ok {
			delay = d
		}
		activeFault.Config["injected_delay"] = delay

	case NetworkPartition:
		partition, ok := fault.Parameters["partition"].(string)
		if !ok {
			return fmt.Errorf("network partition parameter must be a string")
		}
		activeFault.Config["partition_type"] = partition

	case ErrorFault:
		errorRate := 0.1
		if rate, ok := fault.Parameters["error_rate"].(float64); ok {
			errorRate = rate
		}
		activeFault.Config["error_rate"] = errorRate
	}

	nfi.active[fault.ID] = activeFault
	return nil
}

func (nfi *NetworkFaultInjector) Remove(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	nfi.mutex.Lock()
	defer nfi.mutex.Unlock()

	delete(nfi.active, fault.ID)
	return nil
}

func (nfi *NetworkFaultInjector) Status(_ context.Context, fault FaultDefinition, _ ExperimentTarget) (FaultStatus, error) {
	return checkFaultStatus(nfi.active, &nfi.mutex, fault.ID, "requests_affected", func(af *ActiveFault) any {
		return af.Impact.AffectedRequests
	})
}

func NewServiceFaultInjector(config *ServiceFaultConfig) *ServiceFaultInjector {
	return &ServiceFaultInjector{
		config:   config,
		active:   make(map[string]*ActiveFault),
		handlers: make(map[string]http.Handler),
	}
}

func (sfi *ServiceFaultInjector) Inject(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	sfi.mutex.Lock()
	defer sfi.mutex.Unlock()

	activeFault := &ActiveFault{
		ID:        fault.ID,
		Type:      fault.Type,
		StartTime: time.Now(),
		Duration:  fault.Duration,
		Config:    fault.Parameters,
		Impact: &FaultImpact{
			LastUpdated: time.Now(),
			Metrics:     make(map[string]any),
		},
	}

	// Simulate service fault injection
	switch fault.Type {
	case ServiceUnavailable:
		activeFault.Config["status_code"] = 503
		activeFault.Config["message"] = "Service Temporarily Unavailable"

	case TimeoutFault:
		timeout := sfi.config.DefaultTimeout
		if t, ok := fault.Parameters["timeout"].(time.Duration); ok {
			timeout = t
		}
		activeFault.Config["timeout"] = timeout

	case ErrorFault:
		statusCode := 500
		if code, ok := fault.Parameters["status_code"].(int); ok {
			statusCode = code
		}
		activeFault.Config["status_code"] = statusCode
	}

	sfi.active[fault.ID] = activeFault
	return nil
}

func (sfi *ServiceFaultInjector) Remove(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	sfi.mutex.Lock()
	defer sfi.mutex.Unlock()

	delete(sfi.active, fault.ID)
	return nil
}

func (sfi *ServiceFaultInjector) Status(_ context.Context, fault FaultDefinition, _ ExperimentTarget) (FaultStatus, error) {
	return checkFaultStatus(sfi.active, &sfi.mutex, fault.ID, "errors_introduced", func(af *ActiveFault) any {
		return af.Impact.ErrorsIntroduced
	})
}

func NewResourceFaultInjector(config *ResourceFaultConfig) *ResourceFaultInjector {
	return &ResourceFaultInjector{
		config: config,
		active: make(map[string]*ActiveFault),
	}
}

func (rfi *ResourceFaultInjector) Inject(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	rfi.mutex.Lock()
	defer rfi.mutex.Unlock()

	activeFault := &ActiveFault{
		ID:        fault.ID,
		Type:      fault.Type,
		StartTime: time.Now(),
		Duration:  fault.Duration,
		Config:    fault.Parameters,
		Impact: &FaultImpact{
			LastUpdated: time.Now(),
			Metrics:     make(map[string]any),
		},
	}

	// Simulate resource fault injection
	if fault.Type == ResourceExhaustion {
		resourceType, ok := fault.Parameters["resource_type"].(string)
		if !ok {
			return fmt.Errorf("resource_type parameter must be a string")
		}
		percentage, ok := fault.Parameters["percentage"].(float64)
		if !ok {
			return fmt.Errorf("percentage parameter must be a float64")
		}

		activeFault.Config["resource_type"] = resourceType
		activeFault.Config["exhaustion_percentage"] = percentage

		// Simulate resource consumption
		switch resourceType {
		case "cpu":
			activeFault.Config["cpu_load"] = percentage
		case "memory":
			activeFault.Config["memory_usage"] = percentage
		case "disk":
			activeFault.Config["disk_usage"] = percentage
		}
	}

	rfi.active[fault.ID] = activeFault
	return nil
}

func (rfi *ResourceFaultInjector) Remove(_ context.Context, fault FaultDefinition, _ ExperimentTarget) error {
	rfi.mutex.Lock()
	defer rfi.mutex.Unlock()

	delete(rfi.active, fault.ID)
	return nil
}

func (rfi *ResourceFaultInjector) Status(_ context.Context, fault FaultDefinition, _ ExperimentTarget) (FaultStatus, error) {
	rfi.mutex.RLock()
	defer rfi.mutex.RUnlock()

	activeFault, exists := rfi.active[fault.ID]
	if !exists {
		return FaultStatus{Active: false}, nil
	}

	return FaultStatus{
		Active:    true,
		StartTime: activeFault.StartTime,
		Duration:  time.Since(activeFault.StartTime),
		Impact:    map[string]any{"resource_impact": activeFault.Config},
		Metadata:  activeFault.Config,
	}, nil
}

// Utility functions for chaos engineering

func ValidateExperimentSafety(experiment *ChaosExperiment, policy *ChaosPolicy) []string {
	validator := newExperimentSafetyValidator(experiment, policy)
	return validator.validate()
}

// experimentSafetyValidator validates chaos experiment safety
type experimentSafetyValidator struct {
	experiment *ChaosExperiment
	policy     *ChaosPolicy
	violations []string
}

// newExperimentSafetyValidator creates a new safety validator
func newExperimentSafetyValidator(experiment *ChaosExperiment, policy *ChaosPolicy) *experimentSafetyValidator {
	return &experimentSafetyValidator{
		experiment: experiment,
		policy:     policy,
		violations: []string{},
	}
}

// validate performs all safety validations
func (v *experimentSafetyValidator) validate() []string {
	for _, rule := range v.policy.Rules {
		if !rule.Enabled {
			continue
		}

		v.validateRule(rule)
	}
	return v.violations
}

// validateRule validates a single policy rule
func (v *experimentSafetyValidator) validateRule(rule PolicyRule) {
	switch rule.Type {
	case BlastRadiusRule:
		v.validateBlastRadius(rule)
	case TimeWindowRule:
		v.validateTimeWindow(rule)
	case ApprovalRule:
		v.validateApproval(rule)
	}
}

// validateBlastRadius validates blast radius constraints
func (v *experimentSafetyValidator) validateBlastRadius(rule PolicyRule) {
	maxPercentage, ok := rule.Parameters["max_percentage"].(float64)
	if !ok {
		return
	}

	if v.experiment.Target.Scope == ClusterScope && maxPercentage > 10.0 {
		v.violations = append(v.violations, "Experiment exceeds maximum blast radius percentage")
	}
}

// validateTimeWindow validates time window constraints
func (v *experimentSafetyValidator) validateTimeWindow(rule PolicyRule) {
	maxDuration, ok := rule.Parameters["max_duration"].(time.Duration)
	if !ok {
		return
	}

	if v.experiment.Duration > maxDuration {
		v.violations = append(v.violations, "Experiment duration exceeds policy limits")
	}
}

// validateApproval validates approval requirements
func (v *experimentSafetyValidator) validateApproval(_ PolicyRule) {
	for _, fault := range v.experiment.Faults {
		if fault.Severity == CriticalSeverity {
			v.violations = append(v.violations, "Critical severity experiments require approval")
			break // Only add once
		}
	}
}
