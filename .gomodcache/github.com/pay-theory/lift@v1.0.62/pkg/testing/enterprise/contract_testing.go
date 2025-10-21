package enterprise

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Note: JobStatus is defined in infrastructure.go

// ContractTestingFramework provides contract testing capabilities
type ContractTestingFramework struct {
	registry  *ContractRegistry
	validator *BasicContractValidator
	runner    *ContractTestRunner
	reporter  *ContractTestReporter
	config    *ContractTestingConfig
}

// ContractTestingConfig configures contract testing
type ContractTestingConfig struct {
	Environment       map[string]any `json:"environment"`
	ReportFormat      string         `json:"report_format"`
	DefaultTimeout    time.Duration  `json:"default_timeout"`
	MaxRetries        int            `json:"max_retries"`
	ParallelExecution bool           `json:"parallel_execution"`
}

// BasicContractValidator implements ContractValidator interface
type BasicContractValidator struct {
	config  *ValidationConfig
	metrics *ValidationMetrics
	rules   []ValidationRule
}

// ValidationConfig configures contract validation
type ValidationConfig struct {
	Timeout         time.Duration `json:"timeout"`
	MaxErrors       int           `json:"max_errors"`
	StrictMode      bool          `json:"strict_mode"`
	FailFast        bool          `json:"fail_fast"`
	ValidateHeaders bool          `json:"validate_headers"`
	ValidateBody    bool          `json:"validate_body"`
}

// ValidationMetrics tracks validation performance
type ValidationMetrics struct {
	LastValidation        time.Time     `json:"last_validation"`
	TotalValidations      int64         `json:"total_validations"`
	SuccessfulValidations int64         `json:"successful_validations"`
	FailedValidations     int64         `json:"failed_validations"`
	AverageTime           time.Duration `json:"average_time"`
}

// NewContractTestingFramework creates a new contract testing framework
func NewContractTestingFramework(config *ContractTestConfig) *ContractTestingFramework {
	if config == nil {
		config = &ContractTestConfig{
			Environment:    "test",
			Timeout:        30 * time.Second,
			RetryAttempts:  3,
			RetryDelay:     1 * time.Second,
			StrictMode:     false,
			Parallel:       true,
			MaxConcurrency: 5,
		}
	}

	return &ContractTestingFramework{
		registry:  NewContractRegistry(nil),
		validator: NewBasicContractValidator(),
		runner:    NewContractTestRunner(nil),
		reporter:  NewContractTestReporter(),
		config: &ContractTestingConfig{
			DefaultTimeout:    config.Timeout,
			MaxRetries:        config.RetryAttempts,
			ParallelExecution: config.Parallel,
			ReportFormat:      "json",
			Environment:       make(map[string]any),
		},
	}
}

// NewBasicContractValidator creates a new basic contract validator
func NewBasicContractValidator() *BasicContractValidator {
	return &BasicContractValidator{
		rules: []ValidationRule{},
		config: &ValidationConfig{
			StrictMode:      false,
			Timeout:         10 * time.Second,
			MaxErrors:       10,
			FailFast:        false,
			ValidateHeaders: true,
			ValidateBody:    true,
		},
		metrics: &ValidationMetrics{
			LastValidation: time.Now(),
		},
	}
}

// Validate validates a contract interaction
func (v *BasicContractValidator) Validate(_ context.Context, interaction *ContractInteraction) error {
	start := time.Now()
	defer func() {
		v.updateMetrics(time.Since(start))
	}()

	// Validate request
	if err := v.validateRequest(interaction.Request); err != nil {
		v.metrics.FailedValidations++
		return fmt.Errorf("request validation failed: %w", err)
	}

	// Validate response
	if err := v.validateResponse(interaction.Response); err != nil {
		v.metrics.FailedValidations++
		return fmt.Errorf("response validation failed: %w", err)
	}

	v.metrics.SuccessfulValidations++
	return nil
}

// validateRequest validates an interaction request
func (v *BasicContractValidator) validateRequest(request *InteractionRequest) error {
	if request == nil {
		return fmt.Errorf("request cannot be nil")
	}

	if request.Method == "" {
		return fmt.Errorf("request method cannot be empty")
	}

	if request.Path == "" {
		return fmt.Errorf("request path cannot be empty")
	}

	return nil
}

// validateResponse validates an interaction response
func (v *BasicContractValidator) validateResponse(response *InteractionResponse) error {
	if response == nil {
		return fmt.Errorf("response cannot be nil")
	}

	if response.Status < 100 || response.Status > 599 {
		return fmt.Errorf("invalid response status: %d", response.Status)
	}

	return nil
}

// updateMetrics updates validation metrics
func (v *BasicContractValidator) updateMetrics(duration time.Duration) {
	v.metrics.TotalValidations++

	// Calculate average time
	if v.metrics.TotalValidations > 0 {
		totalTime := v.metrics.AverageTime * time.Duration(v.metrics.TotalValidations-1)
		v.metrics.AverageTime = (totalTime + duration) / time.Duration(v.metrics.TotalValidations)
	} else {
		v.metrics.AverageTime = duration
	}

	v.metrics.LastValidation = time.Now()
}

// CreateContractTest creates a new contract test
func (f *ContractTestingFramework) CreateContractTest(contract *ServiceContract, validator ContractValidator) (*ContractTest, error) {
	if contract == nil {
		return nil, fmt.Errorf("contract cannot be nil")
	}

	if validator == nil {
		validator = f.validator
	}

	test := &ContractTest{
		Provider:  contract.Provider.Name,
		Consumer:  contract.Consumer.Name,
		Contract:  contract,
		Validator: validator,
	}

	return test, nil
}

// RunContractTest executes a contract test
func (f *ContractTestingFramework) RunContractTest(ctx context.Context, test *ContractTest) (*ContractTestResult, error) {
	startTime := time.Now()

	// Validate contract
	if _, err := test.Validator.ValidateContract(ctx, test.Contract); err != nil {
		return &ContractTestResult{
			ContractID: test.Contract.ID,
			Provider:   test.Provider,
			Consumer:   test.Consumer,
			StartTime:  startTime,
			EndTime:    time.Now(),
			Duration:   time.Since(startTime),
			Status:     TestStatusFailed,
		}, err
	}

	return &ContractTestResult{
		ContractID: test.Contract.ID,
		Provider:   test.Provider,
		Consumer:   test.Consumer,
		StartTime:  startTime,
		EndTime:    time.Now(),
		Duration:   time.Since(startTime),
		Status:     TestStatusPassed,
	}, nil
}

// NewContractRegistry creates a new contract registry
func NewContractRegistry(_ *ContractTestConfig) *ContractRegistry {
	return &ContractRegistry{
		contracts: make(map[string]*ServiceContract),
		versions:  make(map[string][]string),
	}
}

// NewContractTestRunner creates a new contract test runner
func NewContractTestRunner(config *ContractTestConfig) *ContractTestRunner {
	return &ContractTestRunner{
		registry: NewContractRegistry(config),
		config: &TestConfig{
			Timeout:  30 * time.Second,
			Retries:  3,
			Parallel: true,
		},
	}
}

// NewContractTestReporter creates a new contract test reporter
func NewContractTestReporter() *ContractTestReporter {
	return &ContractTestReporter{
		templates: make(map[string]*ContractReportTemplate),
		exporters: make(map[string]ContractReportExporter),
	}
}

// ValidateContract validates a contract (implementing ContractValidator interface)
func (v *BasicContractValidator) ValidateContract(ctx context.Context, contract *ServiceContract) (*TestResult, error) {
	start := time.Now()

	if contract == nil {
		return &TestResult{
			TestID:    fmt.Sprintf("validation-%d", time.Now().Unix()),
			Status:    TestStatusFailed,
			StartTime: start,
			EndTime:   time.Now(),
			Duration:  time.Since(start),
			Passed:    false,
			Errors:    []string{"contract cannot be nil"},
		}, fmt.Errorf("contract cannot be nil")
	}

	// Validate all interactions
	for _, interaction := range contract.Interactions {
		if err := v.Validate(ctx, &interaction); err != nil {
			return &TestResult{
				TestID:    fmt.Sprintf("validation-%d", time.Now().Unix()),
				Status:    TestStatusFailed,
				StartTime: start,
				EndTime:   time.Now(),
				Duration:  time.Since(start),
				Passed:    false,
				Errors:    []string{err.Error()},
			}, err
		}
	}

	return &TestResult{
		TestID:    fmt.Sprintf("validation-%d", time.Now().Unix()),
		Status:    TestStatusPassed,
		StartTime: start,
		EndTime:   time.Now(),
		Duration:  time.Since(start),
		Passed:    true,
		Errors:    []string{},
	}, nil
}

// ValidateInteraction validates a contract interaction (implementing ContractValidator interface)
func (v *BasicContractValidator) ValidateInteraction(ctx context.Context, interaction *ContractInteraction) (*InteractionResult, error) {
	if err := v.Validate(ctx, interaction); err != nil {
		return &InteractionResult{
			InteractionID: interaction.ID,
			Status:        TestStatusFailed,
			Request:       interaction.Request,
			Response:      interaction.Response,
			Expected:      interaction.Response,
			Errors:        []string{err.Error()},
		}, err
	}

	return &InteractionResult{
		InteractionID: interaction.ID,
		Status:        TestStatusPassed,
		Request:       interaction.Request,
		Response:      interaction.Response,
		Expected:      interaction.Response,
		Errors:        []string{},
	}, nil
}

// ValidateContract validates a complete service contract (implementing ContractTestingFramework method)
func (f *ContractTestingFramework) ValidateContract(_ context.Context, contract *ServiceContract) (*ContractValidationResult, error) {
	startTime := time.Now()

	result := &ContractValidationResult{
		ID:          fmt.Sprintf("validation-%d", time.Now().Unix()),
		ContractID:  contract.ID,
		Status:      TestStatusPassed,
		Errors:      []string{},
		Warnings:    []string{},
		Validations: make(map[string]*InteractionValidation),
		Timestamp:   startTime,
		Metadata:    make(map[string]any),
	}

	// Validate each interaction
	for _, interaction := range contract.Interactions {
		validation := f.validateInteraction(&interaction)
		result.Validations[interaction.ID] = &validation

		if validation.Status == string(JobFailed) {
			result.Status = TestStatusFailed
			result.Errors = append(result.Errors, validation.Errors...)
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// validateInteraction validates a single contract interaction
func (f *ContractTestingFramework) validateInteraction(interaction *ContractInteraction) InteractionValidation {
	start := time.Now()
	validation := InteractionValidation{
		InteractionID: interaction.ID,
		Status:        string(PassedStatus),
		Checks:        make(map[string]*ValidationCheck),
		Errors:        []string{},
		Warnings:      []string{},
		Duration:      0,
		Timestamp:     start,
	}

	// Validate request
	if interaction.Request != nil {
		check := f.validateHTTPMethod(interaction.Request.Method)
		validation.Checks["http_method"] = check
		if !check.Valid {
			validation.Status = string(JobFailed)
			validation.Errors = append(validation.Errors, check.Errors...)
		}

		check = f.validateHTTPPath(interaction.Request.Path)
		validation.Checks["http_path"] = check
		if !check.Valid {
			validation.Status = string(JobFailed)
			validation.Errors = append(validation.Errors, check.Errors...)
		}

		check = f.validateHeaders(interaction.Request.Headers)
		validation.Checks["request_headers"] = check
		if !check.Valid {
			validation.Status = string(JobFailed)
			validation.Errors = append(validation.Errors, check.Errors...)
		}
	}

	// Validate response
	if interaction.Response != nil {
		check := f.validateHeaders(interaction.Response.Headers)
		validation.Checks["response_headers"] = check
		if !check.Valid {
			validation.Status = string(JobFailed)
			validation.Errors = append(validation.Errors, check.Errors...)
		}
	}

	validation.Duration = time.Since(start)
	return validation
}

// validateHTTPMethod validates HTTP method
func (f *ContractTestingFramework) validateHTTPMethod(method string) *ValidationCheck {
	check := &ValidationCheck{
		ID:          fmt.Sprintf("method-%d", time.Now().Unix()),
		Name:        "HTTP Method Validation",
		Description: "Validates HTTP method",
		Status:      string(PassedStatus),
		Valid:       true,
		Expected:    "Valid HTTP method",
		Actual:      method,
		Errors:      []string{},
		Warnings:    []string{},
		Metadata:    make(map[string]any),
	}

	validMethods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}
	for _, valid := range validMethods {
		if method == valid {
			return check
		}
	}

	check.Valid = false
	check.Status = string(JobFailed)
	check.Errors = append(check.Errors, fmt.Sprintf("Invalid HTTP method: %s", method))
	return check
}

// validateHTTPPath validates HTTP path
func (f *ContractTestingFramework) validateHTTPPath(path string) *ValidationCheck {
	check := &ValidationCheck{
		ID:          fmt.Sprintf("path-%d", time.Now().Unix()),
		Name:        "HTTP Path Validation",
		Description: "Validates HTTP path",
		Status:      string(PassedStatus),
		Valid:       true,
		Expected:    "Valid HTTP path starting with /",
		Actual:      path,
		Errors:      []string{},
		Warnings:    []string{},
		Metadata:    make(map[string]any),
	}

	if path == "" {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, "Path cannot be empty")
		return check
	}

	if !strings.HasPrefix(path, "/") {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, "Path must start with /")
		return check
	}

	return check
}

// validateHeaders validates HTTP headers
func (f *ContractTestingFramework) validateHeaders(headers map[string]string) *ValidationCheck {
	check := &ValidationCheck{
		ID:          fmt.Sprintf("headers-%d", time.Now().Unix()),
		Name:        "HTTP Headers Validation",
		Description: "Validates HTTP headers",
		Status:      string(PassedStatus),
		Valid:       true,
		Expected:    "Valid HTTP headers",
		Actual:      headers,
		Errors:      []string{},
		Warnings:    []string{},
		Metadata:    make(map[string]any),
	}

	for name, value := range headers {
		if name == "" {
			check.Valid = false
			check.Status = string(JobFailed)
			check.Errors = append(check.Errors, "Header name cannot be empty")
		}
		if value == "" {
			check.Valid = false
			check.Status = string(JobFailed)
			check.Errors = append(check.Errors, fmt.Sprintf("Header value cannot be empty for %s", name))
		}
	}

	return check
}

// validateSchema validates data against a schema definition
func (f *ContractTestingFramework) validateSchema(data any, schema *SchemaDefinition) (*ValidationCheck, error) {
	check := &ValidationCheck{
		ID:          fmt.Sprintf("schema-%d", time.Now().Unix()),
		Name:        "Schema Validation",
		Description: "Validates data against schema definition",
		Status:      string(PassedStatus),
		Valid:       true,
		Expected:    "Data conforms to schema",
		Actual:      data,
		Errors:      []string{},
		Warnings:    []string{},
		Metadata:    make(map[string]any),
	}

	if schema == nil {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, "Schema definition is required")
		return check, nil
	}

	// Type-specific validations (delegated to helpers to reduce complexity)
	switch schema.Type {
	case "string":
		validateStringSchema(data, schema, check)
	case "number", "integer":
		validateNumberSchema(data, schema, check)
	case "object":
		validateObjectSchemaRequired(data, schema, check)
	}

	return check, nil
}

// helper: validate string constraints
func validateStringSchema(data any, schema *SchemaDefinition, check *ValidationCheck) {
	str, ok := data.(string)
	if !ok {
		return
	}
	if schema.MinLength != nil && len(str) < *schema.MinLength {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, fmt.Sprintf("String too short: %d < %d", len(str), *schema.MinLength))
	}
	if schema.MaxLength != nil && len(str) > *schema.MaxLength {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, fmt.Sprintf("String too long: %d > %d", len(str), *schema.MaxLength))
	}
}

// helper: validate numeric constraints
func validateNumberSchema(data any, schema *SchemaDefinition, check *ValidationCheck) {
	num, ok := data.(float64)
	if !ok {
		return
	}
	if schema.Minimum != nil && num < *schema.Minimum {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, fmt.Sprintf("Number too small: %f < %f", num, *schema.Minimum))
	}
	if schema.Maximum != nil && num > *schema.Maximum {
		check.Valid = false
		check.Status = string(JobFailed)
		check.Errors = append(check.Errors, fmt.Sprintf("Number too large: %f > %f", num, *schema.Maximum))
	}
}

// helper: validate required fields for object
func validateObjectSchemaRequired(data any, schema *SchemaDefinition, check *ValidationCheck) {
	obj, ok := data.(map[string]any)
	if !ok {
		return
	}
	for _, required := range schema.Required {
		if _, exists := obj[required]; !exists {
			check.Valid = false
			check.Status = string(JobFailed)
			check.Errors = append(check.Errors, fmt.Sprintf("Missing required field: %s", required))
		}
	}
}

// Contract Testing Implementation leverages existing framework

// validateType validates that a value matches the expected type
func (f *ContractTestingFramework) validateType(value any, expectedType string) error {
	switch expectedType {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("expected string, got %T", value)
		}
	case "number":
		switch value.(type) {
		case int, int32, int64, float32, float64, uint, uint32, uint64:
			// valid number types
		default:
			return fmt.Errorf("expected number, got %T", value)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("expected boolean, got %T", value)
		}
	case "object":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("expected object, got %T", value)
		}
	case "array":
		switch value.(type) {
		case []any, []string, []int, []map[string]any:
			// valid array types
		default:
			return fmt.Errorf("expected array, got %T", value)
		}
	default:
		return fmt.Errorf("unknown type: %s", expectedType)
	}
	return nil
}

// generateValidationSummary generates a summary of contract validation results
func (f *ContractTestingFramework) generateValidationSummary(results []*ContractValidationResult) string {
	total := len(results)
	passed := 0
	failed := 0

	for _, result := range results {
		if result.Status == TestStatusPassed {
			passed++
		} else {
			failed++
		}
	}

	return fmt.Sprintf("Contract Validation Summary: Total=%d, Passed=%d, Failed=%d", total, passed, failed)
}

// calculateValidationStatus calculates the overall validation status
func (f *ContractTestingFramework) calculateValidationStatus(validations map[string]*InteractionValidation) TestStatus {
	if len(validations) == 0 {
		return TestStatus("unknown")
	}
	for _, validation := range validations {
		if validation.Status == string(JobFailed) {
			return TestStatusFailed
		}
	}
	return TestStatusPassed
}

// calculateInteractionStatus calculates the status of an interaction
func (f *ContractTestingFramework) calculateInteractionStatus(checks map[string]*ValidationCheck) TestStatus {
	if len(checks) == 0 {
		return TestStatus("unknown")
	}
	for _, check := range checks {
		if check.Status == string(JobFailed) {
			return TestStatusFailed
		}
	}
	return TestStatusPassed
}

// Reference certain internal helpers to avoid unused warnings in builds
var (
	_ = (*ContractTestingFramework).validateSchema
	_ = validateStringSchema
	_ = validateNumberSchema
	_ = validateObjectSchemaRequired
	_ = (*ContractTestingFramework).validateType
	_ = (*ContractTestingFramework).generateValidationSummary
	_ = (*ContractTestingFramework).calculateValidationStatus
	_ = (*ContractTestingFramework).calculateInteractionStatus
)
