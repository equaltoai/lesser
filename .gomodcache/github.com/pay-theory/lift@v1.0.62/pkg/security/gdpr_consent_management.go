package security

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"
)

// Error constants
var (
	ErrConsentNotFound = errors.New("consent not found")
	ErrInvalidEmail    = errors.New("invalid email address")
)

// GDPRConsentManager provides comprehensive GDPR consent management
type GDPRConsentManager struct {
	consentStore         ConsentStore
	dataSubjectRights    DataSubjectRightsHandler
	privacyAssessment    PrivacyImpactAssessment
	crossBorderValidator CrossBorderValidator
	auditLogger          GDPRAuditLogger
	config               GDPRConsentConfig
	mu                   sync.RWMutex
}

// GDPRConsentConfig configuration for GDPR consent management
type GDPRConsentConfig struct {
	// 8-byte aligned fields (maps, slices)
	DataRetentionPolicies    map[string]time.Duration `json:"data_retention_policies"`
	CrossBorderTransferRules []CrossBorderRule        `json:"cross_border_transfer_rules"`

	// 4-byte aligned fields (ints)
	ConsentRenewalDays      int `json:"consent_renewal_days"`
	BreachNotificationHours int `json:"breach_notification_hours"`
	ConsentExpiryDays       int `json:"consent_expiry_days"`
	DataRetentionDays       int `json:"data_retention_days"`
	RequestProcessingDays   int `json:"request_processing_days"`

	// Boolean flags (1 byte each)
	Enabled                  bool `json:"enabled"`
	AutomaticConsentRenewal  bool `json:"automatic_consent_renewal"`
	GranularConsentRequired  bool `json:"granular_consent_required"`
	ConsentWithdrawalEnabled bool `json:"consent_withdrawal_enabled"`
	DataPortabilityEnabled   bool `json:"data_portability_enabled"`
	RightToErasureEnabled    bool `json:"right_to_erasure_enabled"`
	PrivacyByDesignEnabled   bool `json:"privacy_by_design_enabled"`
	RequireExplicitConsent   bool `json:"require_explicit_consent"`
	RequireConsentProof      bool `json:"require_consent_proof"`
	ConsentProofRequired     bool `json:"consent_proof_required"`
}

// ConsentStore interface for storing and retrieving consent data
type ConsentStore interface {
	StoreConsent(ctx context.Context, consent *ConsentRecord) error
	GetConsent(ctx context.Context, dataSubjectID, purpose string) (*ConsentRecord, error)
	GetAllConsents(ctx context.Context, dataSubjectID string) ([]*ConsentRecord, error)
	UpdateConsent(ctx context.Context, consentID string, updates *ConsentUpdates) error
	WithdrawConsent(ctx context.Context, consentID string, withdrawal *ConsentWithdrawal) error
	GetExpiredConsents(ctx context.Context) ([]*ConsentRecord, error)
	GetConsentsForRenewal(ctx context.Context) ([]*ConsentRecord, error)
	// Additional methods needed by tests
	RecordConsent(ctx context.Context, consent *ConsentRecord) error
	ListConsents(ctx context.Context, dataSubjectID string) ([]*ConsentRecord, error)
	GetConsentHistory(ctx context.Context, consentID string) ([]*ConsentHistoryEntry, error)
	CleanupExpiredConsents(ctx context.Context) error
}

// DataSubjectRightsHandler interface for handling data subject rights
type DataSubjectRightsHandler interface {
	HandleAccessRequest(ctx context.Context, request *DataAccessRequest) (*DataAccessResponse, error)
	HandlePortabilityRequest(ctx context.Context, request *DataPortabilityRequest) (*DataPortabilityResponse, error)
	HandleErasureRequest(ctx context.Context, request *DataErasureRequest) (*DataErasureResponse, error)
	HandleRectificationRequest(ctx context.Context, request *DataRectificationRequest) (*DataRectificationResponse, error)
	HandleObjectionRequest(ctx context.Context, request *DataObjectionRequest) (*DataObjectionResponse, error)
	GetRequestStatus(ctx context.Context, requestID string) (*RequestStatus, error)
}

// PrivacyImpactAssessment interface for privacy impact assessments
type PrivacyImpactAssessment interface {
	ConductPIA(ctx context.Context, assessment *PIARequest) (*PIAResult, error)
	GetPIATemplate(processingType string) (*PIATemplate, error)
	ValidateDataProcessing(ctx context.Context, processing *DataProcessingActivity) (*ProcessingValidation, error)
	GetRiskAssessment(ctx context.Context, activityID string) (*RiskAssessment, error)
	// Additional methods needed by tests
	UpdatePIA(ctx context.Context, piaID string, updates *PIAUpdate) error
	GetPIA(ctx context.Context, piaID string) (*PIAResult, error)
	ListPIAs(ctx context.Context, filters *PIAFilters) ([]*PIAResult, error)
}

// CrossBorderValidator interface for cross-border data transfer validation
type CrossBorderValidator interface {
	ValidateTransfer(ctx context.Context, transfer *CrossBorderTransfer) (*TransferValidation, error)
	GetAdequacyDecisions() ([]AdequacyDecision, error)
	ValidateStandardContractualClauses(ctx context.Context, clauses *SCCValidation) (*SCCResult, error)
	ValidateBindingCorporateRules(ctx context.Context, bcr *BCRValidation) (*BCRResult, error)
}

// GDPRAuditLogger interface for GDPR-specific audit logging
type GDPRAuditLogger interface {
	LogConsentEvent(ctx context.Context, event *ConsentEvent) error
	LogDataSubjectRequest(ctx context.Context, request *DataSubjectRequestLog) error
	LogDataProcessingActivity(ctx context.Context, activity *DataProcessingLog) error
	LogCrossBorderTransfer(ctx context.Context, transfer *CrossBorderTransferLog) error
	LogPrivacyBreach(ctx context.Context, breach *PrivacyBreachLog) error
}

// ConsentRecord represents a complete consent record
type ConsentRecord struct {
	CreatedAt          time.Time        `json:"created_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	ConsentDate        time.Time        `json:"consent_date"`
	ExpiryDate         *time.Time       `json:"expiry_date,omitempty"`
	Timestamp          *time.Time       `json:"timestamp,omitempty"`
	WithdrawalDate     *time.Time       `json:"withdrawal_date,omitempty"`
	RenewalDate        *time.Time       `json:"renewal_date,omitempty"`
	Metadata           map[string]any   `json:"metadata"`
	ConsentProof       *ConsentProof    `json:"consent_proof,omitempty"`
	ID                 string           `json:"id"`
	WithdrawalMethod   string           `json:"withdrawal_method,omitempty"`
	UserAgent          string           `json:"user_agent,omitempty"`
	IPAddress          string           `json:"ip_address,omitempty"`
	Source             string           `json:"source,omitempty"`
	Purpose            string           `json:"purpose,omitempty"`
	DataSubjectID      string           `json:"data_subject_id"`
	DataSubjectEmail   string           `json:"data_subject_email"`
	ConsentVersion     string           `json:"consent_version"`
	ConsentMethod      string           `json:"consent_method"`
	LegalBasis         string           `json:"legal_basis"`
	Status             string           `json:"status"`
	Recipients         []DataRecipient  `json:"recipients"`
	ConsentScope       []ConsentPurpose `json:"consent_scope"`
	ProcessingPurposes []string         `json:"processing_purposes"`
	DataCategories     []string         `json:"data_categories"`
	RetentionPeriod    time.Duration    `json:"retention_period"`
	Granular           bool             `json:"granular"`
	Specific           bool             `json:"specific"`
	Informed           bool             `json:"informed"`
	Unambiguous        bool             `json:"unambiguous"`
	ConsentGiven       bool             `json:"consent_given"`
}

// ConsentPurpose represents a specific purpose for data processing
type ConsentPurpose struct {
	ConsentDate time.Time `json:"consent_date"`
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	LegalBasis  string    `json:"legal_basis"`
	Required    bool      `json:"required"`
	Consented   bool      `json:"consented"`
}

// DataRecipient represents a recipient of personal data
type DataRecipient struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Type       string   `json:"type"` // "controller", "processor", "third_party"
	Country    string   `json:"country"`
	Purposes   []string `json:"purposes"`
	Safeguards []string `json:"safeguards"`
}

// ConsentProof represents proof of consent
type ConsentProof struct {
	Timestamp time.Time      `json:"timestamp"`
	Metadata  map[string]any `json:"metadata"`
	Type      string         `json:"type"`
	Evidence  string         `json:"evidence"`
	IPAddress string         `json:"ip_address"`
	UserAgent string         `json:"user_agent"`
	Method    string         `json:"method"`
	Signature string         `json:"signature,omitempty"`
	Verified  bool           `json:"verified"`
}

// ConsentUpdates represents updates to consent
type ConsentUpdates struct {
	Timestamp       time.Time        `json:"timestamp,omitempty"`
	Metadata        map[string]any   `json:"metadata,omitempty"`
	RetentionPeriod *time.Duration   `json:"retention_period,omitempty"`
	ExpiryDate      *time.Time       `json:"expiry_date,omitempty"`
	UpdatedBy       string           `json:"updated_by"`
	UpdateReason    string           `json:"update_reason"`
	Reason          string           `json:"reason,omitempty"`
	ConsentScope    []ConsentPurpose `json:"consent_scope,omitempty"`
	Recipients      []DataRecipient  `json:"recipients,omitempty"`
	ConsentGiven    bool             `json:"consent_given,omitempty"`
}

// ConsentWithdrawal represents consent withdrawal
type ConsentWithdrawal struct {
	WithdrawalDate    time.Time      `json:"withdrawal_date"`
	Timestamp         time.Time      `json:"timestamp,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	WithdrawalMethod  string         `json:"withdrawal_method"`
	Reason            string         `json:"reason,omitempty"`
	RequestedBy       string         `json:"requested_by"`
	Method            string         `json:"method,omitempty"`
	WithdrawnPurposes []string       `json:"withdrawn_purposes,omitempty"`
	PartialWithdrawal bool           `json:"partial_withdrawal"`
	Verified          bool           `json:"verified"`
}

// DataAccessRequest represents a data subject access request
type DataAccessRequest struct {
	Timestamp     time.Time             `json:"timestamp,omitempty"`
	DueDate       time.Time             `json:"due_date"`
	RequestDate   time.Time             `json:"request_date"`
	Metadata      map[string]any        `json:"metadata"`
	Verification  *IdentityVerification `json:"verification"`
	ContactInfo   string                `json:"contact_info,omitempty"`
	Status        string                `json:"status"`
	UserID        string                `json:"user_id,omitempty"`
	Purpose       string                `json:"purpose,omitempty"`
	Region        string                `json:"region,omitempty"`
	RequestType   string                `json:"request_type"`
	Email         string                `json:"email"`
	DataSubjectID string                `json:"data_subject_id"`
	ID            string                `json:"id"`
	Scope         []string              `json:"scope"`
}

// DataAccessResponse represents the response to a data access request
type DataAccessResponse struct {
	ResponseDate   time.Time      `json:"response_date"`
	Data           map[string]any `json:"data"`
	Metadata       map[string]any `json:"metadata"`
	RequestID      string         `json:"request_id"`
	Format         string         `json:"format"`
	DeliveryMethod string         `json:"delivery_method"`
	Status         string         `json:"status,omitempty"`
	DataSources    []string       `json:"data_sources"`
	Encrypted      bool           `json:"encrypted"`
}

// DataPortabilityRequest represents a data portability request
type DataPortabilityRequest struct {
	TargetController string `json:"target_controller,omitempty"`
	Format           string `json:"format"`
	DataAccessRequest
	StructuredData bool `json:"structured_data"`
}

// DataPortabilityResponse represents the response to a data portability request
type DataPortabilityResponse struct {
	ResponseDate   time.Time      `json:"response_date"`
	Data           map[string]any `json:"data"`
	Metadata       map[string]any `json:"metadata"`
	RequestID      string         `json:"request_id"`
	Format         string         `json:"format"`
	TransferMethod string         `json:"transfer_method"`
	StructuredData bool           `json:"structured_data"`
}

// DataErasureRequest represents a data erasure request
type DataErasureRequest struct {
	DataAccessRequest
	Reason         string   `json:"reason"`
	ErasureScope   []string `json:"erasure_scope"`
	RetainForLegal bool     `json:"retain_for_legal"`
}

// DataErasureResponse represents the response to a data erasure request
type DataErasureResponse struct {
	ResponseDate       time.Time      `json:"response_date"`
	Metadata           map[string]any `json:"metadata"`
	RequestID          string         `json:"request_id"`
	RetentionReason    string         `json:"retention_reason,omitempty"`
	Status             string         `json:"status,omitempty"`
	ErasedData         []string       `json:"erased_data"`
	RetainedData       []string       `json:"retained_data"`
	DataDeleted        []string       `json:"data_deleted,omitempty"`
	DeletedCount       int            `json:"deleted_count,omitempty"`
	ThirdPartyNotified bool           `json:"third_party_notified"`
}

// DataRectificationRequest represents a data rectification request
type DataRectificationRequest struct {
	IncorrectData map[string]any `json:"incorrect_data"`
	CorrectedData map[string]any `json:"corrected_data"`
	DataAccessRequest
}

// DataRectificationResponse represents the response to a data rectification request
type DataRectificationResponse struct {
	ResponseDate       time.Time      `json:"response_date"`
	RectifiedData      map[string]any `json:"rectified_data"`
	Metadata           map[string]any `json:"metadata"`
	RequestID          string         `json:"request_id"`
	ThirdPartyNotified bool           `json:"third_party_notified"`
}

// DataObjectionRequest represents a data processing objection request
type DataObjectionRequest struct {
	DataAccessRequest
	ObjectionReason    string   `json:"objection_reason"`
	LegalGrounds       string   `json:"legal_grounds"`
	ProcessingPurposes []string `json:"processing_purposes"`
}

// DataObjectionResponse represents the response to a data objection request
type DataObjectionResponse struct {
	ResponseDate        time.Time      `json:"response_date"`
	Metadata            map[string]any `json:"metadata"`
	RequestID           string         `json:"request_id"`
	LegalJustification  string         `json:"legal_justification,omitempty"`
	ContinuedProcessing []string       `json:"continued_processing,omitempty"`
	ProcessingStopped   bool           `json:"processing_stopped"`
}

// IdentityVerification represents identity verification for data subject requests
type IdentityVerification struct {
	VerifiedDate time.Time      `json:"verified_date"`
	Metadata     map[string]any `json:"metadata"`
	Method       string         `json:"method"`
	VerifiedBy   string         `json:"verified_by"`
	Evidence     []string       `json:"evidence"`
	Verified     bool           `json:"verified"`
}

// RequestStatus represents the status of a data subject request
type RequestStatus struct {
	LastUpdated time.Time `json:"last_updated"`
	DueDate     time.Time `json:"due_date"`
	RequestID   string    `json:"request_id"`
	Status      string    `json:"status"`
	NextAction  string    `json:"next_action"`
	AssignedTo  string    `json:"assigned_to"`
	Notes       []string  `json:"notes"`
	Progress    int       `json:"progress"`
}

// PIARequest represents a privacy impact assessment request
type PIARequest struct {
	RequestDate        time.Time               `json:"request_date"`
	DueDate            time.Time               `json:"due_date"`
	ProcessingActivity *DataProcessingActivity `json:"processing_activity"`
	Metadata           map[string]any          `json:"metadata"`
	Purpose            string                  `json:"purpose,omitempty"`
	AssessmentType     string                  `json:"assessment_type"`
	ID                 string                  `json:"id"`
	RequestedBy        string                  `json:"requested_by"`
	ProjectName        string                  `json:"project_name,omitempty"`
	LegalBasis         string                  `json:"legal_basis,omitempty"`
	Scope              []string                `json:"scope"`
	DataTypes          []string                `json:"data_types,omitempty"`
	Stakeholders       []string                `json:"stakeholders"`
}

// PIAResult represents the result of a privacy impact assessment
type PIAResult struct {
	ReviewDate         time.Time           `json:"review_date"`
	CompletionDate     time.Time           `json:"completion_date"`
	Timestamp          time.Time           `json:"timestamp,omitempty"`
	Metadata           map[string]any      `json:"metadata"`
	ApprovalDate       *time.Time          `json:"approval_date,omitempty"`
	RiskLevel          string              `json:"risk_level"`
	Status             string              `json:"status,omitempty"`
	AssessmentID       string              `json:"assessment_id"`
	ID                 string              `json:"id,omitempty"`
	ApprovedBy         string              `json:"approved_by,omitempty"`
	MitigationMeasures []MitigationMeasure `json:"mitigation_measures"`
	Recommendations    []PIARecommendation `json:"recommendations"`
	Findings           []PIAFinding        `json:"findings"`
	RiskScore          float64             `json:"risk_score"`
	ApprovalRequired   bool                `json:"approval_required"`
}

// PIAFinding represents a finding from a privacy impact assessment
type PIAFinding struct {
	ID          string   `json:"id"`
	Category    string   `json:"category"`
	Severity    string   `json:"severity"`
	Description string   `json:"description"`
	Impact      string   `json:"impact"`
	Likelihood  string   `json:"likelihood"`
	Evidence    []string `json:"evidence"`
	RiskScore   float64  `json:"risk_score"`
}

// PIARecommendation represents a recommendation from a privacy impact assessment
type PIARecommendation struct {
	ID          string   `json:"id"`
	Priority    string   `json:"priority"`
	Description string   `json:"description"`
	Timeline    string   `json:"timeline"`
	Owner       string   `json:"owner"`
	Status      string   `json:"status"`
	Actions     []string `json:"actions"`
}

// MitigationMeasure represents a mitigation measure
type MitigationMeasure struct {
	ReviewDate     time.Time `json:"review_date"`
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Description    string    `json:"description"`
	Implementation string    `json:"implementation"`
	Effectiveness  string    `json:"effectiveness"`
	Cost           string    `json:"cost"`
	Timeline       string    `json:"timeline"`
	Owner          string    `json:"owner"`
	Status         string    `json:"status"`
}

// PIATemplate represents a template for privacy impact assessments
type PIATemplate struct {
	Metadata         map[string]any  `json:"metadata"`
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	ProcessingType   string          `json:"processing_type"`
	Questions        []PIAQuestion   `json:"questions"`
	RiskFactors      []PIARiskFactor `json:"risk_factors"`
	RequiredEvidence []string        `json:"required_evidence"`
}

// PIAQuestion represents a question in a PIA template
type PIAQuestion struct {
	ID         string   `json:"id"`
	Category   string   `json:"category"`
	Question   string   `json:"question"`
	Type       string   `json:"type"`
	Guidance   string   `json:"guidance"`
	Options    []string `json:"options,omitempty"`
	RiskWeight float64  `json:"risk_weight"`
	Required   bool     `json:"required"`
}

// PIARiskFactor represents a risk factor in privacy assessment
type PIARiskFactor struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Weight      float64 `json:"weight"`
	Threshold   float64 `json:"threshold"`
}

// DataProcessingActivity represents a data processing activity
type DataProcessingActivity struct {
	NextReview        time.Time       `json:"next_review"`
	LastReview        time.Time       `json:"last_review"`
	Metadata          map[string]any  `json:"metadata"`
	ID                string          `json:"id"`
	Name              string          `json:"name"`
	Description       string          `json:"description"`
	Controller        string          `json:"controller"`
	Processor         string          `json:"processor,omitempty"`
	ThirdCountries    []string        `json:"third_countries"`
	DataCategories    []string        `json:"data_categories"`
	DataSubjects      []string        `json:"data_subjects"`
	Safeguards        []string        `json:"safeguards"`
	SecurityMeasures  []string        `json:"security_measures"`
	DataSources       []string        `json:"data_sources"`
	Recipients        []DataRecipient `json:"recipients"`
	Purposes          []string        `json:"purposes"`
	LegalBasis        []string        `json:"legal_basis"`
	RetentionPeriod   time.Duration   `json:"retention_period"`
	AutomatedDecision bool            `json:"automated_decision"`
	PIACompleted      bool            `json:"pia_completed"`
	PIARequired       bool            `json:"pia_required"`
	HighRisk          bool            `json:"high_risk"`
	Profiling         bool            `json:"profiling"`
}

// ProcessingValidation represents validation of data processing activity
type ProcessingValidation struct {
	ValidationDate  time.Time         `json:"validation_date"`
	Metadata        map[string]any    `json:"metadata"`
	Issues          []ValidationIssue `json:"issues"`
	Recommendations []string          `json:"recommendations"`
	RequiredActions []string          `json:"required_actions"`
	ComplianceScore float64           `json:"compliance_score"`
	Valid           bool              `json:"valid"`
}

// ValidationIssue represents a validation issue
type ValidationIssue struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// RiskAssessment represents a risk assessment
type RiskAssessment struct {
	AssessmentDate time.Time            `json:"assessment_date"`
	ReviewDate     time.Time            `json:"review_date"`
	Metadata       map[string]any       `json:"metadata"`
	ID             string               `json:"id"`
	ActivityID     string               `json:"activity_id"`
	RiskLevel      string               `json:"risk_level"`
	ApprovedBy     string               `json:"approved_by,omitempty"`
	RiskFactors    []AssessedRiskFactor `json:"risk_factors"`
	Mitigations    []MitigationMeasure  `json:"mitigations"`
	RiskScore      float64              `json:"risk_score"`
	ResidualRisk   float64              `json:"residual_risk"`
	Approved       bool                 `json:"approved"`
}

// AssessedRiskFactor represents an assessed risk factor
type AssessedRiskFactor struct {
	Impact     string `json:"impact"`
	Likelihood string `json:"likelihood"`
	Rationale  string `json:"rationale"`
	PIARiskFactor
	Score float64 `json:"score"`
}

// CrossBorderTransfer represents a cross-border data transfer
type CrossBorderTransfer struct {
	TransferDate       time.Time      `json:"transfer_date"`
	Metadata           map[string]any `json:"metadata"`
	DestinationCountry string         `json:"destination_country"`
	DataImporter       string         `json:"data_importer"`
	ID                 string         `json:"id"`
	DataExporter       string         `json:"data_exporter"`
	SourceCountry      string         `json:"source_country"`
	LegalBasis         string         `json:"legal_basis"`
	Frequency          string         `json:"frequency"`
	Volume             string         `json:"volume"`
	Purposes           []string       `json:"purposes"`
	Safeguards         []string       `json:"safeguards"`
	DataCategories     []string       `json:"data_categories"`
	BCRApplied         bool           `json:"bcr_applied"`
	SCCApplied         bool           `json:"scc_applied"`
	AdequacyDecision   bool           `json:"adequacy_decision"`
}

// TransferValidation represents validation of cross-border transfer
type TransferValidation struct {
	ValidationDate  time.Time         `json:"validation_date"`
	Metadata        map[string]any    `json:"metadata"`
	Issues          []ValidationIssue `json:"issues"`
	Recommendations []string          `json:"recommendations"`
	Valid           bool              `json:"valid"`
	LegalBasisValid bool              `json:"legal_basis_valid"`
	SafeguardsValid bool              `json:"safeguards_valid"`
}

// CrossBorderRule represents a rule for cross-border transfers
type CrossBorderRule struct {
	ID                 string   `json:"id"`
	Name               string   `json:"name"`
	SourceCountries    []string `json:"source_countries"`
	DestCountries      []string `json:"dest_countries"`
	DataCategories     []string `json:"data_categories"`
	RequiredSafeguards []string `json:"required_safeguards"`
	Conditions         []string `json:"conditions"`
	Prohibited         bool     `json:"prohibited"`
}

// AdequacyDecision represents an adequacy decision
type AdequacyDecision struct {
	Country      string     `json:"country"`
	Decision     string     `json:"decision"`
	DecisionDate time.Time  `json:"decision_date"`
	ValidUntil   *time.Time `json:"valid_until,omitempty"`
	Conditions   []string   `json:"conditions"`
}

// SCCValidation represents Standard Contractual Clauses validation
type SCCValidation struct {
	Metadata       map[string]any `json:"metadata"`
	ClausesVersion string         `json:"clauses_version"`
	DataExporter   string         `json:"data_exporter"`
	DataImporter   string         `json:"data_importer"`
	DataCategories []string       `json:"data_categories"`
	Purposes       []string       `json:"purposes"`
}

// SCCResult represents the result of SCC validation
type SCCResult struct {
	ValidationDate    time.Time         `json:"validation_date"`
	Metadata          map[string]any    `json:"metadata"`
	Issues            []ValidationIssue `json:"issues"`
	Recommendations   []string          `json:"recommendations"`
	Valid             bool              `json:"valid"`
	ClausesApplicable bool              `json:"clauses_applicable"`
}

// BCRValidation represents Binding Corporate Rules validation
type BCRValidation struct {
	Metadata       map[string]any `json:"metadata"`
	CompanyGroup   string         `json:"company_group"`
	BCRVersion     string         `json:"bcr_version"`
	DataCategories []string       `json:"data_categories"`
	Purposes       []string       `json:"purposes"`
	Countries      []string       `json:"countries"`
}

// BCRResult represents the result of BCR validation
type BCRResult struct {
	ValidationDate  time.Time         `json:"validation_date"`
	Metadata        map[string]any    `json:"metadata"`
	Issues          []ValidationIssue `json:"issues"`
	Recommendations []string          `json:"recommendations"`
	Valid           bool              `json:"valid"`
	BCRApplicable   bool              `json:"bcr_applicable"`
}

// ConsentEvent represents a consent-related event for audit logging
type ConsentEvent struct {
	Timestamp     time.Time      `json:"timestamp"`
	Details       map[string]any `json:"details"`
	Metadata      map[string]any `json:"metadata"`
	EventType     string         `json:"event_type"`
	ConsentID     string         `json:"consent_id"`
	DataSubjectID string         `json:"data_subject_id"`
	IPAddress     string         `json:"ip_address"`
	UserAgent     string         `json:"user_agent"`
}

// DataSubjectRequestLog represents a data subject request for audit logging
type DataSubjectRequestLog struct {
	Timestamp     time.Time      `json:"timestamp"`
	Details       map[string]any `json:"details"`
	Metadata      map[string]any `json:"metadata"`
	RequestID     string         `json:"request_id"`
	RequestType   string         `json:"request_type"`
	DataSubjectID string         `json:"data_subject_id"`
	Status        string         `json:"status"`
	ProcessedBy   string         `json:"processed_by"`
}

// CrossBorderTransferLog represents a cross-border transfer for audit logging
type CrossBorderTransferLog struct {
	Timestamp          time.Time      `json:"timestamp"`
	Metadata           map[string]any `json:"metadata"`
	TransferID         string         `json:"transfer_id"`
	DataExporter       string         `json:"data_exporter"`
	DataImporter       string         `json:"data_importer"`
	SourceCountry      string         `json:"source_country"`
	DestinationCountry string         `json:"destination_country"`
	LegalBasis         string         `json:"legal_basis"`
	Safeguards         []string       `json:"safeguards"`
}

// PrivacyBreachLog represents a privacy breach for audit logging
type PrivacyBreachLog struct {
	DetectedDate      time.Time      `json:"detected_date"`
	ReportedDate      time.Time      `json:"reported_date"`
	Metadata          map[string]any `json:"metadata"`
	BreachID          string         `json:"breach_id"`
	BreachType        string         `json:"breach_type"`
	Severity          string         `json:"severity"`
	Cause             string         `json:"cause"`
	DataCategories    []string       `json:"data_categories"`
	Mitigation        []string       `json:"mitigation"`
	AffectedSubjects  int            `json:"affected_subjects"`
	AuthorityNotified bool           `json:"authority_notified"`
	SubjectsNotified  bool           `json:"subjects_notified"`
}

// NewGDPRConsentManager creates a new GDPR consent manager
func NewGDPRConsentManager(config GDPRConsentConfig) *GDPRConsentManager {
	return &GDPRConsentManager{
		config: config,
	}
}

// SetConsentStore sets the consent store
func (gcm *GDPRConsentManager) SetConsentStore(store ConsentStore) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	gcm.consentStore = store
}

// SetDataSubjectRightsHandler sets the data subject rights handler
func (gcm *GDPRConsentManager) SetDataSubjectRightsHandler(handler DataSubjectRightsHandler) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	gcm.dataSubjectRights = handler
}

// SetPrivacyImpactAssessment sets the privacy impact assessment handler
func (gcm *GDPRConsentManager) SetPrivacyImpactAssessment(pia PrivacyImpactAssessment) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	gcm.privacyAssessment = pia
}

// SetCrossBorderValidator sets the cross-border validator
func (gcm *GDPRConsentManager) SetCrossBorderValidator(validator CrossBorderValidator) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	gcm.crossBorderValidator = validator
}

// SetAuditLogger sets the GDPR audit logger
func (gcm *GDPRConsentManager) SetAuditLogger(logger GDPRAuditLogger) {
	gcm.mu.Lock()
	defer gcm.mu.Unlock()
	gcm.auditLogger = logger
}

// RecordConsent records a new consent
func (gcm *GDPRConsentManager) RecordConsent(ctx context.Context, consent *ConsentRecord) error {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if !gcm.config.Enabled {
		return fmt.Errorf("GDPR consent management not enabled")
	}

	if gcm.consentStore == nil {
		return fmt.Errorf("consent store not configured")
	}

	// Validate consent
	if err := gcm.validateConsent(consent); err != nil {
		return fmt.Errorf("consent validation failed: %w", err)
	}

	// Set expiry date if configured
	if gcm.config.ConsentExpiryDays > 0 && consent.ExpiryDate == nil {
		expiryDate := time.Now().AddDate(0, 0, gcm.config.ConsentExpiryDays)
		consent.ExpiryDate = &expiryDate
	}

	// Store consent
	if err := gcm.consentStore.StoreConsent(ctx, consent); err != nil {
		return fmt.Errorf("failed to store consent: %w", err)
	}

	// Log consent event
	if gcm.auditLogger != nil {
		event := &ConsentEvent{
			EventType:     "consent_recorded",
			ConsentID:     consent.ID,
			DataSubjectID: consent.DataSubjectID,
			Timestamp:     time.Now(),
			Details: map[string]any{
				"consent_method": consent.ConsentMethod,
				"purposes":       consent.ProcessingPurposes,
				"legal_basis":    consent.LegalBasis,
			},
		}
		if err := gcm.auditLogger.LogConsentEvent(ctx, event); err != nil {
			log.Printf("Warning: failed to log consent event: %v", err)
		}
	}

	return nil
}

// GetConsent retrieves consent for a data subject and purpose
func (gcm *GDPRConsentManager) GetConsent(ctx context.Context, dataSubjectID, purpose string) (*ConsentRecord, error) {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if dataSubjectID == "" {
		return nil, fmt.Errorf("data subject ID is required")
	}

	if gcm.consentStore == nil {
		return nil, fmt.Errorf("consent store not configured")
	}

	return gcm.consentStore.GetConsent(ctx, dataSubjectID, purpose)
}

// WithdrawConsent withdraws consent
func (gcm *GDPRConsentManager) WithdrawConsent(ctx context.Context, consentID string, withdrawal *ConsentWithdrawal) error {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if consentID == "" {
		return fmt.Errorf("consent ID is required")
	}

	if withdrawal == nil {
		return fmt.Errorf("withdrawal information is required")
	}

	if !gcm.config.ConsentWithdrawalEnabled {
		return fmt.Errorf("consent withdrawal not enabled")
	}

	if gcm.consentStore == nil {
		return fmt.Errorf("consent store not configured")
	}

	// Withdraw consent
	if err := gcm.consentStore.WithdrawConsent(ctx, consentID, withdrawal); err != nil {
		return fmt.Errorf("failed to withdraw consent: %w", err)
	}

	// Log withdrawal event
	if gcm.auditLogger != nil {
		event := &ConsentEvent{
			EventType: "consent_withdrawn",
			ConsentID: consentID,
			Timestamp: time.Now(),
			Details: map[string]any{
				"withdrawal_method": withdrawal.WithdrawalMethod,
				"reason":            withdrawal.Reason,
				"partial":           withdrawal.PartialWithdrawal,
			},
		}
		if err := gcm.auditLogger.LogConsentEvent(ctx, event); err != nil {
			log.Printf("Warning: failed to log consent event: %v", err)
		}
	}

	return nil
}

// ProcessDataSubjectRequest processes a data subject request
func (gcm *GDPRConsentManager) ProcessDataSubjectRequest(ctx context.Context, request *DataAccessRequest) error {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if gcm.dataSubjectRights == nil {
		return fmt.Errorf("data subject rights handler not configured")
	}

	// Log request
	if gcm.auditLogger != nil {
		requestLog := &DataSubjectRequestLog{
			RequestID:     request.ID,
			RequestType:   request.RequestType,
			DataSubjectID: request.DataSubjectID,
			Timestamp:     time.Now(),
			Status:        "received",
		}
		if err := gcm.auditLogger.LogDataSubjectRequest(ctx, requestLog); err != nil {
			log.Printf("Warning: failed to log data subject request: %v", err)
		}
	}

	// Process based on request type
	switch request.RequestType {
	case "access":
		_, err := gcm.dataSubjectRights.HandleAccessRequest(ctx, request)
		return err
	case "portability":
		portabilityRequest := &DataPortabilityRequest{DataAccessRequest: *request}
		_, err := gcm.dataSubjectRights.HandlePortabilityRequest(ctx, portabilityRequest)
		return err
	case "erasure":
		erasureRequest := &DataErasureRequest{DataAccessRequest: *request}
		_, err := gcm.dataSubjectRights.HandleErasureRequest(ctx, erasureRequest)
		return err
	case "rectification":
		rectificationRequest := &DataRectificationRequest{DataAccessRequest: *request}
		_, err := gcm.dataSubjectRights.HandleRectificationRequest(ctx, rectificationRequest)
		return err
	case "objection":
		objectionRequest := &DataObjectionRequest{DataAccessRequest: *request}
		_, err := gcm.dataSubjectRights.HandleObjectionRequest(ctx, objectionRequest)
		return err
	default:
		return fmt.Errorf("unsupported request type: %s", request.RequestType)
	}
}

// validateConsent validates a consent record
func (gcm *GDPRConsentManager) validateConsent(consent *ConsentRecord) error {
	validator := newConsentValidator(gcm, consent)
	return validator.validate()
}

// consentValidator handles consent validation logic
type consentValidator struct {
	manager *GDPRConsentManager
	consent *ConsentRecord
}

// newConsentValidator creates a new consent validator
func newConsentValidator(manager *GDPRConsentManager, consent *ConsentRecord) *consentValidator {
	return &consentValidator{
		manager: manager,
		consent: consent,
	}
}

// validate performs all consent validations
func (v *consentValidator) validate() error {
	// Basic validations
	if err := v.validateRequired(); err != nil {
		return err
	}

	// Legal basis validation
	if err := v.validateLegalBasis(); err != nil {
		return err
	}

	// Time-based validation
	if err := v.validateExpiry(); err != nil {
		return err
	}

	// Configuration-based validations
	if err := v.validateConfigRequirements(); err != nil {
		return err
	}

	// GDPR compliance validations
	return v.validateGDPRRequirements()
}

// validateRequired checks required fields
func (v *consentValidator) validateRequired() error {
	if v.consent == nil {
		return fmt.Errorf("consent record is required")
	}

	if v.consent.DataSubjectID == "" {
		return fmt.Errorf("data subject ID is required")
	}

	if !v.hasPurpose() {
		return fmt.Errorf("purpose is required")
	}

	if v.consent.LegalBasis == "" {
		return fmt.Errorf("legal basis is required")
	}

	return nil
}

// hasPurpose checks if purpose is provided
func (v *consentValidator) hasPurpose() bool {
	return v.consent.Purpose != "" || len(v.consent.ProcessingPurposes) > 0
}

// validateLegalBasis validates the legal basis value
func (v *consentValidator) validateLegalBasis() error {
	validBases := v.getValidLegalBases()

	for _, validBasis := range validBases {
		if v.consent.LegalBasis == validBasis {
			return nil
		}
	}

	return fmt.Errorf("invalid legal basis")
}

// getValidLegalBases returns valid legal basis values
func (v *consentValidator) getValidLegalBases() []string {
	return []string{
		"consent",
		"contract",
		"legal_obligation",
		"vital_interests",
		"public_task",
		"legitimate_interests",
	}
}

// validateExpiry checks consent expiration
func (v *consentValidator) validateExpiry() error {
	if v.consent.ExpiryDate == nil {
		return nil
	}

	if v.consent.ExpiryDate.Before(time.Now()) {
		return fmt.Errorf("consent has expired")
	}

	return nil
}

// validateConfigRequirements validates configuration-based requirements
func (v *consentValidator) validateConfigRequirements() error {
	if v.manager.config.GranularConsentRequired && !v.consent.Granular {
		return fmt.Errorf("granular consent is required")
	}

	if v.manager.config.ConsentProofRequired && v.consent.ConsentProof == nil {
		return fmt.Errorf("consent proof is required")
	}

	return nil
}

// validateGDPRRequirements validates GDPR compliance requirements
func (v *consentValidator) validateGDPRRequirements() error {
	if !v.consent.Specific {
		return fmt.Errorf("consent must be specific")
	}

	if !v.consent.Informed {
		return fmt.Errorf("consent must be informed")
	}

	if !v.consent.Unambiguous {
		return fmt.Errorf("consent must be unambiguous")
	}

	return nil
}

// Additional types needed by tests

// ConsentUpdate represents updates to consent (alias for ConsentUpdates for test compatibility)
type ConsentUpdate = ConsentUpdates

// ConsentHistoryEntry represents a historical consent entry
type ConsentHistoryEntry struct {
	Timestamp     time.Time      `json:"timestamp"`
	Changes       map[string]any `json:"changes"`
	Metadata      map[string]any `json:"metadata"`
	ID            string         `json:"id"`
	ConsentID     string         `json:"consent_id"`
	Action        string         `json:"action"`
	DataSubjectID string         `json:"data_subject_id"`
	UpdatedBy     string         `json:"updated_by"`
	Reason        string         `json:"reason"`
	IPAddress     string         `json:"ip_address"`
	UserAgent     string         `json:"user_agent"`
}

// PIAUpdate represents updates to a Privacy Impact Assessment
type PIAUpdate struct {
	RiskLevel          *string             `json:"risk_level,omitempty"`
	RiskScore          *float64            `json:"risk_score,omitempty"`
	ApprovalRequired   *bool               `json:"approval_required,omitempty"`
	ApprovalDate       *time.Time          `json:"approval_date,omitempty"`
	ReviewDate         *time.Time          `json:"review_date,omitempty"`
	Metadata           map[string]any      `json:"metadata,omitempty"`
	ApprovedBy         string              `json:"approved_by,omitempty"`
	UpdatedBy          string              `json:"updated_by"`
	UpdateReason       string              `json:"update_reason"`
	Findings           []PIAFinding        `json:"findings,omitempty"`
	Recommendations    []PIARecommendation `json:"recommendations,omitempty"`
	MitigationMeasures []MitigationMeasure `json:"mitigation_measures,omitempty"`
}

// PIAFilters represents filters for PIA queries
type PIAFilters struct {
	RiskLevel       []string   `json:"risk_level,omitempty"`
	AssessmentType  []string   `json:"assessment_type,omitempty"`
	RequestedBy     []string   `json:"requested_by,omitempty"`
	DateFrom        *time.Time `json:"date_from,omitempty"`
	DateTo          *time.Time `json:"date_to,omitempty"`
	ApprovalStatus  []string   `json:"approval_status,omitempty"`
	ProcessingTypes []string   `json:"processing_types,omitempty"`
	Limit           int        `json:"limit,omitempty"`
	Offset          int        `json:"offset,omitempty"`
}

// Additional methods needed by tests

// HandleAccessRequest handles a data access request
func (gcm *GDPRConsentManager) HandleAccessRequest(ctx context.Context, request *DataAccessRequest) (*DataAccessResponse, error) {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if request == nil {
		return nil, fmt.Errorf("request is required")
	}

	if request.DataSubjectID == "" {
		return nil, fmt.Errorf("data subject ID is required")
	}

	if gcm.dataSubjectRights == nil {
		return nil, fmt.Errorf("data subject rights handler not configured")
	}

	return gcm.dataSubjectRights.HandleAccessRequest(ctx, request)
}

// ConductPIA conducts a privacy impact assessment
func (gcm *GDPRConsentManager) ConductPIA(ctx context.Context, request *PIARequest) (*PIAResult, error) {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if request == nil {
		return nil, fmt.Errorf("PIA request is required")
	}

	if request.ProjectName == "" {
		return nil, fmt.Errorf("project name is required")
	}

	if gcm.privacyAssessment == nil {
		return nil, fmt.Errorf("privacy impact assessment handler not configured")
	}

	return gcm.privacyAssessment.ConductPIA(ctx, request)
}

// UpdateConsent updates an existing consent
func (gcm *GDPRConsentManager) UpdateConsent(ctx context.Context, consentID string, updates *ConsentUpdate) error {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if gcm.consentStore == nil {
		return fmt.Errorf("consent store not configured")
	}

	return gcm.consentStore.UpdateConsent(ctx, consentID, updates)
}

// HandleErasureRequest handles a data erasure request
func (gcm *GDPRConsentManager) HandleErasureRequest(ctx context.Context, request *DataErasureRequest) (*DataErasureResponse, error) {
	gcm.mu.RLock()
	defer gcm.mu.RUnlock()

	if gcm.dataSubjectRights == nil {
		return nil, fmt.Errorf("data subject rights handler not configured")
	}

	return gcm.dataSubjectRights.HandleErasureRequest(ctx, request)
}

// Test-only helpers for consent validation, email, ID generation, and expiry are defined in _test.go
