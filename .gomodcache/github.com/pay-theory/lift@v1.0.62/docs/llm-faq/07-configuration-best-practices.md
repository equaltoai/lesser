# Lift Application Configuration Settings Best Practices

## Overview

This guide covers best practices for configuring Lift applications, including configuration structure, environment-specific settings, security considerations, and patterns for maintainable configuration management in production environments.

## Configuration Fundamentals

### The Config Structure

Lift's `Config` struct contains all application-wide settings:

```go
type Config struct {
    // Request/Response Limits
    MaxRequestSize  int64    // Maximum request body size in bytes
    MaxResponseSize int64    // Maximum response body size in bytes
    
    // Timeouts
    Timeout         int      // Request timeout in seconds
    
    // Logging
    LogLevel        string   // Log level: DEBUG, INFO, WARN, ERROR
    
    // Features
    MetricsEnabled  bool     // Enable CloudWatch metrics
    TracingEnabled  bool     // Enable AWS X-Ray tracing
    Debug           bool     // Enable debug mode
    
    // CORS
    CORSEnabled     bool     // Enable CORS middleware
    AllowedOrigins  []string // CORS allowed origins
    
    // Multi-Tenancy
    RequireTenantID bool     // Require tenant ID in requests
}
```

### Default Values

```go
config := lift.DefaultConfig()
// Returns:
// MaxRequestSize:  10 * 1024 * 1024  (10MB)
// MaxResponseSize: 6 * 1024 * 1024   (6MB)
// Timeout:         30                 (seconds)
// LogLevel:        "INFO"
// MetricsEnabled:  true
// TracingEnabled:  false
// Debug:           false
// CORSEnabled:     true
// AllowedOrigins:  ["*"]
// RequireTenantID: false
```

## Best Practice #1: Environment-Based Configuration

### Pattern: Configuration from Environment Variables

**Always configure from environment variables in production:**

```go
func loadConfig() *lift.Config {
    return &lift.Config{
        MaxRequestSize:  getEnvInt64("MAX_REQUEST_SIZE", 10*1024*1024),
        MaxResponseSize: getEnvInt64("MAX_RESPONSE_SIZE", 6*1024*1024),
        Timeout:         getEnvInt("TIMEOUT", 25),
        LogLevel:        getEnv("LOG_LEVEL", "INFO"),
        MetricsEnabled:  getEnvBool("METRICS_ENABLED", true),
        TracingEnabled:  getEnvBool("TRACING_ENABLED", false),
        Debug:           getEnvBool("DEBUG", false),
        CORSEnabled:     getEnvBool("CORS_ENABLED", true),
        AllowedOrigins:  getEnvStringSlice("ALLOWED_ORIGINS", []string{"*"}),
        RequireTenantID: getEnvBool("REQUIRE_TENANT_ID", false),
    }
}

func getEnv(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
    if value := os.Getenv(key); value != "" {
        if intVal, err := strconv.Atoi(value); err == nil {
            return intVal
        }
    }
    return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
    if value := os.Getenv(key); value != "" {
        if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
            return intVal
        }
    }
    return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
    if value := os.Getenv(key); value != "" {
        if boolVal, err := strconv.ParseBool(value); err == nil {
            return boolVal
        }
    }
    return defaultValue
}

func getEnvStringSlice(key string, defaultValue []string) []string {
    if value := os.Getenv(key); value != "" {
        return strings.Split(value, ",")
    }
    return defaultValue
}
```

### Usage

```go
func main() {
    app := lift.New()
    
    // Load configuration from environment
    config := loadConfig()
    app.WithConfig(config)
    
    // Rest of application setup...
}
```

### Environment Variable Examples

```bash
# Production settings
export LOG_LEVEL=INFO
export TIMEOUT=25
export METRICS_ENABLED=true
export TRACING_ENABLED=true
export DEBUG=false
export CORS_ENABLED=true
export ALLOWED_ORIGINS=https://app.example.com,https://www.example.com
export REQUIRE_TENANT_ID=true

# Development settings
export LOG_LEVEL=DEBUG
export TIMEOUT=60
export METRICS_ENABLED=false
export TRACING_ENABLED=false
export DEBUG=true
export CORS_ENABLED=true
export ALLOWED_ORIGINS=*
export REQUIRE_TENANT_ID=false
```

## Best Practice #2: Environment-Specific Configurations

### Pattern: Configuration by Environment

```go
type Environment string

const (
    EnvDevelopment Environment = "development"
    EnvStaging     Environment = "staging"
    EnvProduction  Environment = "production"
)

func getEnvironment() Environment {
    env := os.Getenv("ENVIRONMENT")
    switch env {
    case "production", "prod":
        return EnvProduction
    case "staging", "stage":
        return EnvStaging
    default:
        return EnvDevelopment
    }
}

func loadConfigForEnvironment() *lift.Config {
    env := getEnvironment()
    
    switch env {
    case EnvProduction:
        return productionConfig()
    case EnvStaging:
        return stagingConfig()
    default:
        return developmentConfig()
    }
}

func developmentConfig() *lift.Config {
    return &lift.Config{
        MaxRequestSize:  10 * 1024 * 1024,
        MaxResponseSize: 6 * 1024 * 1024,
        Timeout:         60, // Longer timeout for debugging
        LogLevel:        "DEBUG",
        MetricsEnabled:  false, // No metrics in dev
        TracingEnabled:  false, // No tracing in dev
        Debug:           true,  // Debug mode on
        CORSEnabled:     true,
        AllowedOrigins:  []string{"*"}, // Permissive CORS
        RequireTenantID: false, // Optional tenant ID
    }
}

func stagingConfig() *lift.Config {
    return &lift.Config{
        MaxRequestSize:  10 * 1024 * 1024,
        MaxResponseSize: 6 * 1024 * 1024,
        Timeout:         30,
        LogLevel:        "INFO",
        MetricsEnabled:  true,  // Enable metrics
        TracingEnabled:  true,  // Enable tracing
        Debug:           false,
        CORSEnabled:     true,
        AllowedOrigins:  []string{"https://staging.example.com"},
        RequireTenantID: true, // Require tenant ID
    }
}

func productionConfig() *lift.Config {
    return &lift.Config{
        MaxRequestSize:  5 * 1024 * 1024,  // Stricter limit
        MaxResponseSize: 6 * 1024 * 1024,
        Timeout:         25, // Leave buffer for Lambda timeout
        LogLevel:        "INFO", // Or WARN for less verbose
        MetricsEnabled:  true,
        TracingEnabled:  true,
        Debug:           false,
        CORSEnabled:     true,
        AllowedOrigins:  []string{
            "https://app.example.com",
            "https://www.example.com",
        },
        RequireTenantID: true, // Always require in production
    }
}
```

## Best Practice #3: Request Size Limits

### Why Size Limits Matter

- **Lambda payload limits:** 6MB for synchronous invocations
- **API Gateway limits:** 10MB for HTTP API, 6MB for REST API
- **Performance:** Larger payloads increase cold start time
- **Cost:** More data transfer costs
- **Security:** Prevent DoS attacks via large payloads

### Recommended Size Limits

```go
// API Gateway HTTP API (10MB max)
config := &lift.Config{
    MaxRequestSize: 10 * 1024 * 1024, // 10MB
    MaxResponseSize: 6 * 1024 * 1024, // 6MB (Lambda limit)
}

// API Gateway REST API (6MB max)
config := &lift.Config{
    MaxRequestSize: 6 * 1024 * 1024, // 6MB
    MaxResponseSize: 6 * 1024 * 1024, // 6MB
}

// Internal API (tighter limits)
config := &lift.Config{
    MaxRequestSize: 1 * 1024 * 1024, // 1MB
    MaxResponseSize: 1 * 1024 * 1024, // 1MB
}

// File upload endpoint (larger limit)
config := &lift.Config{
    MaxRequestSize: 50 * 1024 * 1024, // 50MB (use S3 presigned URLs instead)
    MaxResponseSize: 1 * 1024 * 1024, // 1MB
}
```

### Size Limit Best Practices

```go
// ✅ GOOD: Different limits for different endpoints
func main() {
    app := lift.New()
    
    // Default config with normal limits
    app.WithConfig(&lift.Config{
        MaxRequestSize: 1 * 1024 * 1024, // 1MB default
    })
    
    // Standard API endpoints
    api := app.Group("/api/v1")
    api.POST("/users", CreateUser)
    
    // Special endpoint with larger limit (use middleware)
    uploads := app.Group("/uploads")
    uploads.Use(increaseSizeLimitMiddleware(10 * 1024 * 1024))
    uploads.POST("/files", UploadFile)
}

// ❌ AVOID: Excessively large limits
config := &lift.Config{
    MaxRequestSize: 100 * 1024 * 1024, // 100MB - too large!
}
```

## Best Practice #4: Timeout Configuration

### Lambda Timeout Considerations

Your Lift timeout should be **less than** your Lambda function timeout to allow graceful shutdown:

```go
// Lambda timeout: 30 seconds
// Lift timeout: 25 seconds (leaving 5 second buffer)
config := &lift.Config{
    Timeout: 25,
}
```

### Timeout by Use Case

```go
// Quick API responses
config := &lift.Config{
    Timeout: 10, // 10 seconds
}

// Standard API operations
config := &lift.Config{
    Timeout: 25, // 25 seconds (Lambda: 30s)
}

// Batch processing
config := &lift.Config{
    Timeout: 295, // ~5 minutes (Lambda: 300s max)
}

// Background jobs
config := &lift.Config{
    Timeout: 890, // ~15 minutes (Lambda: 900s max)
}
```

### Dynamic Timeout Based on Route

```go
func main() {
    app := lift.New()
    
    // Default timeout
    app.WithConfig(&lift.Config{
        Timeout: 25,
    })
    
    // Quick endpoints
    app.GET("/health", HealthCheck)
    
    // Long-running operations (use context timeout)
    app.POST("/reports/generate", func(ctx *lift.Context) error {
        // Create context with longer timeout
        longCtx, cancel := context.WithTimeout(ctx.Context, 5*time.Minute)
        defer cancel()
        
        report, err := generateReport(longCtx)
        if err != nil {
            return lift.SystemError("report generation failed")
        }
        
        return ctx.JSON(report)
    })
}
```

## Best Practice #5: Logging Configuration

### Log Levels by Environment

```go
// Development: DEBUG - see everything
LogLevel: "DEBUG"

// Staging: INFO - standard operations
LogLevel: "INFO"

// Production: INFO or WARN - reduce noise
LogLevel: "INFO"  // or "WARN" for high-traffic apps
```

### Structured Logging Best Practices

```go
func main() {
    app := lift.New()
    
    config := &lift.Config{
        LogLevel: getEnv("LOG_LEVEL", "INFO"),
    }
    app.WithConfig(config)
    
    // Logger middleware uses configured log level
    app.Use(middleware.Logger())
}

// In handlers, use structured logging
func CreateUser(ctx *lift.Context) error {
    ctx.Logger.Info("Creating user",
        "tenant_id", ctx.TenantID(),
        "request_id", ctx.RequestID(),
    )
    
    user, err := userService.Create(ctx.Context, req)
    if err != nil {
        ctx.Logger.Error("User creation failed",
            "error", err,
            "tenant_id", ctx.TenantID(),
            "request_id", ctx.RequestID(),
        )
        return lift.SystemError("failed to create user")
    }
    
    ctx.Logger.Info("User created successfully",
        "user_id", user.ID,
        "tenant_id", ctx.TenantID(),
    )
    
    return ctx.JSON(user)
}
```

### Log Level Guidelines

- **DEBUG:** Development only - verbose output
- **INFO:** Standard production logging - successful operations
- **WARN:** Production warnings - recoverable errors
- **ERROR:** Production errors - failures requiring attention

## Best Practice #6: Observability Configuration

### Metrics and Tracing

```go
// Development: Metrics off, tracing off (reduce costs)
config := &lift.Config{
    MetricsEnabled: false,
    TracingEnabled: false,
}

// Staging: Metrics on, tracing on (full observability)
config := &lift.Config{
    MetricsEnabled: true,
    TracingEnabled: true,
}

// Production: Metrics always, tracing with sampling
config := &lift.Config{
    MetricsEnabled: true,
    TracingEnabled: true, // Use sampling rate in observability middleware
}

// Add observability middleware with sampling
app.Use(middleware.EnhancedObservabilityMiddleware(
    middleware.EnhancedObservabilityConfig{
        EnableLogging: true,
        EnableMetrics: true,
        EnableTracing: true,
        SampleRate:    0.1, // 10% sampling to reduce costs
        DefaultTags: map[string]string{
            "environment": os.Getenv("ENVIRONMENT"),
            "service":     "my-app",
        },
    },
))
```

### Cost Optimization

```go
// High-traffic production: Lower sampling
SampleRate: 0.01  // 1% - sufficient for patterns

// Medium-traffic production: Moderate sampling
SampleRate: 0.1   // 10% - good balance

// Low-traffic production: Higher sampling
SampleRate: 0.5   // 50% - more coverage

// Development/Staging: Full sampling
SampleRate: 1.0   // 100% - see everything
```

## Best Practice #7: CORS Configuration

### Development CORS

```go
// Development: Permissive CORS
config := &lift.Config{
    CORSEnabled:    true,
    AllowedOrigins: []string{"*"},
}
```

### Production CORS

```go
// Production: Strict CORS
config := &lift.Config{
    CORSEnabled: true,
    AllowedOrigins: []string{
        "https://app.example.com",
        "https://www.example.com",
        "https://admin.example.com",
    },
}

// Or from environment
AllowedOrigins: strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",")
```

### Advanced CORS with Middleware

```go
// For more control, use CORS middleware
corsConfig := middleware.CORSConfig{
    AllowOrigins:     []string{"https://example.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Authorization", "Content-Type"},
    ExposeHeaders:    []string{"X-Request-ID"},
    AllowCredentials: true,
    MaxAge:           3600,
}

app.Use(middleware.CORS(corsConfig))
```

## Best Practice #8: Multi-Tenant Configuration

### When to Require Tenant ID

```go
// SaaS applications - always require in production
config := &lift.Config{
    RequireTenantID: getEnv("ENVIRONMENT", "dev") == "production",
}

// Single-tenant applications - never require
config := &lift.Config{
    RequireTenantID: false,
}

// Development - optional for testing
config := &lift.Config{
    RequireTenantID: false,
}
```

### Enforcing Tenant Isolation

```go
func main() {
    app := lift.New()
    
    // Require tenant ID in production
    config := &lift.Config{
        RequireTenantID: os.Getenv("ENV") == "production",
    }
    app.WithConfig(config)
    
    // JWT middleware extracts tenant_id from token
    api := app.Group("/api/v1")
    api.Use(middleware.JWTAuth(middleware.JWTConfig{
        Secret: os.Getenv("JWT_SECRET"),
    }))
    
    // All handlers automatically get tenant context
    api.GET("/data", func(ctx *lift.Context) error {
        tenantID := ctx.TenantID() // From JWT
        
        // Query scoped to tenant
        data := db.Query("SELECT * FROM data WHERE tenant_id = ?", tenantID)
        
        return ctx.JSON(data)
    })
}
```

## Best Practice #9: Configuration Validation

### Validate Configuration at Startup

```go
func validateConfig(config *lift.Config) error {
    if config.MaxRequestSize <= 0 {
        return fmt.Errorf("MaxRequestSize must be positive")
    }
    
    if config.MaxResponseSize > 6*1024*1024 {
        return fmt.Errorf("MaxResponseSize cannot exceed 6MB (Lambda limit)")
    }
    
    if config.Timeout <= 0 {
        return fmt.Errorf("Timeout must be positive")
    }
    
    if config.Timeout > 900 {
        return fmt.Errorf("Timeout cannot exceed 900 seconds (Lambda limit)")
    }
    
    validLogLevels := map[string]bool{
        "DEBUG": true, "INFO": true, "WARN": true, "ERROR": true,
    }
    if !validLogLevels[config.LogLevel] {
        return fmt.Errorf("Invalid LogLevel: %s", config.LogLevel)
    }
    
    return nil
}

func main() {
    app := lift.New()
    
    config := loadConfig()
    
    // Validate before applying
    if err := validateConfig(config); err != nil {
        log.Fatalf("Invalid configuration: %v", err)
    }
    
    app.WithConfig(config)
    // ...
}
```

## Best Practice #10: Configuration Documentation

### Document Your Configuration

```go
// config.go - Central configuration file

// AppConfig holds all application configuration
// Environment variables can override these defaults
type AppConfig struct {
    // Lift framework configuration
    Lift *lift.Config
    
    // Application-specific configuration
    DatabaseTable   string
    CacheEnabled    bool
    MaxConcurrency  int
    
    // AWS service configuration
    S3Bucket        string
    SQSQueueURL     string
    
    // External service configuration
    PaymentAPI      string
    PaymentAPIKey   string
}

// LoadConfig loads configuration from environment variables
// with sensible defaults for each environment
func LoadConfig() (*AppConfig, error) {
    env := getEnvironment()
    
    config := &AppConfig{
        Lift: &lift.Config{
            MaxRequestSize:  getEnvInt64("MAX_REQUEST_SIZE", 10*1024*1024),
            MaxResponseSize: getEnvInt64("MAX_RESPONSE_SIZE", 6*1024*1024),
            Timeout:         getEnvInt("TIMEOUT", 25),
            LogLevel:        getEnv("LOG_LEVEL", "INFO"),
            MetricsEnabled:  getEnvBool("METRICS_ENABLED", true),
            TracingEnabled:  getEnvBool("TRACING_ENABLED", env == EnvProduction),
            Debug:           getEnvBool("DEBUG", env == EnvDevelopment),
            CORSEnabled:     true,
            AllowedOrigins:  getAllowedOrigins(env),
            RequireTenantID: env == EnvProduction,
        },
        DatabaseTable:  getEnv("DYNAMODB_TABLE", "my-app-table"),
        CacheEnabled:   getEnvBool("CACHE_ENABLED", true),
        MaxConcurrency: getEnvInt("MAX_CONCURRENCY", 10),
        S3Bucket:       getEnv("S3_BUCKET", ""),
        SQSQueueURL:    getEnv("SQS_QUEUE_URL", ""),
        PaymentAPI:     getEnv("PAYMENT_API_URL", ""),
        PaymentAPIKey:  getEnv("PAYMENT_API_KEY", ""),
    }
    
    return config, validateAppConfig(config)
}
```

## Summary

### Key Configuration Best Practices

1. **Environment Variables:** Always use environment variables for configuration
2. **Environment-Specific:** Different configs for dev/staging/production
3. **Size Limits:** Set appropriate request/response size limits
4. **Timeouts:** Keep Lift timeout less than Lambda timeout
5. **Logging:** Use appropriate log levels per environment
6. **Observability:** Enable metrics in production, use sampling for tracing
7. **CORS:** Strict origins in production, permissive in development
8. **Multi-Tenant:** Require tenant ID in production SaaS applications
9. **Validation:** Validate configuration at startup
10. **Documentation:** Document configuration options and their purpose

### Recommended Production Configuration

```go
&lift.Config{
    MaxRequestSize:  5 * 1024 * 1024,  // 5MB
    MaxResponseSize: 6 * 1024 * 1024,  // 6MB
    Timeout:         25,                // 25 seconds (Lambda: 30s)
    LogLevel:        "INFO",
    MetricsEnabled:  true,
    TracingEnabled:  true,              // With sampling
    Debug:           false,
    CORSEnabled:     true,
    AllowedOrigins:  []string{"https://app.example.com"},
    RequireTenantID: true,              // For SaaS apps
}
```

Proper configuration is essential for building secure, performant, and cost-effective Lambda functions with Lift. Follow these best practices to ensure your applications are production-ready.



