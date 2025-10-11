package logging

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// SensitiveDataScrubber handles detection and redaction of sensitive information in logs
type SensitiveDataScrubber struct {
	patterns map[string]*regexp.Regexp
	enabled  bool
}

// NewSensitiveDataScrubber creates a new scrubber with comprehensive sensitive data patterns
func NewSensitiveDataScrubber() *SensitiveDataScrubber {
	patterns := make(map[string]*regexp.Regexp)
	
	// OAuth tokens and API keys
	patterns["bearer_token"] = regexp.MustCompile(`(?i)(bearer[\s]+)([a-zA-Z0-9._-]{20,})`)
	patterns["api_key"] = regexp.MustCompile(`(?i)(api[_-]?key[\s]*[:=][\s]*)([a-zA-Z0-9._-]{20,})`)
	patterns["access_token"] = regexp.MustCompile(`(?i)(access[_-]?token[\s]*[:=][\s]*)([a-zA-Z0-9._-]{20,})`)
	patterns["refresh_token"] = regexp.MustCompile(`(?i)(refresh[_-]?token[\s]*[:=][\s]*)([a-zA-Z0-9._-]{20,})`)
	
	// AWS credentials
	patterns["aws_access_key"] = regexp.MustCompile(`(AKIA[0-9A-Z]{16})`)
	patterns["aws_secret_key"] = regexp.MustCompile(`(?i)(aws[_-]?secret[_-]?access[_-]?key[\s]*[:=][\s]*)([A-Za-z0-9/+=]{40})`)
	patterns["aws_session_token"] = regexp.MustCompile(`(?i)(aws[_-]?session[_-]?token[\s]*[:=][\s]*)([A-Za-z0-9/+=]{100,})`)
	
	// Database credentials
	patterns["db_password"] = regexp.MustCompile(`(?i)(password[\s]*[:=][\s]*)([^\s"'\n]{8,})`)
	patterns["connection_string"] = regexp.MustCompile(`(?i)(postgresql://|mysql://|mongodb://)[^\s"'\n]*:[^\s"'\n]*@[^\s"'\n]*`)
	
	// Private keys and certificates
	patterns["private_key"] = regexp.MustCompile(`-----BEGIN[A-Z\s]*PRIVATE KEY-----[\s\S]*?-----END[A-Z\s]*PRIVATE KEY-----`)
	patterns["rsa_private"] = regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----[\s\S]*?-----END RSA PRIVATE KEY-----`)
	patterns["certificate"] = regexp.MustCompile(`-----BEGIN CERTIFICATE-----[\s\S]*?-----END CERTIFICATE-----`)
	
	// JWT tokens
	patterns["jwt"] = regexp.MustCompile(`(eyJ[a-zA-Z0-9_-]*\.eyJ[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*)`)
	
	// Email addresses (in sensitive contexts)
	patterns["email"] = regexp.MustCompile(`([a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,})`)
	
	// Phone numbers (international format)
	patterns["phone"] = regexp.MustCompile(`(\+?[1-9]\d{1,14})`)
	
	// IP addresses (in auth contexts)
	patterns["ip_address"] = regexp.MustCompile(`\b(?:[0-9]{1,3}\.){3}[0-9]{1,3}\b`)
	
	// Credit card numbers
	patterns["credit_card"] = regexp.MustCompile(`\b(?:4[0-9]{12}(?:[0-9]{3})?|5[1-5][0-9]{14}|3[47][0-9]{13}|3[0-9]{13}|6(?:011|5[0-9]{2})[0-9]{12})\b`)
	
	// Social Security Numbers
	patterns["ssn"] = regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`)
	
	// Session IDs
	patterns["session_id"] = regexp.MustCompile(`(?i)(session[_-]?id[\s]*[:=][\s]*)([a-zA-Z0-9._-]{20,})`)
	
	// Authorization headers
	patterns["auth_header"] = regexp.MustCompile(`(?i)(authorization[\s]*:[\s]*)(.+)`)
	
	// Wallet addresses (ActivityPub context)
	patterns["wallet_address"] = regexp.MustCompile(`(0x[a-fA-F0-9]{40}|[13][a-km-zA-HJ-NP-Z1-9]{25,34}|bc1[a-z0-9]{39,59})`)
	
	// Recovery codes
	patterns["recovery_code"] = regexp.MustCompile(`([A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4})`)
	
	// WebAuthn credentials
	patterns["webauthn_credential"] = regexp.MustCompile(`(?i)(credential[_-]?id[\s]*[:=][\s]*)([A-Za-z0-9+/=]{20,})`)
	
	return &SensitiveDataScrubber{
		patterns: patterns,
		enabled:  true,
	}
}

// ScrubString removes sensitive data from a string
func (s *SensitiveDataScrubber) ScrubString(input string) string {
	if !s.enabled {
		return input
	}
	
	result := input
	
	for patternName, regex := range s.patterns {
		switch patternName {
		case "bearer_token", "api_key", "access_token", "refresh_token", "session_id", "auth_header":
			result = regex.ReplaceAllStringFunc(result, func(match string) string {
				parts := regex.FindStringSubmatch(match)
				if len(parts) >= 3 {
					return parts[1] + "[REDACTED]"
				}
				return "[REDACTED]"
			})
		case "email":
			result = regex.ReplaceAllStringFunc(result, func(match string) string {
				parts := strings.Split(match, "@")
				if len(parts) == 2 {
					return parts[0][:min(3, len(parts[0]))] + "***@" + parts[1]
				}
				return "[EMAIL_REDACTED]"
			})
		case "phone":
			result = regex.ReplaceAllString(result, "[PHONE_REDACTED]")
		case "ip_address":
			result = regex.ReplaceAllStringFunc(result, func(match string) string {
				parts := strings.Split(match, ".")
				if len(parts) == 4 {
					return parts[0] + "." + parts[1] + ".***." + parts[3]
				}
				return "[IP_REDACTED]"
			})
		default:
			result = regex.ReplaceAllString(result, "["+strings.ToUpper(patternName)+"_REDACTED]")
		}
	}
	
	return result
}

// ScrubJSON scrubs sensitive data from JSON objects
func (s *SensitiveDataScrubber) ScrubJSON(input map[string]interface{}) map[string]interface{} {
	if !s.enabled {
		return input
	}
	
	result := make(map[string]interface{})
	sensitiveFields := map[string]bool{
		"password":           true,
		"token":             true,
		"access_token":      true,
		"refresh_token":     true,
		"api_key":           true,
		"secret":            true,
		"private_key":       true,
		"authorization":     true,
		"bearer":            true,
		"session_id":        true,
		"credential_id":     true,
		"recovery_code":     true,
		"client_secret":     true,
		"webhook_secret":    true,
		"signing_key":       true,
		"encryption_key":    true,
		"wallet_address":    true,
		"private_key_pem":   true,
		"jwt":               true,
	}
	
	for key, value := range input {
		lowerKey := strings.ToLower(key)
		
		// Check if field name indicates sensitive data
		isSensitive := false
		for sensitiveField := range sensitiveFields {
			if strings.Contains(lowerKey, sensitiveField) {
				isSensitive = true
				break
			}
		}
		
		if isSensitive {
			result[key] = "[REDACTED]"
		} else {
			switch v := value.(type) {
			case string:
				result[key] = s.ScrubString(v)
			case map[string]interface{}:
				result[key] = s.ScrubJSON(v)
			case []interface{}:
				result[key] = s.scrubArray(v)
			default:
				result[key] = value
			}
		}
	}
	
	return result
}

// scrubArray handles arrays in JSON data
func (s *SensitiveDataScrubber) scrubArray(input []interface{}) []interface{} {
	result := make([]interface{}, len(input))
	
	for i, item := range input {
		switch v := item.(type) {
		case string:
			result[i] = s.ScrubString(v)
		case map[string]interface{}:
			result[i] = s.ScrubJSON(v)
		case []interface{}:
			result[i] = s.scrubArray(v)
		default:
			result[i] = item
		}
	}
	
	return result
}

// Enable turns on scrubbing
func (s *SensitiveDataScrubber) Enable() {
	s.enabled = true
}

// Disable turns off scrubbing
func (s *SensitiveDataScrubber) Disable() {
	s.enabled = false
}

// IsEnabled returns whether scrubbing is enabled
func (s *SensitiveDataScrubber) IsEnabled() bool {
	return s.enabled
}

// ScrubbingCore wraps a zapcore.Core to automatically scrub sensitive data
type ScrubbingCore struct {
	zapcore.Core
	scrubber *SensitiveDataScrubber
}

// NewScrubbingCore creates a new core that automatically scrubs logs
func NewScrubbingCore(core zapcore.Core, scrubber *SensitiveDataScrubber) *ScrubbingCore {
	return &ScrubbingCore{
		Core:     core,
		scrubber: scrubber,
	}
}

// Write implements zapcore.Core interface with scrubbing
func (c *ScrubbingCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	if c.scrubber.IsEnabled() {
		// Scrub the message
		entry.Message = c.scrubber.ScrubString(entry.Message)
		
		// Scrub field values
		scrubbedFields := make([]zapcore.Field, len(fields))
		for i, field := range fields {
			scrubbedFields[i] = c.scrubField(field)
		}
		fields = scrubbedFields
	}
	
	return c.Core.Write(entry, fields)
}

// scrubField scrubs individual zap fields
func (c *ScrubbingCore) scrubField(field zapcore.Field) zapcore.Field {
	switch field.Type {
	case zapcore.StringType:
		field.String = c.scrubber.ScrubString(field.String)
	case zapcore.ByteStringType:
		field.String = c.scrubber.ScrubString(field.String)
	case zapcore.ReflectType:
		// Handle structured data
		if data, ok := field.Interface.(map[string]interface{}); ok {
			field.Interface = c.scrubber.ScrubJSON(data)
		} else if str, ok := field.Interface.(string); ok {
			field.Interface = c.scrubber.ScrubString(str)
		}
	}
	
	return field
}

// ProductionLoggerConfig creates a production logger configuration with scrubbing
func ProductionLoggerConfig(scrubber *SensitiveDataScrubber) zap.Config {
	config := zap.NewProductionConfig()
	
	// Enhanced production settings
	config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	config.Development = false
	config.DisableCaller = false
	config.DisableStacktrace = false
	config.Sampling = &zap.SamplingConfig{
		Initial:    100,
		Thereafter: 100,
	}
	
	// Enhanced field configuration
	config.EncoderConfig = zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime: func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
			enc.AppendString(t.UTC().Format(time.RFC3339))
		},
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}
	
	return config
}

// NewProductionLoggerWithScrubbing creates a production logger with automatic scrubbing
func NewProductionLoggerWithScrubbing() (*zap.Logger, error) {
	scrubber := NewSensitiveDataScrubber()
	config := ProductionLoggerConfig(scrubber)
	
	core, err := config.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build logger config: %w", err)
	}
	
	// Wrap the core with scrubbing
	scrubbingCore := NewScrubbingCore(core.Core(), scrubber)
	logger := zap.New(scrubbingCore, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	
	return logger, nil
}

// LoggerMiddleware provides context-aware logging with automatic scrubbing
type LoggerMiddleware struct {
	logger   *zap.Logger
	scrubber *SensitiveDataScrubber
}

// NewLoggerMiddleware creates a new logger middleware
func NewLoggerMiddleware(logger *zap.Logger, scrubber *SensitiveDataScrubber) *LoggerMiddleware {
	return &LoggerMiddleware{
		logger:   logger,
		scrubber: scrubber,
	}
}

// WithContext adds structured logging context with automatic scrubbing
func (m *LoggerMiddleware) WithContext(ctx context.Context, fields ...zap.Field) *zap.Logger {
	// Extract correlation ID from context if available
	if correlationID := ctx.Value("correlation_id"); correlationID != nil {
		fields = append(fields, zap.String("correlation_id", correlationID.(string)))
	}
	
	// Extract user ID from context if available
	if userID := ctx.Value("user_id"); userID != nil {
		fields = append(fields, zap.String("user_id", userID.(string)))
	}
	
	// Extract request ID from context if available
	if requestID := ctx.Value("request_id"); requestID != nil {
		fields = append(fields, zap.String("request_id", requestID.(string)))
	}
	
	return m.logger.With(fields...)
}

// LogSafeError logs an error with automatic scrubbing of sensitive data
func (m *LoggerMiddleware) LogSafeError(ctx context.Context, message string, err error, fields ...zap.Field) {
	if err != nil {
		// Scrub error message
		errorMessage := m.scrubber.ScrubString(err.Error())
		fields = append(fields, zap.String("error", errorMessage))
	}
	
	logger := m.WithContext(ctx, fields...)
	logger.Error(m.scrubber.ScrubString(message))
}

// LogSafeInfo logs info with automatic scrubbing
func (m *LoggerMiddleware) LogSafeInfo(ctx context.Context, message string, fields ...zap.Field) {
	logger := m.WithContext(ctx, fields...)
	logger.Info(m.scrubber.ScrubString(message))
}

// LogSafeWarn logs warning with automatic scrubbing
func (m *LoggerMiddleware) LogSafeWarn(ctx context.Context, message string, fields ...zap.Field) {
	logger := m.WithContext(ctx, fields...)
	logger.Warn(m.scrubber.ScrubString(message))
}

// LogSafeDebug logs debug info with automatic scrubbing
func (m *LoggerMiddleware) LogSafeDebug(ctx context.Context, message string, fields ...zap.Field) {
	logger := m.WithContext(ctx, fields...)
	logger.Debug(m.scrubber.ScrubString(message))
}

// AuditLogger provides enhanced audit logging for security events
type AuditLogger struct {
	logger   *zap.Logger
	scrubber *SensitiveDataScrubber
}

// NewAuditLogger creates a new audit logger
func NewAuditLogger(logger *zap.Logger, scrubber *SensitiveDataScrubber) *AuditLogger {
	return &AuditLogger{
		logger:   logger.Named("audit"),
		scrubber: scrubber,
	}
}

// LogAuthenticationEvent logs authentication events with scrubbing
func (a *AuditLogger) LogAuthenticationEvent(ctx context.Context, event string, userID string, success bool, metadata map[string]interface{}) {
	scrubbedMetadata := a.scrubber.ScrubJSON(metadata)
	
	a.logger.Info("authentication_event",
		zap.String("event_type", event),
		zap.String("user_id", userID),
		zap.Bool("success", success),
		zap.Any("metadata", scrubbedMetadata),
		zap.Time("timestamp", time.Now()),
	)
}

// LogAuthorizationEvent logs authorization events with scrubbing
func (a *AuditLogger) LogAuthorizationEvent(ctx context.Context, action string, resource string, userID string, granted bool, reason string) {
	a.logger.Info("authorization_event",
		zap.String("action", action),
		zap.String("resource", resource),
		zap.String("user_id", userID),
		zap.Bool("granted", granted),
		zap.String("reason", a.scrubber.ScrubString(reason)),
		zap.Time("timestamp", time.Now()),
	)
}

// LogSecurityEvent logs security-related events
func (a *AuditLogger) LogSecurityEvent(ctx context.Context, eventType string, severity string, description string, metadata map[string]interface{}) {
	scrubbedMetadata := a.scrubber.ScrubJSON(metadata)
	
	a.logger.Warn("security_event",
		zap.String("event_type", eventType),
		zap.String("severity", severity),
		zap.String("description", a.scrubber.ScrubString(description)),
		zap.Any("metadata", scrubbedMetadata),
		zap.Time("timestamp", time.Now()),
	)
}

// Helper function for minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Global scrubber instance
var globalScrubber = NewSensitiveDataScrubber()

// GetGlobalScrubber returns the global scrubber instance
func GetGlobalScrubber() *SensitiveDataScrubber {
	return globalScrubber
}

// ScrubString is a convenience function using the global scrubber
func ScrubString(input string) string {
	return globalScrubber.ScrubString(input)
}

// ScrubJSON is a convenience function using the global scrubber
func ScrubJSON(input map[string]interface{}) map[string]interface{} {
	return globalScrubber.ScrubJSON(input)
}