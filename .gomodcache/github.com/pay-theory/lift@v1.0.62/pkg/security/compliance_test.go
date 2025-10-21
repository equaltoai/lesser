package security

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockLiftContext implements LiftContext for testing
type MockLiftContext struct {
	values map[string]any
	mock.Mock
}

func NewMockLiftContext() *MockLiftContext {
	return &MockLiftContext{
		values: make(map[string]any),
	}
}

func (m *MockLiftContext) Set(key string, value any) {
	m.values[key] = value
	m.Called(key, value)
}

func (m *MockLiftContext) Get(key string) any {
	m.Called(key)
	return m.values[key]
}

func (m *MockLiftContext) UserID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLiftContext) TenantID() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLiftContext) ClientIP() string {
	args := m.Called()
	return args.String(0)
}

func (m *MockLiftContext) Logger() Logger {
	args := m.Called()
	logger, ok := args.Get(0).(Logger)
	if !ok {
		return nil
	}
	return logger
}

func (m *MockLiftContext) GetDataAccessLog() []string {
	args := m.Called()
	log, ok := args.Get(0).([]string)
	if !ok {
		return nil
	}
	return log
}

// MockLogger implements Logger for testing
type MockLogger struct {
	mock.Mock
}

func (m *MockLogger) Error(msg string, keysAndValues ...any) {
	m.Called(msg, keysAndValues)
}

func (m *MockLogger) Info(msg string, keysAndValues ...any) {
	m.Called(msg, keysAndValues)
}

func (m *MockLogger) Warn(msg string, keysAndValues ...any) {
	m.Called(msg, keysAndValues)
}

// MockAuditLogger implements AuditLogger for testing
type MockAuditLogger struct {
	mock.Mock
}

func (m *MockAuditLogger) StartAudit(ctx LiftContext) string {
	args := m.Called(ctx)
	return args.String(0)
}

func (m *MockAuditLogger) LogRequest(auditID string, request *AuditRequest) error {
	args := m.Called(auditID, request)
	return args.Error(0)
}

func (m *MockAuditLogger) LogResponse(auditID string, response *AuditResponse) error {
	args := m.Called(auditID, response)
	return args.Error(0)
}

func (m *MockAuditLogger) LogDataAccess(auditID string, access *DataAccessLog) error {
	args := m.Called(auditID, access)
	return args.Error(0)
}

func (m *MockAuditLogger) LogSecurityEvent(auditID string, event *SecurityEvent) error {
	args := m.Called(auditID, event)
	return args.Error(0)
}

// MockComplianceValidator implements ComplianceValidator for testing
type MockComplianceValidator struct {
	mock.Mock
}

func (m *MockComplianceValidator) ValidateRequest(ctx LiftContext, framework string) (*ComplianceResult, error) {
	args := m.Called(ctx, framework)
	result, ok := args.Get(0).(*ComplianceResult)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *MockComplianceValidator) ValidateDataAccess(ctx LiftContext, dataType string) (*ComplianceResult, error) {
	args := m.Called(ctx, dataType)
	result, ok := args.Get(0).(*ComplianceResult)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

func (m *MockComplianceValidator) ValidateRegion(ctx LiftContext, region string) (*ComplianceResult, error) {
	args := m.Called(ctx, region)
	result, ok := args.Get(0).(*ComplianceResult)
	if !ok && args.Get(0) != nil {
		return nil, args.Error(1)
	}
	return result, args.Error(1)
}

// MockHandler implements LiftHandler for testing
type MockHandler struct {
	mock.Mock
}

func (m *MockHandler) Handle(ctx LiftContext) error {
	args := m.Called(ctx)
	return args.Error(0)
}

func TestComplianceFramework_NewComplianceFramework(t *testing.T) {
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2", "GDPR"},
		AuditRetention:    24 * time.Hour,
	}

	framework := NewComplianceFramework("SOC2", config)

	assert.NotNil(t, framework)
	assert.Equal(t, "SOC2", framework.framework)
	assert.Equal(t, config, framework.config)
}

func TestComplianceFramework_ValidateConfiguration(t *testing.T) {
	tests := []struct {
		name        string
		config      ComplianceConfig
		expectError bool
	}{
		{
			name: "valid configuration",
			config: ComplianceConfig{
				EnabledFrameworks: []string{"SOC2", "GDPR"},
				AuditRetention:    24 * time.Hour,
			},
			expectError: false,
		},
		{
			name: "no frameworks enabled",
			config: ComplianceConfig{
				EnabledFrameworks: []string{},
				AuditRetention:    24 * time.Hour,
			},
			expectError: true,
		},
		{
			name: "invalid framework",
			config: ComplianceConfig{
				EnabledFrameworks: []string{"INVALID"},
				AuditRetention:    24 * time.Hour,
			},
			expectError: true,
		},
		{
			name: "insufficient retention",
			config: ComplianceConfig{
				EnabledFrameworks: []string{"SOC2"},
				AuditRetention:    1 * time.Hour,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			framework := NewComplianceFramework("SOC2", tt.config)
			err := framework.ValidateConfiguration()

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestComplianceFramework_ComplianceAudit(t *testing.T) {
	// Setup
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2"},
		AuditRetention:    24 * time.Hour,
	}

	framework := NewComplianceFramework("SOC2", config)

	// Setup mocks
	mockAuditor := &MockAuditLogger{}
	mockValidator := &MockComplianceValidator{}
	mockLogger := &MockLogger{}
	mockCtx := NewMockLiftContext()
	mockHandler := &MockHandler{}

	framework.SetAuditor(mockAuditor)
	framework.SetValidator(mockValidator)

	// Setup expectations
	mockCtx.On("UserID").Return("user123")
	mockCtx.On("TenantID").Return("tenant123")
	mockCtx.On("ClientIP").Return("192.168.1.1")
	mockCtx.On("Logger").Return(mockLogger)
	mockCtx.On("GetDataAccessLog").Return([]string{"data_access_1"})
	mockCtx.On("Set", "audit_id", mock.AnythingOfType("string"))

	mockAuditor.On("StartAudit", mockCtx).Return("audit123")
	mockAuditor.On("LogRequest", "audit123", mock.AnythingOfType("*security.AuditRequest")).Return(nil)
	mockAuditor.On("LogResponse", "audit123", mock.AnythingOfType("*security.AuditResponse")).Return(nil)

	mockValidator.On("ValidateRequest", mockCtx, "SOC2").Return(&ComplianceResult{
		Compliant: true,
		Framework: "SOC2",
		Timestamp: time.Now(),
	}, nil)

	mockHandler.On("Handle", mockCtx).Return(nil)

	// Execute
	middleware := framework.ComplianceAudit()
	wrappedHandler := middleware(mockHandler)
	err := wrappedHandler.Handle(mockCtx)

	// Assert
	assert.NoError(t, err)
	mockAuditor.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
	mockHandler.AssertExpectations(t)
}

func TestComplianceFramework_ComplianceAuditWithViolations(t *testing.T) {
	// Setup
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2"},
		AuditRetention:    24 * time.Hour,
	}

	framework := NewComplianceFramework("SOC2", config)

	// Setup mocks
	mockAuditor := &MockAuditLogger{}
	mockValidator := &MockComplianceValidator{}
	mockLogger := &MockLogger{}
	mockCtx := NewMockLiftContext()
	mockHandler := &MockHandler{}

	framework.SetAuditor(mockAuditor)
	framework.SetValidator(mockValidator)

	// Setup expectations
	mockCtx.On("UserID").Return("user123")
	mockCtx.On("TenantID").Return("tenant123")
	mockCtx.On("ClientIP").Return("192.168.1.1")
	mockCtx.On("Logger").Return(mockLogger)
	mockCtx.On("Set", "audit_id", mock.AnythingOfType("string"))

	mockAuditor.On("StartAudit", mockCtx).Return("audit123")
	mockAuditor.On("LogRequest", "audit123", mock.AnythingOfType("*security.AuditRequest")).Return(nil)
	mockAuditor.On("LogSecurityEvent", "audit123", mock.AnythingOfType("*security.SecurityEvent")).Return(nil)

	// Create a critical violation
	violation := ComplianceViolation{
		ID:          "violation123",
		RuleID:      "rule123",
		Framework:   "SOC2",
		Severity:    "critical",
		Description: "Critical compliance violation",
		Timestamp:   time.Now(),
	}

	mockValidator.On("ValidateRequest", mockCtx, "SOC2").Return(&ComplianceResult{
		Compliant:  false,
		Framework:  "SOC2",
		Violations: []ComplianceViolation{violation},
		Timestamp:  time.Now(),
	}, nil)

	// Execute
	middleware := framework.ComplianceAudit()
	wrappedHandler := middleware(mockHandler)
	err := wrappedHandler.Handle(mockCtx)

	// Assert - should return error due to critical violation
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "compliance requirements")
	mockAuditor.AssertExpectations(t)
	mockValidator.AssertExpectations(t)
}

func TestComplianceFramework_IsFrameworkEnabled(t *testing.T) {
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2", "GDPR"},
	}

	framework := NewComplianceFramework("SOC2", config)

	assert.True(t, framework.IsFrameworkEnabled("SOC2"))
	assert.True(t, framework.IsFrameworkEnabled("GDPR"))
	assert.False(t, framework.IsFrameworkEnabled("HIPAA"))
}

func TestComplianceFramework_AddCustomRule(t *testing.T) {
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2"},
		CustomRules:       []ComplianceRule{},
	}

	framework := NewComplianceFramework("SOC2", config)

	rule := ComplianceRule{
		ID:          "custom_rule_1",
		Name:        "Custom Rule",
		Framework:   "SOC2",
		Severity:    "medium",
		Description: "Custom compliance rule",
	}

	framework.AddCustomRule(rule)

	rules := framework.GetCustomRules()
	assert.Len(t, rules, 1)
	assert.Equal(t, rule, rules[0])
}

func TestComplianceFramework_GetComplianceStatus(t *testing.T) {
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2"},
	}

	framework := NewComplianceFramework("SOC2", config)

	// Test without validator
	status, err := framework.GetComplianceStatus(context.Background())
	assert.Error(t, err)
	assert.Nil(t, status)

	// Test with validator
	mockValidator := &MockComplianceValidator{}
	framework.SetValidator(mockValidator)

	status, err = framework.GetComplianceStatus(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.True(t, status.Compliant)
	assert.Equal(t, "SOC2", status.Framework)
}

func TestComplianceFramework_HasCriticalViolations(t *testing.T) {
	framework := &ComplianceFramework{}

	tests := []struct {
		name       string
		violations []ComplianceViolation
		expected   bool
	}{
		{
			name:       "no violations",
			violations: []ComplianceViolation{},
			expected:   false,
		},
		{
			name: "low severity violations",
			violations: []ComplianceViolation{
				{Severity: "low"},
				{Severity: "medium"},
			},
			expected: false,
		},
		{
			name: "high severity violation",
			violations: []ComplianceViolation{
				{Severity: "low"},
				{Severity: "high"},
			},
			expected: true,
		},
		{
			name: "critical severity violation",
			violations: []ComplianceViolation{
				{Severity: "medium"},
				{Severity: "critical"},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := framework.hasCriticalViolations(tt.violations)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestComplianceFramework_SanitizeHeaders(t *testing.T) {
	framework := &ComplianceFramework{}

	headers := map[string][]string{
		"Authorization": {"Bearer token123"},
		"Cookie":        {"session=abc123"},
		"X-API-Key":     {"key123"},
		"Content-Type":  {"application/json"},
		"User-Agent":    {"test-agent"},
	}

	sanitized := framework.sanitizeHeaders(headers)

	assert.Equal(t, "[REDACTED]", sanitized["Authorization"])
	assert.Equal(t, "[REDACTED]", sanitized["Cookie"])
	assert.Equal(t, "[REDACTED]", sanitized["X-API-Key"])
	assert.Equal(t, "application/json", sanitized["Content-Type"])
	assert.Equal(t, "test-agent", sanitized["User-Agent"])
}

func TestComplianceFramework_SanitizeQueryParams(t *testing.T) {
	framework := &ComplianceFramework{}

	params := map[string][]string{
		"token":    {"secret123"},
		"api_key":  {"key123"},
		"password": {"pass123"},
		"query":    {"search term"},
		"limit":    {"10"},
	}

	sanitized := framework.sanitizeQueryParams(params)

	assert.Equal(t, "[REDACTED]", sanitized["token"])
	assert.Equal(t, "[REDACTED]", sanitized["api_key"])
	assert.Equal(t, "[REDACTED]", sanitized["password"])
	assert.Equal(t, "search term", sanitized["query"])
	assert.Equal(t, "10", sanitized["limit"])
}

func TestComplianceFramework_MarshalJSON(t *testing.T) {
	config := ComplianceConfig{
		EnabledFrameworks: []string{"SOC2"},
		AuditRetention:    24 * time.Hour,
	}

	framework := NewComplianceFramework("SOC2", config)

	data, err := framework.MarshalJSON()
	assert.NoError(t, err)
	assert.Contains(t, string(data), "SOC2")
	assert.Contains(t, string(data), "enabled_frameworks")
}
