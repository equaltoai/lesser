package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/theory-cloud/tabletheory/pkg/model"
	"github.com/theory-cloud/tabletheory/pkg/schema"
	"github.com/theory-cloud/tabletheory/pkg/session"
	"go.uber.org/zap"
)

// secretEnvVarNames is the set of environment variable names known to carry
// secret material. Any validation message that includes the value of one of
// these variables must use RedactedSecretSentinel instead of the raw value.
var secretEnvVarNames = map[string]bool{
	"JWT_SECRET":                   true,
	"OAUTH_CLIENT_SECRET":          true,
	"PRIVATE_KEY_SECRET":           true,
	"ENCRYPTION_KEY":               true,
	"REPUTATION_PRIVATE_KEY":       true,
	"PRIVACY_MASTER_KEY":           true,
	"INSTANCE_API_KEY":             true,
	"LESSER_HOST_INSTANCE_KEY":     true,
	"CLOUDFRONT_PRIVATE_KEY_PATH":  true,
	"DYNAMODB_ENCRYPTION_KEY":      true,
	"ACTOR_PRIVATE_KEY_ENCRYPTION": true,
	"VAPID_PUBLIC_KEY":             false, // public key OK to show
	"SYSTEM_ACTOR_PUBLIC_KEY":      false, // public key OK to show
}

// isSecretEnvVar reports whether the named environment variable carries secret
// material that must not appear in validation output, recovery snapshots, or
// diagnostic logs.
func isSecretEnvVar(name string) bool {
	return secretEnvVarNames[strings.ToUpper(name)]
}

// safeEnvValue returns the environment variable value for diagnostic use,
// substituting RedactedSecretSentinel for any variable known to carry secrets.
func safeEnvValue(name string) string {
	v := os.Getenv(name)
	if isSecretEnvVar(name) {
		return RedactedSecretSentinel
	}
	return v
}

type tableExistsChecker interface {
	TableExists(tableName string) (bool, error)
}

type s3Client interface {
	HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error)
}

type secretsManagerClient interface {
	DescribeSecret(ctx context.Context, params *secretsmanager.DescribeSecretInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.DescribeSecretOutput, error)
}

var (
	loadDefaultAWSConfig    = awsconfig.LoadDefaultConfig
	newS3Client             = func(cfg aws.Config) s3Client { return s3.NewFromConfig(cfg) }
	newSecretsManagerClient = func(cfg aws.Config) secretsManagerClient { return secretsmanager.NewFromConfig(cfg) }
	newTableExistsChecker   = func(cfg aws.Config) (tableExistsChecker, error) {
		sess, err := session.NewSession(&session.Config{
			Region:              cfg.Region,
			CredentialsProvider: cfg.Credentials,
		})
		if err != nil {
			return nil, err
		}
		registry := model.NewRegistry()
		return schema.NewManager(sess, registry), nil
	}
)

// ValidationResult represents the result of configuration validation
type ValidationResult struct {
	Valid     bool                `json:"valid"`
	Errors    []ValidationError   `json:"errors,omitempty"`
	Warnings  []ValidationWarning `json:"warnings,omitempty"`
	Summary   ValidationSummary   `json:"summary"`
	Resources ResourceValidation  `json:"resources"`
	Security  SecurityValidation  `json:"security"`
	Timestamp time.Time           `json:"timestamp"`
}

// ValidationError represents a configuration validation error.
// The Value field must never contain raw secret material; use
// RedactedSecretSentinel for any field known to hold secrets.
type ValidationError struct {
	Field       string `json:"field"`
	Value       string `json:"value,omitempty"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Remediation string `json:"remediation,omitempty"`
}

// ValidationWarning represents a configuration validation warning
type ValidationWarning struct {
	Field          string `json:"field"`
	Value          string `json:"value,omitempty"`
	Message        string `json:"message"`
	Recommendation string `json:"recommendation,omitempty"`
}

// ValidationSummary provides a summary of validation results
type ValidationSummary struct {
	TotalChecks    int `json:"total_checks"`
	PassedChecks   int `json:"passed_checks"`
	FailedChecks   int `json:"failed_checks"`
	WarningChecks  int `json:"warning_checks"`
	CriticalErrors int `json:"critical_errors"`
}

// ResourceValidation tracks AWS resource availability
type ResourceValidation struct {
	DynamoDB       ResourceStatus `json:"dynamodb"`
	S3             ResourceStatus `json:"s3"`
	SecretsManager ResourceStatus `json:"secrets_manager"`
	Lambda         ResourceStatus `json:"lambda"`
}

// SecurityValidation tracks security configuration status
type SecurityValidation struct {
	EncryptionKeys   SecurityStatus `json:"encryption_keys"`
	PrivateKeys      SecurityStatus `json:"private_keys"`
	OAuthSecrets     SecurityStatus `json:"oauth_secrets"`
	JWTConfiguration SecurityStatus `json:"jwt_configuration"`
	HTTPSEnforcement SecurityStatus `json:"https_enforcement"`
}

// ResourceStatus represents the status of an AWS resource
type ResourceStatus struct {
	Available bool   `json:"available"`
	Message   string `json:"message,omitempty"`
	Error     string `json:"error,omitempty"`
}

// SecurityStatus represents the status of a security configuration
type SecurityStatus struct {
	Configured bool   `json:"configured"`
	Valid      bool   `json:"valid"`
	Message    string `json:"message,omitempty"`
}

// ProductionConfigValidator validates production configuration
type ProductionConfigValidator struct {
	logger    *zap.Logger
	awsConfig aws.Config
	timeout   time.Duration
}

// NewProductionConfigValidator creates a new production configuration validator
func NewProductionConfigValidator(logger *zap.Logger) (*ProductionConfigValidator, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	awsConfig, err := loadDefaultAWSConfig(ctx)
	if err != nil {
		logger.Warn("failed to load AWS config for validation", zap.Error(err))
		// Continue without AWS validation
	}

	return &ProductionConfigValidator{
		logger:    logger,
		awsConfig: awsConfig,
		timeout:   30 * time.Second,
	}, nil
}

// ValidateProductionConfig validates all production configuration requirements
func (v *ProductionConfigValidator) ValidateProductionConfig(ctx context.Context) (*ValidationResult, error) {
	result := &ValidationResult{
		Valid:     true,
		Errors:    make([]ValidationError, 0),
		Warnings:  make([]ValidationWarning, 0),
		Timestamp: time.Now(),
	}

	// Validate required environment variables
	v.validateEnvironmentVariables(result)

	// Validate security settings
	v.validateSecurityConfiguration(result)

	// Validate AWS resources if config is available
	if v.awsConfig.Region != "" {
		v.validateAWSResources(ctx, result)
	}

	// Validate network and connectivity settings
	v.validateNetworkConfiguration(result)

	// Calculate summary
	v.calculateSummary(result)

	// Set overall validity
	result.Valid = len(result.Errors) == 0 || v.hasNoCriticalErrors(result)

	return result, nil
}

// validateEnvironmentVariables validates required environment variables
func (v *ProductionConfigValidator) validateEnvironmentVariables(result *ValidationResult) {
	requiredVars := map[string]string{
		"DOMAIN_NAME":        "The domain name for your Lesser instance",
		"AWS_REGION":         "AWS region for deploying resources",
		"PRIVATE_KEY_SECRET": "Secret name for ActivityPub signing key",
	}

	optionalVars := map[string]string{
		"OAUTH_CLIENT_ID":     "OAuth client ID for authentication",
		"OAUTH_CLIENT_SECRET": "OAuth client secret for authentication",
		"S3_BUCKET_NAME":      "S3 bucket for media storage",
		"LOG_LEVEL":           "Logging level (debug, info, warn, error)",
		"ENVIRONMENT":         "Environment name (production, staging, development)",
		"STAGE":               "Deployment stage (alias for ENVIRONMENT)",
	}

	// Check required variables
	for varName, description := range requiredVars {
		value := os.Getenv(varName)
		if value == "" {
			result.Errors = append(result.Errors, ValidationError{
				Field:       varName,
				Message:     fmt.Sprintf("Required environment variable %s is not set", varName),
				Severity:    "critical",
				Remediation: fmt.Sprintf("Set %s: %s", varName, description),
			})
		} else {
			// Validate specific formats — use redacted values for secret fields.
			safeVal := safeEnvValue(varName)
			v.validateEnvironmentVariableFormat(varName, safeVal, result)
		}
	}

	if _, err := resolveMainTableNameForValidation(); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Field:       "DYNAMODB_TABLE",
			Message:     "DynamoDB table name is not configured and cannot be derived",
			Severity:    "critical",
			Remediation: "Set DYNAMODB_TABLE/DYNAMO_TABLE_NAME or ENVIRONMENT/STAGE so Lesser can derive the canonical table name",
		})
	}

	// JWT secret can come from plaintext or ARN
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	jwtSecretARN := strings.TrimSpace(os.Getenv("JWT_SECRET_ARN"))
	if jwtSecret == "" && jwtSecretARN == "" {
		result.Errors = append(result.Errors, ValidationError{
			Field:       "JWT_SECRET",
			Message:     "JWT secret not configured (provide JWT_SECRET or JWT_SECRET_ARN)",
			Severity:    "critical",
			Remediation: "Set JWT_SECRET for plaintext or JWT_SECRET_ARN for Secrets Manager",
		})
	} else if jwtSecret != "" {
		v.validateEnvironmentVariableFormat("JWT_SECRET", safeEnvValue("JWT_SECRET"), result)
	}

	// Check optional but recommended variables
	for varName, description := range optionalVars {
		value := os.Getenv(varName)
		if value == "" {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:          varName,
				Message:        fmt.Sprintf("Optional environment variable %s is not set", varName),
				Recommendation: fmt.Sprintf("Consider setting %s: %s", varName, description),
			})
		}
	}
}

// validateEnvironmentVariableFormat validates specific environment variable formats
func (v *ProductionConfigValidator) validateEnvironmentVariableFormat(name, value string, result *ValidationResult) {
	switch name {
	case "DOMAIN_NAME":
		if !v.isValidDomain(value) {
			result.Errors = append(result.Errors, ValidationError{
				Field:       name,
				Value:       value,
				Message:     "Invalid domain name format",
				Severity:    "high",
				Remediation: "Ensure domain name follows proper DNS format (example.com)",
			})
		}
	case "AWS_REGION":
		if !v.isValidAWSRegion(value) {
			result.Errors = append(result.Errors, ValidationError{
				Field:       name,
				Value:       value,
				Message:     "Invalid AWS region format",
				Severity:    "high",
				Remediation: "Use valid AWS region format (e.g., us-east-1, eu-west-1)",
			})
		}
	case "LOG_LEVEL":
		validLevels := []string{"debug", "info", "warn", "error", "fatal"}
		if !v.isValueInList(strings.ToLower(value), validLevels) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:          name,
				Value:          value,
				Message:        "Non-standard log level",
				Recommendation: fmt.Sprintf("Use one of: %s", strings.Join(validLevels, ", ")),
			})
		}
	case "ENVIRONMENT":
		validEnvs := []string{"production", "staging", "development", "test"}
		if !v.isValueInList(strings.ToLower(value), validEnvs) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:          name,
				Value:          value,
				Message:        "Non-standard environment name",
				Recommendation: fmt.Sprintf("Consider using: %s", strings.Join(validEnvs, ", ")),
			})
		}
	case "STAGE":
		validStages := []string{"production", "staging", "development", "test", "prod", "stage", "dev"}
		if !v.isValueInList(strings.ToLower(value), validStages) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:          name,
				Value:          value,
				Message:        "Non-standard stage name",
				Recommendation: fmt.Sprintf("Consider using: %s", strings.Join(validStages, ", ")),
			})
		}
	}
}

// validateSecurityConfiguration validates security settings
func (v *ProductionConfigValidator) validateSecurityConfiguration(result *ValidationResult) {
	// Check HTTPS enforcement
	result.Security.HTTPSEnforcement = v.validateHTTPSEnforcement()

	// Check JWT configuration
	result.Security.JWTConfiguration = v.validateJWTConfiguration()

	// Check OAuth secrets
	result.Security.OAuthSecrets = v.validateOAuthSecrets()

	// Check encryption keys
	result.Security.EncryptionKeys = v.validateEncryptionKeys()

	// Check private keys
	result.Security.PrivateKeys = v.validatePrivateKeys()
}

// validateHTTPSEnforcement validates HTTPS enforcement configuration
func (v *ProductionConfigValidator) validateHTTPSEnforcement() SecurityStatus {
	domainName := os.Getenv("DOMAIN_NAME")
	if domainName == "" {
		return SecurityStatus{
			Configured: false,
			Valid:      false,
			Message:    "Domain name not configured",
		}
	}

	// Check if domain starts with https
	if strings.HasPrefix(strings.ToLower(domainName), "http://") {
		return SecurityStatus{
			Configured: true,
			Valid:      false,
			Message:    "Domain configured with HTTP instead of HTTPS",
		}
	}

	return SecurityStatus{
		Configured: true,
		Valid:      true,
		Message:    "HTTPS enforcement properly configured",
	}
}

// validateJWTConfiguration validates JWT configuration
func (v *ProductionConfigValidator) validateJWTConfiguration() SecurityStatus {
	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	jwtSecretARN := strings.TrimSpace(os.Getenv("JWT_SECRET_ARN"))

	if jwtSecret == "" && jwtSecretARN == "" {
		return SecurityStatus{
			Configured: false,
			Valid:      false,
			Message:    "JWT secret not configured",
		}
	}

	if jwtSecret == "" && jwtSecretARN != "" {
		return SecurityStatus{
			Configured: true,
			Valid:      true,
			Message:    "JWT secret configured via ARN",
		}
	}

	// Check JWT secret strength
	if len(jwtSecret) < 32 {
		return SecurityStatus{
			Configured: true,
			Valid:      false,
			Message:    "JWT secret is too short (minimum 32 characters)",
		}
	}

	return SecurityStatus{
		Configured: true,
		Valid:      true,
		Message:    "JWT configuration is valid",
	}
}

// validateOAuthSecrets validates OAuth configuration
func (v *ProductionConfigValidator) validateOAuthSecrets() SecurityStatus {
	clientID := os.Getenv("OAUTH_CLIENT_ID")
	clientSecret := os.Getenv("OAUTH_CLIENT_SECRET")

	if clientID == "" && clientSecret == "" {
		return SecurityStatus{
			Configured: false,
			Valid:      true, // OAuth is optional
			Message:    "OAuth not configured (optional)",
		}
	}

	if clientID == "" || clientSecret == "" {
		return SecurityStatus{
			Configured: true,
			Valid:      false,
			Message:    "OAuth partially configured - need both client ID and secret",
		}
	}

	return SecurityStatus{
		Configured: true,
		Valid:      true,
		Message:    "OAuth configuration is valid",
	}
}

// validateEncryptionKeys validates encryption key configuration
func (v *ProductionConfigValidator) validateEncryptionKeys() SecurityStatus {
	// Check for encryption key environment variable
	encKey := os.Getenv("ENCRYPTION_KEY")
	if encKey == "" {
		return SecurityStatus{
			Configured: false,
			Valid:      false,
			Message:    "Encryption key not configured",
		}
	}

	// Basic validation of key length
	if len(encKey) < 32 {
		return SecurityStatus{
			Configured: true,
			Valid:      false,
			Message:    "Encryption key is too short (minimum 32 characters)",
		}
	}

	return SecurityStatus{
		Configured: true,
		Valid:      true,
		Message:    "Encryption key configuration is valid",
	}
}

// validatePrivateKeys validates ActivityPub private key configuration
func (v *ProductionConfigValidator) validatePrivateKeys() SecurityStatus {
	privateKeySecret := os.Getenv("PRIVATE_KEY_SECRET")
	if privateKeySecret == "" {
		return SecurityStatus{
			Configured: false,
			Valid:      false,
			Message:    "ActivityPub private key secret not configured",
		}
	}

	return SecurityStatus{
		Configured: true,
		Valid:      true,
		Message:    "Private key configuration is valid",
	}
}

// validateAWSResources validates AWS resource availability
func (v *ProductionConfigValidator) validateAWSResources(ctx context.Context, result *ValidationResult) {
	ctx, cancel := context.WithTimeout(ctx, v.timeout)
	defer cancel()

	// Validate DynamoDB
	result.Resources.DynamoDB = v.validateDynamoDB(ctx)

	// Validate S3
	result.Resources.S3 = v.validateS3(ctx)

	// Validate Secrets Manager
	result.Resources.SecretsManager = v.validateSecretsManager(ctx)
}

// validateDynamoDB validates DynamoDB table availability
func (v *ProductionConfigValidator) validateDynamoDB(ctx context.Context) ResourceStatus {
	_ = ctx

	tableName, err := resolveMainTableNameForValidation()
	if err != nil {
		return ResourceStatus{
			Available: false,
			Error:     err.Error(),
		}
	}

	checker, err := newTableExistsChecker(v.awsConfig)
	if err != nil {
		return ResourceStatus{
			Available: false,
			Error:     fmt.Sprintf("failed to initialize TableTheory DynamoDB client: %v", err),
		}
	}

	exists, err := checker.TableExists(tableName)

	if err != nil {
		return ResourceStatus{
			Available: false,
			Error:     fmt.Sprintf("DynamoDB table '%s' not accessible: %v", tableName, err),
		}
	}

	if !exists {
		return ResourceStatus{
			Available: false,
			Error:     fmt.Sprintf("DynamoDB table '%s' not found", tableName),
		}
	}

	return ResourceStatus{
		Available: true,
		Message:   fmt.Sprintf("DynamoDB table '%s' is available", tableName),
	}
}

// validateS3 validates S3 bucket availability
func (v *ProductionConfigValidator) validateS3(ctx context.Context) ResourceStatus {
	bucketName := os.Getenv("S3_BUCKET_NAME")
	if bucketName == "" {
		bucketName = os.Getenv("S3_BUCKET")
	}
	if bucketName == "" {
		return ResourceStatus{
			Available: false,
			Message:   "S3 bucket not configured (optional but recommended)",
		}
	}

	client := newS3Client(v.awsConfig)
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucketName),
	})

	if err != nil {
		return ResourceStatus{
			Available: false,
			Error:     fmt.Sprintf("S3 bucket '%s' not accessible: %v", bucketName, err),
		}
	}

	return ResourceStatus{
		Available: true,
		Message:   fmt.Sprintf("S3 bucket '%s' is available", bucketName),
	}
}

// validateSecretsManager validates Secrets Manager access
func (v *ProductionConfigValidator) validateSecretsManager(ctx context.Context) ResourceStatus {
	secretName := os.Getenv("PRIVATE_KEY_SECRET")
	if secretName == "" {
		return ResourceStatus{
			Available: false,
			Error:     "Private key secret name not configured",
		}
	}

	client := newSecretsManagerClient(v.awsConfig)
	_, err := client.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
		SecretId: aws.String(secretName),
	})

	if err != nil {
		return ResourceStatus{
			Available: false,
			Error:     fmt.Sprintf("Secret '%s' not accessible: %v", secretName, err),
		}
	}

	return ResourceStatus{
		Available: true,
		Message:   fmt.Sprintf("Secret '%s' is available", secretName),
	}
}

// validateNetworkConfiguration validates network and connectivity settings
func (v *ProductionConfigValidator) validateNetworkConfiguration(result *ValidationResult) {
	// Validate domain accessibility
	domainName := os.Getenv("DOMAIN_NAME")
	if domainName != "" {
		if !v.isDomainAccessible(domainName) {
			result.Warnings = append(result.Warnings, ValidationWarning{
				Field:          "DOMAIN_NAME",
				Value:          domainName,
				Message:        "Domain may not be accessible or properly configured",
				Recommendation: "Ensure DNS records are properly configured and domain is accessible",
			})
		}
	}

	// Validate port configurations
	v.validatePortConfiguration(result)
}

// validatePortConfiguration validates port configuration
func (v *ProductionConfigValidator) validatePortConfiguration(result *ValidationResult) {
	portStr := os.Getenv("PORT")
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			result.Errors = append(result.Errors, ValidationError{
				Field:       "PORT",
				Value:       portStr,
				Message:     "Invalid port number",
				Severity:    "medium",
				Remediation: "Use a valid port number between 1 and 65535",
			})
		}
	}
}

// Helper methods

// isValidDomain validates domain name format
func (v *ProductionConfigValidator) isValidDomain(domain string) bool {
	if domain == "" {
		return false
	}

	// Remove protocol if present
	if strings.HasPrefix(domain, "http://") || strings.HasPrefix(domain, "https://") {
		u, err := url.Parse(domain)
		if err != nil {
			return false
		}
		domain = u.Host
	}

	// Basic domain validation
	parts := strings.Split(domain, ".")
	return len(parts) >= 2 && !strings.Contains(domain, " ")
}

// isValidAWSRegion validates AWS region format
func (v *ProductionConfigValidator) isValidAWSRegion(region string) bool {
	validRegions := []string{
		"us-east-1", "us-east-2", "us-west-1", "us-west-2",
		"eu-west-1", "eu-west-2", "eu-west-3", "eu-central-1",
		"ap-southeast-1", "ap-southeast-2", "ap-northeast-1",
		"ap-northeast-2", "ap-south-1", "sa-east-1",
		"ca-central-1", "eu-north-1", "ap-east-1",
	}
	return v.isValueInList(region, validRegions)
}

// isValueInList checks if value exists in list
func (v *ProductionConfigValidator) isValueInList(value string, list []string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// isDomainAccessible checks basic domain accessibility
func (v *ProductionConfigValidator) isDomainAccessible(domain string) bool {
	// This is a basic implementation - in production, you might want
	// to perform actual network connectivity tests
	return v.isValidDomain(domain)
}

// calculateSummary calculates validation summary statistics
func (v *ProductionConfigValidator) calculateSummary(result *ValidationResult) {
	result.Summary.TotalChecks = 15 // Approximate number of checks
	result.Summary.FailedChecks = len(result.Errors)
	result.Summary.WarningChecks = len(result.Warnings)
	result.Summary.PassedChecks = result.Summary.TotalChecks - result.Summary.FailedChecks - result.Summary.WarningChecks

	// Count critical errors
	for _, err := range result.Errors {
		if err.Severity == "critical" {
			result.Summary.CriticalErrors++
		}
	}
}

// hasNoCriticalErrors checks if there are no critical errors
func (v *ProductionConfigValidator) hasNoCriticalErrors(result *ValidationResult) bool {
	return result.Summary.CriticalErrors == 0
}

// QuickValidateProductionConfig performs a quick validation without AWS resource checks
func QuickValidateProductionConfig() error {
	requiredVars := []string{
		"DOMAIN_NAME",
		"AWS_REGION",
		"PRIVATE_KEY_SECRET",
	}

	var missingVars []string
	for _, varName := range requiredVars {
		if os.Getenv(varName) == "" {
			missingVars = append(missingVars, varName)
		}
	}

	jwtSecret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	jwtSecretARN := strings.TrimSpace(os.Getenv("JWT_SECRET_ARN"))
	if jwtSecret == "" && jwtSecretARN == "" {
		missingVars = append(missingVars, "JWT_SECRET or JWT_SECRET_ARN")
	}

	if _, err := resolveMainTableNameForValidation(); err != nil {
		missingVars = append(missingVars, "DYNAMODB_TABLE/DYNAMO_TABLE_NAME or ENVIRONMENT/STAGE")
	}

	if len(missingVars) > 0 {
		return fmt.Errorf("missing required environment variables: %s", strings.Join(missingVars, ", "))
	}

	return nil
}

func resolveMainTableNameForValidation() (string, error) {
	if table := resolveEnvFirst("DYNAMODB_TABLE", "DYNAMO_TABLE_NAME"); table != "" {
		return table, nil
	}

	env, envRaw, stage := resolveEnvironmentAndStage()
	derived := firstNonEmpty(env, envRaw, stage)
	if derived == "" {
		return "", fmt.Errorf("DYNAMODB_TABLE/DYNAMO_TABLE_NAME or ENVIRONMENT/STAGE must be set to determine the DynamoDB table name")
	}
	return fmt.Sprintf("lesser-%s", strings.ToLower(derived)), nil
}
