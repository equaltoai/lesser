package security

import (
	"context"
	"time"
)

// SecurityConfig defines the overall security configuration for the Lift framework
type SecurityConfig struct {
	SecretsProvider  SecretsProvider `json:"-"`
	KMSKeyID         string          `json:"kms_key_id"`
	JWTConfig        JWTConfig       `json:"jwt_config"`
	DefaultRoles     []string        `json:"default_roles"`
	APIKeyConfig     APIKeyConfig    `json:"api_key_config"`
	MaxRequestSize   int64           `json:"max_request_size"`
	RBACEnabled      bool            `json:"rbac_enabled"`
	TenantValidation bool            `json:"tenant_validation"`
	CrossAccountAuth bool            `json:"cross_account_auth"`
	EncryptionAtRest bool            `json:"encryption_at_rest"`
	RequestSigning   bool            `json:"request_signing"`
}

// JWTConfig configures JWT authentication
type JWTConfig struct {
	ValidateTenant  func(tenantID string) error `json:"-"`
	SigningMethod   string                      `json:"signing_method"`
	PublicKeyPath   string                      `json:"public_key_path"`
	PrivateKeyPath  string                      `json:"private_key_path"`
	SecretKey       string                      `json:"secret_key,omitempty"`
	Issuer          string                      `json:"issuer"`
	Audience        []string                    `json:"audience"`
	MaxAge          time.Duration               `json:"max_age"`
	RotationPeriod  time.Duration               `json:"rotation_period"`
	RequireTenantID bool                        `json:"require_tenant_id"`
	KeyRotation     bool                        `json:"key_rotation"`
}

// APIKeyConfig configures API key authentication
type APIKeyConfig struct {
	// Storage settings
	Provider  string `json:"provider"` // "secrets-manager", "parameter-store"
	KeyPrefix string `json:"key_prefix"`

	// Validation settings
	MinLength       int           `json:"min_length"`
	RequireRotation bool          `json:"require_rotation"`
	MaxAge          time.Duration `json:"max_age"`

	// Rate limiting for API keys
	RateLimit  int           `json:"rate_limit"`
	RatePeriod time.Duration `json:"rate_period"`
}

// RateLimitConfig defines rate limiting configuration
type RateLimitConfig struct {
	StorageConfig map[string]any `json:"storage_config"`
	StorageType   string         `json:"storage_type"`
	GlobalLimit   int            `json:"global_limit"`
	GlobalPeriod  time.Duration  `json:"global_period"`
	TenantLimit   int            `json:"tenant_limit"`
	TenantPeriod  time.Duration  `json:"tenant_period"`
	UserLimit     int            `json:"user_limit"`
	UserPeriod    time.Duration  `json:"user_period"`
	GlobalEnabled bool           `json:"global_enabled"`
	TenantEnabled bool           `json:"tenant_enabled"`
	UserEnabled   bool           `json:"user_enabled"`
}

// CORSConfig defines Cross-Origin Resource Sharing settings
type CORSConfig struct {
	ValidateOrigin   func(origin string) bool `json:"-"`
	AllowedOrigins   []string                 `json:"allowed_origins"`
	AllowedMethods   []string                 `json:"allowed_methods"`
	AllowedHeaders   []string                 `json:"allowed_headers"`
	ExposedHeaders   []string                 `json:"exposed_headers"`
	MaxAge           int                      `json:"max_age"`
	AllowCredentials bool                     `json:"allow_credentials"`
}

// RequestValidationConfig defines request validation settings
type RequestValidationConfig struct {
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
	AllowedCIDRs   []string `json:"allowed_cidrs"`
	DeniedCIDRs    []string `json:"denied_cidrs"`
	MaxBodySize    int64    `json:"max_body_size"`
	MaxHeaderSize  int      `json:"max_header_size"`
	ValidateJSON   bool     `json:"validate_json"`
	SanitizeInput  bool     `json:"sanitize_input"`
	EnableIPFilter bool     `json:"enable_ip_filter"`
}

// DefaultSecurityConfig returns a secure default configuration
func DefaultSecurityConfig() *SecurityConfig {
	return &SecurityConfig{
		JWTConfig: JWTConfig{
			SigningMethod:   "RS256",
			MaxAge:          time.Hour,
			RequireTenantID: true,
			KeyRotation:     true,
			RotationPeriod:  24 * time.Hour,
		},
		APIKeyConfig: APIKeyConfig{
			Provider:        "secrets-manager",
			KeyPrefix:       "lift/api-keys/",
			MinLength:       32,
			RequireRotation: true,
			MaxAge:          30 * 24 * time.Hour, // 30 days
			RateLimit:       1000,
			RatePeriod:      time.Hour,
		},
		RBACEnabled:      true,
		DefaultRoles:     []string{"user"},
		TenantValidation: true,
		CrossAccountAuth: false,
		EncryptionAtRest: true,
		RequestSigning:   false,
		MaxRequestSize:   10 * 1024 * 1024, // 10MB
	}
}

// Validate checks if the security configuration is valid
func (c *SecurityConfig) Validate() error {
	if c.JWTConfig.SigningMethod == "" {
		return NewSecurityError("INVALID_CONFIG", "JWT signing method is required")
	}

	if c.JWTConfig.SigningMethod == "HS256" && c.JWTConfig.SecretKey == "" {
		return NewSecurityError("INVALID_CONFIG", "Secret key is required for HS256 signing")
	}

	if c.JWTConfig.SigningMethod == "RS256" && (c.JWTConfig.PublicKeyPath == "" || c.JWTConfig.PrivateKeyPath == "") {
		return NewSecurityError("INVALID_CONFIG", "Public and private key paths are required for RS256 signing")
	}

	if c.MaxRequestSize <= 0 {
		return NewSecurityError("INVALID_CONFIG", "Max request size must be positive")
	}

	if c.APIKeyConfig.MinLength < 16 {
		return NewSecurityError("INVALID_CONFIG", "API key minimum length must be at least 16 characters")
	}

	return nil
}

// SecretsProvider defines the interface for secrets management
type SecretsProvider interface {
	GetSecret(ctx context.Context, name string) (string, error)
	PutSecret(ctx context.Context, name string, value string) error
	RotateSecret(ctx context.Context, name string) error
	DeleteSecret(ctx context.Context, name string) error
}

// SecurityError represents a security-related error
type SecurityError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *SecurityError) Error() string {
	return e.Message
}

// NewSecurityError creates a new security error
func NewSecurityError(code, message string) *SecurityError {
	return &SecurityError{
		Code:    code,
		Message: message,
	}
}
