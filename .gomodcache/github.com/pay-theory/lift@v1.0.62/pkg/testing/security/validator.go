package security

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

// SecurityValidator provides comprehensive security testing automation
type SecurityValidator struct {
	scanners         []VulnerabilityScanner
	penetrationTests []PenetrationTest
	complianceChecks []ComplianceCheck
	authTests        []AuthenticationTest
	dataProtection   []DataProtectionTest
	config           SecurityConfig
}

// SecurityConfig configures security validation behavior
type SecurityConfig struct {
	ThreatThreshold   Severity
	ComplianceLevel   ComplianceLevel
	ReportFormat      ReportFormat
	MaxScanTime       time.Duration
	EnablePenetration bool
	EnableCompliance  bool
	AlertOnCritical   bool
}

// Core security types
type Severity string

const (
	SeverityLow      Severity = "low"
	SeverityMedium   Severity = "medium"
	SeverityHigh     Severity = "high"
	SeverityCritical Severity = "critical"
)

type ComplianceLevel string

const (
	ComplianceLevelBasic      ComplianceLevel = "basic"
	ComplianceLevelStandard   ComplianceLevel = "standard"
	ComplianceLevelStrict     ComplianceLevel = "strict"
	ComplianceLevelEnterprise ComplianceLevel = "enterprise"
)

type ReportFormat string

const (
	ReportFormatJSON ReportFormat = "json"
	ReportFormatHTML ReportFormat = "html"
	ReportFormatPDF  ReportFormat = "pdf"
)

// SecurityTarget represents a target for security testing
type SecurityTarget struct {
	Type        TargetType
	URL         string
	Credentials SecurityCredentials
	Context     SecurityContext
	Assets      []Asset
	ThreatModel ThreatModel
}

type TargetType string

const (
	TargetTypeAPI         TargetType = "api"
	TargetTypeDatabase    TargetType = "database"
	TargetTypeNetwork     TargetType = "network"
	TargetTypeApplication TargetType = "application"
	TargetTypeInfra       TargetType = "infrastructure"
)

// Vulnerability represents a security vulnerability
type Vulnerability struct {
	FoundAt     time.Time
	Impact      ImpactAssessment
	ID          string
	Type        VulnerabilityType
	Severity    Severity
	Title       string
	Description string
	Location    string
	CVSS        CVSSScore
	References  []string
	Evidence    []Evidence
	Remediation RemediationPlan
}

type VulnerabilityType string

const (
	VulnTypeInjection       VulnerabilityType = "injection"
	VulnTypeAuth            VulnerabilityType = "authentication"
	VulnTypeDataExposure    VulnerabilityType = "data_exposure"
	VulnTypeXSS             VulnerabilityType = "xss"
	VulnTypeCSRF            VulnerabilityType = "csrf"
	VulnTypeInsecureConfig  VulnerabilityType = "insecure_config"
	VulnTypeAccessControl   VulnerabilityType = "access_control"
	VulnTypeCrypto          VulnerabilityType = "cryptographic"
	VulnTypeDeserialization VulnerabilityType = "deserialization"
	VulnTypeLogging         VulnerabilityType = "logging"
)

// Security interfaces
type VulnerabilityScanner interface {
	Scan(ctx context.Context, target SecurityTarget) ([]Vulnerability, error)
	GetSeverity(vuln Vulnerability) Severity
	GenerateReport(vulns []Vulnerability) SecurityReport
	GetScanType() ScanType
}

type ScanType string

const (
	ScanTypeOWASP ScanType = "owasp"
	ScanTypeSAST  ScanType = "sast"
	ScanTypeDAST  ScanType = "dast"
	ScanTypeIAST  ScanType = "iast"
	ScanTypeAPI   ScanType = "api"
)

type PenetrationTest interface {
	Execute(ctx context.Context, target SecurityTarget) ([]SecurityFinding, error)
	GetTestType() string
	GetRiskLevel() RiskLevel
	ValidateExploit(finding SecurityFinding) bool
}

type ComplianceCheck interface {
	Validate(ctx context.Context, system SystemInfo) (ComplianceResult, error)
	GetStandard() string
	GetRequirements() []Requirement
	GenerateComplianceReport() ComplianceReport
}

type AuthenticationTest interface {
	TestAuthentication(ctx context.Context, target SecurityTarget) ([]AuthFinding, error)
	TestAuthorization(ctx context.Context, target SecurityTarget) ([]AuthFinding, error)
	TestSessionManagement(ctx context.Context, target SecurityTarget) ([]AuthFinding, error)
}

type DataProtectionTest interface {
	TestEncryption(ctx context.Context, target SecurityTarget) ([]DataFinding, error)
	TestDataLeakage(ctx context.Context, target SecurityTarget) ([]DataFinding, error)
	TestPrivacyCompliance(ctx context.Context, target SecurityTarget) ([]DataFinding, error)
}

// Supporting types
type SecurityCredentials struct {
	Headers  map[string]string
	Username string
	Password string
	Token    string
	APIKey   string
}

type SecurityContext struct {
	UserAgent   string
	SessionID   string
	RequestID   string
	TenantID    string
	UserID      string
	Permissions []string
	Roles       []string
}

type Asset struct {
	ID          string
	Type        AssetType
	Name        string
	Value       AssetValue
	Sensitivity DataSensitivity
	Location    string
	Owner       string
}

type AssetType string

const (
	AssetTypeData        AssetType = "data"
	AssetTypeAPI         AssetType = "api"
	AssetTypeService     AssetType = "service"
	AssetTypeDatabase    AssetType = "database"
	AssetTypeCredentials AssetType = "credentials"
)

type AssetValue string

const (
	AssetValueLow      AssetValue = "low"
	AssetValueMedium   AssetValue = "medium"
	AssetValueHigh     AssetValue = "high"
	AssetValueCritical AssetValue = "critical"
)

type DataSensitivity string

const (
	DataSensitivityPublic       DataSensitivity = "public"
	DataSensitivityInternal     DataSensitivity = "internal"
	DataSensitivityConfidential DataSensitivity = "confidential"
	DataSensitivityRestricted   DataSensitivity = "restricted"
)

// NewSecurityValidator creates a new security validator
func NewSecurityValidator(config SecurityConfig) *SecurityValidator {
	return &SecurityValidator{
		scanners:         make([]VulnerabilityScanner, 0),
		penetrationTests: make([]PenetrationTest, 0),
		complianceChecks: make([]ComplianceCheck, 0),
		authTests:        make([]AuthenticationTest, 0),
		dataProtection:   make([]DataProtectionTest, 0),
		config:           config,
	}
}

// AddScanner adds a vulnerability scanner
func (sv *SecurityValidator) AddScanner(scanner VulnerabilityScanner) {
	sv.scanners = append(sv.scanners, scanner)
}

// AddPenetrationTest adds a penetration test
func (sv *SecurityValidator) AddPenetrationTest(test PenetrationTest) {
	sv.penetrationTests = append(sv.penetrationTests, test)
}

// AddComplianceCheck adds a compliance check
func (sv *SecurityValidator) AddComplianceCheck(check ComplianceCheck) {
	sv.complianceChecks = append(sv.complianceChecks, check)
}

// ValidateTarget performs comprehensive security validation
func (sv *SecurityValidator) ValidateTarget(ctx context.Context, target SecurityTarget) (*SecurityValidationResult, error) {
	result := &SecurityValidationResult{
		Target:     target,
		StartTime:  time.Now(),
		Scanners:   make(map[string][]Vulnerability),
		PenTests:   make(map[string][]SecurityFinding),
		Compliance: make(map[string]ComplianceResult),
		Summary:    SecuritySummary{},
	}

	// Run vulnerability scans
	for _, scanner := range sv.scanners {
		vulns, err := scanner.Scan(ctx, target)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Scanner %s failed: %v", scanner.GetScanType(), err))
			continue
		}
		result.Scanners[string(scanner.GetScanType())] = vulns
		result.Summary.TotalVulnerabilities += len(vulns)
	}

	// Run penetration tests if enabled
	if sv.config.EnablePenetration {
		for _, penTest := range sv.penetrationTests {
			findings, err := penTest.Execute(ctx, target)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("PenTest %s failed: %v", penTest.GetTestType(), err))
				continue
			}
			result.PenTests[penTest.GetTestType()] = findings
			result.Summary.TotalFindings += len(findings)
		}
	}

	// Run compliance checks if enabled
	if sv.config.EnableCompliance {
		systemInfo := SystemInfo{
			Target:      target,
			Timestamp:   time.Now(),
			Environment: "test",
		}

		for _, compCheck := range sv.complianceChecks {
			compResult, err := compCheck.Validate(ctx, systemInfo)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("Compliance %s failed: %v", compCheck.GetStandard(), err))
				continue
			}
			result.Compliance[compCheck.GetStandard()] = compResult
			if !compResult.Compliant {
				result.Summary.ComplianceViolations++
			}
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Calculate risk score
	result.Summary.RiskScore = sv.calculateRiskScore(result)

	return result, nil
}

// calculateRiskScore calculates overall risk score
func (sv *SecurityValidator) calculateRiskScore(result *SecurityValidationResult) float64 {
	var score float64

	// Weight vulnerabilities by severity
	for _, vulns := range result.Scanners {
		for _, vuln := range vulns {
			switch vuln.Severity {
			case SeverityCritical:
				score += 10.0
			case SeverityHigh:
				score += 7.0
			case SeverityMedium:
				score += 4.0
			case SeverityLow:
				score += 1.0
			}
		}
	}

	// Weight penetration test findings
	for _, findings := range result.PenTests {
		for _, finding := range findings {
			switch finding.RiskLevel {
			case RiskLevelCritical:
				score += 15.0
			case RiskLevelHigh:
				score += 10.0
			case RiskLevelMedium:
				score += 5.0
			case RiskLevelLow:
				score += 2.0
			}
		}
	}

	// Weight compliance violations
	score += float64(result.Summary.ComplianceViolations) * 5.0

	// Normalize to 0-100 scale
	maxScore := 100.0
	if score > maxScore {
		score = maxScore
	}

	return score
}

// SecurityValidationResult contains comprehensive security validation results
type SecurityValidationResult struct {
	StartTime  time.Time
	EndTime    time.Time
	Scanners   map[string][]Vulnerability
	PenTests   map[string][]SecurityFinding
	Compliance map[string]ComplianceResult
	Target     SecurityTarget
	Errors     []string
	Summary    SecuritySummary
	Duration   time.Duration
}

type SecuritySummary struct {
	TotalVulnerabilities int
	TotalFindings        int
	ComplianceViolations int
	RiskScore            float64
	CriticalIssues       int
	HighIssues           int
	MediumIssues         int
	LowIssues            int
}

// OWASP Top 10 Scanner Implementation
type OWASPScanner struct {
	client  *http.Client
	timeout time.Duration
}

func NewOWASPScanner(timeout time.Duration) *OWASPScanner {
	return &OWASPScanner{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

func (o *OWASPScanner) Scan(ctx context.Context, target SecurityTarget) ([]Vulnerability, error) {
	var vulnerabilities []Vulnerability

	// OWASP A01: Broken Access Control
	if vulns := o.scanAccessControl(ctx, target); len(vulns) > 0 {
		vulnerabilities = append(vulnerabilities, vulns...)
	}

	// OWASP A02: Cryptographic Failures
	if vulns := o.scanCryptographic(ctx, target); len(vulns) > 0 {
		vulnerabilities = append(vulnerabilities, vulns...)
	}

	// OWASP A03: Injection
	if vulns := o.scanInjection(ctx, target); len(vulns) > 0 {
		vulnerabilities = append(vulnerabilities, vulns...)
	}

	// OWASP A04: Insecure Design
	if vulns := o.scanInsecureDesign(ctx, target); len(vulns) > 0 {
		vulnerabilities = append(vulnerabilities, vulns...)
	}

	// OWASP A05: Security Misconfiguration
	if vulns := o.scanMisconfiguration(ctx, target); len(vulns) > 0 {
		vulnerabilities = append(vulnerabilities, vulns...)
	}

	return vulnerabilities, nil
}

func (o *OWASPScanner) scanAccessControl(ctx context.Context, target SecurityTarget) []Vulnerability {
	var vulns []Vulnerability

	// Test for broken access control
	req, err := http.NewRequestWithContext(ctx, "GET", target.URL+"/admin", nil)
	if err != nil {
		return vulns
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return vulns
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	// Check if admin endpoint is accessible without authentication
	if resp.StatusCode == 200 {
		vulns = append(vulns, Vulnerability{
			ID:          "OWASP-A01-001",
			Type:        VulnTypeAccessControl,
			Severity:    SeverityHigh,
			Title:       "Broken Access Control",
			Description: "Admin endpoint accessible without authentication",
			Location:    target.URL + "/admin",
			FoundAt:     time.Now(),
		})
	}

	return vulns
}

func (o *OWASPScanner) scanCryptographic(_ context.Context, target SecurityTarget) []Vulnerability {
	var vulns []Vulnerability

	// Check if HTTPS is enforced
	if strings.HasPrefix(target.URL, "http://") {
		vulns = append(vulns, Vulnerability{
			ID:          "OWASP-A02-001",
			Type:        VulnTypeCrypto,
			Severity:    SeverityMedium,
			Title:       "Insecure Transport",
			Description: "Application not using HTTPS",
			Location:    target.URL,
			FoundAt:     time.Now(),
		})
	}

	return vulns
}

func (o *OWASPScanner) scanInjection(ctx context.Context, target SecurityTarget) []Vulnerability {
	var vulns []Vulnerability

	// Test for SQL injection
	testPayloads := []string{
		"' OR '1'='1",
		"'; DROP TABLE users; --",
		"' UNION SELECT * FROM users --",
	}

	for _, payload := range testPayloads {
		req, err := http.NewRequestWithContext(ctx, "GET", target.URL+"?id="+payload, nil)
		if err != nil {
			continue
		}

		resp, err := o.client.Do(req)
		if err != nil {
			continue
		}
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}

		// Simple heuristic: SQL errors in response indicate potential injection
		if resp.StatusCode == 500 {
			vulns = append(vulns, Vulnerability{
				ID:          "OWASP-A03-001",
				Type:        VulnTypeInjection,
				Severity:    SeverityCritical,
				Title:       "SQL Injection",
				Description: "Potential SQL injection vulnerability detected",
				Location:    target.URL + "?id=" + payload,
				FoundAt:     time.Now(),
			})
			break // Don't test all payloads if one works
		}
	}

	return vulns
}

func (o *OWASPScanner) scanInsecureDesign(_ context.Context, _ SecurityTarget) []Vulnerability {
	var vulns []Vulnerability

	// Check for common insecure design patterns
	// This is a simplified check - real implementation would be more comprehensive

	return vulns
}

func (o *OWASPScanner) scanMisconfiguration(ctx context.Context, target SecurityTarget) []Vulnerability {
	var vulns []Vulnerability

	// Check for security headers
	req, err := http.NewRequestWithContext(ctx, "GET", target.URL, nil)
	if err != nil {
		return vulns
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return vulns
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Warning: failed to close response body: %v", err)
		}
	}()

	// Check for missing security headers
	securityHeaders := []string{
		"X-Content-Type-Options",
		"X-Frame-Options",
		"X-XSS-Protection",
		"Strict-Transport-Security",
		"Content-Security-Policy",
	}

	for _, header := range securityHeaders {
		if resp.Header.Get(header) == "" {
			vulns = append(vulns, Vulnerability{
				ID:          fmt.Sprintf("OWASP-A05-%s", header),
				Type:        VulnTypeInsecureConfig,
				Severity:    SeverityMedium,
				Title:       "Missing Security Header",
				Description: fmt.Sprintf("Missing security header: %s", header),
				Location:    target.URL,
				FoundAt:     time.Now(),
			})
		}
	}

	return vulns
}

func (o *OWASPScanner) GetSeverity(vuln Vulnerability) Severity {
	return vuln.Severity
}

func (o *OWASPScanner) GenerateReport(vulns []Vulnerability) SecurityReport {
	return SecurityReport{
		ScanType:        ScanTypeOWASP,
		Timestamp:       time.Now(),
		Vulnerabilities: vulns,
		Summary: SecurityReportSummary{
			Total:    len(vulns),
			Critical: countBySeverity(vulns, SeverityCritical),
			High:     countBySeverity(vulns, SeverityHigh),
			Medium:   countBySeverity(vulns, SeverityMedium),
			Low:      countBySeverity(vulns, SeverityLow),
		},
	}
}

func (o *OWASPScanner) GetScanType() ScanType {
	return ScanTypeOWASP
}

// Helper function to count vulnerabilities by severity
func countBySeverity(vulns []Vulnerability, severity Severity) int {
	count := 0
	for _, vuln := range vulns {
		if vuln.Severity == severity {
			count++
		}
	}
	return count
}

// Supporting types for the framework
type SecurityReport struct {
	ScanType        ScanType
	Timestamp       time.Time
	Vulnerabilities []Vulnerability
	Summary         SecurityReportSummary
}

type SecurityReportSummary struct {
	Total    int
	Critical int
	High     int
	Medium   int
	Low      int
}

type SecurityFinding struct {
	FoundAt     time.Time
	ID          string
	Type        string
	RiskLevel   RiskLevel
	Title       string
	Description string
	Remediation string
	Evidence    []Evidence
}

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type Evidence struct {
	Timestamp   time.Time
	Type        string
	Description string
	Data        string
	Screenshot  string
}

type ComplianceResult struct {
	Timestamp       time.Time
	Standard        string
	Requirements    []RequirementResult
	Violations      []ComplianceViolation
	Recommendations []string
	Score           float64
	Compliant       bool
}

type RequirementResult struct {
	ID          string
	Description string
	Status      ComplianceStatus
	Notes       string
	Evidence    []Evidence
}

type ComplianceStatus string

const (
	ComplianceStatusPass ComplianceStatus = "pass"
	ComplianceStatusFail ComplianceStatus = "fail"
	ComplianceStatusNA   ComplianceStatus = "not_applicable"
)

type ComplianceViolation struct {
	RequirementID string
	Severity      Severity
	Description   string
	Remediation   string
}

type Requirement struct {
	ID          string
	Description string
	Category    string
	TestMethod  string
	Mandatory   bool
}

type ComplianceReport struct {
	Standard  string
	Version   string
	Timestamp time.Time
	Results   []RequirementResult
	Summary   ComplianceSummary
}

type ComplianceSummary struct {
	TotalRequirements int
	Passed            int
	Failed            int
	NotApplicable     int
	ComplianceScore   float64
}

type SystemInfo struct {
	Target      SecurityTarget
	Timestamp   time.Time
	Environment string
	Version     string
	Components  []SystemComponent
}

type SystemComponent struct {
	Config  map[string]any
	Name    string
	Version string
	Type    string
}

type ThreatModel struct {
	Assets      []Asset
	Threats     []Threat
	Mitigations []Mitigation
	RiskMatrix  RiskMatrix
}

type Threat struct {
	ID          string
	Name        string
	Description string
	Category    ThreatCategory
	Likelihood  Likelihood
	Impact      Impact
	Vectors     []AttackVector
}

type ThreatCategory string

const (
	ThreatCategorySpoofing        ThreatCategory = "spoofing"
	ThreatCategoryTampering       ThreatCategory = "tampering"
	ThreatCategoryRepudiation     ThreatCategory = "repudiation"
	ThreatCategoryInfoDisclosure  ThreatCategory = "information_disclosure"
	ThreatCategoryDenialOfService ThreatCategory = "denial_of_service"
	ThreatCategoryElevationPriv   ThreatCategory = "elevation_of_privilege"
)

type Likelihood string

const (
	LikelihoodVeryLow  Likelihood = "very_low"
	LikelihoodLow      Likelihood = "low"
	LikelihoodMedium   Likelihood = "medium"
	LikelihoodHigh     Likelihood = "high"
	LikelihoodVeryHigh Likelihood = "very_high"
)

type Impact string

const (
	ImpactVeryLow  Impact = "very_low"
	ImpactLow      Impact = "low"
	ImpactMedium   Impact = "medium"
	ImpactHigh     Impact = "high"
	ImpactVeryHigh Impact = "very_high"
)

type AttackVector struct {
	Name            string
	Description     string
	Complexity      Complexity
	Privileges      PrivilegeLevel
	UserInteraction bool
}

type Complexity string

const (
	ComplexityLow    Complexity = "low"
	ComplexityMedium Complexity = "medium"
	ComplexityHigh   Complexity = "high"
)

type PrivilegeLevel string

const (
	PrivilegeLevelNone PrivilegeLevel = "none"
	PrivilegeLevelLow  PrivilegeLevel = "low"
	PrivilegeLevelHigh PrivilegeLevel = "high"
)

type Mitigation struct {
	ID            string
	Name          string
	Description   string
	Type          MitigationType
	Cost          Cost
	Threats       []string
	Effectiveness float64
}

type MitigationType string

const (
	MitigationTypePreventive MitigationType = "preventive"
	MitigationTypeDetective  MitigationType = "detective"
	MitigationTypeResponsive MitigationType = "responsive"
	MitigationTypeRecovery   MitigationType = "recovery"
)

type Cost string

const (
	CostVeryLow  Cost = "very_low"
	CostLow      Cost = "low"
	CostMedium   Cost = "medium"
	CostHigh     Cost = "high"
	CostVeryHigh Cost = "very_high"
)

type RiskMatrix struct {
	Risks []Risk
}

type Risk struct {
	ThreatID     string
	AssetID      string
	Likelihood   Likelihood
	Impact       Impact
	RiskLevel    RiskLevel
	ResidualRisk RiskLevel
	Mitigations  []string
}

type ImpactAssessment struct {
	Confidentiality Impact
	Integrity       Impact
	Availability    Impact
	Financial       Impact
	Reputation      Impact
	Legal           Impact
}

type RemediationPlan struct {
	Priority  Priority
	Effort    Effort
	Steps     []RemediationStep
	Resources []string
	Timeline  time.Duration
}

type RemediationStep struct {
	ID          string
	Description string
	Action      string
	Owner       string
	Deadline    time.Time
	Status      StepStatus
}

type Priority string

const (
	PriorityLow      Priority = "low"
	PriorityMedium   Priority = "medium"
	PriorityHigh     Priority = "high"
	PriorityCritical Priority = "critical"
)

type Effort string

const (
	EffortMinimal     Effort = "minimal"
	EffortLow         Effort = "low"
	EffortMedium      Effort = "medium"
	EffortHigh        Effort = "high"
	EffortSignificant Effort = "significant"
)

type StepStatus string

const (
	StepStatusPending    StepStatus = "pending"
	StepStatusInProgress StepStatus = "in_progress"
	StepStatusCompleted  StepStatus = "completed"
	StepStatusBlocked    StepStatus = "blocked"
)

type CVSSScore struct {
	Version  string
	Vector   string
	Severity Severity
	Score    float64
}

type AuthFinding struct {
	FoundAt     time.Time
	Type        AuthFindingType
	Severity    Severity
	Description string
	Location    string
	Evidence    []Evidence
}

type AuthFindingType string

const (
	AuthFindingTypeWeakAuth       AuthFindingType = "weak_authentication"
	AuthFindingTypeBrokenAuth     AuthFindingType = "broken_authentication"
	AuthFindingTypeWeakSession    AuthFindingType = "weak_session"
	AuthFindingTypeBrokenAccess   AuthFindingType = "broken_access_control"
	AuthFindingTypePrivEscalation AuthFindingType = "privilege_escalation"
)

type DataFinding struct {
	FoundAt     time.Time
	Type        DataFindingType
	Severity    Severity
	Description string
	DataType    DataType
	Location    string
	Evidence    []Evidence
}

type DataFindingType string

const (
	DataFindingTypeExposure         DataFindingType = "data_exposure"
	DataFindingTypeLeakage          DataFindingType = "data_leakage"
	DataFindingTypeWeakCrypto       DataFindingType = "weak_cryptography"
	DataFindingTypeNoEncryption     DataFindingType = "no_encryption"
	DataFindingTypePrivacyViolation DataFindingType = "privacy_violation"
)

type DataType string

const (
	DataTypePII          DataType = "pii"
	DataTypePHI          DataType = "phi"
	DataTypeFinancial    DataType = "financial"
	DataTypeCredentials  DataType = "credentials"
	DataTypeIntellectual DataType = "intellectual_property"
)
